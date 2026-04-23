package screens

import (
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
)

const markdownFallbackWidth = 80

var (
	mdRendererCache = map[int]*glamour.TermRenderer{}
	mdRendererMu    sync.Mutex
)

// renderMarkdown returns the markdown input rendered for terminal display at
// the requested width. Falls back to the raw text if glamour can't build a
// renderer (e.g. style load failure).
func renderMarkdown(md string, width int) string {
	if strings.TrimSpace(md) == "" {
		return ""
	}
	if width <= 0 {
		width = markdownFallbackWidth
	}
	r := markdownRenderer(width)
	if r == nil {
		return md
	}
	out, err := r.Render(md)
	if err != nil {
		return md
	}
	return strings.TrimRight(out, "\n")
}

// markdownRenderer returns a cached TermRenderer keyed by wrap width, since
// construction is non-trivial and we render on every View().
func markdownRenderer(width int) *glamour.TermRenderer {
	mdRendererMu.Lock()
	defer mdRendererMu.Unlock()
	if r, ok := mdRendererCache[width]; ok {
		return r
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil
	}
	mdRendererCache[width] = r
	return r
}
