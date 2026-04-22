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

// chatTimeout is generous compared to the grader because chat responses tend
// to be longer, but still bounded so a hung stream doesn't lock the UI.
const chatTimeout = 2 * time.Minute

// chatSubmitMsg is emitted by the vim chat input's ctrl+s binding. It carries
// the current buffer text so AIGenerate can queue the user's message and kick
// off the streaming reply.
type chatSubmitMsg struct{ content string }

// AIGenerate is a chat-based screen for authoring flashcards with Claude.
// The conversation streams from the AI package; when the assistant emits
// <card>...</card> blocks, they're parsed into proposedCards and shown to
// the user one at a time for accept/reject.
type AIGenerate struct {
	store *store.Store
	deck  models.Deck

	// full conversation so far — user + assistant turns.
	history []models.GradingMessage

	// transient streaming state
	pending   string // assistant's in-progress reply
	streaming bool
	ctx       context.Context
	cancel    context.CancelFunc
	streamCh  <-chan ai.Event
	spin      spinner.Model
	sendErr   error

	// chat UI
	vp    viewport.Model
	input vimtea.Editor
	// inputH is the current rendered height (1..15) — cached so we only
	// call SetSize on the editor when it actually changes.
	inputH int

	// deck picker overlay
	pickerOpen   bool
	pickerDecks  []models.Deck
	pickerCursor int

	// card review queue
	proposed   []store.CardInput
	reviewIdx  int
	reviewing  bool // true when the user is approving cards
	accepted   []store.CardInput
	justSavedN int // transient: rows saved on last flush, shown as a status line

	// screen size
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
	in.AddBinding(vimtea.KeyBinding{
		Key:  "ctrl+s",
		Mode: vimtea.ModeNormal,
		Handler: func(b vimtea.Buffer) tea.Cmd {
			text := b.Text()
			return func() tea.Msg { return chatSubmitMsg{content: text} }
		},
	})
	in.AddBinding(vimtea.KeyBinding{
		Key:  "ctrl+s",
		Mode: vimtea.ModeInsert,
		Handler: func(b vimtea.Buffer) tea.Cmd {
			text := b.Text()
			return func() tea.Msg { return chatSubmitMsg{content: text} }
		},
	})
	in.SetSize(60, 1)

	vp := viewport.New(80, 12)

	return &AIGenerate{
		store:  s,
		deck:   deck,
		spin:   sp,
		input:  in,
		inputH: 1,
		vp:     vp,
	}
}

func (g *AIGenerate) Init() tea.Cmd {
	return tea.Batch(g.input.Init(), g.loadDecks())
}

type generateDecksLoadedMsg struct {
	decks []models.Deck
	err   error
}

type cardsSavedMsg struct {
	n   int
	err error
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
		g.w = m.Width
		g.h = m.Height
		g.resizeInner()
		return g, nil

	case generateDecksLoadedMsg:
		if m.err == nil {
			g.pickerDecks = m.decks
			for i, d := range m.decks {
				if d.ID == g.deck.ID {
					g.pickerCursor = i
					break
				}
			}
		}
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
		return g, nil

	case streamErrMsg:
		g.streaming = false
		g.sendErr = m.err
		return g, nil

	case cardsSavedMsg:
		g.accepted = nil
		if m.err != nil {
			return g, tui.ToastErr("save failed: " + m.err.Error())
		}
		g.justSavedN = m.n
		return g, tui.Toast(fmt.Sprintf("saved %d card%s to %s", m.n, plural(m.n), g.deck.Name))

	case chatSubmitMsg:
		return g.submitChat(m.content)

	case tea.KeyMsg:
		return g.handleKey(m)
	}

	// forward non-key messages (cursor blink ticks, etc.) to the vim input
	// so its cursor stays animated
	if !g.pickerOpen && !g.reviewing {
		_, cmd := g.input.Update(msg)
		return g, cmd
	}
	return g, nil
}

// submitChat persists the user message, clears the input, and kicks off the
// streaming AI reply. Shared between the ctrl+s binding and, historically,
// the enter keypath (now unused).
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

	// Overlays/review consume keys first.
	if g.pickerOpen {
		if key == "esc" {
			g.pickerOpen = false
			return g, nil
		}
		return g.handlePickerKey(m)
	}
	if g.reviewing {
		if key == "esc" {
			g.reviewing = false
			g.proposed = nil
			g.accepted = nil
			return g, nil
		}
		return g.handleReviewKey(m)
	}

	// ctrl+d opens the deck picker from any mode. We intercept it before
	// vimtea so typing 'd' in normal mode still does its usual thing.
	if key == "ctrl+d" {
		g.pickerOpen = true
		return g, nil
	}

	// esc in NORMAL mode leaves the screen (escape is the natural "I'm
	// done" gesture when the editor is already idle). In insert/visual,
	// forward to vimtea so it returns to normal mode.
	if key == "esc" && g.input.GetMode() == vimtea.ModeNormal {
		if g.cancel != nil {
			g.cancel()
		}
		return g, func() tea.Msg { return tui.NavMsg{Pop: true} }
	}

	// Everything else goes to vimtea. ctrl+s is wired as a binding that
	// dispatches chatSubmitMsg; we handle it back in Update.
	_, cmd := g.input.Update(m)
	g.fitInput()
	return g, cmd
}

