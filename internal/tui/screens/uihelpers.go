package screens

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/tui"
)

func navTo(s tui.Screen) tea.Cmd {
	return func() tea.Msg { return tui.NavMsg{To: s} }
}

func navBack() tea.Msg { return tui.NavMsg{Pop: true} }

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

// cycleFocus wraps at both ends, used by tab/shift-tab focus cycling.
func cycleFocus(focus, delta, count int) int {
	if count <= 0 {
		return 0
	}
	return (focus + delta%count + count) % count
}

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

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}
