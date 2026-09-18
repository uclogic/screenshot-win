//go:build windows

package selector

import (
	"image"
	"image/color"
	"math"

	"screenshot-win/editor"
)

func (state *frozenState) beginSelectedHandle(point image.Point) bool {
	if state.selected == 0 {
		return false
	}
	a, ok := state.document.Get(state.selected)
	if !ok {
		return false
	}
	handle, hit := state.handleAt(point, a)
	if !hit {
		return false
	}
	state.beginTransform(a, handle, point)
	return true
}

func (state *frozenState) selectForMove(a editor.Annotation, point image.Point) {
	if state.selected != a.ID {
		if previous, ok := state.clearSelectionForDrawing(); ok {
			if err := state.clearSelectionOverlay(previous); err != nil {
				state.renderErr = err
			}
		}
		state.selected = a.ID
		state.notifySelectedStyle(a.Style)
		if request := state.activeRequest(); request != nil {
			request.style = a.Style
		}
		state.drawSelectionOverlay(a)
		state.presentOrCloseFrozen()
	}
	state.beginTransform(a, editor.HandleMove, point)
}

func (state *frozenState) rectangleLayout(a editor.Annotation) editor.RectangleLayout {
	return editor.LayoutRectangle(state.viewport.ImageToScreen(a.Start), state.viewport.ImageToScreen(a.End), state.dpi, math.Max(1, a.Style.Width*state.viewport.Scale))
}

func rectangleCursor(handle editor.TransformHandle) int {
	switch handle {
	case editor.HandleRectangleNorthWest, editor.HandleRectangleSouthEast:
		return idcSizeNWSE
	case editor.HandleRectangleNorthEast, editor.HandleRectangleSouthWest:
		return idcSizeNESW
	case editor.HandleRectangleNorth, editor.HandleRectangleSouth:
		return idcSizeNS
	case editor.HandleRectangleWest, editor.HandleRectangleEast:
		return idcSizeWE
	}
	return idcSizeAll
}

func (state *frozenState) rectangleMinimum() image.Point {
	scale := state.viewport.Scale
	if scale <= 0 {
		scale = 1
	}
	n := max(1, int(math.Ceil(16*float64(max(96, state.dpi))/96/scale)))
	b := state.document.Bounds()
	return image.Pt(min(n, max(1, b.Dx()-1)), min(n, max(1, b.Dy()-1)))
}

// These strips include both the full (unselected) border and the editing
// chrome. Visiting strips instead of their union avoids scanning the interior.
func (state *frozenState) rectangleFootprint(a editor.Annotation) [4]image.Rectangle {
	s, e := state.viewport.ImageToScreen(a.Start), state.viewport.ImageToScreen(a.End)
	x0, x1 := min(s.X, e.X), max(s.X, e.X)
	y0, y1 := min(s.Y, e.Y), max(s.Y, e.Y)
	l := state.rectangleLayout(a)
	margin := int(math.Ceil(math.Max(l.Radius+l.Stroke/2, math.Max(1, a.Style.Width*state.viewport.Scale)/2))) + 2
	return [4]image.Rectangle{
		image.Rect(x0-margin, y0-margin, x1+margin+1, y0+margin+1),
		image.Rect(x0-margin, y1-margin, x1+margin+1, y1+margin+1),
		image.Rect(x0-margin, y0-margin, x0+margin+1, y1+margin+1),
		image.Rect(x1-margin, y0-margin, x1+margin+1, y1+margin+1),
	}
}

func (state *frozenState) paintRectangleFootprint(a editor.Annotation, preview *editor.RectanglePreview) {
	for _, strip := range state.rectangleFootprint(a) {
		strip = strip.Intersect(state.client)
		for y := strip.Min.Y; y < strip.Max.Y; y++ {
			for x := strip.Min.X; x < strip.Max.X; x++ {
				p := image.Pt(x, y)
				var c color.NRGBA
				if p.In(state.annotationCanvas()) {
					c = preview.ScreenColor(p)
				} else {
					c = state.desktopColor(p)
				}
				i := (y*state.client.Dx() + x) * 4
				state.pixels[i], state.pixels[i+1], state.pixels[i+2], state.pixels[i+3] = c.B, c.G, c.R, 255
			}
		}
	}
}

