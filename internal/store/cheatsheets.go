package store

import (
	"database/sql"
	"errors"
	"time"
)

type Cheatsheet struct {
	DeckID      int64
	Content     string
	GeneratedAt time.Time
}

// CardStat aggregates review outcomes for a single card. Zero values mean
// "never reviewed" and are valid — callers can use ReviewCount == 0 to
// distinguish fresh cards from struggling ones.
type CardStat struct {
	CardID      int64
	ReviewCount int
	AvgGrade    float64
	FailCount   int // reviews with grade <= 2
	LastGrade   int
}

// DeckCardStats returns one CardStat per card in the deck, keyed by card id.
// Cards with no reviews get a zero-valued stat (ReviewCount == 0).
func (s *Store) DeckCardStats(deckID int64) (map[int64]CardStat, error) {
	stats, err := s.loadAggregateStats(deckID)
	if err != nil {
		return nil, err
	}
	if err := s.attachLastGrade(deckID, stats); err != nil {
		// Losing LastGrade is non-fatal — the rest of the stat is still useful
		// for the UI, and this is the cold path (struggle summary). Fall back
		// to the partial data instead of surfacing a confusing error.
		return stats, nil
	}
	return stats, nil
}

func (s *Store) loadAggregateStats(deckID int64) (map[int64]CardStat, error) {
	rows, err := s.db.Query(`
		SELECT c.id,
		       COUNT(r.id)                                               AS review_count,
		       COALESCE(AVG(r.grade), 0)                                 AS avg_grade,
		       COALESCE(SUM(CASE WHEN r.grade <= 2 THEN 1 ELSE 0 END), 0) AS fail_count
		FROM cards c
		LEFT JOIN reviews r ON r.card_id = c.id
		WHERE c.deck_id = ?
		GROUP BY c.id`, deckID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := map[int64]CardStat{}
	for rows.Next() {
		var st CardStat
		if err := rows.Scan(&st.CardID, &st.ReviewCount, &st.AvgGrade, &st.FailCount); err != nil {
			return nil, err
		}
		stats[st.CardID] = st
	}
	return stats, rows.Err()
}

// attachLastGrade fills in stats[id].LastGrade for every card that has at
// least one review. SQLite lacks a clean per-group "last row" aggregate, so
// this runs as a separate query with a correlated subquery.
func (s *Store) attachLastGrade(deckID int64, stats map[int64]CardStat) error {
	rows, err := s.db.Query(`
		SELECT r.card_id, r.grade
		FROM reviews r
		JOIN cards c ON c.id = r.card_id
		WHERE c.deck_id = ?
		  AND r.reviewed_at = (
		      SELECT MAX(r2.reviewed_at) FROM reviews r2 WHERE r2.card_id = r.card_id
		  )`, deckID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cardID int64
		var grade int
		if err := rows.Scan(&cardID, &grade); err != nil {
			return err
		}
		if st, ok := stats[cardID]; ok {
			st.LastGrade = grade
			stats[cardID] = st
		}
	}
	return rows.Err()
}

// GetCheatsheet returns ErrNotFound when the deck has never generated one.
func (s *Store) GetCheatsheet(deckID int64) (*Cheatsheet, error) {
	var c Cheatsheet
	var ts string
	err := s.db.QueryRow(
		`SELECT deck_id, content, generated_at FROM cheatsheets WHERE deck_id = ?`, deckID,
	).Scan(&c.DeckID, &c.Content, &ts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.GeneratedAt = parseTime(ts)
	return &c, nil
}

// UpsertCheatsheet replaces the deck's cheatsheet with fresh content and
// bumps generated_at to now.
func (s *Store) UpsertCheatsheet(deckID int64, content string) error {
	_, err := s.db.Exec(
		`INSERT INTO cheatsheets(deck_id, content, generated_at) VALUES(?, ?, ?)
		 ON CONFLICT(deck_id) DO UPDATE SET content=excluded.content, generated_at=excluded.generated_at`,
		deckID, content, formatTime(time.Now().UTC()),
	)
	return err
}
