// Package models holds the domain types shared across every layer: decks,
// cards (and their type-specific payloads), reviews, study sessions, and
// the card-kind registry that drives extension points. These types are
// JSON-compatible with the sibling web app's schema.
package models

import "time"

type CardType string

const (
	CardMCQ  CardType = "mcq"
	CardCode CardType = "code"
	CardFill CardType = "fill"
	CardExp  CardType = "exp"
)

type Choice struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	IsCorrect bool   `json:"isCorrect"`
}

type BlankData struct {
	Template string   `json:"template"`
	Blanks   []string `json:"blanks"`
}

type Deck struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Color       string    `json:"color"`
	Language    string    `json:"language"`
	CreatedAt   time.Time `json:"created_at"`
}

type Card struct {
	ID             int64      `json:"id"`
	DeckID         int64      `json:"deck_id"`
	Type           CardType   `json:"type"`
	Language       string     `json:"language"`
	Prompt         string     `json:"prompt"`
	InitialCode    string     `json:"initial_code"`
	ExpectedAnswer string     `json:"expected_answer"`
	BlanksData     *BlankData `json:"blanks_data,omitempty"`
	Choices        []Choice   `json:"choices,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type Review struct {
	ID         int64     `json:"id"`
	CardID     int64     `json:"card_id"`
	Grade      int       `json:"grade"`
	ReviewedAt time.Time `json:"reviewed_at"`
	NextDue    time.Time `json:"next_due"`
	Ease       float64   `json:"ease"`
	Interval   int       `json:"interval"`
}

type StudySession struct {
	ID            int64      `json:"id"`
	DeckID        int64      `json:"deck_id"`
	StartedAt     time.Time  `json:"started_at"`
	EndedAt       *time.Time `json:"ended_at,omitempty"`
	CardsReviewed int        `json:"cards_reviewed"`
}

type GradingMessage struct {
	Role    string `json:"role"` // "user" | "assistant"
	Content string `json:"content"`
}

type DeckWithCounts struct {
	Deck
	CardCount int
	DueCount  int
}
