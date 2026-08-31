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
	csDropShadow       = 0x00020000
	wmContextMenu      = 0x007B
	wmMouseWheel       = 0x020A
	wmRButtonUp        = 0x0205
	htCaption          = 2
	mfString           = 0
	tpmRightButton     = 0x0002
	tpmReturnCmd       = 0x0100
	pinCommandOriginal = 2001
	pinCommandClose    = 2002
	dibStretchHalftone = 4
	rasterSourceCopy   = 0x00CC0020
)

var (
	procPinGetWindowRect   = user32.NewProc("GetWindowRect")
	procPinGetCursorPos    = user32.NewProc("GetCursorPos")
	procPinCreatePopupMenu = user32.NewProc("CreatePopupMenu")
	procPinAppendMenu      = user32.NewProc("AppendMenuW")
	procPinTrackPopupMenu  = user32.NewProc("TrackPopupMenu")
	procPinDestroyMenu     = user32.NewProc("DestroyMenu")
	procPinSetStretchMode  = gdi32.NewProc("SetStretchBltMode")
	procPinStretchDIBits   = gdi32.NewProc("StretchDIBits")
	pinProcedure           = syscall.NewCallback(pinWindowProcedure)
	pinStates              sync.Map
	pinClassOnce           sync.Once
	pinClassInstance       uintptr
	pinClassName           *uint16
	pinClassErr            error
)

type pinStart struct {
	hwnd uintptr
	err  error
}

type pinWindowState struct {
	hwnd       uintptr
	original   image.Point
	pixels     []byte
	bitmapInfo bitmapInfo
	renderErr  error
}

func showPinnedWindow(source image.Image, origin image.Point) (*Pin, error) {
	started := make(chan pinStart, 1)
	done := make(chan struct{})
	go runPinnedWindow(source, origin, started, done)
	result := <-started
	if result.err != nil {
		<-done
		return nil, result.err
	}
	return &Pin{closeWindow: func() { procPostMessage.Call(result.hwnd, wmClose, 0, 0) }, done: done}, nil
}

func ensurePinWindowClass() (uintptr, *uint16, error) {
	pinClassOnce.Do(func() {
		instance, _, callErr := procGetModuleHandle.Call(0)
		if instance == 0 {
			pinClassErr = win32Error("GetModuleHandleW", callErr)
			return
		}
		className, err := syscall.UTF16PtrFromString("ScreenshotWinPinnedImage")
		if err != nil {
			pinClassErr = err
			return
		}
		cursor, _, callErr := procLoadCursor.Call(0, idcArrow)
		if cursor == 0 {
			pinClassErr = win32Error("LoadCursorW", callErr)
			return
		}
		class := windowClassEx{Size: uint32(unsafe.Sizeof(windowClassEx{})), Style: csDropShadow, WindowProcedure: pinProcedure, Instance: instance, Cursor: cursor, ClassName: className}
		if atom, _, callErr := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
			pinClassErr = win32Error("RegisterClassExW", callErr)
			return
		}
		pinClassInstance, pinClassName = instance, className
	})
	return pinClassInstance, pinClassName, pinClassErr
}

func runPinnedWindow(source image.Image, origin image.Point, started chan<- pinStart, done chan<- struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(done)
	instance, className, err := ensurePinWindowClass()
	if err != nil {
		started <- pinStart{err: err}
		return
	}
	original := source.Bounds().Size()
	workArea, err := monitorWorkArea(image.Rectangle{Min: origin, Max: origin.Add(original)})
	if err != nil {
		started <- pinStart{err: err}
		return
	}
	bounds := pinInitialBounds(original, origin, workArea)
	pixels := make([]byte, original.X*original.Y*4)
	if err := copyImageToBGRA(pixels, original.X, original.Y, source); err != nil {
		started <- pinStart{err: err}
		return
	}
	state := &pinWindowState{original: original, pixels: pixels}
	state.bitmapInfo.Header = bitmapInfoHeader{Size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), Width: int32(original.X), Height: -int32(original.Y), Planes: 1, BitCount: 32, Compression: biRGB, SizeImage: uint32(len(pixels))}
	title, _ := syscall.UTF16PtrFromString("screenshot-win pinned image")
	hwnd, _, callErr := procCreateWindowEx.Call(wsExTopmost|wsExToolWindow, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)), wsPopup,
		uintptr(bounds.Min.X), uintptr(bounds.Min.Y), uintptr(bounds.Dx()), uintptr(bounds.Dy()), 0, 0, instance, 0)
	if hwnd == 0 {
		started <- pinStart{err: win32Error("CreateWindowExW", callErr)}
		return
	}
	state.hwnd = hwnd
	pinStates.Store(hwnd, state)
	defer pinStates.Delete(hwnd)
	procShowWindow.Call(hwnd, swShow)
	procSetForegroundWindow.Call(hwnd)
	started <- pinStart{hwnd: hwnd}
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

