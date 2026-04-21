package tui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/store"
)

type editField int

const (
	fType editField = iota
	fLanguage
	fPrompt
	fInitialCode
	fExpected
	fChoices
	fTemplate
)

var blankRe = regexp.MustCompile(`\{\{([^{}]*)\}\}`)

type Edit struct {
	store *store.Store
	card  models.Card
	focus editField

	language textinput.Model

	// inline code editor modal (used for prompt/initial code/expected/template)
	editor       CodeEditor
	editorActive bool
	editingField editField

	// MCQ choice sub-state
	choiceCursor  int
	choiceEditing bool
	choiceInput   textinput.Model
	choiceEditIdx int

	// screen dimensions (from WindowSizeMsg) used to size the inline editor
	w, h int
}

func NewEdit(s *store.Store, card models.Card) *Edit {
	lang := textinput.New()
	lang.CharLimit = 40
	lang.Width = 20
	if card.Language == "" {
		card.Language = "javascript"
	}
	lang.SetValue(card.Language)

	if card.Type == "" {
		card.Type = models.CardCode
	}

	ci := textinput.New()
	ci.CharLimit = 400
	ci.Width = 60

	e := &Edit{
		store:         s,
		card:          card,
		language:      lang,
		choiceInput:   ci,
		choiceEditIdx: -1,
	}
	e.focus = e.visibleFields()[0]
	e.updateFocus()
	return e
}

func (e *Edit) Init() tea.Cmd { return textinput.Blink }

// visibleFields returns the field list for the current card type.
func (e *Edit) visibleFields() []editField {
	switch e.card.Type {
	case models.CardMCQ:
		return []editField{fType, fLanguage, fPrompt, fChoices}
	case models.CardFill:
		return []editField{fType, fLanguage, fPrompt, fTemplate}
	case models.CardCode:
		return []editField{fLanguage, fPrompt, fInitialCode, fExpected}
	default:
		return []editField{fType, fLanguage, fPrompt, fExpected}
	}
}

func (e *Edit) cycleFocus(delta int) {
	fields := e.visibleFields()
	idx := 0
	for i, f := range fields {
		if f == e.focus {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(fields)) % len(fields)
	e.focus = fields[idx]
	e.updateFocus()
}

func (e *Edit) updateFocus() {
	e.language.Blur()
	if e.focus == fLanguage {
		e.language.Focus()
	}
}

func (e *Edit) Update(msg tea.Msg) (Screen, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		e.w = sz.Width
		e.h = sz.Height
		if e.editorActive {
			e.editor = e.editor.SetSize(e.editorWidth(), e.editorHeight())
		}
		return e, nil
	}

	if e.editorActive {
		var cmd tea.Cmd
		e.editor, cmd = e.editor.Update(msg)
		if e.editor.Done() {
			e.commitEditor()
		}
		return e, cmd
	}

	switch m := msg.(type) {
	case tea.KeyMsg:
		if e.choiceEditing {
			return e.updateChoiceEdit(m)
		}
		return e.updateKey(m)
	}

	if e.choiceEditing {
		var cmd tea.Cmd
		e.choiceInput, cmd = e.choiceInput.Update(msg)
		return e, cmd
	}
	if e.focus == fLanguage {
		var cmd tea.Cmd
		e.language, cmd = e.language.Update(msg)
		e.card.Language = e.language.Value()
		return e, cmd
	}
	return e, nil
}

func (e *Edit) commitEditor() {
	val := e.editor.Value()
	switch e.editingField {
	case fPrompt:
		e.card.Prompt = val
	case fInitialCode:
		e.card.InitialCode = val
	case fExpected:
		e.card.ExpectedAnswer = val
	case fTemplate:
		if e.card.BlanksData == nil {
			e.card.BlanksData = &models.BlankData{}
		}
		e.card.BlanksData.Template = val
	}
	e.editorActive = false
}

func (e *Edit) openEditor(field editField, content, lang, title string) tea.Cmd {
	e.editingField = field
	e.editor = NewCodeEditor(title, content, lang, e.editorWidth(), e.editorHeight())
	e.editorActive = true
	return e.editor.Init()
}

