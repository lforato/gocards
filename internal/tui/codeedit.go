package tui

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CodeEditor is an in-TUI multi-line text editor with vim-style modal keybindings
// and syntax highlighting.
type CodeEditor struct {
	lines   []string
	row     int
	col     int
	mode    string // "normal" | "insert"
	lang    string
	title   string
	width   int
	height  int
	offsetY int
	pending string // multi-char command buffer: "g", "d"
	done    bool
	style   *chroma.Style
}

const (
	modeNormal = "normal"
	modeInsert = "insert"
)

func NewCodeEditor(title, content, lang string, width, height int) CodeEditor {
	if width < 20 {
		width = 80
	}
	if height < 6 {
		height = 14
	}
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	st := styles.Get("gruvbox")
	if st == nil {
		st = styles.Fallback
	}
	return CodeEditor{
		lines:  lines,
		mode:   modeNormal,
		lang:   lang,
		title:  title,
		width:  width,
		height: height,
		style:  st,
	}
}

func (e CodeEditor) Done() bool    { return e.done }
func (e CodeEditor) Value() string { return strings.Join(e.lines, "\n") }

func (e *CodeEditor) clampCursor() {
	if e.row < 0 {
		e.row = 0
	}
	if e.row >= len(e.lines) {
		e.row = len(e.lines) - 1
	}
	maxCol := len([]rune(e.lines[e.row]))
	if e.mode == modeNormal && maxCol > 0 {
		maxCol--
	}
	if e.col < 0 {
		e.col = 0
	}
	if e.col > maxCol {
		e.col = maxCol
	}
}

func (e *CodeEditor) ensureVisible() {
	// height includes title bar and help line; reserve 3 lines of chrome
	inner := e.height - 3
	if inner < 1 {
		inner = 1
	}
	if e.row < e.offsetY {
		e.offsetY = e.row
	}
	if e.row >= e.offsetY+inner {
		e.offsetY = e.row - inner + 1
	}
	if e.offsetY < 0 {
		e.offsetY = 0
	}
}

func (e CodeEditor) Update(msg tea.KeyMsg) CodeEditor {
	if e.mode == modeInsert {
		return e.updateInsert(msg)
	}
	return e.updateNormal(msg)
}

func (e CodeEditor) updateNormal(msg tea.KeyMsg) CodeEditor {
	k := msg.String()

	// Multi-char command handling
	if e.pending == "g" {
		e.pending = ""
		if k == "g" {
			e.row = 0
			e.col = 0
			e.clampCursor()
			e.ensureVisible()
			return e
		}
		// fall through — unrecognized
	}
	if e.pending == "d" {
		e.pending = ""
		if k == "d" {
			return e.deleteLine()
		}
	}

	switch k {
	case "esc":
		e.done = true
		return e
	case "h", "left":
		if e.col > 0 {
			e.col--
		}
	case "l", "right":
		maxCol := len([]rune(e.lines[e.row]))
		if maxCol > 0 && e.col < maxCol-1 {
			e.col++
		}
	case "j", "down":
		if e.row < len(e.lines)-1 {
			e.row++
			e.clampCursor()
		}
	case "k", "up":
		if e.row > 0 {
			e.row--
			e.clampCursor()
		}
	case "0":
		e.col = 0
	case "$":
		e.col = len([]rune(e.lines[e.row])) - 1
		if e.col < 0 {
			e.col = 0
		}
	case "g":
		e.pending = "g"
	case "G":
		e.row = len(e.lines) - 1
		e.col = 0
		e.clampCursor()
	case "w":
		e.wordForward()
	case "b":
		e.wordBack()
	case "i":
		e.mode = modeInsert
	case "a":
		maxCol := len([]rune(e.lines[e.row]))
		if maxCol > 0 {
			e.col++
		}
		e.mode = modeInsert
	case "I":
		e.col = 0
		e.mode = modeInsert
	case "A":
		e.col = len([]rune(e.lines[e.row]))
		e.mode = modeInsert
	case "o":
		e.lines = insertStringAt(e.lines, e.row+1, "")
		e.row++
		e.col = 0
		e.mode = modeInsert
	case "O":
		e.lines = insertStringAt(e.lines, e.row, "")
		e.col = 0
		e.mode = modeInsert
	case "x":
		e.deleteCharUnderCursor()
	case "d":
		e.pending = "d"
	}
	e.ensureVisible()
	return e
}

