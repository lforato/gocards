package store

import (
	"database/sql"
	"errors"

	"github.com/lforato/gocards/internal/models"
)

func (s *Store) ListDecks() ([]models.Deck, error) {
	rows, err := s.db.Query(`SELECT id,name,description,color,created_at FROM decks ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.Deck{}
	for rows.Next() {
		var d models.Deck
		var ts string
		if err := rows.Scan(&d.ID, &d.Name, &d.Description, &d.Color, &ts); err != nil {
			return nil, err
		}
		d.CreatedAt = parseTime(ts)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) GetDeck(id int64) (*models.Deck, error) {
	var d models.Deck
	var ts string
	err := s.db.QueryRow(
		`SELECT id,name,description,color,created_at FROM decks WHERE id = ?`, id,
	).Scan(&d.ID, &d.Name, &d.Description, &d.Color, &ts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	d.CreatedAt = parseTime(ts)
	return &d, nil
}

func (s *Store) CreateDeck(name, description, color string) (*models.Deck, error) {
	res, err := s.db.Exec(
		`INSERT INTO decks(name,description,color) VALUES(?,?,?)`,
		name, description, color,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetDeck(id)
}

func (s *Store) UpdateDeck(id int64, name, description, color *string) (*models.Deck, error) {
	up := newPatch()
	up.setIfPtr("name", name)
	up.setIfPtr("description", description)
	up.setIfPtr("color", color)
	if err := up.exec(s.db, "decks", id); err != nil {
		return nil, err
	}
	return s.GetDeck(id)
}

func (s *Store) DeleteDeck(id int64) error {
	_, err := s.db.Exec(`DELETE FROM decks WHERE id = ?`, id)
	return err
}
