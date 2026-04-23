package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/i18n"
	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/store"
	"github.com/lforato/gocards/internal/tui"
	"github.com/lforato/gocards/internal/tui/widgets"
)

type createStep int

const (
	stepPickDeck createStep = iota
	stepNewDeck
	stepPickType
)

// Field indices into Create.deckForm — the form is fixed at 3 inputs in the
// order Name, Description, Color. Callers use these names instead of bare
// integers so the intent is readable.
const (
	deckFormFieldName = iota
	deckFormFieldDesc
	deckFormFieldColor
)

const defaultDeckColor = "#f59e0b"

type Create struct {
	store *store.Store
	step  createStep

	decks       []models.Deck
	deckCursor  int
	preselected int64

	deckForm *widgets.Form

	targetDeck *models.Deck
	typeCursor int
}

// cardTypes lists the card types in canonical menu order (matches the
// registry order in models.AllKinds). Computed once at package init so the
// per-keystroke Update loops don't re-allocate.
var cardTypes = func() []models.CardType {
	kinds := models.AllKinds()
	out := make([]models.CardType, len(kinds))
	for i, k := range kinds {
		out[i] = k.Type
	}
	return out
}()

func NewCreate(s *store.Store, preselectedDeckID int64) *Create {
	return &Create{
		store:       s,
		step:        stepPickDeck,
		preselected: preselectedDeckID,
		deckForm:    widgets.NewForm(newDeckFormInputs()),
	}
}

func newDeckFormInputs() []textinput.Model {
	name := textinput.New()
	name.Placeholder = i18n.T(i18n.KeyCreateDeckNamePlaceholder)
	name.CharLimit = 80
	name.Width = 50

	desc := textinput.New()
	desc.Placeholder = i18n.T(i18n.KeyCreateDeckDescPlaceholder)
	desc.CharLimit = 200
	desc.Width = 50

	color := textinput.New()
	color.Placeholder = defaultDeckColor
	color.CharLimit = 9
	color.Width = 12
	color.SetValue(defaultDeckColor)

	return []textinput.Model{name, desc, color}
}

func (c *Create) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, c.loadDecks())
}

type createDecksLoadedMsg struct {
	decks []models.Deck
	err   error
}

func (c *Create) loadDecks() tea.Cmd {
	return func() tea.Msg {
		ds, err := c.store.ListDecksByLanguage(string(i18n.CurrentLang()))
		return createDecksLoadedMsg{decks: ds, err: err}
	}
}

func (c *Create) Update(msg tea.Msg) (tui.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case createDecksLoadedMsg:
		c.applyLoadedDecks(m)
		return c, nil
	case tui.LangChangedMsg:
		return c, c.loadDecks()
	case tea.KeyMsg:
		return c.handleKey(m)
	}
	if c.step == stepNewDeck {
		return c, c.deckForm.ForwardToFocused(msg)
	}
	return c, nil
}

func (c *Create) applyLoadedDecks(m createDecksLoadedMsg) {
	c.decks = m.decks
	if c.preselected == 0 {
		return
	}
	for _, d := range m.decks {
		if d.ID == c.preselected {
			selected := d
			c.targetDeck = &selected
			c.step = stepPickType
			return
		}
	}
}

func (c *Create) handleKey(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	if m.String() == "esc" {
		return c, navBack
	}
	switch c.step {
	case stepPickDeck:
		return c.handlePickDeckKey(m)
	case stepNewDeck:
		return c.handleNewDeckKey(m)
	case stepPickType:
		return c.handlePickTypeKey(m)
	}
	return c, nil
}

func (c *Create) handlePickDeckKey(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	switch m.String() {
	case "up", "k":
		c.deckCursor = cursorUp(c.deckCursor)
	case "down", "j":
		c.deckCursor = cursorDown(c.deckCursor, len(c.decks)+1)
	case "enter":
		if c.deckCursor == len(c.decks) || len(c.decks) == 0 {
			c.step = stepNewDeck
			return c, textinput.Blink
		}
		selected := c.decks[c.deckCursor]
		c.targetDeck = &selected
		c.step = stepPickType
	}
	return c, nil
}

func (c *Create) handleNewDeckKey(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	if c.deckForm.HandleKey(m) {
		return c, nil
	}
	if m.String() == "enter" {
		return c.submitNewDeck()
	}
	return c, c.deckForm.ForwardToFocused(m)
}

