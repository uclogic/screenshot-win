//go:build windows

package selector

import "testing"

func TestCanvasActivationStaysBelowItsToolbar(t *testing.T) {
	const canvas, toolbar = uintptr(0x7fffffe0), uintptr(0x7fffffe1)
	previous := activeToolbarWindow.Load()
	activeToolbarWindow.Store(toolbar)
	toolbarStates.Store(toolbar, &toolbarState{persistent: true, shortcutTarget: canvas})
	defer activeToolbarWindow.Store(previous)
	defer toolbarStates.Delete(toolbar)
	for _, insertAfter := range []uintptr{0, hwndTopmost} {
		position := windowPosition{hwnd: canvas, insertAfter: insertAfter, x: 10, y: 20, cx: 100, cy: 200}
		keepCanvasBelowToolbar(&position, canvas)
		if position.insertAfter != toolbar {
			t.Fatalf("canvas raise placed after %#x, want toolbar %#x", position.insertAfter, toolbar)
		}
		if position.x != 10 || position.y != 20 || position.cx != 100 || position.cy != 200 || position.flags != 0 {
			t.Fatal("ordering changed canvas geometry or activation flags")
		}
	}
	// The owned text editor uses the canvas handle to identify its toolbar.
	position := windowPosition{hwnd: canvas + 2, insertAfter: hwndTopmost}
	keepCanvasBelowToolbar(&position, canvas)
	if position.insertAfter != toolbar {
		t.Fatal("text editor activation can cover the toolbar")
	}
	for _, position := range []windowPosition{
		{insertAfter: 0, flags: swpNoZOrder},
		{insertAfter: 1},           // HWND_BOTTOM
		{insertAfter: ^uintptr(1)}, // HWND_NOTOPMOST
		{insertAfter: canvas + 3},
	} {
		original := position
		keepCanvasBelowToolbar(&position, canvas)
		if position != original {
			t.Fatal("changed an operation that did not raise the canvas")
		}
	}
	position = windowPosition{insertAfter: hwndTopmost}
	keepCanvasBelowToolbar(&position, canvas+10)
	if position.insertAfter != hwndTopmost {
		t.Fatal("toolbar affected an unrelated canvas")
	}
	toolbarStates.Delete(toolbar)
	keepCanvasBelowToolbar(&position, canvas)
	if position.insertAfter != hwndTopmost {
		t.Fatal("closed toolbar affected canvas order")
	}
	keepCanvasBelowToolbar(nil, canvas)
}
