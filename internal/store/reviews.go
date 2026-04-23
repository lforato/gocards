package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lforato/gocards/internal/models"
)

const reviewCols = "id,card_id,grade,reviewed_at,next_due,ease,interval"

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
	return s.getReviewByID(id)
}

// LastReview returns the most recent review of a given card, or ErrNotFound
// if the card has never been reviewed.
func (s *Store) LastReview(cardID int64) (*models.Review, error) {
	row := s.db.QueryRow(
		`SELECT `+reviewCols+`
		 FROM reviews WHERE card_id = ? ORDER BY reviewed_at DESC LIMIT 1`, cardID,
	)
	return scanReview(row)
}

func (s *Store) getReviewByID(id int64) (*models.Review, error) {
	return scanReview(s.db.QueryRow(`SELECT `+reviewCols+` FROM reviews WHERE id = ?`, id))
}

func scanReview(row rowScanner) (*models.Review, error) {
	var r models.Review
	var reviewedAt, nextDue string
	err := row.Scan(&r.ID, &r.CardID, &r.Grade, &reviewedAt, &nextDue, &r.Ease, &r.Interval)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.ReviewedAt = parseTime(reviewedAt)
	r.NextDue = parseTime(nextDue)
	return &r, nil
}
