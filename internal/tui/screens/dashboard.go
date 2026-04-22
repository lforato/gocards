// Package screens contains one Bubble Tea model per user-facing screen:
// dashboard, deck view, create, edit, study, AI generate, settings. Each
// screen implements tui.Screen and is pushed onto the App's screen stack.
// The per-card-type dispatch tables (cardUIs, studyBehaviors) and the
// per-screen helpers (ai, uihelpers, stream) also live here.
package screens

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/store"
	"github.com/lforato/gocards/internal/tui"
	"github.com/lforato/gocards/internal/tui/widgets"
)

type dashboardStats struct {
	streak    int
	reviewed  int
	retention int
	due       int
	activity  map[string]int
	decks     []models.DeckWithCounts
}

type dashboardLoadedMsg struct {
	stats dashboardStats
	err   error
}

// dashboardEntry is one selectable row: either a top-level action or a deck.
// Uniform entries replace the old "cursor 0..2 = buttons, 3+ = decks" magic.
type dashboardEntry struct {
	kind dashboardEntryKind
	deck models.DeckWithCounts
}

type dashboardEntryKind int

const (
	entryActionNew dashboardEntryKind = iota
	entryActionGenerate
	entryActionSettings
	entryDeck
)

type Dashboard struct {
	store  *store.Store
	cursor int
	stats  dashboardStats
	loaded bool
	err    error
	w      int
	h      int

	pendingDelete *models.Deck
}

func NewDashboard(s *store.Store) *Dashboard {
	return &Dashboard{store: s}
}

func (d *Dashboard) Init() tea.Cmd { return d.load() }

func (d *Dashboard) load() tea.Cmd {
	return func() tea.Msg {
		stats, err := loadDashboardStats(d.store)
		return dashboardLoadedMsg{stats: stats, err: err}
	}
}

func loadDashboardStats(s *store.Store) (dashboardStats, error) {
	streak, err := s.Streak()
	if err != nil {
		return dashboardStats{}, err
	}
	reviewed, err := s.ReviewsToday()
	if err != nil {
		return dashboardStats{}, err
	}
	retention, err := s.Retention()
	if err != nil {
		return dashboardStats{}, err
	}
	due, err := s.DueToday()
	if err != nil {
		return dashboardStats{}, err
	}
	activity, err := s.Activity()
	if err != nil {
		return dashboardStats{}, err
	}
	decks, err := s.DeckSummaries()
	if err != nil {
		return dashboardStats{}, err
	}
	return dashboardStats{
		streak: streak, reviewed: reviewed, retention: retention,
		due: due, activity: activity, decks: decks,
	}, nil
}

func (d *Dashboard) entries() []dashboardEntry {
	out := []dashboardEntry{
		{kind: entryActionNew},
		{kind: entryActionGenerate},
		{kind: entryActionSettings},
	}
	for _, deck := range d.stats.decks {
		out = append(out, dashboardEntry{kind: entryDeck, deck: deck})
	}
	return out
}

func (d *Dashboard) Update(msg tea.Msg) (tui.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		d.w, d.h = m.Width, m.Height
	case dashboardLoadedMsg:
		d.loaded = true
		d.err = m.err
		d.stats = m.stats
	case tea.KeyMsg:
		return d.handleKey(m)
	}
	return d, nil
}

func (d *Dashboard) handleKey(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	if d.pendingDelete != nil {
		return d.handleDeleteConfirm(m)
	}
	entries := d.entries()
	switch m.String() {
	case "q":
		return d, tea.Quit
	case "r":
		d.loaded = false
		return d, d.load()
	case "n":
		return d, navTo(NewCreate(d.store, 0))
	case "g":
		return d, d.openGenerate()
	case "s":
		return d, navTo(NewSettings(d.store))
	case "up", "k":
		d.cursor = cursorUp(d.cursor)
	case "down", "j":
		d.cursor = cursorDown(d.cursor, len(entries))
	case "enter":
		return d, d.activate(entries)
	case "S":
		return d, d.studySelectedDeck(entries)
	case "D":
		d.promptDeleteDeck(entries)
	}
	return d, nil
}

func (d *Dashboard) promptDeleteDeck(entries []dashboardEntry) {
	if d.cursor < 0 || d.cursor >= len(entries) {
		return
	}
	e := entries[d.cursor]
	if e.kind != entryDeck {
		return
	}
	deck := e.deck.Deck
	d.pendingDelete = &deck
}

func (d *Dashboard) handleDeleteConfirm(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	target := d.pendingDelete
	d.pendingDelete = nil
	if m.String() != "y" && m.String() != "Y" {
		return d, nil
	}
	if err := d.store.DeleteDeck(target.ID); err != nil {
		return d, tui.ToastErr("delete failed: " + err.Error())
	}
	d.loaded = false
	return d, tea.Batch(tui.Toast(fmt.Sprintf("deleted %q", target.Name)), d.load())
}

func (d *Dashboard) activate(entries []dashboardEntry) tea.Cmd {
	if d.cursor < 0 || d.cursor >= len(entries) {
		return nil
	}
	e := entries[d.cursor]
	switch e.kind {
	case entryActionNew:
		return navTo(NewCreate(d.store, 0))
	case entryActionGenerate:
		return d.openGenerate()
	case entryActionSettings:
		return navTo(NewSettings(d.store))
	case entryDeck:
		return navTo(NewDeckView(d.store, e.deck.Deck))
	}
	return nil
}

