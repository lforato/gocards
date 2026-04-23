package screens

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lforato/vimtea"

	"github.com/lforato/gocards/internal/ai"
	"github.com/lforato/gocards/internal/i18n"
	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/tui"
	"github.com/lforato/gocards/internal/tui/widgets"
)

// gradeTimeout caps how long we wait for the grader before the ctx fires.
const gradeTimeout = 60 * time.Second

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

func (s *Study) startGrading() tea.Cmd {
	card := s.current()
	if card == nil || !models.Kind(card.Type).IsAIGraded {
		return nil
	}
	client, err := resolveAIClient(s.store)
	if err != nil {
		s.stage = stageAnswered
		s.graderErr = fmt.Errorf("%w — grade manually with 1-5", err)
		return nil
	}

	userAnswer, mode := s.gradingInputFor(card)

	s.ctx, s.cancel = context.WithTimeout(context.Background(), gradeTimeout)
	s.graderBuf = ""
	s.graderScore = 0
	s.gradingHistory = nil
	s.followUpStreaming = false
	s.stage = stageGrading
	s.gradingInput = &ai.GradeInput{
		Prompt:         card.Prompt,
		ExpectedAnswer: card.ExpectedAnswer,
		UserAnswer:     userAnswer,
		Mode:           mode,
	}
	s.streamCh = client.Grade(s.ctx, *s.gradingInput)
	return tea.Batch(s.spin.Tick, pumpStream(s.streamCh))
}

func (s *Study) gradingInputFor(card *models.Card) (userAnswer, mode string) {
	if card.Type == models.CardExp {
		return s.explanationAnswer, "explanation"
	}
	return s.codeAnswer, "code"
}

func (s *Study) viewCode(card *models.Card) string {
	return s.viewCodeOrExp(card, i18n.T(i18n.KeyStudyAnswerLabel))
}

func (s *Study) viewExp(card *models.Card) string {
	return s.viewCodeOrExp(card, i18n.T(i18n.KeyStudyExpLabel))
}

func (s *Study) viewCodeOrExp(card *models.Card, label string) string {
	if s.stage == stageQuestion {
		return s.viewCodeQuestion(card, label)
	}
	s.resizeGradeViewport()
	s.refreshGradeViewport()
	return lipgloss.JoinVertical(lipgloss.Left, s.viewGrading()...)
}

func (s *Study) viewCodeQuestion(card *models.Card, editorLabel string) string {
	totalW := s.editorWidth()
	bodyH := s.bodyHeight()

	prompt := renderPrompt(card.Prompt, totalW)
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

// viewGrading renders the scrollable chat transcript (containing prompt +
// answer + all grader turns) followed by a status line or the follow-up
// input, matching the layout of the Generate With AI screen.
func (s *Study) viewGrading() []string {
	rows := []string{s.gradeViewport.View()}
	switch s.stage {
	case stageGrading:
		rows = append(rows, "", s.spin.View()+tui.StyleMuted.Render(" "+i18n.T(i18n.KeyStudyGrading)))
	case stageAnswered:
		if s.graderErr != nil {
			rows = append(rows, "", tui.StyleDanger.Render(s.graderErr.Error()))
		}
		rows = append(rows, "", tui.StylePrimary.Render(i18n.Tf(i18n.KeyStudyGradeLabel, s.graderScore)))
		if s.followUpEditor != nil {
			rows = append(rows, s.renderFollowUpInput())
		}
	}
	return rows
}

func (s *Study) renderFollowUpInput() string {
	inputW := max(20, s.w-2)
	if s.followUpStreaming {
		waiting := tui.StyleMuted.Render(s.spin.View() + " " + i18n.T(i18n.KeyStudyRethinking))
		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tui.ColorBorder).
			Width(inputW).
			Render(waiting)
	}
	raw := strings.TrimRight(s.followUpEditor.View(), "\n")
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(vimModeBorderColor(s.followUpEditor.GetMode())).
		Width(inputW).
		Render(raw)
}
