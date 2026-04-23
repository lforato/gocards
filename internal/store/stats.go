package store

import (
	"time"

	"github.com/lforato/gocards/internal/models"
)

const dayFormat = "2006-01-02"

// Streak counts consecutive local-timezone days with at least one review,
// ending today. A day without reviews breaks the chain.
func (s *Store) Streak() (int, error) {
	reviewed, err := s.reviewedDaysSet()
	if err != nil {
		return 0, err
	}
	streak := 0
	for day := time.Now().Local(); ; day = day.AddDate(0, 0, -1) {
		if _, ok := reviewed[day.Format(dayFormat)]; !ok {
			return streak, nil
		}
		streak++
	}
}

// reviewedDaysSet returns the set of local-timezone dates (YYYY-MM-DD)
// that have at least one review.
func (s *Store) reviewedDaysSet() (map[string]struct{}, error) {
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
		days[parseTime(ts).Local().Format(dayFormat)] = struct{}{}
	}
	return days, rows.Err()
}

func (s *Store) ReviewsToday() (int, error) {
	startOfToday := startOfLocalDay(time.Now())
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM reviews WHERE reviewed_at >= ?`, formatTime(startOfToday.UTC()),
	).Scan(&n)
	return n, err
}

// Retention is the percentage of reviews graded 4 or 5. Returns 0 if there
// are no reviews yet.
func (s *Store) Retention() (int, error) {
	var total, passed int
	err := s.db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN grade >= 4 THEN 1 ELSE 0 END), 0) FROM reviews`,
	).Scan(&total, &passed)
	if err != nil {
		return 0, err
	}
	if total == 0 {
		return 0, nil
	}
	return int(float64(passed) / float64(total) * 100.0), nil
}

// DueToday counts due cards across all decks. Use DueTodayByLanguage in UI
// flows; this unfiltered variant exists for stats/diagnostics.
func (s *Store) DueToday() (int, error) {
	decks, err := s.ListDecks()
	if err != nil {
		return 0, err
	}
	return s.countDueAcross(decks)
}

// DueTodayByLanguage counts due cards only across decks matching lang —
// feeds the dashboard's "due today" stat after i18n.
func (s *Store) DueTodayByLanguage(lang string) (int, error) {
	decks, err := s.ListDecksByLanguage(lang)
	if err != nil {
		return 0, err
	}
	return s.countDueAcross(decks)
}

func (s *Store) countDueAcross(decks []models.Deck) (int, error) {
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

// Activity returns a YYYY-MM-DD → review-count map for the last 90 days,
// feeding the dashboard heatmap.
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

	byDay := map[string]int{}
	for rows.Next() {
		var ts string
		if err := rows.Scan(&ts); err != nil {
			return nil, err
		}
		byDay[parseTime(ts).Local().Format(dayFormat)]++
	}
	return byDay, rows.Err()
}

// DeckSummaries returns every deck plus its card/due totals. See
// DeckSummariesByLanguage for the UI-friendly variant.
func (s *Store) DeckSummaries() ([]models.DeckWithCounts, error) {
	decks, err := s.ListDecks()
	if err != nil {
		return nil, err
	}
	return s.attachCountsToDecks(decks)
}

// DeckSummariesByLanguage is the i18n-aware variant used by every UI entry
// point that lists decks (dashboard, create, generate).
func (s *Store) DeckSummariesByLanguage(lang string) ([]models.DeckWithCounts, error) {
	decks, err := s.ListDecksByLanguage(lang)
	if err != nil {
		return nil, err
	}
	return s.attachCountsToDecks(decks)
}

func (s *Store) attachCountsToDecks(decks []models.Deck) ([]models.DeckWithCounts, error) {
	limit := s.DailyLimit()
	summaries := make([]models.DeckWithCounts, 0, len(decks))
	for _, d := range decks {
		cardCount, err := s.CountCards(d.ID)
		if err != nil {
			return nil, err
		}
		due, err := s.DueCards(d.ID, limit)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, models.DeckWithCounts{
			Deck:      d,
			CardCount: cardCount,
			DueCount:  len(due),
		})
	}
	return summaries, nil
}
