package screens

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lforato/vimtea"

	"github.com/lforato/gocards/internal/ai"
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

	pending   string
	streaming bool
	ctx       context.Context
	cancel    context.CancelFunc
	streamCh  <-chan ai.Event
	spin      spinner.Model
	sendErr   error

	vp     viewport.Model
	input  vimtea.Editor
	inputH int

	pickerOpen   bool
	pickerDecks  []models.Deck
	pickerCursor int

	proposed   []store.CardInput
	reviewIdx  int
	reviewing  bool
	accepted   []store.CardInput
	justSavedN int

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
		store:  s,
		deck:   deck,
		spin:   sp,
		input:  in,
		inputH: 1,
		vp:     viewport.New(80, 12),
	}
}

func (g *AIGenerate) Init() tea.Cmd {
	return tea.Batch(g.input.Init(), g.loadDecks())
}

func (g *AIGenerate) loadDecks() tea.Cmd {
	return func() tea.Msg {
		decks, err := g.store.ListDecks()
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

	case spinner.TickMsg:
		if !g.streaming {
			return g, nil
		}
		var cmd tea.Cmd
		g.spin, cmd = g.spin.Update(m)
		return g, cmd

	case streamChunkMsg:
		g.pending += m.text
		g.refreshTranscript()
		g.vp.GotoBottom()
		return g, pumpStream(g.streamCh)

	case streamDoneMsg:
		return g, g.finishStream(m)

	case streamErrMsg:
		g.streaming = false
		g.sendErr = m.err
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
		reply = g.pending
	}
	g.streaming = false
	g.history = append(g.history, models.GradingMessage{Role: "assistant", Content: reply})
	g.pending = ""
	g.refreshTranscript()
	g.vp.GotoBottom()

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
		return g, tui.ToastErr("save failed: " + m.err.Error())
	}
	g.justSavedN = m.n
	return g, tui.Toast(fmt.Sprintf("saved %d card%s to %s", m.n, plural(m.n), g.deck.Name))
}

func (g *AIGenerate) submitChat(text string) (tui.Screen, tea.Cmd) {
	text = strings.TrimSpace(text)
	if text == "" || g.streaming {
		return g, nil
	}
	g.input.GetBuffer().Clear()
	g.fitInput()
	g.history = append(g.history, models.GradingMessage{Role: "user", Content: text})
	g.sendErr = nil
	g.refreshTranscript()
	g.vp.GotoBottom()
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

	_, cmd := g.input.Update(m)
	g.fitInput()
	return g, cmd
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
		g.sendErr = err
		g.rewindQueuedUserTurn()
		g.refreshTranscript()
		return nil
	}
	g.ctx, g.cancel = context.WithTimeout(context.Background(), chatTimeout)
	g.streaming = true
	g.pending = ""
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

func clampInputH(n int) int {
	return max(chatInputMinH, min(chatInputMaxH, n))
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
	inputH := clampInputH(g.inputH)
	g.vp.Width = w
	g.vp.Height = max(3, h-(3+inputH+2+1))
	g.input.SetSize(max(20, w-2), inputH)
	g.refreshTranscript()
}

func (g *AIGenerate) fitInput() {
	lines := clampInputH(g.input.GetBuffer().LineCount())
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
		b.WriteString(formatChatTurn(m.Role, m.Content, g.vp.Width))
	}
	if g.streaming && g.pending != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(formatChatTurn("assistant", g.pending, g.vp.Width))
	}
	g.vp.SetContent(b.String())
}

func formatChatTurn(role, content string, width int) string {
	tag := tui.StylePrimary.Render("you ›")
	if role == "assistant" {
		tag = tui.StyleAccent.Render("claude ›")
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
	deckLine := tui.StyleMuted.Render("adding to ") +
		tui.StyleAccent.Render(g.deck.Name) +
		"  " + tui.StyleMuted.Render("· ctrl+d to change")

	return lipgloss.JoinVertical(lipgloss.Left,
		deckLine,
		"",
		g.vp.View(),
		"",
		g.renderInput(),
		g.renderStatusLine(),
	)
}

func (g *AIGenerate) renderInput() string {
	inputW := max(20, g.w-2)
	raw := g.input.View()
	if g.streaming {
		raw = tui.StyleMuted.Render("(waiting for Claude…)") + strings.Repeat("\n", g.inputH-1)
	}
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
		return g.spin.View() + tui.StyleMuted.Render(" thinking… (enter to wait)")
	case g.sendErr != nil:
		return tui.StyleDanger.Render(g.sendErr.Error())
	}
	return ""
}

func (g *AIGenerate) viewReview() string {
	total := len(g.proposed)
	accepted := len(g.accepted)
	header := tui.StyleTitle.Render(fmt.Sprintf("Review card %d / %d", g.reviewIdx+1, total)) + "  " +
		tui.StyleMuted.Render(fmt.Sprintf("(%d accepted so far · adding to %s)", accepted, g.deck.Name))

	if g.reviewIdx >= len(g.proposed) {
		return tui.StyleMuted.Render("no more cards")
	}
	w := g.w
	if w <= 0 {
		w = 80
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, "", previewCard(g.proposed[g.reviewIdx], w))
}

