//go:build windows

package selector

import (
	"bytes"
	"image"
	"image/color"
	"runtime"
	"time"
	"unsafe"

	"screenshot-win/editor"
	"screenshot-win/internal/ui/glass"
)

const (
	wmFrozenBackdrop         = wmUser + 103
	wmToolbarBackdrop        = wmUser + 104
	wmToolbarBackgroundReady = wmUser + 106
	glassFrameTimer          = 903
	glassBackdropTimer       = 904
)

type frozenBackdropRequest struct {
	bounds image.Rectangle
	result chan image.Image
}

func copyFrozenBackdrop(surface *selectionState, bounds image.Rectangle) image.Image {
	if len(surface.pixels) == 0 || bounds.Empty() {
		return nil
	}
	dst := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			sx := max(0, min(x-surface.desktop.Min.X, surface.client.Dx()-1))
			sy := max(0, min(y-surface.desktop.Min.Y, surface.client.Dy()-1))
			i := (sy*surface.client.Dx() + sx) * 4
			dst.SetRGBA(x, y, color.RGBA{surface.pixels[i+2], surface.pixels[i+1], surface.pixels[i], 255})
		}
	}
	return dst
}

type glassWindow struct {
	surface               selectionState
	base, frame, backdrop *image.RGBA
	bounds                image.Rectangle
	dirty                 bool
	fetch                 backdropFetch
	nextSample            time.Time
	levels                []glass.Transition
}

func (window *glassWindow) close() {
	window.surface.closeSurface()
	*window = glassWindow{}
}

func (state *toolbarState) buttonRect(index int) image.Rectangle {
	if state.persistent {
		return glassToolbarButton(index, state.dpi)
	}
	w := (state.clientSize.X - 8) / len(state.actions)
	return image.Rect(4+index*w, 4, 4+(index+1)*w, state.clientSize.Y-4)
}

func (state *toolbarState) actionAt(point image.Point) (Action, bool) {
	if state.persistent {
		i, ok := glassToolbarActionAt(point, len(state.actions), state.dpi)
		if !ok {
			return ActionCancel, false
		}
		return state.actions[i], true
	}
	return toolbarActionAtActions(point, state.clientSize, state.actions)
}

func (state *toolbarState) panelOptionAt(point image.Point) (int, bool) {
	m := glass.Margin(state.dpi)
	return stylePanelOptionAt(point.Sub(image.Pt(m, m)), state.panel.field, state.dpi)
}

func (state *toolbarState) inkColor() color.NRGBA {
	if state.persistent {
		return glass.Light.Ink
	}
	return toolbarIconBaseColor(true)
}

func (state *toolbarState) glassContains(point image.Point, panel bool) bool {
	size, radius := state.clientSize, float64(glass.Scale(12, state.dpi))
	if panel {
		size = state.panel.bounds.Size()
		radius = float64(glass.Scale(12, state.dpi))
	}
	return glass.Contains(point, (image.Rectangle{Max: size}).Inset(glass.Margin(state.dpi)), radius)
}

func (state *toolbarState) updateGlassDPI(dpi int) {
	if dpi <= 0 || dpi == state.dpi {
		return
	}
	state.closeStylePanel()
	procFrozenKillTimer.Call(state.hwnd, glassFrameTimer)
	procFrozenKillTimer.Call(state.hwnd, glassBackdropTimer)
	state.motions = nil
	state.hovering, state.pressed = false, false
	state.glassWindow.close()
	state.dpi = dpi
	state.clientSize = glassToolbarSize(len(state.actions), dpi)
	state.windowBounds = toolbarBounds(state.region, state.workArea, state.clientSize)
	b := state.windowBounds
	procSetWindowPos.Call(state.hwnd, 0, uintptr(b.Min.X), uintptr(b.Min.Y), uintptr(b.Dx()), uintptr(b.Dy()), swpNoActivate)
	if state.tooltip != 0 {
		procDestroyWindow.Call(state.tooltip)
		state.tooltip = 0
	}
	if err := state.createTooltips(state.instance); err != nil {
		state.renderErr = err
	}
	procInvalidateRect.Call(state.hwnd, 0, 0)
}

func (state *toolbarState) paintGlassOrFallback(hwnd uintptr, panel bool) error {
	// Validate WM_PAINT even though presentation uses the layered-window API.
	var paint paintStruct
	procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&paint)))
	procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&paint)))
	if err := state.paintGlass(hwnd, panel); err == nil {
		return nil
	}
	// Preserve the action session if native alpha presentation is unavailable.
	state.glassEnabled = false
	state.glassWindow.close()
	state.panel.glassWindow.close()
	procFrozenSetWindowLongPtr.Call(state.hwnd, ^uintptr(19), wsExTopmost|wsExToolWindow)
	if state.panel.hwnd != 0 {
		procFrozenSetWindowLongPtr.Call(state.panel.hwnd, ^uintptr(19), wsExTopmost|wsExToolWindow)
		procInvalidateRect.Call(state.panel.hwnd, 0, 0)
	}
	procInvalidateRect.Call(state.hwnd, 0, 0)
	return nil
}

