//go:build windows

package selector

import (
	"image"
	"testing"
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
