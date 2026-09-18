//go:build windows

package selector

import (
	"image"
	"testing"
	"time"

	"screenshot-win/editor"
	"screenshot-win/internal/ui/glass"
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
	// Moving while background acquisition is pending must preserve the material
	// and native surface, including when the old request eventually completes.
	memoryDC, bitmap := w.surface.memoryDC, w.surface.bitmap
	oldBounds := w.bounds
	pending := make(chan image.Image, 1)
	w.fetch = backdropFetch{result: pending, bounds: oldBounds}
	state.windowBounds = state.windowBounds.Add(image.Pt(20, 30))
	state.paintGlass(0, false)
	if w.base != base || w.surface.memoryDC != memoryDC || w.surface.bitmap != bitmap || w.fetch.result != pending {
		t.Fatal("moving discarded the glass material, surface, or pending request")
	}
	pending <- image.NewRGBA(oldBounds)
	state.paintGlass(0, false)
	if w.base != base {
		t.Fatal("wrong-sized background replaced the glass material")
	}
	completed := make(chan image.Image, 1)
	sampledBounds := oldBounds.Inset(-glass.Support(state.dpi))
	completed <- image.NewRGBA(sampledBounds)
	w.fetch = backdropFetch{result: completed, bounds: sampledBounds}
	state.paintGlass(0, false)
	if w.base == base || w.backdrop.Bounds() != sampledBounds {
		t.Fatal("completed sample was discarded during movement")
	}
	base = w.base
	// Repaints inside the sampling interval must keep the material without
	// starting another background request.
	state.background = func(bounds image.Rectangle) image.Image { return image.NewRGBA(bounds) }
	w.nextSample = time.Now().Add(time.Hour)
	w.dirty = true
	state.paintGlass(0, false)
	if w.fetch.result != nil || !w.dirty || w.base != base {
		t.Fatal("background sampling ignored the refresh interval")
	}
	state.background = nil
	failed := make(chan image.Image, 1)
	failed <- nil
	w.fetch = backdropFetch{result: failed}
	state.paintGlass(0, false)
	if w.base != base {
		t.Fatal("failed background request replaced the glass material")
	}
	w.close()
	if w.surface.memoryDC != 0 || w.surface.bitmap != 0 || w.surface.pixels != nil {
		t.Fatal("surface resources retained")
	}
}