// editorWidth / editorHeight return the dimensions passed into
// NewCodeEditor / SetSize. CodeEditor's outer rendered size is (width+2) ×
// height because Border adds 1 cell on each horizontal side, so we subtract
// 2 from the available screen width to keep the box from overflowing.
func (e *Edit) editorWidth() int {
	if e.w <= 0 {
		return 80
	}
	return max(20, e.w-2)
}

func (e *Edit) editorHeight() int {
	if e.h <= 0 {
		return 20
	}
	return max(6, e.h)
}

func (e *Edit) updateKey(m tea.KeyMsg) (Screen, tea.Cmd) {
	switch m.String() {
	case "esc":
		return e, func() tea.Msg { return NavMsg{Pop: true} }
	case "ctrl+s":
		return e, e.save()
	}

	// Choices-focused navigation overrides tab/arrow defaults
	if e.focus == fChoices {
		return e.updateChoicesKey(m)
	}

	switch m.String() {
	case "tab", "down":
		e.cycleFocus(1)
		return e, nil
	case "shift+tab", "up":
		e.cycleFocus(-1)
		return e, nil
	case "enter":
		switch e.focus {
		case fPrompt:
			return e, e.openEditor(fPrompt, e.card.Prompt, "markdown", "Question")
		case fInitialCode:
			return e, e.openEditor(fInitialCode, e.card.InitialCode, e.card.Language, "Initial code")
		case fExpected:
			return e, e.openEditor(fExpected, e.card.ExpectedAnswer, e.card.Language, "Expected answer")
		case fTemplate:
			content := ""
			if e.card.BlanksData != nil {
				content = e.card.BlanksData.Template
			}
			return e, e.openEditor(fTemplate, content, e.card.Language, "Template")
		}
	}

	if e.focus == fType {
		changed := false
		switch m.String() {
		case "1":
			e.card.Type = models.CardCode
			changed = true
		case "2":
			e.card.Type = models.CardMCQ
			changed = true
		case "3":
			e.card.Type = models.CardFill
			changed = true
		case "4":
			e.card.Type = models.CardExp
			changed = true
		}
		if changed {
			e.focus = fType
			e.updateFocus()
			return e, nil
		}
	}

	if e.focus == fLanguage {
		var cmd tea.Cmd
		e.language, cmd = e.language.Update(m)
		e.card.Language = e.language.Value()
		return e, cmd
	}
	return e, nil
}

func (e *Edit) updateChoicesKey(m tea.KeyMsg) (Screen, tea.Cmd) {
	choices := e.card.Choices
	switch m.String() {
	case "tab":
		e.cycleFocus(1)
		return e, nil
	case "shift+tab":
		e.cycleFocus(-1)
		return e, nil
	case "up", "k":
		if e.choiceCursor > 0 {
			e.choiceCursor--
		}
		return e, nil
	case "down", "j":
		if e.choiceCursor < len(choices)-1 {
			e.choiceCursor++
		}
		return e, nil
	case " ", "space":
		if len(choices) == 0 {
			return e, nil
		}
		choices[e.choiceCursor].IsCorrect = !choices[e.choiceCursor].IsCorrect
		e.card.Choices = choices
		return e, nil
	case "a":
		if len(choices) >= 26 {
			return e, ToastErr("max 26 choices")
		}
		choices = append(choices, models.Choice{})
		e.card.Choices = choices
		e.choiceCursor = len(choices) - 1
		e.beginChoiceEdit(e.choiceCursor)
		return e, textinput.Blink
	case "d":
		if len(choices) == 0 {
			return e, nil
		}
		e.card.Choices = append(choices[:e.choiceCursor], choices[e.choiceCursor+1:]...)
		if e.choiceCursor > 0 && e.choiceCursor >= len(e.card.Choices) {
			e.choiceCursor = len(e.card.Choices) - 1
		}
		if e.choiceCursor < 0 {
			e.choiceCursor = 0
		}
		return e, nil
	case "e", "enter":
		if len(choices) == 0 {
			return e, nil
		}
		e.beginChoiceEdit(e.choiceCursor)
		return e, textinput.Blink
	}
	return e, nil
}

