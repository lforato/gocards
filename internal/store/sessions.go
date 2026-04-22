package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lforato/gocards/internal/models"
)

func (s *Store) CreateSession(deckID int64) (*models.StudySession, error) {
	res, err := s.db.Exec(`INSERT INTO study_sessions(deck_id) VALUES(?)`, deckID)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}
	return s.GetSession(id)
}

func (s *Store) GetSession(id int64) (*models.StudySession, error) {
	var ss models.StudySession
	var started string
	var ended sql.NullString
	err := s.db.QueryRow(
		`SELECT id,deck_id,started_at,ended_at,cards_reviewed FROM study_sessions WHERE id = ?`, id,
	).Scan(&ss.ID, &ss.DeckID, &started, &ended, &ss.CardsReviewed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	ss.StartedAt = parseTime(started)
	if ended.Valid {
		t := parseTime(ended.String)
		ss.EndedAt = &t
	}
	return &ss, nil
}

func (s *Store) UpdateSession(id int64, cardsReviewed *int, ended *time.Time, clearEnded bool) (*models.StudySession, error) {
	up := newPatch()
	up.setIfPtr("cards_reviewed", cardsReviewed)
	switch {
	case clearEnded:
		up.setRaw("ended_at", "NULL")
	case ended != nil:
		up.set("ended_at", formatTime(*ended))
	}
	if err := up.exec(s.db, "study_sessions", id); err != nil {
		return nil, err
	}
	return s.GetSession(id)
}
