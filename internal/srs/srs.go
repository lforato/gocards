// Package srs implements the spaced-repetition scheduler. CalculateNext
// takes the current ease/interval and a 1-5 grade and returns the next
// review's ease, interval (in days), and due timestamp. The algorithm
// mirrors the JS version in the paired web app so reviews stay consistent
// across clients.
package srs

import (
	"math"
	"time"
)

type Result struct {
	Ease     float64
	Interval int
	NextDue  time.Time
}

// CalculateNext implements Anki's SM-2 scheduling algorithm.
//
// Grade mapping (mirrors Anki's four buttons):
//   1-2 → Again : reset to 1 day, ease −0.20
//   3   → Hard  : interval × 1.2 (min current+1), ease −0.15
//   4   → Good  : interval × ease (min current+1), ease unchanged
//   5   → Easy  : interval × ease × 1.3 (min current+1), ease +0.15
//
// New cards (interval == 0) skip the multiplier and get fixed seed intervals:
// Again/Hard → 1 day, Good → 1 day, Easy → 4 days.
func CalculateNext(grade int, ease float64, interval int) Result {
	const (
		minEase   = 1.3
		easyBonus = 1.3
	)
	if ease == 0 {
		ease = 2.5
	}

	newEase := ease
	var newInterval int

	switch {
	case grade <= 2: // Again
		newInterval = 1
		newEase = math.Max(minEase, ease-0.2)

	case grade == 3: // Hard
		newEase = math.Max(minEase, ease-0.15)
		if interval == 0 {
			newInterval = 1
		} else {
			newInterval = max(interval+1, int(math.Round(float64(interval)*1.2)))
		}

	case grade == 4: // Good
		if interval == 0 {
			newInterval = 1
		} else {
			newInterval = max(interval+1, int(math.Round(float64(interval)*ease)))
		}

	default: // Easy (grade 5)
		newEase = ease + 0.15
		if interval == 0 {
			newInterval = 4
		} else {
			newInterval = max(interval+1, int(math.Round(float64(interval)*ease*easyBonus)))
		}
	}

	if newInterval > 365 {
		newInterval = 365
	}

	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).
		AddDate(0, 0, newInterval)

	return Result{Ease: newEase, Interval: newInterval, NextDue: next}
}
