package selector

import (
	"image"
	"image/color"
	"testing"
)

func TestCopyImageToBGRARendersOpaqueFrozenFrame(t *testing.T) {
	source := image.NewRGBA(image.Rect(4, 7, 6, 8))
	source.Set(4, 7, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	source.Set(5, 7, color.RGBA{R: 40, G: 50, B: 60, A: 100})
	pixels := make([]byte, 8)
	if err := copyImageToBGRA(pixels, 2, 1, source); err != nil {
		t.Fatal(err)
	}
	want := []byte{30, 20, 10, 255, 60, 50, 40, 255}
	for index := range want {
		if pixels[index] != want[index] {
			t.Fatalf("pixels[%d] = %d, want %d", index, pixels[index], want[index])
		}
	}
}

func TestCopyImageToBGRARejectsMismatchedBuffer(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 2, 1))
	if err := copyImageToBGRA(make([]byte, 4), 2, 1, source); err == nil {
		t.Fatal("copyImageToBGRA() accepted a short buffer")
	}
}
