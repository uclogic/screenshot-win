package selector

import "image"

const captureShadeAlpha = 43 // 17% black

func captureStroke(dpi int) int { return max(1, (max(96, dpi)+48)/96) }

// blendCapturePixel composites a straight-alpha color into a premultiplied
// BGRA layered-window pixel. Opaque snapshot pixels use the same path.
func blendCapturePixel(pixels []byte, index int, blue, green, red, alpha byte) {
	remaining := uint32(255 - alpha)
	for channel, value := range [3]byte{blue, green, red} {
		pixels[index+channel] = byte((uint32(value)*uint32(alpha) + uint32(pixels[index+channel])*remaining + 127) / 255)
	}
	pixels[index+3] = byte(uint32(alpha) + (uint32(pixels[index+3])*remaining+127)/255)
}

func capturePixel(pixels []byte, width, height, x, y int, blue, green, red, alpha byte) {
	if x < 0 || y < 0 || x >= width || y >= height {
		return
	}
	blendCapturePixel(pixels, (y*width+x)*4, blue, green, red, alpha)
}

func drawCaptureBorder(pixels []byte, width int, selection image.Rectangle, dpi int, outside bool) {
	if width <= 0 || selection.Empty() {
		return
	}
	height := len(pixels) / (width * 4)
	bounds := image.Rect(0, 0, width, height)
	stroke := captureStroke(dpi)
	// Each ring owns its corners once, avoiding doubled alpha.
	darkInner := selection
	if outside {
		darkInner = selection.Inset(-stroke)
	}
	drawCaptureRing(pixels, width, bounds, darkInner.Inset(-1), darkInner, 0, 0, 0, 46)
	line := selection
	if outside {
		line = selection.Inset(-stroke)
	}
	inner := line.Inset(stroke)
	if outside {
		inner = selection
	} else if line.Dx() <= 2*stroke || line.Dy() <= 2*stroke {
		center := image.Pt((line.Min.X+line.Max.X)/2, (line.Min.Y+line.Max.Y)/2)
		inner = image.Rectangle{Min: center, Max: center}
	}
	drawCaptureRing(pixels, width, bounds, line, inner, 255, 255, 255, 235)
}

func drawCaptureRing(pixels []byte, width int, bounds, outer, inner image.Rectangle, blue, green, red, alpha byte) {
	strips := captureRingStrips(outer, inner)
	height := bounds.Dy()
	for _, strip := range strips {
		strip = strip.Intersect(bounds)
		for y := strip.Min.Y; y < strip.Max.Y; y++ {
			for x := strip.Min.X; x < strip.Max.X; x++ {
				capturePixel(pixels, width, height, x, y, blue, green, red, alpha)
			}
		}
	}
}

func captureRingStrips(outer, inner image.Rectangle) [4]image.Rectangle {
	return [4]image.Rectangle{
		image.Rect(outer.Min.X, outer.Min.Y, outer.Max.X, min(inner.Min.Y, outer.Max.Y)),
		image.Rect(outer.Min.X, max(inner.Max.Y, outer.Min.Y), outer.Max.X, outer.Max.Y),
		image.Rect(outer.Min.X, inner.Min.Y, min(inner.Min.X, outer.Max.X), inner.Max.Y),
		image.Rect(max(inner.Max.X, outer.Min.X), inner.Min.Y, outer.Max.X, inner.Max.Y),
	}
}

func captureLabelBounds(cursor image.Point, width, height int, bounds image.Rectangle, dpi int) image.Rectangle {
	gapX, gapY := max(1, (8*max(96, dpi)+48)/96), max(1, (4*max(96, dpi)+48)/96)
	x, y := cursor.X+gapX, cursor.Y+gapY
	if x+width > bounds.Max.X {
		x = cursor.X - gapX - width
	}
	if y+height > bounds.Max.Y {
		y = cursor.Y - gapY - height
	}
	return image.Rect(max(bounds.Min.X, min(x, bounds.Max.X-width)), max(bounds.Min.Y, min(y, bounds.Max.Y-height)), max(bounds.Min.X, min(x, bounds.Max.X-width))+width, max(bounds.Min.Y, min(y, bounds.Max.Y-height))+height)
}

func drawCaptureCrosshair(pixels []byte, width int, cursor image.Point, dpi int) {
	if width <= 0 {
		return
	}
	height := len(pixels) / (width * 4)
	half := max(1, (13*max(96, dpi)+48)/96)
	stroke := captureStroke(dpi)
	for y := cursor.Y - half; y <= cursor.Y+half; y++ {
		for dx := -stroke; dx <= stroke; dx++ {
			capturePixel(pixels, width, height, cursor.X+dx, y, 255, 255, 255, 115)
		}
	}
	for x := cursor.X - half; x <= cursor.X+half; x++ {
		for dy := -stroke; dy <= stroke; dy++ {
			capturePixel(pixels, width, height, x, cursor.Y+dy, 255, 255, 255, 115)
		}
	}
	for y := cursor.Y - half; y <= cursor.Y+half; y++ {
		for dx := 0; dx < stroke; dx++ {
			capturePixel(pixels, width, height, cursor.X+dx, y, 0, 0, 0, 209)
		}
	}
	for x := cursor.X - half; x <= cursor.X+half; x++ {
		for dy := 0; dy < stroke; dy++ {
			capturePixel(pixels, width, height, x, cursor.Y+dy, 0, 0, 0, 209)
		}
	}
}
