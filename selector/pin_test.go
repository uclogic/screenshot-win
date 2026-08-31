package selector

import (
	"image"
	"math"
	"testing"
)

func TestPinZoomBoundsKeepsCursorAnchored(t *testing.T) {
	bounds := image.Rect(100, 200, 500, 400)
	cursor := image.Pt(200, 250)
	got, scale := pinZoomBounds(bounds, image.Pt(400, 200), cursor, 120)
	if math.Abs(scale-1.1) > 0.0001 {
		t.Fatalf("scale = %v, want 1.1", scale)
	}
	if got != image.Rect(90, 195, 530, 415) {
		t.Fatalf("bounds = %v", got)
	}
	oldX := float64(cursor.X-bounds.Min.X) / float64(bounds.Dx())
	newX := float64(cursor.X-got.Min.X) / float64(got.Dx())
	if math.Abs(oldX-newX) > 0.001 {
		t.Fatalf("cursor image position moved from %v to %v", oldX, newX)
	}
}

func TestPinZoomBoundsClampsScale(t *testing.T) {
	original := image.Pt(100, 50)
	got, scale := pinZoomBounds(image.Rect(0, 0, 10, 5), original, image.Pt(5, 2), -12000)
	if scale != pinMinimumScale || got.Size() != image.Pt(10, 5) {
		t.Fatalf("minimum zoom = (%v, %v)", got, scale)
	}
	got, scale = pinZoomBounds(image.Rect(0, 0, 800, 400), original, image.Pt(400, 200), 12000)
	if scale != pinMaximumScale || got.Size() != image.Pt(800, 400) {
		t.Fatalf("maximum zoom = (%v, %v)", got, scale)
	}
}

func TestPinResetBoundsUsesOriginalSizeAndCenter(t *testing.T) {
	got := pinResetBounds(image.Rect(100, 100, 300, 200), image.Pt(80, 40))
	if want := image.Rect(160, 130, 240, 170); got != want {
		t.Fatalf("pinResetBounds() = %v, want %v", got, want)
	}
}

func TestPinInitialBoundsFitsAndClampsToWorkArea(t *testing.T) {
	got := pinInitialBounds(image.Pt(2000, 1000), image.Pt(1800, 900), image.Rect(0, 0, 1920, 1080))
	if want := image.Rect(384, 312, 1920, 1080); got != want {
		t.Fatalf("pinInitialBounds() = %v, want %v", got, want)
	}
}

func TestPinInitialBoundsFitsVeryTallCaptureBelowNormalMinimumScale(t *testing.T) {
	got := pinInitialBounds(image.Pt(1000, 100000), image.Point{}, image.Rect(0, 0, 1920, 1080))
	if got.Dy() > 864 || got.Dx() < 1 {
		t.Fatalf("very tall initial bounds = %v", got)
	}
	zoomed, scale := pinZoomBounds(got, image.Pt(1000, 100000), got.Min, 120)
	if scale > pinMinimumScale || zoomed.Dy() <= got.Dy() {
		t.Fatalf("first zoom jumped or failed: %v scale=%v", zoomed, scale)
	}
}
