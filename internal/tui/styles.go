package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	ColorBg      = lipgloss.Color("#0b0f14")
	ColorCard    = lipgloss.Color("#111827")
	ColorBorder  = lipgloss.Color("#1f2937")
	ColorMuted   = lipgloss.Color("#6b7280")
	ColorFg      = lipgloss.Color("#e5e7eb")
	ColorPrimary = lipgloss.Color("#f59e0b")
	ColorAccent  = lipgloss.Color("#60a5fa")
	ColorSuccess = lipgloss.Color("#34d399")
	ColorDanger  = lipgloss.Color("#f87171")
	ColorWarn    = lipgloss.Color("#fbbf24")
)

var (
	StyleTitle = lipgloss.NewStyle().
			Foreground(ColorFg).
			Bold(true)

	StyleMuted = lipgloss.NewStyle().Foreground(ColorMuted)

	StyleCard = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(1, 2)

	StatCard = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(0, 1).
			Width(20)

	StyleAccent = lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)

	StylePrimary = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)

	StyleDanger = lipgloss.NewStyle().Foreground(ColorDanger).Bold(true)

	StyleSuccess = lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true)

	StyleHelp = lipgloss.NewStyle().Foreground(ColorMuted).Italic(true)

	StyleButton = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(ColorBg).
			Background(ColorPrimary).
			Bold(true)

	StyleSelected = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)
)

func HelpLine(items ...string) string {
	return StyleHelp.Render(join(items, "  ·  "))
}

// HelpLineSpread justifies items across `width` cells: item · … · item · item.
// Falls back to the compact separator when width is too narrow to spread cleanly.
func HelpLineSpread(width int, items ...string) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) == 1 {
		return StyleHelp.Render(items[0])
	}

	total := 0
	for _, it := range items {
		total += lipgloss.Width(it)
	}

	gaps := len(items) - 1
	space := width - total
	const minGap = 3
	if space < gaps*minGap {
		return HelpLine(items...)
	}

	perGap := space / gaps
	extra := space % gaps

	var b strings.Builder
	for i, it := range items {
		b.WriteString(it)
		if i == len(items)-1 {
			break
		}
		g := perGap
		if i < extra {
			g++
		}
		left := (g - 1) / 2
		right := g - 1 - left
		b.WriteString(strings.Repeat(" ", left))
		b.WriteString("·")
		b.WriteString(strings.Repeat(" ", right))
	}
	return StyleHelp.Render(b.String())
}

func join(items []string, sep string) string {
	out := ""
	for i, s := range items {
		if i > 0 {
			out += sep
		}
		out += s
	}
	return out
}
