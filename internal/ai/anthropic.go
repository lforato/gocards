package ai

import (
	"context"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/lforato/gocards/internal/models"
)

const Model = anthropic.ModelClaudeHaiku4_5_20251001

// Event is one piece of stream output.
type Event struct {
	Chunk string // incremental text
	Err   error
	Done  bool
	Full  string // final accumulated text, only set when Done
}

// ResolveAPIKey returns the key from env or from the DB-backed settings getter.
func ResolveAPIKey(settingKey func() (string, bool, error)) string {
	if k := os.Getenv("ANTHROPIC_API_KEY"); k != "" {
		return k
	}
	if settingKey != nil {
		if v, ok, _ := settingKey(); ok {
			return v
		}
	}
	return ""
}

// Client is a thin wrapper around the Anthropic SDK.
type Client struct {
	inner anthropic.Client
}

func New(apiKey string) *Client {
	c := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &Client{inner: c}
}

// Generate starts a flashcard-generation stream. The first call uses topic +
// history = []. Subsequent turns pass history (including the assistant's prior
// reply) to keep context.
func (c *Client) Generate(ctx context.Context, topic string, history []models.GradingMessage, preferredLanguages string) <-chan Event {
	system := generateSystem(preferredLanguages)
	var messages []anthropic.MessageParam
	if len(history) == 0 {
		messages = append(messages, anthropic.NewUserMessage(
			anthropic.NewTextBlock(fmt.Sprintf("Generate flashcards about: %s", topic)),
		))
	} else {
		for _, m := range history {
			messages = append(messages, msgParam(m))
		}
	}

	return c.stream(ctx, system, messages, 2000)
}

type GradeInput struct {
	Prompt         string
	ExpectedAnswer string
	UserAnswer     string
	History        []models.GradingMessage
	Mode           string // "code" | "explanation"
}

func (c *Client) Grade(ctx context.Context, in GradeInput) <-chan Event {
	system := gradeSystem(in)

	var messages []anthropic.MessageParam
	if len(in.History) == 0 {
		var initial string
		if in.Mode == "explanation" {
			initial = fmt.Sprintf("Student's annotated code (the block above with their comments added):\n\n```\n%s\n```", in.UserAnswer)
		} else {
			initial = fmt.Sprintf("Student's answer:\n```\n%s\n```", in.UserAnswer)
		}
		messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(initial)))
	} else {
		for _, m := range in.History {
			messages = append(messages, msgParam(m))
		}
	}

	return c.stream(ctx, system, messages, 500)
}

func msgParam(m models.GradingMessage) anthropic.MessageParam {
	if m.Role == "assistant" {
		return anthropic.NewAssistantMessage(anthropic.NewTextBlock(m.Content))
	}
	return anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content))
}

func (c *Client) stream(ctx context.Context, system string, messages []anthropic.MessageParam, maxTokens int64) <-chan Event {
	ch := make(chan Event, 16)

	go func() {
		defer close(ch)

		stream := c.inner.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
			Model:     Model,
			MaxTokens: maxTokens,
			System:    []anthropic.TextBlockParam{{Text: system}},
			Messages:  messages,
		})

		var full string
		for stream.Next() {
			event := stream.Current()
			switch ev := event.AsAny().(type) {
			case anthropic.ContentBlockDeltaEvent:
				if td, ok := ev.Delta.AsAny().(anthropic.TextDelta); ok && td.Text != "" {
					full += td.Text
					select {
					case ch <- Event{Chunk: td.Text}:
					case <-ctx.Done():
						return
					}
				}
			}
		}
		if err := stream.Err(); err != nil {
			ch <- Event{Err: err}
			return
		}
		ch <- Event{Done: true, Full: full}
	}()

	return ch
}

