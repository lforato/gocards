package tui

import (
	"strconv"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/store"
)

type Settings struct {
	store  *store.Store
	fields []textinput.Model
	labels []string
	keys   []string
	focus  int
	masked []bool
}

func NewSettings(s *store.Store) *Settings {
	labels := []string{"Daily review limit", "Preferred languages", "Anthropic API key"}
	keys := []string{"dailyLimit", "preferredLanguages", "apiKey"}
	masked := []bool{false, false, true}

	fields := make([]textinput.Model, 3)
	for i := range fields {
		ti := textinput.New()
		ti.CharLimit = 512
		ti.Width = 60
		if masked[i] {
			ti.EchoMode = textinput.EchoPassword
			ti.EchoCharacter = '•'
		}
		v, _, _ := s.GetSetting(keys[i])
		ti.SetValue(v)
		fields[i] = ti
	}
	fields[0].Focus()

	return &Settings{
		store:  s,
		fields: fields,
		labels: labels,
		keys:   keys,
		focus:  0,
		masked: masked,
	}
}

func (s *Settings) Init() tea.Cmd { return textinput.Blink }

func (s *Settings) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case tea.KeyMsg:
		switch m.String() {
		case "esc":
			return s, func() tea.Msg { return NavMsg{Pop: true} }
		case "tab", "down":
			s.fields[s.focus].Blur()
			s.focus = (s.focus + 1) % len(s.fields)
			s.fields[s.focus].Focus()
			return s, nil
		case "shift+tab", "up":
			s.fields[s.focus].Blur()
			s.focus = (s.focus - 1 + len(s.fields)) % len(s.fields)
			s.fields[s.focus].Focus()
			return s, nil
		case "ctrl+s":
			if err := s.save(); err != nil {
				return s, ToastErr("save failed: " + err.Error())
			}
			return s, Toast("settings saved")
		}
	}

	var cmd tea.Cmd
	s.fields[s.focus], cmd = s.fields[s.focus].Update(msg)
	return s, cmd
}

func (s *Settings) save() error {
	for i, key := range s.keys {
		v := s.fields[i].Value()
		if key == "dailyLimit" {
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				n = 50
			}
			if err := s.store.SetSetting(key, n); err != nil {
				return err
			}
			continue
		}
		if err := s.store.SetSetting(key, v); err != nil {
			return err
		}
	}
	return nil
}

func (s *Settings) View() string {
	rows := []string{StyleTitle.Render("Settings"), ""}
	for i, ti := range s.fields {
		label := StyleMuted.Render(s.labels[i])
		rows = append(rows, label, ti.View(), "")
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (s *Settings) HelpKeys() []string {
	return []string{"tab cycle", "ctrl+s save", "esc back"}
}
