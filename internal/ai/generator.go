package ai

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/lforato/gocards/internal/models"
)

// Generate replays history if present, else seeds the stream with topic as
// the first user turn.
func (c *Client) Generate(ctx context.Context, topic string, history []models.ChatMessage, preferredLanguages string) <-chan Event {
	system := generateSystem(preferredLanguages)
	messages := toAnthropicHistory(history)
	if len(history) == 0 {
		messages = append(messages, anthropic.NewUserMessage(
			anthropic.NewTextBlock(fmt.Sprintf("Generate flashcards about: %s", topic)),
		))
	}
	return c.stream(ctx, system, messages, maxTokensGenerate)
}