func generateSystem(preferredLanguages string) string {
	if preferredLanguages == "" {
		preferredLanguages = "javascript, typescript"
	}
	return fmt.Sprintf(`You are a flashcard generator for developers. Generate 3-5 flashcards based on the topic.

Preferred languages: %s

Return ONLY a JSON array of cards in this exact format, no markdown:
[
  {
    "type": "mcq" | "code" | "fill" | "exp",
    "language": "javascript",
    "prompt": "question text",
    "expected_answer": "correct answer or code or reference explanation",
    "choices": [{"id":"a","text":"...","isCorrect":false}, ...],  // only for mcq
    "blanks_data": {"template": "code with ___BLANK___", "blanks": ["value"]}  // only for fill
  }
]

Mix card types.

Card type semantics:
- "mcq": multiple-choice. Include 3-4 "choices" with exactly one isCorrect=true.
- "code": student writes code from scratch. expected_answer is a clean reference solution.
- "fill": student edits a code template directly, replacing each ___BLANK___ marker in place. blanks_data has the template (using ___BLANK___ tokens) and blanks[] with the canonical replacement values in order. Prefer blanks that are short identifiers, enum values, or single expressions.
- "exp": student ANNOTATES a code block with inline comments to explain what it does. The prompt MUST contain a natural-language question followed by a fenced code block of 4-15 lines. expected_answer is a reference prose explanation (3-6 sentences) used only by the grader.

CRITICAL rules for "prompt":
- The prompt is the ONLY text the student sees. They do not see expected_answer.
- If your question references code, include the full code literally INSIDE the prompt using a fenced code block.
- Newlines inside strings must be \n.

Rules for "expected_answer":
- For code cards, give a clean reference solution.
- For fill cards, a short human-readable description; the authoritative answer lives in blanks_data.blanks.
- For mcq cards, the correct choice text or a one-sentence explanation.
- Always include this field, even for fill cards.`, preferredLanguages)
}

func gradeSystem(in GradeInput) string {
	if in.Mode == "explanation" {
		return fmt.Sprintf(`You are a terse programming tutor grading a student's INLINE COMMENTS added to a code block.

Question (includes the original code block): %s
Reference explanation (your internal rubric — never reveal or quote verbatim): %s

Internally assess:
- Does each non-trivial line or block have a comment that correctly explains it?
- Are the comments accurate? Do they capture intent, mechanics, side effects, and any non-obvious choices?
- Do any comments just restate code verbatim or contain factual errors?

OUTPUT RULES — the student reads this directly:
- Do NOT write "the student did X" or any meta-assessment.
- Do NOT praise. No "Good job", no "solid understanding", no summaries of what was correct.
- Address the student in second person ("you").
- Only surface what was MISSING or WRONG. If a part was correct, say nothing about it.
- Use a short bulleted list. Reference specific lines or concepts. Max 4 bullets, each one sentence.
- If nothing was wrong and nothing important was missed, output a single line: "Nothing missing." then the grade lines.
- Do not include any preamble, heading, or closing sentence.
- You may ask ONE targeted follow-up question ONLY if what is missing is ambiguous; otherwise do not ask follow-ups.

On the LAST TWO LINES of your response, write these two lines in this EXACT format (no bold, no backticks, no extra punctuation):
FINAL_GRADE: N
VERDICT: V

where N is a digit 1-5 and V is EXACTLY one of these four strings: Wrong | Partially correct | Correct | Excellent

Grade-to-verdict mapping:
- 1 or 2 → Wrong
- 3 → Partially correct
- 4 → Correct
- 5 → Excellent`, in.Prompt, in.ExpectedAnswer)
	}
	return fmt.Sprintf(`You are a concise programming tutor grading a student's coding answer.

Question: %s
Expected approach: %s

Grade 1-5:
1 = Wrong
2 = Major issues
3 = Partially correct
4 = Correct
5 = Excellent

Be brief. You may ask one follow-up question if needed. When confident, end your response with exactly:
FINAL_GRADE: [1-5]
VERDICT: [Wrong|Partially correct|Correct|Excellent]`, in.Prompt, in.ExpectedAnswer)
}
