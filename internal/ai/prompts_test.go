package ai

import (
	"strings"
	"testing"

	"github.com/lforato/gocards/internal/i18n"
	"github.com/lforato/gocards/internal/models"
)

// Each render call panics on a broken template, so the asserts below double
// as parse/execute smoke tests for every .tmpl file in prompts/.
func TestPromptTemplatesRender(t *testing.T) {
	if out := chatSystem("OpenGL", "opengl cards"); !strings.Contains(out, "OpenGL") {
		t.Errorf("chatSystem: deck name not interpolated")
	}
	if out := chatSystem("OpenGL", ""); !strings.Contains(out, "(no description)") {
		t.Errorf("chatSystem: empty description fallback missing")
	}
	if out := generateSystem(""); !strings.Contains(out, "javascript") {
		t.Errorf("generateSystem: default language fallback missing")
	}
	if out := gradeSystem(GradeInput{Prompt: "p", ExpectedAnswer: "a"}); !strings.Contains(out, "FINAL_GRADE") {
		t.Errorf("gradeSystem (code): expected grade tokens")
	}
	if out := gradeSystem(GradeInput{Prompt: "p", ExpectedAnswer: "a", Mode: "explanation"}); !strings.Contains(out, "INLINE COMMENTS") {
		t.Errorf("gradeSystem (explanation): expected rubric marker")
	}
	if out := cheatsheetSystem(); !strings.Contains(out, "Markdown") {
		t.Errorf("cheatsheetSystem: expected markdown keyword")
	}

	cards := []CheatsheetCard{
		{Card: models.Card{Type: models.CardCode, Language: "go", Prompt: "q1", ExpectedAnswer: "a1"}, Tier: TierStruggling},
		{Card: models.Card{Type: models.CardMCQ, Prompt: "q2", Choices: []models.Choice{{Text: "yes", IsCorrect: true}, {Text: "no"}}}, Tier: TierShaky},
		{Card: models.Card{Type: models.CardFill, Prompt: "q3", BlanksData: &models.BlankData{Template: "t", Blanks: []string{"x", "y"}}}, Tier: TierNew},
	}
	out := cheatsheetUser("Deck", "desc", cards)
	for _, want := range []string{"Deck", "q1", "q2", "yes", "x, y", "Struggling", "Shaky", "New"} {
		if !strings.Contains(out, want) {
			t.Errorf("cheatsheetUser: missing %q in output", want)
		}
	}
}

// TestPromptTemplatesPtBR asserts every prompt file resolves under the
// pt-BR locale and that the machine-parseable FINAL_GRADE / VERDICT
// tokens in the grader prompts stay exactly as the study screen's regex
// expects across languages.
func TestPromptTemplatesPtBR(t *testing.T) {
	i18n.SetLang(i18n.LangPtBR)
	defer i18n.SetLang(i18n.DefaultLang)

	if out := chatSystem("OpenGL", "baralho opengl"); !strings.Contains(out, "OpenGL") {
		t.Errorf("pt-BR chatSystem: deck name not interpolated")
	}
	if out := generateSystem("javascript"); !strings.Contains(out, "javascript") {
		t.Errorf("pt-BR generateSystem: language not interpolated")
	}

	codeOut := gradeSystem(GradeInput{Prompt: "p", ExpectedAnswer: "a"})
	if !strings.Contains(codeOut, "FINAL_GRADE") || !strings.Contains(codeOut, "VERDICT") {
		t.Errorf("pt-BR grade_code: FINAL_GRADE/VERDICT tokens must stay verbatim")
	}

	expOut := gradeSystem(GradeInput{Prompt: "p", ExpectedAnswer: "a", Mode: "explanation"})
	if !strings.Contains(expOut, "FINAL_GRADE") || !strings.Contains(expOut, "VERDICT") {
		t.Errorf("pt-BR grade_explanation: FINAL_GRADE/VERDICT tokens must stay verbatim")
	}

	if out := cheatsheetSystem(); out == "" {
		t.Errorf("pt-BR cheatsheetSystem: empty output")
	}
}
