package selector

import (
	"image"
	"testing"

	"screenshot-win/editor"
)

func TestToolbarGDIPlusGlyphsRenderToMemory(t *testing.T) {
	const buttonSize = 40
	actions := []Action{
		ActionCancel, ActionSave, ActionCopy, ActionScroll, ActionSaveAs, ActionPin,
		ActionEdit, ActionRectangle, ActionArrow, ActionText, ActionColor, ActionWidth,
	}
	surface := selectionState{client: image.Rect(0, 0, len(actions)*buttonSize, buttonSize)}
	if err := surface.initializeSurface(); err != nil {
		t.Fatalf("initialize in-memory toolbar surface: %v", err)
	}
	defer surface.closeSurface()

	renderer := newToolbarIconRenderer(surface.memoryDC)
	if renderer.graphics == 0 {
		t.Fatal("GDI+ toolbar renderer is unavailable")
	}
	style := editor.DefaultStyle()
	for index, action := range actions {
		button := image.Rect(index*buttonSize+4, 4, (index+1)*buttonSize-4, buttonSize-4)
		renderer.draw(action, button, true, style, 96)
	}
	renderer.close()

	stride := surface.client.Dx() * 4
	partialPixels := 0
	for index, action := range actions {
		changed := 0
		left := index * buttonSize
		right := left + buttonSize
		for y := 0; y < buttonSize; y++ {
			for x := left; x < right; x++ {
				offset := y*stride + x*4
				blue, green, red := surface.pixels[offset], surface.pixels[offset+1], surface.pixels[offset+2]
				if blue != 0 || green != 0 || red != 0 {
					changed++
				}
				// The first ten glyphs use only the fixed 235/238/242 base
				// color, so an intermediate blue channel there must come from
				// GDI+ edge coverage rather than a selected color overlay.
				if index < len(actions)-2 && blue > 0 && blue < 242 && green > 0 && green < 238 && red > 0 && red < 235 {
					partialPixels++
				}
			}
		}
		if changed == 0 {
			t.Errorf("toolbar action %d did not render any pixels", action)
		}
	}
	if partialPixels == 0 {
		t.Fatal("toolbar glyphs rendered without any anti-aliased edge pixels")
	}
}
