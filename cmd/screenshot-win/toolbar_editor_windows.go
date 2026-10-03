//go:build windows

package main

import (
	"image"
	"sync"
	"syscall"
	"unsafe"

	"screenshot-win/selector"
)

var (
	toolbarEditors         sync.Map
	toolbarEditorProcedure uintptr
	editorBeginPaint       = settingsUser32.NewProc("BeginPaint")
	editorEndPaint         = settingsUser32.NewProc("EndPaint")
	editorInvalidate       = settingsUser32.NewProc("InvalidateRect")
	editorFillRect         = settingsUser32.NewProc("FillRect")
	editorFrameRect        = settingsUser32.NewProc("FrameRect")
	editorDrawText         = settingsUser32.NewProc("DrawTextW")
	editorSetCapture       = settingsUser32.NewProc("SetCapture")
	editorReleaseCapture   = settingsUser32.NewProc("ReleaseCapture")
	editorCreateBrush      = settingsGDI32.NewProc("CreateSolidBrush")
	editorDeleteObject     = settingsGDI32.NewProc("DeleteObject")
	editorSelectObject     = settingsGDI32.NewProc("SelectObject")
	editorSetTextColor     = settingsGDI32.NewProc("SetTextColor")
	editorSetBkMode        = settingsGDI32.NewProc("SetBkMode")
)

func init() { toolbarEditorProcedure = syscall.NewCallback(toolbarEditorProc) }

var (
	editorCreateDC     = settingsGDI32.NewProc("CreateCompatibleDC")
	editorCreateBitmap = settingsGDI32.NewProc("CreateCompatibleBitmap")
	editorDeleteDC     = settingsGDI32.NewProc("DeleteDC")
	editorBitBlt       = settingsGDI32.NewProc("BitBlt")
	editorAlphaBlend   = syscall.NewLazyDLL("msimg32.dll").NewProc("AlphaBlend")
	editorGetCapture   = settingsUser32.NewProc("GetCapture")
)

// An offscreen surface avoids flicker while dragging. All GDI resources are
// released after painting, including the selected bitmap.
func editorSurface(dc uintptr, width, height int) (uintptr, func()) {
	memory, _, _ := editorCreateDC.Call(dc)
	if memory == 0 {
		return 0, func() {}
	}
	bitmap, _, _ := editorCreateBitmap.Call(dc, uintptr(width), uintptr(height))
	if bitmap == 0 {
		editorDeleteDC.Call(memory)
		return 0, func() {}
	}
	old, _, _ := editorSelectObject.Call(memory, bitmap)
	return memory, func() {
		editorSelectObject.Call(memory, old)
		editorDeleteObject.Call(bitmap)
		editorDeleteDC.Call(memory)
	}
}

type editorPaintStruct struct {
	DC                 uintptr
	Erase              int32
	Paint              settingsRect
	Restore, IncUpdate int32
	Reserved           [32]byte
}

type editorToolInfo struct {
	Size, Flags uint32
	Window, ID  uintptr
	Area        settingsRect
	Instance    uintptr
	Text        *uint16
	Parameter   uintptr
}

type toolbarEditor struct {
	owner                  *settingsWindow
	hwnd, tooltip          uintptr
	kind                   selector.ToolbarKind
	ids                    []string
	tipText                *uint16
	tipID, message, dragID string
	origin, pointer        image.Point
	pressed, dragging      bool
}

func newToolbarEditor(owner *settingsWindow, hwnd uintptr, index int) (*toolbarEditor, error) {
	editor := &toolbarEditor{owner: owner, hwnd: hwnd, kind: selector.ToolbarKind(index)}
	toolbarEditors.Store(hwnd, editor)
	if ok, _, err := procSettingsSetWindowSubclass.Call(hwnd, toolbarEditorProcedure, 1, 0); ok == 0 {
		toolbarEditors.Delete(hwnd)
		return nil, trayWin32Error("SetWindowSubclass toolbar editor", err)
	}
	class, _ := syscall.UTF16PtrFromString("tooltips_class32")
	tip, _, err := procSettingsCreateWindowEx.Call(8, uintptr(unsafe.Pointer(class)), 0, 0x80000003, 0, 0, 0, 0, hwnd, 0, owner.host.instance, 0)
	if tip == 0 {
		return nil, trayWin32Error("CreateWindowExW toolbar tooltip", err)
	}
	editor.tooltip = tip
	editor.tipText, _ = syscall.UTF16PtrFromString(" ")
	info := editorToolInfo{Size: uint32(unsafe.Offsetof(editorToolInfo{}.Parameter)), Flags: 0x11, Window: hwnd, ID: hwnd, Text: editor.tipText}
	if ok, _, _ := procSettingsSendMessage.Call(tip, 0x0432, 0, uintptr(unsafe.Pointer(&info))); ok == 0 {
		procSettingsDestroyWindow.Call(tip)
		return nil, syscall.EINVAL
	}
	procSettingsSendMessage.Call(tip, 0x0418, 0, 280) // TTM_SETMAXTIPWIDTH
	return editor, nil
}

