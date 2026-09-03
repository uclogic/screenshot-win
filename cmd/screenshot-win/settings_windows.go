//go:build windows

package main

import (
	"fmt"
	"math"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const (
	settingsClassName = "ScreenshotWinSettings"

	settingsIDApply           = 3
	settingsIDBrowse          = 4
	settingsIDTree            = 2001
	settingsIDHotkey          = 2002
	settingsIDLanguage        = 2003
	settingsIDLongCaptureMode = 2100
	settingsIDInterval        = 2101
	settingsIDMaxScroll       = 2102
	settingsIDMaxDifference   = 2103
	settingsIDMinConfidence   = 2104
	settingsIDStationary      = 2105
	settingsIDDiagnostics     = 2201
	settingsIDDiagnosticDir   = 2202
	settingsIDDiagnosticLimit = 2203

	settingsWMCreate     = 0x0001
	settingsWMDestroy    = 0x0002
	settingsWMClose      = 0x0010
	settingsWMCommand    = 0x0111
	settingsWMNotify     = 0x004E
	settingsWMNCCreate   = 0x0081
	settingsWMDPIChanged = 0x02E0

	settingsWSChild           = 0x40000000
	settingsWSVisible         = 0x10000000
	settingsWSTabStop         = 0x00010000
	settingsWSBorder          = 0x00800000
	settingsWSCaption         = 0x00C00000
	settingsWSSysMenu         = 0x00080000
	settingsWSMinimizeBox     = 0x00020000
	settingsWSExControlParent = 0x00010000

	settingsBSDefaultPushButton = 0x00000001
	settingsBSAutoCheckBox      = 0x00000003
	settingsBSGroupBox          = 0x00000007
	settingsCBSDropDownList     = 0x00000003
	settingsESAutoHScroll       = 0x00000080
	settingsESNumber            = 0x00002000

	settingsTVSHasLines      = 0x0002
	settingsTVSLinesAtRoot   = 0x0004
	settingsTVSShowSelAlways = 0x0020
	settingsTVMInsertItemW   = 0x1132
	settingsTVMGetNextItem   = 0x110A
	settingsTVMSelectItem    = 0x110B
	settingsTVGNCaret        = 0x0009
	settingsTVIFText         = 0x0001
	settingsTVIFParam        = 0x0004

	settingsHKMSetHotkey = 0x0401
	settingsHKMGetHotkey = 0x0402
	settingsHKMSetRules  = 0x0403
	settingsHotkeyFShift = 0x01
	settingsHotkeyFCtrl  = 0x02
	settingsHotkeyFAlt   = 0x04

	settingsBMGetCheck         = 0x00F0
	settingsBMSetCheck         = 0x00F1
	settingsBSTChecked         = 1
	settingsENChange           = 0x0300
	settingsBNClicked          = 0
	settingsCBNSelectionChange = 1
	settingsCBAddString        = 0x0143
	settingsCBGetCurrent       = 0x0147
	settingsCBSetCurrent       = 0x014E

	settingsSWHide         = 0
	settingsSWShow         = 5
	settingsSWRestore      = 9
	settingsWMSetFont      = 0x0030
	settingsDefaultGUIFont = 17

	settingsICCWin95Classes     = 0x000000FF
	settingsBIFReturnOnlyFSDirs = 0x00000001
	settingsBIFNewDialogStyle   = 0x00000040
)

var (
	settingsUser32                = syscall.NewLazyDLL("user32.dll")
	settingsGDI32                 = syscall.NewLazyDLL("gdi32.dll")
	settingsComctl32              = syscall.NewLazyDLL("comctl32.dll")
	settingsShell32               = syscall.NewLazyDLL("shell32.dll")
	settingsOle32                 = syscall.NewLazyDLL("ole32.dll")
	procSettingsCreateWindowEx    = settingsUser32.NewProc("CreateWindowExW")
	procSettingsDefWindowProc     = settingsUser32.NewProc("DefWindowProcW")
	procSettingsDestroyWindow     = settingsUser32.NewProc("DestroyWindow")
	procSettingsEnableWindow      = settingsUser32.NewProc("EnableWindow")
	procSettingsGetDPIForWindow   = settingsUser32.NewProc("GetDpiForWindow")
	procSettingsGetWindowText     = settingsUser32.NewProc("GetWindowTextW")
	procSettingsGetWindowTextLen  = settingsUser32.NewProc("GetWindowTextLengthW")
	procSettingsLoadCursor        = settingsUser32.NewProc("LoadCursorW")
	procSettingsRegisterClassEx   = settingsUser32.NewProc("RegisterClassExW")
	procSettingsSendMessage       = settingsUser32.NewProc("SendMessageW")
	procSettingsSetFocus          = settingsUser32.NewProc("SetFocus")
	procSettingsSetForeground     = settingsUser32.NewProc("SetForegroundWindow")
	procSettingsSetWindowPos      = settingsUser32.NewProc("SetWindowPos")
	procSettingsSetWindowText     = settingsUser32.NewProc("SetWindowTextW")
	procSettingsShowWindow        = settingsUser32.NewProc("ShowWindow")
	procSettingsGetStockObject    = settingsGDI32.NewProc("GetStockObject")
	procSettingsInitControls      = settingsComctl32.NewProc("InitCommonControlsEx")
	procSettingsBrowseForFolder   = settingsShell32.NewProc("SHBrowseForFolderW")
	procSettingsGetPathFromIDList = settingsShell32.NewProc("SHGetPathFromIDListW")
	procSettingsCoInitializeEx    = settingsOle32.NewProc("CoInitializeEx")
	procSettingsCoUninitialize    = settingsOle32.NewProc("CoUninitialize")
	procSettingsCoTaskMemFree     = settingsOle32.NewProc("CoTaskMemFree")

	settingsWindowProcedure = syscall.NewCallback(settingsWndProc)
	settingsWindows         sync.Map
	pendingSettingsWindow   atomic.Pointer[settingsWindow]
)

type settingsWindow struct {
	host                      *windowsTrayHost
	hwnd                      uintptr
	dpi                       int
	loading                   bool
	dirty                     bool
	controls                  map[int]uintptr
	general                   []uintptr
	advanced                  []uintptr
	generalItem, advancedItem uintptr
}

type settingsWindowClass struct {
	Size, Style             uint32
	WindowProcedure         uintptr
	ClassExtra, WindowExtra int32
	Instance, Icon, Cursor  uintptr
	Background              uintptr
	MenuName, ClassName     *uint16
	SmallIcon               uintptr
}

type settingsInitCommonControls struct {
	Size uint32
	ICC  uint32
}

type settingsTreeItem struct {
	Mask, State, StateMask uint32
	Item                   uintptr
	Text                   *uint16
	TextMax                int32
	Image, SelectedImage   int32
	Children               int32
	Param                  uintptr
}

type settingsTreeInsert struct {
	Parent, InsertAfter uintptr
	Item                settingsTreeItem
}

type settingsNMHDR struct {
	Window uintptr
	ID     uintptr
	Code   uint32
}

type settingsRect struct{ Left, Top, Right, Bottom int32 }

type settingsBrowseInfo struct {
	Owner       uintptr
	Root        uintptr
	DisplayName *uint16
	Title       *uint16
	Flags       uint32
	Callback    uintptr
	Param       uintptr
	Image       int32
}

func (host *windowsTrayHost) openSettings() {
	if host.settings != nil && host.settings.hwnd != 0 {
		procSettingsShowWindow.Call(host.settings.hwnd, settingsSWRestore)
		procSettingsSetForeground.Call(host.settings.hwnd)
		return
	}
	if err := host.ensureSettingsClass(); err != nil {
		showErrorMessage(host.hwnd, err)
		return
	}
	controls := settingsInitCommonControls{Size: uint32(unsafe.Sizeof(settingsInitCommonControls{})), ICC: settingsICCWin95Classes}
	if ok, _, callErr := procSettingsInitControls.Call(uintptr(unsafe.Pointer(&controls))); ok == 0 {
		showErrorMessage(host.hwnd, trayWin32Error("InitCommonControlsEx", callErr))
		return
	}
	state := &settingsWindow{host: host, dpi: 96, controls: make(map[int]uintptr)}
	pendingSettingsWindow.Store(state)
	defer pendingSettingsWindow.Store(nil)
	className, _ := syscall.UTF16PtrFromString(settingsClassName)
	title, _ := syscall.UTF16PtrFromString("screenshot-win - " + localize(host.preferences.General.Language, textSettings))
	hwnd, _, callErr := procSettingsCreateWindowEx.Call(
		settingsWSExControlParent, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)),
		settingsWSCaption|settingsWSSysMenu|settingsWSMinimizeBox,
		0x80000000, 0x80000000, 680, 487, host.hwnd, 0, host.instance, 0,
	)
	runtime.KeepAlive(state)
	if hwnd == 0 {
		showErrorMessage(host.hwnd, trayWin32Error("CreateWindowExW settings", callErr))
		return
	}
	state.hwnd = hwnd
	host.settings = state
	procSettingsShowWindow.Call(hwnd, settingsSWShow)
	procSettingsSetForeground.Call(hwnd)
}

