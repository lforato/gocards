package ai

import (
	"testing"

	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/store"
)

func TestOrderByStrugglePrioritizesHardest(t *testing.T) {
	cards := []models.Card{
		{ID: 1, Prompt: "solid"},      // high avg grade
		{ID: 2, Prompt: "struggling"}, // low avg grade + fails
		{ID: 3, Prompt: "new"},        // no reviews
		{ID: 4, Prompt: "shaky"},      // mid avg grade
	}
	stats := map[int64]store.CardStat{
		1: {CardID: 1, ReviewCount: 5, AvgGrade: 4.6},
		2: {CardID: 2, ReviewCount: 6, AvgGrade: 2.1, FailCount: 4},
		4: {CardID: 4, ReviewCount: 4, AvgGrade: 3.3, FailCount: 1},
		// id 3 has no stat entry → zero value → TierNew
	}
	got := OrderByStruggle(cards, stats)
	if len(got) != 4 {
		t.Fatalf("expected 4 ordered cards, got %d", len(got))
	}
	wantIDs := []int64{2, 4, 1, 3}
	for i, cc := range got {
		if cc.Card.ID != wantIDs[i] {
			t.Errorf("slot %d: want id %d, got %d (%s)", i, wantIDs[i], cc.Card.ID, cc.Tier.Key)
		}
	}
	if got[0].Tier != TierStruggling {
		t.Errorf("slot 0 tier: want %q, got %q", TierStruggling.Key, got[0].Tier.Key)
	}
	if got[3].Tier != TierNew {
		t.Errorf("slot 3 tier: want %q, got %q", TierNew.Key, got[3].Tier.Key)
	}
}

func TestClassifyStruggleBoundaries(t *testing.T) {
	cases := []struct {
		name string
		st   store.CardStat
		want StruggleTier
	}{
		{"never reviewed", store.CardStat{}, TierNew},
		{"very low avg", store.CardStat{ReviewCount: 1, AvgGrade: 2.0}, TierStruggling},
		{"just below 3", store.CardStat{ReviewCount: 1, AvgGrade: 2.9}, TierStruggling},
		{"at 3", store.CardStat{ReviewCount: 1, AvgGrade: 3.0}, TierShaky},
		{"just below 4", store.CardStat{ReviewCount: 1, AvgGrade: 3.99}, TierShaky},
		{"at 4", store.CardStat{ReviewCount: 1, AvgGrade: 4.0}, TierSolid},
	}
	for _, tc := range cases {
		if got := classifyStruggle(tc.st); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got.Key, tc.want.Key)
		}
	}
}
