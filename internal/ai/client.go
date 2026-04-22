// Package ai wraps the Anthropic SDK with the three stream-based
// operations gocards needs: one-shot card generation, conversational
// authoring, and answer grading. Prompt construction and panic-safe
// goroutine streaming live here so the TUI screens can consume plain
// channels of Event.
package ai

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Haiku streams fast enough for TUI latency; swap for Sonnet if code
// understanding matters more than per-token cost.
const Model = anthropic.ModelClaudeHaiku4_5_20251001

const (
	maxTokensGenerate int64 = 2000
	maxTokensChat     int64 = 4000
	maxTokensGrade    int64 = 500
)

// Event carries one delta from a streaming call. Streams emit many
// {Chunk: ...} events, then exactly one terminal {Err} or {Done, Full}.
type Event struct {
	Chunk string
	Err   error
	Done  bool
	Full  string
}

// ResolveAPIKey checks ANTHROPIC_API_KEY first, then falls back to the
// DB-backed settings getter. Empty return means no key is configured.
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

type Client struct {
	inner anthropic.Client
}

func New(apiKey string) *Client {
	return &Client{inner: anthropic.NewClient(option.WithAPIKey(apiKey))}
}

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
