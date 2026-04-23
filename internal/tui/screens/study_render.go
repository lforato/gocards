package screens

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/tui"
)

// renderPrompt formats a card prompt through glamour for terminal markdown
// rendering (headings, code blocks, lists, emphasis). Width governs word
// wrap so long lines don't overflow the frame.
func renderPrompt(p string, width int) string {
	return renderMarkdown(p, width)
}

func codeBox(s string) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorBorder).
		Foreground(lipgloss.Color("#d1d5db")).
		Padding(0, 1).
		Render(s)
}

// extractCodeBlock returns the body of the first fenced block in prompt,
// used by CardExp to preload the vim editor.
func extractCodeBlock(prompt string) string {
	var buf []string
	in := false
	for _, ln := range strings.Split(prompt, "\n") {
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

// extractGrade parses Claude's FINAL_GRADE: N footer. (0, false) means the
// grader didn't commit — the UI surfaces that instead of recording a 0.
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
