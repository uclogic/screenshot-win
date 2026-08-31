//go:build windows

package selector

import (
	"fmt"
	"image"
	"runtime"
	"syscall"
	"unsafe"
)

type borderStart struct {
	hwnd uintptr
	err  error
}

// ShowBorder displays a click-through blue border just outside region. The
// window remains visible until the returned Border is closed.
func ShowBorder(region image.Rectangle) (*Border, error) {
	desktop := virtualDesktopBounds()
	region = region.Intersect(desktop)
	if region.Empty() {
		return nil, fmt.Errorf("capture region %v is outside virtual desktop %v", region, desktop)
	}

	started := make(chan borderStart, 1)
	done := make(chan struct{})
	go runBorderWindow(desktop, region, started, done)

	result := <-started
	if result.err != nil {
		<-done
		return nil, result.err
	}
	return &Border{
		window:      result.hwnd,
		closeWindow: func() { procPostMessage.Call(result.hwnd, wmClose, 0, 0) },
		done:        done,
	}, nil
}

func runBorderWindow(desktop, region image.Rectangle, started chan<- borderStart, done chan<- struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(done)

	instance, _, callErr := procGetModuleHandle.Call(0)
	if instance == 0 {
		started <- borderStart{err: win32Error("GetModuleHandleW", callErr)}
		return
	}
	className, err := syscall.UTF16PtrFromString("ScreenshotWinCaptureBorder")
	if err != nil {
		started <- borderStart{err: err}
		return
	}
	class := windowClassEx{
		Size: uint32(unsafe.Sizeof(windowClassEx{})), WindowProcedure: borderProcedure,
		Instance: instance, ClassName: className,
	}
	atom, _, callErr := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		started <- borderStart{err: win32Error("RegisterClassExW", callErr)}
		return
	}
	defer procUnregisterClass.Call(uintptr(unsafe.Pointer(className)), instance)

	title, _ := syscall.UTF16PtrFromString("screenshot-win capture border")
	hwnd, _, callErr := procCreateWindowEx.Call(
		wsExTopmost|wsExToolWindow|wsExLayered|wsExTransparent|wsExNoActivate,
		uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)), wsPopup,
		uintptr(desktop.Min.X), uintptr(desktop.Min.Y), uintptr(desktop.Dx()), uintptr(desktop.Dy()),
		0, 0, instance, 0,
	)
	if hwnd == 0 {
		started <- borderStart{err: win32Error("CreateWindowExW", callErr)}
		return
	}

	state := newSelectionState(desktop)
	state.hwnd = hwnd
	if err := state.initializeSurface(); err != nil {
		procDestroyWindow.Call(hwnd)
		started <- borderStart{err: err}
		return
	}
	defer state.closeSurface()
	if err := state.renderCaptureBorder(region); err != nil {
		procDestroyWindow.Call(hwnd)
		started <- borderStart{err: err}
		return
	}
	procShowWindow.Call(hwnd, swShowNoActivate)
	started <- borderStart{hwnd: hwnd}

	var msg message
	for {
		status, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(status) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
	runtime.KeepAlive(state)
}

func borderWindowProcedure(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmNCHitTest:
		return htTransparent
	case wmClose:
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	result, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}

func (state *selectionState) renderCaptureBorder(region image.Rectangle) error {
	clear(state.pixels)
	local := region.Sub(state.desktop.Min)
	drawOuterPixelBorder(state.pixels, state.client.Dx(), state.client, local)
	return state.present()
}

func drawOuterPixelBorder(pixels []byte, width int, bounds, selection image.Rectangle) {
	const thickness = 3
	edges := []image.Rectangle{
		image.Rect(selection.Min.X-thickness, selection.Min.Y-thickness, selection.Max.X+thickness, selection.Min.Y),
		image.Rect(selection.Min.X-thickness, selection.Max.Y, selection.Max.X+thickness, selection.Max.Y+thickness),
		image.Rect(selection.Min.X-thickness, selection.Min.Y, selection.Min.X, selection.Max.Y),
		image.Rect(selection.Max.X, selection.Min.Y, selection.Max.X+thickness, selection.Max.Y),
	}
	for _, edge := range edges {
		edge = edge.Intersect(bounds)
		for y := edge.Min.Y; y < edge.Max.Y; y++ {
			for x := edge.Min.X; x < edge.Max.X; x++ {
				setBluePixel(pixels, (y*width+x)*4)
			}
		}
	}
}
