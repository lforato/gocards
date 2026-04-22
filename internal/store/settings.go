package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
)

const DefaultDailyLimit = 50

// GetSetting normalizes JSON-encoded strings or numbers into a plain string.
// ok=false means the row is missing (nil error).
func (s *Store) GetSetting(key string) (string, bool, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}

	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err == nil {
		switch v := decoded.(type) {
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
		return DefaultDailyLimit
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return DefaultDailyLimit
	}
	return n
}
