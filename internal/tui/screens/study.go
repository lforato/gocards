package screens

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lforato/vimtea"

	"github.com/lforato/gocards/internal/ai"
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

	resultGrade int
	resultNote  string

	w, h int

	codeEditor vimtea.Editor
}

func NewStudy(s *store.Store, d models.Deck) *Study {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return &Study{store: s, deck: d, spin: sp, gradeViewport: viewport.New(80, 12)}
}

type studyLoadedMsg struct {
	cards   []models.Card
	session *models.StudySession
	err     error
}

func (s *Study) Init() tea.Cmd {
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
		s.explanationAnswer = extractCodeBlock(card.Prompt)
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
		return s, nil

	case studyLoadedMsg:
		if m.err != nil {
			return s, tui.ToastErr("study load: " + m.err.Error())
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
		s.gradeViewport.SetContent(s.graderBuf)
		s.gradeViewport.GotoBottom()
		return s, pumpStream(s.streamCh)

	case streamDoneMsg:
		return s, s.finishGrade(m)

	case streamErrMsg:
		s.graderErr = m.err
		s.stage = stageAnswered
		return s, nil

	case codeSubmitMsg:
		return s.handleCodeSubmit(m)

	case tea.KeyMsg:
		return s.handleKey(m)
	}

	return s, s.forwardToEmbedded(msg)
}

func (s *Study) finishGrade(m streamDoneMsg) tea.Cmd {
	if m.full != "" {
		s.graderBuf = m.full
	}
	s.stage = stageAnswered
	grade, ok := extractGrade(s.graderBuf)
	if !ok {
		s.graderScore = 0
		s.graderErr = fmt.Errorf("grader did not return a FINAL_GRADE — use 1-5 to grade manually")
		return nil
	}
	s.graderScore = grade
	return s.recordReview(grade)
}

// forwardToEmbedded routes non-key ticks (cursor blinks etc.) to the widget
// active for the current card type so its animations stay alive.
func (s *Study) forwardToEmbedded(msg tea.Msg) tea.Cmd {
	if s.stage != stageQuestion {
		return nil
	}
	card := s.current()
	if card == nil {
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

	if s.inVimQuestion(card) {
		return s.routeToVim(m)
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
	switch m.String() {
	case "1", "2", "3", "4", "5":
		if models.Kind(card.Type).IsAIGraded {
			g, _ := strconv.Atoi(m.String())
			return s, s.recordReview(g)
		}
	case "enter", "n":
		return s, s.advance()
	}
	return s, nil
}

func (s *Study) cancelAndExit() tea.Cmd {
	if s.cancel != nil {
		s.cancel()
	}
	return s.endAndPop()
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
		return tui.ToastErr("review save failed: " + err.Error())
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
	s.idx++
	if s.idx >= len(s.cards) {
		s.stage = stageDone
		s.markSessionEnded()
		return nil
	}
	return s.resetPerCardState()
}

func (s *Study) endAndPop() tea.Cmd {
	s.markSessionEnded()
	return navBack
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
		return tui.StyleMuted.Render("loading…")
	}
	if s.stage == stageDone || len(s.cards) == 0 {
		return s.viewDone()
	}

	card := s.current()
	if card == nil {
		return s.viewDone()
	}
	return lipgloss.JoinVertical(lipgloss.Left, s.renderHeader(), "", s.renderBody(card))
}

func (s *Study) renderHeader() string {
	return lipgloss.JoinHorizontal(lipgloss.Top,
		tui.StyleTitle.Render(s.deck.Name),
		"   ",
		tui.StyleMuted.Render(fmt.Sprintf("card %d / %d", s.idx+1, len(s.cards))),
	)
}

func (s *Study) renderBody(card *models.Card) string {
	if b, ok := studyBehaviors[card.Type]; ok && b.Render != nil {
		return b.Render(s, card)
	}
	return tui.StyleMuted.Render("(unknown card type)")
}

func (s *Study) viewDone() string {
	if len(s.cards) == 0 {
		return tui.StyleMuted.Render("nothing due — check back later")
	}
	return tui.StylePrimary.Render("🎉 session complete")
}

func (s *Study) HelpKeys() []string {
	if s.stage == stageDone || len(s.cards) == 0 {
		return []string{"enter back"}
	}
	card := s.current()
	if card == nil {
		return []string{"esc end"}
	}
	switch s.stage {
	case stageQuestion:
		return questionStageHelp(card.Type)
	case stageGrading:
		return []string{"ctrl+x cancel"}
	case stageAnswered:
		return answeredStageHelp(card.Type)
	}
	return []string{"esc end"}
}

func questionStageHelp(t models.CardType) []string { return ui(t).QuestionHelp }
func answeredStageHelp(t models.CardType) []string { return ui(t).AnsweredHelp }
