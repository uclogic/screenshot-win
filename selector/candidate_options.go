package selector

import "image"

type SelectionOptions struct {
	Mode     CandidateMode
	Desktop  image.Rectangle
	Snapshot image.Image
	// BeforeClose prepares the next window while the selection overlay is visible.
	BeforeClose func(image.Rectangle) error
}

func drawFrozenCandidate(pixels []byte, source image.Image, selection image.Rectangle) {
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