func (e CodeEditor) updateInsert(msg tea.KeyMsg) CodeEditor {
	k := msg.String()
	switch k {
	case "esc":
		e.mode = modeNormal
		if e.col > 0 {
			e.col--
		}
		e.clampCursor()
		e.ensureVisible()
		return e
	case "enter":
		runes := []rune(e.lines[e.row])
		before := string(runes[:e.col])
		after := string(runes[e.col:])
		e.lines[e.row] = before
		e.lines = insertStringAt(e.lines, e.row+1, after)
		e.row++
		e.col = 0
	case "backspace":
		runes := []rune(e.lines[e.row])
		if e.col > 0 {
			e.lines[e.row] = string(runes[:e.col-1]) + string(runes[e.col:])
			e.col--
		} else if e.row > 0 {
			prev := e.lines[e.row-1]
			e.col = len([]rune(prev))
			e.lines[e.row-1] = prev + e.lines[e.row]
			e.lines = append(e.lines[:e.row], e.lines[e.row+1:]...)
			e.row--
		}
	case "left":
		if e.col > 0 {
			e.col--
		}
	case "right":
		if e.col < len([]rune(e.lines[e.row])) {
			e.col++
		}
	case "up":
		if e.row > 0 {
			e.row--
			e.clampCursor()
		}
	case "down":
		if e.row < len(e.lines)-1 {
			e.row++
			e.clampCursor()
		}
	case "tab":
		e.insertRunes([]rune("  "))
	default:
		if len(msg.Runes) > 0 {
			e.insertRunes(msg.Runes)
		}
	}
	e.ensureVisible()
	return e
}

func (e *CodeEditor) insertRunes(rs []rune) {
	runes := []rune(e.lines[e.row])
	before := runes[:e.col]
	after := runes[e.col:]
	e.lines[e.row] = string(before) + string(rs) + string(after)
	e.col += len(rs)
}

func (e *CodeEditor) deleteCharUnderCursor() {
	runes := []rune(e.lines[e.row])
	if len(runes) == 0 {
		return
	}
	if e.col >= len(runes) {
		return
	}
	e.lines[e.row] = string(runes[:e.col]) + string(runes[e.col+1:])
	if e.col > 0 && e.col >= len([]rune(e.lines[e.row])) {
		e.col--
	}
}

func (e CodeEditor) deleteLine() CodeEditor {
	if len(e.lines) == 1 {
		e.lines[0] = ""
		e.col = 0
		return e
	}
	e.lines = append(e.lines[:e.row], e.lines[e.row+1:]...)
	if e.row >= len(e.lines) {
		e.row = len(e.lines) - 1
	}
	e.col = 0
	e.clampCursor()
	e.ensureVisible()
	return e
}

func (e *CodeEditor) wordForward() {
	runes := []rune(e.lines[e.row])
	c := e.col
	for c < len(runes) && !isWordChar(runes[c]) {
		c++
	}
	for c < len(runes) && isWordChar(runes[c]) {
		c++
	}
	for c < len(runes) && runes[c] == ' ' {
		c++
	}
	if c >= len(runes) {
		if e.row < len(e.lines)-1 {
			e.row++
			e.col = 0
		} else {
			e.col = len(runes) - 1
			if e.col < 0 {
				e.col = 0
			}
		}
		return
	}
	e.col = c
}

func (e *CodeEditor) wordBack() {
	runes := []rune(e.lines[e.row])
	c := e.col
	if c > 0 {
		c--
	}
	for c > 0 && runes[c] == ' ' {
		c--
	}
	for c > 0 && isWordChar(runes[c-1]) {
		c--
	}
	if c == 0 && !isWordChar(runes[0]) && e.row > 0 {
		e.row--
		e.col = len([]rune(e.lines[e.row])) - 1
		if e.col < 0 {
			e.col = 0
		}
		return
	}
	e.col = c
}

