package screens

import (
	"errors"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/store"
	"github.com/lforato/gocards/internal/tui"
)

func (e *Edit) save() tea.Cmd {
	in, err := e.buildCardInput()
	if err != nil {
		return tui.ToastErr(err.Error())
	}
	return e.persist(in)
}

func (e *Edit) buildCardInput() (store.CardInput, error) {
	if strings.TrimSpace(e.card.Prompt) == "" {
		return store.CardInput{}, errors.New("question required")
	}
	in := store.CardInput{
		Type:        e.card.Type,
		Language:    strings.TrimSpace(e.card.Language),
		Prompt:      e.card.Prompt,
		InitialCode: e.card.InitialCode,
	}
	return populateTypeSpecificInput(in, e.card)
}

// populateTypeSpecificInput validates and fills the card-type-specific
// fields of in. Each card type has a tiny helper in the same style, so
// adding a new type is one more case and one more function.
func populateTypeSpecificInput(in store.CardInput, card models.Card) (store.CardInput, error) {
	switch card.Type {
	case models.CardCode, models.CardExp:
		return withExpectedAnswer(in, card)
	case models.CardMCQ:
		return withMCQChoices(in, card)
	case models.CardFill:
		return withFillTemplate(in, card)
	}
	return in, nil
}

func withExpectedAnswer(in store.CardInput, card models.Card) (store.CardInput, error) {
	if strings.TrimSpace(card.ExpectedAnswer) == "" {
		return in, errors.New("expected answer required")
	}
	in.ExpectedAnswer = card.ExpectedAnswer
	return in, nil
}

func withMCQChoices(in store.CardInput, card models.Card) (store.CardInput, error) {
	if len(card.Choices) < 2 {
		return in, errors.New("add at least 2 choices")
	}
	if !anyCorrectChoice(card.Choices) {
		return in, errors.New("mark at least one choice correct")
	}
	in.Choices = normalizeChoiceIDs(card.Choices)
	return in, nil
}

func anyCorrectChoice(choices []models.Choice) bool {
	for _, ch := range choices {
		if ch.IsCorrect {
			return true
		}
	}
	return false
}

func normalizeChoiceIDs(choices []models.Choice) []models.Choice {
	out := make([]models.Choice, len(choices))
	for i, ch := range choices {
		out[i] = models.Choice{
			ID:        string(rune('a' + i)),
			Text:      ch.Text,
			IsCorrect: ch.IsCorrect,
		}
	}
	return out
}

func withFillTemplate(in store.CardInput, card models.Card) (store.CardInput, error) {
	if card.BlanksData == nil || strings.TrimSpace(card.BlanksData.Template) == "" {
		return in, errors.New("template required")
	}
	blanks := extractBlanks(card.BlanksData.Template)
	if len(blanks) == 0 {
		return in, errors.New("template needs at least one {{blank}}")
	}
	in.BlanksData = &models.BlankData{Template: card.BlanksData.Template, Blanks: blanks}
	return in, nil
}

func extractBlanks(template string) []string {
	matches := blankRe.FindAllStringSubmatch(template, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

func (e *Edit) persist(in store.CardInput) tea.Cmd {
	if e.card.ID == 0 {
		return e.create(in)
	}
	return e.update(in)
}

func (e *Edit) create(in store.CardInput) tea.Cmd {
	cs, err := e.store.BulkCreateCards(e.card.DeckID, []store.CardInput{in})
	if err != nil {
		return tui.ToastErr("save failed: " + err.Error())
	}
	if len(cs) > 0 {
		e.card = cs[0]
	}
	return tea.Batch(tui.Toast("card created"), navBack)
}

func (e *Edit) update(in store.CardInput) tea.Cmd {
	if _, err := e.store.UpdateCard(e.card.ID, in); err != nil {
		return tui.ToastErr("update failed: " + err.Error())
	}
	return tea.Batch(tui.Toast("card saved"), navBack)
}
