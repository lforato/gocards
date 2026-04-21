package tui

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
)

// codeSubmitMsg is emitted by the inline vimtea editor's ctrl+s binding and
// carries the user's final answer so the Study screen can start grading.
type codeSubmitMsg struct{ content string }

type studyStage int

const (
	stageQuestion studyStage = iota
	stageAnswered            // user submitted answer; MCQ/fill shows instant result, code/exp shows graded view
	stageGrading             // streaming the grader
	stageDone
)

type Study struct {
	store *store.Store
	deck  models.Deck

	cards   []models.Card
	idx     int
	session *models.StudySession

	stage studyStage

	// answering state
	mcqCursor         int
	fillInputs        []textinput.Model
	fillFocus         int
	codeAnswer        string
	explanationAnswer string

	// grading state (code/exp cards)
	ctx         context.Context
	cancel      context.CancelFunc
	streamCh    <-chan ai.Event
	spin        spinner.Model
	vp          viewport.Model
	grader      string
	graderErr   error
	graderGrade int

	// result for mcq/fill
	resultGrade int
	resultNote  string

	// screen dimensions (from WindowSizeMsg) used to size the inline editor
	w, h int

	// inline vim editor used for CardCode / CardExp question stage
	codeEditor vimtea.Editor
}

func NewStudy(s *store.Store, d models.Deck) *Study {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	vp := viewport.New(80, 12)
	return &Study{store: s, deck: d, spin: sp, vp: vp}
}

type studyLoadedMsg struct {
	cards   []models.Card
	session *models.StudySession
	err     error
}

func (s *Study) Init() tea.Cmd {
	return tea.Batch(
		s.spin.Tick,
		func() tea.Msg {
			cards, err := s.store.DueCards(s.deck.ID, s.store.DailyLimit())
			if err != nil {
				return studyLoadedMsg{err: err}
			}
			sess, err := s.store.CreateSession(s.deck.ID)
			if err != nil {
				return studyLoadedMsg{err: err}
			}
			return studyLoadedMsg{cards: cards, session: sess}
		},
	)
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
	s.grader = ""
	s.graderErr = nil
	s.graderGrade = 0
	s.resultGrade = 0
	s.resultNote = ""
	s.stage = stageQuestion
	s.codeEditor = nil

	// prep fill inputs
	if card.Type == models.CardFill && card.BlanksData != nil {
		s.fillInputs = make([]textinput.Model, len(card.BlanksData.Blanks))
		for i := range s.fillInputs {
			ti := textinput.New()
			ti.CharLimit = 100
			ti.Width = 32
			if i == 0 {
				ti.Focus()
			}
			s.fillInputs[i] = ti
		}
		s.fillFocus = 0
	} else {
		s.fillInputs = nil
	}

	// preload code answer for "exp" cards: extract the fenced code block from the prompt.
	if card.Type == models.CardExp {
		s.explanationAnswer = extractCodeBlock(card.Prompt)
	}

	if card.Type == models.CardCode || card.Type == models.CardExp {
		initial := card.InitialCode
		if card.Type == models.CardExp {
			initial = s.explanationAnswer
		}
		ed := vimtea.NewEditor(
			vimtea.WithContent(initial),
			vimtea.WithFileName("code"+langExt(card.Language)),
			vimtea.WithEnableStatusBar(false),
		)
		ed.AddBinding(vimtea.KeyBinding{
			Key:  "ctrl+s",
			Mode: vimtea.ModeNormal,
			Handler: func(b vimtea.Buffer) tea.Cmd {
				content := b.Text()
				return func() tea.Msg { return codeSubmitMsg{content: content} }
			},
		})
		ed.SetSize(s.editorWidth(), s.editorHeight())
		s.codeEditor = ed
		return ed.Init()
	}
	return nil
}

