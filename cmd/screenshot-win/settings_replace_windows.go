//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	settingsKernel32       = syscall.NewLazyDLL("kernel32.dll")
	procMoveSettingsFileEx = settingsKernel32.NewProc("MoveFileExW")
)

const (
	moveFileReplaceExisting = 0x00000001
	moveFileWriteThrough    = 0x00000008
)

func replaceSettingsFile(source, destination string) error {
	sourceUTF16, err := syscall.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	destinationUTF16, err := syscall.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	ok, _, callErr := procMoveSettingsFileEx.Call(
		uintptr(unsafe.Pointer(sourceUTF16)), uintptr(unsafe.Pointer(destinationUTF16)),
		moveFileReplaceExisting|moveFileWriteThrough,
	)
	if ok == 0 {
		return callErr
	}
	return nil
}
