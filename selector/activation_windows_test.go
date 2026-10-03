//go:build windows

package selector

import (
	"image"
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"screenshot-win/editor"
)

func TestSwitchingToolbarToolsDoesNotRequestCanvasActivation(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	className, _ := syscall.UTF16PtrFromString("STATIC")
	// A message-only window records canvas commands without opening a desktop UI.
	canvas, _, err := procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(className)), 0, 0, 0, 0, 0, 0, ^uintptr(2), 0, 0, 0)
	if canvas == 0 {
		t.Fatal(win32Error("CreateWindowExW activation test", err))
	}
	defer procDestroyWindow.Call(canvas)
	const hwnd = uintptr(0x7fffffd0)
	events := make(chan ToolbarEvent, 1)
	state := &toolbarState{
		hwnd: hwnd, persistent: true, ready: true, motionClosed: true,
		shortcutTarget: canvas, events: events, dpi: 96,
		actions: []Action{ActionRectangle, ActionArrow, ActionText},
	}
	toolbarStates.Store(hwnd, state)
	defer toolbarStates.Delete(hwnd)
	peekMessage := user32.NewProc("PeekMessageW")
	for _, index := range []int{0, 1, 2, 0, 2, 1} {
		button := state.buttonRect(index)
		point := button.Min.Add(button.Size().Div(2))
		location := uintptr(uint16(point.X)) | uintptr(uint16(point.Y))<<16
		toolbarWindowProcedure(hwnd, wmLButtonDown, 0, location)
		toolbarWindowProcedure(hwnd, wmLButtonUp, 0, location)
		if state.pending {
			state.rearmPersistentActions()
		}
		select {
		case event := <-events:
			if event.Action != state.actions[index] {
				t.Fatalf("tool switch emitted %v, want %v", event.Action, state.actions[index])
			}
		default:
			t.Fatal("tool switch did not emit an action")
		}
		var queued message
		// wmUser+107 was the focus command posted on every drawing-tool click.
		if found, _, _ := peekMessage.Call(uintptr(unsafe.Pointer(&queued)), canvas, wmUser+107, wmUser+107, 1); found != 0 {
			t.Fatal("tool selection requested canvas activation without a canvas click")
		}
		if !state.hovering || state.hover != state.actions[index] || state.pressed {
			t.Fatal("tool switch lost pointer feedback")
		}
	}
}

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