// Study layout — prompt stacked on top, vim editor below, both full width:
//
//	<header>                                   (1 row)
//	<blank>                                    (1 row)
//	prompt text (full width, wraps)            promptH rows
//	<blank>                                    (1 row)
//	your answer:                               (1 row)
//	<vim editor, fills the rest>               editorH rows
//	<blank>                                    (1 row)
//	<help>                                     (1 row)
//	<blank>                                    (1 row)
//
// The editor height depends on how many rows the prompt occupies at the
// current screen width, so View computes it from lipgloss.Height(prompt).

const (
	// studyChromeRows covers the rows that study.View adds around the per-card
	// body: deck header (1) + blank (1). The global help is now rendered by
	// app.go, so no rows are reserved for it here.
	studyChromeRows   = 2
	studyRightLabel   = 1 // "your answer:" line above the editor
	studyPromptChrome = 2 // blank line + label between prompt and editor
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

// editorHeight returns a fallback size used when the editor is created before
// the prompt has been rendered (View computes the exact value and calls
// SetSize with it every frame).
func (s *Study) editorHeight() int {
	return max(5, s.bodyHeight()-studyRightLabel-studyPromptChrome-2)
}

func (s *Study) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		s.w = m.Width
		s.h = m.Height
		if s.codeEditor != nil {
			s.codeEditor.SetSize(s.editorWidth(), s.editorHeight())
		}
		return s, nil

	case studyLoadedMsg:
		if m.err != nil {
			return s, ToastErr("study load: " + m.err.Error())
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
		s.graderGrade = extractGrade(s.grader)
		s.stage = stageAnswered
		// record review
		return s, s.recordReview(s.graderGrade)

	case streamErrMsg:
		s.graderErr = m.err
		s.stage = stageAnswered
		return s, nil

	case codeSubmitMsg:
		if s.stage != stageQuestion {
			return s, nil
		}
		card := s.current()
		if card == nil {
			return s, nil
		}
		if card.Type == models.CardCode {
			s.codeAnswer = m.content
		} else if card.Type == models.CardExp {
			s.explanationAnswer = m.content
		}
		return s, s.startGrading()

	case tea.KeyMsg:
		return s.handleKey(m)
	}

	if s.stage == stageQuestion {
		card := s.current()
		if card != nil && s.codeEditor != nil &&
			(card.Type == models.CardCode || card.Type == models.CardExp) {
			_, cmd := s.codeEditor.Update(msg)
			return s, cmd
		}
		if card != nil && card.Type == models.CardFill && s.fillFocus < len(s.fillInputs) {
			var cmd tea.Cmd
			s.fillInputs[s.fillFocus], cmd = s.fillInputs[s.fillFocus].Update(msg)
			return s, cmd
		}
	}
	return s, nil
}