func (state *toolbarState) paintGlass(hwnd uintptr, panel bool) error {
	window, bounds, radius := &state.glassWindow, state.windowBounds, float64(glass.Scale(12, state.dpi))
	count := len(state.actions)
	if panel {
		window, bounds, radius = &state.panel.glassWindow, state.panel.bounds, float64(glass.Scale(12, state.dpi))
		count = len(editor.PresetColors())
		if state.panel.field == editor.StyleFieldWidth {
			count = len(editor.PresetWidths())
		}
	}
	if window.bounds.Size() != bounds.Size() || window.frame == nil {
		window.close()
		window.bounds = bounds
		window.dirty = true
		window.surface.client = image.Rectangle{Max: bounds.Size()}
		if err := window.surface.initializeSurface(); err != nil {
			return err
		}
		window.frame = image.NewRGBA(window.surface.client)
		window.levels = make([]glass.Transition, count*3)
	}
	// Moving keeps the existing material and native surface visible while the
	// new screen location is sampled asynchronously.
	if window.bounds != bounds {
		window.bounds = bounds
		window.dirty = true
	}
	sampleBounds := bounds.Inset(-glass.Support(state.dpi))
	requestedBounds := window.fetch.bounds
	source, received := window.fetch.poll()
	if received && (source == nil || requestedBounds.Size() != sampleBounds.Size()) {
		// Failed or differently sized samples cannot replace the material.
		source, received = nil, false
		window.dirty = true
	}
	if window.dirty {
		if state.background == nil {
			window.dirty = false
		} else if !time.Now().Before(window.nextSample) && window.fetch.start(state.background, sampleBounds, func() {
			procPostMessage.Call(hwnd, wmToolbarBackgroundReady, 0, 0)
		}) {
			window.dirty = false
			window.nextSample = time.Now().Add(time.Second / 30)
		}
	}
	if window.dirty && window.fetch.result == nil && state.background != nil {
		delay := max(int64(1), time.Until(window.nextSample).Milliseconds()+1)
		procFrozenSetTimer.Call(state.hwnd, glassBackdropTimer, uintptr(delay), 0)
	}
	if received || window.base == nil {
		// Present the completed sample even if the toolbar moved meanwhile.
		// Rendering in its original coordinates avoids stretching/clamping it;
		// the next scheduled sample catches up to the latest position.
		renderBounds := bounds
		if received {
			sampleBounds = requestedBounds
			renderBounds = requestedBounds.Inset(glass.Support(state.dpi))
		}
		back := glass.Crop(source, sampleBounds)
		if window.base == nil || window.backdrop == nil || !bytes.Equal(back.Pix, window.backdrop.Pix) {
			body := (image.Rectangle{Max: bounds.Size()}).Inset(glass.Margin(state.dpi))
			theme := glass.Light
			theme.TintAmount = .784
			window.base = glass.RenderFrosted(back, renderBounds, body, radius, state.dpi, theme)
			window.backdrop = back
		}
	}
	copy(window.frame.Pix, window.base.Pix)
	now := time.Now()
	animating := false
	var frames []toolbarMotionFrame
	if !panel {
		frames, animating = state.motionFrames(now)
	}
	for i := 0; i < count; i++ {
		button := state.buttonRect(i)
		if !panel {
			f := frames[i]
			colors := toolbarMotionColors()
			for j, amount := range []float64{f.hover, f.selected, f.press} {
				glass.Overlay(window.frame, button.Inset(glass.Scale(2, state.dpi)), float64(glass.Scale(10, state.dpi)), colors[j], amount)
			}
			continue
		}
		button = state.styleOptionRect(i)
		hover := i == state.panel.hoverIndex || i == state.panel.keyIndex
		selected := i == state.currentPanelIndex()
		pressed := i == state.panel.pressedIndex && i == state.panel.hoverIndex
		colors := []color.NRGBA{glass.Light.Hover, glass.Light.Selected, {35, 75, 130, 55}}
		for j, on := range []bool{hover, selected, pressed} {
			amount, active := window.levels[i*3+j].Update(on, now)
			animating = animating || active
			glass.Overlay(window.frame, button.Inset(glass.Scale(2, state.dpi)), float64(glass.Scale(10, state.dpi)), colors[j], amount)
		}
	}
	// GDI draws RGB into the opaque interior. Restore the material alpha after
	// drawing so GDI cannot punch transparent holes in text or vector strokes.
	for i := 0; i < len(window.frame.Pix); i += 4 {
		p, q := window.frame.Pix[i:i+4], window.surface.pixels[i:i+4]
		q[0], q[1], q[2], q[3] = p[2], p[1], p[0], p[3]
	}
	if panel {
		for i := 0; i < count; i++ {
			state.drawStyleOption(window.surface.memoryDC, i)
		}
	} else {
		renderer := newToolbarIconRenderer(window.surface.memoryDC)
		for i, action := range state.actions {
			ink := frames[i].ink()
			renderer.ink = &ink
			renderer.motion = &frames[i]
			renderer.draw(action, state.buttonRect(i), toolbarActionEnabled(action), state.style, state.dpi)
		}
		renderer.close()
	}
	for i := 3; i < len(window.frame.Pix); i += 4 {
		window.surface.pixels[i] = window.frame.Pix[i]
	}
	destination := point{X: int32(bounds.Min.X), Y: int32(bounds.Min.Y)}
	dimensions := size{Width: int32(bounds.Dx()), Height: int32(bounds.Dy())}
	origin := point{}
	blend := blendFunction{Operation: acSrcOver, SourceConstantAlpha: 255, AlphaFormat: acSrcAlpha}
	ok, _, err := procUpdateLayeredWindow.Call(hwnd, 0, uintptr(unsafe.Pointer(&destination)), uintptr(unsafe.Pointer(&dimensions)), window.surface.memoryDC, uintptr(unsafe.Pointer(&origin)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)
	runtime.KeepAlive(window)
	if ok == 0 {
		return win32Error("UpdateLayeredWindow glass toolbar", err)
	}
	if animating {
		procFrozenSetTimer.Call(state.hwnd, glassFrameTimer, 16, 0)
	}
	return nil
}
