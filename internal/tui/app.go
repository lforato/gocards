package tui

import (
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

// Frame geometry the App reserves around every screen: rounded border (1 cell
// each side) + 1-cell padding (each side) + header row + blank spacer + help
// row. Screens use the "inner" WindowSizeMsg which already has these rows
// subtracted.
const (
	frameBorderWidth  = 2
	framePaddingWidth = 2
	frameHorizontal   = frameBorderWidth + framePaddingWidth
	frameHeaderRows   = 1
	frameBlankRows    = 1
	frameHelpRows     = 1
	frameVertical     = frameBorderWidth + framePaddingWidth + frameHeaderRows + frameBlankRows + frameHelpRows
	minContentW       = 110
)

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

// NewApp starts the screen stack on initial — callers pass in whichever
// screen the app should boot into (typically screens.NewDashboard).
func NewApp(s *store.Store, initial Screen) *App {
	return &App{store: s, stack: []Screen{initial}, xMargin: 2, yMargin: 1}
}

func (a *App) Init() tea.Cmd { return a.top().Init() }

func (a *App) top() Screen { return a.stack[len(a.stack)-1] }

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.w = m.Width
		a.h = m.Height

		a.xMargin = computeXMargin(m.Width)
		a.contentW = max(0, m.Width-(a.xMargin*2))
		a.contentH = max(0, m.Height-(a.yMargin*2))

		next, cmd := a.top().Update(a.innerSize())
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

	bodyH := max(1, a.contentH-frameVertical)
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
	next, resizeCmd := a.top().Update(a.innerSize())
	a.stack[len(a.stack)-1] = next
	return tea.Batch(initCmd, resizeCmd)
}

func (a *App) innerSize() tea.WindowSizeMsg {
	return tea.WindowSizeMsg{
		Width:  max(0, a.contentW-frameHorizontal),
		Height: max(0, a.contentH-frameVertical),
	}
}

func computeXMargin(width int) int {
	switch {
	case width >= 3*minContentW:
		return width / 3
	case width >= minContentW:
		return (width - minContentW) / 2
	}
	return 0
}

type clearToastMsg struct{}

func Toast(s string) tea.Cmd {
	return func() tea.Msg { return ToastMsg{Text: s} }
}

func ToastErr(s string) tea.Cmd {
	return func() tea.Msg { return ToastMsg{Text: s, IsError: true} }
}