func (s *Study) handleKey(m tea.KeyMsg) (Screen, tea.Cmd) {
	// Code/exp cards in the question stage route keys to the inline vim editor.
	// Esc in normal mode is the only study-level escape hatch; all other keys
	// (including esc while in insert/visual) go to vimtea.
	card := s.current()
	if s.stage == stageQuestion && card != nil && s.codeEditor != nil &&
		(card.Type == models.CardCode || card.Type == models.CardExp) {
		if m.String() == "esc" && s.codeEditor.GetMode() == vimtea.ModeNormal {
			if s.cancel != nil {
				s.cancel()
			}
			return s, s.endAndPop()
		}
		_, cmd := s.codeEditor.Update(m)
		return s, cmd
	}

	switch m.String() {
	case "esc":
		if s.cancel != nil {
			s.cancel()
		}
		return s, s.endAndPop()
	}

	if s.stage == stageDone {
		switch m.String() {
		case "enter", "q":
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
		// numeric grade override (1-5) then next
		switch m.String() {
		case "1", "2", "3", "4", "5":
			g, _ := strconv.Atoi(m.String())
			if card.Type == models.CardCode || card.Type == models.CardExp {
				// user overrides AI grade
				return s, s.recordReview(g)
			}
			return s, nil
		case "enter", "n":
			return s, s.advance()
		}
	}
	return s, nil
}

func (s *Study) handleQuestionKey(m tea.KeyMsg, card *models.Card) (Screen, tea.Cmd) {
	switch card.Type {
	case models.CardMCQ:
		switch m.String() {
		case "up", "k":
			if s.mcqCursor > 0 {
				s.mcqCursor--
			}
		case "down", "j":
			if s.mcqCursor < len(card.Choices)-1 {
				s.mcqCursor++
			}
		case "enter", " ":
			chosen := card.Choices[s.mcqCursor]
			grade := 5
			if !chosen.IsCorrect {
				grade = 1
			}
			s.resultGrade = grade
			s.resultNote = "correct"
			if grade == 1 {
				s.resultNote = "incorrect"
			}
			s.stage = stageAnswered
			return s, s.recordReview(grade)
		}

	case models.CardFill:
		switch m.String() {
		case "tab", "down":
			if len(s.fillInputs) == 0 {
				return s, nil
			}
			s.fillInputs[s.fillFocus].Blur()
			s.fillFocus = (s.fillFocus + 1) % len(s.fillInputs)
			s.fillInputs[s.fillFocus].Focus()
		case "shift+tab", "up":
			if len(s.fillInputs) == 0 {
				return s, nil
			}
			s.fillInputs[s.fillFocus].Blur()
			s.fillFocus = (s.fillFocus - 1 + len(s.fillInputs)) % len(s.fillInputs)
			s.fillInputs[s.fillFocus].Focus()
		case "ctrl+s", "enter":
			if len(s.fillInputs) == 0 || card.BlanksData == nil {
				return s, nil
			}
			allCorrect := true
			partial := 0
			for i, ti := range s.fillInputs {
				want := strings.TrimSpace(card.BlanksData.Blanks[i])
				got := strings.TrimSpace(ti.Value())
				if strings.EqualFold(got, want) {
					partial++
				} else {
					allCorrect = false
				}
			}
			grade := 1
			switch {
			case allCorrect:
				grade = 5
			case partial > 0:
				grade = 3
			}
			s.resultGrade = grade
			s.resultNote = fmt.Sprintf("%d / %d blanks correct", partial, len(s.fillInputs))
			s.stage = stageAnswered
			return s, s.recordReview(grade)
		default:
			if len(s.fillInputs) == 0 {
				return s, nil
			}
			var cmd tea.Cmd
			s.fillInputs[s.fillFocus], cmd = s.fillInputs[s.fillFocus].Update(m)
			return s, cmd
		}

	case models.CardCode, models.CardExp:
		// Keys for code/exp in the question stage are consumed by the inline
		// vim editor in handleKey before reaching here.
	}
	return s, nil
}

func (s *Study) startGrading() tea.Cmd {
	card := s.current()
	if card == nil {
		return nil
	}
	apiKey := ai.ResolveAPIKey(func() (string, bool, error) {
		return s.store.GetSetting("apiKey")
	})
	if apiKey == "" {
		// no key → fall back to self-grade
		s.stage = stageAnswered
		s.graderErr = fmt.Errorf("no API key configured — grade manually with 1-5")
		return nil
	}

	var userAnswer string
	mode := "code"
	if card.Type == models.CardExp {
		userAnswer = s.explanationAnswer
		mode = "explanation"
	} else {
		userAnswer = s.codeAnswer
	}

	client := ai.New(apiKey)
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.grader = ""
	s.graderGrade = 0
	s.stage = stageGrading
	s.streamCh = client.Grade(s.ctx, ai.GradeInput{
		Prompt:         card.Prompt,
		ExpectedAnswer: card.ExpectedAnswer,
		UserAnswer:     userAnswer,
		Mode:           mode,
	})
	return tea.Batch(s.spin.Tick, pumpStream(s.streamCh))
}

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
		return ToastErr("review save failed: " + err.Error())
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
		if s.session != nil {
			t := time.Now().UTC()
			s.store.UpdateSession(s.session.ID, nil, &t, false)
		}
		return nil
	}
	return s.resetPerCardState()
}