func (g *AIGenerate) viewPicker() string {
	rows := []string{tui.StyleTitle.Render("Change deck"), ""}
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
		return []string{"↑/↓ move", "enter pick", "esc cancel"}
	}
	if g.reviewing {
		return []string{"a accept", "r reject", "esc discard remaining"}
	}
	return []string{"i insert", "esc normal", "ctrl+s send", "ctrl+d deck", "esc back (normal)"}
}

type aiCardDTO struct {
	Type           string            `json:"type"`
	Language       string            `json:"language"`
	Prompt         string            `json:"prompt"`
	ExpectedAnswer string            `json:"expected_answer"`
	InitialCode    string            `json:"initial_code"`
	Choices        []models.Choice   `json:"choices"`
	BlanksData     *models.BlankData `json:"blanks_data"`
}

var cardTagRe = regexp.MustCompile(`(?s)<card>(.*?)</card>`)

// extractProposedCards scans the assistant reply for <card>…</card> JSON
// blocks. Malformed blocks are skipped — the user can always ask Claude to
// regenerate.
func extractProposedCards(reply string) []store.CardInput {
	var out []store.CardInput
	for _, m := range cardTagRe.FindAllStringSubmatch(reply, -1) {
		if card, ok := parseCardBlock(m[1]); ok {
			out = append(out, card)
		}
	}
	return out
}

func parseCardBlock(raw string) (store.CardInput, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return store.CardInput{}, false
	}
	var dto aiCardDTO
	if err := json.Unmarshal([]byte(raw), &dto); err != nil {
		return store.CardInput{}, false
	}
	ct := models.CardType(strings.ToLower(strings.TrimSpace(dto.Type)))
	if !models.IsKnownCardType(ct) {
		return store.CardInput{}, false
	}

	in := store.CardInput{
		Type:           ct,
		Language:       defaultLanguage(dto.Language),
		Prompt:         dto.Prompt,
		InitialCode:    dto.InitialCode,
		ExpectedAnswer: dto.ExpectedAnswer,
		Choices:        dto.Choices,
		BlanksData:     dto.BlanksData,
	}
	if in.Type == models.CardMCQ {
		assignMissingChoiceIDs(in.Choices)
	}
	return in, true
}

func defaultLanguage(lang string) string {
	if strings.TrimSpace(lang) == "" {
		return "javascript"
	}
	return lang
}

// assignMissingChoiceIDs fills in a/b/c/… for choices Claude returned
// without an ID, so the study screen can label them consistently.
func assignMissingChoiceIDs(choices []models.Choice) {
	for i := range choices {
		if strings.TrimSpace(choices[i].ID) == "" {
			choices[i].ID = string(rune('a' + i))
		}
	}
}

func previewCard(in store.CardInput, width int) string {
	rows := []string{
		tui.StyleMuted.Render("type") + "  " + typeBadge(in.Type, true) + "   " +
			tui.StyleMuted.Render("lang") + "  " + in.Language,
		"",
		tui.StyleMuted.Render("prompt"),
		previewBlock(in.Prompt, "(empty)", width),
	}
	rows = append(rows, cardSpecificPreview(in, width)...)
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func cardSpecificPreview(in store.CardInput, width int) []string {
	switch in.Type {
	case models.CardMCQ:
		rows := []string{"", tui.StyleMuted.Render("choices")}
		for _, ch := range in.Choices {
			mark := "[ ]"
			if ch.IsCorrect {
				mark = tui.StyleSuccess.Render("[x]")
			}
			rows = append(rows, fmt.Sprintf("  %s %s. %s", mark, ch.ID, ch.Text))
		}
		return rows
	case models.CardFill:
		if in.BlanksData == nil {
			return nil
		}
		return []string{
			"", tui.StyleMuted.Render("template"),
			previewBlock(in.BlanksData.Template, "(empty)", width),
			"", tui.StyleMuted.Render("blanks: " + strings.Join(in.BlanksData.Blanks, ", ")),
		}
	case models.CardCode, models.CardExp:
		if in.ExpectedAnswer == "" {
			return nil
		}
		return []string{"", tui.StyleMuted.Render("expected answer"), previewBlock(in.ExpectedAnswer, "", width)}
	}
	return nil
}

// previewBlock wraps content in a rounded border constrained to totalWidth.
// Content past 10 lines is clipped with an ellipsis. The explicit width is
// what keeps the border from running past the terminal edge.
func previewBlock(content, placeholder string, totalWidth int) string {
	if totalWidth < 10 {
		totalWidth = 10
	}
	innerW := max(4, totalWidth-4)

	raw := strings.TrimSpace(content)
	if raw == "" {
		raw = placeholder
	}
	lines := strings.Split(raw, "\n")
	if len(lines) > 10 {
		lines = append(lines[:10], "…")
	}
	body := lipgloss.NewStyle().Width(innerW).Render(strings.Join(lines, "\n"))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorBorder).
		Padding(0, 1).
		Width(innerW + 2).
		Render(body)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
