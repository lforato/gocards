package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrNotFound is returned by single-row lookups (GetDeck, GetCard, etc.) when
// the row doesn't exist. Callers can use errors.Is to distinguish from
// transport-level failures.
var ErrNotFound = errors.New("not found")

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

const (
	tsLayout    = "2006-01-02T15:04:05.000Z"
	tsLayoutAlt = "2006-01-02T15:04:05Z"
)

// parseTime accepts the ISO-8601 variants the JS web app writes plus Go's
// canonical RFC3339. Returns zero time on empty input or unrecognized format.
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

// startOfLocalDay returns midnight of t in the local timezone.
func startOfLocalDay(t time.Time) time.Time {
	local := t.Local()
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
}

// patch accumulates conditional UPDATE column assignments so update methods
// don't rebuild the `SET col = ?, col = ?, …` string by hand. Only columns
// whose values were actually provided end up in the final statement.
type patch struct {
	sets []string
	args []any
}

func newPatch() *patch { return &patch{} }

func (p *patch) set(col string, val any) {
	p.sets = append(p.sets, col+" = ?")
	p.args = append(p.args, val)
}

func (p *patch) setRaw(col, literal string) {
	p.sets = append(p.sets, col+" = "+literal)
}

// setIfPtr appends col = *v only when v is non-nil, centralizing the nil-check
// every optional-update caller was duplicating.
func (p *patch) setIfPtr(col string, v any) {
	switch val := v.(type) {
	case *string:
		if val != nil {
			p.set(col, *val)
		}
	case *int:
		if val != nil {
			p.set(col, *val)
		}
	}
}

func (p *patch) exec(db *sql.DB, table string, id int64) error {
	if len(p.sets) == 0 {
		return nil
	}
	q := fmt.Sprintf(`UPDATE %s SET %s WHERE id = ?`, table, strings.Join(p.sets, ", "))
	args := append(p.args, id)
	_, err := db.Exec(q, args...)
	return err
}