func (d *Dashboard) studySelectedDeck(entries []dashboardEntry) tea.Cmd {
	if d.cursor < 0 || d.cursor >= len(entries) {
		return nil
	}
	e := entries[d.cursor]
	if e.kind != entryDeck || e.deck.DueCount == 0 {
		return nil
	}
	return navTo(NewStudy(d.store, e.deck.Deck))
}

// openGenerate seeds the AI chat with the deck under the cursor if one is
// highlighted, else the first deck. Bails with a toast if no decks exist.
func (d *Dashboard) openGenerate() tea.Cmd {
	deck := d.preferredGenerateDeck()
	if deck.ID == 0 {
		return tui.ToastErr("create a deck first")
	}
	return navTo(NewAIGenerate(d.store, deck))
}

func (d *Dashboard) preferredGenerateDeck() models.Deck {
	entries := d.entries()
	if d.cursor >= 0 && d.cursor < len(entries) && entries[d.cursor].kind == entryDeck {
		return entries[d.cursor].deck.Deck
	}
	if len(d.stats.decks) > 0 {
		return d.stats.decks[0].Deck
	}
	return models.Deck{}
}

func (d *Dashboard) View() string {
	if !d.loaded {
		return tui.StyleMuted.Render("loading…")
	}
	if d.err != nil {
		return tui.StyleDanger.Render("error: " + d.err.Error())
	}

	body := lipgloss.JoinVertical(lipgloss.Left,
		d.renderStats(),
		"",
		d.renderHeatmap(),
		"",
		d.renderEntries(),
	)
	if d.pendingDelete != nil {
		body = lipgloss.JoinVertical(lipgloss.Left, body, "", d.renderDeletePrompt())
	}
	return body
}

func (d *Dashboard) renderDeletePrompt() string {
	return tui.StyleDanger.Render(fmt.Sprintf(
		"delete deck %q and all its cards? y/N", d.pendingDelete.Name,
	))
}

func (d *Dashboard) renderStats() string {
	width := (d.w / 4) - 4
	s := d.stats
	return lipgloss.JoinHorizontal(lipgloss.Top,
		statBox("streak", fmt.Sprintf("%dd", s.streak), width), "  ",
		statBox("reviewed", fmt.Sprintf("%d", s.reviewed), width), "  ",
		statBox("retention", fmt.Sprintf("%d%%", s.retention), width), "  ",
		statBox("due today", fmt.Sprintf("%d", s.due), width),
	)
}

func (d *Dashboard) renderHeatmap() string {
	// lipgloss.Width = content + padding (the border adds 2 more cells).
	// Compute both outer and inner widths so the grid can't wrap.
	outerW := d.w - 6
	innerW := outerW - 2
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorBorder).
		Padding(0, 1).
		Width(outerW).
		Render(widgets.Heatmap(d.stats.activity, innerW))
}

func (d *Dashboard) renderEntries() string {
	entries := d.entries()
	rows := []string{
		renderActionRow("+ New cards", "n", d.cursor == 0),
		renderActionRow("🤖 Generate with AI", "g", d.cursor == 1),
		renderActionRow("⚙ Settings", "s", d.cursor == 2),
		"",
		tui.StyleMuted.Render(fmt.Sprintf("decks · %d", len(d.stats.decks))),
	}
	for i, e := range entries {
		if e.kind != entryDeck {
			continue
		}
		rows = append(rows, renderDeckRow(e.deck, i == d.cursor))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func renderDeckRow(deck models.DeckWithCounts, selected bool) string {
	name := deck.Name
	if selected {
		name = tui.StyleSelected.Render(name)
	}
	due := ""
	if deck.DueCount > 0 {
		due = "  " + tui.StylePrimary.Render(fmt.Sprintf("%d due", deck.DueCount))
	}
	return fmt.Sprintf("%s%s %s  %s%s",
		selectionPrefix(selected),
		colorBullet(deck.Color),
		name,
		tui.StyleMuted.Render(fmt.Sprintf("(%s)", pluralize(deck.CardCount, "card", "cards"))),
		due,
	)
}

func (d *Dashboard) HelpKeys() []string {
	if d.pendingDelete != nil {
		return []string{"y delete", "N cancel"}
	}
	return []string{"↑/↓ select", "enter open", "S study", "D delete", "n new", "g ai", "s settings", "r reload", "q quit"}
}

func statBox(label, value string, w int) string {
	return tui.StatCard.Width(w).Render(lipgloss.JoinVertical(lipgloss.Left,
		tui.StyleMuted.Render(label),
		lipgloss.NewStyle().Foreground(tui.ColorFg).Bold(true).Render(value),
	))
}

func renderActionRow(text, key string, selected bool) string {
	style := lipgloss.NewStyle().Foreground(tui.ColorFg)
	if selected {
		style = tui.StyleSelected
	}
	return selectionPrefix(selected) + style.Render(text) + "  " + tui.StyleMuted.Render("["+key+"]")
}