func (host *windowsTrayHost) ensureSettingsClass() error {
	if host.settingsClassRegistered {
		return nil
	}
	name, _ := syscall.UTF16PtrFromString(settingsClassName)
	cursor, _, _ := procSettingsLoadCursor.Call(0, 32512)
	class := settingsWindowClass{
		Size: uint32(unsafe.Sizeof(settingsWindowClass{})), WindowProcedure: settingsWindowProcedure,
		Instance: host.instance, Cursor: cursor, Background: 16, ClassName: name,
	}
	atom, _, callErr := procSettingsRegisterClassEx.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		return trayWin32Error("RegisterClassExW settings", callErr)
	}
	host.settingsClassRegistered = true
	return nil
}

func settingsWndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	if message == settingsWMNCCreate {
		if state := pendingSettingsWindow.Load(); state != nil {
			state.hwnd = hwnd
			settingsWindows.Store(hwnd, state)
		}
	}
	value, found := settingsWindows.Load(hwnd)
	if !found {
		result, _, _ := procSettingsDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
		return result
	}
	state := value.(*settingsWindow)
	switch message {
	case settingsWMCreate:
		if err := state.createControls(); err != nil {
			showErrorMessage(hwnd, err)
			procSettingsDestroyWindow.Call(hwnd)
			return ^uintptr(0)
		}
		return 0
	case settingsWMCommand:
		state.handleCommand(int(wParam&0xffff), uint32((wParam>>16)&0xffff))
		return 0
	case settingsWMNotify:
		header := (*settingsNMHDR)(settingsPointer(lParam))
		if header.ID == settingsIDTree && int32(header.Code) == -451 { // TVN_SELCHANGEDW
			state.updateSelectedPage()
		}
		return 0
	case settingsWMDPIChanged:
		state.dpi = int(wParam & 0xffff)
		rect := (*settingsRect)(settingsPointer(lParam))
		procSettingsSetWindowPos.Call(hwnd, 0, uintptr(rect.Left), uintptr(rect.Top), uintptr(rect.Right-rect.Left), uintptr(rect.Bottom-rect.Top), 0x0014)
		state.layout()
		return 0
	case settingsWMClose:
		procSettingsDestroyWindow.Call(hwnd)
		return 0
	case settingsWMDestroy:
		settingsWindows.Delete(hwnd)
		if state.host.settings == state {
			state.host.settings = nil
		}
		state.hwnd = 0
		return 0
	}
	result, _, _ := procSettingsDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}