func (e *toolbarEditor) geometry() toolbarEditorGeometry {
	return toolbarEditorGeometry{dpi: e.owner.dpi}
}
func (e *toolbarEditor) scale(v int) int                     { return e.geometry().scale(v) }
func (e *toolbarEditor) rect(x, y, w, h int) image.Rectangle { return e.geometry().rect(x, y, w, h) }
func (e *toolbarEditor) row(candidate bool) image.Rectangle  { return e.geometry().row(candidate) }
func (e *toolbarEditor) cell(index int, candidate bool) image.Rectangle {
	return e.geometry().cell(index, candidate)
}
func (e *toolbarEditor) hit(p image.Point) (string, bool) { return e.geometry().hit(e.kind, e.ids, p) }
func (e *toolbarEditor) drop(p image.Point) (int, bool)   { return e.geometry().drop(len(e.ids), p) }
func (e *toolbarEditor) refresh()                         { editorInvalidate.Call(e.hwnd, 0, 0) }
func (e *toolbarEditor) cancelDrag() {
	if !e.pressed {
		return
	}
	e.pressed, e.dragging = false, false
	e.dragID = ""
	if capture, _, _ := editorGetCapture.Call(); capture == e.hwnd {
		editorReleaseCapture.Call()
	}
	e.refresh()
}
func (e *toolbarEditor) setTip(id string) {
	if e.tipID == id {
		return
	}
	e.tipID = id
	text := " "
	if item, ok := selector.ToolbarItemForID(id); ok {
		text = toolbarItemLabel(e.owner.host.preferences.General.Language, item)
	}
	e.tipText, _ = syscall.UTF16PtrFromString(text)
	info := editorToolInfo{Size: uint32(unsafe.Offsetof(editorToolInfo{}.Parameter)), Flags: 0x11, Window: e.hwnd, ID: e.hwnd, Text: e.tipText}
	procSettingsSendMessage.Call(e.tooltip, 0x0439, 0, uintptr(unsafe.Pointer(&info))) // TTM_UPDATETIPTEXTW
	if id == "" {
		procSettingsSendMessage.Call(e.tooltip, 0x041c, 0, 0)
	}
}

func toolbarEditorProc(hwnd uintptr, message uint32, wParam, lParam, subclass, data uintptr) uintptr {
	value, ok := toolbarEditors.Load(hwnd)
	if !ok {
		result, _, _ := procSettingsDefSubclassProc.Call(hwnd, uintptr(message), wParam, lParam)
		return result
	}
	e := value.(*toolbarEditor)
	point := image.Pt(int(int16(lParam&0xffff)), int(int16(lParam>>16)))
	switch message {
	case 0x000f: // WM_PAINT
		e.paint()
		return 0
	case 0x0014: // WM_ERASEBKGND
		return 1
	case 0x0087: // WM_GETDLGCODE: receive Esc while dragging rather than close settings
		if e.pressed {
			return 4
		}
	case 0x0201: // WM_LBUTTONDOWN
		if id, found := e.hit(point); found {
			e.cancelDrag()
			procSettingsSetFocus.Call(hwnd)
			e.dragID, e.origin, e.pointer, e.pressed = id, point, point, true
			e.message = ""
			editorSetCapture.Call(hwnd)
			e.refresh()
		}
		return 0
	case 0x0200: // WM_MOUSEMOVE
		if e.pressed {
			e.pointer = point
			if absEditor(point.X-e.origin.X) >= e.scale(4) || absEditor(point.Y-e.origin.Y) >= e.scale(4) {
				e.dragging = true
			}
			e.setTip("")
			e.refresh()
		} else {
			id, _ := e.hit(point)
			e.setTip(id)
		}
		return 0
	case 0x0202: // WM_LBUTTONUP
		if e.pressed && e.dragging {
			if index, valid := e.drop(point); valid {
				next, err := selector.ChangeToolbarLayout(e.kind, e.ids, e.dragID, index)
				if err != nil {
					e.message = localize(e.owner.host.preferences.General.Language, textToolbarMinimum)
				} else {
					e.ids = next
					e.owner.setDirty(true)
				}
			}
		}
		e.cancelDrag()
		e.refresh()
		return 0
	case 0x0100: // WM_KEYDOWN
		if wParam == 0x1b && e.pressed {
			e.cancelDrag()
			return 0
		}
	case 0x0008, 0x001f, 0x0215: // loss of focus / cancel mode / capture changed
		e.cancelDrag()
	case 0x0082: // WM_NCDESTROY
		e.cancelDrag()
		toolbarEditors.Delete(hwnd)
		if e.tooltip != 0 {
			procSettingsDestroyWindow.Call(e.tooltip)
			e.tooltip = 0
		}
		procSettingsRemoveWindowSubclass.Call(hwnd, toolbarEditorProcedure, subclass)
	}
	result, _, _ := procSettingsDefSubclassProc.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}
