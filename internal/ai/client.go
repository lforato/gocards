package ai

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Model is the single Anthropic model the app uses for every call (generation,
// chat, grading). Haiku is chosen for low latency streaming in the TUI —
// switch to Sonnet for richer code understanding at higher cost/latency.
const Model = anthropic.ModelClaudeHaiku4_5_20251001

// Token budgets. Kept narrow so responses render in a terminal viewport
// without scrolling forever and so a runaway model can't consume free credits.
const (
	maxTokensGenerate int64 = 2000 // 3-5 cards per one-shot generation
	maxTokensChat     int64 = 4000 // conversational turns + card proposals
	maxTokensGrade    int64 = 500  // brief verdict + FINAL_GRADE line
)

// Event is one piece of a stream. A normal stream produces many {Chunk: text}
// events, then exactly one terminal event (either {Err: ...} or {Done: true,
// Full: <accumulated>}).
type Event struct {
	Chunk string
	Err   error
	Done  bool
	Full  string
}

// ResolveAPIKey returns the first key it finds, in order: ANTHROPIC_API_KEY
// env var, then the DB-backed settings getter. Empty string means "no key
// configured"; callers should surface a hint rather than sending the request.
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
	return &Client{inner: anthropic.NewClient(option.WithAPIKey(apiKey))}
}

// stream runs a single streaming Messages.Create request in its own goroutine
// and fans deltas onto a buffered channel. The goroutine always terminates
// (panic recovery + ctx check) and always closes the channel.
func (c *Client) stream(ctx context.Context, system string, messages []anthropic.MessageParam, maxTokens int64) <-chan Event {
	ch := make(chan Event, 16)

	go func() {
		defer close(ch)
		defer func() {
			if r := recover(); r != nil {
				select {
				case ch <- Event{Err: fmt.Errorf("stream panic: %v", r)}:
				default:
				}
			}
		}()

		stream := c.inner.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
			Model:     Model,
			MaxTokens: maxTokens,
			System:    []anthropic.TextBlockParam{{Text: system}},
			Messages:  messages,
		})

		var full strings.Builder
		for stream.Next() {
			event := stream.Current()
			if ev, ok := event.AsAny().(anthropic.ContentBlockDeltaEvent); ok {
				if td, ok := ev.Delta.AsAny().(anthropic.TextDelta); ok && td.Text != "" {
					full.WriteString(td.Text)
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
		ch <- Event{Done: true, Full: full.String()}
	}()

	return ch
}