func settingsPointer(value uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&value))
}

func (state *settingsWindow) createControls() error {
	language := state.host.preferences.General.Language
	if procSettingsGetDPIForWindow.Find() == nil {
		if dpi, _, _ := procSettingsGetDPIForWindow.Call(state.hwnd); dpi != 0 {
			state.dpi = int(dpi)
		}
	}
	if state.dpi != 96 {
		width := uintptr((680*state.dpi + 48) / 96)
		height := uintptr((487*state.dpi + 48) / 96)
		procSettingsSetWindowPos.Call(state.hwnd, 0, 0, 0, width, height, 0x0016)
	}
	font, _, _ := procSettingsGetStockObject.Call(settingsDefaultGUIFont)
	create := func(id int, class, text string, style uint32) (uintptr, error) {
		classUTF16, _ := syscall.UTF16PtrFromString(class)
		textUTF16, _ := syscall.UTF16PtrFromString(text)
		hwnd, _, callErr := procSettingsCreateWindowEx.Call(
			0, uintptr(unsafe.Pointer(classUTF16)), uintptr(unsafe.Pointer(textUTF16)),
			uintptr(settingsWSChild|settingsWSVisible|style), 0, 0, 0, 0,
			state.hwnd, uintptr(id), state.host.instance, 0,
		)
		if hwnd == 0 {
			return 0, trayWin32Error("CreateWindowExW "+class, callErr)
		}
		state.controls[id] = hwnd
		procSettingsSendMessage.Call(hwnd, settingsWMSetFont, font, 1)
		return hwnd, nil
	}
	must := func(id int, class, text string, style uint32) uintptr {
		hwnd, err := create(id, class, text, style)
		if err != nil {
			panic(err)
		}
		return hwnd
	}
	var createErr error
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				createErr = recovered.(error)
			}
		}()
		tree := must(settingsIDTree, "SysTreeView32", "", settingsWSTabStop|settingsWSBorder|settingsTVSHasLines|settingsTVSLinesAtRoot|settingsTVSShowSelAlways)
		state.generalItem = state.insertTreeItem(tree, localize(language, textGeneral), 1)
		state.advancedItem = state.insertTreeItem(tree, localize(language, textAdvanced), 2)

		generalGroup := must(2301, "BUTTON", localize(language, textKeyboardShortcut), settingsBSGroupBox)
		generalLabel := must(2302, "STATIC", localize(language, textStartCaptureLabel), 0)
		hotkey := must(settingsIDHotkey, "msctls_hotkey32", "", settingsWSTabStop|settingsWSBorder)
		generalHelp := must(2303, "STATIC", localize(language, textHotkeyHelp), 0)
		languageLabel := must(2304, "STATIC", localize(language, textLanguageLabel), 0)
		languageCombo := must(settingsIDLanguage, "COMBOBOX", "", settingsWSTabStop|settingsCBSDropDownList)
		for _, option := range availableLanguages {
			state.addComboString(languageCombo, option.Name)
		}
		state.general = []uintptr{generalGroup, generalLabel, hotkey, generalHelp, languageLabel, languageCombo}

		captureGroup := must(2401, "BUTTON", localize(language, textScrollingMatching), settingsBSGroupBox)
		modeLabel := must(2407, "STATIC", localize(language, textLongCaptureMode), 0)
		mode := must(settingsIDLongCaptureMode, "COMBOBOX", "", settingsWSTabStop|settingsCBSDropDownList)
		state.addComboString(mode, localize(language, textLongCaptureBidirectional))
		state.addComboString(mode, localize(language, textLongCaptureLegacy))
		intervalLabel := must(2402, "STATIC", localize(language, textCaptureInterval), 0)
		interval := must(settingsIDInterval, "EDIT", "", settingsWSTabStop|settingsWSBorder|settingsESAutoHScroll|settingsESNumber)
		maxScrollLabel := must(2403, "STATIC", localize(language, textMaxScrollRatio), 0)
		maxScroll := must(settingsIDMaxScroll, "EDIT", "", settingsWSTabStop|settingsWSBorder|settingsESAutoHScroll)
		maxDiffLabel := must(2404, "STATIC", localize(language, textMaxMeanDifference), 0)
		maxDiff := must(settingsIDMaxDifference, "EDIT", "", settingsWSTabStop|settingsWSBorder|settingsESAutoHScroll)
		confidenceLabel := must(2405, "STATIC", localize(language, textMinConfidence), 0)
		confidence := must(settingsIDMinConfidence, "EDIT", "", settingsWSTabStop|settingsWSBorder|settingsESAutoHScroll)
		stationaryLabel := must(2406, "STATIC", localize(language, textStationaryThreshold), 0)
		stationary := must(settingsIDStationary, "EDIT", "", settingsWSTabStop|settingsWSBorder|settingsESAutoHScroll)
		diagnosticGroup := must(2501, "BUTTON", localize(language, textDiagnostics), settingsBSGroupBox)
		diagnostics := must(settingsIDDiagnostics, "BUTTON", localize(language, textSaveDiagnostics), settingsWSTabStop|settingsBSAutoCheckBox)
		directoryLabel := must(2502, "STATIC", localize(language, textDirectory), 0)
		directory := must(settingsIDDiagnosticDir, "EDIT", "", settingsWSTabStop|settingsWSBorder|settingsESAutoHScroll)
		browse := must(settingsIDBrowse, "BUTTON", localize(language, textBrowse), settingsWSTabStop)
		limitLabel := must(2503, "STATIC", localize(language, textRejectedFrameLimit), 0)
		limit := must(settingsIDDiagnosticLimit, "EDIT", "", settingsWSTabStop|settingsWSBorder|settingsESAutoHScroll|settingsESNumber)
		overrideNote := must(2504, "STATIC", state.overrideMessage(), 0)
		state.advanced = []uintptr{captureGroup, modeLabel, mode, intervalLabel, interval, maxScrollLabel, maxScroll, maxDiffLabel, maxDiff, confidenceLabel, confidence, stationaryLabel, stationary, diagnosticGroup, diagnostics, directoryLabel, directory, browse, limitLabel, limit, overrideNote}

		must(1, "BUTTON", localize(language, textOK), settingsWSTabStop|settingsBSDefaultPushButton)
		must(2, "BUTTON", localize(language, textCancel), settingsWSTabStop)
		must(settingsIDApply, "BUTTON", localize(language, textApply), settingsWSTabStop)
	}()
	if createErr != nil {
		return createErr
	}
	state.layout()
	state.load(state.host.preferences)
	procSettingsSendMessage.Call(state.controls[settingsIDTree], settingsTVMSelectItem, settingsTVGNCaret, state.generalItem)
	state.showPage(false)
	state.setDirty(false)
	return nil
}

