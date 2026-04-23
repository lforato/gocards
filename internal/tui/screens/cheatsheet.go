package screens

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lforato/gocards/internal/ai"
	"github.com/lforato/gocards/internal/i18n"
	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/store"
	"github.com/lforato/gocards/internal/tui"
)

const cheatsheetTimeout = 3 * time.Minute

type cheatsheetLoadedMsg struct {
	sheet *store.Cheatsheet
	cards []ai.CheatsheetCard
	err   error
}

type cheatsheetSavedMsg struct{ err error }

// CheatsheetView renders (and regenerates on demand) a per-deck markdown
// article synthesized from the deck's cards. The saved cheatsheet lives in
// the cheatsheets table so reopening the screen doesn't re-hit Claude.
type CheatsheetView struct {
	store *store.Store
	deck  models.Deck

	cards       []ai.CheatsheetCard
	content     string
	generatedAt time.Time
	loaded      bool
	loadErr     error

	streaming bool
	pending   string
	ctx       context.Context
	cancel    context.CancelFunc
	streamCh  <-chan ai.Event
	streamErr error
	spin      spinner.Model

	viewport viewport.Model
	w, h     int
}

func NewCheatsheetView(s *store.Store, d models.Deck) *CheatsheetView {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return &CheatsheetView{
		store:    s,
		deck:     d,
		spin:     sp,
		viewport: viewport.New(80, 20),
	}
}

func (c *CheatsheetView) Init() tea.Cmd { return c.load() }

func (c *CheatsheetView) load() tea.Cmd {
	return func() tea.Msg {
		cards, err := c.store.ListCards(c.deck.ID)
		if err != nil {
			return cheatsheetLoadedMsg{err: err}
		}
		stats, err := c.store.DeckCardStats(c.deck.ID)
		if err != nil {
			return cheatsheetLoadedMsg{err: err}
		}
		ordered := ai.OrderByStruggle(cards, stats)
		sheet, err := c.store.GetCheatsheet(c.deck.ID)
		if err != nil && err != store.ErrNotFound {
			return cheatsheetLoadedMsg{cards: ordered, err: err}
		}
		return cheatsheetLoadedMsg{sheet: sheet, cards: ordered}
	}
}

func (c *CheatsheetView) Update(msg tea.Msg) (tui.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		c.w, c.h = m.Width, m.Height
		c.resize()
		return c, nil

	case cheatsheetLoadedMsg:
		c.applyLoaded(m)
		return c, nil

	case spinner.TickMsg:
		if !c.streaming {
			return c, nil
		}
		var cmd tea.Cmd
		c.spin, cmd = c.spin.Update(m)
		return c, cmd

	case streamChunkMsg:
		c.pending += m.text
		c.renderViewport()
		c.viewport.GotoBottom()
		return c, pumpStream(c.streamCh)

	case streamDoneMsg:
		return c, c.finishStream(m)

	case streamErrMsg:
		c.streaming = false
		c.streamErr = m.err
		c.pending = ""
		c.renderViewport()
		return c, nil

	case cheatsheetSavedMsg:
		if m.err != nil {
			return c, tui.ToastErr(i18n.T(i18n.KeyCheatsheetSaveFailPfx) + m.err.Error())
		}
		return c, tui.Toast(i18n.T(i18n.KeyCheatsheetSaved))

	case tea.KeyMsg:
		return c.handleKey(m)
	}
	return c, nil
}

func (c *CheatsheetView) applyLoaded(m cheatsheetLoadedMsg) {
	c.loaded = true
	c.loadErr = m.err
	c.cards = m.cards
	if m.sheet != nil {
		c.content = m.sheet.Content
		c.generatedAt = m.sheet.GeneratedAt
	}
	c.renderViewport()
}

func (c *CheatsheetView) handleKey(m tea.KeyMsg) (tui.Screen, tea.Cmd) {
	switch m.String() {
	case "esc", "backspace":
		if c.streaming && c.cancel != nil {
			c.cancel()
		}
		return c, navBack
	case "q":
		if c.streaming && c.cancel != nil {
			c.cancel()
		}
		return c, tea.Quit
	case "r":
		if !c.streaming {
			return c, c.startGenerate()
		}
	case "x":
		if c.streaming && c.cancel != nil {
			c.cancel()
		}
	case "up", "k":
		c.viewport.LineUp(1)
	case "down", "j":
		c.viewport.LineDown(1)
	case "pgup", "ctrl+u":
		c.viewport.HalfViewUp()
	case "pgdown", "ctrl+d":
		c.viewport.HalfViewDown()
	case "g":
		c.viewport.GotoTop()
	case "G":
		c.viewport.GotoBottom()
	}
	return c, nil
}

