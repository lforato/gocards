package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/store"
	"github.com/lforato/gocards/internal/tui"
)

type createStep int

const (
	stepPickDeck createStep = iota
	stepNewDeck
	stepPickType
)

type Create struct {
	store *store.Store

	step createStep

	decks       []models.Deck
	deckCursor  int
	preselected int64

	newName       textinput.Model
	newDesc       textinput.Model
	newColor      textinput.Model
	newFieldFocus int

	targetDeck *models.Deck

	typeCursor int
}

var cardTypes = []models.CardType{
	models.CardCode,
	models.CardMCQ,
	models.CardFill,
	models.CardExp,
}

func NewCreate(s *store.Store, preselectedDeckID int64) *Create {
	nn := textinput.New()
	nn.Placeholder = "deck name"
	nn.CharLimit = 80
	nn.Width = 50

	nd := textinput.New()
	nd.Placeholder = "short description (optional)"
	nd.CharLimit = 200
	nd.Width = 50

	nc := textinput.New()
	nc.Placeholder = "#f59e0b"
	nc.CharLimit = 9
	nc.Width = 12
	nc.SetValue("#f59e0b")

	return &Create{
		store:       s,
		step:        stepPickDeck,
		preselected: preselectedDeckID,
		newName:     nn,
		newDesc:     nd,
		newColor:    nc,
	}
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
		ds, err := c.store.ListDecks()
		return createDecksLoadedMsg{decks: ds, err: err}
	}
}

func (c *Create) Update(msg tea.Msg) (tui.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case createDecksLoadedMsg:
		c.decks = m.decks
		if c.preselected > 0 {
			for _, d := range m.decks {
				if d.ID == c.preselected {
					dd := d
					c.targetDeck = &dd
					c.step = stepPickType
					return c, nil
				}
			}
		}
		return c, nil

	case tea.KeyMsg:
		return c.handleKey(m)
	}

	var cmd tea.Cmd
	if c.step == stepNewDeck {
		switch c.newFieldFocus {
		case 0:
			c.newName, cmd = c.newName.Update(msg)
		case 1:
			c.newDesc, cmd = c.newDesc.Update(msg)
		case 2:
			c.newColor, cmd = c.newColor.Update(msg)
		}
	}
	return c, cmd
}

func (c *Create) handleKey(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	if m.String() == "esc" {
		return c, func() tea.Msg { return tui.NavMsg{Pop: true} }
	}

	switch c.step {
	case stepPickDeck:
		switch m.String() {
		case "up", "k":
			c.deckCursor = cursorUp(c.deckCursor)
		case "down", "j":
			c.deckCursor = cursorDown(c.deckCursor, len(c.decks)+1)
		case "enter":
			if c.deckCursor == len(c.decks) || len(c.decks) == 0 {
				c.step = stepNewDeck
				c.newFieldFocus = 0
				c.newName.Focus()
				c.newDesc.Blur()
				c.newColor.Blur()
				return c, textinput.Blink
			}
			d := c.decks[c.deckCursor]
			c.targetDeck = &d
			c.step = stepPickType
			return c, nil
		}

	case stepNewDeck:
		switch m.String() {
		case "tab", "down":
			c.cycleNewFocus(1)
			return c, nil
		case "shift+tab", "up":
			c.cycleNewFocus(-1)
			return c, nil
		case "enter":
			if c.newName.Value() == "" {
				return c, tui.ToastErr("deck name required")
			}
			color := strings.TrimSpace(c.newColor.Value())
			if color == "" {
				color = "#f59e0b"
			}
			deck, err := c.store.CreateDeck(c.newName.Value(), c.newDesc.Value(), color)
			if err != nil {
				return c, tui.ToastErr("create failed: " + err.Error())
			}
			c.targetDeck = deck
			c.step = stepPickType
			return c, nil
		}
		var cmd tea.Cmd
		switch c.newFieldFocus {
		case 0:
			c.newName, cmd = c.newName.Update(m)
		case 1:
			c.newDesc, cmd = c.newDesc.Update(m)
		case 2:
			c.newColor, cmd = c.newColor.Update(m)
		}
		return c, cmd

	case stepPickType:
		switch m.String() {
		case "up", "k":
			c.typeCursor = cursorUp(c.typeCursor)
		case "down", "j":
			c.typeCursor = cursorDown(c.typeCursor, len(cardTypes))
		case "enter":
			if c.targetDeck == nil {
				return c, nil
			}
			t := cardTypes[c.typeCursor]
			draft := models.Card{
				DeckID:   c.targetDeck.ID,
				Type:     t,
				Language: "javascript",
			}
			return c, func() tea.Msg { return tui.NavMsg{To: NewEdit(c.store, draft)} }
		}
	}
	return c, nil
}

func (c *Create) cycleNewFocus(delta int) {
	fields := []*textinput.Model{&c.newName, &c.newDesc, &c.newColor}
	fields[c.newFieldFocus].Blur()
	c.newFieldFocus = cycleFocus(c.newFieldFocus, delta, len(fields))
	fields[c.newFieldFocus].Focus()
}

// --- view ---

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
	rows := []string{tui.StyleTitle.Render("New card — pick a deck"), ""}
	if len(c.decks) == 0 {
		rows = append(rows, tui.StyleMuted.Render("no decks yet"))
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
	label := "+ new deck"
	if sel {
		label = tui.StyleSelected.Render(label)
	}
	rows = append(rows, selectionPrefix(sel)+label)
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (c *Create) viewNewDeck() string {
	rows := []string{
		tui.StyleTitle.Render("Create deck"), "",
		tui.StyleMuted.Render("Name"), c.newName.View(), "",
		tui.StyleMuted.Render("Description"), c.newDesc.View(), "",
		tui.StyleMuted.Render("Color (#hex)"), c.newColor.View(),
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (c *Create) viewPickType() string {
	title := tui.StyleTitle.Render("New card — pick type")
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
		return []string{"tab cycle", "enter create", "esc back"}
	default:
		return []string{"↑/↓ move", "enter select", "esc back"}
	}
}

func typeDescription(t models.CardType) string {
	switch t {
	case models.CardCode:
		return "write code to solve a problem"
	case models.CardMCQ:
		return "multiple choice question"
	case models.CardFill:
		return "fill in the blanks"
	case models.CardExp:
		return "annotate / explain a code snippet"
	}
	return ""
}
