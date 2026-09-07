package selector

import (
	"image"
	"testing"
)

func TestCandidateExtent(t *testing.T) {
	small := image.Rect(40, 40, 60, 60)
	wide := image.Rect(0, 40, 100, 60)
	outer := image.Rect(0, 0, 120, 120)
	rs := []image.Rectangle{outer, wide, small}
	var e candidateExtent
	check := func(p image.Point, want image.Rectangle) {
		t.Helper()
		got, ok := e.at(rs, p)
		if got != want || ok != !want.Empty() {
			t.Fatalf("at %v: got %v, %v; want %v", p, got, ok, want)
		}
	}
	check(image.Pt(50, 50), small)
	e.down(image.Pt(50, 50))
	check(image.Pt(80, 50), wide)
	e.down(image.Pt(80, 50)) // Auto-repeat must not move the anchor.
	check(image.Pt(50, 50), small)
	check(image.Pt(20, 20), outer) // Reverse drag.
	e.held = false
	check(image.Pt(20, 20), outer)              // Release and click retain the extent.
	check(image.Pt(50, 50), outer)              // Movement inside the choice keeps it locked.
	check(image.Pt(120, 50), image.Rectangle{}) // Leaving the choice resumes hover.
	check(image.Pt(50, 50), small)              // Re-entering uses ordinary hover.
	e.down(image.Pt(50, 50))
	check(image.Pt(120, 50), image.Rectangle{}) // Exclusive boundary, no enclosure.
}

func TestCandidateExtentRequiresBothCorners(t *testing.T) {
	var e candidateExtent
	e.down(image.Pt(10, 10))
	r, ok := e.at([]image.Rectangle{image.Rect(40, 40, 60, 60)}, image.Pt(50, 50))
	if ok || !r.Empty() {
		t.Fatalf("selected rectangle that excludes anchor: %v", r)
	}
}

func TestCandidateExtentReleasePreservesChoice(t *testing.T) {
	small := image.Rect(40, 40, 60, 60)
	wide := image.Rect(0, 40, 100, 60)
	outer := image.Rect(0, 0, 120, 120)
	var e candidateExtent
	e.down(image.Pt(50, 50))
	e.at([]image.Rectangle{small, wide, outer}, image.Pt(80, 50))
	e.held = false
	got, _ := e.at([]image.Rectangle{small, wide, outer}, image.Pt(50, 50))
	if got != wide {
		t.Fatalf("inside movement changed choice to %v", got)
	}
	g := candidateGesture{threshold: 4}
	g.down(image.Pt(50, 50), got)
	if r, ok := g.up(image.Pt(50, 50), outer); !ok || r != wide {
		t.Fatalf("click got %v, %v", r, ok)
	}
	got, _ = e.at([]image.Rectangle{small, wide, outer}, image.Pt(50, 60))
	if got != outer {
		t.Fatalf("leaving choice got %v", got)
	}
	e.down(image.Pt(50, 50))
	got, _ = e.at([]image.Rectangle{small, wide, outer}, image.Pt(51, 51))
	if got != small {
		t.Fatalf("new Tab gesture got %v", got)
	}
}
