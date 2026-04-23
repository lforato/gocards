package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/lforato/gocards/internal/db"
	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/srs"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	p := filepath.Join(t.TempDir(), "t.db")
	conn, err := db.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return New(conn)
}

func TestDeckCRUD(t *testing.T) {
	s := openTestStore(t)
	d, err := s.CreateDeck("Algo", "algorithms", "#10b981", "en")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetDeck(d.ID)
	if err != nil || got.Name != "Algo" {
		t.Fatalf("get: %v %+v", err, got)
	}

	name := "Algorithms"
	if _, err := s.UpdateDeck(d.ID, &name, nil, nil); err != nil {
		t.Fatal(err)
	}
	got2, _ := s.GetDeck(d.ID)
	if got2.Name != "Algorithms" {
		t.Fatalf("update: %s", got2.Name)
	}
}

func TestCardsAndDue(t *testing.T) {
	s := openTestStore(t)
	d, _ := s.CreateDeck("JS", "", "#f59e0b", "en")
	ins, err := s.BulkCreateCards(d.ID, []CardInput{
		{Type: models.CardCode, Language: "js", Prompt: "p1", ExpectedAnswer: "a"},
		{Type: models.CardMCQ, Language: "js", Prompt: "p2", Choices: []models.Choice{{ID: "a", Text: "A", IsCorrect: true}}},
	})
	if err != nil || len(ins) != 2 {
		t.Fatalf("bulk insert: %v %d", err, len(ins))
	}

	cards, _ := s.ListCards(d.ID)
	if len(cards) != 2 {
		t.Fatalf("list: %d", len(cards))
	}

	due, _ := s.DueCards(d.ID, 50)
	if len(due) != 2 {
		t.Fatalf("due should include unreviewed cards, got %d", len(due))
	}

	// After a strong review, card should no longer be due.
	r := srs.CalculateNext(5, 2.5, 0)
	if _, err := s.CreateReview(cards[0].ID, 5, r.Ease, r.Interval, r.NextDue); err != nil {
		t.Fatal(err)
	}
	due2, _ := s.DueCards(d.ID, 50)
	if len(due2) != 1 {
		t.Fatalf("after review, due should be 1, got %d", len(due2))
	}
}

func TestSettings(t *testing.T) {
	s := openTestStore(t)
	if err := s.SetSetting("dailyLimit", 25); err != nil {
		t.Fatal(err)
	}
	if v, ok, _ := s.GetSetting("dailyLimit"); !ok || v != "25" {
		t.Fatalf("got %q ok=%v", v, ok)
	}
	if n := s.DailyLimit(); n != 25 {
		t.Fatalf("DailyLimit = %d", n)
	}
}

func TestStreakAndActivity(t *testing.T) {
	s := openTestStore(t)
	d, _ := s.CreateDeck("x", "", "#f59e0b", "en")
	cs, _ := s.BulkCreateCards(d.ID, []CardInput{
		{Type: models.CardCode, Language: "js", Prompt: "p"},
	})
	now := time.Now()
	if _, err := s.CreateReview(cs[0].ID, 5, 2.5, 1, now.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	streak, err := s.Streak()
	if err != nil || streak < 1 {
		t.Fatalf("streak: %v %d", err, streak)
	}
	n, _ := s.ReviewsToday()
	if n < 1 {
		t.Fatalf("reviews today: %d", n)
	}
	ret, _ := s.Retention()
	if ret == 0 {
		t.Fatalf("retention should be >0")
	}
	act, _ := s.Activity()
	if len(act) == 0 {
		t.Fatalf("activity empty")
	}
}
