package ai

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/lforato/gocards/internal/i18n"
)

//go:embed prompts/*/*.tmpl
var promptFS embed.FS

// templatesByLang holds one parsed template set per supported language.
// The English set is the fallback when renderTemplate can't find a
// localized variant, so every prompt keeps working even if a translation
// lags behind.
var templatesByLang = func() map[i18n.Lang]*template.Template {
	out := make(map[i18n.Lang]*template.Template, len(i18n.Supported))
	for _, lang := range i18n.Supported {
		t := template.Must(
			template.New("prompts-" + string(lang)).
				Funcs(template.FuncMap{
					"add":  func(a, b int) int { return a + b },
					"join": strings.Join,
				}).
				ParseFS(promptFS, "prompts/"+string(lang)+"/*.tmpl"),
		)
		out[lang] = t
	}
	return out
}()

// renderTemplate looks up the named template in the current-language set,
// falling back to the default language when the current locale either
// lacks the whole set (shouldn't happen at build time) or lacks that
// specific file.
func renderTemplate(name string, data any) string {
	lang := i18n.CurrentLang()
	if set, ok := templatesByLang[lang]; ok {
		if out, ok := executeOptional(set, name, data); ok {
			return out
		}
	}
	if lang != i18n.DefaultLang {
		if set, ok := templatesByLang[i18n.DefaultLang]; ok {
			if out, ok := executeOptional(set, name, data); ok {
				return out
			}
		}
	}
	panic(fmt.Errorf("render prompt %s: no template registered for lang %q or fallback", name, lang))
}

func executeOptional(set *template.Template, name string, data any) (string, bool) {
	if set.Lookup(name) == nil {
		return "", false
	}
	var buf bytes.Buffer
	if err := set.ExecuteTemplate(&buf, name, data); err != nil {
		panic(fmt.Errorf("render prompt %s: %w", name, err))
	}
	return buf.String(), true
}

func chatSystem(deckName, deckDescription string) string {
	desc := deckDescription
	if strings.TrimSpace(desc) == "" {
		desc = "(no description)"
	}
	return renderTemplate("chat_system.tmpl", struct {
		DeckName        string
		DeckDescription string
	}{DeckName: deckName, DeckDescription: desc})
}

func generateSystem(preferredLanguages string) string {
	if preferredLanguages == "" {
		preferredLanguages = "javascript, typescript"
	}
	return renderTemplate("generate_system.tmpl", struct {
		PreferredLanguages string
	}{PreferredLanguages: preferredLanguages})
}

// gradeSystem selects between two rubrics: explanation-mode grades
// student-authored comments; code-mode grades a full solution.
func gradeSystem(in GradeInput) string {
	name := "grade_code.tmpl"
	if in.Mode == "explanation" {
		name = "grade_explanation.tmpl"
	}
	return renderTemplate(name, struct {
		Prompt         string
		ExpectedAnswer string
	}{Prompt: in.Prompt, ExpectedAnswer: in.ExpectedAnswer})
}

func cheatsheetSystem() string {
	return renderTemplate("cheatsheet_system.tmpl", nil)
}

func cheatsheetUser(deckName, deckDescription string, cards []CheatsheetCard) string {
	tiers := groupCardsByTier(cards)
	for i := range tiers {
		tiers[i].Tier.Label = localizedTierLabel(tiers[i].Tier.Key)
	}
	return renderTemplate("cheatsheet_user.tmpl", struct {
		DeckName        string
		DeckDescription string
		Tiers           []cheatsheetTierBlock
	}{
		DeckName:        deckName,
		DeckDescription: strings.TrimSpace(deckDescription),
		Tiers:           tiers,
	})
}

// localizedTierLabel resolves the struggle-tier label from i18n so pt-BR
// cheatsheet prompts receive "Difíceis" instead of "Struggling". Keys map
// 1:1 to StruggleTier.Key values.
func localizedTierLabel(key string) string {
	switch key {
	case TierStruggling.Key:
		return i18n.T(i18n.KeyTierStruggling)
	case TierShaky.Key:
		return i18n.T(i18n.KeyTierShaky)
	case TierSolid.Key:
		return i18n.T(i18n.KeyTierSolid)
	case TierNew.Key:
		return i18n.T(i18n.KeyTierNew)
	}
	return key
}

type cheatsheetTierBlock struct {
	Tier  StruggleTier
	Cards []CheatsheetCard
}

// groupCardsByTier preserves the input order (which OrderByStruggle already
// sorted hardest-first) but clusters consecutive cards of the same tier so
// the template can render one heading per tier instead of per card.
func groupCardsByTier(cards []CheatsheetCard) []cheatsheetTierBlock {
	var out []cheatsheetTierBlock
	for _, c := range cards {
		if len(out) == 0 || out[len(out)-1].Tier.Key != c.Tier.Key {
			out = append(out, cheatsheetTierBlock{Tier: c.Tier})
		}
		idx := len(out) - 1
		out[idx].Cards = append(out[idx].Cards, c)
	}
	return out
}
