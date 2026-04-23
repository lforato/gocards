package ai

import (
	"github.com/anthropics/anthropic-sdk-go"

	"github.com/lforato/gocards/internal/models"
)

// toAnthropicHistory maps our chat history into the Anthropic SDK's
// MessageParam shape so we can replay a multi-turn conversation.
func toAnthropicHistory(history []models.ChatMessage) []anthropic.MessageParam {
	out := make([]anthropic.MessageParam, 0, len(history))
	for _, m := range history {
		out = append(out, toAnthropicMessage(m))
	}
	return out
}

func toAnthropicMessage(m models.ChatMessage) anthropic.MessageParam {
	block := anthropic.NewTextBlock(m.Content)
	if m.Role == models.RoleAssistant {
		return anthropic.NewAssistantMessage(block)
	}
	return anthropic.NewUserMessage(block)
}
