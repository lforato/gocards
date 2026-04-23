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

// CardStat aggregates review outcomes for a single card. Zero values are
// valid and mean "never reviewed".
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
	rows, err := s.db.Query(`
		SELECT c.id,
		       COUNT(r.id)                                              AS n,
		       COALESCE(AVG(r.grade), 0)                                AS avg_grade,
		       COALESCE(SUM(CASE WHEN r.grade <= 2 THEN 1 ELSE 0 END),0) AS fails
		FROM cards c
		LEFT JOIN reviews r ON r.card_id = c.id
		WHERE c.deck_id = ?
		GROUP BY c.id`, deckID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]CardStat{}
	for rows.Next() {
		var st CardStat
		if err := rows.Scan(&st.CardID, &st.ReviewCount, &st.AvgGrade, &st.FailCount); err != nil {
			return nil, err
		}
		out[st.CardID] = st
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Attach each card's most recent grade in a second pass — SQLite lacks a
	// clean per-group "last row" aggregate, so we keep the primary query simple.
	lastRows, err := s.db.Query(`
		SELECT r.card_id, r.grade
		FROM reviews r
		JOIN cards c ON c.id = r.card_id
		WHERE c.deck_id = ?
		  AND r.reviewed_at = (SELECT MAX(r2.reviewed_at) FROM reviews r2 WHERE r2.card_id = r.card_id)`, deckID)
	if err != nil {
		return out, nil
	}
	defer lastRows.Close()
	for lastRows.Next() {
		var id int64
		var grade int
		if err := lastRows.Scan(&id, &grade); err != nil {
			return out, nil
		}
		if st, ok := out[id]; ok {
			st.LastGrade = grade
			out[id] = st
		}
	}
	return out, nil
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

// UpsertCheatsheet replaces the deck's cheatsheet with fresh content and bumps
// generated_at to now.
func (s *Store) UpsertCheatsheet(deckID int64, content string) error {
	_, err := s.db.Exec(
		`INSERT INTO cheatsheets(deck_id, content, generated_at) VALUES(?, ?, ?)
		 ON CONFLICT(deck_id) DO UPDATE SET content=excluded.content, generated_at=excluded.generated_at`,
		deckID, content, formatTime(time.Now().UTC()),
	)
	return err
}
