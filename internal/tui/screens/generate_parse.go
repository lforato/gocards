package screens

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/store"
)

type aiCardDTO struct {
	Type           string            `json:"type"`
	Language       string            `json:"language"`
	Prompt         string            `json:"prompt"`
	ExpectedAnswer string            `json:"expected_answer"`
	InitialCode    string            `json:"initial_code"`
	Choices        []models.Choice   `json:"choices"`
	BlanksData     *models.BlankData `json:"blanks_data"`
}

var cardTagRe = regexp.MustCompile(`(?s)<card>(.*?)</card>`)

// extractProposedCards scans the assistant reply for <card>…</card> JSON
// blocks. Malformed blocks are skipped — the user can ask Claude to
// regenerate rather than see a parse error.
func extractProposedCards(reply string) []store.CardInput {
	var out []store.CardInput
	for _, m := range cardTagRe.FindAllStringSubmatch(reply, -1) {
		if card, ok := parseCardBlock(m[1]); ok {
			out = append(out, card)
		}
	}
	return out
}

func parseCardBlock(raw string) (store.CardInput, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return store.CardInput{}, false
	}
	var dto aiCardDTO
	if err := json.Unmarshal([]byte(raw), &dto); err != nil {
		return store.CardInput{}, false
	}
	ct := models.CardType(strings.ToLower(strings.TrimSpace(dto.Type)))
	if !models.IsKnownCardType(ct) {
		return store.CardInput{}, false
	}

	in := store.CardInput{
		Type:           ct,
		Language:       defaultLanguage(dto.Language),
		Prompt:         dto.Prompt,
		InitialCode:    dto.InitialCode,
		ExpectedAnswer: dto.ExpectedAnswer,
		Choices:        dto.Choices,
		BlanksData:     dto.BlanksData,
	}
	if in.Type == models.CardMCQ {
		assignMissingChoiceIDs(in.Choices)
	}
	return in, true
}

func defaultLanguage(lang string) string {
	if strings.TrimSpace(lang) == "" {
		return "javascript"
	}
	return lang
}

// assignMissingChoiceIDs fills in a/b/c/… for choices Claude returned
// without an ID, so the study screen can label them consistently.
func assignMissingChoiceIDs(choices []models.Choice) {
	for i := range choices {
		if strings.TrimSpace(choices[i].ID) == "" {
			choices[i].ID = string(rune('a' + i))
		}
	}
}
