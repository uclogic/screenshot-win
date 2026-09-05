//go:build windows

package selector

import (
	"image"
	"unsafe"
)

func (state *selectionState) refreshCandidate() bool {
	var cursor point
	if ok, _, _ := procPinGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor))); ok == 0 {
		return false
	}
	p := image.Pt(int(cursor.X), int(cursor.Y))
	r, ok := SmallestRectangleAt(state.candidates, p.Sub(state.desktop.Min))
	if !ok {
		area := rect{cursor.X, cursor.Y, cursor.X + 1, cursor.Y + 1}
		monitor, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&area)), monitorDefaultToNearest)
		info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
		if monitor != 0 {
			if success, _, _ := procGetMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info))); success != 0 {
				r = image.Rect(int(info.Monitor.Left), int(info.Monitor.Top), int(info.Monitor.Right), int(info.Monitor.Bottom)).Sub(state.desktop.Min).Intersect(state.client)
			}
		}
	}
	changed := r != state.candidate
	state.candidate = r
	return changed
}
