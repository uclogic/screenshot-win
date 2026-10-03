//go:build windows

package selector

import (
	"image"
	"image/color"
	"time"

	"screenshot-win/editor"
)

var toolbarRoundRect = gdi32.NewProc("RoundRect")

func (state *toolbarState) motionInput(action Action) toolbarMotionInput {
	hover := state.hovering && state.hover == action && toolbarActionEnabled(action)
	selected := state.active && (state.activeAction == action || action == ActionArrow && arrowFormatAction(state.activeAction))
	if state.panel.hwnd != 0 {
		selected = selected || action == ActionArrow && state.panel.format || action == ActionColor && state.panel.field == editor.StyleFieldColor || action == ActionWidth && state.panel.field == editor.StyleFieldWidth
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

// Keep popup animation state independent of the rendering surface so a glass
// fallback preserves hover and press transitions.
func (state *toolbarState) panelMotionFrames(now time.Time) ([]toolbarMotionFrame, bool) {
	count := state.panelOptionCount()
	if len(state.panel.motions) != count {
		state.panel.motions = make([]toolbarMotion, count)
	}
	frames := make([]toolbarMotionFrame, count)
	active := false
	for i := range frames {
		hover := i == state.panel.hoverIndex || state.panel.keyFocused && i == state.panel.keyIndex
		input := toolbarMotionInput{
			hover: hover, selected: i == state.currentPanelIndex(),
			pressed: i == state.panel.pressedIndex && i == state.panel.hoverIndex,
		}
		motion := &state.panel.motions[i]
		if !motion.initialized {
			motion.selected = toolbarTween{from: boolLevel(input.selected), to: boolLevel(input.selected)}
		}
		var moving bool
		frames[i], moving = motion.update(ActionColor, input, now)
		active = active || moving
	}
	return frames, active
}

// Retarget on input messages as well as paints so rapid press/release events
// are captured even when Windows coalesces WM_PAINT messages.
func (state *toolbarState) refreshMotion() {
	if !(state.persistent || state.capture) || state.motionClosed {
		return
	}
	now := time.Now()
	_, active := state.motionFrames(now)
	if state.panel.hwnd != 0 {
		_, panelActive := state.panelMotionFrames(now)
		active = active || panelActive
		if panelActive {
			procInvalidateRect.Call(state.panel.hwnd, 0, 0)
		}
	}
	if active {
		procInvalidateRect.Call(state.hwnd, 0, 0)
		procFrozenSetTimer.Call(state.hwnd, glassFrameTimer, 16, 0)
	}
}

func toolbarMotionColors() [3]color.NRGBA {
	return [3]color.NRGBA{{0, 0, 0, 13}, {90, 160, 245, 90}, {35, 75, 130, 55}}
}

func (state *toolbarState) drawMotionBackground(dc uintptr, index int, frame toolbarMotionFrame) {
	state.drawMotionBackgroundRect(dc, state.buttonRect(index), frame)
}

func (state *toolbarState) drawMotionBackgroundRect(dc uintptr, bounds image.Rectangle, frame toolbarMotionFrame) {
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
	b := bounds.Inset(scaleForDPI(2, state.dpi))
	diameter := uintptr(scaleForDPI(20, state.dpi))
	toolbarRoundRect.Call(dc, uintptr(b.Min.X), uintptr(b.Min.Y), uintptr(b.Max.X), uintptr(b.Max.Y), diameter, diameter)
}
