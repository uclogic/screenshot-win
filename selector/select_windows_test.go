//go:build windows

package selector

import (
	"image"
	"testing"
	"time"
)

func TestDrawSelectionOverlayDimsOnlyOutsideSelection(t *testing.T) {
	const width, height = 12, 10
	pixels := make([]byte, width*height*4)
	selection := image.Rect(2, 2, 10, 8)
	drawSelectionOverlay(pixels, width, selection, true)

	if got := selectionPixel(pixels, width, image.Pt(0, 0)); got != [4]byte{0, 0, 0, selectionShadeAlpha} {
		t.Fatalf("outside pixel = %v, want translucent shade", got)
	}
	if got := selectionPixel(pixels, width, image.Pt(5, 5)); got != [4]byte{0, 0, 0, 1} {
		t.Fatalf("selection interior pixel = %v, want transparent", got)
	}
	if got := selectionPixel(pixels, width, image.Pt(2, 4)); got != [4]byte{0xff, 0x8c, 0x16, 0xff} {
		t.Fatalf("selection border pixel = %v, want blue border", got)
	}
}

func TestDrawSelectionOverlayDimsWholeDesktopBeforeDrag(t *testing.T) {
	const width, height = 5, 4
	pixels := make([]byte, width*height*4)
	drawSelectionOverlay(pixels, width, image.Rectangle{}, false)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if got := selectionPixel(pixels, width, image.Pt(x, y)); got != [4]byte{0, 0, 0, selectionShadeAlpha} {
				t.Fatalf("pixel (%d,%d) = %v, want translucent shade", x, y, got)
			}
		}
	}
}

func selectionPixel(pixels []byte, width int, point image.Point) [4]byte {
	index := (point.Y*width + point.X) * 4
	return [4]byte{pixels[index], pixels[index+1], pixels[index+2], pixels[index+3]}
}

func TestSelectionHandoffPumpsMessagesAndDefersCancellation(t *testing.T) {
	state := newSelectionState(image.Rect(0, 0, 100, 100))
	state.result, state.selected = image.Rect(5, 5, 80, 80), true
	state.gesture.threshold = 4
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	state.beforeClose = func(image.Rectangle) error {
		close(entered)
		<-release
		return nil
	}
	previous := activeSelection
	activeSelection = state
	defer func() { activeSelection = previous }()
	state.finishSelection()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("window preparation did not start")
	}
	// A second click must not mutate the confirmed region or begin another drag.
	selectionWindowProcedure(0, wmLButtonDown, 0, 20|(20<<16))
	if state.dragging || state.result != image.Rect(5, 5, 80, 80) {
		t.Fatal("handoff accepted another drag")
	}
	selectionWindowProcedure(0, wmCaptureChanged, 0, 0)
	if state.gesture.threshold != 4 {
		t.Fatal("capture loss reset drag threshold")
	}
	selectionWindowProcedure(0, wmKeyDown, vkEscape, 0)
	if !state.handoff.pending() || !state.handoff.cancelled {
		t.Fatal("cancel did not wait for window preparation")
	}
	release <- struct{}{}
	// finish waits for publication, as the queued ready message does in production.
	selectionWindowProcedure(0, wmSelectionReady, 0, 0)
	if state.selected || state.handoff.pending() {
		t.Fatal("cancelled handoff returned a selection")
	}
}
