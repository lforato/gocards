package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/store"
)

// Screen is any sub-model rendered inside the App.
type Screen interface {
	Init() tea.Cmd
	Update(tea.Msg) (Screen, tea.Cmd)
	View() string
	// HelpKeys returns the help-line items for the screen's current state.
	// The App renders these at the bottom of the frame using HelpLineSpread,
	// so individual screens must not include a help line in their View output.
	HelpKeys() []string
}

// NavMsg pushes or replaces the active screen.
type NavMsg struct {
	To      Screen
	Replace bool
	Pop     bool
}

// ToastMsg shows a transient status line at the bottom.
type ToastMsg struct {
	Text    string
	IsError bool
}

// App is the root Bubble Tea model.
type App struct {
	store              *store.Store
	stack              []Screen
	w, h               int // full terminal/pane size
	contentW, contentH int // usable area (terminal minus margins)
	xMargin, yMargin   int
	toast              string
	toastIsErr         bool
}

func NewApp(s *store.Store) *App {
	return &App{store: s, stack: []Screen{NewDashboard(s)}, xMargin: 2, yMargin: 1}
}

func (a *App) Init() tea.Cmd { return a.top().Init() }

func (a *App) top() Screen { return a.stack[len(a.stack)-1] }

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.w = m.Width
		a.h = m.Height

		const minContentW = 110
		switch {
		case m.Width >= 3*minContentW:
			// plenty of room — each side gets a third, content also a third
			a.xMargin = m.Width / 3
		case m.Width >= minContentW:
			// below the 1/3 ideal but still fits the minimum — shrink margins, keep content
			a.xMargin = (m.Width - minContentW) / 2
		default:
			// terminal too small for the minimum — no margin, content shrinks
			a.xMargin = 0
		}

		a.contentW = max(0, m.Width-(a.xMargin*2))
		a.contentH = max(0, m.Height-(a.yMargin*2))

		// forward the usable size to screens. Subtract:
		//   horizontal — border (2) + padding (2) = 4
		//   vertical   — border (2) + padding (2) + app header (1) + blank (1) + help (1) = 7
		inner := tea.WindowSizeMsg{Width: max(0, a.contentW-4), Height: max(0, a.contentH-7)}
		next, cmd := a.top().Update(inner)
		a.stack[len(a.stack)-1] = next
		return a, cmd
	case tea.KeyMsg:
		if m.String() == "ctrl+c" {
			return a, tea.Quit
		}
	case NavMsg:
		if m.Replace {
			a.stack = []Screen{m.To}
			return a, a.activateTop(m.To.Init())
		}
		if m.Pop {
			if len(a.stack) > 1 {
				a.stack = a.stack[:len(a.stack)-1]
			}
			return a, a.activateTop(a.top().Init())
		}
		if m.To != nil {
			a.stack = append(a.stack, m.To)
			return a, a.activateTop(m.To.Init())
		}
	case ToastMsg:
		a.toast = m.Text
		a.toastIsErr = m.IsError
		return a, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return clearToastMsg{} })
	case clearToastMsg:
		a.toast = ""
		return a, nil
	}

	next, cmd := a.top().Update(msg)
	a.stack[len(a.stack)-1] = next
	return a, cmd
}

func (a *App) View() string {
	header := StylePrimary.Render("gocards") + StyleMuted.Render("  terminal flashcards")
	content := a.top().View()

	// Inner vertical budget available to the screen body (everything inside
	// the frame, minus the app header + blank + help row). The help row is
	// rendered below in a separate JoinVertical step so it sits pinned to
	// the last row of the frame's content area.
	// Frame content area = contentH - 4 (border 2 + padding 2).
	// Body layout inside: header(1) + blank(1) + paddedContent(bodyH) + help(1) = contentH - 4
	// → bodyH = contentH - 7
	bodyH := max(1, a.contentH-7)
	// Pad the screen body so it fills the reserved area — keeps the help
	// line from floating up when a screen's content is short.
	paddedContent := lipgloss.NewStyle().Height(bodyH).Render(content)

	helpItems := a.top().HelpKeys()
	helpLine := HelpLine(helpItems...)

	body := lipgloss.JoinVertical(lipgloss.Left, header, "", paddedContent, helpLine)

	if a.toast != "" {
		style := StyleSuccess
		if a.toastIsErr {
			style = StyleDanger
		}
		body = lipgloss.JoinVertical(lipgloss.Left, body, "", style.Render(a.toast))
	}

	if a.w <= 0 || a.h <= 0 {
		return body
	}

	// Frame the body with a rounded border + 1 cell padding all around.
	// Width/Height set the frame's content+padding area; border adds 2 more cells
	// in each dimension, so final rendered size = contentW × contentH.
	framed := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(1).
		Width(max(0, a.contentW-2)).
		Height(max(0, a.contentH-2)).
		Render(body)

	// Place centers the framed box in the full terminal — the surrounding gap
	// is exactly xMargin/yMargin on each side, since contentW = w - 2*xMargin.
	return lipgloss.Place(a.w, a.h, lipgloss.Center, lipgloss.Top, framed)
}

// activateTop forwards the current inner window size to the top-of-stack
// screen so newly-pushed (or revealed) screens know how much room they have
// to work with — Bubble Tea only sends WindowSizeMsg on terminal resize.
func (a *App) activateTop(initCmd tea.Cmd) tea.Cmd {
	if a.contentW <= 0 || a.contentH <= 0 {
		return initCmd
	}
	inner := tea.WindowSizeMsg{Width: max(0, a.contentW-4), Height: max(0, a.contentH-7)}
	next, resizeCmd := a.top().Update(inner)
	a.stack[len(a.stack)-1] = next
	return tea.Batch(initCmd, resizeCmd)
}

type clearToastMsg struct{}

func Toast(s string) tea.Cmd {
	return func() tea.Msg { return ToastMsg{Text: s} }
}

func ToastErr(s string) tea.Cmd {
	return func() tea.Msg { return ToastMsg{Text: s, IsError: true} }
}

// --- helpers ---

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
