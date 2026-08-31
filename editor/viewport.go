package editor

import (
	"image"
	"math"
)

const MinScale, MaxScale = 0.05, 16.0

// Viewport maps original-image coordinates to a window canvas. Offset is the
// canvas position of image coordinate (0,0).
type Viewport struct {
	Scale  float64
	Offset image.Point
}

func Fit(imageSize, canvasSize image.Point) Viewport {
	if imageSize.X <= 0 || imageSize.Y <= 0 || canvasSize.X <= 0 || canvasSize.Y <= 0 {
		return Viewport{Scale: 1}
	}
	scale := math.Min(1, math.Min(float64(canvasSize.X)/float64(imageSize.X), float64(canvasSize.Y)/float64(imageSize.Y)))
	size := image.Pt(int(math.Round(float64(imageSize.X)*scale)), int(math.Round(float64(imageSize.Y)*scale)))
	return Viewport{Scale: scale, Offset: image.Pt((canvasSize.X-size.X)/2, (canvasSize.Y-size.Y)/2)}
}

func (viewport Viewport) ImageToScreen(point image.Point) image.Point {
	return image.Pt(viewport.Offset.X+int(math.Round(float64(point.X)*viewport.scale())), viewport.Offset.Y+int(math.Round(float64(point.Y)*viewport.scale())))
}

func (viewport Viewport) ScreenToImage(point image.Point) image.Point {
	scale := viewport.scale()
	return image.Pt(int(math.Round(float64(point.X-viewport.Offset.X)/scale)), int(math.Round(float64(point.Y-viewport.Offset.Y)/scale)))
}

func (viewport *Viewport) Pan(delta image.Point) { viewport.Offset = viewport.Offset.Add(delta) }

// ZoomAt changes scale while keeping the image coordinate beneath anchor fixed.
func (viewport *Viewport) ZoomAt(anchor image.Point, factor float64) {
	if viewport == nil || factor <= 0 {
		return
	}
	before := viewport.ScreenToImage(anchor)
	viewport.Scale = math.Max(MinScale, math.Min(MaxScale, viewport.scale()*factor))
	viewport.Offset = image.Pt(anchor.X-int(math.Round(float64(before.X)*viewport.Scale)), anchor.Y-int(math.Round(float64(before.Y)*viewport.Scale)))
}

func (viewport Viewport) VisibleImage(canvas image.Rectangle, bounds image.Rectangle) image.Rectangle {
	minimum := viewport.ScreenToImage(canvas.Min)
	maximum := viewport.ScreenToImage(canvas.Max)
	return image.Rect(min(minimum.X, maximum.X)-1, min(minimum.Y, maximum.Y)-1, max(minimum.X, maximum.X)+1, max(minimum.Y, maximum.Y)+1).Intersect(bounds)
}

func (viewport Viewport) scale() float64 {
	if viewport.Scale <= 0 {
		return 1
	}
	return viewport.Scale
}

// RenderViewport allocates only the visible canvas, regardless of source height.
func RenderViewport(source image.Image, viewport Viewport, canvas image.Rectangle) *image.NRGBA {
	result := image.NewNRGBA(image.Rect(0, 0, canvas.Dx(), canvas.Dy()))
	if source == nil {
		return result
	}
	for y := 0; y < result.Bounds().Dy(); y++ {
		for x := 0; x < result.Bounds().Dx(); x++ {
			point := viewport.ScreenToImage(image.Pt(canvas.Min.X+x, canvas.Min.Y+y))
			if point.In(source.Bounds()) {
				result.Set(x, y, source.At(point.X, point.Y))
			}
		}
	}
	return result
}
