package widgets

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Form manages focus cycling and key forwarding across a slice of
// textinput.Model fields. Callers own the labels and layout; Form only owns
// focus state and message routing.
type Form struct {
	inputs []textinput.Model
	focus  int
}

func NewForm(inputs []textinput.Model) *Form {
	f := &Form{inputs: inputs}
	if len(inputs) > 0 {
		f.inputs[0].Focus()
	}
	return f
}

func (f *Form) Len() int                     { return len(f.inputs) }
func (f *Form) Focus() int                   { return f.focus }
func (f *Form) Input(i int) *textinput.Model { return &f.inputs[i] }
func (f *Form) Value(i int) string           { return f.inputs[i].Value() }
func (f *Form) Values() []string {
	out := make([]string, len(f.inputs))
	for i, ti := range f.inputs {
		out[i] = ti.Value()
	}
	return out
}

// Cycle moves focus by delta, wrapping at both ends, and transfers the
// underlying textinput focus state.
func (f *Form) Cycle(delta int) {
	if len(f.inputs) == 0 {
		return
	}
	f.inputs[f.focus].Blur()
	n := len(f.inputs)
	f.focus = (f.focus + delta%n + n) % n
	f.inputs[f.focus].Focus()
}

// HandleKey consumes tab/shift-tab/up/down navigation. Returns true when the
// key was a nav key (screen should skip further handling).
func (f *Form) HandleKey(m tea.KeyMsg) bool {
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

// ForwardToFocused sends msg to the currently focused textinput. Used for
// character input that fell through Form.HandleKey.
func (f *Form) ForwardToFocused(msg tea.Msg) tea.Cmd {
	if len(f.inputs) == 0 {
		return nil
	}
	var cmd tea.Cmd
	f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
	return cmd
}