func (g *AIGenerate) handlePickerKey(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	switch m.String() {
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
	case "a", "y":
		if g.reviewIdx < len(g.proposed) {
			g.accepted = append(g.accepted, g.proposed[g.reviewIdx])
		}
		g.reviewIdx++
	case "r", "n":
		g.reviewIdx++
	}

	if g.reviewIdx >= len(g.proposed) {
		// end of queue — persist accepted cards
		toSave := g.accepted
		g.reviewing = false
		g.proposed = nil
		g.accepted = nil
		g.reviewIdx = 0
		if len(toSave) == 0 {
			return g, nil
		}
		deckID := g.deck.ID
		store := g.store
		return g, func() tea.Msg {
			_, err := store.BulkCreateCards(deckID, toSave)
			if err != nil {
				return cardsSavedMsg{err: err}
			}
			return cardsSavedMsg{n: len(toSave)}
		}
	}
	return g, nil
}

func (g *AIGenerate) startStream() tea.Cmd {
	client, err := resolveAIClient(g.store)
	if err != nil {
		g.sendErr = err
		// rewind: drop the queued user message since we can't send it
		if len(g.history) > 0 && g.history[len(g.history)-1].Role == "user" {
			g.history = g.history[:len(g.history)-1]
		}
		g.refreshTranscript()
		return nil
	}
	g.ctx, g.cancel = context.WithTimeout(context.Background(), chatTimeout)
	g.streaming = true
	g.pending = ""
	g.streamCh = client.Chat(g.ctx, g.deck.Name, g.deck.Description, g.history)
	return tea.Batch(g.spin.Tick, pumpStream(g.streamCh))
}

// ---------- layout & rendering ----------

const (
	chatInputMinH = 1
	chatInputMaxH = 15
)

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
	// chrome: deck line + blank + blank-before-input + input box (inputH + 2 border) + status
	inputH := clampInputH(g.inputH)
	vpH := max(3, h-(3+inputH+2+1))
	g.vp.Width = w
	g.vp.Height = vpH
	g.input.SetSize(max(20, w-2), inputH)
	g.refreshTranscript()
}

// fitInput sizes the vim input to its buffer line count and resizes the chat
// viewport to absorb the change.
func (g *AIGenerate) fitInput() {
	lines := clampInputH(g.input.GetBuffer().LineCount())
	if g.inputH == lines {
		return
	}
	g.inputH = lines
	g.resizeInner()
}

// refreshTranscript re-renders the full chat transcript (including the
// streaming assistant reply, if any) into the viewport.
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
	var tag string
	if role == "assistant" {
		tag = tui.StyleAccent.Render("claude ›")
	} else {
		tag = tui.StylePrimary.Render("you ›")
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

	deckLine := tui.StyleMuted.Render("adding to ") +
		tui.StyleAccent.Render(g.deck.Name) +
		"  " + tui.StyleMuted.Render("· ctrl+d to change")

	statusLine := ""
	switch {
	case g.streaming:
		statusLine = g.spin.View() + tui.StyleMuted.Render(" thinking… (enter to wait)")
	case g.sendErr != nil:
		statusLine = tui.StyleDanger.Render(g.sendErr.Error())
	}

	inputW := max(20, g.w-2)
	var rawInput string
	if g.streaming {
		rawInput = tui.StyleMuted.Render("(waiting for Claude…)")
		for i := 1; i < g.inputH; i++ {
			rawInput += "\n"
		}
	} else {
		rawInput = g.input.View()
	}
	// Colour the border by vim mode so the user always knows what state the
	// input is in — matches the codeedit convention.
	borderColor := tui.ColorBorder
	switch g.input.GetMode() {
	case vimtea.ModeInsert:
		borderColor = tui.ColorSuccess
	case vimtea.ModeVisual:
		borderColor = tui.ColorPrimary
	case vimtea.ModeCommand:
		borderColor = tui.ColorWarn
	}
	inputBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(inputW).
		Render(rawInput)

	return lipgloss.JoinVertical(lipgloss.Left,
		deckLine,
		"",
		g.vp.View(),
		"",
		inputBox,
		statusLine,
	)
}

