package store

import (
	"time"

	"github.com/lforato/gocards/internal/models"
)

// Streak returns the number of consecutive local-timezone days ending today on
// which at least one review was logged. A gap of one day (no reviews
// yesterday) breaks the streak.
func (s *Store) Streak() (int, error) {
	rows, err := s.db.Query(`SELECT reviewed_at FROM reviews ORDER BY reviewed_at DESC`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	days := map[string]struct{}{}
	for rows.Next() {
		var ts string
		if err := rows.Scan(&ts); err != nil {
			return 0, err
		}
		t := parseTime(ts).Local()
		days[t.Format("2006-01-02")] = struct{}{}
	}

	streak := 0
	cur := time.Now().Local()
	for {
		if _, ok := days[cur.Format("2006-01-02")]; !ok {
			return streak, nil
		}
		streak++
		cur = cur.AddDate(0, 0, -1)
	}
}

// ReviewsToday counts reviews logged since midnight local time.
func (s *Store) ReviewsToday() (int, error) {
	start := startOfLocalDay(time.Now())
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM reviews WHERE reviewed_at >= ?`, formatTime(start.UTC()),
	).Scan(&n)
	return n, err
}

// Retention returns the percentage of all reviews graded 4 or 5.
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

// DueToday counts cards eligible for review right now across all decks. Used
// by the dashboard header.
func (s *Store) DueToday() (int, error) {
	decks, err := s.ListDecks()
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

// Activity returns a map of YYYY-MM-DD → reviews-that-day for the last 90
// days, used by the dashboard heatmap.
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
		t := parseTime(ts).Local()
		out[t.Format("2006-01-02")]++
	}
	return out, rows.Err()
}

// DeckSummaries returns every deck decorated with its card count and current
// due-card count. Used by the dashboard's deck list.
func (s *Store) DeckSummaries() ([]models.DeckWithCounts, error) {
	decks, err := s.ListDecks()
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
