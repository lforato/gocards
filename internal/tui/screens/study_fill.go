package screens

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/tui"
)

func (s *Study) initFillInputs(card *models.Card) {
	if card.BlanksData == nil {
		s.fillInputs = nil
		return
	}
	s.fillInputs = make([]textinput.Model, len(card.BlanksData.Blanks))
	for i := range s.fillInputs {
		ti := textinput.New()
		ti.CharLimit = 100
		ti.Width = 32
		if i == 0 {
			ti.Focus()
		}
		s.fillInputs[i] = ti
	}
	s.fillFocus = 0
}

func (s *Study) handleFillKey(m tea.KeyMsg, card *models.Card) (tui.Screen, tea.Cmd) {
	if len(s.fillInputs) == 0 {
		return s, nil
	}
	if s.fillFocus < 0 || s.fillFocus >= len(s.fillInputs) {
		s.fillFocus = 0
	}
	switch m.String() {
	case "tab", "down":
		s.focusFillInput(cycleFocus(s.fillFocus, 1, len(s.fillInputs)))
	case "shift+tab", "up":
		s.focusFillInput(cycleFocus(s.fillFocus, -1, len(s.fillInputs)))
	case "ctrl+s", "enter":
		return s.submitFill(card)
	default:
		var cmd tea.Cmd
		s.fillInputs[s.fillFocus], cmd = s.fillInputs[s.fillFocus].Update(m)
		return s, cmd
	}
	return s, nil
}

func (s *Study) focusFillInput(next int) {
	s.fillInputs[s.fillFocus].Blur()
	s.fillFocus = next
	s.fillInputs[s.fillFocus].Focus()
}

// submitFill grades the student's filled blanks against the card's canonical
// values. Matching is case-insensitive and trims surrounding whitespace so
// minor typographical differences don't mark a correct answer wrong.
func (s *Study) submitFill(card *models.Card) (tui.Screen, tea.Cmd) {
	if card.BlanksData == nil || len(card.BlanksData.Blanks) != len(s.fillInputs) {
		return s, tui.ToastErr("fill card is malformed — skipping")
	}
	partial := 0
	for i, ti := range s.fillInputs {
		want := strings.TrimSpace(card.BlanksData.Blanks[i])
		got := strings.TrimSpace(ti.Value())
		if strings.EqualFold(got, want) {
			partial++
		}
	}
	grade := gradeFillFromPartial(partial, len(s.fillInputs))
	s.resultGrade = grade
	s.resultNote = fmt.Sprintf("%d / %d blanks correct", partial, len(s.fillInputs))
	s.stage = stageAnswered
	return s, s.recordReview(grade)
}

// gradeFillFromPartial maps the correct-blank count to the 1-5 SRS scale:
// all correct → 5, some correct → 3, none correct → 1.
func gradeFillFromPartial(correct, total int) int {
	switch {
	case correct == total && total > 0:
		return 5
	case correct > 0:
		return 3
	}
	return 1
}

// renderFillTemplate replaces every {{blank}} placeholder with an underscore
// run the same visual width as the blank's name so the template reads like
// "use ____ to declare a constant" at study time.
func renderFillTemplate(s string) string {
	return blankRe.ReplaceAllStringFunc(s, func(match string) string {
		inner := match[2 : len(match)-2]
		return strings.Repeat("_", utf8.RuneCountInString(inner))
	})
}

func (s *Study) viewFill(card *models.Card) string {
	template := ""
	if card.BlanksData != nil {
		template = renderFillTemplate(card.BlanksData.Template)
	}

	rows := []string{
		renderPrompt(card.Prompt),
		"",
		codeBox(template),
		"",
		tui.StyleMuted.Render("blanks:"),
	}
	for i := range s.fillInputs {
		prefix := "  "
		if i == s.fillFocus {
			prefix = tui.StylePrimary.Render("▶ ")
		}
		rows = append(rows, prefix+fmt.Sprintf("%d: ", i+1)+s.fillInputs[i].View())
	}
	rows = append(rows, "")

	if s.stage == stageAnswered {
		rows = append(rows, tui.StylePrimary.Render(fmt.Sprintf("→ %s  (grade %d)", s.resultNote, s.resultGrade)))
		if card.BlanksData != nil {
			rows = append(rows, tui.StyleMuted.Render("answers: "+strings.Join(card.BlanksData.Blanks, ", ")))
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}
