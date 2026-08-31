//go:build windows

package capture

import (
	"fmt"
	"image"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	srcCopyCaptureBLT = 0x40CC0020
	dibRGBColors      = 0
	biRGB             = 0
	vkEscape          = 0x1B
)

var (
	user32                            = syscall.NewLazyDLL("user32.dll")
	gdi32                             = syscall.NewLazyDLL("gdi32.dll")
	procSetProcessDPIAware            = user32.NewProc("SetProcessDPIAware")
	procSetProcessDPIAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procGetDC                         = user32.NewProc("GetDC")
	procReleaseDC                     = user32.NewProc("ReleaseDC")
	procGetAsyncKeyState              = user32.NewProc("GetAsyncKeyState")
	procCreateCompatibleDC            = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC                      = gdi32.NewProc("DeleteDC")
	procCreateDIBSection              = gdi32.NewProc("CreateDIBSection")
	procSelectObject                  = gdi32.NewProc("SelectObject")
	procDeleteObject                  = gdi32.NewProc("DeleteObject")
	procBitBlt                        = gdi32.NewProc("BitBlt")
)

const dpiAwarenessContextPerMonitorAwareV2 = ^uintptr(3) // DPI_AWARENESS_CONTEXT(-4)

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

func Supported() bool { return true }

// Initialize configures the process before any screen coordinates are queried.
// Windows reports physical pixels to per-monitor-aware processes, keeping the
// selector and GDI capture coordinates in the same coordinate space.
func Initialize() error {
	if err := procSetProcessDPIAwarenessContext.Find(); err == nil {
		ok, _, callErr := procSetProcessDPIAwarenessContext.Call(dpiAwarenessContextPerMonitorAwareV2)
		if ok != 0 || isAlreadyDPIAware(callErr) {
			return nil
		}
	}
	ok, _, callErr := procSetProcessDPIAware.Call()
	if ok != 0 || isAlreadyDPIAware(callErr) {
		return nil
	}
	return win32CallError("SetProcessDPIAware", callErr, "initializing capture")
}

func isAlreadyDPIAware(err error) bool {
	errno, ok := err.(syscall.Errno)
	return ok && errno == syscall.Errno(5) // ERROR_ACCESS_DENIED: awareness was already set.
}

func EscapePressed() bool {
	state, _, _ := procGetAsyncKeyState.Call(vkEscape)
	return state&0x8000 != 0
}

// Region captures a rectangle in virtual desktop coordinates using Win32 GDI.
func Region(x, y, width, height int) (*image.RGBA, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("capture dimensions must be positive")
	}
	details := fmt.Sprintf("region=(%d,%d %dx%d)", x, y, width, height)
	screenDC, _, callErr := procGetDC.Call(0)
	if screenDC == 0 {
		return nil, win32CallError("GetDC", callErr, details)
	}
	defer procReleaseDC.Call(0, screenDC)

	memoryDC, _, callErr := procCreateCompatibleDC.Call(screenDC)
	if memoryDC == 0 {
		return nil, win32CallError("CreateCompatibleDC", callErr, details)
	}
	defer procDeleteDC.Call(memoryDC)

	info := bitmapInfo{Header: bitmapInfoHeader{
		Size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		Width:       int32(width),
		Height:      -int32(height), // top-down rows
		Planes:      1,
		BitCount:    32,
		Compression: biRGB,
	}}
	var bits unsafe.Pointer
	bitmap, _, callErr := procCreateDIBSection.Call(
		screenDC,
		uintptr(unsafe.Pointer(&info)),
		dibRGBColors,
		uintptr(unsafe.Pointer(&bits)),
		0,
		0,
	)
	if bitmap == 0 || bits == nil {
		return nil, win32CallError("CreateDIBSection", callErr, details)
	}
	defer procDeleteObject.Call(bitmap)

	oldObject, _, callErr := procSelectObject.Call(memoryDC, bitmap)
	if oldObject == 0 || oldObject == ^uintptr(0) {
		return nil, win32CallError("SelectObject", callErr, details)
	}
	defer procSelectObject.Call(memoryDC, oldObject)

	ok, _, callErr := procBitBlt.Call(memoryDC, 0, 0, uintptr(width), uintptr(height), screenDC, uintptr(x), uintptr(y), srcCopyCaptureBLT)
	if ok == 0 {
		return nil, win32CallError("BitBlt", callErr, details)
	}

	// A 32-bit BI_RGB DIB uses BGRA byte order. Negative Height above makes the
	// buffer top-down, so no row reversal is necessary.
	bgra := unsafe.Slice((*byte)(bits), width*height*4)

	result := image.NewRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < width*height; i++ {
		result.Pix[i*4] = bgra[i*4+2]
		result.Pix[i*4+1] = bgra[i*4+1]
		result.Pix[i*4+2] = bgra[i*4]
		result.Pix[i*4+3] = 0xff
	}
	// Keep the Go-owned BITMAPINFO alive until the Win32 calls using it finish.
	runtime.KeepAlive(info)
	return result, nil
}

func win32CallError(operation string, callErr error, details string) error {
	if errno, ok := callErr.(syscall.Errno); ok && errno == 0 {
		return fmt.Errorf("%s failed (%s; GetLastError was not set)", operation, details)
	}
	return fmt.Errorf("%s failed (%s; GetLastError=%v)", operation, details, callErr)
}
