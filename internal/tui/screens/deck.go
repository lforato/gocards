package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/store"
	"github.com/lforato/gocards/internal/tui"
)

// Rows the deck screen reserves for non-list chrome (header block + blank
// spacer). Delete prompt, when visible, eats two more rows below.
const (
	deckHeaderRows  = 4
	deckPromptRows  = 2
	deckMinListRows = 3
)

type deckLoadedMsg struct {
	cards []models.Card
	due   []models.Card
	err   error
}

type DeckView struct {
	store         *store.Store
	deck          models.Deck
	cards         []models.Card
	dueIDs        map[int64]bool
	cursor        int
	loaded        bool
	err           error
	confirmDelete bool

	viewport viewport.Model
	w, h     int
}

func NewDeckView(s *store.Store, d models.Deck) *DeckView {
	return &DeckView{
		store:    s,
		deck:     d,
		dueIDs:   map[int64]bool{},
		viewport: viewport.New(80, 10),
	}
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

func (d *DeckView) Update(msg tea.Msg) (tui.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		d.w, d.h = m.Width, m.Height
		d.resizeViewport()
		return d, nil
	case deckLoadedMsg:
		d.applyLoaded(m)
		return d, nil
	case tea.KeyMsg:
		return d.handleKey(m)
	}
	return d, nil
}

func (d *DeckView) applyLoaded(m deckLoadedMsg) {
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
	d.resizeViewport()
}

func (d *DeckView) handleKey(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	if d.confirmDelete {
		return d.handleDeleteConfirm(m)
	}
	switch m.String() {
	case "esc", "backspace":
		return d, navBack
	case "q":
		return d, tea.Quit
	case "up", "k":
		d.cursor = cursorUp(d.cursor)
	case "down", "j":
		d.cursor = cursorDown(d.cursor, len(d.cards))
	case "s":
		return d, d.startStudy()
	case "n":
		return d, navTo(NewCreate(d.store, d.deck.ID))
	case "enter", "e":
		if d.cursor < len(d.cards) {
			return d, navTo(NewEdit(d.store, d.cards[d.cursor]))
		}
	case "d", "delete", "x":
		if d.cursor < len(d.cards) {
			d.confirmDelete = true
			d.resizeViewport()
		}
	case "r":
		d.loaded = false
		return d, d.load()
	}
	return d, nil
}

func (d *DeckView) handleDeleteConfirm(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	d.confirmDelete = false
	d.resizeViewport()
	if m.String() != "y" && m.String() != "Y" {
		return d, nil
	}
	if d.cursor >= len(d.cards) {
		return d, nil
	}
	if err := d.store.DeleteCard(d.cards[d.cursor].ID); err != nil {
		return d, tui.ToastErr("delete failed: " + err.Error())
	}
	return d, tea.Batch(tui.Toast("card deleted"), d.load())
}

func (d *DeckView) startStudy() tea.Cmd {
	if d.dueCount() == 0 {
		return tui.ToastErr("nothing due right now")
	}
	return navTo(NewStudy(d.store, d.deck))
}

func (d *DeckView) dueCount() int {
	n := 0
	for _, c := range d.cards {
		if d.dueIDs[c.ID] {
			n++
		}
	}
	return n
}

func (d *DeckView) resizeViewport() {
	if d.w <= 0 {
		return
	}
	reserved := deckHeaderRows
	if d.confirmDelete {
		reserved += deckPromptRows
	}
	d.viewport.Width = d.w
	d.viewport.Height = max(deckMinListRows, d.h-reserved)
}

// scrollToCursor keeps the highlighted card inside the viewport's visible
// window. Each card is exactly one line so cursor index == line index.
func (d *DeckView) scrollToCursor() {
	if d.viewport.Height <= 0 {
		return
	}
	switch {
	case d.cursor < d.viewport.YOffset:
		d.viewport.SetYOffset(d.cursor)
	case d.cursor >= d.viewport.YOffset+d.viewport.Height:
		d.viewport.SetYOffset(d.cursor - d.viewport.Height + 1)
	}
}

func (d *DeckView) View() string {
	if !d.loaded {
		return tui.StyleMuted.Render("loading deck…")
	}
	if d.err != nil {
		return tui.StyleDanger.Render("error: " + d.err.Error())
	}

	d.viewport.SetContent(d.renderCards())
	d.scrollToCursor()

	body := lipgloss.JoinVertical(lipgloss.Left, d.renderHeader(), "", d.viewport.View())
	if d.confirmDelete && d.cursor < len(d.cards) {
		prompt := tui.StyleDanger.Render(fmt.Sprintf("delete card %d? y/N", d.cards[d.cursor].ID))
		return lipgloss.JoinVertical(lipgloss.Left, body, "", prompt)
	}
	return body
}

func (d *DeckView) renderHeader() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		fmt.Sprintf("%s  %s", colorBullet(d.deck.Color), tui.StyleTitle.Render(d.deck.Name)),
		tui.StyleMuted.Render(d.deck.Description),
		tui.StyleMuted.Render(fmt.Sprintf("%s  ·  %s due",
			pluralize(len(d.cards), "card", "cards"),
			tui.StylePrimary.Render(fmt.Sprintf("%d", d.dueCount())),
		)),
	)
}

func (d *DeckView) renderCards() string {
	if len(d.cards) == 0 {
		return tui.StyleMuted.Render("no cards yet — press 'n' to add some")
	}
	rows := make([]string, len(d.cards))
	for i, c := range d.cards {
		rows[i] = d.renderCardRow(c, i == d.cursor)
	}
	return strings.Join(rows, "\n")
}

func (d *DeckView) renderCardRow(c models.Card, selected bool) string {
	style := lipgloss.NewStyle().Foreground(tui.ColorFg)
	if selected {
		style = tui.StyleSelected
	}
	dueMark := "  "
	if d.dueIDs[c.ID] {
		dueMark = tui.StylePrimary.Render("● ")
	}
	return fmt.Sprintf("%s%s%s  %s  %s",
		selectionPrefix(selected),
		dueMark,
		cardTypeBadge(c.Type),
		tui.StyleMuted.Render(fmt.Sprintf("(%s)", c.Language)),
		style.Render(truncate(flat(c.Prompt), 80)),
	)
}

func (d *DeckView) HelpKeys() []string {
	if d.confirmDelete && d.cursor < len(d.cards) {
		return []string{"y delete", "N cancel"}
	}
	return []string{"↑/↓ move", "enter edit", "s study", "n new", "d delete", "r reload", "esc back"}
}

// flat replaces newlines/CRs with spaces so a card prompt fits on one line.
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
