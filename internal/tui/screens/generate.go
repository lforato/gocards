package screens

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lforato/vimtea"
	"golang.design/x/clipboard"

	"github.com/lforato/gocards/internal/ai"
	"github.com/lforato/gocards/internal/i18n"
	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/store"
	"github.com/lforato/gocards/internal/tui"
)

const (
	chatTimeout   = 2 * time.Minute
	chatInputMinH = 1
	chatInputMaxH = 15
)

type chatSubmitMsg struct{ content string }

type generateDecksLoadedMsg struct {
	decks []models.Deck
	err   error
}

type cardsSavedMsg struct {
	n   int
	err error
}

// AIGenerate is a chat-based authoring screen. The model streams a reply,
// the caller extracts any <card>…</card> blocks from it, and the user
// approves/rejects each one before the accepted set is persisted.
type AIGenerate struct {
	store *store.Store
	deck  models.Deck

	history []models.GradingMessage

	pendingReply string
	streaming    bool
	ctx          context.Context
	cancel       context.CancelFunc
	streamCh     <-chan ai.Event
	spin         spinner.Model
	lastErr      error

	chatViewport viewport.Model
	input        vimtea.Editor
	inputH       int

	pickerOpen   bool
	pickerDecks  []models.Deck
	pickerCursor int

	proposed  []store.CardInput
	reviewIdx int
	reviewing bool
	accepted  []store.CardInput

	w, h int
}

func NewAIGenerate(s *store.Store, deck models.Deck) *AIGenerate {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	in := vimtea.NewEditor(
		vimtea.WithContent(""),
		vimtea.WithEnableStatusBar(false),
		vimtea.WithHideLineNumbers(),
		vimtea.WithFileName("prompt.md"),
	)
	submit := func(b vimtea.Buffer) tea.Cmd {
		text := b.Text()
		return func() tea.Msg { return chatSubmitMsg{content: text} }
	}
	in.AddBinding(vimtea.KeyBinding{Key: "ctrl+s", Mode: vimtea.ModeNormal, Handler: submit})
	in.AddBinding(vimtea.KeyBinding{Key: "ctrl+s", Mode: vimtea.ModeInsert, Handler: submit})
	in.SetSize(60, 1)

	return &AIGenerate{
		store:        s,
		deck:         deck,
		spin:         sp,
		input:        in,
		inputH:       1,
		chatViewport: viewport.New(80, 12),
	}
}

func (g *AIGenerate) Init() tea.Cmd {
	// Chat input starts in insert mode so plain typing and pastes land as
	// text instead of triggering vim commands. Users can still press esc to
	// drop into normal mode for editing.
	return tea.Batch(g.input.Init(), g.input.SetMode(vimtea.ModeInsert), g.loadDecks())
}

func (g *AIGenerate) loadDecks() tea.Cmd {
	return func() tea.Msg {
		decks, err := g.store.ListDecksByLanguage(string(i18n.CurrentLang()))
		return generateDecksLoadedMsg{decks: decks, err: err}
	}
}

func (g *AIGenerate) Update(msg tea.Msg) (tui.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		g.w, g.h = m.Width, m.Height
		g.resizeInner()
		return g, nil

	case generateDecksLoadedMsg:
		g.applyDecksLoaded(m)
		return g, nil

	case tui.LangChangedMsg:
		return g, g.loadDecks()

	case spinner.TickMsg:
		if !g.streaming {
			return g, nil
		}
		var cmd tea.Cmd
		g.spin, cmd = g.spin.Update(m)
		return g, cmd

	case streamChunkMsg:
		g.pendingReply += m.text
		g.refreshTranscript()
		g.chatViewport.GotoBottom()
		return g, pumpStream(g.streamCh)

	case streamDoneMsg:
		return g, g.finishStream(m)

	case streamErrMsg:
		g.streaming = false
		g.lastErr = m.err
		return g, nil

	case cardsSavedMsg:
		return g.handleSaved(m)

	case chatSubmitMsg:
		return g.submitChat(m.content)

	case tea.KeyMsg:
		return g.handleKey(m)
	}

	if !g.pickerOpen && !g.reviewing {
		_, cmd := g.input.Update(msg)
		return g, cmd
	}
	return g, nil
}

