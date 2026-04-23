package widgets

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/tui"
)

// FormField is one cell in a MultiForm. Implementations wrap a concrete
// input widget (textinput, picker, etc.) and own focus + rendering for
// that cell.
type FormField interface {
	Focus()
	Blur()
	View() string
	Update(tea.Msg) tea.Cmd
	Value() string
}

// TextFormField wraps a textinput.Model so it can sit alongside pickers
// in a MultiForm. New fields should prefer this over reaching into the
// raw textinput.Model directly.
type TextFormField struct{ Input textinput.Model }

func NewTextFormField(ti textinput.Model) *TextFormField { return &TextFormField{Input: ti} }

func (f *TextFormField) Focus()             { f.Input.Focus() }
func (f *TextFormField) Blur()              { f.Input.Blur() }
func (f *TextFormField) View() string       { return f.Input.View() }
func (f *TextFormField) Value() string      { return f.Input.Value() }
func (f *TextFormField) Update(m tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	f.Input, cmd = f.Input.Update(m)
	return cmd
}

// PickerFormField offers a left/right-scrollable choice from a fixed set
// of options. Rendered as "‹ option ›" when focused, plain when blurred.
type PickerFormField struct {
	Options  []string
	Selected int
	focused  bool
}

func NewPickerFormField(options []string, initial string) *PickerFormField {
	p := &PickerFormField{Options: options}
	for i, o := range options {
		if o == initial {
			p.Selected = i
			return p
		}
	}
	return p
}

func (p *PickerFormField) Focus()        { p.focused = true }
func (p *PickerFormField) Blur()         { p.focused = false }
func (p *PickerFormField) Value() string {
	if p.Selected < 0 || p.Selected >= len(p.Options) {
		return ""
	}
	return p.Options[p.Selected]
}

func (p *PickerFormField) Update(m tea.Msg) tea.Cmd {
	km, ok := m.(tea.KeyMsg)
	if !ok || !p.focused {
		return nil
	}
	switch km.String() {
	case "left", "h":
		if p.Selected > 0 {
			p.Selected--
		} else {
			p.Selected = len(p.Options) - 1
		}
	case "right", "l":
		if p.Selected < len(p.Options)-1 {
			p.Selected++
		} else {
			p.Selected = 0
		}
	}
	return nil
}

func (p *PickerFormField) View() string {
	if len(p.Options) == 0 {
		return tui.StyleMuted.Render("(no options)")
	}
	val := p.Value()
	if p.focused {
		return lipgloss.JoinHorizontal(lipgloss.Top,
			tui.StylePrimary.Render("‹ "),
			tui.StyleSelected.Render(val),
			tui.StylePrimary.Render(" ›"),
		)
	}
	return tui.StyleMuted.Render("  " + val + "  ")
}

// MultiForm manages focus cycling across a heterogeneous slice of
// FormFields (text inputs, pickers, etc.). Tab/shift-tab/up/down cycle
// focus; left/right inside a focused field are consumed by the field.
type MultiForm struct {
	fields []FormField
	focus  int
}

func NewMultiForm(fields []FormField) *MultiForm {
	f := &MultiForm{fields: fields}
	if len(fields) > 0 {
		f.fields[0].Focus()
	}
	return f
}

func (f *MultiForm) Len() int                { return len(f.fields) }
func (f *MultiForm) Focus() int              { return f.focus }
func (f *MultiForm) Field(i int) FormField   { return f.fields[i] }
func (f *MultiForm) Value(i int) string      { return f.fields[i].Value() }
func (f *MultiForm) Values() []string {
	out := make([]string, len(f.fields))
	for i, fl := range f.fields {
		out[i] = fl.Value()
	}
	return out
}

func (f *MultiForm) Cycle(delta int) {
	if len(f.fields) == 0 {
		return
	}
	f.fields[f.focus].Blur()
	n := len(f.fields)
	f.focus = (f.focus + delta%n + n) % n
	f.fields[f.focus].Focus()
}

// HandleKey consumes tab/shift-tab/up/down for focus cycling. Returns
// true when the key was a nav key (caller should skip further handling).
func (f *MultiForm) HandleKey(m tea.KeyMsg) bool {
	switch m.String() {
	case "tab", "down":
		f.Cycle(1)
		return true
	case "shift+tab", "up":
		f.Cycle(-1)
		return true
	}
	return false
}

// ForwardToFocused sends msg to the currently focused field. Used for
// character input (text inputs) and for left/right (pickers) that fell
// through HandleKey.
func (f *MultiForm) ForwardToFocused(msg tea.Msg) tea.Cmd {
	if len(f.fields) == 0 {
		return nil
	}
	return f.fields[f.focus].Update(msg)
}
