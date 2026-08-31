//go:build windows

package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

const (
	ofnOverwritePrompt = 0x00000002
	ofnPathMustExist   = 0x00000800
	ofnNoChangeDir     = 0x00000008
	ofnEnableHook      = 0x00000020
	ofnExplorer        = 0x00080000
	mbOK               = 0x00000000
	mbIconError        = 0x00000010
)

var (
	comdlg32                 = syscall.NewLazyDLL("comdlg32.dll")
	mainUser32               = syscall.NewLazyDLL("user32.dll")
	procGetSaveFileName      = comdlg32.NewProc("GetSaveFileNameW")
	procCommDlgExtendedError = comdlg32.NewProc("CommDlgExtendedError")
	procMessageBox           = mainUser32.NewProc("MessageBoxW")
	procDialogPostMessage    = mainUser32.NewProc("PostMessageW")
	procDialogGetParent      = mainUser32.NewProc("GetParent")

	saveDialogHook      = syscall.NewCallback(saveDialogHookProcedure)
	saveDialogMu        sync.Mutex
	activeSaveDialog    atomic.Uintptr
	saveDialogCancelled atomic.Bool
)

type openFileName struct {
	Size            uint32
	Owner           uintptr
	Instance        uintptr
	Filter          *uint16
	CustomFilter    *uint16
	MaxCustomFilter uint32
	FilterIndex     uint32
	File            *uint16
	MaxFile         uint32
	FileTitle       *uint16
	MaxFileTitle    uint32
	InitialDir      *uint16
	Title           *uint16
	Flags           uint32
	FileOffset      uint16
	FileExtension   uint16
	DefaultExt      *uint16
	CustomData      uintptr
	Hook            uintptr
	TemplateName    *uint16
	Reserved        unsafe.Pointer
	ReservedFlags   uint32
	FlagsEx         uint32
}

func choosePNGPath(owner uintptr, now time.Time) (string, bool, error) {
	return choosePNGPathContext(context.Background(), owner, now)
}

func choosePNGPathContext(ctx context.Context, owner uintptr, now time.Time) (string, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	saveDialogMu.Lock()
	defer saveDialogMu.Unlock()
	activeSaveDialog.Store(0)
	saveDialogCancelled.Store(false)
	stopCancellation := context.AfterFunc(ctx, func() {
		saveDialogCancelled.Store(true)
		if dialog := activeSaveDialog.Load(); dialog != 0 {
			procDialogPostMessage.Call(dialog, 0x0111, 2, 0) // WM_COMMAND, IDCANCEL
		}
	})
	defer stopCancellation()
	defer activeSaveDialog.Store(0)

	fileBuffer := make([]uint16, 32768)
	copy(fileBuffer, syscall.StringToUTF16(suggestedScreenshotName(now)))
	language := uiLanguage()
	filter := utf16.Encode([]rune(localize(language, textPNGFilter)))
	title, _ := syscall.UTF16PtrFromString(localize(language, textSaveScreenshot))
	defaultExt, _ := syscall.UTF16PtrFromString("png")
	dialog := openFileName{
		Size: uint32(unsafe.Sizeof(openFileName{})), Owner: owner, Filter: &filter[0], FilterIndex: 1,
		File: &fileBuffer[0], MaxFile: uint32(len(fileBuffer)), Title: title,
		Flags:      ofnOverwritePrompt | ofnPathMustExist | ofnNoChangeDir | ofnEnableHook | ofnExplorer,
		DefaultExt: defaultExt, Hook: saveDialogHook,
	}
	ok, _, _ := procGetSaveFileName.Call(uintptr(unsafe.Pointer(&dialog)))
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if ok == 0 {
		code, _, _ := procCommDlgExtendedError.Call()
		if code == 0 {
			return "", false, nil
		}
		return "", false, fmt.Errorf("GetSaveFileNameW failed (CommDlgExtendedError=0x%X)", code)
	}
	path := syscall.UTF16ToString(fileBuffer)
	if !strings.EqualFold(filepath.Ext(path), ".png") {
		path += ".png"
	}
	return path, true, nil
}

func saveDialogHookProcedure(hwnd uintptr, message uint32, _, _ uintptr) uintptr {
	if message != 0x0110 { // WM_INITDIALOG
		return 0
	}
	dialog, _, _ := procDialogGetParent.Call(hwnd)
	if dialog == 0 {
		dialog = hwnd
	}
	activeSaveDialog.Store(dialog)
	if saveDialogCancelled.Load() {
		procDialogPostMessage.Call(dialog, 0x0111, 2, 0) // WM_COMMAND, IDCANCEL
	}
	return 0
}

func suggestedScreenshotName(now time.Time) string {
	return "Screenshot_" + now.Format("20060102_150405") + ".png"
}

func showErrorMessage(owner uintptr, err error) {
	if err == nil {
		return
	}
	title, _ := syscall.UTF16PtrFromString("screenshot-win")
	message, conversionErr := syscall.UTF16PtrFromString(err.Error())
	if conversionErr != nil {
		return
	}
	procMessageBox.Call(owner, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(title)), mbOK|mbIconError)
}
