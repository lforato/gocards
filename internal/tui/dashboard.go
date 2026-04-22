package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/store"
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

type Dashboard struct {
	store  *store.Store
	cursor int // 0 = New cards button, 1 = Settings button, 2.. = decks
	stats  dashboardStats
	loaded bool
	err    error
	w      int
	h      int
}

func NewDashboard(s *store.Store) *Dashboard {
	return &Dashboard{store: s}
}

func (d *Dashboard) Init() tea.Cmd {
	return d.load()
}

func (d *Dashboard) load() tea.Cmd {
	return func() tea.Msg {
		streak, err := d.store.Streak()
		if err != nil {
			return dashboardLoadedMsg{err: err}
		}
		reviewed, err := d.store.ReviewsToday()
		if err != nil {
			return dashboardLoadedMsg{err: err}
		}
		retention, err := d.store.Retention()
		if err != nil {
			return dashboardLoadedMsg{err: err}
		}
		due, err := d.store.DueToday()
		if err != nil {
			return dashboardLoadedMsg{err: err}
		}
		activity, err := d.store.Activity()
		if err != nil {
			return dashboardLoadedMsg{err: err}
		}
		decks, err := d.store.DeckSummaries()
		if err != nil {
			return dashboardLoadedMsg{err: err}
		}
		return dashboardLoadedMsg{stats: dashboardStats{
			streak: streak, reviewed: reviewed, retention: retention,
			due: due, activity: activity, decks: decks,
		}}
	}
}

func (d *Dashboard) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch m := msg.(type) {

	case tea.WindowSizeMsg:
		d.w = m.Width
		d.h = m.Height

	case dashboardLoadedMsg:
		d.loaded = true
		d.err = m.err
		d.stats = m.stats
		return d, nil

	case tea.KeyMsg:
		switch m.String() {
		case "q":
			return d, tea.Quit
		case "r":
			d.loaded = false
			return d, d.load()
		case "n":
			return d, func() tea.Msg { return NavMsg{To: NewCreate(d.store, 0)} }
		case "g":
			return d, d.openGenerate()
		case "s":
			return d, func() tea.Msg { return NavMsg{To: NewSettings(d.store)} }
		case "up", "k":
			d.cursor = cursorUp(d.cursor)
		case "down", "j":
			d.cursor = cursorDown(d.cursor, len(d.stats.decks)+3)
		case "enter":
			return d, d.activate()
		case "S":
			if d.cursor >= 3 && d.cursor-3 < len(d.stats.decks) {
				deck := d.stats.decks[d.cursor-3]
				if deck.DueCount > 0 {
					return d, func() tea.Msg {
						return NavMsg{To: NewStudy(d.store, deck.Deck)}
					}
				}
			}
		}
	}
	return d, nil
}

func (d *Dashboard) activate() tea.Cmd {
	switch d.cursor {
	case 0:
		return func() tea.Msg { return NavMsg{To: NewCreate(d.store, 0)} }
	case 1:
		return d.openGenerate()
	case 2:
		return func() tea.Msg { return NavMsg{To: NewSettings(d.store)} }
	default:
		idx := d.cursor - 3
		if idx < 0 || idx >= len(d.stats.decks) {
			return nil
		}
		deck := d.stats.decks[idx]
		return func() tea.Msg { return NavMsg{To: NewDeckView(d.store, deck.Deck)} }
	}
}

// openGenerate opens the AI-chat card generator, seeded with the deck under
// the cursor if any, else the first deck, else a synthetic fallback (the
// picker inside AIGenerate lets the user change it afterward).
func (d *Dashboard) openGenerate() tea.Cmd {
	var deck models.Deck
	if d.cursor >= 3 {
		idx := d.cursor - 3
		if idx >= 0 && idx < len(d.stats.decks) {
			deck = d.stats.decks[idx].Deck
		}
	}
	if deck.ID == 0 && len(d.stats.decks) > 0 {
		deck = d.stats.decks[0].Deck
	}
	if deck.ID == 0 {
		return ToastErr("create a deck first")
	}
	return func() tea.Msg { return NavMsg{To: NewAIGenerate(d.store, deck)} }
}

func (d *Dashboard) View() string {
	if !d.loaded {
		return StyleMuted.Render("loading…")
	}
	if d.err != nil {
		return StyleDanger.Render("error: " + d.err.Error())
	}

	s := d.stats
	width := (d.w / 4) - 4

	// Stat strip
	stats := lipgloss.JoinHorizontal(lipgloss.Top,
		statBox("streak", fmt.Sprintf("%dd", s.streak), width),
		"  ",
		statBox("reviewed", fmt.Sprintf("%d", s.reviewed), width),
		"  ",
		statBox("retention", fmt.Sprintf("%d%%", s.retention), width),
		"  ",
		statBox("due today", fmt.Sprintf("%d", s.due), width),
	)

	// Heatmap — fill the dashboard width (minus border + padding)
	// lipgloss Width = content + padding (border adds on top). Keep these in sync
	// so the grid can't overflow and wrap.
	heatOuterW := d.w - 6        // lipgloss Width value
	heatInnerW := heatOuterW - 2 // subtract horizontal padding (1 each side)
	heat := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(0, 1).
		Width(heatOuterW).
		Render(Heatmap(s.activity, heatInnerW))

	// Actions + decks list
	var rows []string
	rows = append(rows, renderRow("+ New cards", "n", d.cursor == 0))
	rows = append(rows, renderRow("🤖 Generate with AI", "g", d.cursor == 1))
	rows = append(rows, renderRow("⚙ Settings", "s", d.cursor == 2))
	rows = append(rows, "")
	rows = append(rows, StyleMuted.Render(fmt.Sprintf("decks · %d", len(s.decks))))

	for i, deck := range s.decks {
		sel := d.cursor == i+3
		name := deck.Name
		if sel {
			name = StyleSelected.Render(name)
		}
		due := ""
		if deck.DueCount > 0 {
			due = "  " + StylePrimary.Render(fmt.Sprintf("%d due", deck.DueCount))
		}
		rows = append(rows, fmt.Sprintf("%s%s %s  %s%s",
			selectionPrefix(sel),
			colorBullet(deck.Color),
			name,
			StyleMuted.Render(fmt.Sprintf("(%s)", pluralize(deck.CardCount, "card", "cards"))),
			due,
		))
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		stats,
		"",
		heat,
		"",
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
}

func (d *Dashboard) HelpKeys() []string {
	return []string{"↑/↓ select", "enter open", "S study", "n new", "g ai", "s settings", "r reload", "q quit"}
}

func statBox(label, value string, w int) string {
	content := lipgloss.JoinVertical(lipgloss.Left,
		StyleMuted.Render(label),
		lipgloss.NewStyle().Foreground(ColorFg).Bold(true).Render(value),
	)
	return StatCard.Width(w).Render(content)
}

func renderRow(text, key string, selected bool) string {
	style := lipgloss.NewStyle().Foreground(ColorFg)
	if selected {
		style = StyleSelected
	}
	return selectionPrefix(selected) + style.Render(text) + "  " + StyleMuted.Render("["+key+"]")
}