func (g *AIGenerate) viewReview() string {
	total := len(g.proposed)
	accepted := len(g.accepted)
	header := tui.StyleTitle.Render(fmt.Sprintf("Review card %d / %d", g.reviewIdx+1, total)) + "  " +
		tui.StyleMuted.Render(fmt.Sprintf("(%d accepted so far · adding to %s)", accepted, g.deck.Name))

	if g.reviewIdx >= len(g.proposed) {
		return tui.StyleMuted.Render("no more cards")
	}
	card := g.proposed[g.reviewIdx]
	w := g.w
	if w <= 0 {
		w = 80
	}
	preview := previewCard(card, w)

	return lipgloss.JoinVertical(lipgloss.Left,
		header, "",
		preview,
	)
}

func (g *AIGenerate) viewPicker() string {
	header := tui.StyleTitle.Render("Change deck")
	var rows []string
	rows = append(rows, header, "")
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

// ---------- card extraction ----------

// aiCardDTO mirrors the JSON schema Claude is asked to emit inside each
// <card>...</card> block.
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

// extractProposedCards scans the assistant reply for <card>...</card> blocks
// and converts each into a store.CardInput. Malformed blocks are silently
// skipped — the user can always ask Claude to regenerate.
func extractProposedCards(reply string) []store.CardInput {
	matches := cardTagRe.FindAllStringSubmatch(reply, -1)
	var out []store.CardInput
	for _, m := range matches {
		raw := strings.TrimSpace(m[1])
		if raw == "" {
			continue
		}
		var dto aiCardDTO
		if err := json.Unmarshal([]byte(raw), &dto); err != nil {
			continue
		}
		ct := models.CardType(strings.ToLower(strings.TrimSpace(dto.Type)))
		switch ct {
		case models.CardMCQ, models.CardCode, models.CardFill, models.CardExp:
		default:
			continue
		}
		lang := dto.Language
		if strings.TrimSpace(lang) == "" {
			lang = "javascript"
		}
		in := store.CardInput{
			Type:           ct,
			Language:       lang,
			Prompt:         dto.Prompt,
			InitialCode:    dto.InitialCode,
			ExpectedAnswer: dto.ExpectedAnswer,
			Choices:        dto.Choices,
			BlanksData:     dto.BlanksData,
		}
		// Normalize MCQ choice IDs to a, b, c, … so the study screen can
		// render them consistently. Claude sometimes omits them.
		if in.Type == models.CardMCQ && len(in.Choices) > 0 {
			for i := range in.Choices {
				if strings.TrimSpace(in.Choices[i].ID) == "" {
					in.Choices[i].ID = string(rune('a' + i))
				}
			}
		}
		out = append(out, in)
	}
	return out
}

func previewCard(in store.CardInput, width int) string {
	var rows []string
	rows = append(rows,
		tui.StyleMuted.Render("type")+"  "+typeBadge(in.Type, true)+"   "+
			tui.StyleMuted.Render("lang")+"  "+in.Language,
		"",
		tui.StyleMuted.Render("prompt"),
		previewBlock(in.Prompt, "(empty)", width),
	)
	switch in.Type {
	case models.CardMCQ:
		rows = append(rows, "", tui.StyleMuted.Render("choices"))
		for _, ch := range in.Choices {
			mark := "[ ]"
			if ch.IsCorrect {
				mark = tui.StyleSuccess.Render("[x]")
			}
			rows = append(rows, fmt.Sprintf("  %s %s. %s", mark, ch.ID, ch.Text))
		}
	case models.CardFill:
		if in.BlanksData != nil {
			rows = append(rows, "", tui.StyleMuted.Render("template"), previewBlock(in.BlanksData.Template, "(empty)", width))
			rows = append(rows, "", tui.StyleMuted.Render("blanks: "+strings.Join(in.BlanksData.Blanks, ", ")))
		}
	case models.CardCode, models.CardExp:
		if in.ExpectedAnswer != "" {
			rows = append(rows, "", tui.StyleMuted.Render("expected answer"), previewBlock(in.ExpectedAnswer, "", width))
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// previewBlock renders a multi-line string inside a single rounded border
// constrained to the given total width. Long lines wrap; content beyond 10
// lines is truncated with an ellipsis. The explicit width is what makes the
// border render cleanly even when the underlying text is wider than the
// screen.
func previewBlock(content, placeholder string, totalWidth int) string {
	if totalWidth < 10 {
		totalWidth = 10
	}
	innerW := totalWidth - 4 // border (2) + padding (2)
	if innerW < 4 {
		innerW = 4
	}
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
