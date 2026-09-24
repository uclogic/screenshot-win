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
	return state.refreshCandidateAt(image.Pt(int(cursor.X), int(cursor.Y)).Sub(state.desktop.Min))
}

func (state *selectionState) refreshCandidateAt(p image.Point) bool {
	cursor := p.Add(state.desktop.Min)
	r, ok := state.candidateExtent.at(state.candidates, p)
	if !ok {
		area := rect{int32(cursor.X), int32(cursor.Y), int32(cursor.X + 1), int32(cursor.Y + 1)}
		monitor, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&area)), monitorDefaultToNearest)
		info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
		if monitor != 0 {
			if success, _, _ := procGetMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info))); success != 0 {
				r = image.Rect(int(info.Monitor.Left), int(info.Monitor.Top), int(info.Monitor.Right), int(info.Monitor.Bottom)).Sub(state.desktop.Min).Intersect(state.client)
			}
		}
	}
	// A monitor fallback must also contain the entire indicated area.
	if state.candidateExtent.active && !state.candidateExtent.path.In(r) {
		r = state.client
	}
	changed := r != state.candidate
	state.candidate = r
	state.candidateExtent.chosen = r
	return changed
}
