//go:build windows

package selector

import (
	"image"
	"image/color"
	"testing"
)

func TestReplacingPinImageUpdatesCopyAndPaintWithoutChangingZoom(t *testing.T) {
	original := image.NewRGBA(image.Rect(0, 0, 3, 2))
	state := &pinWindowState{scale: 2.5}
	if err := state.setSource(original); err != nil {
		t.Fatal(err)
	}
	state.softwareRaster = true
	updated := image.NewRGBA(original.Bounds())
	updated.SetRGBA(0, 0, color.RGBA{R: 23, G: 67, B: 101, A: 255})
	if err := state.setSource(updated); err != nil {
		t.Fatal(err)
	}
	if state.source != updated || state.original != original.Bounds().Size() || state.scale != 2.5 {
		t.Fatal("replacement changed zoom or retained the old source used by Copy")
	}
	if state.pixels[0] != 101 || state.pixels[1] != 67 || state.pixels[2] != 23 || state.pixels[3] != 255 || state.softwareRaster {
		t.Fatal("replacement retained old paint pixels or fallback state")
	}
	if err := state.setSource(nil); err == nil || state.source != updated || state.pixels[0] != 101 {
		t.Fatal("invalid replacement discarded the current pin")
	}
}

func TestDrawPinWithFallback(t *testing.T) {
	for _, test := range []struct {
		name    string
		results []int32
		wantErr bool
	}{
		{"success", []int32{20}, false},
		{"zero scan lines retries", []int32{0, 20}, false},
		{"GDI error retries", []int32{-1, 20}, false},
		{"both attempts draw nothing", []int32{0, 0}, true},
		{"both attempts fail", []int32{-1, -1}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			err := drawPinWithFallback(func(mode uintptr) int32 {
				if calls >= len(test.results) {
					t.Fatal("unexpected extra drawing attempt")
				}
				wantMode := uintptr(dibStretchHalftone)
				if calls > 0 {
					wantMode = 3
				}
				if mode != wantMode {
					t.Fatalf("stretch mode = %d, want %d", mode, wantMode)
				}
				result := test.results[calls]
				calls++
				return result
			})
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, test.wantErr)
			}
			if calls != len(test.results) {
				t.Fatalf("drawing attempts = %d, want %d", calls, len(test.results))
			}
		})
	}
}
