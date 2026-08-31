package selector

import (
	"image"
	"testing"
)

func TestDragRectangleNormalizesEveryDirection(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 80)
	want := image.Rect(20, 10, 70, 60)
	for _, points := range [][2]image.Point{
		{{20, 10}, {70, 60}},
		{{70, 10}, {20, 60}},
		{{20, 60}, {70, 10}},
		{{70, 60}, {20, 10}},
	} {
		if got := dragRectangle(bounds, points[0], points[1]); got != want {
			t.Errorf("dragRectangle(%v, %v) = %v, want %v", points[0], points[1], got, want)
		}
	}
}

func TestDragRectangleClampsToBoundsWithNegativeCoordinates(t *testing.T) {
	bounds := image.Rect(-200, -100, 300, 250)
	if got, want := dragRectangle(bounds, image.Pt(-400, -200), image.Pt(500, 400)), bounds; got != want {
		t.Fatalf("dragRectangle() = %v, want %v", got, want)
	}
}

func TestDragRectangleAllowsZeroSize(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 80)
	if got := dragRectangle(bounds, image.Pt(20, 20), image.Pt(20, 20)); !got.Empty() {
		t.Fatalf("dragRectangle() = %v, want empty", got)
	}
}

func TestDesktopRectangleTranslatesClientCoordinates(t *testing.T) {
	desktop := image.Rect(-1920, -200, 2560, 1440)
	if got, want := desktopRectangle(desktop, image.Pt(100, 50), image.Pt(900, 650)), image.Rect(-1820, -150, -1020, 450); got != want {
		t.Fatalf("desktopRectangle() = %v, want %v", got, want)
	}
}
