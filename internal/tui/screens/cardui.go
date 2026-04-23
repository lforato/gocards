package screens

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/i18n"
	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/tui"
)

// cardUI is the screen-side companion to models.CardKind: theme color,
// help-row builders, editor field list. QuestionFn/AnsweredFn are funcs so
// the returned strings can be i18n-resolved at render time (the current
// language may change without a restart).
type cardUI struct {
	Color      lipgloss.Color
	QuestionFn func() []string
	AnsweredFn func() []string
	EditFields []editField
}

// questionHelpMCQ etc. are funcs so translation lookups happen at call
// time — the current language may change without a restart.
func questionHelpMCQ() []string {
	return []string{
		i18n.Help("↑/↓", i18n.KeyHelpPick),
		i18n.Help("enter", i18n.KeyHelpSubmit),
		i18n.Help("esc", i18n.KeyHelpEnd),
	}
}
func answeredHelpMCQ() []string {
	return []string{i18n.Help("esc", i18n.KeyHelpEnd)}
}
func questionHelpCode() []string {
	return []string{
		i18n.Help("i", i18n.KeyHelpInsert),
		i18n.Help("esc", i18n.KeyHelpNormal),
		i18n.Help("ctrl+s", i18n.KeyHelpSubmit),
		i18n.Help("esc", i18n.KeyHelpEscEnd),
	}
}
func answeredHelpCode() []string {
	return []string{
		i18n.Help("ctrl+s", i18n.KeyHelpSend),
		i18n.Help("shift+↑/↓", i18n.KeyHelpScroll),
		i18n.Help("esc", i18n.KeyHelpEscEnd),
	}
}
func questionHelpFill() []string {
	return []string{
		i18n.Help("tab", i18n.KeyHelpCycle),
		i18n.Help("enter", i18n.KeyHelpSubmit),
		i18n.Help("esc", i18n.KeyHelpEnd),
	}
}
func answeredHelpFill() []string {
	return []string{i18n.Help("esc", i18n.KeyHelpEnd)}
}

var cardUIs = map[models.CardType]cardUI{
	models.CardMCQ: {
		Color:       tui.ColorAccent,
		QuestionFn:  questionHelpMCQ,
		AnsweredFn:  answeredHelpMCQ,
		EditFields:  []editField{fType, fLanguage, fPrompt, fChoices},
	},
	models.CardCode: {
		Color:       tui.ColorSuccess,
		QuestionFn:  questionHelpCode,
		AnsweredFn:  answeredHelpCode,
		EditFields:  []editField{fLanguage, fPrompt, fInitialCode, fExpected},
	},
	models.CardFill: {
		Color:       tui.ColorWarn,
		QuestionFn:  questionHelpFill,
		AnsweredFn:  answeredHelpFill,
		EditFields:  []editField{fType, fLanguage, fPrompt, fTemplate},
	},
	models.CardExp: {
		Color:       tui.ColorPrimary,
		QuestionFn:  questionHelpCode,
		AnsweredFn:  answeredHelpCode,
		EditFields:  []editField{fType, fLanguage, fPrompt, fInitialCode, fExpected},
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
