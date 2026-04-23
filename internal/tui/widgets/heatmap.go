// Package widgets holds reusable UI components that aren't tied to a
// specific screen: the GitHub-style activity heatmap, the vim-backed
// modal code editor, and form widgets with focus cycling.
package widgets

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/tui"
)

const (
	daysPerWeek     = 7
	cellCharsPerDay = 2 // "■ " or "· " — two display cells wide so the grid stays square
	heatCellGlyph   = "■ "
	emptyGlyph      = "· "
	dayFormat       = "2006-01-02"
)

// heatPalette maps review-count thresholds to hex colors, darkest → brightest.
// A day's count is matched against the first (upper-exclusive) threshold it
// doesn't exceed; counts above the last threshold render in primary accent.
var heatPalette = []struct {
	upperExclusive int    // count strictly below this bucket
	hex            string // foreground for the cell's ■
}{
	{upperExclusive: 1, hex: "#1f2937"},  // zero reviews: dim slate
	{upperExclusive: 3, hex: "#78350f"},  // 1–2 reviews: deep amber
	{upperExclusive: 6, hex: "#b45309"},  // 3–5 reviews: amber
	{upperExclusive: 12, hex: "#d97706"}, // 6–11 reviews: bright amber
}

// Heatmap renders a GitHub-style activity grid: 7 rows × ceil(width/2)
// columns, rightmost column ending today. Cells are 2 display cells wide.
// Days beyond today show a muted dot placeholder.
func Heatmap(activity map[string]int, width int) string {
	weeks := width / cellCharsPerDay
	if weeks <= 0 {
		return ""
	}

	firstDay, lastDay := weekAlignedRange(weeks)
	now := time.Now().Local()

	cells := make([]string, weeks*daysPerWeek)
	for i := range cells {
		day := firstDay.AddDate(0, 0, i)
		isPast := !day.After(now)
		cells[i] = renderHeatCell(activity[day.Format(dayFormat)], isPast)
	}
	_ = lastDay // documenting the range; lastDay is implicit in `cells` length

	return joinGridRows(cells, weeks)
}

// weekAlignedRange returns the first and last day of a `weeks`-wide grid
// whose rightmost column ends this week's Saturday.
func weekAlignedRange(weeks int) (firstDay, lastDay time.Time) {
	now := time.Now().Local()
	daysUntilSaturday := int(time.Saturday) - int(now.Weekday())
	lastDay = now.AddDate(0, 0, daysUntilSaturday)
	firstDay = lastDay.AddDate(0, 0, -(weeks*daysPerWeek - 1))
	return firstDay, lastDay
}

// joinGridRows pivots a day-indexed slice (column-major) into display rows,
// so row N prints each week's Nth weekday horizontally.
func joinGridRows(cells []string, weeks int) string {
	var b strings.Builder
	for row := range daysPerWeek {
		if row > 0 {
			b.WriteString("\n")
		}
		for col := range weeks {
			b.WriteString(cells[col*daysPerWeek+row])
		}
	}
	return b.String()
}

func renderHeatCell(count int, past bool) string {
	if !past {
		return lipgloss.NewStyle().Foreground(tui.ColorBorder).Render(emptyGlyph)
	}
	for _, bucket := range heatPalette {
		if count < bucket.upperExclusive {
			return lipgloss.NewStyle().Foreground(lipgloss.Color(bucket.hex)).Render(heatCellGlyph)
		}
	}
	return lipgloss.NewStyle().Foreground(tui.ColorPrimary).Render(heatCellGlyph)
}