func (state *settingsWindow) insertTreeItem(tree uintptr, label string, param uintptr) uintptr {
	text, _ := syscall.UTF16PtrFromString(label)
	insert := settingsTreeInsert{
		Parent: 0, InsertAfter: ^uintptr(0xFFFD),
		Item: settingsTreeItem{Mask: settingsTVIFText | settingsTVIFParam, Text: text, Param: param},
	}
	item, _, _ := procSettingsSendMessage.Call(tree, settingsTVMInsertItemW, 0, uintptr(unsafe.Pointer(&insert)))
	return item
}

func (state *settingsWindow) layout() {
	scale := func(value int) uintptr { return uintptr((value*state.dpi + 48) / 96) }
	move := func(id, x, y, width, height int) {
		if hwnd := state.controls[id]; hwnd != 0 {
			procSettingsSetWindowPos.Call(hwnd, 0, scale(x), scale(y), scale(width), scale(height), 0x0014)
		}
	}
	move(settingsIDTree, 12, 12, 145, 388)
	move(2301, 174, 12, 476, 150)
	move(2302, 194, 48, 92, 22)
	move(settingsIDHotkey, 292, 44, 190, 26)
	move(2303, 194, 82, 390, 22)
	move(2304, 194, 108, 92, 22)
	move(settingsIDLanguage, 292, 104, 190, 120)

	move(2401, 174, 12, 476, 237)
	move(2407, 194, 45, 160, 22)
	move(settingsIDLongCaptureMode, 365, 41, 220, 160)
	labels := []int{2402, 2403, 2404, 2405, 2406}
	edits := []int{settingsIDInterval, settingsIDMaxScroll, settingsIDMaxDifference, settingsIDMinConfidence, settingsIDStationary}
	for index := range labels {
		y := 74 + index*32
		move(labels[index], 194, y+3, 160, 22)
		move(edits[index], 365, y, 140, 24)
	}
	move(2501, 174, 258, 476, 142)
	move(settingsIDDiagnostics, 194, 281, 180, 22)
	move(2502, 194, 313, 52, 22)
	move(settingsIDDiagnosticDir, 246, 310, 290, 24)
	move(settingsIDBrowse, 544, 309, 82, 26)
	move(2503, 194, 347, 168, 22)
	move(settingsIDDiagnosticLimit, 365, 344, 140, 24)
	move(2504, 194, 372, 420, 20)

	move(1, 410, 416, 76, 28)
	move(2, 492, 416, 76, 28)
	move(settingsIDApply, 574, 416, 76, 28)
}