func (g *AIGenerate) applyDecksLoaded(m generateDecksLoadedMsg) {
	if m.err != nil {
		return
	}
	g.pickerDecks = m.decks
	for i, d := range m.decks {
		if d.ID == g.deck.ID {
			g.pickerCursor = i
			return
		}
	}
}

func (g *AIGenerate) finishStream(m streamDoneMsg) tea.Cmd {
	reply := m.full
	if reply == "" {
		reply = g.pendingReply
	}
	g.streaming = false
	g.history = append(g.history, models.GradingMessage{Role: "assistant", Content: reply})
	g.pendingReply = ""
	g.refreshTranscript()
	g.chatViewport.GotoBottom()

	cards := extractProposedCards(reply)
	if len(cards) > 0 {
		g.proposed = cards
		g.reviewIdx = 0
		g.accepted = nil
		g.reviewing = true
	}
	return nil
}

func (g *AIGenerate) handleSaved(m cardsSavedMsg) (tui.Screen, tea.Cmd) {
	g.accepted = nil
	if m.err != nil {
		return g, tui.ToastErr(i18n.T(i18n.KeyGenerateSavedPfx) + m.err.Error())
	}
	return g, tui.Toast(i18n.Tf(i18n.KeyGenerateSavedFmt, m.n, plural(m.n), g.deck.Name))
}

func (g *AIGenerate) submitChat(text string) (tui.Screen, tea.Cmd) {
	text = strings.TrimSpace(text)
	if text == "" || g.streaming {
		return g, nil
	}
	g.input.GetBuffer().Clear()
	g.fitInput()
	g.history = append(g.history, models.GradingMessage{Role: "user", Content: text})
	g.lastErr = nil
	g.refreshTranscript()
	g.chatViewport.GotoBottom()
	return g, g.startStream()
}

func (g *AIGenerate) handleKey(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	key := m.String()

	if g.pickerOpen {
		return g.handlePickerKey(m)
	}
	if g.reviewing {
		return g.handleReviewKey(m)
	}

	if g.handleChatScroll(m) {
		return g, nil
	}

	if key == "ctrl+d" {
		g.pickerOpen = true
		return g, nil
	}

	// Esc in normal mode is the user's "I'm done" gesture. In insert/visual,
	// vim swallows it to return to normal mode first.
	if key == "esc" && g.input.GetMode() == vimtea.ModeNormal {
		if g.cancel != nil {
			g.cancel()
		}
		return g, navBack
	}

	if m.Paste {
		return g.handlePaste(m)
	}

	// Vim's normal-mode `p`/`P` reads the system clipboard internally. If
	// the clipboard is multi-line, the editor's internal viewport ratchets
	// up during insertion at the old (1-row) height. Pre-size the input
	// first so vim has room to scroll correctly.
	if g.input.GetMode() == vimtea.ModeNormal && (key == "p" || key == "P") {
		g.presizeForVimPaste()
	}

	m = normalizeNewlineKey(m)
	_, cmd := g.input.Update(m)
	g.fitInput()
	return g, cmd
}

func (g *AIGenerate) presizeForVimPaste() {
	data := clipboard.Read(clipboard.FmtText)
	if len(data) == 0 {
		return
	}
	extra := strings.Count(string(data), "\n")
	target := g.clampInputH(g.input.GetBuffer().LineCount() + extra)
	if target == g.inputH {
		return
	}
	g.inputH = target
	g.resizeInner()
}

