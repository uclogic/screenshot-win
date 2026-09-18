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
	settingsIDTree            = 2001
	settingsIDHotkey          = 2002
	settingsIDLanguage        = 2003
	settingsIDPinHotkey       = 2004
	settingsIDClearPinHotkey  = 2005
	settingsIDClearHotkey     = 2006
	settingsIDTransparency    = 2007
	settingsIDLongCaptureMode = 2100
	settingsIDInterval        = 2101
	settingsIDMaxScroll       = 2102
	settingsIDMaxDifference   = 2103
	settingsIDMinConfidence   = 2104
	settingsIDStationary      = 2105

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

	settingsENChange           = 0x0300
	settingsCBNSelectionChange = 1
	settingsCBAddString        = 0x0143
	settingsCBGetCurrent       = 0x0147
	settingsCBSetCurrent       = 0x014E

	settingsSWHide         = 0
	settingsSWShow         = 5
	settingsSWRestore      = 9
	settingsWMSetFont      = 0x0030
	settingsDefaultGUIFont = 17

	settingsICCWin95Classes = 0x000000FF
)

var (
	settingsUser32               = syscall.NewLazyDLL("user32.dll")
	settingsGDI32                = syscall.NewLazyDLL("gdi32.dll")
	settingsComctl32             = syscall.NewLazyDLL("comctl32.dll")
	procSettingsCreateWindowEx   = settingsUser32.NewProc("CreateWindowExW")
	procSettingsDefWindowProc    = settingsUser32.NewProc("DefWindowProcW")
	procSettingsDestroyWindow    = settingsUser32.NewProc("DestroyWindow")
	procSettingsEnableWindow     = settingsUser32.NewProc("EnableWindow")
	procSettingsGetDPIForWindow  = settingsUser32.NewProc("GetDpiForWindow")
	procSettingsGetWindowText    = settingsUser32.NewProc("GetWindowTextW")
	procSettingsGetWindowTextLen = settingsUser32.NewProc("GetWindowTextLengthW")
	procSettingsLoadCursor       = settingsUser32.NewProc("LoadCursorW")
	procSettingsRegisterClassEx  = settingsUser32.NewProc("RegisterClassExW")
	procSettingsSendMessage      = settingsUser32.NewProc("SendMessageW")
	procSettingsSetFocus         = settingsUser32.NewProc("SetFocus")
	procSettingsSetForeground    = settingsUser32.NewProc("SetForegroundWindow")
	procSettingsSetWindowPos     = settingsUser32.NewProc("SetWindowPos")
	procSettingsSetWindowText    = settingsUser32.NewProc("SetWindowTextW")
	procSettingsShowWindow       = settingsUser32.NewProc("ShowWindow")
	procSettingsGetStockObject   = settingsGDI32.NewProc("GetStockObject")
	procSettingsInitControls     = settingsComctl32.NewProc("InitCommonControlsEx")

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
		state.host.finishHotkeyRecording()
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
		clearCapture := must(settingsIDClearHotkey, "BUTTON", localize(language, textClearHotkey), settingsWSTabStop)
		pinLabel := must(2305, "STATIC", localize(language, textPinClipboard), 0)
		pinHotkey := must(settingsIDPinHotkey, "msctls_hotkey32", "", settingsWSTabStop|settingsWSBorder)
		clearPin := must(settingsIDClearPinHotkey, "BUTTON", localize(language, textClearHotkey), settingsWSTabStop)
		for _, control := range []uintptr{hotkey, pinHotkey} {
			ok, _, err := procSettingsSetWindowSubclass.Call(control, settingsHotkeySubclass, 1, state.hwnd)
			if ok == 0 {
				panic(trayWin32Error("SetWindowSubclass", err))
			}
		}
		generalHelp := must(2303, "STATIC", localize(language, textHotkeyHelp), 0)
		languageLabel := must(2304, "STATIC", localize(language, textLanguageLabel), 0)
		languageCombo := must(settingsIDLanguage, "COMBOBOX", "", settingsWSTabStop|settingsCBSDropDownList)
		for _, option := range availableLanguages {
			state.addComboString(languageCombo, option.Name)
		}
		state.general = []uintptr{generalGroup, generalLabel, hotkey, clearCapture, pinLabel, pinHotkey, clearPin, generalHelp, languageLabel, languageCombo}
		transparencyLabel := must(2306, "STATIC", localize(language, textToolbarTransparency), 0)
		transparency := must(settingsIDTransparency, "EDIT", "", settingsWSTabStop|settingsWSBorder|settingsESAutoHScroll|settingsESNumber)
		transparencyHelp := must(2307, "STATIC", localize(language, textToolbarTransparencyHelp), 0)
		state.general = append(state.general, transparencyLabel, transparency, transparencyHelp)

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
		state.advanced = []uintptr{captureGroup, modeLabel, mode, intervalLabel, interval, maxScrollLabel, maxScroll, maxDiffLabel, maxDiff, confidenceLabel, confidence, stationaryLabel, stationary}

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
	move(2301, 174, 12, 476, 160)
	move(2302, 194, 48, 92, 22)
	move(settingsIDHotkey, 292, 44, 190, 26)
	move(settingsIDClearHotkey, 492, 44, 76, 26)
	move(2305, 194, 84, 96, 22)
	move(settingsIDPinHotkey, 292, 80, 190, 26)
	move(settingsIDClearPinHotkey, 492, 80, 76, 26)
	move(2303, 194, 118, 432, 44)
	move(2304, 194, 192, 92, 22)
	move(settingsIDLanguage, 292, 188, 190, 120)
	move(2306, 194, 240, 200, 22)
	move(settingsIDTransparency, 404, 236, 78, 26)
	move(2307, 194, 274, 432, 44)

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

	move(1, 410, 416, 76, 28)
	move(2, 492, 416, 76, 28)
	move(settingsIDApply, 574, 416, 76, 28)
}

