package selector

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

func TestFrozenCandidateRendererMatchesOriginal(t *testing.T) {
	rgba := image.NewRGBA(image.Rect(7, 11, 100, 90))
	for y := 11; y < 90; y++ {
		for x := 7; x < 100; x++ {
			rgba.SetRGBA(x, y, color.RGBA{uint8(x * 3), uint8(y * 5), uint8(x + y), uint8(x * y)})
		}
	}
	sources := []image.Image{rgba, rgba.SubImage(image.Rect(15, 20, 85, 75)), image.NewUniform(color.NRGBA{R: 120, G: 200, B: 80, A: 180})}
	// Use bounded generic images to exercise the non-RGBA conversion path.
	generic := image.NewNRGBA(image.Rect(-3, -4, 67, 51))
	for y := -4; y < 51; y++ {
		for x := -3; x < 67; x++ {
			generic.Set(x, y, sources[2].At(x, y))
		}
	}
	sources[2] = generic
	for _, source := range sources {
		renderer := newFrozenCandidateRenderer(source)
		actual := make([]byte, source.Bounds().Dx()*source.Bounds().Dy()*4)
		expected := make([]byte, len(actual))
		for _, r := range []image.Rectangle{
			{}, image.Rect(0, 0, 93, 79), image.Rect(4, 5, 30, 40), image.Rect(4, 5, 30, 40),
			image.Rect(20, 25, 65, 60), image.Rect(1, 1, 2, 2), image.Rect(-10, -20, 15, 25),
			image.Rect(200, 200, 210, 210), image.Rect(5, 5, 5, 20), {},
		} {
			renderer.draw(actual, r)
			referenceFrozenCandidate(expected, source, r)
			if !bytes.Equal(actual, expected) {
				t.Fatalf("%T bounds=%v selection=%v differs", source, source.Bounds(), r)
			}
		}
	}
}

func BenchmarkFrozenCandidateRedraw(b *testing.B) {
	source := image.NewRGBA(image.Rect(0, 0, 5120, 1440))
	selections := []image.Rectangle{image.Rect(2560, 0, 5120, 1440), image.Rect(3789, 407, 4904, 1035)}
	b.Run("original", func(b *testing.B) {
		pixels := make([]byte, 5120*1440*4)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			referenceFrozenCandidate(pixels, source, selections[i%2])
		}
	})
	b.Run("cached", func(b *testing.B) {
		pixels := make([]byte, 5120*1440*4)
		renderer := newFrozenCandidateRenderer(source)
		renderer.draw(pixels, selections[1])
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			renderer.draw(pixels, selections[i%2])
		}
	})
}

func referenceFrozenCandidate(pixels []byte, source image.Image, selection image.Rectangle) {
	b := source.Bounds()
	w := b.Dx()
	rgba, fast := source.(*image.RGBA)
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < w; x++ {
			var r, g, blue uint32
			if fast {
				i := rgba.PixOffset(x+b.Min.X, y+b.Min.Y)
				r = uint32(rgba.Pix[i]) * 257
				g = uint32(rgba.Pix[i+1]) * 257
				blue = uint32(rgba.Pix[i+2]) * 257
			} else {
				r, g, blue, _ = source.At(x+b.Min.X, y+b.Min.Y).RGBA()
			}
			if !image.Pt(x, y).In(selection) {
				r = r * 207 / 255
				g = g * 207 / 255
				blue = blue * 207 / 255
			}
			i := (y*w + x) * 4
			pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = byte(blue>>8), byte(g>>8), byte(r>>8), 255
		}
	}
	selection = selection.Intersect(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := selection.Min.Y; y < selection.Max.Y; y++ {
		for x := selection.Min.X; x < selection.Max.X; x++ {
			if x < selection.Min.X+2 || x >= selection.Max.X-2 || y < selection.Min.Y+2 || y >= selection.Max.Y-2 {
				i := (y*w + x) * 4
				pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = 255, 140, 22, 255
			}
		}
	}
}
