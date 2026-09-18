//go:build windows

package selector

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"testing"

	"screenshot-win/editor"
)

func rectangleTestState(t *testing.T, dpi int) (*frozenState, editor.Annotation) {
	t.Helper()
	bounds := image.Rect(0, 0, 260, 200)
	source := image.NewRGBA(bounds)
	for y := 0; y < 200; y++ {
		for x := 0; x < 260; x++ {
			source.SetRGBA(x, y, color.RGBA{uint8(x), uint8(y), uint8(x + y), 255})
		}
	}
	region := image.Rect(20, 20, 240, 180)
	doc, err := editor.NewDocument(source.SubImage(region))
	if err != nil {
		t.Fatal(err)
	}
	a := editor.Annotation{Tool: editor.ToolRectangle, Start: image.Pt(0, 0), End: image.Pt(180, 120), Style: editor.DefaultStyle()}
	a.Style.Width = 8
	id, err := doc.Add(a)
	if err != nil {
		t.Fatal(err)
	}
	a, _ = doc.Get(id)
	_, err = doc.Add(editor.Annotation{Tool: editor.ToolRectangle, Start: image.Pt(80, 0), End: image.Pt(160, 140), Style: editor.Style{Color: color.NRGBA{20, 200, 60, 255}, Width: 3}})
	if err != nil {
		t.Fatal(err)
	}
	state := &frozenState{
		selectionState: &selectionState{client: bounds, pixels: make([]byte, 260*200*4)},
		source:         source, region: region, document: doc, viewport: editor.Viewport{Scale: 1, Offset: region.Min}, dpi: dpi, selected: id,
	}
	if err := copyImageToBGRA(state.pixels, 260, 200, source); err != nil {
		t.Fatal(err)
	}
	state.copyViewport(editor.RenderViewport(doc.Rendered(), state.viewport, region), region)
	drawOuterPixelBorder(state.pixels, 260, bounds, region)
	return state, a
}

func TestRectangleChromeRestoresOutsideCaptureAndAllDPI(t *testing.T) {
	for _, dpi := range []int{96, 120, 144, 168, 192} {
		t.Run(fmt.Sprint(dpi), func(t *testing.T) {
			s, a := rectangleTestState(t, dpi)
			original := append([]byte(nil), s.pixels...)
			s.drawSelectedRectangle(a)
			outsideChanged := false
			for y := 0; y < s.client.Dy(); y++ {
				for x := 0; x < s.client.Dx(); x++ {
					i := (y*s.client.Dx() + x) * 4
					if !image.Pt(x, y).In(s.region) && !bytes.Equal(original[i:i+4], s.pixels[i:i+4]) {
						outsideChanged = true
					}
				}
			}
			if !outsideChanged {
				t.Fatal("edge handles were clipped to capture region")
			}
			if err := s.clearSelectionOverlay(a); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(original, s.pixels) {
				t.Fatal("deselection left gaps, rings, or exterior chrome")
			}
		})
	}
}

func TestRectangleIncrementalTransformMatchesFullScene(t *testing.T) {
	for _, dpi := range []int{96, 120, 144, 168, 192} {
		s, a := rectangleTestState(t, dpi)
		s.drawSelectedRectangle(a)
		tr := &frozenTransform{id: a.ID, original: a, previous: a, draft: a}
		for _, end := range []image.Point{{150, 110}, {18, 18}, {210, 150}, {40, 80}} {
			tr.draft.Start = image.Pt(5, 5)
			tr.draft.End = end
			s.paintRectangleTransform(tr)
			actual := append([]byte(nil), s.pixels...)
			// Rebuild the complete scene independently of the dirty footprint.
			if err := copyImageToBGRA(s.pixels, s.client.Dx(), s.client.Dy(), s.source); err != nil {
				t.Fatal(err)
			}
			preview := s.document.NewRectanglePreview(a.ID)
			preview.Update(tr.draft, s.viewport, dpi)
			for y := s.region.Min.Y; y < s.region.Max.Y; y++ {
				for x := s.region.Min.X; x < s.region.Max.X; x++ {
					c := preview.ScreenColor(image.Pt(x, y))
					i := (y*s.client.Dx() + x) * 4
					s.pixels[i], s.pixels[i+1], s.pixels[i+2], s.pixels[i+3] = c.B, c.G, c.R, 255
				}
			}
			drawOuterPixelBorder(s.pixels, s.client.Dx(), s.client, s.region)
			s.drawRectangleHandles(tr.draft)
			if !bytes.Equal(actual, s.pixels) {
				t.Fatalf("dpi=%d end=%v incremental frame differs from full scene", dpi, end)
			}
			tr.previous = tr.draft
		}
	}
}

func TestRectangleTransformStableFrameAllocations(t *testing.T) {
	s, a := rectangleTestState(t, 144)
	tr := &frozenTransform{id: a.ID, original: a, previous: a, draft: a}
	s.paintRectangleTransform(tr)
	if allocs := testing.AllocsPerRun(10, func() { s.paintRectangleTransform(tr) }); allocs != 0 {
		t.Fatalf("rectangle frame allocations=%v", allocs)
	}
}

func TestRectangleMinimumAndOutsideHandleHit(t *testing.T) {
	s, a := rectangleTestState(t, 144)
	if got := s.rectangleMinimum(); got != image.Pt(24, 24) {
		t.Fatalf("minimum=%v", got)
	}
	s.viewport.Scale = .5
	if got := s.rectangleMinimum(); got != image.Pt(48, 48) {
		t.Fatalf("zoom minimum=%v", got)
	}
	s.viewport.Scale = 1
	if got, ok := s.handleAt(image.Pt(15, 15), a); !ok || got != editor.HandleRectangleNorthWest {
		t.Fatal("outside portion of corner handle cannot be hit")
	}
	if got := s.cursorAt(image.Pt(15, 15)); got != idcSizeNWSE {
		t.Fatal("cursor and outside handle hit disagree")
	}
	s.selected = 0
	s.request = &frozenAnnotationRequest{tool: editor.ToolRectangle}
	if got := s.cursorAt(image.Pt(55, 20)); got != idcSizeAll {
		t.Fatal("unselected shape should expose move cursor even with drawing tool active")
	}
	if got := s.cursorAt(image.Pt(55, 60)); got != idcCross {
		t.Fatal("rectangle interior should remain drawing space")
	}
}
