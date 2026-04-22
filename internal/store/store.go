// Package store is the persistence layer. Methods on *Store are the only
// allowed path from domain logic to SQL — screens and business code never
// touch database/sql directly. Files split by domain (decks, cards,
// reviews, sessions, settings, stats) sharing the patch builder and time
// helpers defined here.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrNotFound distinguishes missing rows from transport-level failures.
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

// parseTime accepts both the JS web app's ISO-8601 variants and Go's RFC3339.
// Returns zero time on unrecognized formats.
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

func startOfLocalDay(t time.Time) time.Time {
	local := t.Local()
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
}

// patch builds a conditional UPDATE … SET clause, emitting only the columns
// whose values were actually supplied.
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

// setIfPtr is the pointer-aware variant for optional-update callers. Nil
// values are skipped; typed dereferences avoid reflection cost.
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
