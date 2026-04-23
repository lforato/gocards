package db

import (
	"database/sql"
	"encoding/json"
)

// ensureSeed is run on every Open. It tops up default settings (idempotent
// via INSERT OR IGNORE) and, on the very first launch, inserts the tutorial
// decks. Tutorial decks are created once; if the user deletes them they
// stay gone — `decksExist` keeps this behavior stable across restarts.
func ensureSeed(conn *sql.DB) error {
	if err := seedDefaultSettings(conn); err != nil {
		return err
	}
	if seeded, err := decksExist(conn); err != nil || seeded {
		return err
	}
	return seedTutorialDecks(conn)
}

func seedDefaultSettings(conn *sql.DB) error {
	defaults := map[string]any{
		"dailyLimit":         50,
		"preferredLanguages": "javascript,typescript,python",
		"apiKey":             "",
		"language":           "en",
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

// tutorialCard is the seed-file shape for a card. It mirrors the runtime
// models.Card but stays local so the db package doesn't depend on models.
type tutorialCard struct {
	Type           string
	Language       string
	Prompt         string
	InitialCode    string
	ExpectedAnswer string
	Choices        []map[string]any
	Blanks         map[string]any
}

type tutorialDeck struct {
	Name        string
	Description string
	Color       string
	Language    string
	Cards       []tutorialCard
}

func seedTutorialDecks(conn *sql.DB) error {
	for _, d := range tutorialDecks() {
		id, err := insertDeck(conn, d.Name, d.Description, d.Color, d.Language)
		if err != nil {
			return err
		}
		for _, c := range d.Cards {
			if err := insertTutorialCard(conn, id, c); err != nil {
				return err
			}
		}
	}
	return nil
}

func insertTutorialCard(conn *sql.DB, deckID int64, c tutorialCard) error {
	var choices, blanks sql.NullString
	if c.Choices != nil {
		raw, err := json.Marshal(c.Choices)
		if err != nil {
			return err
		}
		choices = sql.NullString{String: string(raw), Valid: true}
	}
	if c.Blanks != nil {
		raw, err := json.Marshal(c.Blanks)
		if err != nil {
			return err
		}
		blanks = sql.NullString{String: string(raw), Valid: true}
	}
	_, err := conn.Exec(
		`INSERT INTO cards(deck_id, type, language, prompt, initial_code, expected_answer, choices, blanks_data)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		deckID, c.Type, c.Language, c.Prompt, c.InitialCode, c.ExpectedAnswer, choices, blanks,
	)
	return err
}

func insertDeck(conn *sql.DB, name, desc, color, language string) (int64, error) {
	res, err := conn.Exec(
		`INSERT INTO decks(name, description, color, language) VALUES(?, ?, ?, ?)`,
		name, desc, color, language,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// tutorialDecks returns the first-run tutorial content in every supported
// language. Each deck walks the user through the four card types and the
// core dashboard controls, then invites them to delete it.
func tutorialDecks() []tutorialDeck {
	return []tutorialDeck{tutorialDeckEN(), tutorialDeckPtBR()}
}

func tutorialDeckEN() tutorialDeck {
	return tutorialDeck{
		Name:        "Welcome to gocards",
		Description: "A quick tour of the app. Delete this deck (D on the dashboard) when you're done.",
		Color:       "#8b5cf6",
		Language:    "en",
		Cards: []tutorialCard{
			{
				Type:           "mcq",
				Language:       "english",
				Prompt:         "What is gocards?",
				ExpectedAnswer: "A terminal flashcards app with spaced repetition",
				Choices: []map[string]any{
					{"id": "a", "text": "A paint program", "isCorrect": false},
					{"id": "b", "text": "A terminal flashcards app with spaced repetition", "isCorrect": true},
					{"id": "c", "text": "A database GUI", "isCorrect": false},
					{"id": "d", "text": "A Git client", "isCorrect": false},
				},
			},
			{
				Type:           "mcq",
				Language:       "english",
				Prompt:         "From the dashboard, which key starts a study session on the selected deck?",
				ExpectedAnswer: "S",
				Choices: []map[string]any{
					{"id": "a", "text": "Q", "isCorrect": false},
					{"id": "b", "text": "S", "isCorrect": true},
					{"id": "c", "text": "X", "isCorrect": false},
					{"id": "d", "text": "space", "isCorrect": false},
				},
			},
			{
				Type:           "fill",
				Language:       "english",
				Prompt:         "After each card you grade your recall from 1 to 4. A grade of ___BLANK___ means you nailed it.",
				ExpectedAnswer: "4",
				Blanks: map[string]any{
					"template": "1 = blackout   2 = hard   3 = good   ___BLANK___ = easy",
					"blanks":   []string{"4"},
				},
			},
			{
				Type:           "code",
				Language:       "javascript",
				Prompt:         "Write a function `greet(name)` that returns the string 'Hello, ' followed by the given name.",
				InitialCode:    "function greet(name) {\n  // your code here\n}",
				ExpectedAnswer: "function greet(name) {\n  return 'Hello, ' + name;\n}",
			},
			{
				Type:           "exp",
				Language:       "javascript",
				Prompt:         "In one sentence, explain what this snippet does.",
				InitialCode:    "[1, 2, 3].map(n => n * 2);",
				ExpectedAnswer: "It doubles each number in the array, producing [2, 4, 6].",
			},
			{
				Type:           "mcq",
				Language:       "english",
				Prompt:         "Tutorial complete! How do you delete this deck when you're ready to start fresh?",
				ExpectedAnswer: "From the dashboard, highlight it and press D",
				Choices: []map[string]any{
					{"id": "a", "text": "From the dashboard, highlight it and press D", "isCorrect": true},
					{"id": "b", "text": "Uninstall the app", "isCorrect": false},
					{"id": "c", "text": "It can't be deleted", "isCorrect": false},
					{"id": "d", "text": "Edit the database by hand", "isCorrect": false},
				},
			},
		},
	}
}

func tutorialDeckPtBR() tutorialDeck {
	return tutorialDeck{
		Name:        "Bem-vindo ao gocards",
		Description: "Um tour rápido pelo app. Apague este deck (D no dashboard) quando terminar.",
		Color:       "#10b981",
		Language:    "pt-BR",
		Cards: []tutorialCard{
			{
				Type:           "mcq",
				Language:       "portuguese",
				Prompt:         "O que é o gocards?",
				ExpectedAnswer: "Um app de flashcards no terminal com repetição espaçada",
				Choices: []map[string]any{
					{"id": "a", "text": "Um programa de pintura", "isCorrect": false},
					{"id": "b", "text": "Um app de flashcards no terminal com repetição espaçada", "isCorrect": true},
					{"id": "c", "text": "Uma interface de banco de dados", "isCorrect": false},
					{"id": "d", "text": "Um cliente Git", "isCorrect": false},
				},
			},
			{
				Type:           "mcq",
				Language:       "portuguese",
				Prompt:         "No dashboard, qual tecla inicia uma sessão de estudo no deck selecionado?",
				ExpectedAnswer: "S",
				Choices: []map[string]any{
					{"id": "a", "text": "Q", "isCorrect": false},
					{"id": "b", "text": "S", "isCorrect": true},
					{"id": "c", "text": "X", "isCorrect": false},
					{"id": "d", "text": "espaço", "isCorrect": false},
				},
			},
			{
				Type:           "fill",
				Language:       "portuguese",
				Prompt:         "Depois de cada carta você avalia seu recall de 1 a 4. A nota ___BLANK___ significa que você mandou muito bem.",
				ExpectedAnswer: "4",
				Blanks: map[string]any{
					"template": "1 = apagou   2 = difícil   3 = bom   ___BLANK___ = fácil",
					"blanks":   []string{"4"},
				},
			},
			{
				Type:           "code",
				Language:       "javascript",
				Prompt:         "Escreva uma função `saudar(nome)` que retorna a string 'Olá, ' seguida do nome recebido.",
				InitialCode:    "function saudar(nome) {\n  // seu código aqui\n}",
				ExpectedAnswer: "function saudar(nome) {\n  return 'Olá, ' + nome;\n}",
			},
			{
				Type:           "exp",
				Language:       "javascript",
				Prompt:         "Em uma frase, explique o que este trecho faz.",
				InitialCode:    "[1, 2, 3].map(n => n * 2);",
				ExpectedAnswer: "Dobra cada número do array, resultando em [2, 4, 6].",
			},
			{
				Type:           "mcq",
				Language:       "portuguese",
				Prompt:         "Tutorial concluído! Como apagar este deck quando quiser começar do zero?",
				ExpectedAnswer: "No dashboard, selecione o deck e pressione D",
				Choices: []map[string]any{
					{"id": "a", "text": "No dashboard, selecione o deck e pressione D", "isCorrect": true},
					{"id": "b", "text": "Desinstale o app", "isCorrect": false},
					{"id": "c", "text": "Não dá para apagar", "isCorrect": false},
					{"id": "d", "text": "Edite o banco manualmente", "isCorrect": false},
				},
			},
		},
	}
}
