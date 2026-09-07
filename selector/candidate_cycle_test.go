package selector

import (
	"image"
	"testing"
)

func TestCandidateCycle(t *testing.T) {
	small := image.Rect(40, 40, 60, 60)
	wide := image.Rect(0, 40, 100, 60)
	tall := image.Rect(40, 0, 60, 120)
	outer := image.Rect(0, 0, 120, 120)
	rectangles := []image.Rectangle{image.Rect(200, 200, 210, 210), small, wide, tall, outer}
	var cycle candidateCycle
	p := image.Pt(50, 50)
	for _, tc := range []struct {
		name  string
		point image.Point
		step  int
		want  image.Rectangle
		ok    bool
	}{
		{"smallest", p, 0, small, true},
		{"wider", p, 1, wide, true},
		{"refresh preserves choice", p, 0, wide, true},
		{"taller", p, 1, tall, true},
		{"outer", p, 1, outer, true},
		{"wrap forward", p, 1, small, true},
		{"wrap backward", p, -1, outer, true},
		{"previous", p, -1, tall, true},
		{"pointer resets", image.Pt(51, 50), 0, small, true},
		{"no hit", image.Pt(300, 300), 1, image.Rectangle{}, false},
		{"single hit", image.Pt(110, 110), -1, outer, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := cycle.at(rectangles, tc.point, tc.step)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("got %v, %v; want %v, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
	// A click refresh must keep the cycled rectangle for gesture confirmation.
	cycle.at(rectangles, p, 0)
	chosen, _ := cycle.at(rectangles, p, 1)
	refreshed, _ := cycle.at(rectangles, p, 0)
	g := candidateGesture{threshold: 4}
	g.down(p, refreshed)
	if got, ok := g.up(p, outer); !ok || got != chosen {
		t.Fatalf("click got %v, %v; want %v", got, ok, chosen)
	}
}
