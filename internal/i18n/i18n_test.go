package i18n

import (
	"reflect"
	"regexp"
	"testing"
)

// TestPlaceholderParity asserts that every translated value uses the same
// fmt verb sequence as the English canonical — mismatched verbs would
// make Tf panic or silently corrupt rendered strings. Verifies using a
// regex over every value in every non-default table.
func TestPlaceholderParity(t *testing.T) {
	verbRe := regexp.MustCompile(`%[#+\- 0]?\d*(?:\.\d+)?[dsqvTtfgebcoxXUp]`)
	extractVerbs := func(s string) []string {
		m := verbRe.FindAllString(s, -1)
		if m == nil {
			return nil
		}
		return m
	}

	en := tables[LangEN]
	if en == nil {
		t.Fatal("English table not registered")
	}

	for lang, tbl := range tables {
		if lang == LangEN {
			continue
		}
		for k, enVal := range en {
			tVal, ok := tbl[k]
			if !ok {
				continue // missing is handled by fallback; separate test covers it
			}
			enVerbs := extractVerbs(enVal)
			tVerbs := extractVerbs(tVal)
			if !reflect.DeepEqual(enVerbs, tVerbs) {
				t.Errorf("lang=%s key=%s: verb mismatch\n  en: %q %v\n  %s: %q %v",
					lang, k, enVal, enVerbs, lang, tVal, tVerbs)
			}
		}
	}
}

// TestFallbackToEnglish asserts T() for a key present in English but
// missing in another language returns the English value, not an empty
// string or the raw key slug.
func TestFallbackToEnglish(t *testing.T) {
	const synthetic Key = "test.synthetic_en_only"
	oldEN := stringsEN[synthetic]
	stringsEN[synthetic] = "en-value"
	defer func() {
		if oldEN == "" {
			delete(stringsEN, synthetic)
		} else {
			stringsEN[synthetic] = oldEN
		}
	}()

	SetLang(LangPtBR)
	defer SetLang(DefaultLang)

	if got := T(synthetic); got != "en-value" {
		t.Fatalf("fallback failed: got %q, want %q", got, "en-value")
	}
}

// TestUnknownKeyReturnsSlug asserts a key missing from every table falls
// through to its stringified value rather than returning empty.
func TestUnknownKeyReturnsSlug(t *testing.T) {
	const synthetic Key = "test.never_registered"
	SetLang(LangEN)
	if got := T(synthetic); got != string(synthetic) {
		t.Fatalf("unknown key: got %q, want %q", got, string(synthetic))
	}
}

// TestIsSupportedRejectsUnknown asserts SetLang ignores unsupported langs
// so a bad DB value can't poison the package state.
func TestIsSupportedRejectsUnknown(t *testing.T) {
	SetLang(LangEN)
	SetLang(Lang("klingon"))
	if CurrentLang() != LangEN {
		t.Fatalf("unsupported lang was accepted; current=%q", CurrentLang())
	}
}