func (e *Edit) beginChoiceEdit(idx int) {
	e.choiceEditing = true
	e.choiceEditIdx = idx
	e.choiceInput.SetValue(e.card.Choices[idx].Text)
	e.choiceInput.CursorEnd()
	e.choiceInput.Focus()
}

func (e *Edit) updateChoiceEdit(m tea.KeyMsg) (Screen, tea.Cmd) {
	switch m.String() {
	case "esc":
		e.choiceEditing = false
		e.choiceInput.Blur()
		return e, nil
	case "enter":
		if e.choiceEditIdx >= 0 && e.choiceEditIdx < len(e.card.Choices) {
			e.card.Choices[e.choiceEditIdx].Text = e.choiceInput.Value()
		}
		e.choiceEditing = false
		e.choiceInput.Blur()
		return e, nil
	}
	var cmd tea.Cmd
	e.choiceInput, cmd = e.choiceInput.Update(m)
	return e, cmd
}

func (e *Edit) save() tea.Cmd {
	in := store.CardInput{
		Type:        e.card.Type,
		Language:    strings.TrimSpace(e.card.Language),
		Prompt:      e.card.Prompt,
		InitialCode: e.card.InitialCode,
	}
	if strings.TrimSpace(in.Prompt) == "" {
		return ToastErr("question required")
	}

	switch e.card.Type {
	case models.CardCode, models.CardExp:
		if strings.TrimSpace(e.card.ExpectedAnswer) == "" {
			return ToastErr("expected answer required")
		}
		in.ExpectedAnswer = e.card.ExpectedAnswer

	case models.CardMCQ:
		if len(e.card.Choices) < 2 {
			return ToastErr("add at least 2 choices")
		}
		anyCorrect := false
		for _, ch := range e.card.Choices {
			if ch.IsCorrect {
				anyCorrect = true
				break
			}
		}
		if !anyCorrect {
			return ToastErr("mark at least one choice correct")
		}
		normalized := make([]models.Choice, len(e.card.Choices))
		for i, ch := range e.card.Choices {
			normalized[i] = models.Choice{
				ID:        string(rune('a' + i)),
				Text:      ch.Text,
				IsCorrect: ch.IsCorrect,
			}
		}
		in.Choices = normalized

	case models.CardFill:
		if e.card.BlanksData == nil || strings.TrimSpace(e.card.BlanksData.Template) == "" {
			return ToastErr("template required")
		}
		tmpl := e.card.BlanksData.Template
		matches := blankRe.FindAllStringSubmatch(tmpl, -1)
		if len(matches) == 0 {
			return ToastErr("template needs at least one {{blank}}")
		}
		blanks := make([]string, 0, len(matches))
		for _, mm := range matches {
			blanks = append(blanks, mm[1])
		}
		in.BlanksData = &models.BlankData{Template: tmpl, Blanks: blanks}
	}

	if e.card.ID == 0 {
		cs, err := e.store.BulkCreateCards(e.card.DeckID, []store.CardInput{in})
		if err != nil {
			return ToastErr("save failed: " + err.Error())
		}
		if len(cs) > 0 {
			e.card = cs[0]
		}
		return tea.Batch(Toast("card created"), func() tea.Msg { return NavMsg{Pop: true} })
	}

	if _, err := e.store.UpdateCard(e.card.ID, in); err != nil {
		return ToastErr("update failed: " + err.Error())
	}
	return tea.Batch(Toast("card saved"), func() tea.Msg { return NavMsg{Pop: true} })
}

