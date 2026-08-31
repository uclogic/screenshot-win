//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

const attachParentProcess = uintptr(0xFFFFFFFF)

var (
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	attachConsole         = kernel32.NewProc("AttachConsole")
	freeConsole           = kernel32.NewProc("FreeConsole")
	getConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
)

// attachParentConsole keeps the Windows GUI executable quiet when it is opened
// from Explorer, while restoring stdout and stderr when a terminal launched it.
func attachParentConsole() {
	attached, _, attachErr := attachConsole.Call(attachParentProcess)
	if attached == 0 {
		// A console-subsystem development build is already attached. For the
		// windowsgui release, every other failure means there is no parent
		// console. A console containing only this process was allocated because
		// a console build was opened from Explorer; detach it as a fallback.
		if attachErr == syscall.ERROR_ACCESS_DENIED {
			if consoleIsShared() {
				return
			}
			freeConsole.Call()
		}
		redirectOutput("NUL")
		return
	}

	redirectOutput("CONOUT$")
}

func consoleIsShared() bool {
	var processID uint32
	count, _, _ := getConsoleProcessList.Call(
		uintptr(unsafe.Pointer(&processID)),
		1,
	)
	return count > 1
}

func redirectOutput(device string) {
	stdout, stdoutErr := os.OpenFile(device, os.O_WRONLY, 0)
	if stdoutErr == nil {
		os.Stdout = stdout
	}
	stderr, stderrErr := os.OpenFile(device, os.O_WRONLY, 0)
	if stderrErr == nil {
		os.Stderr = stderr
	}
}
