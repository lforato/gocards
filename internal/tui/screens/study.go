package screens

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lforato/vimtea"

	"github.com/lforato/gocards/internal/ai"
	"github.com/lforato/gocards/internal/i18n"
	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/srs"
	"github.com/lforato/gocards/internal/store"
	"github.com/lforato/gocards/internal/tui"
)

type studyStage int

const (
	stageQuestion studyStage = iota
	stageAnswered
	stageGrading
	stageDone
)

// Study drives the review loop. Per-card-type logic (MCQ/fill/code/exp)
// lives in study_mcq.go, study_fill.go, and study_code.go.
type Study struct {
	store *store.Store
	deck  models.Deck

	cards   []models.Card
	idx     int
	session *models.StudySession
	stage   studyStage

	mcqCursor         int
	fillInputs        []textinput.Model
	fillFocus         int
	codeAnswer        string
	explanationAnswer string

	ctx           context.Context
	cancel        context.CancelFunc
	streamCh      <-chan ai.Event
	spin          spinner.Model
	gradeViewport viewport.Model
	graderBuf     string
	graderErr     error
	graderScore   int

	// Multi-turn grading state. gradingHistory holds the committed
	// user/assistant turns (seeded after the first response); graderBuf
	// holds the currently-streaming reply.
	gradingInput      *ai.GradeInput
	gradingHistory    []models.GradingMessage
	followUpStreaming bool
	followUpEditor    vimtea.Editor
	followUpH         int
	// gradePending is true between the grader emitting a grade and us
	// recording it. recordReview is deferred so follow-up revisions become
	// the final recorded grade.
	gradePending bool

	resultGrade int
	resultNote  string

	pendingDelete bool

	w, h int

	codeEditor vimtea.Editor
}

func NewStudy(s *store.Store, d models.Deck) *Study {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return &Study{store: s, deck: d, spin: sp, gradeViewport: viewport.New(80, 10)}
}

type studyLoadedMsg struct {
	cards   []models.Card
	session *models.StudySession
	err     error
}

func (s *Study) Init() tea.Cmd {
	// When re-entering after navigating away (e.g. returning from the edit
	// screen), don't restart the session — just pull the current card from
	// store so any edits show up.
	if s.cards != nil {
		return tea.Batch(s.spin.Tick, s.refreshCurrentCard())
	}
	return tea.Batch(s.spin.Tick, s.loadDue())
}

func (s *Study) loadDue() tea.Cmd {
	return func() tea.Msg {
		cards, err := s.store.DueCards(s.deck.ID, s.store.DailyLimit())
		if err != nil {
			return studyLoadedMsg{err: err}
		}
		sess, err := s.store.CreateSession(s.deck.ID)
		if err != nil {
			return studyLoadedMsg{err: err}
		}
		return studyLoadedMsg{cards: cards, session: sess}
	}
}

type cardRefreshedMsg struct{ card models.Card }

func (s *Study) refreshCurrentCard() tea.Cmd {
	card := s.current()
	if card == nil {
		return nil
	}
	id := card.ID
	return func() tea.Msg {
		c, err := s.store.GetCard(id)
		if err != nil || c == nil {
			return nil
		}
		return cardRefreshedMsg{card: *c}
	}
}

func (s *Study) current() *models.Card {
	if s.idx >= len(s.cards) {
		return nil
	}
	return &s.cards[s.idx]
}

func (s *Study) resetPerCardState() tea.Cmd {
	card := s.current()
	if card == nil {
		return nil
	}
	s.mcqCursor = 0
	s.codeAnswer = card.InitialCode
	s.explanationAnswer = ""
	s.graderBuf = ""
	s.graderErr = nil
	s.graderScore = 0
	s.gradingInput = nil
	s.gradingHistory = nil
	s.followUpStreaming = false
	s.followUpEditor = nil
	s.followUpH = 1
	s.gradePending = false
	s.resultGrade = 0
	s.resultNote = ""
	s.stage = stageQuestion
	s.codeEditor = nil

	kind := models.Kind(card.Type)
	s.fillInputs = nil
	if kind.UsesBlanks {
		s.initFillInputs(card)
	}

	if card.Type == models.CardExp {
		s.explanationAnswer = ""
	}
	if kind.UsesCodeEditor {
		return s.initCodeEditor(card)
	}
	return nil
}

