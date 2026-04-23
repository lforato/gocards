package ai

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/lforato/gocards/internal/models"
)

type GradeInput struct {
	Prompt         string
	ExpectedAnswer string
	UserAnswer     string
	History        []models.GradingMessage
	Mode           string // "code" | "explanation"
}

// Grade streams a verdict whose last line is `FINAL_GRADE: N`. A missing
// FINAL_GRADE means the grader didn't commit — callers fall back to manual.
func (c *Client) Grade(ctx context.Context, in GradeInput) <-chan Event {
	messages := toAnthropic(in.History)
	if len(in.History) == 0 {
		messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(FirstGraderTurn(in))))
	}
	return c.stream(ctx, gradeSystem(in), messages, maxTokensGrade)
}

// FirstGraderTurn renders the initial user turn the grader sees. Exported so
// the study screen can seed its in-memory conversation history with the same
// wording after the first streaming call.
func FirstGraderTurn(in GradeInput) string {
	if in.Mode == "explanation" {
		return fmt.Sprintf("Student's annotated code (the block above with their comments added):\n\n```\n%s\n```", in.UserAnswer)
	}
	return fmt.Sprintf("Student's answer:\n```\n%s\n```", in.UserAnswer)
}
