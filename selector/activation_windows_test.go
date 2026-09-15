//go:build windows

package selector

import (
	"image"
	"runtime"
	"testing"

	"screenshot-win/editor"
)

func TestSelectionDeactivationCancelsUnfinishedDrag(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	state := newSelectionState(image.Rect(0, 0, 100, 100))
	state.dragging = true
	state.gesture = candidateGesture{pressed: true, threshold: 4}
	previous := activeSelection
	activeSelection = state
	defer func() { activeSelection = previous }()
	selectionWindowProcedure(0, wmActivate, 0, 0)
	if state.dragging || state.gesture.pressed || state.gesture.threshold != 4 {
		t.Fatal("deactivation retained a drag whose button-up may never arrive")
	}
}

func TestFrozenDeactivationPreservesToolButDropsGesture(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	request := &frozenAnnotationRequest{tool: editor.ToolArrow, dragging: true, drawing: true}
	state := &frozenState{selectionState: newSelectionState(image.Rect(0, 0, 100, 100)), request: request, panning: true}
	const hwnd = uintptr(0x7ffffff0)
	frozenStates.Store(hwnd, state)
	defer frozenStates.Delete(hwnd)
	frozenWindowProcedure(hwnd, wmActivate, 0, 0)
	if state.panning || request.dragging || state.request != request || request.tool != editor.ToolArrow {
		t.Fatal("deactivation must end the gesture without discarding the active tool")
	}
}

func TestToolbarDeactivationClearsPressedState(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const hwnd = uintptr(0x7ffffff0)
	state := &toolbarState{hwnd: hwnd, persistent: true, pressed: true, hovering: true, active: true, activeAction: ActionArrow}
	toolbarStates.Store(hwnd, state)
	defer toolbarStates.Delete(hwnd)
	toolbarWindowProcedure(hwnd, wmActivate, 0, 0)
	if state.pressed || state.hovering || !state.active || state.activeAction != ActionArrow {
		t.Fatal("deactivation must clear pointer feedback and retain tool selection")
	}
}
