package db

import (
	"database/sql"
	"encoding/json"
)

func ensureSeed(conn *sql.DB) error {
	if err := seedDefaultSettings(conn); err != nil {
		return err
	}
	if seeded, err := decksExist(conn); err != nil || seeded {
		return err
	}
	return seedSampleCards(conn)
}

func seedDefaultSettings(conn *sql.DB) error {
	defaults := map[string]any{
		"dailyLimit":         50,
		"preferredLanguages": "javascript,typescript,python",
		"apiKey":             "",
	}
	for k, v := range defaults {
		raw, _ := json.Marshal(v)
		if _, err := conn.Exec(
			`INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO NOTHING`,
			k, string(raw),
		); err != nil {
			return err
		}
	}
	return nil
}

func decksExist(conn *sql.DB) (bool, error) {
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM decks`).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func seedSampleCards(conn *sql.DB) error {
	jsID, err := insertDeck(conn, "JavaScript Fundamentals", "Core JS concepts every dev should know", "#f59e0b")
	if err != nil {
		return err
	}
	reactID, err := insertDeck(conn, "React Hooks", "useEffect, useMemo, useCallback edge cases", "#3b82f6")
	if err != nil {
		return err
	}

	mcqChoices, _ := json.Marshal([]map[string]any{
		{"id": "a", "text": "\"null\"", "isCorrect": false},
		{"id": "b", "text": "\"object\"", "isCorrect": true},
		{"id": "c", "text": "\"undefined\"", "isCorrect": false},
		{"id": "d", "text": "\"string\"", "isCorrect": false},
	})
	if _, err := conn.Exec(
		`INSERT INTO cards(deck_id,type,language,prompt,expected_answer,choices) VALUES(?,?,?,?,?,?)`,
		jsID, "mcq", "javascript", "What is the result of: typeof null ?", "\"object\"", string(mcqChoices),
	); err != nil {
		return err
	}

	if _, err := conn.Exec(
		`INSERT INTO cards(deck_id,type,language,prompt,expected_answer) VALUES(?,?,?,?,?)`,
		jsID, "code", "javascript",
		"Write a function `debounce(fn, ms)` that returns a debounced version of fn.",
		"function debounce(fn, ms) {\n  let timer;\n  return function(...args) {\n    clearTimeout(timer);\n    timer = setTimeout(() => fn.apply(this, args), ms);\n  };\n}",
	); err != nil {
		return err
	}

	fillBlanks, _ := json.Marshal(map[string]any{
		"template": "// Which runs first?\nPromise.resolve().then(() => console.log('___BLANK___')); // microtask\nsetTimeout(() => console.log('macro'), 0);               // macrotask",
		"blanks":   []string{"micro"},
	})
	if _, err := conn.Exec(
		`INSERT INTO cards(deck_id,type,language,prompt,expected_answer,blanks_data) VALUES(?,?,?,?,?,?)`,
		jsID, "fill", "javascript",
		"Complete the event loop order: microtasks run ___ macrotasks.",
		"before", string(fillBlanks),
	); err != nil {
		return err
	}

	if _, err := conn.Exec(
		`INSERT INTO cards(deck_id,type,language,prompt,expected_answer) VALUES(?,?,?,?,?)`,
		reactID, "code", "typescript",
		"Write a custom hook `useDebounce<T>(value: T, delay: number): T` that debounces a value.",
		"function useDebounce<T>(value: T, delay: number): T {\n  const [debounced, setDebounced] = useState(value);\n  useEffect(() => {\n    const id = setTimeout(() => setDebounced(value), delay);\n    return () => clearTimeout(id);\n  }, [value, delay]);\n  return debounced;\n}",
	); err != nil {
		return err
	}

	return nil
}

func insertDeck(conn *sql.DB, name, desc, color string) (int64, error) {
	res, err := conn.Exec(
		`INSERT INTO decks(name, description, color) VALUES(?, ?, ?)`,
		name, desc, color,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}
