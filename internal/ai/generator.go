package ai

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/lforato/gocards/internal/models"
)

// Generate starts a one-shot flashcard generation stream. When history is
// empty we seed with the topic as the first user message; when it's non-empty
// we replay the prior turns (so the caller can keep asking for more/different
// cards with continued context).
func (c *Client) Generate(ctx context.Context, topic string, history []models.GradingMessage, preferredLanguages string) <-chan Event {
	system := generateSystem(preferredLanguages)
	messages := toAnthropic(history)
	if len(history) == 0 {
		messages = append(messages, anthropic.NewUserMessage(
			anthropic.NewTextBlock(fmt.Sprintf("Generate flashcards about: %s", topic)),
		))
	}
	return c.stream(ctx, system, messages, maxTokensGenerate)
}
