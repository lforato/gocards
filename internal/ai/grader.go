package ai

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/lforato/gocards/internal/models"
)

// GradeMode selects which rubric the grader uses: "code" scores a full
// solution, "explanation" scores inline annotations on a given block.
type GradeMode string

const (
	GradeModeCode        GradeMode = "code"
	GradeModeExplanation GradeMode = "explanation"
)

type GradeInput struct {
	Prompt         string
	ExpectedAnswer string
	UserAnswer     string
	History        []models.ChatMessage
	Mode           GradeMode
}

// Grade streams a verdict whose last line is `FINAL_GRADE: N`. A missing
// FINAL_GRADE means the grader didn't commit — callers fall back to manual.
func (c *Client) Grade(ctx context.Context, in GradeInput) <-chan Event {
	messages := toAnthropicHistory(in.History)
	if len(in.History) == 0 {
		messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(FirstGraderTurn(in))))
	}
	return c.stream(ctx, gradeSystem(in), messages, maxTokensGrade)
}

// FirstGraderTurn renders the initial user turn the grader sees. Exported so
// the study screen can seed its in-memory conversation history with the same
// wording after the first streaming call.
func FirstGraderTurn(in GradeInput) string {
	if in.Mode == GradeModeExplanation {
		return fmt.Sprintf("Student's annotated code (the block above with their comments added):\n\n```\n%s\n```", in.UserAnswer)
	}
	return fmt.Sprintf("Student's answer:\n```\n%s\n```", in.UserAnswer)
}
