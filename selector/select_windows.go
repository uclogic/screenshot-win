//go:build windows

package selector

import (
	"context"
	"fmt"
	"image"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCXVirtualScreen = 78
	smCYVirtualScreen = 79

	csHRedraw = 0x0002
	csVRedraw = 0x0001

	wsPopup         = 0x80000000
	wsExTopmost     = 0x00000008
	wsExToolWindow  = 0x00000080
	wsExLayered     = 0x00080000
	wsExTransparent = 0x00000020
	wsExNoActivate  = 0x08000000

	swShow           = 5
	swHide           = 0
	swShowNoActivate = 4

	wmDestroy     = 0x0002
	wmClose       = 0x0010
	wmNCHitTest   = 0x0084
	wmKeyDown     = 0x0100
	wmMouseMove   = 0x0200
	wmLButtonDown = 0x0201
	wmLButtonUp   = 0x0202
	wmRButtonDown = 0x0204

	vkEscape      = 0x1B
	idcCross      = 32515
	htTransparent = ^uintptr(0)

	dibRGBColors = 0
	biRGB        = 0
	ulwAlpha     = 0x00000002
	acSrcOver    = 0
	acSrcAlpha   = 1

	selectionShadeAlpha = 48
)

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	gdi32                   = syscall.NewLazyDLL("gdi32.dll")
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	procRegisterClassEx     = user32.NewProc("RegisterClassExW")
	procUnregisterClass     = user32.NewProc("UnregisterClassW")
	procCreateWindowEx      = user32.NewProc("CreateWindowExW")
	procDefWindowProc       = user32.NewProc("DefWindowProcW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procPostMessage         = user32.NewProc("PostMessageW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procSetFocus            = user32.NewProc("SetFocus")
	procUpdateLayeredWindow = user32.NewProc("UpdateLayeredWindow")
	procGetMessage          = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessage     = user32.NewProc("DispatchMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procSetCapture          = user32.NewProc("SetCapture")
	procReleaseCapture      = user32.NewProc("ReleaseCapture")
	procLoadCursor          = user32.NewProc("LoadCursorW")
	procGetDC               = user32.NewProc("GetDC")
	procReleaseDC           = user32.NewProc("ReleaseDC")
	procCreateCompatibleDC  = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC            = gdi32.NewProc("DeleteDC")
	procCreateDIBSection    = gdi32.NewProc("CreateDIBSection")
	procSelectObject        = gdi32.NewProc("SelectObject")
	procDeleteObject        = gdi32.NewProc("DeleteObject")
	procGetModuleHandle     = kernel32.NewProc("GetModuleHandleW")

	windowProcedure = syscall.NewCallback(selectionWindowProcedure)
	borderProcedure = syscall.NewCallback(borderWindowProcedure)
	activeSelection *selectionState
)

type selectionState struct {
	hwnd      uintptr
	desktop   image.Rectangle
	client    image.Rectangle
	anchor    image.Point
	current   image.Point
	dragging  bool
	selected  bool
	result    image.Rectangle
	renderErr error
	memoryDC  uintptr
	bitmap    uintptr
	oldBitmap uintptr
	pixels    []byte
}

type windowClassEx struct {
	Size, Style             uint32
	WindowProcedure         uintptr
	ClassExtra, WindowExtra int32
	Instance, Icon, Cursor  uintptr
	Background              uintptr
	MenuName, ClassName     *uint16
	SmallIcon               uintptr
}

type point struct{ X, Y int32 }
type size struct{ Width, Height int32 }

type message struct {
	Window  uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   point
	Private uint32
}

type blendFunction struct {
	Operation, Flags, SourceConstantAlpha, AlphaFormat byte
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}

// Select displays a virtual-desktop overlay and returns the rectangle dragged
// by the user in physical virtual-desktop coordinates. A false selected value
// means that the user cancelled with Escape, right-click, or window close.
func Select() (result image.Rectangle, selected bool, err error) {
	return SelectContext(context.Background())
}

// SelectContext behaves like Select and closes the overlay when ctx is done.
func SelectContext(ctx context.Context) (result image.Rectangle, selected bool, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	desktop := virtualDesktopBounds()
	if desktop.Empty() {
		return image.Rectangle{}, false, fmt.Errorf("virtual desktop has invalid bounds %v", desktop)
	}
	state := newSelectionState(desktop)

	instance, _, callErr := procGetModuleHandle.Call(0)
	if instance == 0 {
		return image.Rectangle{}, false, win32Error("GetModuleHandleW", callErr)
	}
	className, err := syscall.UTF16PtrFromString("ScreenshotWinSelectionOverlay")
	if err != nil {
		return image.Rectangle{}, false, err
	}
	cursor, _, callErr := procLoadCursor.Call(0, idcCross)
	if cursor == 0 {
		return image.Rectangle{}, false, win32Error("LoadCursorW", callErr)
	}
	class := windowClassEx{
		Size: uint32(unsafe.Sizeof(windowClassEx{})), Style: csHRedraw | csVRedraw,
		WindowProcedure: windowProcedure, Instance: instance, Cursor: cursor, ClassName: className,
	}
	atom, _, callErr := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		return image.Rectangle{}, false, win32Error("RegisterClassExW", callErr)
	}
	defer procUnregisterClass.Call(uintptr(unsafe.Pointer(className)), instance)

	title, _ := syscall.UTF16PtrFromString("screenshot-win")
	activeSelection = state
	defer func() { activeSelection = nil }()
	hwnd, _, callErr := procCreateWindowEx.Call(
		wsExTopmost|wsExToolWindow|wsExLayered,
		uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)), wsPopup,
		uintptr(desktop.Min.X), uintptr(desktop.Min.Y), uintptr(desktop.Dx()), uintptr(desktop.Dy()),
		0, 0, instance, 0,
	)
	if hwnd == 0 {
		return image.Rectangle{}, false, win32Error("CreateWindowExW", callErr)
	}
	state.hwnd = hwnd
	stopCancellation := context.AfterFunc(ctx, func() {
		procPostMessage.Call(hwnd, wmClose, 0, 0)
	})
	defer stopCancellation()
	if err := state.initializeSurface(); err != nil {
		procDestroyWindow.Call(hwnd)
		return image.Rectangle{}, false, err
	}
	defer state.closeSurface()
	if err := state.render(); err != nil {
		procDestroyWindow.Call(hwnd)
		return image.Rectangle{}, false, err
	}
	procShowWindow.Call(hwnd, swShow)
	procSetForegroundWindow.Call(hwnd)
	procSetFocus.Call(hwnd)

	var msg message
	for {
		status, _, callErr := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(status) == -1 {
			procDestroyWindow.Call(hwnd)
			return image.Rectangle{}, false, win32Error("GetMessageW", callErr)
		}
		if status == 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
	runtime.KeepAlive(state)
	if state.renderErr != nil {
		return image.Rectangle{}, false, state.renderErr
	}
	if err := ctx.Err(); err != nil {
		return image.Rectangle{}, false, err
	}
	return state.result, state.selected, nil
}

