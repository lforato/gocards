package screens

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}
