package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lforato/gocards/internal/models"
)

func (s *Store) CreateReview(cardID int64, grade int, ease float64, interval int, nextDue time.Time) (*models.Review, error) {
	res, err := s.db.Exec(
		`INSERT INTO reviews(card_id,grade,ease,interval,next_due) VALUES(?,?,?,?,?)`,
		cardID, grade, ease, interval, formatTime(nextDue),
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}
	return s.readReview(id)
}

func (s *Store) LastReview(cardID int64) (*models.Review, error) {
	var r models.Review
	var ts, ndue string
	err := s.db.QueryRow(
		`SELECT id,card_id,grade,reviewed_at,next_due,ease,interval
         FROM reviews WHERE card_id = ? ORDER BY reviewed_at DESC LIMIT 1`, cardID,
	).Scan(&r.ID, &r.CardID, &r.Grade, &ts, &ndue, &r.Ease, &r.Interval)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.ReviewedAt = parseTime(ts)
	r.NextDue = parseTime(ndue)
	return &r, nil
}

func (s *Store) readReview(id int64) (*models.Review, error) {
	var r models.Review
	var ts, ndue string
	err := s.db.QueryRow(
		`SELECT id,card_id,grade,reviewed_at,next_due,ease,interval FROM reviews WHERE id = ?`, id,
	).Scan(&r.ID, &r.CardID, &r.Grade, &ts, &ndue, &r.Ease, &r.Interval)
	if err != nil {
		return nil, err
	}
	r.ReviewedAt = parseTime(ts)
	r.NextDue = parseTime(ndue)
	return &r, nil
}
