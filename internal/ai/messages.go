package ai

import (
	"github.com/anthropics/anthropic-sdk-go"

	"github.com/lforato/gocards/internal/models"
)

// toAnthropic converts a gocards GradingMessage history into the SDK's param
// shape. User and assistant turns are tagged accordingly; anything that isn't
// "assistant" is treated as a user turn.
func toAnthropic(history []models.GradingMessage) []anthropic.MessageParam {
	out := make([]anthropic.MessageParam, 0, len(history))
	for _, m := range history {
		out = append(out, msgParam(m))
	}
	return out
}

func msgParam(m models.GradingMessage) anthropic.MessageParam {
	if m.Role == "assistant" {
		return anthropic.NewAssistantMessage(anthropic.NewTextBlock(m.Content))
	}
	return anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content))
}
