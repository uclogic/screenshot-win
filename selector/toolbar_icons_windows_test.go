package selector

import (
	"image"
	"math"
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

func TestToolbarAnimatedGlyphsStayInsideFixedButtons(t *testing.T) {
	for _, dpi := range []int{96, 144, 192} {
		button := glassToolbarButton(0, dpi)
		surface := selectionState{client: image.Rectangle{Max: glassToolbarSize(1, dpi)}}
		if err := surface.initializeSurface(); err != nil {
			t.Fatal(err)
		}
		renderer := newToolbarIconRenderer(surface.memoryDC)
		for _, fallback := range []bool{false, true} {
			graphics := renderer.graphics
			if fallback {
				renderer.graphics = 0
			}
			for _, frame := range []toolbarMotionFrame{{scale: 1}, {scale: 1.08, y: -1, semantic: 1}, {scale: .94}} {
				renderer.motion = &frame
				for _, action := range selectionToolbarActions {
					clear(surface.pixels)
					renderer.draw(action, button, true, editor.DefaultStyle(), dpi)
					count := 0
					for y := 0; y < surface.client.Dy(); y++ {
						for x := 0; x < surface.client.Dx(); x++ {
							i := (y*surface.client.Dx() + x) * 4
							if surface.pixels[i]|surface.pixels[i+1]|surface.pixels[i+2] == 0 {
								continue
							}
							count++
							if !image.Pt(x, y).In(button.Inset(scaleForDPI(2, dpi))) {
								t.Fatalf("glyph escaped button: dpi=%d action=%d fallback=%v", dpi, action, fallback)
							}
						}
					}
					if count == 0 {
						t.Fatal("empty animated glyph", dpi, action, fallback)
					}
				}
			}
			renderer.graphics = graphics
		}
		renderer.close()
		surface.closeSurface()
		if got, ok := glassToolbarActionAt(button.Min, 1, dpi); !ok || got != 0 {
			t.Fatal("fixed hit target changed")
		}
	}
}

func TestToolbarGlyphRotationKeepsCenter(t *testing.T) {
	transform := toolbarGlyphTransform{x: 10, y: 20, scale: 2, angle: -5 * math.Pi / 180}
	center := transform.point(12, 12)
	if center.X != 34 || center.Y != 44 {
		t.Fatal("rotation moved center", center)
	}
	p := transform.point(18, 12)
	if math.Abs(math.Hypot(float64(p.X-center.X), float64(p.Y-center.Y))-12) > 1e-5 {
		t.Fatal("rotation changed radius")
	}
}
