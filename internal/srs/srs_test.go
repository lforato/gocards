package srs

import "testing"

func TestAgainResetsIntervalAndDropsEase(t *testing.T) {
	r := ScheduleNext(1, 2.5, 20)
	if r.Interval != 1 {
		t.Fatalf("grade 1 should reset interval to 1, got %d", r.Interval)
	}
	if r.Ease >= 2.5 {
		t.Fatalf("ease should drop below 2.5, got %f", r.Ease)
	}
}

func TestHardKeepsProgressAndLowersEase(t *testing.T) {
	r := ScheduleNext(3, 2.5, 10)
	if r.Interval <= 10 {
		t.Fatalf("Hard should still grow interval (min current+1), got %d", r.Interval)
	}
	if r.Ease >= 2.5 {
		t.Fatalf("Hard should lower ease, got %f", r.Ease)
	}
}

func TestGoodGrowsIntervalAndHoldsEase(t *testing.T) {
	r := ScheduleNext(4, 2.5, 10)
	if r.Interval <= 10 {
		t.Fatalf("Good should grow interval, got %d", r.Interval)
	}
	if r.Ease != 2.5 {
		t.Fatalf("Good should leave ease unchanged, got %f", r.Ease)
	}
}

func TestEasyGrowsIntervalAndRaisesEase(t *testing.T) {
	r := ScheduleNext(5, 2.5, 4)
	if r.Interval <= 4 {
		t.Fatalf("Easy should grow interval beyond %d, got %d", 4, r.Interval)
	}
	if r.Ease <= 2.5 {
		t.Fatalf("Easy should raise ease, got %f", r.Ease)
	}
}

func TestIntervalCappedAt365(t *testing.T) {
	r := ScheduleNext(5, 4.0, 400)
	if r.Interval > 365 {
		t.Fatalf("interval cap exceeded: %d", r.Interval)
	}
}

func TestNewCardSeedIntervals(t *testing.T) {
	if got := ScheduleNext(4, 2.5, 0).Interval; got != 1 {
		t.Fatalf("new card + Good should seed at 1 day, got %d", got)
	}
	if got := ScheduleNext(5, 2.5, 0).Interval; got != 4 {
		t.Fatalf("new card + Easy should seed at 4 days, got %d", got)
	}
}
