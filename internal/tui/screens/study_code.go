package screens

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lforato/vimtea"

	"github.com/lforato/gocards/internal/ai"
	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/tui"
	"github.com/lforato/gocards/internal/tui/widgets"
)

// gradeTimeout bounds how long we wait for the grader to finish streaming.
// If the model hangs, the ctx fires and the user sees the timeout error
// rather than watching the spinner forever.
const gradeTimeout = 60 * time.Second

// codeSubmitMsg is emitted by the inline vimtea editor's ctrl+s binding and
// carries the user's final answer so the Study screen can start grading.
type codeSubmitMsg struct{ content string }

func (s *Study) initCodeEditor(card *models.Card) tea.Cmd {
	initial := card.InitialCode
	if card.Type == models.CardExp {
		initial = s.explanationAnswer
	}
	ed := vimtea.NewEditor(
		vimtea.WithContent(initial),
		vimtea.WithFileName("code"+widgets.LangExt(card.Language)),
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

func (s *Study) handleCodeSubmit(m codeSubmitMsg) (tui.Screen, tea.Cmd) {
	if s.stage != stageQuestion {
		return s, nil
	}
	card := s.current()
	if card == nil {
		return s, nil
	}
	switch card.Type {
	case models.CardCode:
		s.codeAnswer = m.content
	case models.CardExp:
		s.explanationAnswer = m.content
	default:
		return s, nil
	}
	return s, s.startGrading()
}

// startGrading kicks off an AI grading stream for the current code/exp card.
// If no API key is configured, the card falls back to manual 1-5 grading and
// a hint is surfaced in the grader error panel.
func (s *Study) startGrading() tea.Cmd {
	card := s.current()
	if card == nil {
		return nil
	}
	if card.Type != models.CardCode && card.Type != models.CardExp {
		return nil
	}
	client, err := resolveAIClient(s.store)
	if err != nil {
		s.stage = stageAnswered
		s.graderErr = fmt.Errorf("%w — grade manually with 1-5", err)
		return nil
	}

	var userAnswer, mode string
	switch card.Type {
	case models.CardExp:
		userAnswer = s.explanationAnswer
		mode = "explanation"
	case models.CardCode:
		userAnswer = s.codeAnswer
		mode = "code"
	}

	s.ctx, s.cancel = context.WithTimeout(context.Background(), gradeTimeout)
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

func (s *Study) viewCode(card *models.Card) string {
	return s.viewCodeOrExp(card, s.codeAnswer, "your answer")
}

func (s *Study) viewExp(card *models.Card) string {
	return s.viewCodeOrExp(card, s.explanationAnswer, "annotated source")
}

func (s *Study) viewCodeOrExp(card *models.Card, answer, label string) string {
	if s.stage == stageQuestion {
		return s.viewCodeQuestion(card, label)
	}
	rows := []string{renderPrompt(card.Prompt), ""}
	if answer != "" {
		rows = append(rows, tui.StyleMuted.Render(label+":"), codeBox(answer), "")
	}
	rows = append(rows, s.viewGrading()...)
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// viewCodeQuestion renders the question-stage layout: prompt on top,
// full-width vim editor below, help line at the bottom.
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

	return lipgloss.JoinVertical(lipgloss.Left,
		prompt,
		"",
		tui.StyleMuted.Render(editorLabel+":"),
		editorView,
	)
}

// viewGrading returns the rows shown below a code/exp prompt while the grader
// streams, and after the grader finishes. Shared between CardCode and CardExp.
func (s *Study) viewGrading() []string {
	switch s.stage {
	case stageGrading:
		return []string{
			s.spin.View() + " grading…",
			"",
			lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
				BorderForeground(tui.ColorBorder).Padding(0, 1).Render(s.vp.View()),
		}
	case stageAnswered:
		rows := []string{}
		if s.graderErr != nil {
			rows = append(rows, tui.StyleDanger.Render(s.graderErr.Error()))
		}
		rows = append(rows, tui.StyleMuted.Render("grader:"), renderPrompt(s.grader), "",
			tui.StylePrimary.Render(fmt.Sprintf("grade: %d", s.graderGrade)))
		return rows
	}
	return nil
}