func virtualDesktopBounds() image.Rectangle {
	x := systemMetric(smXVirtualScreen)
	y := systemMetric(smYVirtualScreen)
	return image.Rect(x, y, x+systemMetric(smCXVirtualScreen), y+systemMetric(smCYVirtualScreen))
}

// DesktopBounds returns the physical-pixel bounds of the Windows virtual desktop.
func DesktopBounds() image.Rectangle { return virtualDesktopBounds() }

func systemMetric(index uintptr) int {
	value, _, _ := procGetSystemMetrics.Call(index)
	return int(int32(value))
}

func newSelectionState(desktop image.Rectangle) *selectionState {
	return &selectionState{desktop: desktop, client: image.Rect(0, 0, desktop.Dx(), desktop.Dy())}
}

func selectionWindowProcedure(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	state := activeSelection
	if state == nil {
		result, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
		return result
	}
	switch message {
	case wmLButtonDown:
		state.anchor = mousePoint(lParam)
		state.current = state.anchor
		state.dragging = true
		procSetCapture.Call(hwnd)
		state.renderOrClose()
		return 0
	case wmMouseMove:
		if state.dragging {
			state.current = mousePoint(lParam)
			state.renderOrClose()
		}
		return 0
	case wmLButtonUp:
		if state.dragging {
			state.current = mousePoint(lParam)
			state.dragging = false
			procReleaseCapture.Call()
			selection := dragRectangle(state.client, state.anchor, state.current)
			if !selection.Empty() {
				state.result = desktopRectangle(state.desktop, state.anchor, state.current)
				state.selected = true
				procDestroyWindow.Call(hwnd)
			} else {
				state.renderOrClose()
			}
		}
		return 0
	case wmRButtonDown, wmClose:
		procDestroyWindow.Call(hwnd)
		return 0
	case wmKeyDown:
		if wParam == vkEscape {
			procDestroyWindow.Call(hwnd)
			return 0
		}
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	result, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}

func mousePoint(lParam uintptr) image.Point {
	return image.Pt(int(int16(lParam&0xffff)), int(int16((lParam>>16)&0xffff)))
}

func (state *selectionState) initializeSurface() error {
	screenDC, _, callErr := procGetDC.Call(0)
	if screenDC == 0 {
		return win32Error("GetDC", callErr)
	}
	defer procReleaseDC.Call(0, screenDC)

	state.memoryDC, _, callErr = procCreateCompatibleDC.Call(screenDC)
	if state.memoryDC == 0 {
		return win32Error("CreateCompatibleDC", callErr)
	}
	info := bitmapInfo{Header: bitmapInfoHeader{
		Size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), Width: int32(state.client.Dx()),
		Height: -int32(state.client.Dy()), Planes: 1, BitCount: 32, Compression: biRGB,
	}}
	var bits unsafe.Pointer
	state.bitmap, _, callErr = procCreateDIBSection.Call(
		screenDC, uintptr(unsafe.Pointer(&info)), dibRGBColors, uintptr(unsafe.Pointer(&bits)), 0, 0,
	)
	if state.bitmap == 0 || bits == nil {
		state.closeSurface()
		return win32Error("CreateDIBSection", callErr)
	}
	state.oldBitmap, _, callErr = procSelectObject.Call(state.memoryDC, state.bitmap)
	if state.oldBitmap == 0 || state.oldBitmap == ^uintptr(0) {
		state.closeSurface()
		return win32Error("SelectObject", callErr)
	}
	state.pixels = unsafe.Slice((*byte)(bits), state.client.Dx()*state.client.Dy()*4)
	return nil
}

