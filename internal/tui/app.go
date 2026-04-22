package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/store"
)

// Screen is one pushed node in App.stack. HelpKeys feeds the global help
// line — screens must NOT render their own help row inside View.
type Screen interface {
	Init() tea.Cmd
	Update(tea.Msg) (Screen, tea.Cmd)
	View() string
	HelpKeys() []string
}

// NavMsg drives the screen stack. Exactly one of Replace/Pop/To is honored
// per send; set To alone to push.
type NavMsg struct {
	To      Screen
	Replace bool
	Pop     bool
}

type ToastMsg struct {
	Text    string
	IsError bool
}

func Toast(s string) tea.Cmd    { return func() tea.Msg { return ToastMsg{Text: s} } }
func ToastErr(s string) tea.Cmd { return func() tea.Msg { return ToastMsg{Text: s, IsError: true} } }

type clearToastMsg struct{}

// Frame geometry the App reserves around every screen: border (1 cell each
// side) + padding (1 cell each side) + header row + blank + help row.
// Screens receive the "inner" WindowSizeMsg with these rows already subtracted.
const (
	frameBorderWidth  = 2
	framePaddingWidth = 2
	frameHorizontal   = frameBorderWidth + framePaddingWidth
	frameHeaderRows   = 1
	frameBlankRows    = 1
	frameHelpRows     = 1
	frameVertical     = frameBorderWidth + framePaddingWidth + frameHeaderRows + frameBlankRows + frameHelpRows

	minContentW  = 110
	minTerminalW = 60
	minTerminalH = 16
	toastDuration = 2 * time.Second
)

type App struct {
	store              *store.Store
	stack              []Screen
	w, h               int
	contentW, contentH int
	xMargin, yMargin   int
	toast              string
	toastIsErr         bool
}

func NewApp(s *store.Store, initial Screen) *App {
	return &App{store: s, stack: []Screen{initial}, xMargin: 2, yMargin: 1}
}

func (a *App) Init() tea.Cmd { return a.top().Init() }
func (a *App) top() Screen   { return a.stack[len(a.stack)-1] }

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.resize(m.Width, m.Height)
		return a, a.forwardToTop(a.innerSize())
	case tea.KeyMsg:
		if m.String() == "ctrl+c" {
			return a, tea.Quit
		}
	case NavMsg:
		return a, a.handleNav(m)
	case ToastMsg:
		a.toast = m.Text
		a.toastIsErr = m.IsError
		return a, tea.Tick(toastDuration, func(time.Time) tea.Msg { return clearToastMsg{} })
	case clearToastMsg:
		a.toast = ""
		return a, nil
	}
	return a, a.forwardToTop(msg)
}

func (a *App) resize(w, h int) {
	a.w, a.h = w, h
	a.xMargin = computeXMargin(w)
	a.contentW = max(0, w-(a.xMargin*2))
	a.contentH = max(0, h-(a.yMargin*2))
}

func (a *App) forwardToTop(msg tea.Msg) tea.Cmd {
	next, cmd := a.top().Update(msg)
	a.stack[len(a.stack)-1] = next
	return cmd
}

func (a *App) handleNav(m NavMsg) tea.Cmd {
	switch {
	case m.Replace && m.To != nil:
		a.stack = []Screen{m.To}
	case m.Pop:
		if len(a.stack) > 1 {
			a.stack = a.stack[:len(a.stack)-1]
		}
	case m.To != nil:
		a.stack = append(a.stack, m.To)
	default:
		return nil
	}
	return a.activateTop(a.top().Init())
}

// activateTop replays the current inner size to the now-top screen. Bubble
// Tea only sends WindowSizeMsg on terminal resize, so pushed/popped screens
// would otherwise never learn their dimensions.
func (a *App) activateTop(initCmd tea.Cmd) tea.Cmd {
	if a.contentW <= 0 || a.contentH <= 0 {
		return initCmd
	}
	resizeCmd := a.forwardToTop(a.innerSize())
	return tea.Batch(initCmd, resizeCmd)
}

func (a *App) innerSize() tea.WindowSizeMsg {
	return tea.WindowSizeMsg{
		Width:  max(0, a.contentW-frameHorizontal),
		Height: max(0, a.contentH-frameVertical),
	}
}

func (a *App) View() string {
	if a.tooSmall() {
		return a.tooSmallNotice()
	}

	body := a.renderBody()
	if a.toast != "" {
		body = lipgloss.JoinVertical(lipgloss.Left, body, "", a.toastStyle().Render(a.toast))
	}

	if a.w <= 0 || a.h <= 0 {
		return body
	}

	framed := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(1).
		Width(max(0, a.contentW-2)).
		Height(max(0, a.contentH-2)).
		Render(body)
	return lipgloss.Place(a.w, a.h, lipgloss.Center, lipgloss.Top, framed)
}

func (a *App) tooSmall() bool {
	return a.w > 0 && (a.w < minTerminalW || a.h < minTerminalH)
}

func (a *App) tooSmallNotice() string {
	return StyleDanger.Render("terminal too small ") +
		StyleMuted.Render(fmt.Sprintf("(need at least %d×%d, got %d×%d)",
			minTerminalW, minTerminalH, a.w, a.h))
}

func (a *App) renderBody() string {
	header := StylePrimary.Render("gocards") + StyleMuted.Render("  terminal flashcards")
	body := lipgloss.NewStyle().Height(max(1, a.contentH-frameVertical)).Render(a.top().View())
	help := HelpLine(a.top().HelpKeys()...)
	return lipgloss.JoinVertical(lipgloss.Left, header, "", body, help)
}

func (a *App) toastStyle() lipgloss.Style {
	if a.toastIsErr {
		return StyleDanger
	}
	return StyleSuccess
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
