// Package i18n owns the app's localized strings and the single
// current-language setting. UI code asks for a string by typed Key and
// reads T(key) / Tf(key, args...) / Help(hotkey, key); the package looks
// the value up in the map for the current Lang, falling back to English
// (DefaultLang) when a pt-BR (or future locale) entry is missing.
//
// Adding a new language is a three-file diff: a Lang constant in this
// file, a new map in strings_<code>.go, and entries in the Supported list
// in keys.go. Adding a new string is a two-file diff: a Key constant in
// keys.go and one entry per map.
package i18n

import (
	"fmt"
	"sync"
)

// Lang is a BCP 47-style language tag used throughout the app. Values are
// compared as-is, so keep them lowercase / case-sensitive to match the
// settings DB value.
type Lang string

const (
	LangEN   Lang = "en"
	LangPtBR Lang = "pt-BR"
)

// DefaultLang is the fallback locale — used on fresh install, when the
// setting is unset, and as a backstop for missing translations.
const DefaultLang = LangEN

// Supported enumerates the languages the UI and AI prompts are ready to
// serve. Settings' picker iterates this list; IsSupported gate-keeps the
// value read from the DB.
var Supported = []Lang{LangEN, LangPtBR}

func IsSupported(l Lang) bool {
	for _, s := range Supported {
		if s == l {
			return true
		}
	}
	return false
}

var (
	mu      sync.RWMutex
	current = DefaultLang
)

// SetLang updates the current language. Unknown languages are ignored —
// callers should gate their input through IsSupported first.
func SetLang(l Lang) {
	if !IsSupported(l) {
		return
	}
	mu.Lock()
	current = l
	mu.Unlock()
}

func CurrentLang() Lang {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// T returns the localized string for k in the current language, falling
// back to English, then to the key's stringified form so missing entries
// stay debuggable rather than rendering as empty cells.
func T(k Key) string {
	lang := CurrentLang()
	if v, ok := lookup(lang, k); ok {
		return v
	}
	if lang != DefaultLang {
		if v, ok := lookup(DefaultLang, k); ok {
			return v
		}
	}
	return string(k)
}

// Tf is T plus fmt.Sprintf. Every locale's format string must use the
// same positional verbs (%d, %s, ...) — enforced by placeholderParity in
// the test suite.
func Tf(k Key, args ...any) string {
	return fmt.Sprintf(T(k), args...)
}

// Help renders one entry of a screen's help row: "hotkey label". Hotkey
// letters and symbols stay verbatim in every language (muscle memory); the
// label after the space is translated.
func Help(hotkey string, k Key) string {
	return hotkey + " " + T(k)
}

func lookup(lang Lang, k Key) (string, bool) {
	tbl, ok := tables[lang]
	if !ok {
		return "", false
	}
	v, ok := tbl[k]
	return v, ok
}

// tables is the registry of per-language string maps. Each locale's file
// (strings_en.go, strings_ptbr.go) registers itself in init(). Adding a
// new language means a new strings_<code>.go with the same shape.
var tables = map[Lang]map[Key]string{}

func register(lang Lang, m map[Key]string) {
	tables[lang] = m
}
