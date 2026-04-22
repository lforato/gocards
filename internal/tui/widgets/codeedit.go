package widgets

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lforato/vimtea"

	"github.com/lforato/gocards/internal/tui"
)

// CodeEditor embeds a vimtea.Editor as a modal widget inside a parent
// tea.Model. Pressing <esc> in normal mode dispatches a vimtea.QuitMsg that
// this wrapper intercepts to set Done()/Value() for the parent.
type CodeEditor struct {
	editor vimtea.Editor
	title  string
	lang   string
	width  int
	height int
	value  string
	done   bool
}

func NewCodeEditor(title, content, lang string, width, height int) CodeEditor {
	if width < 20 {
		width = 80
	}
	if height < 6 {
		height = 14
	}

	ed := vimtea.NewEditor(
		vimtea.WithContent(content),
		vimtea.WithFileName("code"+LangExt(lang)),
		vimtea.WithEnableStatusBar(false),
	)
	ed.AddBinding(vimtea.KeyBinding{
		Key:     "esc",
		Mode:    vimtea.ModeNormal,
		Handler: vimtea.QuitCmd,
	})

	inner := max(1, height-3)
	ed.SetSize(width-2, inner)

	return CodeEditor{
		editor: ed,
		title:  title,
		lang:   lang,
		width:  width,
		height: height,
		value:  content,
	}
}

func (e CodeEditor) Init() tea.Cmd { return e.editor.Init() }
func (e CodeEditor) Done() bool    { return e.done }
func (e CodeEditor) Value() string { return e.value }

// SetSize re-fits the editor chrome and the underlying vimtea viewport.
// Returns the updated value so callers holding CodeEditor by value can
// reassign.
func (e CodeEditor) SetSize(width, height int) CodeEditor {
	if width < 20 {
		width = 80
	}
	if height < 6 {
		height = 14
	}
	e.width = width
	e.height = height
	e.editor.SetSize(width-2, max(1, height-3))
	return e
}

func (e CodeEditor) Update(msg tea.Msg) (CodeEditor, tea.Cmd) {
	if q, ok := msg.(vimtea.QuitMsg); ok {
		e.value = q.Content
		e.done = true
		return e, nil
	}
	_, cmd := e.editor.Update(msg)
	return e, cmd
}

func (e CodeEditor) View() string {
	modeLabel := "NORMAL"
	modeStyle := lipgloss.NewStyle().Foreground(tui.ColorAccent).Bold(true)
	switch e.editor.GetMode() {
	case vimtea.ModeInsert:
		modeLabel = "INSERT"
		modeStyle = lipgloss.NewStyle().Foreground(tui.ColorSuccess).Bold(true)
	case vimtea.ModeVisual:
		modeLabel = "VISUAL"
		modeStyle = lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
	case vimtea.ModeCommand:
		modeLabel = "COMMAND"
		modeStyle = lipgloss.NewStyle().Foreground(tui.ColorWarn).Bold(true)
	}

	titleBar := tui.StyleTitle.Render(e.title) + "  " +
		tui.StyleMuted.Render("["+e.lang+"]") + "  " +
		modeStyle.Render(modeLabel)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorPrimary).
		Padding(0, 1).
		Width(e.width).
		Render(e.editor.View())

	return lipgloss.JoinVertical(lipgloss.Left, titleBar, box)
}

// langExtByName maps language identifiers to filename extensions so vimtea's
// chroma-based highlighter can pick the right lexer.
var langExtByName = map[string]string{
	"javascript": ".js", "js": ".js",
	"typescript": ".ts", "ts": ".ts",
	"tsx":    ".tsx",
	"jsx":    ".jsx",
	"python": ".py", "py": ".py",
	"go": ".go", "golang": ".go",
	"rust": ".rs", "rs": ".rs",
	"c":   ".c",
	"cpp": ".cpp", "c++": ".cpp",
	"csharp": ".cs", "cs": ".cs", "c#": ".cs",
	"java": ".java",
	"ruby": ".rb", "rb": ".rb",
	"sql": ".sql",
	"sh":  ".sh", "bash": ".sh", "shell": ".sh",
	"html": ".html",
	"css":  ".css",
	"json": ".json",
	"md":   ".md", "markdown": ".md",
}

// LangExt returns the filename extension for a source language, defaulting to
// .txt for unknown values.
func LangExt(lang string) string {
	if ext, ok := langExtByName[strings.ToLower(lang)]; ok {
		return ext
	}
	return ".txt"
}
