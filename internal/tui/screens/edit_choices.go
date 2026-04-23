package screens

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/i18n"
	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/tui"
)

const maxMCQChoices = 26

func (e *Edit) updateChoicesKey(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	switch m.String() {
	case "tab":
		e.cycleFocus(1)
	case "shift+tab":
		e.cycleFocus(-1)
	case "up", "k":
		e.choiceCursor = cursorUp(e.choiceCursor)
	case "down", "j":
		e.choiceCursor = cursorDown(e.choiceCursor, len(e.card.Choices))
	case " ", "space":
		e.toggleChoiceCorrect()
	case "a":
		return e.addChoice()
	case "d":
		e.deleteChoice()
	case "e", "enter":
		if len(e.card.Choices) > 0 {
			e.beginChoiceEdit(e.choiceCursor)
			return e, textinput.Blink
		}
	}
	return e, nil
}

func (e *Edit) toggleChoiceCorrect() {
	if len(e.card.Choices) == 0 {
		return
	}
	e.card.Choices[e.choiceCursor].IsCorrect = !e.card.Choices[e.choiceCursor].IsCorrect
}

func (e *Edit) addChoice() (tui.Screen, tea.Cmd) {
	if len(e.card.Choices) >= maxMCQChoices {
		return e, tui.ToastErr(i18n.Tf(i18n.KeyEditMaxChoices, maxMCQChoices))
	}
	e.card.Choices = append(e.card.Choices, models.Choice{})
	e.choiceCursor = len(e.card.Choices) - 1
	e.beginChoiceEdit(e.choiceCursor)
	return e, textinput.Blink
}

func (e *Edit) deleteChoice() {
	if len(e.card.Choices) == 0 {
		return
	}
	e.card.Choices = append(e.card.Choices[:e.choiceCursor], e.card.Choices[e.choiceCursor+1:]...)
	e.choiceCursor = max(0, min(e.choiceCursor, len(e.card.Choices)-1))
}

func (e *Edit) beginChoiceEdit(idx int) {
	e.choiceEditing = true
	e.choiceEditIdx = idx
	e.choiceInput.SetValue(e.card.Choices[idx].Text)
	e.choiceInput.CursorEnd()
	e.choiceInput.Focus()
}

func (e *Edit) updateChoiceEdit(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	switch m.String() {
	case "esc":
		e.stopChoiceEdit()
		return e, nil
	case "enter":
		e.commitChoiceEdit()
		return e, nil
	}
	var cmd tea.Cmd
	e.choiceInput, cmd = e.choiceInput.Update(m)
	return e, cmd
}

func (e *Edit) commitChoiceEdit() {
	if e.choiceEditIdx >= 0 && e.choiceEditIdx < len(e.card.Choices) {
		e.card.Choices[e.choiceEditIdx].Text = e.choiceInput.Value()
	}
	e.stopChoiceEdit()
}

func (e *Edit) stopChoiceEdit() {
	e.choiceEditing = false
	e.choiceInput.Blur()
}

func (e *Edit) viewChoices() string {
	if len(e.card.Choices) == 0 && !e.choiceEditing {
		return tui.StyleMuted.Render(i18n.T(i18n.KeyEditNoChoicesHint))
	}
	var lines []string
	for i, ch := range e.card.Choices {
		lines = append(lines, e.renderChoiceRow(i, ch))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (e *Edit) renderChoiceRow(i int, ch models.Choice) string {
	mark := "[ ]"
	if ch.IsCorrect {
		mark = tui.StyleSuccess.Render("[x]")
	}
	text := ch.Text
	if e.choiceEditing && e.choiceEditIdx == i {
		text = e.choiceInput.View()
	} else if text == "" {
		text = tui.StyleMuted.Render(i18n.T(i18n.KeyEditEmptyChoice))
	}
	selected := i == e.choiceCursor && e.focus == fChoices
	return fmt.Sprintf("%s%s %s. %s", selectionPrefix(selected), mark, string(rune('a'+i)), text)
}
