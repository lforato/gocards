package ai

import (
	"sort"

	"github.com/lforato/gocards/internal/models"
	"github.com/lforato/gocards/internal/store"
)

// StruggleTier buckets a card by how much the user is struggling with it.
// Lower priority values surface higher in the cheatsheet.
type StruggleTier struct {
	Key      string // stable identifier used in templates ("struggling", "shaky", "solid", "new")
	Label    string // human label rendered in section headings
	Emoji    string // single-cell visual marker
	Priority int    // 0 = hardest, higher = easier
}

var (
	TierStruggling = StruggleTier{Key: "struggling", Label: "Struggling", Emoji: "🔥", Priority: 0}
	TierShaky      = StruggleTier{Key: "shaky", Label: "Shaky", Emoji: "🟡", Priority: 1}
	TierSolid      = StruggleTier{Key: "solid", Label: "Solid", Emoji: "✅", Priority: 2}
	TierNew        = StruggleTier{Key: "new", Label: "New", Emoji: "💤", Priority: 3}
)

// CheatsheetCard pairs a card with the struggle tier used to order and label
// its section in the generated cheatsheet.
type CheatsheetCard struct {
	Card  models.Card
	Tier  StruggleTier
	Stats store.CardStat
}

// classifyStruggle bins a card's review stats into a tier. A card that has
// never been reviewed lands in TierNew regardless of avg/fail counts.
func classifyStruggle(st store.CardStat) StruggleTier {
	if st.ReviewCount == 0 {
		return TierNew
	}
	switch {
	case st.AvgGrade < 3.0:
		return TierStruggling
	case st.AvgGrade < 4.0:
		return TierShaky
	default:
		return TierSolid
	}
}

// OrderByStruggle returns CheatsheetCards ordered hardest-first. Ties are
// broken by fail count (more failures → higher) then review count → the most
// ground-out cards surface above rarely-reviewed ones at the same tier.
func OrderByStruggle(cards []models.Card, stats map[int64]store.CardStat) []CheatsheetCard {
	out := make([]CheatsheetCard, 0, len(cards))
	for _, c := range cards {
		st := stats[c.ID]
		out = append(out, CheatsheetCard{Card: c, Tier: classifyStruggle(st), Stats: st})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Tier.Priority != b.Tier.Priority {
			return a.Tier.Priority < b.Tier.Priority
		}
		if a.Stats.FailCount != b.Stats.FailCount {
			return a.Stats.FailCount > b.Stats.FailCount
		}
		if a.Stats.ReviewCount != b.Stats.ReviewCount {
			return a.Stats.ReviewCount > b.Stats.ReviewCount
		}
		if a.Stats.AvgGrade != b.Stats.AvgGrade {
			return a.Stats.AvgGrade < b.Stats.AvgGrade
		}
		return a.Card.ID < b.Card.ID
	})
	return out
}
