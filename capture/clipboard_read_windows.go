//go:build windows

package capture

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	procGetClipboardData           = user32.NewProc("GetClipboardData")
	procIsClipboardFormatAvailable = user32.NewProc("IsClipboardFormatAvailable")
	procRegisterClipboardFormat    = user32.NewProc("RegisterClipboardFormatW")
	procGlobalSize                 = clipboardKernel32.NewProc("GlobalSize")
)

// ReadClipboard reads images first, then Unicode text without changing the clipboard.
func ReadClipboard() (ClipboardContent, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := openClipboardWithRetry(); err != nil {
		return ClipboardContent{}, err
	}
	images, textData, readErr := snapshotClipboard()
	procCloseClipboard.Call()
	return decodeClipboardSnapshot(images, textData, readErr)
}

func snapshotClipboard() ([]clipboardImageData, []byte, error) {
	pngName, _ := syscall.UTF16PtrFromString("PNG")
	pngFormat, _, _ := procRegisterClipboardFormat.Call(uintptr(unsafe.Pointer(pngName)))
	var images []clipboardImageData
	var textData []byte
	var lastErr error
	remaining := uintptr(maximumClipboardBytes)
	for _, format := range []uintptr{pngFormat, 17, 8, 13} {
		if format == 0 {
			continue
		}
		available, _, _ := procIsClipboardFormatAvailable.Call(format)
		if available == 0 {
			continue
		}
		limit := remaining
		if format == 13 && limit > 8<<20 {
			limit = 8 << 20
		}
		data, err := readClipboardBytes(format, limit)
		if err != nil {
			lastErr = err
			continue
		}
		remaining -= uintptr(len(data))
		if format == 13 {
			textData = data
		} else {
			images = append(images, clipboardImageData{png: format == pngFormat, data: data})
		}
	}
	return images, textData, lastErr
}

func readClipboardBytes(format, limit uintptr) ([]byte, error) {
	handle, _, err := procGetClipboardData.Call(format)
	if handle == 0 {
		return nil, win32CallError("GetClipboardData", err, "reading clipboard")
	}
	size, _, _ := procGlobalSize.Call(handle)
	if size == 0 || size > limit {
		return nil, fmt.Errorf("clipboard data is empty or exceeds the snapshot size limit")
	}
	pointer, _, err := procGlobalLock.Call(handle)
	if pointer == 0 {
		return nil, win32CallError("GlobalLock", err, "reading clipboard")
	}
	defer procGlobalUnlock.Call(handle)
	return append([]byte(nil), unsafe.Slice((*byte)(unsafe.Pointer(pointer)), int(size))...), nil
}
