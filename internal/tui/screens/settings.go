package screens

import (
	"strconv"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/i18n"
	"github.com/lforato/gocards/internal/store"
	"github.com/lforato/gocards/internal/tui"
	"github.com/lforato/gocards/internal/tui/widgets"
)

// Setting row identifiers — shared with the store's settings table so the
// same strings can't drift between the form and persistence layer.
const (
	settingKeyDailyLimit         = "dailyLimit"
	settingKeyPreferredLanguages = "preferredLanguages"
	settingKeyAPIKey             = "apiKey"
	settingKeyLanguage           = "language"
)

type settingField struct {
	key    string
	labelK i18n.Key
	kind   settingKind
	masked bool
}

type settingKind int

const (
	settingText settingKind = iota
	settingPicker
)

var settingFields = []settingField{
	{key: settingKeyDailyLimit, labelK: i18n.KeySettingsDailyLimit, kind: settingText},
	{key: settingKeyPreferredLanguages, labelK: i18n.KeySettingsPrefLangs, kind: settingText},
	{key: settingKeyAPIKey, labelK: i18n.KeySettingsAPIKey, kind: settingText, masked: true},
	{key: settingKeyLanguage, labelK: i18n.KeySettingsLanguage, kind: settingPicker},
}

type Settings struct {
	store *store.Store
	form  *widgets.MultiForm
}

func NewSettings(s *store.Store) *Settings {
	fields := make([]widgets.FormField, len(settingFields))
	for i, sf := range settingFields {
		switch sf.kind {
		case settingPicker:
			opts := make([]string, len(i18n.Supported))
			for j, l := range i18n.Supported {
				opts[j] = string(l)
			}
			current, _, _ := s.GetSetting(sf.key)
			if current == "" {
				current = string(i18n.DefaultLang)
			}
			fields[i] = widgets.NewPickerFormField(opts, current)
		default:
			ti := textinput.New()
			ti.CharLimit = 512
			ti.Width = 60
			if sf.masked {
				ti.EchoMode = textinput.EchoPassword
				ti.EchoCharacter = '•'
			}
			v, _, _ := s.GetSetting(sf.key)
			ti.SetValue(v)
			fields[i] = widgets.NewTextFormField(ti)
		}
	}
	return &Settings{store: s, form: widgets.NewMultiForm(fields)}
}

func (s *Settings) Init() tea.Cmd { return textinput.Blink }

func (s *Settings) Update(msg tea.Msg) (tui.Screen, tea.Cmd) {
	if m, ok := msg.(tea.KeyMsg); ok {
		switch m.String() {
		case "esc":
			return s, navBack
		case "ctrl+s":
			changedLang, err := s.save()
			if err != nil {
				return s, tui.ToastErr(i18n.T(i18n.KeySettingsSaveFail) + err.Error())
			}
			cmds := []tea.Cmd{tui.Toast(i18n.T(i18n.KeySettingsSaved))}
			if changedLang {
				cmds = append(cmds, func() tea.Msg { return tui.LangChangedMsg{} })
			}
			return s, tea.Batch(cmds...)
		}
		if s.form.HandleKey(m) {
			return s, nil
		}
	}
	return s, s.form.ForwardToFocused(msg)
}

// save persists every field and, if the language changed, applies it
// immediately via i18n.SetLang so the next View()s already render in the
// new locale. Returns whether the language value changed.
func (s *Settings) save() (langChanged bool, err error) {
	for i, sf := range settingFields {
		raw := s.form.Value(i)
		if sf.key == settingKeyLanguage {
			if changed, err := s.saveLanguage(raw); err != nil {
				return false, err
			} else if changed {
				langChanged = true
			}
			continue
		}
		if err := s.store.SetSetting(sf.key, coerceSettingValue(sf.key, raw)); err != nil {
			return false, err
		}
	}
	return langChanged, nil
}

func (s *Settings) saveLanguage(raw string) (changed bool, err error) {
	prev, _, _ := s.store.GetSetting(settingKeyLanguage)
	if err := s.store.SetSetting(settingKeyLanguage, raw); err != nil {
		return false, err
	}
	i18n.SetLang(i18n.Lang(raw))
	return prev != raw, nil
}

// coerceSettingValue converts the raw string from the form into the setting's
// canonical storage type. Currently only dailyLimit has typed handling; other
// keys are stored verbatim.
func coerceSettingValue(key, raw string) any {
	if key == settingKeyDailyLimit {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n
		}
		return store.DefaultDailyLimit
	}
	return raw
}

func (s *Settings) View() string {
	rows := []string{tui.StyleTitle.Render(i18n.T(i18n.KeySettingsTitle)), ""}
	for i, sf := range settingFields {
		rows = append(rows, tui.StyleMuted.Render(i18n.T(sf.labelK)), s.form.Field(i).View(), "")
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (s *Settings) HelpKeys() []string {
	return []string{
		i18n.Help("tab", i18n.KeyHelpCycle),
		i18n.Help("←/→", i18n.KeyHelpSelect),
		i18n.Help("ctrl+s", i18n.KeyHelpSave),
		i18n.Help("esc", i18n.KeyHelpBack),
	}
}
