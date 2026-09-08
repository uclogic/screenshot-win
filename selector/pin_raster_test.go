package selector

import (
	"errors"
	"image"
	"testing"
)

func TestPinRasterTiles(t *testing.T) {
	original := image.Pt(513, 1100)
	pixels := make([]byte, original.X*original.Y*4)
	for i := range pixels {
		pixels[i] = byte(i*17 + i/251)
	}
	for _, size := range []image.Point{original, image.Pt(301, 601), image.Pt(40000, 80000)} {
		visible := image.Rect(13, 17, 300, 599)
		seen := make(map[image.Point]bool)
		err := drawPinRasterTiles(pixels, original, size, visible, func(tile image.Rectangle, data []byte) error {
			if len(data) > 256*256*4 || len(data) != tile.Dx()*tile.Dy()*4 {
				t.Fatalf("invalid tile buffer: %v, %d", tile, len(data))
			}
			for y := tile.Min.Y; y < tile.Max.Y; y++ {
				for x := tile.Min.X; x < tile.Max.X; x++ {
					p := image.Pt(x, y)
					if seen[p] || !p.In(visible) {
						t.Fatalf("overlapping or out-of-bounds pixel: %v", p)
					}
					seen[p] = true
					src := ((y*original.Y/size.Y)*original.X + x*original.X/size.X) * 4
					dst := ((y-tile.Min.Y)*tile.Dx() + x - tile.Min.X) * 4
					for c := 0; c < 4; c++ {
						if data[dst+c] != pixels[src+c] {
							t.Fatalf("wrong sample at %v, size %v", p, size)
						}
					}
				}
			}
			return nil
		})
		if err != nil || len(seen) != visible.Dx()*visible.Dy() {
			t.Fatalf("incomplete repaint: pixels=%d, error=%v", len(seen), err)
		}
	}
}

func TestPinRasterTilesEmptyAndFailure(t *testing.T) {
	want := errors.New("drawing failed")
	calls := 0
	draw := func(image.Rectangle, []byte) error { calls++; return want }
	if err := drawPinRasterTiles(nil, image.Point{}, image.Point{}, image.Rect(0, 0, 10, 10), draw); err != nil || calls != 0 {
		t.Fatalf("empty paint: calls=%d, error=%v", calls, err)
	}
	err := drawPinRasterTiles(make([]byte, 4), image.Pt(1, 1), image.Pt(600, 600), image.Rect(0, 0, 600, 600), draw)
	if !errors.Is(err, want) || calls != 1 {
		t.Fatalf("failure not propagated: calls=%d, error=%v", calls, err)
	}
}
