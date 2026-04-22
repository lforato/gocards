package screens

import (
	"strconv"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/store"
	"github.com/lforato/gocards/internal/tui"
	"github.com/lforato/gocards/internal/tui/widgets"
)

type settingField struct {
	key    string
	label  string
	masked bool
}

var settingFields = []settingField{
	{key: "dailyLimit", label: "Daily review limit"},
	{key: "preferredLanguages", label: "Preferred languages"},
	{key: "apiKey", label: "Anthropic API key", masked: true},
}

type Settings struct {
	store *store.Store
	form  *widgets.Form
}

func NewSettings(s *store.Store) *Settings {
	inputs := make([]textinput.Model, len(settingFields))
	for i, sf := range settingFields {
		ti := textinput.New()
		ti.CharLimit = 512
		ti.Width = 60
		if sf.masked {
			ti.EchoMode = textinput.EchoPassword
			ti.EchoCharacter = '•'
		}
		v, _, _ := s.GetSetting(sf.key)
		ti.SetValue(v)
		inputs[i] = ti
	}
	return &Settings{store: s, form: widgets.NewForm(inputs)}
}

func (s *Settings) Init() tea.Cmd { return textinput.Blink }

func (s *Settings) Update(msg tea.Msg) (tui.Screen, tea.Cmd) {
	if m, ok := msg.(tea.KeyMsg); ok {
		switch m.String() {
		case "esc":
			return s, navBack
		case "ctrl+s":
			if err := s.save(); err != nil {
				return s, tui.ToastErr("save failed: " + err.Error())
			}
			return s, tui.Toast("settings saved")
		}
		if s.form.HandleKey(m) {
			return s, nil
		}
	}
	return s, s.form.ForwardToFocused(msg)
}

func (s *Settings) save() error {
	for i, sf := range settingFields {
		if err := s.store.SetSetting(sf.key, parseSettingValue(sf.key, s.form.Value(i))); err != nil {
			return err
		}
	}
	return nil
}

// parseSettingValue coerces the raw string from the form into the setting's
// canonical type. Currently only dailyLimit has typed handling.
func parseSettingValue(key, raw string) any {
	if key == "dailyLimit" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n
		}
		return store.DefaultDailyLimit
	}
	return raw
}

func (s *Settings) View() string {
	rows := []string{tui.StyleTitle.Render("Settings"), ""}
	for i, sf := range settingFields {
		rows = append(rows, tui.StyleMuted.Render(sf.label), s.form.Input(i).View(), "")
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (s *Settings) HelpKeys() []string {
	return []string{"tab cycle", "ctrl+s save", "esc back"}
}