func (state *frozenState) desktopColor(p image.Point) color.NRGBA {
	if state.source == nil {
		return color.NRGBA{A: 255}
	}
	b := state.source.Bounds()
	x, y := p.X+b.Min.X, p.Y+b.Min.Y
	if source, ok := state.source.(*image.RGBA); ok {
		c := source.RGBAAt(x, y)
		return color.NRGBA{R: c.R, G: c.G, B: c.B, A: 255}
	}
	return color.NRGBAModel.Convert(state.source.At(x, y)).(color.NRGBA)
}

func (state *frozenState) drawRectangleHandles(a editor.Annotation) {
	l := state.rectangleLayout(a)
	outline := editor.DefaultStyle().Color
	channels := [3]byte{outline.B, outline.G, outline.R}
	// Paint each affected pixel once, including overlapping small-box handles.
	margin := int(math.Ceil(l.Radius + l.Stroke/2 + .5))
	for n := 0; n < l.Count; n++ {
		h := l.Handles[n].Point
		b := image.Rect(int(math.Floor(h.X))-margin, int(math.Floor(h.Y))-margin, int(math.Ceil(h.X))+margin+1, int(math.Ceil(h.Y))+margin+1).Intersect(state.client)
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				// Earlier handle boxes own overlap pixels, so alpha never doubles.
				owned := false
				for j := 0; j < n; j++ {
					q := l.Handles[j].Point
					if x >= int(math.Floor(q.X))-margin && x <= int(math.Ceil(q.X))+margin && y >= int(math.Floor(q.Y))-margin && y <= int(math.Ceil(q.Y))+margin {
						owned = true
						break
					}
				}
				if owned {
					continue
				}
				coverage := l.HandleCoverage(editor.ScreenPoint{X: float64(x), Y: float64(y)})
				if coverage <= 0 {
					continue
				}
				i := (y*state.client.Dx() + x) * 4
				for channel := 0; channel < 3; channel++ {
					v := float64(state.pixels[i+channel])
					state.pixels[i+channel] = byte(math.Round(v + (float64(channels[channel])-v)*coverage))
				}
				state.pixels[i+3] = 255
			}
		}
	}
}

func (state *frozenState) renderRectangleTransform(t *frozenTransform) error {
	state.paintRectangleTransform(t)
	return state.present()
}

func (state *frozenState) paintRectangleTransform(t *frozenTransform) {
	if t.rectanglePreview == nil {
		t.rectanglePreview = state.document.NewRectanglePreview(t.id)
	}
	t.rectanglePreview.Update(t.draft, state.viewport, state.dpi)
	state.paintRectangleFootprint(t.previous, t.rectanglePreview)
	state.paintRectangleFootprint(t.draft, t.rectanglePreview)
	state.drawRectangleHandles(t.draft)
	state.drawCaptureChrome()
	t.fastPrepared = true
}

func (state *frozenState) drawSelectedRectangle(a editor.Annotation) {
	if state.document != nil {
		preview := state.document.NewRectanglePreview(a.ID)
		preview.Update(a, state.viewport, state.dpi)
		state.paintRectangleFootprint(a, preview)
	}
	state.drawRectangleHandles(a)
	state.drawCaptureChrome()
}

// vectorPixelIndex reuses the high-water capacity across drawing frames.
func (state *frozenState) vectorPixelIndex() map[int]int {
	if state.draftSeen == nil {
		state.draftSeen = make(map[int]int)
	} else {
		clear(state.draftSeen)
	}
	return state.draftSeen
}

func (state *frozenState) presentOrCloseFrozen() {
	if err := state.present(); err != nil {
		state.renderErr = err
		procDestroyWindow.Call(state.hwnd)
	}
}
