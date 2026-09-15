//go:build windows

package selector

import (
	"image"
	"testing"

	"screenshot-win/editor"
)

func TestGlassNativeSurfaceAndBackgroundOwnership(t *testing.T) {
	state := &toolbarState{persistent: true, glassEnabled: true, dpi: 96, actions: selectionToolbarActions, style: editor.DefaultStyle()}
	state.clientSize = glassToolbarSize(len(state.actions), 96)
	state.windowBounds = image.Rectangle{Max: state.clientSize}
	defer state.glassWindow.close()
	// A null HWND deliberately rejects presentation after rendering to memory.
	if err := state.paintGlass(0, false); err == nil {
		t.Fatal("presentation accepted a null window")
	}
	w := &state.glassWindow
	if len(w.surface.pixels) == 0 {
		t.Fatal("native surface was not rendered")
	}
	different := 0
	for i := 0; i < len(w.frame.Pix); i += 4 {
		if w.surface.pixels[i+3] != w.frame.Pix[i+3] {
			t.Fatal("GDI damaged material alpha")
		}
		if w.surface.pixels[i] != w.frame.Pix[i+2] || w.surface.pixels[i+2] != w.frame.Pix[i] {
			different++
		}
	}
	if different == 0 {
		t.Fatal("icons did not render")
	}
	w.surface.desktop = state.windowBounds
	copy := copyFrozenBackdrop(&w.surface, image.Rect(-2, -2, 5, 5))
	if copy == nil || copy.Bounds() != image.Rect(-2, -2, 5, 5) {
		t.Fatal("screen-coordinate crop failed")
	}
	before := copy.At(0, 0)
	w.surface.pixels[0] ^= 255
	if copy.At(0, 0) != before {
		t.Fatal("background copy aliases native pixels")
	}
	base := w.base
	w.dirty = true
	state.paintGlass(0, false)
	if w.base != base {
		t.Fatal("unchanged backdrop rebuilt material")
	}
	w.close()
	if w.surface.memoryDC != 0 || w.surface.bitmap != 0 || w.surface.pixels != nil {
		t.Fatal("surface resources retained")
	}
}