func (c *Create) submitNewDeck() (tui.Screen, tea.Cmd) {
	name := c.deckForm.Value(deckFormFieldName)
	if name == "" {
		return c, tui.ToastErr(i18n.T(i18n.KeyCreateValidationName))
	}
	color := strings.TrimSpace(c.deckForm.Value(deckFormFieldColor))
	if color == "" {
		color = defaultDeckColor
	}
	description := c.deckForm.Value(deckFormFieldDesc)
	deck, err := c.store.CreateDeck(name, description, color, string(i18n.CurrentLang()))
	if err != nil {
		return c, tui.ToastErr(i18n.T(i18n.KeyCreateFailedPrefix) + err.Error())
	}
	c.targetDeck = deck
	c.step = stepPickType
	return c, nil
}

func (c *Create) handlePickTypeKey(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	switch m.String() {
	case "up", "k":
		c.typeCursor = cursorUp(c.typeCursor)
	case "down", "j":
		c.typeCursor = cursorDown(c.typeCursor, len(cardTypes))
	case "enter":
		if c.targetDeck == nil {
			return c, nil
		}
		draft := models.Card{
			DeckID:   c.targetDeck.ID,
			Type:     cardTypes[c.typeCursor],
			Language: "javascript",
		}
		return c, navTo(NewEdit(c.store, draft))
	}
	return c, nil
}

func (c *Create) View() string {
	switch c.step {
	case stepPickDeck:
		return c.viewPickDeck()
	case stepNewDeck:
		return c.viewNewDeck()
	case stepPickType:
		return c.viewPickType()
	}
	return ""
}

func (c *Create) viewPickDeck() string {
	rows := []string{tui.StyleTitle.Render(i18n.T(i18n.KeyCreatePickDeckTitle)), ""}
	if len(c.decks) == 0 {
		rows = append(rows, tui.StyleMuted.Render(i18n.T(i18n.KeyCreateNoDecks)))
	}
	for i, d := range c.decks {
		sel := i == c.deckCursor
		name := d.Name
		if sel {
			name = tui.StyleSelected.Render(name)
		}
		rows = append(rows, fmt.Sprintf("%s%s  %s", selectionPrefix(sel), colorBullet(d.Color), name))
	}
	sel := c.deckCursor == len(c.decks)
	label := i18n.T(i18n.KeyCreateNewDeckAction)
	if sel {
		label = tui.StyleSelected.Render(label)
	}
	rows = append(rows, selectionPrefix(sel)+label)
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (c *Create) viewNewDeck() string {
	rows := []string{tui.StyleTitle.Render(i18n.T(i18n.KeyCreateDeckTitle)), ""}
	labels := []string{
		i18n.T(i18n.KeyCreateFormName),
		i18n.T(i18n.KeyCreateFormDesc),
		i18n.T(i18n.KeyCreateFormColor),
	}
	for i, label := range labels {
		rows = append(rows, tui.StyleMuted.Render(label), c.deckForm.Input(i).View(), "")
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (c *Create) viewPickType() string {
	title := tui.StyleTitle.Render(i18n.T(i18n.KeyCreatePickTypeTitle))
	if c.targetDeck != nil {
		title += "  " + tui.StyleMuted.Render("→ "+c.targetDeck.Name)
	}
	rows := []string{title, ""}
	for i, t := range cardTypes {
		sel := i == c.typeCursor
		rows = append(rows, selectionPrefix(sel)+typeBadge(t, sel)+"  "+tui.StyleMuted.Render(typeDescription(t)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (c *Create) HelpKeys() []string {
	switch c.step {
	case stepNewDeck:
		return []string{
			i18n.Help("tab", i18n.KeyHelpCycle),
			i18n.Help("enter", i18n.KeyHelpNew),
			i18n.Help("esc", i18n.KeyHelpBack),
		}
	default:
		return []string{
			i18n.Help("↑/↓", i18n.KeyHelpMove),
			i18n.Help("enter", i18n.KeyHelpSelect),
			i18n.Help("esc", i18n.KeyHelpBack),
		}
	}
}

func typeDescription(t models.CardType) string {
	return models.Kind(t).Description
}
