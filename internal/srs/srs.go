// Package srs implements the spaced-repetition scheduler. ScheduleNext takes
// the current ease/interval and a 1-5 grade and returns the next review's
// ease, interval (in days), and due timestamp. The algorithm mirrors Anki's
// SM-2 so reviews stay consistent with the paired web app.
package srs

import (
	"math"
	"time"
)

// Tunable constants for Anki-style SM-2. Names mirror Anki's deck options so
// anyone familiar with the UI can find the equivalent knob quickly.
const (
	startingEase   = 2.5
	minimumEase    = 1.3
	easyBonus      = 1.3 // extra multiplier applied on top of ease when graded Easy
	maxIntervalDay = 365 // hard cap so a runaway card can't schedule years out

	// Seed intervals applied when a card has never been reviewed (interval == 0).
	// Anki uses "learning steps" for this; we collapse it to one scheduling hop.
	seedIntervalOnPass = 1 // used when Again, Hard, or Good is pressed on a new card
	seedIntervalOnEasy = 4 // used when Easy is pressed on a new card

	// Ease adjustments (deltas applied after each button press).
	easeDeltaAgain = -0.20
	easeDeltaHard  = -0.15
	easeDeltaGood  = 0.00
	easeDeltaEasy  = +0.15

	// Interval multipliers relative to the current interval.
	hardMultiplier = 1.2 // Good and Easy scale by ease instead of a fixed number.
)

// Result is the SM-2 output for one grade press: the updated ease/interval
// and the concrete date when the card is next due (midnight in local time).
type Result struct {
	Ease     float64
	Interval int
	NextDue  time.Time
}

// ScheduleNext applies Anki's SM-2 rules for one review.
//
// Grade mapping (matches Anki's four buttons):
//
//	1-2 → Again : reset to 1 day, ease −0.20
//	3   → Hard  : interval × 1.2 (min current+1), ease −0.15
//	4   → Good  : interval × ease (min current+1), ease unchanged
//	5   → Easy  : interval × ease × 1.3 (min current+1), ease +0.15
//
// New cards (interval == 0) skip the multiplier and use seed intervals:
// Again/Hard/Good → 1 day, Easy → 4 days.
func ScheduleNext(grade int, ease float64, interval int) Result {
	if ease == 0 {
		ease = startingEase
	}

	nextEase, nextInterval := applyGrade(grade, ease, interval)

	if nextInterval > maxIntervalDay {
		nextInterval = maxIntervalDay
	}

	return Result{
		Ease:     nextEase,
		Interval: nextInterval,
		NextDue:  nextMidnightAfter(nextInterval),
	}
}

// applyGrade returns (newEase, newInterval) for the given button press. Kept
// split out so the four cases read top-down without nested control flow.
func applyGrade(grade int, ease float64, interval int) (float64, int) {
	switch {
	case grade <= 2:
		return clampEase(ease + easeDeltaAgain), seedIntervalOnPass
	case grade == 3:
		return clampEase(ease + easeDeltaHard), growInterval(interval, hardMultiplier, seedIntervalOnPass)
	case grade == 4:
		return ease + easeDeltaGood, growInterval(interval, ease, seedIntervalOnPass)
	default: // grade == 5
		return ease + easeDeltaEasy, growInterval(interval, ease*easyBonus, seedIntervalOnEasy)
	}
}

// growInterval grows a non-zero interval by mult, guaranteeing at least one
// day of growth so a card never re-appears on the same day. Zero-interval
// cards jump straight to seedWhenNew.
func growInterval(interval int, mult float64, seedWhenNew int) int {
	if interval == 0 {
		return seedWhenNew
	}
	scaled := int(math.Round(float64(interval) * mult))
	if scaled < interval+1 {
		return interval + 1
	}
	return scaled
}

func clampEase(e float64) float64 {
	if e < minimumEase {
		return minimumEase
	}
	return e
}

// nextMidnightAfter returns local midnight of (today + days). Anki schedules
// cards by calendar day, not by hours, so the hour-of-day of the review
// doesn't drift the next due date around.
func nextMidnightAfter(days int) time.Time {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return today.AddDate(0, 0, days)
}
