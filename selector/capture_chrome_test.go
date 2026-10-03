package selector

import (
	"image"
	"testing"
)

func TestCaptureBorderAndShade(t *testing.T) {
	const width, height = 20, 16
	selection := image.Rect(3, 3, 17, 13)
	pixels := make([]byte, width*height*4)
	for i := 3; i < len(pixels); i += 4 {
		pixels[i] = captureShadeAlpha
	}
	for y := selection.Min.Y; y < selection.Max.Y; y++ {
		for x := selection.Min.X; x < selection.Max.X; x++ {
			pixels[(y*width+x)*4+3] = 1
		}
	}
	drawCaptureBorder(pixels, width, selection, 96, false)
	get := func(x, y int) [4]byte {
		i := (y*width + x) * 4
		return [4]byte{pixels[i], pixels[i+1], pixels[i+2], pixels[i+3]}
	}
	if got := get(10, 8); got != [4]byte{0, 0, 0, 1} {
		t.Fatalf("interior changed: %v", got)
	}
	if get(3, 3) != get(3, 8) || get(3, 3) != get(10, 3) {
		t.Fatal("corner opacity differs from straight edges")
	}
	if got := get(10, 0); got != [4]byte{0, 0, 0, captureShadeAlpha} {
		t.Fatalf("outside shade changed: %v", got)
	}
	if got := get(3, 8); got != [4]byte{235, 235, 235, 235} {
		t.Fatalf("border color = %v", got)
	}
}

func TestCaptureBorderScalesWithDPI(t *testing.T) {
	for _, tc := range []struct{ dpi, stroke int }{{96, 1}, {144, 2}, {192, 2}} {
		pixels := make([]byte, 30*30*4)
		drawCaptureBorder(pixels, 30, image.Rect(4, 4, 26, 26), tc.dpi, false)
		if got := captureStroke(tc.dpi); got != tc.stroke {
			t.Fatalf("dpi %d stroke = %d", tc.dpi, got)
		}
		for x := 4; x < 4+tc.stroke; x++ {
			if pixels[(15*30+x)*4] == 0 {
				t.Fatalf("dpi %d border gap at x=%d", tc.dpi, x)
			}
		}
		if pixels[(15*30+4+tc.stroke)*4] != 0 {
			t.Fatalf("dpi %d border exceeds stroke", tc.dpi)
		}
	}
}

func TestCaptureBorderPaintsSinglePixelSelection(t *testing.T) {
	pixels := make([]byte, 5*5*4)
	drawCaptureBorder(pixels, 5, image.Rect(2, 2, 3, 3), 192, false)
	if got := pixels[(2*5+2)*4]; got != 235 {
		t.Fatalf("single pixel selection border = %d", got)
	}
}

func TestCaptureLabelFlipsWithinDesktop(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 70)
	for _, tc := range []struct {
		cursor       image.Point
		right, below bool
	}{
		{image.Pt(20, 20), true, true},
		{image.Pt(90, 20), false, true},
		{image.Pt(20, 65), true, false},
		{image.Pt(90, 65), false, false},
	} {
		label := captureLabelBounds(tc.cursor, 24, 28, bounds, 96)
		if !label.Min.In(bounds) || !image.Pt(label.Max.X-1, label.Max.Y-1).In(bounds) || label.Dx() != 24 || label.Dy() != 28 {
			t.Fatalf("cursor %v label %v outside bounds", tc.cursor, label)
		}
		if (label.Min.X > tc.cursor.X) != tc.right || (label.Min.Y > tc.cursor.Y) != tc.below {
			t.Fatalf("cursor %v label %v on wrong side", tc.cursor, label)
		}
	}
}

func TestCaptureCrosshairClipsAtDesktopEdges(t *testing.T) {
	const width, height = 50, 40
	for _, dpi := range []int{96, 144, 192} {
		for _, cursor := range []image.Point{{0, 0}, {49, 0}, {0, 39}, {49, 39}, {25, 20}} {
			pixels := make([]byte, width*height*4)
			for i := 3; i < len(pixels); i += 4 {
				pixels[i] = captureShadeAlpha
			}
			drawCaptureCrosshair(pixels, width, cursor, dpi)
			half, stroke := (13*dpi+48)/96, captureStroke(dpi)
			vertical := image.Rect(cursor.X-stroke, cursor.Y-half, cursor.X+stroke+1, cursor.Y+half+1)
			horizontal := image.Rect(cursor.X-half, cursor.Y-stroke, cursor.X+half+1, cursor.Y+stroke+1)
			for y := 0; y < height; y++ {
				for x := 0; x < width; x++ {
					i := (y*width + x) * 4
					point := image.Pt(x, y)
					got := [4]byte{pixels[i], pixels[i+1], pixels[i+2], pixels[i+3]}
					if !point.In(vertical) && !point.In(horizontal) {
						if got != [4]byte{0, 0, 0, captureShadeAlpha} {
							t.Fatalf("dpi %d cursor %v changed outside pixel %v: %v", dpi, cursor, point, got)
						}
					} else if got[3] <= captureShadeAlpha || got[0] != got[1] || got[1] != got[2] || got[0] > got[3] {
						t.Fatalf("dpi %d cursor %v invalid crosshair pixel %v: %v", dpi, cursor, point, got)
					}
				}
			}
		}
	}
}
