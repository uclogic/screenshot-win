//go:build windows

package main

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	application "screenshot-win/app"
)

const (
	trayCallbackMessage = 0x8001
	trayExitReady       = 0x8002
	trayCommandCapture  = 1001
	trayCommandExit     = 1002
	trayCaptureHotkeyID = 1

	wmDestroy     = 0x0002
	wmCommand     = 0x0111
	wmHotkey      = 0x0312
	wmLButtonUp   = 0x0202
	wmRButtonUp   = 0x0205
	wmContextMenu = 0x007B
	ninSelect     = 0x0400
	ninKeySelect  = 0x0401

	nimAdd        = 0x00000000
	nimDelete     = 0x00000002
	nimSetVersion = 0x00000004
	nifMessage    = 0x00000001
	nifIcon       = 0x00000002
	nifTip        = 0x00000004
	notifyVersion = 4

	mfString       = 0x00000000
	mfSeparator    = 0x00000800
	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100

	idiApplication     = 32512
	errorAlreadyExists = 183
	modAlt             = 0x0001
	modShift           = 0x0004
	modNoRepeat        = 0x4000
	vkA                = 0x41
)

var (
	trayUser32                    = syscall.NewLazyDLL("user32.dll")
	trayShell32                   = syscall.NewLazyDLL("shell32.dll")
	trayKernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procTrayRegisterClassEx       = trayUser32.NewProc("RegisterClassExW")
	procTrayUnregisterClass       = trayUser32.NewProc("UnregisterClassW")
	procTrayCreateWindowEx        = trayUser32.NewProc("CreateWindowExW")
	procTrayDefWindowProc         = trayUser32.NewProc("DefWindowProcW")
	procTrayDestroyWindow         = trayUser32.NewProc("DestroyWindow")
	procTrayGetMessage            = trayUser32.NewProc("GetMessageW")
	procTrayTranslateMessage      = trayUser32.NewProc("TranslateMessage")
	procTrayDispatchMessage       = trayUser32.NewProc("DispatchMessageW")
	procTrayPostQuitMessage       = trayUser32.NewProc("PostQuitMessage")
	procTrayPostMessage           = trayUser32.NewProc("PostMessageW")
	procTrayRegisterHotKey        = trayUser32.NewProc("RegisterHotKey")
	procTrayUnregisterHotKey      = trayUser32.NewProc("UnregisterHotKey")
	procTrayRegisterWindowMessage = trayUser32.NewProc("RegisterWindowMessageW")
	procTrayLoadIcon              = trayUser32.NewProc("LoadIconW")
	procTrayCreatePopupMenu       = trayUser32.NewProc("CreatePopupMenu")
	procTrayAppendMenu            = trayUser32.NewProc("AppendMenuW")
	procTrayTrackPopupMenu        = trayUser32.NewProc("TrackPopupMenu")
	procTrayDestroyMenu           = trayUser32.NewProc("DestroyMenu")
	procTraySetForegroundWindow   = trayUser32.NewProc("SetForegroundWindow")
	procTrayGetCursorPos          = trayUser32.NewProc("GetCursorPos")
	procShellNotifyIcon           = trayShell32.NewProc("Shell_NotifyIconW")
	procTrayGetModuleHandle       = trayKernel32.NewProc("GetModuleHandleW")
	procTrayCreateMutex           = trayKernel32.NewProc("CreateMutexW")
	procTrayCloseHandle           = trayKernel32.NewProc("CloseHandle")

	trayWindowProcedure = syscall.NewCallback(trayWndProc)
	trayHosts           sync.Map
)

type trayWindowClass struct {
	Size, Style             uint32
	WindowProcedure         uintptr
	ClassExtra, WindowExtra int32
	Instance, Icon, Cursor  uintptr
	Background              uintptr
	MenuName, ClassName     *uint16
	SmallIcon               uintptr
}

type trayPoint struct{ X, Y int32 }

type trayMessage struct {
	Window  uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   trayPoint
	Private uint32
}

type notifyIconData struct {
	Size            uint32
	Window          uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            uintptr
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUID            [16]byte
	BalloonIcon     uintptr
}

