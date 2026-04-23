package widgets

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lforato/vimtea"

	"github.com/lforato/gocards/internal/tui"
)

const (
	minEditorWidth      = 20
	defaultEditorWidth  = 80
	minEditorHeight     = 6
	defaultEditorHeight = 14
	// borderAndTitleOverhead accounts for the 2-cell rounded border plus the
	// 1-line title bar that frame the vimtea editor.
	borderAndTitleOverhead = 3
	// borderHorizontalOverhead is the combined left+right border cell count.
	borderHorizontalOverhead = 2
)

// CodeEditor is a modal vim-backed text editor. Parents watch Done() to
// detect when the user pressed esc in normal mode, then read Value().
type CodeEditor struct {
	editor vimtea.Editor
	title  string
	lang   string
	width  int
	height int
	value  string
	done   bool
}

// NewCodeEditor builds an editor with the given content, language (used
// for syntax highlighting), and outer size. Sizes below the minimums are
// expanded to safe defaults so the editor always renders legibly.
func NewCodeEditor(title, content, lang string, width, height int) CodeEditor {
	width, height = clampEditorSize(width, height)

	editor := vimtea.NewEditor(
		vimtea.WithContent(content),
		vimtea.WithFileName("code"+LangExt(lang)),
		vimtea.WithEnableStatusBar(false),
	)
	editor.AddBinding(vimtea.KeyBinding{
		Key:     "esc",
		Mode:    vimtea.ModeNormal,
		Handler: vimtea.QuitCmd,
	})
	editor.SetSize(innerWidth(width), innerHeight(height))

	return CodeEditor{
		editor: editor,
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

// SetSize returns a resized copy so callers holding CodeEditor by value can
// reassign: `e = e.SetSize(w, h)`.
func (e CodeEditor) SetSize(width, height int) CodeEditor {
	width, height = clampEditorSize(width, height)
	e.width = width
	e.height = height
	e.editor.SetSize(innerWidth(width), innerHeight(height))
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
	modeLabel, modeStyle := vimModeBadge(e.editor.GetMode())

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

// vimModeBadge returns the label and style to paint next to the title,
// mirroring vim's bottom-row mode indicator (NORMAL / INSERT / VISUAL / COMMAND).
func vimModeBadge(mode vimtea.EditorMode) (string, lipgloss.Style) {
	switch mode {
	case vimtea.ModeInsert:
		return "INSERT", lipgloss.NewStyle().Foreground(tui.ColorSuccess).Bold(true)
	case vimtea.ModeVisual:
		return "VISUAL", lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
	case vimtea.ModeCommand:
		return "COMMAND", lipgloss.NewStyle().Foreground(tui.ColorWarn).Bold(true)
	default:
		return "NORMAL", lipgloss.NewStyle().Foreground(tui.ColorAccent).Bold(true)
	}
}

func clampEditorSize(width, height int) (int, int) {
	if width < minEditorWidth {
		width = defaultEditorWidth
	}
	if height < minEditorHeight {
		height = defaultEditorHeight
	}
	return width, height
}

func innerWidth(outerWidth int) int   { return outerWidth - borderHorizontalOverhead }
func innerHeight(outerHeight int) int { return max(1, outerHeight-borderAndTitleOverhead) }

// vimtea's chroma highlighter picks lexers by filename extension. We build a
// synthetic filename like "code.go" so the highlighter applies the right lexer.
var langExtByName = map[string]string{
	"javascript": ".js", "js": ".js",
	"typescript": ".ts", "ts": ".ts",
	"tsx":    ".tsx",
	"jsx":    ".jsx",
	"python": ".py", "py": ".py",
	"go": ".go", "golang": ".go",
	"rust": ".rs", "rs": ".rs",
	"c":      ".c",
	"opengl": ".c",
	"cpp":    ".cpp", "c++": ".cpp",
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

// LangExt maps a language name to the filename extension vimtea uses to pick
// a syntax-highlighting lexer. Unknown languages fall back to plain ".txt".
func LangExt(lang string) string {
	if ext, ok := langExtByName[strings.ToLower(lang)]; ok {
		return ext
	}
	return ".txt"
}