func (state *settingsWindow) load(value preferences) {
	state.loading = true
	defer func() { state.loading = false }()
	hotkey, _ := parseConfiguredHotkey(value.General.Hotkey)
	procSettingsSendMessage.Call(state.controls[settingsIDHotkey], settingsHKMSetRules, 0x0001, settingsHotkeyFCtrl|settingsHotkeyFAlt)
	procSettingsSendMessage.Call(state.controls[settingsIDHotkey], settingsHKMSetHotkey, uintptr(hotkeyToControl(hotkey)), 0)
	procSettingsSendMessage.Call(state.controls[settingsIDLanguage], settingsCBSetCurrent, uintptr(languageIndex(value.General.Language)), 0)
	modeIndex := uintptr(0)
	if value.LongCapture.Mode == longCaptureModeLegacy {
		modeIndex = 1
	}
	procSettingsSendMessage.Call(state.controls[settingsIDLongCaptureMode], settingsCBSetCurrent, modeIndex, 0)
	state.setText(settingsIDInterval, strconv.Itoa(value.LongCapture.IntervalMS))
	state.setText(settingsIDMaxScroll, strconv.FormatFloat(value.LongCapture.MaxScrollRatio, 'g', -1, 64))
	state.setText(settingsIDMaxDifference, strconv.FormatFloat(value.LongCapture.MaxMeanDifference, 'g', -1, 64))
	state.setText(settingsIDMinConfidence, strconv.FormatFloat(value.LongCapture.MinimumConfidence, 'g', -1, 64))
	state.setText(settingsIDStationary, strconv.FormatFloat(value.LongCapture.StationaryThreshold, 'g', -1, 64))
	checked := uintptr(0)
	if value.Diagnostics.Enabled {
		checked = settingsBSTChecked
	}
	procSettingsSendMessage.Call(state.controls[settingsIDDiagnostics], settingsBMSetCheck, checked, 0)
	state.setText(settingsIDDiagnosticDir, value.Diagnostics.Directory)
	state.setText(settingsIDDiagnosticLimit, strconv.Itoa(value.Diagnostics.Limit))
	state.updateDiagnosticControls()
}