// Layout budget for the per-card body. View reserves these rows and passes
// the remainder to the inline vim editor.
const (
	studyChromeRows   = 2
	studyRightLabel   = 1
	studyPromptChrome = 2
)

func (s *Study) bodyHeight() int {
	h := s.h
	if h <= 0 {
		h = 20
	}
	return max(8, h-studyChromeRows)
}

func (s *Study) editorWidth() int {
	w := s.w
	if w <= 0 {
		w = 80
	}
	return max(20, w)
}

// editorHeight is a fallback used when the editor is created before View has
// measured the prompt. View overrides it with the real height every frame.
func (s *Study) editorHeight() int {
	return max(5, s.bodyHeight()-studyRightLabel-studyPromptChrome-2)
}

func (s *Study) Update(msg tea.Msg) (tui.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		s.w, s.h = m.Width, m.Height
		if s.codeEditor != nil {
			s.codeEditor.SetSize(s.editorWidth(), s.editorHeight())
		}
		if s.followUpEditor != nil {
			s.followUpEditor.SetSize(max(20, s.w-2), s.followUpH)
		}
		s.gradeViewport.Width = max(40, s.w)
		s.refreshGradeViewport()
		return s, nil

	case studyLoadedMsg:
		if m.err != nil {
			return s, tui.ToastErr(i18n.T(i18n.KeyStudyLoadFailPfx) + m.err.Error())
		}
		s.cards = m.cards
		s.session = m.session
		if len(s.cards) == 0 {
			s.stage = stageDone
			return s, nil
		}
		return s, s.resetPerCardState()

	case spinner.TickMsg:
		var cmd tea.Cmd
		s.spin, cmd = s.spin.Update(m)
		return s, cmd

	case streamChunkMsg:
		s.graderBuf += m.text
		s.refreshGradeViewport()
		return s, pumpStream(s.streamCh)

	case streamDoneMsg:
		return s, s.finishGrade(m)

	case streamErrMsg:
		s.graderErr = m.err
		s.followUpStreaming = false
		s.stage = stageAnswered
		return s, s.ensureFollowUpEditor()

	case codeSubmitMsg:
		return s.handleCodeSubmit(m)

	case followUpSubmitMsg:
		return s, s.submitFollowUp(m.content)

	case cardRefreshedMsg:
		if s.idx < len(s.cards) {
			s.cards[s.idx] = m.card
			return s, s.resetPerCardState()
		}
		return s, nil

	case cardDeletedMsg:
		return s.handleCardDeleted(m)

	case tea.KeyMsg:
		return s.handleKey(m)
	}

	return s, s.forwardToEmbedded(msg)
}

type cardDeletedMsg struct{ err error }

// handleCardDeleted drops the just-deleted card from the in-memory list and
// advances to the next one (or ends the session if it was the last).
func (s *Study) handleCardDeleted(m cardDeletedMsg) (tui.Screen, tea.Cmd) {
	if m.err != nil {
		return s, tui.ToastErr("delete failed: " + m.err.Error())
	}
	if s.idx >= len(s.cards) {
		return s, nil
	}
	s.cards = append(s.cards[:s.idx], s.cards[s.idx+1:]...)
	if len(s.cards) == 0 {
		s.stage = stageDone
		s.markSessionEnded()
		return s, tui.Toast(i18n.T(i18n.KeyStudyCardDeleted))
	}
	if s.idx >= len(s.cards) {
		s.idx = len(s.cards) - 1
	}
	return s, tea.Batch(tui.Toast(i18n.T(i18n.KeyStudyCardDeleted)), s.resetPerCardState())
}

