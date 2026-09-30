//go:build windows

package selector

const (
	wmWindowPosChanging = 0x0046
	swpNoZOrder         = 0x0004
)

// windowPosition matches Win32 WINDOWPOS, including its pointer-sized handles.
type windowPosition struct {
	hwnd, insertAfter uintptr
	x, y, cx, cy      int32
	flags             uint32
}

// Keep activation from raising the canvas over its independent toolbar. Change
// the pending order before Windows presents it, rather than relying only on the
// asynchronous wmToolbarRaise repair. Do not attach the windows' input queues.
func keepCanvasBelowToolbar(position *windowPosition, canvas uintptr) {
	if position == nil || position.flags&swpNoZOrder != 0 {
		return
	}
	// Only constrain raises; retain explicit lowering and geometry-only changes.
	if position.insertAfter != 0 && position.insertAfter != hwndTopmost {
		return
	}
	toolbar := activeToolbarWindow.Load()
	value, found := toolbarStates.Load(toolbar)
	if !found {
		return
	}
	state := value.(*toolbarState)
	if state.persistent && state.shortcutTarget == canvas && toolbar != canvas {
		position.insertAfter = toolbar
	}
}
