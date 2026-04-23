package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lforato/gocards/internal/models"
)

// CardInput mirrors models.Card minus the server-managed fields (id, deck_id,
// created_at). Callers use this to create or update cards without being able
// to forge identity or timestamps.
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
	var blanksJSON, choicesJSON sql.NullString
	var createdAt string
	err := sc.Scan(
		&c.ID, &c.DeckID, &c.Type, &c.Language,
		&c.Prompt, &c.InitialCode, &c.ExpectedAnswer, &blanksJSON, &choicesJSON, &createdAt,
	)
	if err != nil {
		return c, err
	}
	c.CreatedAt = parseTime(createdAt)

	if hasJSONValue(blanksJSON) {
		var bd models.BlankData
		if err := json.Unmarshal([]byte(blanksJSON.String), &bd); err != nil {
			return c, fmt.Errorf("card %d: decode blanks_data: %w", c.ID, err)
		}
		c.BlanksData = &bd
	}

	if hasJSONValue(choicesJSON) {
		var cs []models.Choice
		if err := json.Unmarshal([]byte(choicesJSON.String), &cs); err != nil {
			return c, fmt.Errorf("card %d: decode choices: %w", c.ID, err)
		}
		c.Choices = cs
	}

	return c, nil
}

// hasJSONValue reports whether a nullable JSON column actually carries a
// value. Empty string, NULL, and the literal "null" are all treated as
// "no value" — the column stays on the model as nil/empty slice.
func hasJSONValue(col sql.NullString) bool {
	return col.Valid && col.String != "" && col.String != "null"
}

// encodeCardJSON returns the DB-ready representations of the two optional
// JSON columns. Nil-valued fields encode to nil (not "null") so the column
// stays NULL.
func encodeCardJSON(in CardInput) (blanksJSON, choicesJSON any, err error) {
	if in.BlanksData != nil {
		b, e := json.Marshal(in.BlanksData)
		if e != nil {
			return nil, nil, fmt.Errorf("encode blanks_data: %w", e)
		}
		blanksJSON = string(b)
	}
	if in.Choices != nil {
		b, e := json.Marshal(in.Choices)
		if e != nil {
			return nil, nil, fmt.Errorf("encode choices: %w", e)
		}
		choicesJSON = string(b)
	}
	return blanksJSON, choicesJSON, nil
}

func (s *Store) ListCards(deckID int64) ([]models.Card, error) {
	return s.queryCards(
		`SELECT `+cardCols+` FROM cards WHERE deck_id = ? ORDER BY created_at ASC`,
		deckID,
	)
}

func (s *Store) CountCards(deckID int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM cards WHERE deck_id = ?`, deckID).Scan(&n)
	return n, err
}

// DueCards returns cards whose last review's next_due has elapsed. Cards that
// have never been reviewed count as due. Results are oldest-first, capped at
// limit rows.
func (s *Store) DueCards(deckID int64, limit int) ([]models.Card, error) {
	const query = `
		SELECT ` + cardCols + ` FROM cards c
		LEFT JOIN (
			SELECT card_id, MAX(next_due) AS last_next_due
			FROM reviews
			GROUP BY card_id
		) last_review ON last_review.card_id = c.id
		WHERE c.deck_id = ?
		  AND (last_review.last_next_due IS NULL OR last_review.last_next_due <= ?)
		ORDER BY c.created_at ASC
		LIMIT ?`
	return s.queryCards(query, deckID, formatTime(time.Now()), limit)
}

func (s *Store) queryCards(query string, args ...any) ([]models.Card, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cards := []models.Card{}
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}
	return cards, rows.Err()
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

// BulkCreateCards inserts every input in one transaction so a mid-batch
// failure leaves the deck untouched. Returns the created cards in the same
// order as the inputs.
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

	insertedIDs := make([]int64, 0, len(inputs))
	for _, in := range inputs {
		id, err := insertCardTx(tx, deckID, in)
		if err != nil {
			return nil, err
		}
		insertedIDs = append(insertedIDs, id)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	committed = true

	created := make([]models.Card, 0, len(insertedIDs))
	for _, id := range insertedIDs {
		c, err := s.GetCard(id)
		if err != nil {
			return nil, err
		}
		created = append(created, *c)
	}
	return created, nil
}

func insertCardTx(tx *sql.Tx, deckID int64, in CardInput) (int64, error) {
	blanksJSON, choicesJSON, err := encodeCardJSON(in)
	if err != nil {
		return 0, err
	}
	res, err := tx.Exec(
		`INSERT INTO cards(deck_id,type,language,prompt,initial_code,expected_answer,blanks_data,choices)
		 VALUES(?,?,?,?,?,?,?,?)`,
		deckID, string(in.Type), in.Language, in.Prompt, in.InitialCode, in.ExpectedAnswer, blanksJSON, choicesJSON,
	)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
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
