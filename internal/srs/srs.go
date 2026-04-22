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

// CalculateNext mirrors the JS SRS in server/src/lib/srs.ts.
func CalculateNext(grade int, ease float64, interval int) Result {
	if ease == 0 {
		ease = 2.5
	}

	newEase := ease
	newInterval := interval

	switch {
	case grade <= 2:
		newInterval = 1
		newEase = math.Max(1.3, ease-0.2)
	case grade == 3:
		v := int(math.Round(float64(interval) * 1.2))
		if v < 1 {
			v = 1
		}
		newInterval = v
	case grade == 4:
		if interval == 0 {
			newInterval = 1
		} else {
			v := int(math.Round(float64(interval) * ease))
			if v < 2 {
				v = 2
			}
			newInterval = v
		}
		newEase = ease + 0.05
	default:
		if interval == 0 {
			newInterval = 2
		} else {
			v := int(math.Round(float64(interval) * ease * 1.3))
			if v < 2 {
				v = 2
			}
			newInterval = v
		}
		newEase = ease + 0.15
	}

	if newInterval > 365 {
		newInterval = 365
	}

	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).
		AddDate(0, 0, newInterval)

	return Result{Ease: newEase, Interval: newInterval, NextDue: next}
}
