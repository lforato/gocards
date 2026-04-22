package screens

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/store"
	"github.com/lforato/gocards/internal/tui"
	"github.com/lforato/gocards/internal/tui/widgets"
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

	editor       widgets.CodeEditor
	editorActive bool
	editingField editField

	choiceCursor  int
	choiceEditing bool
	choiceInput   textinput.Model
	choiceEditIdx int

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

func (e *Edit) visibleFields() []editField {
	return ui(e.card.Type).EditFields
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
	e.focus = fields[cycleFocus(idx, delta, len(fields))]
	e.updateFocus()
}

func (e *Edit) updateFocus() {
	e.language.Blur()
	if e.focus == fLanguage {
		e.language.Focus()
	}
}

func (e *Edit) Update(msg tea.Msg) (tui.Screen, tea.Cmd) {
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
	e.editor = widgets.NewCodeEditor(title, content, lang, e.editorWidth(), e.editorHeight())
	e.editorActive = true
	return e.editor.Init()
}

// CodeEditor's border adds 1 cell on each side, so subtract 2 from the
// available width to keep it from overflowing.
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

func (e *Edit) updateKey(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	switch m.String() {
	case "esc":
		return e, navBack
	case "ctrl+s":
		return e, e.save()
	}

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


func (e *Edit) View() string {
	if e.editorActive {
		return e.editor.View()
	}

	label := func(s string, sel bool) string {
		if sel {
			return tui.StyleSelected.Render("▶ " + s)
		}
		return tui.StyleMuted.Render("  " + s)
	}

	title := "New card"
	if e.card.ID > 0 {
		title = fmt.Sprintf("Edit card #%d", e.card.ID)
	}

	typeLine := fmt.Sprintf("%s  %s",
		typeBadge(e.card.Type, true),
		tui.StyleMuted.Render("(1 code · 2 mcq · 3 fill · 4 exp)"),
	)

	var rows []string
	rows = append(rows, tui.StyleTitle.Render(title), "")

	if e.card.Type == models.CardCode {
		rows = append(rows,
			tui.StyleMuted.Render("Type"),
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
			tui.StyleMuted.Render("  {{answer}} marks a blank"),
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

func previewBox(content, placeholder string) string {
	if strings.TrimSpace(content) == "" {
		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tui.ColorBorder).
			Padding(0, 1).
			Foreground(tui.ColorMuted).
			Render(placeholder)
	}
	lines := strings.Split(content, "\n")
	if len(lines) > 10 {
		lines = append(lines[:10], tui.StyleMuted.Render("…"))
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorBorder).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}