// normalizeNewlineKey folds raw LF into KeyEnter so pastes coming through
// channels that don't use bracketed paste (nvim :term, tmux send-keys,
// direct piping) produce real line breaks instead of dropping newlines.
// Bubbletea maps \r to KeyEnter but leaves \n as KeyCtrlJ, and vimtea
// ignores ctrl+j in insert mode.
func normalizeNewlineKey(m tea.KeyMsg) tea.KeyMsg {
	if m.Type == tea.KeyCtrlJ {
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	for _, r := range m.Runes {
		if r == '\n' {
			return tea.KeyMsg{Type: tea.KeyEnter}
		}
	}
	return m
}

// handlePaste feeds a bracketed-paste message into the vim input one rune
// at a time because vimtea's insert mode drops keys whose .String() is
// longer than one character. Two things need special care:
//
//  1. Normal-mode pastes are coerced into insert mode first so the runes
//     don't run as vim commands.
//  2. We pre-size the input to the final expected height before feeding
//     runes. Otherwise vimtea's internal viewport uses the pre-paste
//     height (1 row) while lines are being inserted and ratchets YOffset
//     up past the cursor, leaving the box rendering blank space after the
//     paste completes.
func (g *AIGenerate) handlePaste(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	var cmds []tea.Cmd
	if g.input.GetMode() != vimtea.ModeInsert {
		if cmd := g.input.SetMode(vimtea.ModeInsert); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	g.presizeForPaste(m.Runes)

	for _, r := range m.Runes {
		var synth tea.KeyMsg
		if r == '\n' || r == '\r' {
			synth = tea.KeyMsg{Type: tea.KeyEnter}
		} else {
			synth = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
		}
		if _, cmd := g.input.Update(synth); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	g.fitInput()
	return g, tea.Batch(cmds...)
}

// presizeForPaste grows the input to the height it will need AFTER the
// paste finishes, so vim's internal viewport doesn't overshoot during the
// per-rune insertion loop.
func (g *AIGenerate) presizeForPaste(runes []rune) {
	current := g.input.GetBuffer().LineCount()
	newlines := 0
	for _, r := range runes {
		if r == '\n' || r == '\r' {
			newlines++
		}
	}
	target := g.clampInputH(current + newlines)
	if target == g.inputH {
		return
	}
	g.inputH = target
	g.resizeInner()
}

// handleChatScroll maps scroll keys onto the transcript viewport so users
// can look back at previous turns without leaving the input focused.
func (g *AIGenerate) handleChatScroll(m tea.KeyMsg) bool {
	switch m.String() {
	case "shift+up":
		g.chatViewport.LineUp(1)
	case "shift+down":
		g.chatViewport.LineDown(1)
	case "pgup":
		g.chatViewport.HalfViewUp()
	case "pgdown", "pgdn":
		g.chatViewport.HalfViewDown()
	case "home":
		g.chatViewport.GotoTop()
	case "end":
		g.chatViewport.GotoBottom()
	default:
		return false
	}
	return true
}

func (g *AIGenerate) handlePickerKey(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	switch m.String() {
	case "esc":
		g.pickerOpen = false
	case "up", "k":
		g.pickerCursor = cursorUp(g.pickerCursor)
	case "down", "j":
		g.pickerCursor = cursorDown(g.pickerCursor, len(g.pickerDecks))
	case "enter":
		if g.pickerCursor < len(g.pickerDecks) {
			g.deck = g.pickerDecks[g.pickerCursor]
		}
		g.pickerOpen = false
	}
	return g, nil
}

func (g *AIGenerate) handleReviewKey(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	switch m.String() {
	case "esc":
		g.discardReview()
		return g, nil
	case "a", "y":
		if g.reviewIdx < len(g.proposed) {
			g.accepted = append(g.accepted, g.proposed[g.reviewIdx])
		}
		g.reviewIdx++
	case "r", "n":
		g.reviewIdx++
	}

	if g.reviewIdx >= len(g.proposed) {
		return g, g.flushAccepted()
	}
	return g, nil
}

func (g *AIGenerate) discardReview() {
	g.reviewing = false
	g.proposed = nil
	g.accepted = nil
	g.reviewIdx = 0
}

func (g *AIGenerate) flushAccepted() tea.Cmd {
	toSave := g.accepted
	deckID := g.deck.ID
	store := g.store
	g.discardReview()
	if len(toSave) == 0 {
		return nil
	}
	return func() tea.Msg {
		if _, err := store.BulkCreateCards(deckID, toSave); err != nil {
			return cardsSavedMsg{err: err}
		}
		return cardsSavedMsg{n: len(toSave)}
	}
}

func (g *AIGenerate) startStream() tea.Cmd {
	client, err := resolveAIClient(g.store)
	if err != nil {
		g.lastErr = err
		g.rewindQueuedUserTurn()
		g.refreshTranscript()
		return nil
	}
	g.ctx, g.cancel = context.WithTimeout(context.Background(), chatTimeout)
	g.streaming = true
	g.pendingReply = ""
	g.streamCh = client.Chat(g.ctx, g.deck.Name, g.deck.Description, g.history)
	return tea.Batch(g.spin.Tick, pumpStream(g.streamCh))
}

// rewindQueuedUserTurn drops the most recent user message when the send
// fails so history doesn't stay out of sync with what Claude saw.
func (g *AIGenerate) rewindQueuedUserTurn() {
	if len(g.history) > 0 && g.history[len(g.history)-1].Role == "user" {
		g.history = g.history[:len(g.history)-1]
	}
}

// clampInputH caps the chat input height at chatInputMaxH, but also at
// roughly half the terminal height so the input never swallows the
// transcript on a short terminal.
func (g *AIGenerate) clampInputH(n int) int {
	cap := chatInputMaxH
	if g.h > 0 {
		room := max(chatInputMinH, (g.h-8)/2)
		if room < cap {
			cap = room
		}
	}
	return max(chatInputMinH, min(cap, n))
}

func (g *AIGenerate) resizeInner() {
	w := g.w
	if w <= 0 {
		w = 80
	}
	h := g.h
	if h <= 0 {
		h = 20
	}
	// Chrome around the viewport: deck line + blank + blank-before-input +
	// bordered input (inputH + 2 border) + status line.
	inputH := g.clampInputH(g.inputH)
	g.chatViewport.Width = w
	g.chatViewport.Height = max(3, h-(3+inputH+2+1))
	g.input.SetSize(max(20, w-2), inputH)
	g.refreshTranscript()
}

func (g *AIGenerate) fitInput() {
	lines := g.clampInputH(g.input.GetBuffer().LineCount())
	if g.inputH == lines {
		return
	}
	g.inputH = lines
	g.resizeInner()
}

func (g *AIGenerate) refreshTranscript() {
	var b strings.Builder
	for i, m := range g.history {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(formatChatTurn(m.Role, m.Content, g.chatViewport.Width))
	}
	if g.streaming && g.pendingReply != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(formatChatTurn("assistant", g.pendingReply, g.chatViewport.Width))
	}
	g.chatViewport.SetContent(b.String())
}

func formatChatTurn(role, content string, width int) string {
	tag := tui.StylePrimary.Render(i18n.T(i18n.KeyGenerateYouTag))
	if role == "assistant" {
		tag = tui.StyleAccent.Render(i18n.T(i18n.KeyGenerateClaudeTag))
	}
	body := lipgloss.NewStyle().Width(max(20, width)).Render(content)
	return tag + "\n" + body
}

func (g *AIGenerate) View() string {
	if g.pickerOpen {
		return g.viewPicker()
	}
	if g.reviewing {
		return g.viewReview()
	}
	return g.viewChat()
}

func (g *AIGenerate) viewChat() string {
	deckLine := tui.StyleMuted.Render(i18n.T(i18n.KeyGenerateAddingTo)) +
		tui.StyleAccent.Render(g.deck.Name) +
		"  " + tui.StyleMuted.Render(i18n.T(i18n.KeyGenerateDeckSwitch))

	return lipgloss.JoinVertical(lipgloss.Left,
		deckLine,
		"",
		g.chatViewport.View(),
		"",
		g.renderInput(),
		g.renderStatusLine(),
	)
}

func (g *AIGenerate) renderInput() string {
	inputW := max(20, g.w-2)
	raw := g.input.View()
	if g.streaming {
		raw = tui.StyleMuted.Render(i18n.T(i18n.KeyGenerateWaiting)) + strings.Repeat("\n", g.inputH-1)
	}
	// vimtea appends a trailing newline after its last line, which lipgloss
	// otherwise renders as an extra visual row inside the border.
	raw = strings.TrimRight(raw, "\n")
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(vimModeBorderColor(g.input.GetMode())).
		Width(inputW).
		Render(raw)
}

// vimModeBorderColor maps the current vim mode onto the input box border
// color so the user always sees at a glance which mode they're in.
func vimModeBorderColor(m vimtea.EditorMode) lipgloss.Color {
	switch m {
	case vimtea.ModeInsert:
		return tui.ColorSuccess
	case vimtea.ModeVisual:
		return tui.ColorPrimary
	case vimtea.ModeCommand:
		return tui.ColorWarn
	}
	return tui.ColorBorder
}

func (g *AIGenerate) renderStatusLine() string {
	switch {
	case g.streaming:
		return g.spin.View() + tui.StyleMuted.Render(i18n.T(i18n.KeyGenerateThinking))
	case g.lastErr != nil:
		return tui.StyleDanger.Render(g.lastErr.Error())
	}
	return ""
}

func (g *AIGenerate) viewReview() string {
	total := len(g.proposed)
	accepted := len(g.accepted)
	header := tui.StyleTitle.Render(i18n.Tf(i18n.KeyGenerateReviewTitle, g.reviewIdx+1, total)) + "  " +
		tui.StyleMuted.Render(i18n.Tf(i18n.KeyGenerateReviewInfo, accepted, g.deck.Name))

	if g.reviewIdx >= len(g.proposed) {
		return tui.StyleMuted.Render(i18n.T(i18n.KeyGenerateNoMoreCards))
	}
	w := g.w
	if w <= 0 {
		w = 80
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, "", previewCard(g.proposed[g.reviewIdx], w))
}

func (g *AIGenerate) viewPicker() string {
	rows := []string{tui.StyleTitle.Render(i18n.T(i18n.KeyGenerateChangeDeck)), ""}
	for i, d := range g.pickerDecks {
		sel := i == g.pickerCursor
		name := d.Name
		if sel {
			name = tui.StyleSelected.Render(name)
		}
		rows = append(rows, fmt.Sprintf("%s%s  %s", selectionPrefix(sel), colorBullet(d.Color), name))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (g *AIGenerate) HelpKeys() []string {
	if g.pickerOpen {
		return []string{
			i18n.Help("↑/↓", i18n.KeyHelpMove),
			i18n.Help("enter", i18n.KeyHelpPick),
			i18n.Help("esc", i18n.KeyHelpCancel),
		}
	}
	if g.reviewing {
		return []string{
			i18n.Help("a", i18n.KeyHelpAccept),
			i18n.Help("r", i18n.KeyHelpReject),
			i18n.Help("esc", i18n.KeyHelpDiscard),
		}
	}
	return []string{
		i18n.Help("i", i18n.KeyHelpInsert),
		i18n.Help("esc", i18n.KeyHelpNormal),
		i18n.Help("ctrl+s", i18n.KeyHelpSend),
		i18n.Help("shift+↑/↓", i18n.KeyHelpScroll),
		i18n.Help("ctrl+d", i18n.KeyHelpDeck),
		i18n.Help("esc", i18n.KeyHelpBackNormal),
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
