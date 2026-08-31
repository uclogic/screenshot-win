package selector

import (
	"errors"
	"image"
	"testing"
)

func TestPreviewBoundsPrefersRight(t *testing.T) {
	got, placement, ok := previewWindowBounds(image.Rect(100, 100, 500, 700), image.Rect(0, 0, 1920, 1080), image.Pt(120, 378), 8, 8)
	want := image.Rect(508, 100, 628, 478)
	if !ok || placement != previewRight || got != want {
		t.Fatalf("previewWindowBounds() = (%v, %v, %v), want (%v, right, true)", got, placement, ok, want)
	}
}

func TestPreviewBoundsFallsBackToLeft(t *testing.T) {
	got, placement, ok := previewWindowBounds(image.Rect(1700, 200, 1910, 800), image.Rect(0, 0, 1920, 1080), image.Pt(120, 378), 8, 8)
	want := image.Rect(1572, 200, 1692, 578)
	if !ok || placement != previewLeft || got != want {
		t.Fatalf("previewWindowBounds() = (%v, %v, %v), want (%v, left, true)", got, placement, ok, want)
	}
}

func TestPreviewBoundsFallsBackInsideAndShrinksHeight(t *testing.T) {
	got, placement, ok := previewWindowBounds(image.Rect(10, 10, 190, 210), image.Rect(0, 0, 200, 220), image.Pt(120, 378), 8, 8)
	want := image.Rect(40, 18, 160, 202)
	if !ok || placement != previewInside || got != want {
		t.Fatalf("previewWindowBounds() = (%v, %v, %v), want (%v, inside, true)", got, placement, ok, want)
	}
}

func TestPreviewBoundsHidesForNarrowInsideRegion(t *testing.T) {
	_, placement, ok := previewWindowBounds(image.Rect(10, 10, 100, 210), image.Rect(0, 0, 110, 220), image.Pt(120, 77), 8, 8)
	if ok || placement != previewInside {
		t.Fatalf("previewWindowBounds() = placement %v, ok %v; want inside, false", placement, ok)
	}
}

func TestPreviewBoundsSupportsNegativeMonitorAndClampsTop(t *testing.T) {
	got, placement, ok := previewWindowBounds(image.Rect(-1500, -300, -900, 500), image.Rect(-1920, -200, 0, 880), image.Pt(120, 378), 8, 8)
	want := image.Rect(-892, -200, -772, 178)
	if !ok || placement != previewRight || got != want {
		t.Fatalf("previewWindowBounds() = (%v, %v, %v), want (%v, right, true)", got, placement, ok, want)
	}
}

func TestPreviewBoundsUsesScaledDimensions(t *testing.T) {
	got, _, ok := previewWindowBounds(image.Rect(100, 100, 500, 700), image.Rect(0, 0, 1920, 1080), image.Pt(180, 378), 12, 12)
	if !ok || got != image.Rect(512, 100, 692, 478) {
		t.Fatalf("scaled preview bounds = (%v, %v)", got, ok)
	}
}

func TestPreviewUpdateAndCloseLifecycle(t *testing.T) {
	updates := 0
	closed := make(chan struct{})
	preview := &Preview{
		updateImage: func(source image.Image) error {
			updates++
			return nil
		},
		closeWindow: func() { close(closed) },
		done:        closed,
	}
	if err := preview.Update(image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if updates != 1 {
		t.Fatalf("preview updates = %d, want 1", updates)
	}
	preview.Close()
	preview.Close()
	if err := preview.Update(image.NewRGBA(image.Rect(0, 0, 2, 2))); err == nil {
		t.Fatal("closed preview accepted an update")
	}
}

func TestPreviewUpdatePropagatesRenderFailure(t *testing.T) {
	want := errors.New("render failed")
	preview := &Preview{updateImage: func(image.Image) error { return want }}
	if err := preview.Update(image.NewRGBA(image.Rect(0, 0, 1, 1))); !errors.Is(err, want) {
		t.Fatalf("Preview.Update() error = %v, want %v", err, want)
	}
}
