package screens

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
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

	viewport viewport.Model
	w, h     int
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
		viewport:      viewport.New(80, 10),
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
		e.resizeViewport()
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


const editViewportFallbackH = 20

func (e *Edit) resizeViewport() {
	w := e.w
	if w <= 0 {
		w = previewBoxFallbackW
	}
	h := e.h
	if h <= 0 {
		h = editViewportFallbackH
	}
	e.viewport.Width = w
	e.viewport.Height = max(3, h)
}

// scrollToField ensures the focused field's label is visible. If the field
// sits above the current window, scroll up so the label is at the top;
// if it sits below, scroll down so the label is at the top. This keeps
// labels anchored predictably as the user tabs through tall forms.
func (e *Edit) scrollToField(fieldTop map[editField]int) {
	top, ok := fieldTop[e.focus]
	if !ok || e.viewport.Height <= 0 {
		return
	}
	if top < e.viewport.YOffset || top >= e.viewport.YOffset+e.viewport.Height {
		e.viewport.SetYOffset(top)
	}
}

// editView accumulates rendered lines and records each focusable field's
// starting line so the viewport can scroll to it on focus change.
type editView struct {
	lines    []string
	fieldTop map[editField]int
}

func newEditView() *editView { return &editView{fieldTop: map[editField]int{}} }

func (v *editView) add(lines ...string)   { v.lines = append(v.lines, lines...) }
func (v *editView) blank()                { v.lines = append(v.lines, "") }
func (v *editView) field(f editField, lines ...string) {
	v.fieldTop[f] = len(v.lines)
	v.lines = append(v.lines, lines...)
}
func (v *editView) render() string { return strings.Join(v.lines, "\n") }

func (e *Edit) View() string {
	if e.editorActive {
		return e.editor.View()
	}

	e.resizeViewport()
	v := newEditView()
	e.buildView(v)
	e.viewport.SetContent(v.render())
	e.scrollToField(v.fieldTop)
	return e.viewport.View()
}

func (e *Edit) buildView(v *editView) {
	v.add(tui.StyleTitle.Render(e.title()), "")
	e.addTypeSection(v)
	v.blank()
	e.addLanguageSection(v)
	v.blank()
	e.addPromptSection(v)
	v.blank()
	e.addTypeSpecificSections(v)
}

func (e *Edit) title() string {
	if e.card.ID > 0 {
		return fmt.Sprintf("Edit card #%d", e.card.ID)
	}
	return "New card"
}

func (e *Edit) fieldLabel(text string, field editField) string {
	if e.focus == field {
		return tui.StyleSelected.Render("▶ " + text)
	}
	return tui.StyleMuted.Render("  " + text)
}

func (e *Edit) typeLine() string {
	return fmt.Sprintf("%s  %s",
		typeBadge(e.card.Type, true),
		tui.StyleMuted.Render("(1 code · 2 mcq · 3 fill · 4 exp)"),
	)
}

// addTypeSection renders the Type row. For code cards the row is a muted
// header (type is locked to code), for others it's a focusable field that
// accepts 1-4 to switch types.
func (e *Edit) addTypeSection(v *editView) {
	if e.card.Type == models.CardCode {
		v.add(tui.StyleMuted.Render("Type"), "  "+e.typeLine())
		return
	}
	v.field(fType, e.fieldLabel("Type", fType), "  "+e.typeLine())
}

func (e *Edit) addLanguageSection(v *editView) {
	v.field(fLanguage, e.fieldLabel("Language", fLanguage), "  "+e.language.View())
}

func (e *Edit) addPromptSection(v *editView) {
	v.field(fPrompt, e.fieldLabel("Question", fPrompt),
		e.previewBox(e.card.Prompt, placeholderFor(e.card.Type)))
}

func (e *Edit) addTypeSpecificSections(v *editView) {
	switch e.card.Type {
	case models.CardCode:
		v.field(fInitialCode, e.fieldLabel("Initial code", fInitialCode),
			e.previewBox(e.card.InitialCode, "(empty — press enter to edit)"))
		v.blank()
		v.field(fExpected, e.fieldLabel("Expected answer", fExpected),
			e.previewBox(e.card.ExpectedAnswer, "(empty — press enter to edit)"))
		v.blank()
	case models.CardExp:
		v.field(fExpected, e.fieldLabel("Expected answer", fExpected),
			e.previewBox(e.card.ExpectedAnswer, "(empty — press enter to open vim)"))
		v.blank()
	case models.CardMCQ:
		v.field(fChoices, e.fieldLabel("Choices", fChoices), e.viewChoices())
		v.blank()
	case models.CardFill:
		tmpl := ""
		if e.card.BlanksData != nil {
			tmpl = e.card.BlanksData.Template
		}
		v.field(fTemplate, e.fieldLabel("Template", fTemplate),
			e.previewBox(tmpl, "(empty — press enter to open vim)"),
			tui.StyleMuted.Render("  {{answer}} marks a blank"))
		v.blank()
	}
}

func placeholderFor(t models.CardType) string {
	if t == models.CardCode {
		return "(empty — press enter to edit)"
	}
	return "(empty — press enter to open vim)"
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

const (
	previewBoxMaxLines  = 10
	previewBoxFallbackW = 80
	previewBoxPadding   = 2 // 1 cell each side
	previewBoxBorder    = 2 // 1 cell each side
)

// previewBox renders content inside a rounded border sized to the screen.
// An explicit inner width is mandatory — without it, lipgloss draws a
// jagged border around uneven-length lines.
func (e *Edit) previewBox(content, placeholder string) string {
	innerW := e.previewInnerWidth()

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorBorder).
		Padding(0, 1).
		Width(innerW)

	if strings.TrimSpace(content) == "" {
		return style.Foreground(tui.ColorMuted).Render(placeholder)
	}

	lines := strings.Split(content, "\n")
	if len(lines) > previewBoxMaxLines {
		lines = append(lines[:previewBoxMaxLines], tui.StyleMuted.Render("…"))
	}
	return style.Render(strings.Join(lines, "\n"))
}

func (e *Edit) previewInnerWidth() int {
	w := e.w
	if w <= 0 {
		w = previewBoxFallbackW
	}
	return max(10, w-previewBoxBorder-previewBoxPadding)
}
