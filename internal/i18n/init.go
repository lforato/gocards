package i18n

// Loader is the minimal read-only surface i18n needs from the settings
// store — avoids a hard import on store and keeps the package test-friendly.
type Loader interface {
	GetSetting(key string) (string, bool, error)
}

// SettingKey is the settings row i18n reads on startup. Kept in i18n
// rather than store so it stays with the rest of the language plumbing.
const SettingKey = "language"

// LoadFromStore reads the persisted language setting and calls SetLang.
// Unsupported or missing values silently fall back to DefaultLang; an
// error from the loader itself is returned to the caller so main can
// decide whether to panic or continue with English.
func LoadFromStore(l Loader) error {
	v, ok, err := l.GetSetting(SettingKey)
	if err != nil {
		return err
	}
	if !ok {
		SetLang(DefaultLang)
		return nil
	}
	lang := Lang(v)
	if !IsSupported(lang) {
		SetLang(DefaultLang)
		return nil
	}
	SetLang(lang)
	return nil
}