func (s *Study) endAndPop() tea.Cmd {
	if s.session != nil {
		t := time.Now().UTC()
		s.store.UpdateSession(s.session.ID, nil, &t, false)
	}
	return func() tea.Msg { return NavMsg{Pop: true} }
}

// --- view ---

func (s *Study) View() string {
	if s.cards == nil {
		return StyleMuted.Render("loading…")
	}
	if s.stage == stageDone || len(s.cards) == 0 {
		return s.viewDone()
	}

	card := s.current()
	if card == nil {
		return s.viewDone()
	}

	header := lipgloss.JoinHorizontal(lipgloss.Top,
		StyleTitle.Render(s.deck.Name),
		"   ",
		StyleMuted.Render(fmt.Sprintf("card %d / %d", s.idx+1, len(s.cards))),
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
		body = StyleMuted.Render("(unknown card type)")
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, "", body)
}

func (s *Study) viewMCQ(card *models.Card) string {
	rows := []string{renderPrompt(card.Prompt), ""}
	for i, ch := range card.Choices {
		prefix := "  "
		if i == s.mcqCursor && s.stage == stageQuestion {
			prefix = StylePrimary.Render("▶ ")
		}
		label := fmt.Sprintf("%s. %s", ch.ID, ch.Text)
		if s.stage == stageAnswered {
			switch {
			case ch.IsCorrect:
				label = StyleSuccess.Render(label + "  ✓")
			case i == s.mcqCursor && !ch.IsCorrect:
				label = StyleDanger.Render(label + "  ✗")
			default:
				label = StyleMuted.Render(label)
			}
		} else if i == s.mcqCursor {
			label = StyleSelected.Render(label)
		}
		rows = append(rows, prefix+label)
	}
	if s.stage == stageAnswered {
		rows = append(rows, "", StylePrimary.Render(fmt.Sprintf("→ %s  (grade %d)", s.resultNote, s.resultGrade)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

var fillBlankRe = regexp.MustCompile(`\{\{([^{}]*)\}\}`)

func renderFillTemplate(s string) string {
	return fillBlankRe.ReplaceAllStringFunc(s, func(match string) string {
		inner := match[2 : len(match)-2]
		return strings.Repeat("_", utf8.RuneCountInString(inner))
	})
}

func (s *Study) viewFill(card *models.Card) string {
	template := ""
	if card.BlanksData != nil {
		template = renderFillTemplate(card.BlanksData.Template)
	}

	inputsSection := []string{StyleMuted.Render("blanks:")}
	for i := range s.fillInputs {
		prefix := fmt.Sprintf("%d: ", i+1)
		if i == s.fillFocus {
			prefix = StylePrimary.Render("▶ ") + prefix
		} else {
			prefix = "  " + prefix
		}
		inputsSection = append(inputsSection, prefix+s.fillInputs[i].View())
	}

	rows := []string{
		renderPrompt(card.Prompt),
		"",
		codeBox(template),
		"",
	}
	rows = append(rows, inputsSection...)
	rows = append(rows, "")

	if s.stage == stageAnswered {
		rows = append(rows, StylePrimary.Render(fmt.Sprintf("→ %s  (grade %d)", s.resultNote, s.resultGrade)))
		if card.BlanksData != nil {
			rows = append(rows, StyleMuted.Render("answers: "+strings.Join(card.BlanksData.Blanks, ", ")))
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (s *Study) viewCode(card *models.Card) string {
	if s.stage == stageQuestion {
		return s.viewCodeQuestion(card, "your answer")
	}
	rows := []string{renderPrompt(card.Prompt), ""}
	if s.codeAnswer != "" {
		rows = append(rows, StyleMuted.Render("your answer:"), codeBox(s.codeAnswer), "")
	}
	rows = append(rows, s.viewGrading()...)
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (s *Study) viewExp(card *models.Card) string {
	if s.stage == stageQuestion {
		return s.viewCodeQuestion(card, "annotated source")
	}
	rows := []string{renderPrompt(card.Prompt), ""}
	if s.explanationAnswer != "" {
		rows = append(rows, StyleMuted.Render("annotated source:"), codeBox(s.explanationAnswer), "")
	}
	rows = append(rows, s.viewGrading()...)
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// viewCodeQuestion renders the question-stage layout for CardCode and CardExp:
// prompt on top, full-width vim editor below, help line at the bottom.
func (s *Study) viewCodeQuestion(card *models.Card, editorLabel string) string {
	totalW := s.editorWidth()
	bodyH := s.bodyHeight()

	prompt := lipgloss.NewStyle().Width(totalW).Render(renderPrompt(card.Prompt))
	promptH := lipgloss.Height(prompt)

	editorH := max(5, bodyH-promptH-studyPromptChrome-studyRightLabel)
	var editorView string
	if s.codeEditor != nil {
		s.codeEditor.SetSize(totalW, editorH)
		editorView = s.codeEditor.View()
	}

	body := lipgloss.JoinVertical(lipgloss.Left,
		prompt,
		"",
		StyleMuted.Render(editorLabel+":"),
		editorView,
	)

	return body
}

// viewGrading returns the rows shown below a code/exp prompt while the grader
// streams, and after the grader finishes. Shared between CardCode and CardExp.
func (s *Study) viewGrading() []string {
	var rows []string
	switch s.stage {
	case stageGrading:
		rows = append(rows, s.spin.View()+" grading…", "",
			lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorBorder).Padding(0, 1).Render(s.vp.View()))
	case stageAnswered:
		if s.graderErr != nil {
			rows = append(rows, StyleDanger.Render(s.graderErr.Error()))
		}
		rows = append(rows, StyleMuted.Render("grader:"))
		rows = append(rows, renderPrompt(s.grader))
		rows = append(rows, "", StylePrimary.Render(fmt.Sprintf("grade: %d", s.graderGrade)))
	}
	return rows
}

func (s *Study) viewDone() string {
	if len(s.cards) == 0 {
		return StyleMuted.Render("nothing due — check back later")
	}
	return StylePrimary.Render("🎉 session complete")
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
		switch card.Type {
		case models.CardCode, models.CardExp:
			return []string{"1-5 override", "enter next", "esc end"}
		}
		return []string{"enter next", "esc end"}
	}
	return []string{"esc end"}
}

// --- helpers ---

func renderPrompt(p string) string {
	// Render fenced code blocks specially.
	lines := strings.Split(p, "\n")
	var out []string
	var inCode bool
	var buf []string
	for _, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "```") {
			if inCode {
				out = append(out, codeBox(strings.Join(buf, "\n")))
				buf = nil
				inCode = false
			} else {
				inCode = true
			}
			continue
		}
		if inCode {
			buf = append(buf, ln)
			continue
		}
		out = append(out, ln)
	}
	if len(buf) > 0 {
		out = append(out, codeBox(strings.Join(buf, "\n")))
	}
	return strings.Join(out, "\n")
}

func codeBox(s string) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Foreground(lipgloss.Color("#d1d5db")).
		Padding(0, 1).
		Render(s)
}

func extractCodeBlock(prompt string) string {
	lines := strings.Split(prompt, "\n")
	var buf []string
	in := false
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "```") {
			if in {
				return strings.Join(buf, "\n")
			}
			in = true
			continue
		}
		if in {
			buf = append(buf, ln)
		}
	}
	return strings.Join(buf, "\n")
}

var gradeRegex = regexp.MustCompile(`FINAL_GRADE:\s*([1-5])`)

func extractGrade(text string) int {
	m := gradeRegex.FindStringSubmatch(text)
	if len(m) >= 2 {
		g, _ := strconv.Atoi(m[1])
		return g
	}
	return 0
}