func (state *settingsWindow) handleCommand(id int, notification uint32) {
	switch id {
	case 1:
		if !state.dirty || state.apply() {
			procSettingsDestroyWindow.Call(state.hwnd)
		}
	case 2:
		procSettingsDestroyWindow.Call(state.hwnd)
	case settingsIDApply:
		state.apply()
	case settingsIDBrowse:
		if notification == settingsBNClicked {
			if path, ok := browseSettingsDirectory(state.hwnd); ok {
				state.setText(settingsIDDiagnosticDir, path)
				state.setDirty(true)
			}
		}
	case settingsIDDiagnostics:
		if notification == settingsBNClicked {
			state.updateDiagnosticControls()
			state.setDirty(true)
		}
	case settingsIDLanguage, settingsIDLongCaptureMode:
		if notification == settingsCBNSelectionChange && !state.loading {
			state.setDirty(true)
		}
	default:
		if notification == settingsENChange && !state.loading {
			state.setDirty(true)
		}
	}
}

func (state *settingsWindow) apply() bool {
	value, control, err := state.read()
	if err != nil {
		showErrorMessage(state.hwnd, err)
		if control != 0 {
			procSettingsSetFocus.Call(control)
		}
		return false
	}
	languageChanged := value.General.Language != state.host.preferences.General.Language
	if err := state.host.applyPreferences(value); err != nil {
		showErrorMessage(state.hwnd, err)
		return false
	}
	if languageChanged {
		// Recreate the native controls so every label, tree item, and title is
		// updated immediately after Apply, without requiring an app restart.
		procSettingsDestroyWindow.Call(state.hwnd)
		procTrayPostMessage.Call(state.host.hwnd, wmCommand, trayCommandSettings, 0)
		return true
	}
	state.load(state.host.preferences)
	state.setDirty(false)
	return true
}

