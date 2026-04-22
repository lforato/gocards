package screens

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/tui"
)

// studyBehavior is the per-card-type dispatch table the Study screen uses
// for question-stage keypresses and body rendering. To add a new card type,
// register a behavior here (in addition to its models.CardKind and cardUI).
type studyBehavior struct {
	HandleKey func(s *Study, m tea.KeyMsg, card *models.Card) (tui.Screen, tea.Cmd)
	Render    func(s *Study, card *models.Card) string
}

var studyBehaviors = map[models.CardType]studyBehavior{
	models.CardMCQ: {
		HandleKey: (*Study).handleMCQKey,
		Render:    (*Study).viewMCQ,
	},
	models.CardFill: {
		HandleKey: (*Study).handleFillKey,
		Render:    (*Study).viewFill,
	},
	models.CardCode: {
		Render: (*Study).viewCode,
	},
	models.CardExp: {
		Render: (*Study).viewExp,
	},
}
