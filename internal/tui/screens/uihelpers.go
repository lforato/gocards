package screens

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/tui"
)

// cursorUp / cursorDown / cycleFocus centralize the bounds math every list-y
// screen was duplicating. count is the number of items (>= 0); cursor is
// clamped into [0, count-1] on every call.

func cursorUp(cursor int) int {
	if cursor <= 0 {
		return 0
	}
	return cursor - 1
}

func cursorDown(cursor, count int) int {
	if count == 0 {
		return 0
	}
	if cursor >= count-1 {
		return count - 1
	}
	return cursor + 1
}

// cycleFocus advances a ring-buffer cursor by delta, wrapping at both ends.
func cycleFocus(focus, delta, count int) int {
	if count <= 0 {
		return 0
	}
	return (focus + delta%count + count) % count
}

// selectionPrefix is the leading gutter used by every selectable list row.
func selectionPrefix(selected bool) string {
	if selected {
		return tui.StylePrimary.Render("▶ ")
	}
	return "  "
}

func colorBullet(hex string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render("●")
}

func cardTypeColor(t models.CardType) lipgloss.Color {
	switch t {
	case models.CardMCQ:
		return tui.ColorAccent
	case models.CardCode:
		return tui.ColorSuccess
	case models.CardFill:
		return tui.ColorWarn
	case models.CardExp:
		return tui.ColorPrimary
	}
	return tui.ColorMuted
}

func cardTypeBadge(t models.CardType) string {
	return lipgloss.NewStyle().Foreground(cardTypeColor(t)).Render(fmt.Sprintf("[%s]", t))
}

// truncate clips s to at most n runes, appending … when truncation happens.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// pluralize returns "N singular" when N == 1 and "N plural" otherwise.
func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}
