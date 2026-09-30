//go:build windows

package selector

import (
	"image"
	"syscall"
	"unsafe"
)

var (
	procCaptureMonitorFromPoint = user32.NewProc("MonitorFromPoint")
	procCaptureGetDpiForMonitor = syscall.NewLazyDLL("shcore.dll").NewProc("GetDpiForMonitor")
)

func captureDPIAt(point image.Point, fallback int) int {
	if procCaptureGetDpiForMonitor.Find() != nil {
		return max(96, fallback)
	}
	packed := uintptr(uint32(point.X)) | uintptr(uint64(uint32(point.Y))<<32)
	monitor, _, _ := procCaptureMonitorFromPoint.Call(packed, monitorDefaultToNearest)
	if monitor == 0 {
		return max(96, fallback)
	}
	var horizontal, vertical uint32
	result, _, _ := procCaptureGetDpiForMonitor.Call(monitor, 0, uintptr(unsafe.Pointer(&horizontal)), uintptr(unsafe.Pointer(&vertical)))
	if result != 0 || horizontal == 0 {
		return max(96, fallback)
	}
	return max(96, int(horizontal))
}
