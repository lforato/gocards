package srs

import "testing"

func TestFailResetsInterval(t *testing.T) {
	r := CalculateNext(1, 2.5, 20)
	if r.Interval != 1 {
		t.Fatalf("grade 1 should reset interval, got %d", r.Interval)
	}
	if r.Ease >= 2.5 {
		t.Fatalf("ease should drop, got %f", r.Ease)
	}
}

func TestGoodGrowsInterval(t *testing.T) {
	r := CalculateNext(5, 2.5, 4)
	if r.Interval <= 4 {
		t.Fatalf("grade 5 should grow interval beyond %d, got %d", 4, r.Interval)
	}
	if r.Ease <= 2.5 {
		t.Fatalf("ease should climb on 5, got %f", r.Ease)
	}
}

func TestCap(t *testing.T) {
	r := CalculateNext(5, 4.0, 400)
	if r.Interval > 365 {
		t.Fatalf("interval cap exceeded: %d", r.Interval)
	}
}
