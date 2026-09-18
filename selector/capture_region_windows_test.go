//go:build windows

package selector

import (
	"image"
	"image/color"
	"testing"

	"screenshot-win/editor"
)

func TestResizeCaptureRegionHandlesAndBounds(t *testing.T) {
	original := image.Rect(20, 30, 120, 100)
	bounds := image.Rect(0, 0, 160, 130)
	for _, test := range []struct {
		handle editor.TransformHandle
		delta  image.Point
		want   image.Rectangle
	}{
		{editor.HandleRectangleNorthWest, image.Pt(5, 7), image.Rect(25, 37, 120, 100)},
		{editor.HandleRectangleNorth, image.Pt(99, 7), image.Rect(20, 37, 120, 100)},
		{editor.HandleRectangleNorthEast, image.Pt(5, 7), image.Rect(20, 37, 125, 100)},
		{editor.HandleRectangleEast, image.Pt(5, 99), image.Rect(20, 30, 125, 100)},
		{editor.HandleRectangleSouthEast, image.Pt(5, 7), image.Rect(20, 30, 125, 107)},
		{editor.HandleRectangleSouth, image.Pt(99, 7), image.Rect(20, 30, 120, 107)},
		{editor.HandleRectangleSouthWest, image.Pt(5, 7), image.Rect(25, 30, 120, 107)},
		{editor.HandleRectangleWest, image.Pt(5, 99), image.Rect(25, 30, 120, 100)},
	} {
		if got := resizeCaptureRegion(original, test.handle, test.delta, bounds, 16); got != test.want {
			t.Errorf("handle %v: got %v, want %v", test.handle, got, test.want)
		}
	}
	if got := resizeCaptureRegion(original, editor.HandleRectangleNorthWest, image.Pt(1000, 1000), bounds, 16); got != image.Rect(104, 84, 120, 100) {
		t.Fatalf("minimum clamp = %v", got)
	}
	if got := resizeCaptureRegion(original, editor.HandleRectangleSouthEast, image.Pt(1000, 1000), bounds, 16); got != image.Rect(20, 30, 160, 130) {
		t.Fatalf("desktop clamp = %v", got)
	}
}

func TestCaptureRegionLayoutAlwaysHasEightHandles(t *testing.T) {
	state := &frozenState{dpi: 96}
	if layout := state.captureRegionLayout(image.Rect(4, 5, 20, 21)); layout.Count != 8 {
		t.Fatalf("handle count = %d", layout.Count)
	}
}

func TestEditableCaptureCropsDocumentWithoutMovingAnnotations(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 100, 80))
	document, err := editor.NewDocument(source)
	if err != nil {
		t.Fatal(err)
	}
	style := editor.Style{Color: color.NRGBA{R: 255, A: 255}, Width: 3}
	if _, err := document.Add(editor.Annotation{Tool: editor.ToolRectangle, Start: image.Pt(35, 35), End: image.Pt(75, 60), Style: style}); err != nil {
		t.Fatal(err)
	}
	state := &frozenState{
		selectionState: &selectionState{desktop: image.Rect(-100, 20, 0, 100)},
		region:         image.Rect(40, 30, 70, 55), document: document, editableRegion: true,
	}
	region, output := state.capture()
	if region != image.Rect(-60, 50, -30, 75) || output.Bounds() != image.Rect(0, 0, 30, 25) {
		t.Fatalf("region=%v bounds=%v", region, output.Bounds())
	}
	if got := color.NRGBAModel.Convert(output.At(0, 0)).(color.NRGBA); got.R == 255 {
		t.Fatal("annotation outside the crop leaked into the output origin")
	}
	if got := color.NRGBAModel.Convert(output.At(0, 5)).(color.NRGBA); got.R == 0 {
		t.Fatal("intersecting annotation was not retained in the crop")
	}
}
