package ai

import (
	"context"

	"github.com/lforato/gocards/internal/models"
)

// Chat drives a conversational authoring session. Claude emits proposed
// cards inline as <card>...</card> JSON blocks for the caller to extract.
func (c *Client) Chat(ctx context.Context, deckName, deckDescription string, history []models.ChatMessage) <-chan Event {
	return c.stream(ctx, chatSystem(deckName, deckDescription), toAnthropicHistory(history), maxTokensChat)
}
