package ai

import (
	"context"

	"github.com/lforato/gocards/internal/models"
)

// Chat runs a conversational card-authoring session with Claude. The caller
// owns the history and appends both the user's latest message and the model's
// reply after each turn. Claude emits cards inline as <card>...</card> JSON
// blocks; the TUI extractor parses them out of the final reply.
func (c *Client) Chat(ctx context.Context, deckName, deckDescription string, history []models.GradingMessage) <-chan Event {
	return c.stream(ctx, chatSystem(deckName, deckDescription), toAnthropic(history), maxTokensChat)
}
