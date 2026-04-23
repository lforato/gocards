package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/i18n"
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
	selected      map[int64]bool
	bulkQueue     []int64
	cursor        int
	loaded        bool
	err           error
	confirmDelete bool

	cardLineStarts []int
	cardLineHeight []int

	viewport viewport.Model
	w, h     int
}

func NewDeckView(s *store.Store, d models.Deck) *DeckView {
	return &DeckView{
		store:    s,
		deck:     d,
		dueIDs:   map[int64]bool{},
		selected: map[int64]bool{},
		viewport: viewport.New(80, 10),
	}
}

func (d *DeckView) Init() tea.Cmd {
	if next, ok := d.popBulkQueue(); ok {
		return tea.Batch(d.load(), navTo(NewEdit(d.store, next)))
	}
	return d.load()
}

func (d *DeckView) popBulkQueue() (models.Card, bool) {
	for len(d.bulkQueue) > 0 {
		id := d.bulkQueue[0]
		d.bulkQueue = d.bulkQueue[1:]
		for _, c := range d.cards {
			if c.ID == id {
				return c, true
			}
		}
	}
	return models.Card{}, false
}

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
	present := map[int64]bool{}
	for _, c := range m.cards {
		present[c.ID] = true
	}
	for id := range d.selected {
		if !present[id] {
			delete(d.selected, id)
		}
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
		if len(d.selected) > 0 {
			d.selected = map[int64]bool{}
			return d, nil
		}
		return d, navBack
	case "q":
		return d, tea.Quit
	case "up", "k":
		d.cursor = cursorUp(d.cursor)
	case "down", "j":
		d.cursor = cursorDown(d.cursor, len(d.cards))
	case " ":
		if d.cursor < len(d.cards) {
			id := d.cards[d.cursor].ID
			if d.selected[id] {
				delete(d.selected, id)
			} else {
				d.selected[id] = true
			}
		}
	case "a":
		d.selectAll()
	case "s":
		return d, d.startStudy()
	case "c":
		return d, navTo(NewCheatsheetView(d.store, d.deck))
	case "n":
		return d, navTo(NewCreate(d.store, d.deck.ID))
	case "enter", "e":
		return d, d.startEdit()
	case "d", "delete", "x":
		if d.hasTargets() {
			d.confirmDelete = true
			d.resizeViewport()
		}
	case "r":
		d.loaded = false
		return d, d.load()
	}
	return d, nil
}

func (d *DeckView) selectAll() {
	if len(d.selected) == len(d.cards) && len(d.cards) > 0 {
		d.selected = map[int64]bool{}
		return
	}
	d.selected = map[int64]bool{}
	for _, c := range d.cards {
		d.selected[c.ID] = true
	}
}

func (d *DeckView) hasTargets() bool {
	if len(d.selected) > 0 {
		return true
	}
	return d.cursor < len(d.cards)
}

// targetIDs returns selection (in card order) if any, else the cursor card.
func (d *DeckView) targetIDs() []int64 {
	if len(d.selected) > 0 {
		ids := make([]int64, 0, len(d.selected))
		for _, c := range d.cards {
			if d.selected[c.ID] {
				ids = append(ids, c.ID)
			}
		}
		return ids
	}
	if d.cursor < len(d.cards) {
		return []int64{d.cards[d.cursor].ID}
	}
	return nil
}

func (d *DeckView) startEdit() tea.Cmd {
	ids := d.targetIDs()
	if len(ids) == 0 {
		return nil
	}
	first := ids[0]
	d.bulkQueue = ids[1:]
	d.selected = map[int64]bool{}
	for _, c := range d.cards {
		if c.ID == first {
			return navTo(NewEdit(d.store, c))
		}
	}
	return nil
}

func (d *DeckView) handleDeleteConfirm(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	d.confirmDelete = false
	d.resizeViewport()
	if m.String() != "y" && m.String() != "Y" {
		return d, nil
	}
	ids := d.targetIDs()
	if len(ids) == 0 {
		return d, nil
	}
	var failed int
	for _, id := range ids {
		if err := d.store.DeleteCard(id); err != nil {
			failed++
		}
	}
	d.selected = map[int64]bool{}
	if failed > 0 {
		return d, tea.Batch(tui.ToastErr(fmt.Sprintf("%d of %d deletes failed", failed, len(ids))), d.load())
	}
	msg := "card deleted"
	if len(ids) > 1 {
		msg = fmt.Sprintf("%d cards deleted", len(ids))
	}
	return d, tea.Batch(tui.Toast(msg), d.load())
}

