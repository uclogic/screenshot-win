//go:build windows

package capture

import (
	"fmt"
	"image"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

const (
	cfDIB        = 8
	gmemMoveable = 0x0002
)

var (
	clipboardKernel32    = syscall.NewLazyDLL("kernel32.dll")
	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procSetClipboardData = user32.NewProc("SetClipboardData")
	procGlobalAlloc      = clipboardKernel32.NewProc("GlobalAlloc")
	procGlobalLock       = clipboardKernel32.NewProc("GlobalLock")
	procGlobalUnlock     = clipboardKernel32.NewProc("GlobalUnlock")
	procGlobalFree       = clipboardKernel32.NewProc("GlobalFree")
)

// CopyImage writes source to the Windows clipboard as CF_DIB.
func CopyImage(source image.Image) error {
	size, err := dibSize(source)
	if err != nil {
		return err
	}
	handle, _, callErr := procGlobalAlloc.Call(gmemMoveable, uintptr(size))
	if handle == 0 {
		return win32CallError("GlobalAlloc", callErr, fmt.Sprintf("allocating %d clipboard bytes", size))
	}
	owned := true
	defer func() {
		if owned {
			procGlobalFree.Call(handle)
		}
	}()
	memory, _, callErr := procGlobalLock.Call(handle)
	if memory == 0 {
		return win32CallError("GlobalLock", callErr, "copying selected image")
	}
	destination := unsafe.Slice((*byte)(unsafe.Pointer(memory)), size)
	if err := writeDIB(destination, source); err != nil {
		procGlobalUnlock.Call(handle)
		return err
	}
	procGlobalUnlock.Call(handle)

	if err := openClipboardWithRetry(); err != nil {
		return err
	}
	defer procCloseClipboard.Call()
	ok, _, callErr := procEmptyClipboard.Call()
	if ok == 0 {
		return win32CallError("EmptyClipboard", callErr, "copying selected image")
	}
	stored, _, callErr := procSetClipboardData.Call(cfDIB, handle)
	if stored == 0 {
		return win32CallError("SetClipboardData", callErr, "copying selected image")
	}
	owned = false
	runtime.KeepAlive(source)
	return nil
}

func openClipboardWithRetry() error {
	var lastErr error
	for attempt := 0; attempt < 6; attempt++ {
		ok, _, callErr := procOpenClipboard.Call(0)
		if ok != 0 {
			return nil
		}
		lastErr = callErr
		time.Sleep(15 * time.Millisecond)
	}
	return win32CallError("OpenClipboard", lastErr, "copying selected image")
}
