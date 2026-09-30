package selector

import (
	"image"
	"testing"
)

func TestArrowFormatMenuHitTargetsAndBounds(t *testing.T) {
	for _, dpi := range []int{0, 96, 120, 144, 192} {
		size := arrowFormatPanelSize(dpi)
		padding, cell := scaleForDPICommon(4, dpi), scaleForDPICommon(36, dpi)
		for i := range arrowFormats {
			point := image.Pt(padding+i*cell+cell/2, padding+cell/2)
			got, ok := arrowFormatOptionAt(point, dpi)
			if !ok || got != i {
				t.Fatalf("dpi %d option %d = %d %v", dpi, i, got, ok)
			}
		}
		for _, point := range []image.Point{image.Pt(-1, 0), image.Pt(size.X, size.Y/2), image.Pt(size.X/2, size.Y), image.Pt(padding+cell*len(arrowFormats), size.Y/2)} {
			if _, ok := arrowFormatOptionAt(point, dpi); ok {
				t.Fatalf("dpi %d outside point %v accepted", dpi, point)
			}
		}
		work := image.Rect(-600, 0, 200, 600)
		below := stylePanelBounds(image.Rect(-100, 100, -60, 140), work, size, 4)
		above := stylePanelBounds(image.Rect(160, 550, 200, 590), work, size, 4)
		if below.Min.Y != 144 || !below.In(work) {
			t.Fatalf("below=%v", below)
		}
		if above.Max.Y != 546 || !above.In(work) {
			t.Fatalf("above=%v", above)
		}
	}
}

func TestArrowFormatDefaultsAndToolbarLayoutCompatibility(t *testing.T) {
	if currentArrowAction(ActionCancel) != ActionArrow || arrowFormatIndex(ActionCancel) != 0 {
		t.Fatal("new toolbar should default to arrow")
	}
	for i, format := range arrowFormats {
		if currentArrowAction(format.action) != format.action || arrowFormatIndex(format.action) != i {
			t.Fatalf("format %v is not retained", format)
		}
	}
	for _, actions := range [][]Action{selectionToolbarActions, annotationToolbarActions} {
		count := 0
		for _, action := range actions {
			if arrowFormatAction(action) {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("toolbar has %d arrow slots", count)
		}
	}
}