func (state *settingsWindow) read() (preferences, uintptr, error) {
	value := state.host.preferences
	languageIndex, _, _ := procSettingsSendMessage.Call(state.controls[settingsIDLanguage], settingsCBGetCurrent, 0, 0)
	value.General.Language = languageEnglish
	if int(languageIndex) < len(availableLanguages) {
		value.General.Language = availableLanguages[languageIndex].Code
	}
	controlValue, _, _ := procSettingsSendMessage.Call(state.controls[settingsIDHotkey], settingsHKMGetHotkey, 0, 0)
	hotkey := hotkeyFromControl(uint16(controlValue))
	if hotkey.Modifiers == 0 || hotkey.Key == 0 {
		return value, state.controls[settingsIDHotkey], fmt.Errorf("截图快捷键必须包含 Ctrl、Alt 或 Shift 与一个普通按键")
	}
	value.General.Hotkey = formatConfiguredHotkey(hotkey)
	modeIndex, _, _ := procSettingsSendMessage.Call(state.controls[settingsIDLongCaptureMode], settingsCBGetCurrent, 0, 0)
	value.LongCapture.Mode = longCaptureModeBidirectional
	if modeIndex == 1 {
		value.LongCapture.Mode = longCaptureModeLegacy
	}
	var err error
	if value.LongCapture.IntervalMS, err = state.readInt(settingsIDInterval); err != nil {
		return value, state.controls[settingsIDInterval], fmt.Errorf("截图间隔必须是整数")
	}
	if value.LongCapture.IntervalMS <= 0 {
		return value, state.controls[settingsIDInterval], fmt.Errorf("截图间隔必须大于 0 毫秒")
	}
	floatFields := []struct {
		id     int
		target *float64
		label  string
	}{
		{settingsIDMaxScroll, &value.LongCapture.MaxScrollRatio, "最大滚动比例"},
		{settingsIDMaxDifference, &value.LongCapture.MaxMeanDifference, "最大平均差异"},
		{settingsIDMinConfidence, &value.LongCapture.MinimumConfidence, "最小置信度"},
		{settingsIDStationary, &value.LongCapture.StationaryThreshold, "静止判定阈值"},
	}
	for _, field := range floatFields {
		if *field.target, err = state.readFloat(field.id); err != nil {
			return value, state.controls[field.id], fmt.Errorf("%s必须是数字", field.label)
		}
		if math.IsNaN(*field.target) || math.IsInf(*field.target, 0) {
			return value, state.controls[field.id], fmt.Errorf("%s必须是有限数字", field.label)
		}
	}
	if value.LongCapture.MaxScrollRatio <= 0 || value.LongCapture.MaxScrollRatio >= 1 {
		return value, state.controls[settingsIDMaxScroll], fmt.Errorf("最大滚动比例必须大于 0 且小于 1")
	}
	if value.LongCapture.MaxMeanDifference < 0 || value.LongCapture.MaxMeanDifference > 255 {
		return value, state.controls[settingsIDMaxDifference], fmt.Errorf("最大平均差异必须在 0 到 255 之间")
	}
	if value.LongCapture.MinimumConfidence < 0 || value.LongCapture.MinimumConfidence > 256 {
		return value, state.controls[settingsIDMinConfidence], fmt.Errorf("最小置信度必须在 0 到 256 之间")
	}
	if value.LongCapture.StationaryThreshold < 0 || value.LongCapture.StationaryThreshold > 255 {
		return value, state.controls[settingsIDStationary], fmt.Errorf("静止判定阈值必须在 0 到 255 之间")
	}
	checked, _, _ := procSettingsSendMessage.Call(state.controls[settingsIDDiagnostics], settingsBMGetCheck, 0, 0)
	value.Diagnostics.Enabled = checked == settingsBSTChecked
	value.Diagnostics.Directory = strings.TrimSpace(state.text(settingsIDDiagnosticDir))
	if value.Diagnostics.Enabled && value.Diagnostics.Directory == "" {
		return value, state.controls[settingsIDDiagnosticDir], fmt.Errorf("启用诊断时目录不能为空")
	}
	if value.Diagnostics.Limit, err = state.readInt(settingsIDDiagnosticLimit); err != nil {
		return value, state.controls[settingsIDDiagnosticLimit], fmt.Errorf("诊断上限必须是整数")
	}
	if value.Diagnostics.Limit < 0 {
		return value, state.controls[settingsIDDiagnosticLimit], fmt.Errorf("诊断上限不能为负数")
	}
	if err := value.Validate(); err != nil {
		return value, 0, err
	}
	return value, 0, nil
}

func (state *settingsWindow) updateSelectedPage() {
	item, _, _ := procSettingsSendMessage.Call(state.controls[settingsIDTree], settingsTVMGetNextItem, settingsTVGNCaret, 0)
	state.showPage(item == state.advancedItem)
}

func (state *settingsWindow) showPage(advanced bool) {
	for _, hwnd := range state.general {
		command := uintptr(settingsSWShow)
		if advanced {
			command = settingsSWHide
		}
		procSettingsShowWindow.Call(hwnd, command)
	}
	for _, hwnd := range state.advanced {
		command := uintptr(settingsSWHide)
		if advanced {
			command = settingsSWShow
		}
		procSettingsShowWindow.Call(hwnd, command)
	}
}

