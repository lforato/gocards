package screens

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/tui"
)

// renderPrompt formats a card prompt for display: plain text stays plain,
// fenced ```code``` blocks render inside a bordered codeBox.
func renderPrompt(p string) string {
	lines := strings.Split(p, "\n")
	var out []string
	var inCode bool
	var buf []string
	for _, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "```") {
			if inCode {
				out = append(out, codeBox(strings.Join(buf, "\n")))
				buf = nil
				inCode = false
			} else {
				inCode = true
			}
			continue
		}
		if inCode {
			buf = append(buf, ln)
			continue
		}
		out = append(out, ln)
	}
	if len(buf) > 0 {
		out = append(out, codeBox(strings.Join(buf, "\n")))
	}
	return strings.Join(out, "\n")
}

func codeBox(s string) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorBorder).
		Foreground(lipgloss.Color("#d1d5db")).
		Padding(0, 1).
		Render(s)
}

// extractCodeBlock returns the content of the first fenced code block in
// prompt (without the fences). Used by CardExp to preload the vim editor.
func extractCodeBlock(prompt string) string {
	lines := strings.Split(prompt, "\n")
	var buf []string
	in := false
	for _, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "```") {
			if in {
				return strings.Join(buf, "\n")
			}
			in = true
			continue
		}
		if in {
			buf = append(buf, ln)
		}
	}
	return strings.Join(buf, "\n")
}

var gradeRegex = regexp.MustCompile(`FINAL_GRADE:\s*([1-5])`)

// extractGrade parses the FINAL_GRADE: N line Claude emits at the end of a
// grade stream. Returns (0, false) when no such line is present so the caller
// can surface the missing-grade case instead of silently persisting a 0.
func extractGrade(text string) (int, bool) {
	m := gradeRegex.FindStringSubmatch(text)
	if len(m) < 2 {
		return 0, false
	}
	g, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return g, true
}
