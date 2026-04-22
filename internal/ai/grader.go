package ai

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/lforato/gocards/internal/models"
)

// GradeInput bundles everything the grader needs to score a student's answer
// for a code or exp card. Mode selects the grading rubric.
type GradeInput struct {
	Prompt         string
	ExpectedAnswer string
	UserAnswer     string
	History        []models.GradingMessage
	Mode           string // "code" | "explanation"
}

// Grade streams the grader's verdict. The final message always ends with:
//
//	FINAL_GRADE: N
//	VERDICT: <label>
//
// The caller extracts N to persist a review. Missing FINAL_GRADE means the
// grader didn't commit — the TUI falls back to manual grading.
func (c *Client) Grade(ctx context.Context, in GradeInput) <-chan Event {
	messages := toAnthropic(in.History)
	if len(in.History) == 0 {
		messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(firstGraderTurn(in))))
	}
	return c.stream(ctx, gradeSystem(in), messages, maxTokensGrade)
}

func firstGraderTurn(in GradeInput) string {
	if in.Mode == "explanation" {
		return fmt.Sprintf("Student's annotated code (the block above with their comments added):\n\n```\n%s\n```", in.UserAnswer)
	}
	return fmt.Sprintf("Student's answer:\n```\n%s\n```", in.UserAnswer)
}
