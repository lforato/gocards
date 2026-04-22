package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lforato/gocards/internal/models"
)

// CardInput mirrors models.Card minus the server-managed fields so callers
// can't accidentally set ID/DeckID/CreatedAt.
type CardInput struct {
	Type           models.CardType
	Language       string
	Prompt         string
	InitialCode    string
	ExpectedAnswer string
	BlanksData     *models.BlankData
	Choices        []models.Choice
}

const cardCols = "id,deck_id,type,language,prompt,initial_code,expected_answer,blanks_data,choices,created_at"

type rowScanner interface {
	Scan(dest ...any) error
}

func scanCard(sc rowScanner) (models.Card, error) {
	var c models.Card
	var blanks, choices sql.NullString
	var ts string
	err := sc.Scan(
		&c.ID, &c.DeckID, &c.Type, &c.Language,
		&c.Prompt, &c.InitialCode, &c.ExpectedAnswer, &blanks, &choices, &ts,
	)
	if err != nil {
		return c, err
	}
	c.CreatedAt = parseTime(ts)
	if blanks.Valid && blanks.String != "" && blanks.String != "null" {
		var bd models.BlankData
		if err := json.Unmarshal([]byte(blanks.String), &bd); err != nil {
			return c, fmt.Errorf("card %d: decode blanks_data: %w", c.ID, err)
		}
		c.BlanksData = &bd
	}
	if choices.Valid && choices.String != "" && choices.String != "null" {
		var cs []models.Choice
		if err := json.Unmarshal([]byte(choices.String), &cs); err != nil {
			return c, fmt.Errorf("card %d: decode choices: %w", c.ID, err)
		}
		c.Choices = cs
	}
	return c, nil
}

func encodeCardJSON(in CardInput) (blanks, choices any, err error) {
	if in.BlanksData != nil {
		b, e := json.Marshal(in.BlanksData)
		if e != nil {
			return nil, nil, fmt.Errorf("encode blanks_data: %w", e)
		}
		blanks = string(b)
	}
	if in.Choices != nil {
		b, e := json.Marshal(in.Choices)
		if e != nil {
			return nil, nil, fmt.Errorf("encode choices: %w", e)
		}
		choices = string(b)
	}
	return blanks, choices, nil
}

func (s *Store) ListCards(deckID int64) ([]models.Card, error) {
	rows, err := s.db.Query(
		`SELECT `+cardCols+` FROM cards WHERE deck_id = ? ORDER BY created_at ASC`, deckID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.Card{}
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) CountCards(deckID int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM cards WHERE deck_id = ?`, deckID).Scan(&n)
	return n, err
}

// DueCards returns cards whose next_due has elapsed. Unreviewed cards count
// as due. Capped at limit rows.
func (s *Store) DueCards(deckID int64, limit int) ([]models.Card, error) {
	q := `
        SELECT ` + cardCols + ` FROM cards c
        LEFT JOIN (
            SELECT card_id, MAX(next_due) AS latest FROM reviews GROUP BY card_id
        ) lr ON lr.card_id = c.id
        WHERE c.deck_id = ?
          AND (lr.latest IS NULL OR lr.latest <= ?)
        ORDER BY c.created_at ASC
        LIMIT ?`

	rows, err := s.db.Query(q, deckID, formatTime(time.Now()), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.Card{}
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetCard(id int64) (*models.Card, error) {
	row := s.db.QueryRow(`SELECT `+cardCols+` FROM cards WHERE id = ?`, id)
	c, err := scanCard(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) BulkCreateCards(deckID int64, inputs []CardInput) ([]models.Card, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	ids := make([]int64, 0, len(inputs))
	for _, in := range inputs {
		blanksJSON, choicesJSON, err := encodeCardJSON(in)
		if err != nil {
			return nil, err
		}
		res, err := tx.Exec(
			`INSERT INTO cards(deck_id,type,language,prompt,initial_code,expected_answer,blanks_data,choices)
             VALUES(?,?,?,?,?,?,?,?)`,
			deckID, string(in.Type), in.Language, in.Prompt, in.InitialCode, in.ExpectedAnswer, blanksJSON, choicesJSON,
		)
		if err != nil {
			return nil, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("last insert id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	committed = true

	out := make([]models.Card, 0, len(ids))
	for _, id := range ids {
		c, err := s.GetCard(id)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, nil
}

func (s *Store) UpdateCard(id int64, in CardInput) (*models.Card, error) {
	blanksJSON, choicesJSON, err := encodeCardJSON(in)
	if err != nil {
		return nil, err
	}
	_, err = s.db.Exec(
		`UPDATE cards SET type=?, language=?, prompt=?, initial_code=?, expected_answer=?, blanks_data=?, choices=? WHERE id=?`,
		string(in.Type), in.Language, in.Prompt, in.InitialCode, in.ExpectedAnswer, blanksJSON, choicesJSON, id,
	)
	if err != nil {
		return nil, err
	}
	return s.GetCard(id)
}

func (s *Store) DeleteCard(id int64) error {
	_, err := s.db.Exec(`DELETE FROM cards WHERE id = ?`, id)
	return err
}