func (e *Edit) View() string {
	if e.editorActive {
		return e.editor.View()
	}

	label := func(s string, sel bool) string {
		if sel {
			return StyleSelected.Render("▶ " + s)
		}
		return StyleMuted.Render("  " + s)
	}

	title := "New card"
	if e.card.ID > 0 {
		title = fmt.Sprintf("Edit card #%d", e.card.ID)
	}

	typeLine := fmt.Sprintf("%s  %s",
		typeBadge(e.card.Type, true),
		StyleMuted.Render("(1 code · 2 mcq · 3 fill · 4 exp)"),
	)

	var rows []string
	rows = append(rows, StyleTitle.Render(title), "")

	if e.card.Type == models.CardCode {
		// Type is a muted header (not a focusable field) for code cards.
		rows = append(rows,
			StyleMuted.Render("Type"),
			"  "+typeLine,
			"",
			label("Language", e.focus == fLanguage),
			"  "+e.language.View(),
			"",
			label("Question", e.focus == fPrompt),
			previewBox(e.card.Prompt, "(empty — press enter to edit)"),
			"",
			label("Initial code", e.focus == fInitialCode),
			previewBox(e.card.InitialCode, "(empty — press enter to edit)"),
			"",
			label("Expected answer", e.focus == fExpected),
			previewBox(e.card.ExpectedAnswer, "(empty — press enter to edit)"),
			"",
		)
		return lipgloss.JoinVertical(lipgloss.Left, rows...)
	}

	rows = append(rows,
		label("Type", e.focus == fType),
		"  "+typeLine,
		"",
		label("Language", e.focus == fLanguage),
		"  "+e.language.View(),
		"",
		label("Question", e.focus == fPrompt),
		previewBox(e.card.Prompt, "(empty — press enter to open vim)"),
		"",
	)

	switch e.card.Type {
	case models.CardExp:
		rows = append(rows,
			label("Expected answer", e.focus == fExpected),
			previewBox(e.card.ExpectedAnswer, "(empty — press enter to open vim)"),
			"",
		)
	case models.CardMCQ:
		rows = append(rows,
			label("Choices", e.focus == fChoices),
			e.viewChoices(),
			"",
		)
	case models.CardFill:
		tmpl := ""
		if e.card.BlanksData != nil {
			tmpl = e.card.BlanksData.Template
		}
		rows = append(rows,
			label("Template", e.focus == fTemplate),
			previewBox(tmpl, "(empty — press enter to open vim)"),
			StyleMuted.Render("  {{answer}} marks a blank"),
			"",
		)
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (e *Edit) HelpKeys() []string {
	if e.editorActive {
		return []string{"vim keys", "esc save & back"}
	}
	if e.choiceEditing {
		return []string{"type text", "enter commit", "esc cancel"}
	}
	if e.focus == fChoices {
		return []string{"↑/↓ move", "space correct", "a add", "e edit", "d delete", "tab next field", "ctrl+s save", "esc back"}
	}
	if e.card.Type == models.CardCode {
		return []string{"tab cycle", "enter open editor", "ctrl+s save", "esc back"}
	}
	return []string{"tab cycle", "enter edit field", "1-4 type", "ctrl+s save", "esc back"}
}

func (e *Edit) viewChoices() string {
	if len(e.card.Choices) == 0 && !e.choiceEditing {
		return StyleMuted.Render("  (no choices — press a to add)")
	}
	var lines []string
	for i, ch := range e.card.Choices {
		sel := i == e.choiceCursor && e.focus == fChoices
		mark := "[ ]"
		if ch.IsCorrect {
			mark = StyleSuccess.Render("[x]")
		}
		id := string(rune('a' + i))
		text := ch.Text
		if e.choiceEditing && e.choiceEditIdx == i {
			text = e.choiceInput.View()
		} else if text == "" {
			text = StyleMuted.Render("(empty)")
		}
		prefix := "  "
		if sel {
			prefix = StylePrimary.Render("▶ ")
		}
		lines = append(lines, fmt.Sprintf("%s%s %s. %s", prefix, mark, id, text))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func typeBadge(t models.CardType, bold bool) string {
	style := lipgloss.NewStyle().Bold(bold)
	switch t {
	case models.CardCode:
		return style.Foreground(ColorSuccess).Render("code")
	case models.CardMCQ:
		return style.Foreground(ColorAccent).Render("mcq")
	case models.CardFill:
		return style.Foreground(ColorWarn).Render("fill")
	case models.CardExp:
		return style.Foreground(ColorPrimary).Render("exp")
	}
	return string(t)
}

func previewBox(content, placeholder string) string {
	if strings.TrimSpace(content) == "" {
		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(0, 1).
			Foreground(ColorMuted).
			Render(placeholder)
	}
	lines := strings.Split(content, "\n")
	if len(lines) > 10 {
		lines = append(lines[:10], StyleMuted.Render("…"))
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}