func pinWindowProcedure(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	value, found := pinStates.Load(hwnd)
	if !found {
		result, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
		return result
	}
	state := value.(*pinWindowState)
	switch message {
	case wmNCHitTest:
		return htCaption
	case wmMouseWheel:
		bounds, ok := pinWindowBounds(hwnd)
		if ok {
			delta := int(int16((wParam >> 16) & 0xffff))
			cursor := image.Pt(int(int16(lParam&0xffff)), int(int16((lParam>>16)&0xffff)))
			zoomed, _ := pinZoomBounds(bounds, state.original, cursor, delta)
			pinMoveWindow(hwnd, zoomed)
		}
		return 0
	case wmContextMenu, wmRButtonUp:
		state.showContextMenu(hwnd)
		return 0
	case wmKeyDown:
		if wParam == vkEscape {
			procDestroyWindow.Call(hwnd)
			return 0
		}
	case wmPaint:
		if err := state.paint(hwnd); err != nil {
			state.renderErr = err
			procDestroyWindow.Call(hwnd)
		}
		return 0
	case wmEraseBkgnd:
		return 1
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

func pinWindowBounds(hwnd uintptr) (image.Rectangle, bool) {
	var area rect
	if ok, _, _ := procPinGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&area))); ok == 0 {
		return image.Rectangle{}, false
	}
	return image.Rect(int(area.Left), int(area.Top), int(area.Right), int(area.Bottom)), true
}

func pinMoveWindow(hwnd uintptr, bounds image.Rectangle) {
	procSetWindowPos.Call(hwnd, ^uintptr(0), uintptr(bounds.Min.X), uintptr(bounds.Min.Y), uintptr(bounds.Dx()), uintptr(bounds.Dy()), 0)
	procInvalidateRect.Call(hwnd, 0, 0)
}

func (state *pinWindowState) showContextMenu(hwnd uintptr) {
	menu, _, _ := procPinCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procPinDestroyMenu.Call(menu)
	originalText, _ := syscall.UTF16PtrFromString("原始大小")
	closeText, _ := syscall.UTF16PtrFromString("关闭")
	procPinAppendMenu.Call(menu, mfString, pinCommandOriginal, uintptr(unsafe.Pointer(originalText)))
	procPinAppendMenu.Call(menu, mfString, pinCommandClose, uintptr(unsafe.Pointer(closeText)))
	var cursor point
	procPinGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor)))
	procSetForegroundWindow.Call(hwnd)
	command, _, _ := procPinTrackPopupMenu.Call(menu, tpmRightButton|tpmReturnCmd, uintptr(cursor.X), uintptr(cursor.Y), 0, hwnd, 0)
	switch command {
	case pinCommandOriginal:
		if bounds, ok := pinWindowBounds(hwnd); ok {
			pinMoveWindow(hwnd, pinResetBounds(bounds, state.original))
		}
	case pinCommandClose:
		procDestroyWindow.Call(hwnd)
	}
}

func (state *pinWindowState) paint(hwnd uintptr) error {
	var paint paintStruct
	dc, _, callErr := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&paint)))
	if dc == 0 {
		return win32Error("BeginPaint", callErr)
	}
	defer procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&paint)))
	bounds, ok := pinWindowBounds(hwnd)
	if !ok {
		return fmt.Errorf("GetWindowRect failed")
	}
	procPinSetStretchMode.Call(dc, dibStretchHalftone)
	result, _, callErr := procPinStretchDIBits.Call(dc, 0, 0, uintptr(bounds.Dx()), uintptr(bounds.Dy()), 0, 0,
		uintptr(state.original.X), uintptr(state.original.Y), uintptr(unsafe.Pointer(&state.pixels[0])), uintptr(unsafe.Pointer(&state.bitmapInfo)), dibRGBColors, rasterSourceCopy)
	if int32(result) == -1 {
		return win32Error("StretchDIBits", callErr)
	}
	return nil
}
