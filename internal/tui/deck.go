package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/store"
)

type deckLoadedMsg struct {
	cards []models.Card
	due   []models.Card
	err   error
}

type DeckView struct {
	store   *store.Store
	deck    models.Deck
	cards   []models.Card
	dueIDs  map[int64]bool
	cursor  int
	loaded  bool
	err     error
	confirmDelete bool
}

func NewDeckView(s *store.Store, d models.Deck) *DeckView {
	return &DeckView{store: s, deck: d, dueIDs: map[int64]bool{}}
}

func (d *DeckView) Init() tea.Cmd { return d.load() }

func (d *DeckView) load() tea.Cmd {
	return func() tea.Msg {
		cs, err := d.store.ListCards(d.deck.ID)
		if err != nil {
			return deckLoadedMsg{err: err}
		}
		due, err := d.store.DueCards(d.deck.ID, d.store.DailyLimit())
		if err != nil {
			return deckLoadedMsg{err: err}
		}
		return deckLoadedMsg{cards: cs, due: due}
	}
}

func (d *DeckView) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case deckLoadedMsg:
		d.loaded = true
		d.err = m.err
		d.cards = m.cards
		d.dueIDs = map[int64]bool{}
		for _, c := range m.due {
			d.dueIDs[c.ID] = true
		}
		if d.cursor >= len(d.cards) {
			d.cursor = max(0, len(d.cards)-1)
		}
		return d, nil

	case tea.KeyMsg:
		if d.confirmDelete {
			switch m.String() {
			case "y", "Y":
				if d.cursor < len(d.cards) {
					id := d.cards[d.cursor].ID
					if err := d.store.DeleteCard(id); err != nil {
						d.confirmDelete = false
						return d, ToastErr("delete failed: " + err.Error())
					}
				}
				d.confirmDelete = false
				return d, tea.Batch(Toast("card deleted"), d.load())
			default:
				d.confirmDelete = false
				return d, nil
			}
		}

		switch m.String() {
		case "esc", "backspace":
			return d, func() tea.Msg { return NavMsg{Pop: true} }
		case "q":
			return d, tea.Quit
		case "up", "k":
			if d.cursor > 0 {
				d.cursor--
			}
		case "down", "j":
			if d.cursor < len(d.cards)-1 {
				d.cursor++
			}
		case "s":
			dueCount := 0
			for _, c := range d.cards {
				if d.dueIDs[c.ID] {
					dueCount++
				}
			}
			if dueCount == 0 {
				return d, ToastErr("nothing due right now")
			}
			return d, func() tea.Msg { return NavMsg{To: NewStudy(d.store, d.deck)} }
		case "n":
			return d, func() tea.Msg { return NavMsg{To: NewCreate(d.store, d.deck.ID)} }
		case "enter", "e":
			if d.cursor < len(d.cards) {
				card := d.cards[d.cursor]
				return d, func() tea.Msg { return NavMsg{To: NewEdit(d.store, card)} }
			}
		case "d", "delete", "x":
			if d.cursor < len(d.cards) {
				d.confirmDelete = true
			}
		case "r":
			d.loaded = false
			return d, d.load()
		}
	}
	return d, nil
}

func (d *DeckView) View() string {
	if !d.loaded {
		return StyleMuted.Render("loading deck…")
	}
	if d.err != nil {
		return StyleDanger.Render("error: " + d.err.Error())
	}

	color := lipgloss.NewStyle().Foreground(lipgloss.Color(d.deck.Color)).Render("●")

	dueCount := 0
	for _, c := range d.cards {
		if d.dueIDs[c.ID] {
			dueCount++
		}
	}
	header := lipgloss.JoinVertical(lipgloss.Left,
		fmt.Sprintf("%s  %s", color, StyleTitle.Render(d.deck.Name)),
		StyleMuted.Render(d.deck.Description),
		StyleMuted.Render(fmt.Sprintf("%s  ·  %s due",
			pluralize(len(d.cards), "card", "cards"),
			StylePrimary.Render(fmt.Sprintf("%d", dueCount)),
		)),
	)

	// card list
	rows := []string{}
	if len(d.cards) == 0 {
		rows = append(rows, StyleMuted.Render("no cards yet — press 'n' to add some"))
	}
	for i, c := range d.cards {
		sel := i == d.cursor
		prefix := "  "
		style := lipgloss.NewStyle().Foreground(ColorFg)
		if sel {
			prefix = StylePrimary.Render("▶ ")
			style = StyleSelected
		}
		due := "  "
		if d.dueIDs[c.ID] {
			due = StylePrimary.Render("● ")
		}
		kind := fmt.Sprintf("[%s]", c.Type)
		kindStyle := StyleMuted
		switch c.Type {
		case models.CardMCQ:
			kindStyle = lipgloss.NewStyle().Foreground(ColorAccent)
		case models.CardCode:
			kindStyle = lipgloss.NewStyle().Foreground(ColorSuccess)
		case models.CardFill:
			kindStyle = lipgloss.NewStyle().Foreground(ColorWarn)
		case models.CardExp:
			kindStyle = lipgloss.NewStyle().Foreground(ColorPrimary)
		}
		line := fmt.Sprintf("%s%s%s  %s  %s",
			prefix,
			due,
			kindStyle.Render(kind),
			StyleMuted.Render(fmt.Sprintf("(%s)", c.Language)),
			style.Render(truncate(flat(c.Prompt), 80)),
		)
		rows = append(rows, line)
	}

	if d.confirmDelete && d.cursor < len(d.cards) {
		return lipgloss.JoinVertical(lipgloss.Left,
			header,
			"",
			lipgloss.JoinVertical(lipgloss.Left, rows...),
			"",
			StyleDanger.Render(fmt.Sprintf("delete card %d? y/N", d.cards[d.cursor].ID)),
		)
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		"",
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
}

func (d *DeckView) HelpKeys() []string {
	if d.confirmDelete && d.cursor < len(d.cards) {
		return []string{"y delete", "N cancel"}
	}
	return []string{"↑/↓ move", "enter edit", "s study", "n new", "d delete", "r reload", "esc back"}
}

func flat(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '\n' || r == '\r' {
			out = append(out, ' ')
			continue
		}
		out = append(out, r)
	}
	return string(out)
}
