// Package ai wraps the Anthropic SDK with the stream-based operations
// gocards needs: card generation, conversational authoring, answer
// grading, and cheatsheet synthesis. Prompt construction and panic-safe
// goroutine streaming live here so the TUI screens can consume plain
// channels of Event without knowing about the Anthropic SDK.
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

// streamBufferSize is large enough that normal token-rate streaming never
// blocks on the receiver, but small enough that a stuck consumer doesn't
// balloon memory. A goroutine writes chunks here; the caller reads.
const streamBufferSize = 16

// stream runs one Anthropic call in a goroutine and returns the Events as a
// channel. Always emits exactly one terminal event (Err or Done) and then
// closes the channel, so callers can range until close safely.
func (c *Client) stream(ctx context.Context, system string, messages []anthropic.MessageParam, maxTokens int64) <-chan Event {
	events := make(chan Event, streamBufferSize)

	go func() {
		defer close(events)
		defer reportPanicAsEvent(events)

		response := c.inner.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
			Model:     Model,
			MaxTokens: maxTokens,
			System:    []anthropic.TextBlockParam{{Text: system}},
			Messages:  messages,
		})

		var full strings.Builder
		for response.Next() {
			chunk, ok := extractTextDelta(response.Current())
			if !ok || chunk == "" {
				continue
			}
			full.WriteString(chunk)
			select {
			case events <- Event{Chunk: chunk}:
			case <-ctx.Done():
				return
			}
		}
		if err := response.Err(); err != nil {
			events <- Event{Err: err}
			return
		}
		events <- Event{Done: true, Full: full.String()}
	}()

	return events
}

// extractTextDelta returns the new text from a content-block-delta event, or
// ("", false) for any other event type (tool calls, stop events, etc.).
func extractTextDelta(raw anthropic.MessageStreamEventUnion) (string, bool) {
	deltaEvent, ok := raw.AsAny().(anthropic.ContentBlockDeltaEvent)
	if !ok {
		return "", false
	}
	textDelta, ok := deltaEvent.Delta.AsAny().(anthropic.TextDelta)
	if !ok {
		return "", false
	}
	return textDelta.Text, true
}

// reportPanicAsEvent is deferred inside the streaming goroutine so an
// unexpected panic surfaces as an Event.Err instead of killing the process.
// The non-blocking select prevents a deadlock if the caller has already
// stopped reading.
func reportPanicAsEvent(events chan<- Event) {
	r := recover()
	if r == nil {
		return
	}
	select {
	case events <- Event{Err: fmt.Errorf("stream panic: %v", r)}:
	default:
	}
}