func (s *Study) finishGrade(m streamDoneMsg) tea.Cmd {
	reply := m.full
	if reply == "" {
		reply = s.graderBuf
	}

	// Commit this turn to the history so follow-ups can continue the
	// dialogue. On the first response we also seed the initial user turn
	// (the one Grade() built internally from the student's answer).
	if len(s.gradingHistory) == 0 && s.gradingInput != nil {
		s.gradingHistory = append(s.gradingHistory, models.GradingMessage{
			Role:    "user",
			Content: ai.FirstGraderTurn(*s.gradingInput),
		})
	}
	s.gradingHistory = append(s.gradingHistory, models.GradingMessage{
		Role:    "assistant",
		Content: reply,
	})

	s.graderBuf = ""
	s.followUpStreaming = false
	s.stage = stageAnswered

	if grade, ok := extractGrade(reply); ok {
		s.graderScore = grade
		s.graderErr = nil
		s.gradePending = true
	} else if s.graderScore == 0 {
		s.graderErr = fmt.Errorf("%s", i18n.T(i18n.KeyStudyGraderNoGrade))
	}

	cmd := s.ensureFollowUpEditor()
	s.refreshGradeViewport()
	return cmd
}

// forwardToEmbedded routes non-key ticks (cursor blinks etc.) to the widget
// active for the current card type so its animations stay alive.
func (s *Study) forwardToEmbedded(msg tea.Msg) tea.Cmd {
	card := s.current()
	if card == nil {
		return nil
	}
	if s.stage == stageAnswered && s.followUpEditor != nil {
		_, cmd := s.followUpEditor.Update(msg)
		return cmd
	}
	if s.stage != stageQuestion {
		return nil
	}
	kind := models.Kind(card.Type)
	switch {
	case kind.UsesCodeEditor && s.codeEditor != nil:
		_, cmd := s.codeEditor.Update(msg)
		return cmd
	case kind.UsesBlanks && s.fillFocus < len(s.fillInputs):
		var cmd tea.Cmd
		s.fillInputs[s.fillFocus], cmd = s.fillInputs[s.fillFocus].Update(msg)
		return cmd
	}
	return nil
}

func (s *Study) handleKey(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	card := s.current()

	if s.pendingDelete {
		return s.handleDeleteConfirm(m)
	}

	if consumed, cmd := s.handleGlobalLifecycleKey(m, card); consumed {
		return s, cmd
	}

	if consumed, cmd := s.handleGlobalNavKey(m); consumed {
		return s, cmd
	}

	if s.handleGradeScroll(m) {
		return s, nil
	}

	if s.inVimQuestion(card) {
		return s.routeToVim(m)
	}

	if s.inFollowUpChat(card) {
		return s.routeFollowUp(m)
	}

	if m.String() == "esc" {
		return s, s.cancelAndExit()
	}

	if s.stage == stageDone {
		if m.String() == "enter" || m.String() == "q" {
			return s, s.endAndPop()
		}
		return s, nil
	}

	if card == nil {
		return s, nil
	}

	switch s.stage {
	case stageQuestion:
		return s.handleQuestionKey(m, card)
	case stageGrading:
		if m.String() == "ctrl+x" && s.cancel != nil {
			s.cancel()
		}
		return s, nil
	case stageAnswered:
		return s.handleAnsweredKey(m, card)
	}
	return s, nil
}

// handleGlobalNavKey binds ctrl+n to advance and ctrl+p to go back. These
// are standard ASCII control codes so they pass through every terminal
// without any CSI-u / modifyOtherKeys configuration.
func (s *Study) handleGlobalNavKey(m tea.KeyMsg) (bool, tea.Cmd) {
	switch m.String() {
	case "ctrl+n":
		if s.stage == stageGrading || s.stage == stageDone {
			return true, nil
		}
		return true, s.advance()
	case "ctrl+p":
		if s.idx <= 0 {
			return true, nil
		}
		return true, s.previous()
	}
	return false, nil
}

func (s *Study) inFollowUpChat(card *models.Card) bool {
	return card != nil &&
		s.stage == stageAnswered &&
		models.Kind(card.Type).IsAIGraded &&
		s.followUpEditor != nil
}

