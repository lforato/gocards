package screens

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/tui"
)

// cardUI is the screen-side companion to models.CardKind: theme color,
// question-stage help keys, answered-stage help keys. Adding a new card
// type means adding one entry here on top of models.cardKinds.
type cardUI struct {
	Color        lipgloss.Color
	QuestionHelp []string
	AnsweredHelp []string
	EditFields   []editField
}

var cardUIs = map[models.CardType]cardUI{
	models.CardMCQ: {
		Color:        tui.ColorAccent,
		QuestionHelp: []string{"↑/↓ pick", "enter submit", "esc end"},
		AnsweredHelp: []string{"enter next", "esc end"},
		EditFields:   []editField{fType, fLanguage, fPrompt, fChoices},
	},
	models.CardCode: {
		Color:        tui.ColorSuccess,
		QuestionHelp: []string{"i insert", "esc normal", "ctrl+s submit", "esc end (from normal)"},
		AnsweredHelp: []string{"1-5 override", "enter next", "esc end"},
		EditFields:   []editField{fLanguage, fPrompt, fInitialCode, fExpected},
	},
	models.CardFill: {
		Color:        tui.ColorWarn,
		QuestionHelp: []string{"tab switch", "enter submit", "esc end"},
		AnsweredHelp: []string{"enter next", "esc end"},
		EditFields:   []editField{fType, fLanguage, fPrompt, fTemplate},
	},
	models.CardExp: {
		Color:        tui.ColorPrimary,
		QuestionHelp: []string{"i insert", "esc normal", "ctrl+s submit", "esc end (from normal)"},
		AnsweredHelp: []string{"1-5 override", "enter next", "esc end"},
		EditFields:   []editField{fType, fLanguage, fPrompt, fExpected},
	},
}

func ui(t models.CardType) cardUI {
	if u, ok := cardUIs[t]; ok {
		return u
	}
	return cardUI{Color: tui.ColorMuted}
}

func cardTypeColor(t models.CardType) lipgloss.Color { return ui(t).Color }

func cardTypeBadge(t models.CardType) string {
	return lipgloss.NewStyle().Foreground(cardTypeColor(t)).Render(fmt.Sprintf("[%s]", t))
}

// typeBadge is the emphasized variant used in previews and the type picker.
func typeBadge(t models.CardType, bold bool) string {
	return lipgloss.NewStyle().
		Foreground(cardTypeColor(t)).
		Bold(bold).
		Render(string(t))
}