func (state *settingsWindow) load(value preferences) {
	state.loading = true
	defer func() { state.loading = false }()
	hotkey, _ := parseConfiguredHotkey(value.General.Hotkey)
	procSettingsSendMessage.Call(state.controls[settingsIDHotkey], settingsHKMSetRules, 0, 0)
	procSettingsSendMessage.Call(state.controls[settingsIDHotkey], settingsHKMSetHotkey, uintptr(hotkeyToControl(hotkey)), 0)
	pin, _ := parseConfiguredHotkey(value.General.PinHotkey)
	procSettingsSendMessage.Call(state.controls[settingsIDPinHotkey], settingsHKMSetRules, 0, 0)
	procSettingsSendMessage.Call(state.controls[settingsIDPinHotkey], settingsHKMSetHotkey, uintptr(hotkeyToControl(pin)), 0)
	procSettingsSendMessage.Call(state.controls[settingsIDLanguage], settingsCBSetCurrent, uintptr(languageIndex(value.General.Language)), 0)
	modeIndex := uintptr(0)
	if value.LongCapture.Mode == longCaptureModeLegacy {
		modeIndex = 1
	}
	procSettingsSendMessage.Call(state.controls[settingsIDLongCaptureMode], settingsCBSetCurrent, modeIndex, 0)
	state.setText(settingsIDInterval, strconv.Itoa(value.LongCapture.IntervalMS))
	state.setText(settingsIDTransparency, strconv.Itoa(value.General.ToolbarTransparency))
	state.setText(settingsIDMaxScroll, strconv.FormatFloat(value.LongCapture.MaxScrollRatio, 'g', -1, 64))
	state.setText(settingsIDMaxDifference, strconv.FormatFloat(value.LongCapture.MaxMeanDifference, 'g', -1, 64))
	state.setText(settingsIDMinConfidence, strconv.FormatFloat(value.LongCapture.MinimumConfidence, 'g', -1, 64))
	state.setText(settingsIDStationary, strconv.FormatFloat(value.LongCapture.StationaryThreshold, 'g', -1, 64))
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
	case settingsIDClearHotkey:
		procSettingsSendMessage.Call(state.controls[settingsIDHotkey], settingsHKMSetHotkey, 0, 0)
		state.setDirty(true)
	case settingsIDClearPinHotkey:
		procSettingsSendMessage.Call(state.controls[settingsIDPinHotkey], settingsHKMSetHotkey, 0, 0)
		state.setDirty(true)
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
	transparency, parseErr := state.readInt(settingsIDTransparency)
	if parseErr != nil || transparency < 0 || transparency > 100 {
		return value, state.controls[settingsIDTransparency], fmt.Errorf("%s 0–100", localize(value.General.Language, textToolbarTransparency))
	}
	value.General.ToolbarTransparency = transparency
	languageIndex, _, _ := procSettingsSendMessage.Call(state.controls[settingsIDLanguage], settingsCBGetCurrent, 0, 0)
	value.General.Language = languageEnglish
	if int(languageIndex) < len(availableLanguages) {
		value.General.Language = availableLanguages[languageIndex].Code
	}
	controlValue, _, _ := procSettingsSendMessage.Call(state.controls[settingsIDHotkey], settingsHKMGetHotkey, 0, 0)
	hotkey := hotkeyFromControl(uint16(controlValue))
	value.General.Hotkey = ""
	if controlValue != 0 {
		if err := validateConfiguredHotkey(hotkey); err != nil {
			return value, state.controls[settingsIDHotkey], err
		}
		value.General.Hotkey = formatConfiguredHotkey(hotkey)
	}
	pinValue, _, _ := procSettingsSendMessage.Call(state.controls[settingsIDPinHotkey], settingsHKMGetHotkey, 0, 0)
	value.General.PinHotkey = ""
	if pinValue != 0 {
		pin := hotkeyFromControl(uint16(pinValue))
		if err := validateConfiguredHotkey(pin); err != nil {
			return value, state.controls[settingsIDPinHotkey], err
		}
		if pin == hotkey {
			return value, state.controls[settingsIDPinHotkey], fmt.Errorf("截图和贴图不能使用相同的快捷键")
		}
		value.General.PinHotkey = formatConfiguredHotkey(pin)
	}
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

var (
	procSettingsSetWindowSubclass    = settingsComctl32.NewProc("SetWindowSubclass")
	procSettingsDefSubclassProc      = settingsComctl32.NewProc("DefSubclassProc")
	procSettingsRemoveWindowSubclass = settingsComctl32.NewProc("RemoveWindowSubclass")
	settingsHotkeySubclass           uintptr
)

func init() { settingsHotkeySubclass = syscall.NewCallback(settingsHotkeyProc) }

func settingsHotkeyProc(hwnd uintptr, message uint32, wParam, lParam, id, parent uintptr) uintptr {
	if message == 0x0082 {
		procSettingsRemoveWindowSubclass.Call(hwnd, settingsHotkeySubclass, id)
	} // WM_NCDESTROY
	if value, ok := settingsWindows.Load(parent); ok {
		state := value.(*settingsWindow)
		switch message {
		case 0x0007: // WM_SETFOCUS: allow recording even an already registered chord.
			state.host.recordingHotkey = true
			state.host.hotkeys.close()
		case 0x0008: // WM_KILLFOCUS
			state.host.finishHotkeyRecording()
		}
	}
	result, _, _ := procSettingsDefSubclassProc.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}
