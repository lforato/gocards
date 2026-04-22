// Package db owns the SQLite connection lifecycle: opening (or creating)
// the database file under ~/.gocards/, applying the embedded schema,
// running idempotent migrations, and seeding a starter deck on first run.
// Returns a ready-to-use *sql.DB for the store package to consume.
package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// migrate holds idempotent ALTER-style patches that sit outside the
// declarative schema.sql. Each branch must check its precondition first.
func migrate(conn *sql.DB) error {
	has, err := hasColumn(conn, "cards", "initial_code")
	if err != nil {
		return err
	}
	if !has {
		if _, err := conn.Exec(`ALTER TABLE cards ADD COLUMN initial_code TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add cards.initial_code: %w", err)
		}
	}
	return nil
}

var knownTables = map[string]struct{}{
	"decks":          {},
	"cards":          {},
	"reviews":        {},
	"study_sessions": {},
	"settings":       {},
}

func hasColumn(conn *sql.DB, table, col string) (bool, error) {
	if _, ok := knownTables[table]; !ok {
		return false, fmt.Errorf("unknown table %q", table)
	}
	rows, err := conn.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if strings.EqualFold(name, col) {
			return true, nil
		}
	}
	return false, rows.Err()
}

//go:embed schema.sql
var schemaSQL string

// Open returns a ready-to-use SQLite handle with schema applied and seed
// rows inserted. Empty path resolves to ~/.gocards/gocards.db.
func Open(path string) (*sql.DB, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir: %w", err)
		}
		dir := filepath.Join(home, ".gocards")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create data dir: %w", err)
		}
		path = filepath.Join(dir, "gocards.db")
	}

	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", path)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	conn.SetMaxOpenConns(1)

	if _, err := conn.Exec(schemaSQL); err != nil {
		conn.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	if err := migrate(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	if err := ensureSeed(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("seed: %w", err)
	}

	return conn, nil
}
