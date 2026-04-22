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

// Study is the review-loop screen. It drives a session of due cards through
// the stages defined above. Card-type-specific logic lives in study_mcq.go,
// study_fill.go, and study_code.go to keep this file focused on the overall
// state machine.
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

	ctx         context.Context
	cancel      context.CancelFunc
	streamCh    <-chan ai.Event
	spin        spinner.Model
	vp          viewport.Model
	grader      string
	graderErr   error
	graderGrade int

	resultGrade int
	resultNote  string

	w, h int

	codeEditor vimtea.Editor
}

func NewStudy(s *store.Store, d models.Deck) *Study {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return &Study{store: s, deck: d, spin: sp, vp: viewport.New(80, 12)}
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

// resetPerCardState reinitializes all card-type-specific state (MCQ cursor,
// fill inputs, code editor) for the card at s.idx. Called after advancing.
func (s *Study) resetPerCardState() tea.Cmd {
	card := s.current()
	if card == nil {
		return nil
	}
	s.mcqCursor = 0
	s.codeAnswer = card.InitialCode
	s.explanationAnswer = ""
	s.grader = ""
	s.graderErr = nil
	s.graderGrade = 0
	s.resultGrade = 0
	s.resultNote = ""
	s.stage = stageQuestion
	s.codeEditor = nil

	switch card.Type {
	case models.CardFill:
		s.initFillInputs(card)
	default:
		s.fillInputs = nil
	}

	if card.Type == models.CardExp {
		s.explanationAnswer = extractCodeBlock(card.Prompt)
	}
	if card.Type == models.CardCode || card.Type == models.CardExp {
		return s.initCodeEditor(card)
	}
	return nil
}

const (
	// Rows study.View adds around the per-card body: deck header + blank.
	studyChromeRows = 2
	// "your answer:" label row above the editor.
	studyRightLabel = 1
	// Blank line + label between prompt and editor.
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

// editorHeight returns a fallback used when the editor is created before the
// prompt has been rendered. View computes the real value each frame.
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
		s.grader += m.text
		s.vp.SetContent(s.grader)
		s.vp.GotoBottom()
		return s, pumpStream(s.streamCh)

	case streamDoneMsg:
		if m.full != "" {
			s.grader = m.full
		}
		s.stage = stageAnswered
		grade, ok := extractGrade(s.grader)
		if !ok {
			s.graderGrade = 0
			s.graderErr = fmt.Errorf("grader did not return a FINAL_GRADE — use 1-5 to grade manually")
			return s, nil
		}
		s.graderGrade = grade
		return s, s.recordReview(grade)

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

// forwardToEmbedded hands non-key Bubble Tea messages (cursor blinks, etc.)
// to whichever inner widget is active for the current card type so its
// animations stay alive.
func (s *Study) forwardToEmbedded(msg tea.Msg) tea.Cmd {
	if s.stage != stageQuestion {
		return nil
	}
	card := s.current()
	if card == nil {
		return nil
	}
	switch card.Type {
	case models.CardCode, models.CardExp:
		if s.codeEditor != nil {
			_, cmd := s.codeEditor.Update(msg)
			return cmd
		}
	case models.CardFill:
		if s.fillFocus < len(s.fillInputs) {
			var cmd tea.Cmd
			s.fillInputs[s.fillFocus], cmd = s.fillInputs[s.fillFocus].Update(msg)
			return cmd
		}
	}
	return nil
}

func (s *Study) handleKey(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	card := s.current()

	// Code/exp cards in the question stage route keys to the inline vim
	// editor. Esc from normal mode is the only study-level escape hatch.
	if s.stage == stageQuestion && card != nil && s.codeEditor != nil &&
		(card.Type == models.CardCode || card.Type == models.CardExp) {
		if m.String() == "esc" && s.codeEditor.GetMode() == vimtea.ModeNormal {
			return s, s.cancelAndExit()
		}
		_, cmd := s.codeEditor.Update(m)
		return s, cmd
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

func (s *Study) handleQuestionKey(m tea.KeyMsg, card *models.Card) (tui.Screen, tea.Cmd) {
	switch card.Type {
	case models.CardMCQ:
		return s.handleMCQKey(m, card)
	case models.CardFill:
		return s.handleFillKey(m, card)
	}
	// Code/exp keys are consumed by the inline vim editor above.
	return s, nil
}

func (s *Study) handleAnsweredKey(m tea.KeyMsg, card *models.Card) (tui.Screen, tea.Cmd) {
	switch m.String() {
	case "1", "2", "3", "4", "5":
		if card.Type == models.CardCode || card.Type == models.CardExp {
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

// recordReview persists a grade, computes the next SRS schedule, and bumps the
// session counter. Out-of-range grades are a no-op so extractGrade's fallback
// can't accidentally record a 0.
func (s *Study) recordReview(grade int) tea.Cmd {
	card := s.current()
	if card == nil || grade < 1 || grade > 5 {
		return nil
	}
	prev, _ := s.store.LastReview(card.ID)
	ease := 2.5
	interval := 0
	if prev != nil {
		ease = prev.Ease
		interval = prev.Interval
	}
	r := srs.CalculateNext(grade, ease, interval)
	if _, err := s.store.CreateReview(card.ID, grade, r.Ease, r.Interval, r.NextDue); err != nil {
		return tui.ToastErr("review save failed: " + err.Error())
	}
	if s.session != nil {
		cr := s.idx + 1
		s.store.UpdateSession(s.session.ID, &cr, nil, false)
	}
	return nil
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
	return func() tea.Msg { return tui.NavMsg{Pop: true} }
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

	header := lipgloss.JoinHorizontal(lipgloss.Top,
		tui.StyleTitle.Render(s.deck.Name),
		"   ",
		tui.StyleMuted.Render(fmt.Sprintf("card %d / %d", s.idx+1, len(s.cards))),
	)

	var body string
	switch card.Type {
	case models.CardMCQ:
		body = s.viewMCQ(card)
	case models.CardFill:
		body = s.viewFill(card)
	case models.CardCode:
		body = s.viewCode(card)
	case models.CardExp:
		body = s.viewExp(card)
	default:
		body = tui.StyleMuted.Render("(unknown card type)")
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, "", body)
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
		switch card.Type {
		case models.CardMCQ:
			return []string{"↑/↓ pick", "enter submit", "esc end"}
		case models.CardFill:
			return []string{"tab switch", "enter submit", "esc end"}
		case models.CardCode, models.CardExp:
			return []string{"i insert", "esc normal", "ctrl+s submit", "esc end (from normal)"}
		}
	case stageGrading:
		return []string{"ctrl+x cancel"}
	case stageAnswered:
		if card.Type == models.CardCode || card.Type == models.CardExp {
			return []string{"1-5 override", "enter next", "esc end"}
		}
		return []string{"enter next", "esc end"}
	}
	return []string{"esc end"}
}
