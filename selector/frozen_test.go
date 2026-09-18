package selector

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

type genericFrozenImage struct{ image.Image }

func TestCopyImageToBGRASubimageMatchesGeneric(t *testing.T) {
	source := image.NewRGBA(image.Rect(-5, 7, 12, 16))
	for i := range source.Pix {
		source.Pix[i] = byte(i * 37)
	}
	sub := source.SubImage(image.Rect(-2, 9, 8, 14)).(*image.RGBA)
	got, want := make([]byte, 10*5*4), make([]byte, 10*5*4)
	if err := copyImageToBGRA(got, 10, 5, sub); err != nil {
		t.Fatal(err)
	}
	if err := copyImageToBGRA(want, 10, 5, genericFrozenImage{sub}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("RGBA subimage conversion differs from generic conversion")
	}
}

func BenchmarkCopyImageToBGRA4K(b *testing.B) {
	rgba := image.NewRGBA(image.Rect(0, 0, 3840, 2160))
	for _, tc := range []struct {
		name   string
		source image.Image
	}{{"RGBA", rgba}, {"Generic", genericFrozenImage{rgba}}} {
		b.Run(tc.name, func(b *testing.B) {
			pixels := make([]byte, 3840*2160*4)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := copyImageToBGRA(pixels, 3840, 2160, tc.source); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

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

func TestCropImageUsesZeroBasedBoundsAndSourceOffset(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 8, 6))
	source.SetRGBA(3, 2, color.RGBA{R: 11, G: 22, B: 33, A: 255})
	cropped := cropImage(source, image.Rect(3, 2, 7, 5))
	if cropped == nil || cropped.Bounds() != image.Rect(0, 0, 4, 3) {
		t.Fatalf("cropped bounds = %v", cropped.Bounds())
	}
	if got := color.RGBAModel.Convert(cropped.At(0, 0)).(color.RGBA); got != (color.RGBA{R: 11, G: 22, B: 33, A: 255}) {
		t.Fatalf("cropped origin = %v", got)
	}
}