func absEditor(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func editorNativeRect(r image.Rectangle) settingsRect {
	return settingsRect{int32(r.Min.X), int32(r.Min.Y), int32(r.Max.X), int32(r.Max.Y)}
}
func editorFill(dc uintptr, r image.Rectangle, rgb uint32, frame bool) {
	brush, _, _ := editorCreateBrush.Call(uintptr(rgb))
	defer editorDeleteObject.Call(brush)
	rect := editorNativeRect(r)
	if frame {
		editorFrameRect.Call(dc, uintptr(unsafe.Pointer(&rect)), brush)
	} else {
		editorFillRect.Call(dc, uintptr(unsafe.Pointer(&rect)), brush)
	}
}
func editorText(dc uintptr, r image.Rectangle, text string, rgb uint32) {
	rect := editorNativeRect(r)
	value, _ := syscall.UTF16PtrFromString(text)
	editorSetTextColor.Call(dc, uintptr(rgb))
	editorDrawText.Call(dc, uintptr(unsafe.Pointer(value)), ^uintptr(0), uintptr(unsafe.Pointer(&rect)), 0x24) // single line + vertical center
}
func (e *toolbarEditor) paint() {
	var ps editorPaintStruct
	dc, _, _ := editorBeginPaint.Call(e.hwnd, uintptr(unsafe.Pointer(&ps)))
	if dc == 0 {
		return
	}
	defer editorEndPaint.Call(e.hwnd, uintptr(unsafe.Pointer(&ps)))
	screen := dc
	width, height := e.scale(476), e.scale(184)
	if memory, closeSurface := editorSurface(screen, width, height); memory != 0 {
		dc = memory
		defer closeSurface()
		defer editorBitBlt.Call(screen, 0, 0, uintptr(width), uintptr(height), dc, 0, 0, 0x00cc0020)
	}
	font, _, _ := procSettingsGetStockObject.Call(settingsDefaultGUIFont)
	oldFont, _, _ := editorSelectObject.Call(dc, font)
	defer editorSelectObject.Call(dc, oldFont)
	editorSetBkMode.Call(dc, 1)
	editorFill(dc, e.rect(0, 0, 476, 184), 0xf0f0f0, false)
	language := e.owner.host.preferences.General.Language
	title := textScreenshotToolbar
	if e.kind == selector.LongCaptureToolbar {
		title = textLongCaptureToolbar
	}
	editorText(dc, e.rect(8, 2, 320, 24), localize(language, title), 0x302820)
	editorText(dc, e.rect(8, 28, 460, 20), localize(language, textToolbarResult), 0x706050)
	editorText(dc, e.rect(8, 92, 460, 20), localize(language, textToolbarCandidates), 0x706050)
	for _, candidate := range []bool{false, true} {
		ids := e.ids
		if candidate {
			ids = selector.ToolbarCandidates(e.kind, e.ids)
		}
		editorFill(dc, e.row(candidate), 0xffffff, false)
		editorFill(dc, e.row(candidate), 0xd8d4d0, true)
		if candidate && len(ids) == 0 {
			editorText(dc, e.rect(16, 114, 440, 40), localize(language, textToolbarAllVisible), 0x908070)
		}
		for i, id := range ids {
			item, _ := selector.ToolbarItemForID(id)
			r := e.cell(i, candidate)
			selector.DrawToolbarIcon(dc, r, item.Action, e.owner.dpi, e.dragging && e.dragID == id)
		}
	}
	if e.dragging {
		if index, valid := e.drop(e.pointer); valid {
			if index < 0 {
				editorFill(dc, e.row(true), 0xd59550, true)
			} else {
				editorFill(dc, e.rect(12+index*40, 53, 2, 34), 0xd59550, false)
			}
		}
		item, _ := selector.ToolbarItemForID(e.dragID)
		size := e.scale(40)
		ghost := image.Rect(e.pointer.X-size/2, e.pointer.Y-size/2, e.pointer.X+size/2, e.pointer.Y+size/2)
		if memory, closeSurface := editorSurface(dc, size, size); memory != 0 {
			local := image.Rect(0, 0, size, size)
			editorFill(memory, local, 0xf9eee3, false)
			editorFill(memory, local, 0xd8c4af, true)
			selector.DrawToolbarIcon(memory, local, item.Action, e.owner.dpi, false)
			// AC_SRC_OVER with constant alpha 170, no per-pixel alpha.
			editorAlphaBlend.Call(dc, uintptr(ghost.Min.X), uintptr(ghost.Min.Y), uintptr(size), uintptr(size), memory, 0, 0, uintptr(size), uintptr(size), 170<<16)
			closeSurface()
		} else {
			selector.DrawToolbarIcon(dc, ghost, item.Action, e.owner.dpi, true)
		}
	}
	help := localize(language, textToolbarHelp)
	rgb := uint32(0x706050)
	if e.message != "" {
		help = e.message
		rgb = 0x3030b0
	}
	editorText(dc, e.rect(8, 158, 460, 24), help, rgb)
}

func toolbarItemLabel(language string, item selector.ToolbarItem) string {
	if language != languageChinese {
		return item.Label
	}
	labels := map[string]string{"rectangle": "矩形", "arrow": "箭头", "text": "文字", "color": "颜色", "width": "线宽", "scroll": "长截图", "edit": "停止并标注", "pin": "贴图", "save": "保存", "save_as": "另存为", "copy": "复制", "cancel": "取消"}
	return labels[item.ID]
}