func (state *selectionState) closeSurface() {
	state.pixels = nil
	if state.memoryDC != 0 && state.oldBitmap != 0 && state.oldBitmap != ^uintptr(0) {
		procSelectObject.Call(state.memoryDC, state.oldBitmap)
		state.oldBitmap = 0
	}
	if state.bitmap != 0 {
		procDeleteObject.Call(state.bitmap)
		state.bitmap = 0
	}
	if state.memoryDC != 0 {
		procDeleteDC.Call(state.memoryDC)
		state.memoryDC = 0
	}
}

func (state *selectionState) renderOrClose() {
	if err := state.render(); err != nil {
		state.renderErr = err
		procDestroyWindow.Call(state.hwnd)
	}
}

func (state *selectionState) render() error {
	selection := dragRectangle(state.client, state.anchor, state.current)
	drawSelectionOverlay(state.pixels, state.client.Dx(), selection, state.dragging)
	return state.present()
}

func (state *selectionState) present() error {
	destination := point{X: int32(state.desktop.Min.X), Y: int32(state.desktop.Min.Y)}
	source := point{}
	dimensions := size{Width: int32(state.client.Dx()), Height: int32(state.client.Dy())}
	blend := blendFunction{Operation: acSrcOver, SourceConstantAlpha: 255, AlphaFormat: acSrcAlpha}
	ok, _, callErr := procUpdateLayeredWindow.Call(
		state.hwnd, 0, uintptr(unsafe.Pointer(&destination)), uintptr(unsafe.Pointer(&dimensions)),
		state.memoryDC, uintptr(unsafe.Pointer(&source)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha,
	)
	if ok == 0 {
		return win32Error("UpdateLayeredWindow", callErr)
	}
	return nil
}

func drawPixelBorder(pixels []byte, width int, selection image.Rectangle) {
	const thickness = 2
	edges := []image.Rectangle{
		image.Rect(selection.Min.X, selection.Min.Y, selection.Max.X, min(selection.Min.Y+thickness, selection.Max.Y)),
		image.Rect(selection.Min.X, max(selection.Max.Y-thickness, selection.Min.Y), selection.Max.X, selection.Max.Y),
		image.Rect(selection.Min.X, selection.Min.Y, min(selection.Min.X+thickness, selection.Max.X), selection.Max.Y),
		image.Rect(max(selection.Max.X-thickness, selection.Min.X), selection.Min.Y, selection.Max.X, selection.Max.Y),
	}
	for _, edge := range edges {
		for y := edge.Min.Y; y < edge.Max.Y; y++ {
			for x := edge.Min.X; x < edge.Max.X; x++ {
				index := (y*width + x) * 4
				setBluePixel(pixels, index)
			}
		}
	}
}

func drawSelectionOverlay(pixels []byte, width int, selection image.Rectangle, dragging bool) {
	for index := 0; index < len(pixels); index += 4 {
		pixels[index], pixels[index+1], pixels[index+2], pixels[index+3] = 0, 0, 0, selectionShadeAlpha
	}
	if !dragging || selection.Empty() || width <= 0 {
		return
	}
	height := len(pixels) / (width * 4)
	selection = selection.Intersect(image.Rect(0, 0, width, height))
	for y := selection.Min.Y; y < selection.Max.Y; y++ {
		for x := selection.Min.X; x < selection.Max.X; x++ {
			index := (y*width + x) * 4
			// Alpha 1 keeps the layered overlay hit-testable while remaining
			// visually transparent over the selected desktop area.
			pixels[index], pixels[index+1], pixels[index+2], pixels[index+3] = 0, 0, 0, 1
		}
	}
	drawPixelBorder(pixels, width, selection)
}

func setBluePixel(pixels []byte, index int) {
	// DIB pixels are BGRA. #168CFF stays visible on both light and dark pages.
	pixels[index], pixels[index+1], pixels[index+2], pixels[index+3] = 0xff, 0x8c, 0x16, 0xff
}

func win32Error(operation string, callErr error) error {
	if errno, ok := callErr.(syscall.Errno); ok && errno == 0 {
		return fmt.Errorf("%s failed (GetLastError was not set)", operation)
	}
	return fmt.Errorf("%s failed (GetLastError=%v)", operation, callErr)
}
