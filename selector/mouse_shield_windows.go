//go:build windows

package selector

import (
	"fmt"
	"image"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

const (
	wmMouseHWheel = 0x020E
)

var (
	procMouseShieldGetCursorPos = user32.NewProc("GetCursorPos")
	procWindowFromPoint         = user32.NewProc("WindowFromPoint")
	procGetForegroundWindow     = user32.NewProc("GetForegroundWindow")
	mouseShieldProcedure        = syscall.NewCallback(mouseShieldWindowProcedure)
	mouseShieldStates           sync.Map
)

type mouseShieldStart struct {
	hwnd uintptr
	err  error
}

type mouseShieldState struct {
	*selectionState
	wheelTarget uintptr
}

// ShowMouseShield places a non-activating input window over region. Its nearly
// transparent fallback surface keeps older Windows versions usable when
// WDA_EXCLUDEFROMCAPTURE is unavailable.
func ShowMouseShield(region image.Rectangle) (*MouseShield, error) {
	region = region.Intersect(virtualDesktopBounds())
	if region.Empty() {
		return nil, fmt.Errorf("mouse shield region is outside the virtual desktop")
	}
	started := make(chan mouseShieldStart, 1)
	done := make(chan struct{})
	wheelTarget := mouseShieldWheelTarget(region)
	go runMouseShieldWindow(region, wheelTarget, started, done)
	result := <-started
	if result.err != nil {
		<-done
		return nil, result.err
	}
	return &MouseShield{
		closeWindow: func() { procPostMessage.Call(result.hwnd, wmClose, 0, 0) },
		done:        done,
	}, nil
}

func runMouseShieldWindow(region image.Rectangle, wheelTarget uintptr, started chan<- mouseShieldStart, done chan<- struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(done)

	instance, _, callErr := procGetModuleHandle.Call(0)
	if instance == 0 {
		started <- mouseShieldStart{err: win32Error("GetModuleHandleW", callErr)}
		return
	}
	className, _ := syscall.UTF16PtrFromString("ScreenshotWinMouseShield")
	arrow, _, callErr := procLoadCursor.Call(0, 32512)
	if arrow == 0 {
		started <- mouseShieldStart{err: win32Error("LoadCursorW", callErr)}
		return
	}
	class := windowClassEx{
		Size: uint32(unsafe.Sizeof(windowClassEx{})), WindowProcedure: mouseShieldProcedure,
		Instance: instance, Cursor: arrow, ClassName: className,
	}
	atom, _, callErr := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		started <- mouseShieldStart{err: win32Error("RegisterClassExW", callErr)}
		return
	}
	defer procUnregisterClass.Call(uintptr(unsafe.Pointer(className)), instance)

	title, _ := syscall.UTF16PtrFromString("screenshot-win mouse shield")
	hwnd, _, callErr := procCreateWindowEx.Call(
		wsExTopmost|wsExToolWindow|wsExLayered|wsExNoActivate,
		uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)), wsPopup,
		uintptr(region.Min.X), uintptr(region.Min.Y), uintptr(region.Dx()), uintptr(region.Dy()),
		0, 0, instance, 0,
	)
	if hwnd == 0 {
		started <- mouseShieldStart{err: win32Error("CreateWindowExW", callErr)}
		return
	}

	state := &mouseShieldState{selectionState: newSelectionState(region), wheelTarget: wheelTarget}
	state.hwnd = hwnd
	mouseShieldStates.Store(hwnd, state)
	defer mouseShieldStates.Delete(hwnd)
	if err := state.initializeSurface(); err != nil {
		procDestroyWindow.Call(hwnd)
		started <- mouseShieldStart{err: err}
		return
	}
	defer state.closeSurface()
	for index := 3; index < len(state.pixels); index += 4 {
		state.pixels[index] = 1
	}
	if err := state.present(); err != nil {
		procDestroyWindow.Call(hwnd)
		started <- mouseShieldStart{err: err}
		return
	}
	procShowWindow.Call(hwnd, swShowNoActivate)
	if ensureCaptureExclusionSupported() == nil {
		procSetWindowDisplayAffinity.Call(hwnd, wdaExcludeFromCapture)
	}
	started <- mouseShieldStart{hwnd: hwnd}

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

func mouseShieldWheelTarget(region image.Rectangle) uintptr {
	cursor := point{X: int32(region.Min.X + region.Dx()/2), Y: int32(region.Min.Y + region.Dy()/2)}
	var current point
	if ok, _, _ := procMouseShieldGetCursorPos.Call(uintptr(unsafe.Pointer(&current))); ok != 0 {
		at := image.Pt(int(current.X), int(current.Y))
		if at.In(region) {
			cursor = current
		}
	}
	// The supported release target is windows/amd64, where POINT is passed in
	// one 64-bit register. Query before creating the shield so the result is the
	// actual child window underneath the capture region.
	packedPoint := uintptr(uint64(uint32(cursor.X)) | uint64(uint32(cursor.Y))<<32)
	target, _, _ := procWindowFromPoint.Call(packedPoint)
	if target == 0 {
		target, _, _ = procGetForegroundWindow.Call()
	}
	return target
}

func mouseShieldWindowProcedure(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	value, found := mouseShieldStates.Load(hwnd)
	if !found {
		result, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
		return result
	}
	state := value.(*mouseShieldState)
	switch message {
	case wmNCHitTest:
		return htClient
	case wmMouseWheel, wmMouseHWheel:
		if state.wheelTarget != 0 && state.wheelTarget != hwnd {
			procPostMessage.Call(state.wheelTarget, uintptr(message), wParam, lParam)
		}
		return 0
	case wmMouseMove, wmLButtonDown, wmLButtonUp, wmRButtonDown:
		return 0
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
