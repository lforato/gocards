package ai

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/lforato/gocards/internal/models"
)

const maxTokensCheatsheet int64 = 6000

// Cheatsheet streams a markdown article synthesized from the deck's cards.
// Cards are expected pre-ordered hardest-first so sections render with the
// user's weakest topics at the top.
func (c *Client) Cheatsheet(ctx context.Context, deck models.Deck, cards []CheatsheetCard) <-chan Event {
	system := cheatsheetSystem()
	user := cheatsheetUser(deck.Name, deck.Description, cards)
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(user)),
	}
	return c.stream(ctx, system, messages, maxTokensCheatsheet)
}
