package store

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/lforato/gocards/internal/models"
)

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// ErrInvalidDeck wraps validation failures in CreateDeck. Callers surface
// the message as a user-facing toast.
var ErrInvalidDeck = errors.New("invalid deck")

func validateDeckFields(name, color string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidDeck)
	}
	if color != "" && !hexColor.MatchString(color) {
		return fmt.Errorf("%w: color must be #rrggbb", ErrInvalidDeck)
	}
	return nil
}

func (s *Store) ListDecks() ([]models.Deck, error) {
	return s.queryDecks(`SELECT id,name,description,color,language,created_at FROM decks ORDER BY created_at ASC`)
}

// ListDecksByLanguage returns decks whose language column matches lang.
// Use this in any UI entry point — the unfiltered ListDecks is kept for
// seed/tests/migrations that need to see every row.
func (s *Store) ListDecksByLanguage(lang string) ([]models.Deck, error) {
	return s.queryDecks(
		`SELECT id,name,description,color,language,created_at FROM decks WHERE language = ? ORDER BY created_at ASC`,
		lang,
	)
}

func (s *Store) queryDecks(query string, args ...any) ([]models.Deck, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.Deck{}
	for rows.Next() {
		var d models.Deck
		var ts string
		if err := rows.Scan(&d.ID, &d.Name, &d.Description, &d.Color, &d.Language, &ts); err != nil {
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
		`SELECT id,name,description,color,language,created_at FROM decks WHERE id = ?`, id,
	).Scan(&d.ID, &d.Name, &d.Description, &d.Color, &d.Language, &ts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	d.CreatedAt = parseTime(ts)
	return &d, nil
}

// CreateDeck persists a new deck tagged with the given language (the
// caller passes the current i18n.CurrentLang() so the deck sticks to
// that locale on subsequent dashboard views).
func (s *Store) CreateDeck(name, description, color, language string) (*models.Deck, error) {
	if err := validateDeckFields(name, color); err != nil {
		return nil, err
	}
	if strings.TrimSpace(language) == "" {
		language = "en"
	}
	res, err := s.db.Exec(
		`INSERT INTO decks(name,description,color,language) VALUES(?,?,?,?)`,
		strings.TrimSpace(name), description, color, language,
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