type windowsTrayHost struct {
	hwnd            uintptr
	instance        uintptr
	className       *uint16
	icon            notifyIconData
	iconAdded       bool
	taskbarCreated  uint32
	controller      *trayController
	shutdownStarted bool
}

func runTrayHost(runner *application.Runner, config application.Config) error {
	defer runner.ClosePinnedImages()
	mutexName, _ := syscall.UTF16PtrFromString(`Local\ScreenshotWin.TrayHost`)
	mutex, _, mutexErr := procTrayCreateMutex.Call(0, 0, uintptr(unsafe.Pointer(mutexName)))
	if mutex == 0 {
		return trayWin32Error("CreateMutexW", mutexErr)
	}
	defer procTrayCloseHandle.Call(mutex)
	if errno, ok := mutexErr.(syscall.Errno); ok && errno == errorAlreadyExists {
		showInformationMessage("screenshot-win 已在通知区域运行。")
		return nil
	}

	host := &windowsTrayHost{}
	host.controller = newTrayController(func(ctx context.Context) error {
		return runner.RunContext(ctx, config)
	}, func(err error) {
		showErrorMessage(host.hwnd, err)
	})
	return host.run()
}

func (host *windowsTrayHost) run() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	instance, _, callErr := procTrayGetModuleHandle.Call(0)
	if instance == 0 {
		return trayWin32Error("GetModuleHandleW", callErr)
	}
	host.instance = instance
	host.className, _ = syscall.UTF16PtrFromString("ScreenshotWinTrayHost")
	class := trayWindowClass{
		Size: uint32(unsafe.Sizeof(trayWindowClass{})), WindowProcedure: trayWindowProcedure,
		Instance: instance, ClassName: host.className,
	}
	atom, _, callErr := procTrayRegisterClassEx.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		return trayWin32Error("RegisterClassExW", callErr)
	}
	defer procTrayUnregisterClass.Call(uintptr(unsafe.Pointer(host.className)), instance)

	title, _ := syscall.UTF16PtrFromString("screenshot-win tray host")
	hwnd, _, callErr := procTrayCreateWindowEx.Call(
		0, uintptr(unsafe.Pointer(host.className)), uintptr(unsafe.Pointer(title)), 0,
		0, 0, 0, 0, 0, 0, instance, 0,
	)
	if hwnd == 0 {
		return trayWin32Error("CreateWindowExW", callErr)
	}
	host.hwnd = hwnd
	trayHosts.Store(hwnd, host)
	defer trayHosts.Delete(hwnd)
	defer func() {
		host.removeIcon()
		if host.hwnd != 0 {
			procTrayDestroyWindow.Call(host.hwnd)
		}
	}()
	registered, _, callErr := procTrayRegisterHotKey.Call(hwnd, trayCaptureHotkeyID, modAlt|modShift|modNoRepeat, vkA)
	if registered == 0 {
		return trayWin32Error("RegisterHotKey(Alt+Shift+A)", callErr)
	}
	defer procTrayUnregisterHotKey.Call(hwnd, trayCaptureHotkeyID)

	taskbarName, _ := syscall.UTF16PtrFromString("TaskbarCreated")
	message, _, callErr := procTrayRegisterWindowMessage.Call(uintptr(unsafe.Pointer(taskbarName)))
	if message == 0 {
		return trayWin32Error("RegisterWindowMessageW", callErr)
	}
	host.taskbarCreated = uint32(message)
	if err := host.addIcon(); err != nil {
		return err
	}

	var msg trayMessage
	for {
		status, _, callErr := procTrayGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(status) == -1 {
			return trayWin32Error("GetMessageW", callErr)
		}
		if status == 0 {
			break
		}
		procTrayTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procTrayDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
	return nil
}

