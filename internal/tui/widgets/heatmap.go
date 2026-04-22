// Package widgets holds reusable UI components that aren't tied to a
// specific screen: the GitHub-style activity heatmap, the vim-backed
// modal code editor, and a textinput Form with focus cycling.
package widgets

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/tui"
)

// Heatmap is a GitHub-style activity grid — 7 rows × ceil(width/2) columns,
// rightmost column ending today. Cells are 2 display cells wide.
func Heatmap(activity map[string]int, width int) string {
	weeks := width / 2
	if weeks <= 0 {
		return ""
	}

	now := time.Now().Local()
	offset := int(now.Weekday()) // 0 = Sunday ... 6 = Saturday
	end := now.AddDate(0, 0, 6-offset)
	start := end.AddDate(0, 0, -(weeks*7 - 1))

	total := weeks * 7
	cells := make([]string, total)
	for i := range total {
		d := start.AddDate(0, 0, i)
		cells[i] = heatCell(activity[d.Format("2006-01-02")], !d.After(now))
	}

	var b strings.Builder
	for row := range 7 {
		if row > 0 {
			b.WriteString("\n")
		}
		for col := range weeks {
			b.WriteString(cells[col*7+row])
		}
	}
	return b.String()
}

func heatCell(count int, past bool) string {
	if !past {
		return lipgloss.NewStyle().Foreground(tui.ColorBorder).Render("· ")
	}
	switch {
	case count == 0:
		return heatFill("#1f2937")
	case count < 3:
		return heatFill("#78350f")
	case count < 6:
		return heatFill("#b45309")
	case count < 12:
		return heatFill("#d97706")
	default:
		return lipgloss.NewStyle().Foreground(tui.ColorPrimary).Render("■ ")
	}
}

func heatFill(hex string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render("■ ")
}
