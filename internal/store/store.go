package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lforato/gocards/internal/models"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

const tsLayout = "2006-01-02T15:04:05.000Z"
const tsLayoutAlt = "2006-01-02T15:04:05Z"

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{tsLayout, tsLayoutAlt, time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func formatTime(t time.Time) string {
	return t.UTC().Format(tsLayout)
}

// -------- Decks --------

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
	sets := []string{}
	args := []any{}
	if name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *name)
	}
	if description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *description)
	}
	if color != nil {
		sets = append(sets, "color = ?")
		args = append(args, *color)
	}
	if len(sets) == 0 {
		return s.GetDeck(id)
	}
	args = append(args, id)
	q := fmt.Sprintf(`UPDATE decks SET %s WHERE id = ?`, strings.Join(sets, ", "))
	if _, err := s.db.Exec(q, args...); err != nil {
		return nil, err
	}
	return s.GetDeck(id)
}

func (s *Store) DeleteDeck(id int64) error {
	_, err := s.db.Exec(`DELETE FROM decks WHERE id = ?`, id)
	return err
}

// -------- Cards --------

func scanCard(scanner interface {
	Scan(dest ...any) error
}) (models.Card, error) {
	var c models.Card
	var blanks, choices sql.NullString
	var ts string
	err := scanner.Scan(
		&c.ID, &c.DeckID, &c.Type, &c.Language,
		&c.Prompt, &c.InitialCode, &c.ExpectedAnswer, &blanks, &choices, &ts,
	)
	if err != nil {
		return c, err
	}
	c.CreatedAt = parseTime(ts)
	if blanks.Valid && blanks.String != "" && blanks.String != "null" {
		var bd models.BlankData
		if err := json.Unmarshal([]byte(blanks.String), &bd); err == nil {
			c.BlanksData = &bd
		}
	}
	if choices.Valid && choices.String != "" && choices.String != "null" {
		var cs []models.Choice
		if err := json.Unmarshal([]byte(choices.String), &cs); err == nil {
			c.Choices = cs
		}
	}
	return c, nil
}

const cardCols = "id,deck_id,type,language,prompt,initial_code,expected_answer,blanks_data,choices,created_at"

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

type CardInput struct {
	Type           models.CardType
	Language       string
	Prompt         string
	InitialCode    string
	ExpectedAnswer string
	BlanksData     *models.BlankData
	Choices        []models.Choice
}