func (host *windowsTrayHost) addIcon() error {
	icon, _, _ := procTrayLoadIcon.Call(host.instance, 1)
	if icon == 0 {
		icon, _, _ = procTrayLoadIcon.Call(0, idiApplication)
	}
	host.icon = notifyIconData{
		Size: uint32(unsafe.Sizeof(notifyIconData{})), Window: host.hwnd, ID: 1,
		Flags: nifMessage | nifIcon | nifTip, CallbackMessage: trayCallbackMessage, Icon: icon,
	}
	copy(host.icon.Tip[:], syscall.StringToUTF16("screenshot-win - Alt+Shift+A 开始截图"))
	ok, _, callErr := procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&host.icon)))
	if ok == 0 {
		return trayWin32Error("Shell_NotifyIconW(NIM_ADD)", callErr)
	}
	host.iconAdded = true
	host.icon.Version = notifyVersion
	procShellNotifyIcon.Call(nimSetVersion, uintptr(unsafe.Pointer(&host.icon)))
	return nil
}

func (host *windowsTrayHost) removeIcon() {
	if !host.iconAdded {
		return
	}
	procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&host.icon)))
	host.iconAdded = false
}

func (host *windowsTrayHost) restoreIcon() {
	host.iconAdded = false
	if err := host.addIcon(); err != nil {
		showErrorMessage(host.hwnd, err)
	}
}

func (host *windowsTrayHost) showMenu() {
	menu, _, _ := procTrayCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procTrayDestroyMenu.Call(menu)
	captureText, _ := syscall.UTF16PtrFromString("开始截图\tAlt+Shift+A")
	exitText, _ := syscall.UTF16PtrFromString("退出")
	procTrayAppendMenu.Call(menu, mfString, trayCommandCapture, uintptr(unsafe.Pointer(captureText)))
	procTrayAppendMenu.Call(menu, mfSeparator, 0, 0)
	procTrayAppendMenu.Call(menu, mfString, trayCommandExit, uintptr(unsafe.Pointer(exitText)))
	var cursor trayPoint
	procTrayGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor)))
	procTraySetForegroundWindow.Call(host.hwnd)
	command, _, _ := procTrayTrackPopupMenu.Call(
		menu, tpmRightButton|tpmReturnCmd, uintptr(cursor.X), uintptr(cursor.Y), 0, host.hwnd, 0,
	)
	procTrayPostMessage.Call(host.hwnd, 0, 0, 0) // WM_NULL lets repeated menus dismiss correctly.
	host.handleCommand(command)
}

func (host *windowsTrayHost) handleCommand(command uintptr) {
	switch command {
	case trayCommandCapture:
		host.controller.Trigger()
	case trayCommandExit:
		host.beginShutdown()
	}
}

func (host *windowsTrayHost) beginShutdown() {
	if host.shutdownStarted {
		return
	}
	host.shutdownStarted = true
	done := host.controller.Shutdown()
	go func() {
		<-done
		procTrayPostMessage.Call(host.hwnd, trayExitReady, 0, 0)
	}()
}

func trayWndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	value, found := trayHosts.Load(hwnd)
	if !found {
		result, _, _ := procTrayDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
		return result
	}
	host := value.(*windowsTrayHost)
	if message == host.taskbarCreated {
		host.restoreIcon()
		return 0
	}
	switch message {
	case trayCallbackMessage:
		event := uint32(lParam & 0xffff)
		switch event {
		case wmLButtonUp, ninSelect, ninKeySelect:
			host.controller.Trigger()
		case wmRButtonUp, wmContextMenu:
			host.showMenu()
		}
		return 0
	case wmCommand:
		host.handleCommand(wParam & 0xffff)
		return 0
	case wmHotkey:
		if wParam == trayCaptureHotkeyID {
			host.controller.Trigger()
		}
		return 0
	case trayExitReady:
		host.removeIcon()
		procTrayDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		host.hwnd = 0
		procTrayPostQuitMessage.Call(0)
		return 0
	}
	result, _, _ := procTrayDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}

func showInformationMessage(text string) {
	title, _ := syscall.UTF16PtrFromString("screenshot-win")
	message, _ := syscall.UTF16PtrFromString(text)
	const mbIconInformation = 0x00000040
	procMessageBox.Call(0, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(title)), mbOK|mbIconInformation)
}

func trayWin32Error(operation string, callErr error) error {
	if errno, ok := callErr.(syscall.Errno); ok && errno == 0 {
		return fmt.Errorf("%s failed (GetLastError was not set)", operation)
	}
	return fmt.Errorf("%s failed (GetLastError=%v)", operation, callErr)
}
