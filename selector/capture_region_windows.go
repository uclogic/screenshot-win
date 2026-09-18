//go:build windows

package selector

import (
	"image"
	"math"

	"screenshot-win/editor"
)

func (state *frozenState) captureRegionLayout(region image.Rectangle) editor.RectangleLayout {
	return editor.LayoutRectangleAllHandles(region.Min, region.Max, state.dpi, 3)
}

func (state *frozenState) beginCaptureRegionResize(point image.Point) bool {
	if !state.editableRegion || state.regionTransform != nil {
		return false
	}
	handle, hit := state.captureRegionLayout(state.region).Hit(point)
	if !hit {
		return false
	}
	state.cancelAnimationFrame()
	state.regionTransform = &frozenRegionTransform{handle: handle, original: state.region, draft: state.region, anchor: point}
	procSetCapture.Call(state.hwnd)
	state.setCursor(rectangleCursor(handle))
	return true
}

func (state *frozenState) updateCaptureRegionResize(point image.Point) {
	transform := state.regionTransform
	if transform == nil {
		return
	}
	next := resizeCaptureRegion(transform.original, transform.handle, point.Sub(transform.anchor), state.client, scaleForDPI(16, state.dpi))
	if next == transform.draft {
		return
	}
	previous := transform.draft
	transform.draft = next
	state.setRegion(next, true)
	state.repaintCaptureRegion(previous, next)
}

func (state *frozenState) commitCaptureRegionResize() {
	if state.regionTransform == nil {
		return
	}
	state.regionTransform = nil
	procReleaseCapture.Call()
	state.publishRegion(state.region.Add(state.desktop.Min))
}

func (state *frozenState) cancelCaptureRegionResize() bool {
	transform := state.regionTransform
	if transform == nil {
		return false
	}
	previous := transform.draft
	state.regionTransform = nil
	state.setRegion(transform.original, true)
	state.repaintCaptureRegion(previous, transform.original)
	return true
}

func resizeCaptureRegion(original image.Rectangle, handle editor.TransformHandle, delta image.Point, bounds image.Rectangle, minimum int) image.Rectangle {
	minimum = max(1, minimum)
	next := original
	switch handle {
	case editor.HandleRectangleNorthWest, editor.HandleRectangleWest, editor.HandleRectangleSouthWest:
		next.Min.X = max(bounds.Min.X, min(original.Min.X+delta.X, original.Max.X-minimum))
	case editor.HandleRectangleNorthEast, editor.HandleRectangleEast, editor.HandleRectangleSouthEast:
		next.Max.X = min(bounds.Max.X, max(original.Max.X+delta.X, original.Min.X+minimum))
	}
	switch handle {
	case editor.HandleRectangleNorthWest, editor.HandleRectangleNorth, editor.HandleRectangleNorthEast:
		next.Min.Y = max(bounds.Min.Y, min(original.Min.Y+delta.Y, original.Max.Y-minimum))
	case editor.HandleRectangleSouthWest, editor.HandleRectangleSouth, editor.HandleRectangleSouthEast:
		next.Max.Y = min(bounds.Max.Y, max(original.Max.Y+delta.Y, original.Min.Y+minimum))
	}
	return next
}

func (state *frozenState) repaintCaptureRegion(regions ...image.Rectangle) {
	rendered := state.document.Rendered()
	for _, region := range regions {
		for _, strip := range state.captureRegionFootprint(region) {
			strip = strip.Intersect(state.client)
			if strip.Empty() {
				continue
			}
			state.copyViewport(editor.RenderViewport(rendered, state.viewport, strip), strip)
		}
	}
	if state.selected != 0 && state.textEdit == nil {
		if annotation, ok := state.document.Get(state.selected); ok {
			state.drawSelectionOverlay(annotation)
		}
	}
	state.drawCaptureChrome()
	state.presentOrCloseFrozen()
}

func (state *frozenState) captureRegionFootprint(region image.Rectangle) [4]image.Rectangle {
	layout := state.captureRegionLayout(region)
	margin := int(math.Ceil(layout.Radius+layout.Stroke/2)) + 4
	return [4]image.Rectangle{
		image.Rect(region.Min.X-margin, region.Min.Y-margin, region.Max.X+margin+1, region.Min.Y+margin+1),
		image.Rect(region.Min.X-margin, region.Max.Y-margin, region.Max.X+margin+1, region.Max.Y+margin+1),
		image.Rect(region.Min.X-margin, region.Min.Y-margin, region.Min.X+margin+1, region.Max.Y+margin+1),
		image.Rect(region.Max.X-margin, region.Min.Y-margin, region.Max.X+margin+1, region.Max.Y+margin+1),
	}
}

func (state *frozenState) drawCaptureChrome() {
	drawOuterPixelBorder(state.pixels, state.client.Dx(), state.client, state.region)
	if !state.editableRegion {
		return
	}
	layout := state.captureRegionLayout(state.region)
	const blueR, blueG, blueB = 22, 140, 255
	margin := int(math.Ceil(layout.Radius + layout.Stroke/2 + .5))
	for n := 0; n < layout.Count; n++ {
		handle := layout.Handles[n].Point
		bounds := image.Rect(int(math.Floor(handle.X))-margin, int(math.Floor(handle.Y))-margin, int(math.Ceil(handle.X))+margin+1, int(math.Ceil(handle.Y))+margin+1).Intersect(state.client)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				coverage := layout.HandleCoverage(editor.ScreenPoint{X: float64(x), Y: float64(y)})
				if coverage <= 0 {
					continue
				}
				index := (y*state.client.Dx() + x) * 4
				channels := [3]byte{blueB, blueG, blueR}
				for channel := range 3 {
					value := float64(state.pixels[index+channel])
					state.pixels[index+channel] = byte(math.Round(value + (float64(channels[channel])-value)*coverage))
				}
				state.pixels[index+3] = 255
			}
		}
	}
}

func (state *frozenState) setCursor(cursorID int) {
	if cursor, _, _ := procLoadCursor.Call(0, uintptr(cursorID)); cursor != 0 {
		procFrozenSetCursor.Call(cursor)
	}
}