func (s *Store) BulkCreateCards(deckID int64, inputs []CardInput) ([]models.Card, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	ids := []int64{}
	for _, in := range inputs {
		var blanksJSON, choicesJSON any
		if in.BlanksData != nil {
			b, _ := json.Marshal(in.BlanksData)
			blanksJSON = string(b)
		}
		if in.Choices != nil {
			b, _ := json.Marshal(in.Choices)
			choicesJSON = string(b)
		}
		res, err := tx.Exec(
			`INSERT INTO cards(deck_id,type,language,prompt,initial_code,expected_answer,blanks_data,choices)
             VALUES(?,?,?,?,?,?,?,?)`,
			deckID, string(in.Type), in.Language, in.Prompt, in.InitialCode, in.ExpectedAnswer, blanksJSON, choicesJSON,
		)
		if err != nil {
			return nil, err
		}
		id, _ := res.LastInsertId()
		ids = append(ids, id)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

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

func (s *Store) UpdateCard(id int64, in CardInput) (*models.Card, error) {
	var blanksJSON, choicesJSON any
	if in.BlanksData != nil {
		b, _ := json.Marshal(in.BlanksData)
		blanksJSON = string(b)
	}
	if in.Choices != nil {
		b, _ := json.Marshal(in.Choices)
		choicesJSON = string(b)
	}
	_, err := s.db.Exec(
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

// -------- Reviews --------

func (s *Store) CreateReview(cardID int64, grade int, ease float64, interval int, nextDue time.Time) (*models.Review, error) {
	res, err := s.db.Exec(
		`INSERT INTO reviews(card_id,grade,ease,interval,next_due) VALUES(?,?,?,?,?)`,
		cardID, grade, ease, interval, formatTime(nextDue),
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	var r models.Review
	var ts, ndue string
	err = s.db.QueryRow(
		`SELECT id,card_id,grade,reviewed_at,next_due,ease,interval FROM reviews WHERE id = ?`, id,
	).Scan(&r.ID, &r.CardID, &r.Grade, &ts, &ndue, &r.Ease, &r.Interval)
	if err != nil {
		return nil, err
	}
	r.ReviewedAt = parseTime(ts)
	r.NextDue = parseTime(ndue)
	return &r, nil
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

// -------- Sessions --------

func (s *Store) CreateSession(deckID int64) (*models.StudySession, error) {
	res, err := s.db.Exec(`INSERT INTO study_sessions(deck_id) VALUES(?)`, deckID)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
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
	sets := []string{}
	args := []any{}
	if cardsReviewed != nil {
		sets = append(sets, "cards_reviewed = ?")
		args = append(args, *cardsReviewed)
	}
	if clearEnded {
		sets = append(sets, "ended_at = NULL")
	} else if ended != nil {
		sets = append(sets, "ended_at = ?")
		args = append(args, formatTime(*ended))
	}
	if len(sets) == 0 {
		return s.GetSession(id)
	}
	args = append(args, id)
	q := fmt.Sprintf(`UPDATE study_sessions SET %s WHERE id = ?`, strings.Join(sets, ", "))
	if _, err := s.db.Exec(q, args...); err != nil {
		return nil, err
	}
	return s.GetSession(id)
}

// -------- Settings --------

func (s *Store) GetSetting(key string) (string, bool, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}

	// Stored as JSON-encoded string or number; normalize to a string.
	var any any
	if err := json.Unmarshal([]byte(raw), &any); err == nil {
		switch v := any.(type) {
		case string:
			return v, true, nil
		case float64:
			if v == float64(int64(v)) {
				return strconv.FormatInt(int64(v), 10), true, nil
			}
			return strconv.FormatFloat(v, 'f', -1, 64), true, nil
		}
	}
	return raw, true, nil
}

func (s *Store) SetSetting(key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, string(raw),
	)
	return err
}

func (s *Store) DailyLimit() int {
	v, ok, _ := s.GetSetting("dailyLimit")
	if !ok {
		return 50
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 50
	}
	return n
}

// -------- Stats --------

func (s *Store) Streak() (int, error) {
	rows, err := s.db.Query(`SELECT reviewed_at FROM reviews ORDER BY reviewed_at DESC`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	days := map[string]struct{}{}
	for rows.Next() {
		var ts string
		if err := rows.Scan(&ts); err != nil {
			return 0, err
		}
		t := parseTime(ts).Local()
		days[t.Format("2006-01-02")] = struct{}{}
	}

	streak := 0
	cur := time.Now().Local()
	for {
		if _, ok := days[cur.Format("2006-01-02")]; !ok {
			return streak, nil
		}
		streak++
		cur = cur.AddDate(0, 0, -1)
	}
}

func (s *Store) ReviewsToday() (int, error) {
	now := time.Now().Local()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM reviews WHERE reviewed_at >= ?`, formatTime(start.UTC()),
	).Scan(&n)
	return n, err
}

func (s *Store) Retention() (int, error) {
	var total, good int
	err := s.db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN grade >= 4 THEN 1 ELSE 0 END), 0) FROM reviews`,
	).Scan(&total, &good)
	if err != nil {
		return 0, err
	}
	if total == 0 {
		return 0, nil
	}
	return int(float64(good) / float64(total) * 100.0), nil
}

func (s *Store) DueToday() (int, error) {
	decks, err := s.ListDecks()
	if err != nil {
		return 0, err
	}
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

	out := map[string]int{}
	for rows.Next() {
		var ts string
		if err := rows.Scan(&ts); err != nil {
			return nil, err
		}
		t := parseTime(ts).Local()
		out[t.Format("2006-01-02")]++
	}
	return out, rows.Err()
}

// DeckSummary returns card + due counts per deck (used by dashboard).
func (s *Store) DeckSummaries() ([]models.DeckWithCounts, error) {
	decks, err := s.ListDecks()
	if err != nil {
		return nil, err
	}
	limit := s.DailyLimit()
	out := make([]models.DeckWithCounts, 0, len(decks))
	for _, d := range decks {
		cc, err := s.CountCards(d.ID)
		if err != nil {
			return nil, err
		}
		due, err := s.DueCards(d.ID, limit)
		if err != nil {
			return nil, err
		}
		out = append(out, models.DeckWithCounts{Deck: d, CardCount: cc, DueCount: len(due)})
	}
	return out, nil
}