// routeFollowUp routes keys to the chat input during AI-graded stageAnswered.
// ctrl+s submits; esc from normal mode ends study; everything else is an
// edit keystroke for the vim input.
func (s *Study) routeFollowUp(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	if s.followUpStreaming {
		if m.String() == "ctrl+x" && s.cancel != nil {
			s.cancel()
		}
		return s, nil
	}
	key := m.String()
	if key == "esc" && s.followUpEditor.GetMode() == vimtea.ModeNormal {
		return s, s.cancelAndExit()
	}
	if m.Paste {
		return s.handleFollowUpPaste(m)
	}
	m = normalizeNewlineKey(m)
	_, cmd := s.followUpEditor.Update(m)
	s.fitFollowUp()
	return s, cmd
}

func (s *Study) handleFollowUpPaste(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	var cmds []tea.Cmd
	if s.followUpEditor.GetMode() != vimtea.ModeInsert {
		if cmd := s.followUpEditor.SetMode(vimtea.ModeInsert); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	for _, r := range m.Runes {
		var synth tea.KeyMsg
		if r == '\n' || r == '\r' {
			synth = tea.KeyMsg{Type: tea.KeyEnter}
		} else {
			synth = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
		}
		if _, cmd := s.followUpEditor.Update(synth); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	s.fitFollowUp()
	return s, tea.Batch(cmds...)
}

// handleGlobalLifecycleKey intercepts ctrl+e / ctrl+d regardless of stage or
// input mode so the user can edit or delete the current card mid-study even
// while the inline vim editor or a text input is focused. Ctrl-prefixed keys
// don't collide with text entry so this is safe everywhere. Returns
// (consumed, cmd) so the caller can short-circuit key routing.
func (s *Study) handleGlobalLifecycleKey(m tea.KeyMsg, card *models.Card) (bool, tea.Cmd) {
	if card == nil {
		return false, nil
	}
	switch m.String() {
	case "ctrl+e":
		if s.cancel != nil {
			s.cancel()
		}
		return true, navTo(NewEdit(s.store, *card))
	case "ctrl+d":
		s.pendingDelete = true
		return true, nil
	}
	return false, nil
}

func (s *Study) handleDeleteConfirm(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	s.pendingDelete = false
	if m.String() != "y" && m.String() != "Y" {
		return s, nil
	}
	card := s.current()
	if card == nil {
		return s, nil
	}
	id := card.ID
	return s, func() tea.Msg {
		return cardDeletedMsg{err: s.store.DeleteCard(id)}
	}
}

func (s *Study) inVimQuestion(card *models.Card) bool {
	return s.stage == stageQuestion && card != nil && s.codeEditor != nil &&
		models.Kind(card.Type).UsesCodeEditor
}

// routeToVim forwards keys to the inline editor. Only esc-from-normal-mode
// escapes the screen; every other key (including esc from insert/visual)
// belongs to vim.
func (s *Study) routeToVim(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	if m.String() == "esc" && s.codeEditor.GetMode() == vimtea.ModeNormal {
		return s, s.cancelAndExit()
	}
	_, cmd := s.codeEditor.Update(m)
	return s, cmd
}

func (s *Study) handleQuestionKey(m tea.KeyMsg, card *models.Card) (tui.Screen, tea.Cmd) {
	if b := studyBehaviors[card.Type]; b.HandleKey != nil {
		return b.HandleKey(s, m, card)
	}
	return s, nil
}

func (s *Study) handleAnsweredKey(m tea.KeyMsg, card *models.Card) (tui.Screen, tea.Cmd) {
	if models.Kind(card.Type).IsAIGraded {
		switch m.String() {
		case "1", "2", "3", "4", "5":
			g, _ := strconv.Atoi(m.String())
			s.graderScore = g
			s.gradePending = true
		}
		return s, nil
	}
	return s, nil
}

func (s *Study) cancelAndExit() tea.Cmd {
	if s.cancel != nil {
		s.cancel()
	}
	s.flushPendingGrade()
	return s.endAndPop()
}

// flushPendingGrade writes the current graderScore for AI-graded cards when
// the grade hasn't been recorded yet. Called before advancing, going back,
// or ending the session so follow-up revisions become the saved grade.
func (s *Study) flushPendingGrade() {
	if !s.gradePending {
		return
	}
	if s.graderScore >= 1 && s.graderScore <= 5 {
		s.recordReview(s.graderScore)
	}
	s.gradePending = false
}

// recordReview schedules the next SRS interval and persists it. Grades
// outside 1..5 are rejected silently so extractGrade's fallback doesn't
// accidentally record a 0.
func (s *Study) recordReview(grade int) tea.Cmd {
	card := s.current()
	if card == nil || grade < 1 || grade > 5 {
		return nil
	}
	ease, interval := s.priorSchedule(card.ID)
	r := srs.CalculateNext(grade, ease, interval)
	if _, err := s.store.CreateReview(card.ID, grade, r.Ease, r.Interval, r.NextDue); err != nil {
		return tui.ToastErr(i18n.T(i18n.KeyStudyReviewSaveFail) + err.Error())
	}
	s.bumpSessionCounter()
	return nil
}

func (s *Study) priorSchedule(cardID int64) (ease float64, interval int) {
	ease, interval = 2.5, 0
	if prev, _ := s.store.LastReview(cardID); prev != nil {
		ease, interval = prev.Ease, prev.Interval
	}
	return
}

func (s *Study) bumpSessionCounter() {
	if s.session == nil {
		return
	}
	reviewed := s.idx + 1
	s.store.UpdateSession(s.session.ID, &reviewed, nil, false)
}

func (s *Study) advance() tea.Cmd {
	if s.cancel != nil {
		s.cancel()
	}
	s.flushPendingGrade()
	s.idx++
	if s.idx >= len(s.cards) {
		s.stage = stageDone
		s.markSessionEnded()
		return nil
	}
	return s.resetPerCardState()
}

func (s *Study) previous() tea.Cmd {
	if s.idx <= 0 {
		return nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.flushPendingGrade()
	s.idx--
	return s.resetPerCardState()
}

func (s *Study) endAndPop() tea.Cmd {
	s.markSessionEnded()
	return navBack
}

// Follow-up chat (AI-graded stageAnswered) lets the user argue with the
// grader. The grader's new FINAL_GRADE overrides the previous one.
const (
	followUpMinH = 1
	followUpMaxH = 8
)

type followUpSubmitMsg struct{ content string }

func (s *Study) ensureFollowUpEditor() tea.Cmd {
	if s.followUpEditor != nil {
		return nil
	}
	ed := vimtea.NewEditor(
		vimtea.WithContent(""),
		vimtea.WithEnableStatusBar(false),
		vimtea.WithHideLineNumbers(),
		vimtea.WithFileName("followup.md"),
	)
	submit := func(b vimtea.Buffer) tea.Cmd {
		text := b.Text()
		return func() tea.Msg { return followUpSubmitMsg{content: text} }
	}
	ed.AddBinding(vimtea.KeyBinding{Key: "ctrl+s", Mode: vimtea.ModeNormal, Handler: submit})
	ed.AddBinding(vimtea.KeyBinding{Key: "ctrl+s", Mode: vimtea.ModeInsert, Handler: submit})
	ed.SetSize(max(20, s.w-2), followUpMinH)
	s.followUpEditor = ed
	s.followUpH = followUpMinH
	return tea.Batch(ed.Init(), ed.SetMode(vimtea.ModeInsert))
}

func (s *Study) clampFollowUpH(n int) int {
	return max(followUpMinH, min(followUpMaxH, n))
}

func (s *Study) fitFollowUp() {
	if s.followUpEditor == nil {
		return
	}
	lines := s.clampFollowUpH(s.followUpEditor.GetBuffer().LineCount())
	if lines == s.followUpH {
		return
	}
	s.followUpH = lines
	s.followUpEditor.SetSize(max(20, s.w-2), s.followUpH)
}

// refreshGradeViewport renders the full chat transcript (prompt, the
// student's answer, every committed grader/user turn, and the in-progress
// assistant reply) into the shared viewport. The very first user turn is
// the seeded "Student's answer: ..." recap and is skipped since its content
// is already shown by the answer code box.
func (s *Study) refreshGradeViewport() {
	w := s.gradeViewport.Width
	if w <= 0 {
		w = max(40, s.w)
	}
	card := s.current()
	var parts []string
	if card != nil {
		if p := strings.TrimRight(renderPrompt(card.Prompt, w), "\n"); p != "" {
			parts = append(parts, p)
		}
		answer, label := s.answerForViewport(card)
		if strings.TrimSpace(answer) != "" {
			parts = append(parts, tui.StyleMuted.Render(label+":"))
			parts = append(parts, renderMarkdown(fmt.Sprintf("```%s\n%s\n```", card.Language, answer), w))
		}
	}
	for i, t := range s.gradingHistory {
		if i == 0 && t.Role == "user" {
			continue
		}
		parts = append(parts, formatGradeTurn(t.Role, t.Content, w))
	}
	if s.graderBuf != "" {
		parts = append(parts, formatGradeTurn("assistant", s.graderBuf, w))
	}
	s.gradeViewport.SetContent(strings.Join(parts, "\n\n"))
	s.gradeViewport.GotoBottom()
}

func (s *Study) answerForViewport(card *models.Card) (answer, label string) {
	if card == nil {
		return "", ""
	}
	if card.Type == models.CardExp {
		return s.explanationAnswer, "annotated source"
	}
	return s.codeAnswer, "your answer"
}

// resizeGradeViewport fits the chat viewport to the current window, leaving
// room for the chrome: deck header + blank + grade/spinner + blank + input
// box (if present) + status line. Called before every render so changes to
// followUpH (chat input growing/shrinking) are picked up immediately.
func (s *Study) resizeGradeViewport() {
	w := s.w
	if w <= 0 {
		w = 80
	}
	h := s.h
	if h <= 0 {
		h = 20
	}
	s.gradeViewport.Width = max(40, w)

	chrome := 5 // deck header (1) + spacer (1) + spacer (1) + grade/spinner (1) + status line (1)
	if s.stage == stageAnswered && s.followUpEditor != nil {
		chrome += s.followUpH + 2 // bordered input
	}
	s.gradeViewport.Height = max(6, h-chrome)
}

// handleGradeScroll maps scroll keys onto the chat viewport. Runs before
// key routing to vim/follow-up so scroll works regardless of what's
// focused. Returns true if the key was handled.
func (s *Study) handleGradeScroll(m tea.KeyMsg) bool {
	if s.stage != stageAnswered && s.stage != stageGrading {
		return false
	}
	card := s.current()
	if card == nil || !models.Kind(card.Type).IsAIGraded {
		return false
	}
	switch m.String() {
	case "shift+up":
		s.gradeViewport.LineUp(1)
	case "shift+down":
		s.gradeViewport.LineDown(1)
	case "pgup":
		s.gradeViewport.HalfViewUp()
	case "pgdown", "pgdn":
		s.gradeViewport.HalfViewDown()
	case "home":
		s.gradeViewport.GotoTop()
	case "end":
		s.gradeViewport.GotoBottom()
	default:
		return false
	}
	return true
}

func formatGradeTurn(role, content string, width int) string {
	tag := tui.StylePrimary.Render("you ›")
	if role == "assistant" {
		tag = tui.StyleAccent.Render("grader ›")
	}
	body := lipgloss.NewStyle().Width(max(20, width)).Render(content)
	return tag + "\n" + body
}

// submitFollowUp appends the user's follow-up to the history and re-streams
// the grader with the full conversation.
func (s *Study) submitFollowUp(text string) tea.Cmd {
	text = strings.TrimSpace(text)
	if text == "" || s.followUpStreaming || s.gradingInput == nil {
		return nil
	}
	client, err := resolveAIClient(s.store)
	if err != nil {
		s.graderErr = fmt.Errorf("%w", err)
		return nil
	}
	s.gradingHistory = append(s.gradingHistory, models.GradingMessage{
		Role:    "user",
		Content: text,
	})
	s.followUpEditor.GetBuffer().Clear()
	s.fitFollowUp()

	s.ctx, s.cancel = context.WithTimeout(context.Background(), gradeTimeout)
	s.graderBuf = ""
	s.graderErr = nil
	s.followUpStreaming = true

	in := *s.gradingInput
	in.History = append([]models.GradingMessage{}, s.gradingHistory...)
	s.streamCh = client.Grade(s.ctx, in)
	s.refreshGradeViewport()
	return tea.Batch(s.spin.Tick, pumpStream(s.streamCh))
}

func (s *Study) markSessionEnded() {
	if s.session == nil {
		return
	}
	t := time.Now().UTC()
	s.store.UpdateSession(s.session.ID, nil, &t, false)
}

func (s *Study) View() string {
	if s.cards == nil {
		return tui.StyleMuted.Render(i18n.T(i18n.KeyStudyLoading))
	}
	if s.stage == stageDone || len(s.cards) == 0 {
		return s.viewDone()
	}

	card := s.current()
	if card == nil {
		return s.viewDone()
	}
	body := lipgloss.JoinVertical(lipgloss.Left, s.renderHeader(), "", s.renderBody(card))
	if s.pendingDelete {
		prompt := tui.StyleDanger.Render(i18n.Tf(i18n.KeyStudyDeleteConfirm, card.ID))
		body = lipgloss.JoinVertical(lipgloss.Left, body, "", prompt)
	}
	return body
}

func (s *Study) renderHeader() string {
	return lipgloss.JoinHorizontal(lipgloss.Top,
		tui.StyleTitle.Render(s.deck.Name),
		"   ",
		tui.StyleMuted.Render(i18n.Tf(i18n.KeyStudyCardCounter, s.idx+1, len(s.cards))),
	)
}

func (s *Study) renderBody(card *models.Card) string {
	if b, ok := studyBehaviors[card.Type]; ok && b.Render != nil {
		return b.Render(s, card)
	}
	return tui.StyleMuted.Render(i18n.T(i18n.KeyStudyUnknownType))
}

func (s *Study) viewDone() string {
	if len(s.cards) == 0 {
		return tui.StyleMuted.Render(i18n.T(i18n.KeyStudyNothingDue))
	}
	return tui.StylePrimary.Render(i18n.T(i18n.KeyStudySessionDone))
}

func (s *Study) HelpKeys() []string {
	if s.pendingDelete {
		return []string{
			i18n.Help("y", i18n.KeyHelpYDelete),
			i18n.Help("N", i18n.KeyHelpNCancel),
		}
	}
	if s.stage == stageDone || len(s.cards) == 0 {
		return []string{i18n.Help("enter", i18n.KeyHelpBack)}
	}
	card := s.current()
	if card == nil {
		return []string{i18n.Help("esc", i18n.KeyHelpEnd)}
	}
	switch s.stage {
	case stageQuestion:
		return withLifecycleHelp(questionStageHelp(card.Type), s.idx)
	case stageGrading:
		return []string{
			i18n.Help("ctrl+x", i18n.KeyHelpCancel),
			i18n.Help("ctrl+e", i18n.KeyHelpEdit),
			i18n.Help("ctrl+d", i18n.KeyHelpDelete),
		}
	case stageAnswered:
		return withLifecycleHelp(answeredStageHelp(card.Type), s.idx)
	}
	return []string{i18n.Help("esc", i18n.KeyHelpEnd)}
}

func withLifecycleHelp(base []string, idx int) []string {
	out := append([]string{}, base...)
	out = append(out, i18n.Help("ctrl+n", i18n.KeyHelpNext))
	if idx > 0 {
		out = append(out, i18n.Help("ctrl+p", i18n.KeyHelpPrev))
	}
	return append(out,
		i18n.Help("ctrl+e", i18n.KeyHelpEdit),
		i18n.Help("ctrl+d", i18n.KeyHelpDelete),
	)
}

func questionStageHelp(t models.CardType) []string {
	if fn := ui(t).QuestionFn; fn != nil {
		return fn()
	}
	return nil
}
func answeredStageHelp(t models.CardType) []string {
	if fn := ui(t).AnsweredFn; fn != nil {
		return fn()
	}
	return nil
}