func (c *CheatsheetView) startGenerate() tea.Cmd {
	if len(c.cards) == 0 {
		return tui.ToastErr(i18n.T(i18n.KeyCheatsheetNoCards))
	}
	client, err := resolveAIClient(c.store)
	if err != nil {
		return tui.ToastErr(err.Error())
	}
	c.ctx, c.cancel = context.WithTimeout(context.Background(), cheatsheetTimeout)
	c.streaming = true
	c.pending = ""
	c.streamErr = nil
	c.streamCh = client.Cheatsheet(c.ctx, c.deck, c.cards)
	c.renderViewport()
	return tea.Batch(c.spin.Tick, pumpStream(c.streamCh))
}

func (c *CheatsheetView) finishStream(m streamDoneMsg) tea.Cmd {
	final := m.full
	if final == "" {
		final = c.pending
	}
	c.streaming = false
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	if strings.TrimSpace(final) == "" {
		c.pending = ""
		c.renderViewport()
		return nil
	}
	c.content = final
	c.generatedAt = time.Now().UTC()
	c.pending = ""
	c.renderViewport()
	deckID := c.deck.ID
	store := c.store
	return func() tea.Msg {
		return cheatsheetSavedMsg{err: store.UpsertCheatsheet(deckID, final)}
	}
}

func (c *CheatsheetView) resize() {
	if c.w <= 0 {
		return
	}
	// Reserve: title (1) + meta (1) + blank (1) + status (1) = 4 rows.
	c.viewport.Width = c.w
	c.viewport.Height = max(3, c.h-4)
	c.renderViewport()
}

func (c *CheatsheetView) renderViewport() {
	width := c.w
	if width <= 0 {
		width = 80
	}
	var body string
	switch {
	case c.streaming && c.pending != "":
		body = renderMarkdown(c.pending, width)
	case c.content != "":
		body = renderMarkdown(c.content, width)
	case c.streaming:
		body = tui.StyleMuted.Render(i18n.T(i18n.KeyCheatsheetAsking))
	default:
		body = c.emptyState()
	}
	c.viewport.SetContent(body)
}

func (c *CheatsheetView) emptyState() string {
	if c.loadErr != nil {
		return tui.StyleDanger.Render(i18n.T(i18n.KeyErrorPrefix) + c.loadErr.Error())
	}
	if len(c.cards) == 0 {
		return tui.StyleMuted.Render(i18n.T(i18n.KeyCheatsheetEmptyDeck))
	}
	return tui.StyleMuted.Render(i18n.T(i18n.KeyCheatsheetNoSheet))
}

func (c *CheatsheetView) View() string {
	if !c.loaded {
		return tui.StyleMuted.Render(i18n.T(i18n.KeyCheatsheetLoading))
	}
	title := fmt.Sprintf("%s  %s",
		colorBullet(c.deck.Color),
		tui.StyleTitle.Render(c.deck.Name+i18n.T(i18n.KeyCheatsheetTitleSuffix)),
	)
	meta := c.renderMeta()
	status := c.renderStatus()
	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		meta,
		"",
		c.viewport.View(),
		status,
	)
}

func (c *CheatsheetView) renderMeta() string {
	parts := []string{pluralize(len(c.cards), "card", "cards")}
	if tiers := c.tierSummary(); tiers != "" {
		parts = append(parts, tiers)
	}
	if !c.generatedAt.IsZero() {
		parts = append(parts, "generated "+humanSince(c.generatedAt))
	} else {
		parts = append(parts, "never generated")
	}
	return tui.StyleMuted.Render(strings.Join(parts, "  ·  "))
}

// tierSummary renders a compact "🔥 3 · 🟡 7 · ✅ 12 · 💤 4" breakdown so the
// reader can see at a glance how the deck splits across struggle tiers.
func (c *CheatsheetView) tierSummary() string {
	if len(c.cards) == 0 {
		return ""
	}
	counts := map[string]int{}
	order := []ai.StruggleTier{ai.TierStruggling, ai.TierShaky, ai.TierSolid, ai.TierNew}
	for _, cc := range c.cards {
		counts[cc.Tier.Key]++
	}
	var parts []string
	for _, t := range order {
		if n := counts[t.Key]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", t.Emoji, n))
		}
	}
	return strings.Join(parts, " · ")
}

func (c *CheatsheetView) renderStatus() string {
	switch {
	case c.streaming:
		return c.spin.View() + tui.StyleMuted.Render(i18n.T(i18n.KeyCheatsheetGenerating))
	case c.streamErr != nil:
		return tui.StyleDanger.Render(c.streamErr.Error())
	}
	return ""
}

func (c *CheatsheetView) HelpKeys() []string {
	if c.streaming {
		return []string{
			i18n.Help("x", i18n.KeyHelpCancel),
			i18n.Help("esc", i18n.KeyHelpBack),
		}
	}
	return []string{
		i18n.Help("↑/↓", i18n.KeyHelpScroll),
		i18n.Help("g/G", i18n.KeyHelpTopBottom),
		i18n.Help("r", i18n.KeyHelpRegen),
		i18n.Help("esc", i18n.KeyHelpBack),
	}
}

// humanSince renders a coarse "N minutes ago" / "N hours ago" / "N days ago"
// string for the cheatsheet's generated_at timestamp.
func humanSince(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
