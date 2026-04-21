package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Heatmap renders a GitHub-style activity grid: 7 rows (weekdays) × N columns
// (weeks), where N is as many weeks as fit in `width` display cells. Each cell
// is 2 display cells wide ("■ "). The rightmost column is the current week.
func Heatmap(activity map[string]int, width int) string {
	weeks := width / 2
	if weeks <= 0 {
		return ""
	}

	now := time.Now().Local()
	// Align the rightmost column so it contains today: jump forward to Saturday.
	offset := int(now.Weekday()) // 0 = Sunday ... 6 = Saturday
	end := now.AddDate(0, 0, 6-offset)
	start := end.AddDate(0, 0, -(weeks*7 - 1))

	total := weeks * 7
	cells := make([]string, total)
	for i := range total {
		d := start.AddDate(0, 0, i)
		key := d.Format("2006-01-02")
		cells[i] = cell(activity[key], !d.After(now))
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

func cell(count int, past bool) string {
	if !past {
		return lipgloss.NewStyle().Foreground(ColorBorder).Render("· ")
	}
	switch {
	case count == 0:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#1f2937")).Render("■ ")
	case count < 3:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#78350f")).Render("■ ")
	case count < 6:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#b45309")).Render("■ ")
	case count < 12:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#d97706")).Render("■ ")
	default:
		return lipgloss.NewStyle().Foreground(ColorPrimary).Render("■ ")
	}
}
