//go:build windows

package selector

import (
	"image/color"
	"time"

	"screenshot-win/editor"
)

var toolbarRoundRect = gdi32.NewProc("RoundRect")

func (state *toolbarState) motionInput(action Action) toolbarMotionInput {
	hover := state.hovering && state.hover == action && toolbarActionEnabled(action)
	selected := state.active && state.activeAction == action
	if state.panel.hwnd != 0 {
		selected = selected || action == ActionColor && state.panel.field == editor.StyleFieldColor || action == ActionWidth && state.panel.field == editor.StyleFieldWidth
	}
	return toolbarMotionInput{hover: hover, selected: selected, pressed: state.pressed && state.pressedAction == action && hover}
}

func (state *toolbarState) motionFrames(now time.Time) ([]toolbarMotionFrame, bool) {
	if len(state.motions) != len(state.actions) {
		state.motions = make([]toolbarMotion, len(state.actions))
	}
	frames := make([]toolbarMotionFrame, len(state.actions))
	active := false
	for i, action := range state.actions {
		var moving bool
		frames[i], moving = state.motions[i].update(action, state.motionInput(action), now)
		active = active || moving
	}
	return frames, active
}

// Retarget on input messages as well as paints so rapid press/release events
// are captured even when Windows coalesces WM_PAINT messages.
func (state *toolbarState) refreshMotion() {
	if !state.persistent || state.motionClosed {
		return
	}
	_, active := state.motionFrames(time.Now())
	if active {
		procInvalidateRect.Call(state.hwnd, 0, 0)
		procFrozenSetTimer.Call(state.hwnd, glassFrameTimer, 16, 0)
	}
}

func toolbarMotionColors() [3]color.NRGBA {
	return [3]color.NRGBA{{0, 0, 0, 13}, {90, 160, 245, 90}, {35, 75, 130, 55}}
}

func (state *toolbarState) drawMotionBackground(dc uintptr, index int, frame toolbarMotionFrame) {
	ink := color.NRGBA{245, 249, 255, 255}
	colors := toolbarMotionColors()
	for j, amount := range []float64{frame.hover, frame.selected, frame.press} {
		ink = toolbarMixColor(ink, colors[j], float64(colors[j].A)/255*amount)
	}
	brush, _, _ := procCreateSolidBrush.Call(rgb(ink.R, ink.G, ink.B))
	if brush == 0 {
		return
	}
	defer procDeleteObject.Call(brush)
	pen, _, _ := procCreatePen.Call(psSolid, 1, rgb(ink.R, ink.G, ink.B))
	if pen == 0 {
		return
	}
	defer procDeleteObject.Call(pen)
	oldBrush, _, _ := procSelectObject.Call(dc, brush)
	defer procSelectObject.Call(dc, oldBrush)
	oldPen, _, _ := procSelectObject.Call(dc, pen)
	defer procSelectObject.Call(dc, oldPen)
	b := state.buttonRect(index).Inset(scaleForDPI(2, state.dpi))
	diameter := uintptr(scaleForDPI(20, state.dpi))
	toolbarRoundRect.Call(dc, uintptr(b.Min.X), uintptr(b.Min.Y), uintptr(b.Max.X), uintptr(b.Max.Y), diameter, diameter)
}