func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

func insertStringAt(lines []string, idx int, s string) []string {
	if idx < 0 {
		idx = 0
	}
	if idx > len(lines) {
		idx = len(lines)
	}
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:idx]...)
	out = append(out, s)
	out = append(out, lines[idx:]...)
	return out
}

// --- rendering ---

func (e CodeEditor) View() string {
	inner := e.height - 3
	if inner < 1 {
		inner = 1
	}
	innerW := e.width - 4
	if innerW < 10 {
		innerW = 10
	}

	var rows []string
	end := e.offsetY + inner
	if end > len(e.lines) {
		end = len(e.lines)
	}
	gutterW := len(itoa(len(e.lines))) + 1

	for i := e.offsetY; i < end; i++ {
		ln := e.renderLine(e.lines[i], i == e.row, e.col)
		gutter := StyleMuted.Render(padLeft(itoa(i+1), gutterW-1) + " ")
		rows = append(rows, gutter+ln)
	}
	// pad to fill inner height
	for len(rows) < inner {
		rows = append(rows, StyleMuted.Render(padLeft("~", gutterW-1)+" "))
	}

	modeLabel := "NORMAL"
	modeStyle := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	if e.mode == modeInsert {
		modeLabel = "INSERT"
		modeStyle = lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true)
	}

	titleBar := StyleTitle.Render(e.title) + "  " +
		StyleMuted.Render("["+e.lang+"]") + "  " +
		modeStyle.Render(modeLabel)

	body := lipgloss.JoinVertical(lipgloss.Left, rows...)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Padding(0, 1).
		Width(e.width).
		Render(body)

	help := HelpLine(e.helpKeys()...)

	return lipgloss.JoinVertical(lipgloss.Left, titleBar, box, help)
}

func (e CodeEditor) helpKeys() []string {
	if e.mode == modeInsert {
		return []string{"esc normal", "enter newline", "backspace del"}
	}
	return []string{"i/a/o insert", "hjkl move", "w/b word", "0/$ line", "gg/G file", "x del", "dd delline", "esc save & back"}
}

// renderLine applies chroma tokenization + cursor overlay.
func (e CodeEditor) renderLine(line string, isCursorRow bool, cursorCol int) string {
	lexer := lexers.Get(e.lang)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	iterator, err := lexer.Tokenise(nil, line)
	if err != nil {
		return line
	}
	var out strings.Builder
	col := 0
	for _, tok := range iterator.Tokens() {
		style := e.tokenStyle(tok.Type)
		for _, r := range tok.Value {
			rs := string(r)
			if rs == "\n" {
				continue
			}
			if isCursorRow && col == cursorCol {
				out.WriteString(cursorRender(rs, e.mode))
			} else {
				out.WriteString(style.Render(rs))
			}
			col++
		}
	}
	if isCursorRow && cursorCol >= col {
		out.WriteString(cursorRender(" ", e.mode))
	}
	return out.String()
}

func cursorRender(s, mode string) string {
	st := lipgloss.NewStyle().Reverse(true)
	if mode == modeInsert {
		st = lipgloss.NewStyle().Foreground(ColorPrimary).Underline(true)
	}
	return st.Render(s)
}

func (e CodeEditor) tokenStyle(t chroma.TokenType) lipgloss.Style {
	s := lipgloss.NewStyle()
	if e.style == nil {
		return s
	}
	entry := e.style.Get(t)
	if entry.Colour.IsSet() {
		s = s.Foreground(lipgloss.Color(entry.Colour.String()))
	}
	if entry.Bold == chroma.Yes {
		s = s.Bold(true)
	}
	if entry.Italic == chroma.Yes {
		s = s.Italic(true)
	}
	return s
}

// --- small helpers ---

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func padLeft(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return strings.Repeat(" ", w-len(s)) + s
}
