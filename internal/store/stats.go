package store

import (
	"time"

	"github.com/lforato/gocards/internal/models"
)

// Streak counts consecutive local-timezone days with at least one review,
// ending today. A day without reviews breaks the chain.
func (s *Store) Streak() (int, error) {
	days, err := s.reviewDaysLocal()
	if err != nil {
		return 0, err
	}
	streak := 0
	for cur := time.Now().Local(); ; cur = cur.AddDate(0, 0, -1) {
		if _, ok := days[cur.Format("2006-01-02")]; !ok {
			return streak, nil
		}
		streak++
	}
}

func (s *Store) reviewDaysLocal() (map[string]struct{}, error) {
	rows, err := s.db.Query(`SELECT reviewed_at FROM reviews ORDER BY reviewed_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	days := map[string]struct{}{}
	for rows.Next() {
		var ts string
		if err := rows.Scan(&ts); err != nil {
			return nil, err
		}
		days[parseTime(ts).Local().Format("2006-01-02")] = struct{}{}
	}
	return days, rows.Err()
}

func (s *Store) ReviewsToday() (int, error) {
	start := startOfLocalDay(time.Now())
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM reviews WHERE reviewed_at >= ?`, formatTime(start.UTC()),
	).Scan(&n)
	return n, err
}

// Retention is the percentage of reviews graded 4 or 5. Returns 0 if there
// are no reviews yet.
func (s *Store) Retention() (int, error) {
	var total, good int
	err := s.db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN grade >= 4 THEN 1 ELSE 0 END), 0) FROM reviews`,
	).Scan(&total, &good)
	if err != nil {
		return 0, err
	}
	if total == 0 {
		return 0, nil
	}
	return int(float64(good) / float64(total) * 100.0), nil
}

func (s *Store) DueToday() (int, error) {
	return s.dueTodayFromDecks(func() ([]models.Deck, error) { return s.ListDecks() })
}

// DueTodayByLanguage counts due cards only across decks matching the
// given language — feeds the dashboard's "due today" stat after i18n.
func (s *Store) DueTodayByLanguage(lang string) (int, error) {
	return s.dueTodayFromDecks(func() ([]models.Deck, error) { return s.ListDecksByLanguage(lang) })
}

func (s *Store) dueTodayFromDecks(fetch func() ([]models.Deck, error)) (int, error) {
	decks, err := fetch()
	if err != nil {
		return 0, err
	}
	limit := s.DailyLimit()
	total := 0
	for _, d := range decks {
		cards, err := s.DueCards(d.ID, limit)
		if err != nil {
			return 0, err
		}
		total += len(cards)
	}
	return total, nil
}

// Activity returns YYYY-MM-DD → review count for the last 90 days, feeding
// the dashboard heatmap.
func (s *Store) Activity() (map[string]int, error) {
	cutoff := time.Now().Local().AddDate(0, 0, -90)
	rows, err := s.db.Query(
		`SELECT reviewed_at FROM reviews WHERE reviewed_at >= ?`,
		formatTime(cutoff.UTC()),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var ts string
		if err := rows.Scan(&ts); err != nil {
			return nil, err
		}
		out[parseTime(ts).Local().Format("2006-01-02")]++
	}
	return out, rows.Err()
}

func (s *Store) DeckSummaries() ([]models.DeckWithCounts, error) {
	return s.deckSummariesFrom(func() ([]models.Deck, error) { return s.ListDecks() })
}

// DeckSummariesByLanguage is the i18n-aware variant used by every UI
// entry point that lists decks (dashboard, create, generate).
func (s *Store) DeckSummariesByLanguage(lang string) ([]models.DeckWithCounts, error) {
	return s.deckSummariesFrom(func() ([]models.Deck, error) { return s.ListDecksByLanguage(lang) })
}

func (s *Store) deckSummariesFrom(fetch func() ([]models.Deck, error)) ([]models.DeckWithCounts, error) {
	decks, err := fetch()
	if err != nil {
		return nil, err
	}
	limit := s.DailyLimit()
	out := make([]models.DeckWithCounts, 0, len(decks))
	for _, d := range decks {
		cc, err := s.CountCards(d.ID)
		if err != nil {
			return nil, err
		}
		due, err := s.DueCards(d.ID, limit)
		if err != nil {
			return nil, err
		}
		out = append(out, models.DeckWithCounts{Deck: d, CardCount: cc, DueCount: len(due)})
	}
	return out, nil
}
