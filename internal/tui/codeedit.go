package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lforato/vimtea"
)

// CodeEditor embeds a vimtea.Editor as a modal widget inside a parent tea.Model.
// Pressing <esc> in normal mode dispatches a vimtea.QuitMsg that this wrapper
// intercepts to set Done()/Value() for the parent.
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
		vimtea.WithFileName("code"+langExt(lang)),
		vimtea.WithEnableStatusBar(false),
	)
	ed.AddBinding(vimtea.KeyBinding{
		Key:     "esc",
		Mode:    vimtea.ModeNormal,
		Handler: vimtea.QuitCmd,
	})

	inner := height - 3
	if inner < 1 {
		inner = 1
	}
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

// SetSize re-fits the editor chrome and the underlying vimtea viewport to new
// outer dimensions. Returns the updated CodeEditor so callers holding it by
// value can reassign: `e.editor = e.editor.SetSize(w, h)`.
func (e CodeEditor) SetSize(width, height int) CodeEditor {
	if width < 20 {
		width = 80
	}
	if height < 6 {
		height = 14
	}
	e.width = width
	e.height = height
	inner := height - 3
	if inner < 1 {
		inner = 1
	}
	if e.editor != nil {
		e.editor.SetSize(width-2, inner)
	}
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
	modeStyle := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	switch e.editor.GetMode() {
	case vimtea.ModeInsert:
		modeLabel = "INSERT"
		modeStyle = lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true)
	case vimtea.ModeVisual:
		modeLabel = "VISUAL"
		modeStyle = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	case vimtea.ModeCommand:
		modeLabel = "COMMAND"
		modeStyle = lipgloss.NewStyle().Foreground(ColorWarn).Bold(true)
	}

	titleBar := StyleTitle.Render(e.title) + "  " +
		StyleMuted.Render("["+e.lang+"]") + "  " +
		modeStyle.Render(modeLabel)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Padding(0, 1).
		Width(e.width).
		Render(e.editor.View())

	return lipgloss.JoinVertical(lipgloss.Left, titleBar, box)
}

// langExt maps a language identifier to a filename extension so vimtea's
// chroma-based highlighter can pick the right lexer.
func langExt(lang string) string {
	switch strings.ToLower(lang) {
	case "javascript", "js":
		return ".js"
	case "typescript", "ts":
		return ".ts"
	case "tsx":
		return ".tsx"
	case "jsx":
		return ".jsx"
	case "python", "py":
		return ".py"
	case "go", "golang":
		return ".go"
	case "rust", "rs":
		return ".rs"
	case "c":
		return ".c"
	case "cpp", "c++":
		return ".cpp"
	case "csharp", "cs", "c#":
		return ".cs"
	case "java":
		return ".java"
	case "ruby", "rb":
		return ".rb"
	case "sql":
		return ".sql"
	case "sh", "bash", "shell":
		return ".sh"
	case "html":
		return ".html"
	case "css":
		return ".css"
	case "json":
		return ".json"
	case "md", "markdown":
		return ".md"
	default:
		return ".txt"
	}
}
