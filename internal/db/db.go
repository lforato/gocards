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

// migrate applies schema changes that can't be expressed with CREATE TABLE IF NOT EXISTS.
// Add ALTER TABLE migrations here, guarded by column checks so they're idempotent.
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

func hasColumn(conn *sql.DB, table, col string) (bool, error) {
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

// Open connects to the gocards sqlite database, applying migrations + seeds.
// If path is empty, defaults to ~/.gocards/gocards.db.
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