func (d *DeckView) startStudy() tea.Cmd {
	if d.dueCount() == 0 {
		return tui.ToastErr(i18n.T(i18n.KeyDeckNothingDue))
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
// window. Rows can span multiple lines, so we consult cardLineStarts/Height
// (populated by renderCards) rather than assuming index == line.
func (d *DeckView) scrollToCursor() {
	if d.viewport.Height <= 0 || d.cursor >= len(d.cardLineStarts) {
		return
	}
	top := d.cardLineStarts[d.cursor]
	height := 1
	if d.cursor < len(d.cardLineHeight) {
		height = d.cardLineHeight[d.cursor]
	}
	bottom := top + height - 1
	switch {
	case top < d.viewport.YOffset:
		d.viewport.SetYOffset(top)
	case bottom >= d.viewport.YOffset+d.viewport.Height:
		d.viewport.SetYOffset(bottom - d.viewport.Height + 1)
	}
}

func (d *DeckView) View() string {
	if !d.loaded {
		return tui.StyleMuted.Render(i18n.T(i18n.KeyDeckLoading))
	}
	if d.err != nil {
		return tui.StyleDanger.Render(i18n.T(i18n.KeyErrorPrefix) + d.err.Error())
	}

	d.viewport.SetContent(d.renderCards())
	d.scrollToCursor()

	body := lipgloss.JoinVertical(lipgloss.Left, d.renderHeader(), "", d.viewport.View())
	if d.confirmDelete && d.hasTargets() {
		return lipgloss.JoinVertical(lipgloss.Left, body, "", d.renderDeletePrompt())
	}
	return body
}

func (d *DeckView) renderDeletePrompt() string {
	ids := d.targetIDs()
	if len(ids) > 1 {
		return tui.StyleDanger.Render(i18n.Tf(i18n.KeyDeckDeleteMany, len(ids)))
	}
	if len(ids) == 1 {
		return tui.StyleDanger.Render(i18n.Tf(i18n.KeyDeckDeleteOne, ids[0]))
	}
	return ""
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
		d.cardLineStarts = nil
		d.cardLineHeight = nil
		return tui.StyleMuted.Render(i18n.T(i18n.KeyDeckNoCardsYet))
	}
	separator := d.cardSeparator()
	sepLines := strings.Count(separator, "\n") + 1

	parts := make([]string, 0, len(d.cards)*2-1)
	d.cardLineStarts = make([]int, len(d.cards))
	d.cardLineHeight = make([]int, len(d.cards))
	line := 0
	for i, c := range d.cards {
		row := d.renderCardRow(c, i == d.cursor)
		h := strings.Count(row, "\n") + 1
		d.cardLineStarts[i] = line
		d.cardLineHeight[i] = h
		line += h
		parts = append(parts, row)
		if i < len(d.cards)-1 {
			parts = append(parts, separator)
			line += sepLines
		}
	}
	return strings.Join(parts, "\n")
}

// cardSeparator draws a muted rule with vertical breathing room around it to
// visually split adjacent cards.
func (d *DeckView) cardSeparator() string {
	width := d.w
	if width <= 0 {
		width = 80
	}
	rule := lipgloss.NewStyle().Foreground(tui.ColorBorder).Render(strings.Repeat("─", width))
	return "\n" + rule + "\n"
}

func (d *DeckView) renderCardRow(c models.Card, cursor bool) string {
	selectMark := "  "
	if d.selected[c.ID] {
		selectMark = tui.StylePrimary.Render("● ")
	}
	header := fmt.Sprintf("%s%s%s  %s",
		selectionPrefix(cursor),
		selectMark,
		cardTypeBadge(c.Type),
		tui.StyleMuted.Render(fmt.Sprintf("(%s)", c.Language)),
	)
	body := d.renderPromptMarkdown(c.Prompt)
	if body == "" {
		return header
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, body)
}

// renderPromptMarkdown passes the prompt through the shared glamour renderer
// sized to the viewport. Leading/trailing blank lines from glamour's padding
// are trimmed so rows stay tight in the list.
func (d *DeckView) renderPromptMarkdown(prompt string) string {
	width := d.w
	if width <= 0 {
		width = 80
	}
	return strings.Trim(renderMarkdown(prompt, width), "\n")
}

func (d *DeckView) HelpKeys() []string {
	if d.confirmDelete && d.hasTargets() {
		return []string{
			i18n.Help("y", i18n.KeyHelpYDelete),
			i18n.Help("N", i18n.KeyHelpNCancel),
		}
	}
	if len(d.selected) > 0 {
		return []string{
			i18n.Help("↑/↓", i18n.KeyHelpMove),
			i18n.Help("space", i18n.KeyHelpSelect),
			i18n.Help("a", i18n.KeyHelpSelect),
			fmt.Sprintf("enter %s %d", i18n.T(i18n.KeyHelpEdit), len(d.selected)),
			fmt.Sprintf("d %s %d", i18n.T(i18n.KeyHelpDelete), len(d.selected)),
			i18n.Help("esc", i18n.KeyHelpCancel),
		}
	}
	return []string{
		i18n.Help("↑/↓", i18n.KeyHelpMove),
		i18n.Help("space", i18n.KeyHelpSelect),
		i18n.Help("enter", i18n.KeyHelpEdit),
		i18n.Help("s", i18n.KeyHelpStudy),
		i18n.Help("c", i18n.KeyCheatsheetTitleSuffix),
		i18n.Help("n", i18n.KeyHelpNew),
		i18n.Help("d", i18n.KeyHelpDelete),
		i18n.Help("r", i18n.KeyHelpReload),
		i18n.Help("esc", i18n.KeyHelpBack),
	}
}

