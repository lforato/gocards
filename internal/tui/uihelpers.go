package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/models"
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
// Used by forms and paged pickers.
func cycleFocus(focus, delta, count int) int {
	if count <= 0 {
		return 0
	}
	return (focus + delta%count + count) % count
}

// selectionPrefix is the leading gutter used by every selectable list row:
// a highlighted "▶ " when selected, two spaces otherwise.
func selectionPrefix(selected bool) string {
	if selected {
		return StylePrimary.Render("▶ ")
	}
	return "  "
}

// colorBullet is the little "●" rendered in the deck's accent color.
func colorBullet(hex string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render("●")
}

// cardTypeColor returns the theme color assigned to a card type. Falls back to
// the muted palette for unknown values so new types don't crash rendering.
func cardTypeColor(t models.CardType) lipgloss.Color {
	switch t {
	case models.CardMCQ:
		return ColorAccent
	case models.CardCode:
		return ColorSuccess
	case models.CardFill:
		return ColorWarn
	case models.CardExp:
		return ColorPrimary
	}
	return ColorMuted
}

// cardTypeBadge renders "[type]" with the type's theme color. Used in lists.
func cardTypeBadge(t models.CardType) string {
	return lipgloss.NewStyle().Foreground(cardTypeColor(t)).Render(fmt.Sprintf("[%s]", t))
}

