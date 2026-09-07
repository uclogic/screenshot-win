package selector

import "image"

type SelectionOptions struct {
	Mode     CandidateMode
	Desktop  image.Rectangle
	Snapshot image.Image
	// BeforeClose prepares the next window while the selection overlay is visible.
	BeforeClose func(image.Rectangle) error
}

// frozenCandidateRenderer caches the immutable screenshot in the DIB's BGRA
// format. Its destination must retain the previous frame between calls.
type frozenCandidateRenderer struct {
	bounds         image.Rectangle
	normal, dimmed []byte
	previous       image.Rectangle
	initialized    bool
}

func newFrozenCandidateRenderer(source image.Image) *frozenCandidateRenderer {
	b := source.Bounds()
	w, h := b.Dx(), b.Dy()
	renderer := &frozenCandidateRenderer{
		bounds: image.Rect(0, 0, w, h),
		normal: make([]byte, w*h*4),
		dimmed: make([]byte, w*h*4),
	}
	rgba, fast := source.(*image.RGBA)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, blue uint32
			if fast {
				i := rgba.PixOffset(x+b.Min.X, y+b.Min.Y)
				r, g, blue = uint32(rgba.Pix[i])*257, uint32(rgba.Pix[i+1])*257, uint32(rgba.Pix[i+2])*257
			} else {
				r, g, blue, _ = source.At(x+b.Min.X, y+b.Min.Y).RGBA()
			}
			i := (y*w + x) * 4
			renderer.normal[i], renderer.normal[i+1], renderer.normal[i+2], renderer.normal[i+3] = byte(blue>>8), byte(g>>8), byte(r>>8), 255
			renderer.dimmed[i], renderer.dimmed[i+1], renderer.dimmed[i+2], renderer.dimmed[i+3] = byte((blue*207/255)>>8), byte((g*207/255)>>8), byte((r*207/255)>>8), 255
		}
	}
	return renderer
}

func (renderer *frozenCandidateRenderer) draw(pixels []byte, selection image.Rectangle) {
	selection = selection.Intersect(renderer.bounds)
	if renderer.initialized && selection == renderer.previous {
		return
	}
	if !renderer.initialized {
		copy(pixels, renderer.dimmed)
		renderer.initialized = true
	} else {
		renderer.copyRectangle(pixels, renderer.dimmed, renderer.previous)
	}
	renderer.copyRectangle(pixels, renderer.normal, selection)
	// Visit only the border strips, never the selection interior.
	for _, edge := range []image.Rectangle{
		image.Rect(selection.Min.X, selection.Min.Y, selection.Max.X, min(selection.Min.Y+2, selection.Max.Y)),
		image.Rect(selection.Min.X, max(selection.Min.Y, selection.Max.Y-2), selection.Max.X, selection.Max.Y),
		image.Rect(selection.Min.X, selection.Min.Y, min(selection.Min.X+2, selection.Max.X), selection.Max.Y),
		image.Rect(max(selection.Min.X, selection.Max.X-2), selection.Min.Y, selection.Max.X, selection.Max.Y),
	} {
		for y := edge.Min.Y; y < edge.Max.Y; y++ {
			for x := edge.Min.X; x < edge.Max.X; x++ {
				i := (y*renderer.bounds.Dx() + x) * 4
				pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = 255, 140, 22, 255
			}
		}
	}
	renderer.previous = selection
}

func (renderer *frozenCandidateRenderer) copyRectangle(destination, source []byte, r image.Rectangle) {
	width := renderer.bounds.Dx()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		start, end := (y*width+r.Min.X)*4, (y*width+r.Max.X)*4
		copy(destination[start:end], source[start:end])
	}
}

func drawFrozenCandidate(pixels []byte, source image.Image, selection image.Rectangle) {
	newFrozenCandidateRenderer(source).draw(pixels, selection)
}