func (state *settingsWindow) updateDiagnosticControls() {
	checked, _, _ := procSettingsSendMessage.Call(state.controls[settingsIDDiagnostics], settingsBMGetCheck, 0, 0)
	enabled := uintptr(0)
	if checked == settingsBSTChecked {
		enabled = 1
	}
	procSettingsEnableWindow.Call(state.controls[settingsIDDiagnosticDir], enabled)
	procSettingsEnableWindow.Call(state.controls[settingsIDBrowse], enabled)
	procSettingsEnableWindow.Call(state.controls[settingsIDDiagnosticLimit], enabled)
}

func (state *settingsWindow) setDirty(dirty bool) {
	state.dirty = dirty
	enabled := uintptr(0)
	if dirty {
		enabled = 1
	}
	procSettingsEnableWindow.Call(state.controls[settingsIDApply], enabled)
}

func (state *settingsWindow) setText(id int, text string) {
	value, _ := syscall.UTF16PtrFromString(text)
	procSettingsSetWindowText.Call(state.controls[id], uintptr(unsafe.Pointer(value)))
}

func (state *settingsWindow) addComboString(combo uintptr, text string) {
	value, _ := syscall.UTF16PtrFromString(text)
	procSettingsSendMessage.Call(combo, settingsCBAddString, 0, uintptr(unsafe.Pointer(value)))
}

func (state *settingsWindow) text(id int) string {
	hwnd := state.controls[id]
	length, _, _ := procSettingsGetWindowTextLen.Call(hwnd)
	buffer := make([]uint16, length+1)
	procSettingsGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	return syscall.UTF16ToString(buffer)
}

func (state *settingsWindow) readInt(id int) (int, error) {
	return strconv.Atoi(strings.TrimSpace(state.text(id)))
}

func (state *settingsWindow) readFloat(id int) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(state.text(id)), 64)
}

func (state *settingsWindow) overrideMessage() string {
	overrides := state.host.overrides
	if !overrides.Interval && !overrides.MaxScrollRatio && !overrides.MaxMeanDifference && !overrides.MinimumConfidence && !overrides.StationaryThreshold && !overrides.DiagnosticDirectory && !overrides.DiagnosticLimit {
		return ""
	}
	return localize(state.host.preferences.General.Language, textOverrideNote)
}

func hotkeyToControl(value configuredHotkey) uint16 {
	modifiers := uint16(0)
	if value.Modifiers&hotkeyModifierShift != 0 {
		modifiers |= settingsHotkeyFShift
	}
	if value.Modifiers&hotkeyModifierControl != 0 {
		modifiers |= settingsHotkeyFCtrl
	}
	if value.Modifiers&hotkeyModifierAlt != 0 {
		modifiers |= settingsHotkeyFAlt
	}
	return uint16(value.Key&0xff) | modifiers<<8
}

func hotkeyFromControl(value uint16) configuredHotkey {
	result := configuredHotkey{Key: uint32(value & 0xff)}
	modifiers := value >> 8
	if modifiers&settingsHotkeyFShift != 0 {
		result.Modifiers |= hotkeyModifierShift
	}
	if modifiers&settingsHotkeyFCtrl != 0 {
		result.Modifiers |= hotkeyModifierControl
	}
	if modifiers&settingsHotkeyFAlt != 0 {
		result.Modifiers |= hotkeyModifierAlt
	}
	return result
}

func browseSettingsDirectory(owner uintptr) (string, bool) {
	result, _, _ := procSettingsCoInitializeEx.Call(0, 0x00000002|0x00000004)
	if int32(result) >= 0 {
		defer procSettingsCoUninitialize.Call()
	}
	display := make([]uint16, 260)
	title, _ := syscall.UTF16PtrFromString(localize(uiLanguage(), textSelectDiagnosticsDirectory))
	info := settingsBrowseInfo{
		Owner: owner, DisplayName: &display[0], Title: title,
		Flags: settingsBIFReturnOnlyFSDirs | settingsBIFNewDialogStyle,
	}
	item, _, _ := procSettingsBrowseForFolder.Call(uintptr(unsafe.Pointer(&info)))
	if item == 0 {
		return "", false
	}
	defer procSettingsCoTaskMemFree.Call(item)
	path := make([]uint16, 32768)
	ok, _, _ := procSettingsGetPathFromIDList.Call(item, uintptr(unsafe.Pointer(&path[0])))
	if ok == 0 {
		return "", false
	}
	return syscall.UTF16ToString(path), true
}
