package screens

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/i18n"
	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/tui"
)

func (s *Study) handleMCQKey(m tea.KeyMsg, card *models.Card) (tui.Screen, tea.Cmd) {
	if len(card.Choices) == 0 {
		return s, tui.ToastErr(i18n.T(i18n.KeyStudyMCQNoChoices))
	}
	if s.mcqCursor >= len(card.Choices) {
		s.mcqCursor = 0
	}
	switch m.String() {
	case "up", "k":
		s.mcqCursor = cursorUp(s.mcqCursor)
	case "down", "j":
		s.mcqCursor = cursorDown(s.mcqCursor, len(card.Choices))
	case "enter", " ":
		chosen := card.Choices[s.mcqCursor]
		grade := 5
		note := i18n.T(i18n.KeyStudyMCQCorrect)
		if !chosen.IsCorrect {
			grade = 1
			note = i18n.T(i18n.KeyStudyMCQIncorrect)
		}
		s.resultGrade = grade
		s.resultNote = note
		s.stage = stageAnswered
		return s, s.recordReview(grade)
	}
	return s, nil
}

func (s *Study) viewMCQ(card *models.Card) string {
	rows := []string{renderPrompt(card.Prompt, s.w), ""}
	for i, ch := range card.Choices {
		rows = append(rows, mcqChoiceRow(ch, i, s.mcqCursor, s.stage))
	}
	if s.stage == stageAnswered {
		rows = append(rows, "", tui.StylePrimary.Render(i18n.Tf(i18n.KeyStudyMCQResult, s.resultNote, s.resultGrade)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func mcqChoiceRow(ch models.Choice, idx, cursor int, stage studyStage) string {
	selected := idx == cursor
	prefix := "  "
	if selected && stage == stageQuestion {
		prefix = tui.StylePrimary.Render("▶ ")
	}

	label := fmt.Sprintf("%s. %s", ch.ID, ch.Text)
	switch {
	case stage == stageAnswered && ch.IsCorrect:
		label = tui.StyleSuccess.Render(label + "  ✓")
	case stage == stageAnswered && selected && !ch.IsCorrect:
		label = tui.StyleDanger.Render(label + "  ✗")
	case stage == stageAnswered:
		label = tui.StyleMuted.Render(label)
	case selected:
		label = tui.StyleSelected.Render(label)
	}
	return prefix + label
}
