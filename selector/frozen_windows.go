//go:build windows

package selector

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"screenshot-win/editor"
)

const (
	wmKillFocus       = 0x0008
	wmTimer           = 0x0113
	wmLButtonDblClk   = 0x0203
	wmMButtonDown     = 0x0207
	wmMButtonUp       = 0x0208
	wmFrozenAnnotate  = wmUser + 91
	wmFrozenRender    = wmUser + 92
	wmFrozenCancel    = wmUser + 93
	wmFrozenShortcut  = wmUser + 94
	wmFrozenTextSave  = wmUser + 95
	wmFrozenTextDrop  = wmUser + 96
	wmFrozenStyle     = wmUser + 99
	wmFrozenToolSwap  = wmUser + 100
	wmFrozenToolStyle = wmUser + 101
	htClient          = 1
	idcArrow          = 32512
	idcIBeam          = 32513
	idcSizeNWSE       = 32642
	idcSizeNESW       = 32643
	idcSizeWE         = 32644
	idcSizeNS         = 32645
	idcSizeAll        = 32646

	wsBorder         = 0x00800000
	wsVisible        = 0x10000000
	wsTabStop        = 0x00010000
	esAutoHScroll    = 0x0080
	emSetSel         = 0x00B1
	vkReturn         = 0x0D
	vkBack           = 0x08
	vkControl        = 0x11
	vkShift          = 0x10
	vkLeft           = 0x25
	vkUp             = 0x26
	vkRight          = 0x27
	vkDown           = 0x28
	vkDelete         = 0x2E
	shortcutUndo     = 1
	shortcutRedo     = 2
	shortcutDelete   = 3
	shortcutLeft     = 4
	shortcutUp       = 5
	shortcutRight    = 6
	shortcutDown     = 7
	shortcutEscape   = 8
	frozenFrameTimer = 1
)

const frozenFrameInterval = time.Second / 60

var (
	frozenProcedure            = syscall.NewCallback(frozenWindowProcedure)
	frozenTextProcedure        = syscall.NewCallback(frozenTextWindowProcedure)
	frozenStates               sync.Map
	frozenTextStates           sync.Map
	procFrozenGetKeyState      = user32.NewProc("GetKeyState")
	procFrozenSetCursor        = user32.NewProc("SetCursor")
	procFrozenSetWindowLongPtr = user32.NewProc("SetWindowLongPtrW")
	procFrozenCallWindowProc   = user32.NewProc("CallWindowProcW")
	procFrozenGetWindowTextLen = user32.NewProc("GetWindowTextLengthW")
	procFrozenGetWindowText    = user32.NewProc("GetWindowTextW")
	procFrozenCreateFont       = gdi32.NewProc("CreateFontW")
	procFrozenSetTimer         = user32.NewProc("SetTimer")
	procFrozenKillTimer        = user32.NewProc("KillTimer")
)

type frozenStart struct {
	hwnd  uintptr
	state *frozenState
	err   error
}
type frozenAnnotationRequest struct {
	tool             editor.Tool
	style            editor.Style
	result           chan error
	start            image.Point
	current          image.Point
	pointerStart     image.Point
	dragging         bool
	drawing          bool
	clearedSelection bool
}
type frozenStyleRequest struct {
	change editor.StyleChange
	result chan frozenStyleResult
}
type frozenStyleResult struct {
	changed bool
	err     error
}
type frozenTransform struct {
	id              editor.AnnotationID
	handle          editor.TransformHandle
	original, draft editor.Annotation
	anchor          image.Point
	previous        editor.Annotation
	fastPrepared    bool
}
type frozenTextEdit struct {
	hwnd, oldProcedure, font uintptr
	id                       editor.AnnotationID
	style                    editor.Style
	start                    image.Point
	closing                  bool
}
type frozenState struct {
	*selectionState
	source        image.Image
	region        image.Rectangle
	document      *editor.Document
	viewport      editor.Viewport
	mu            sync.Mutex
	request       *frozenAnnotationRequest
	selected      editor.AnnotationID
	transform     *frozenTransform
	textEdit      *frozenTextEdit
	panning       bool
	panLast       image.Point
	dpi           int
	draftPixels   []frozenDraftPixel
	lastDraw      time.Time
	framePending  bool
	frameTimerSet bool
	styleUpdates  chan *frozenStyleRequest
	styleEvents   chan editor.Style
	toolSwapWait  bool
	toolSwapFrom  editor.Tool
	toolStyleWait bool
	toolStyleFor  editor.Tool
	toolStyle     editor.Style
}

type frozenDraftPixel struct {
	index    int
	bgra     [4]byte
	coverage float64
}

func ShowFrozenDesktop(source image.Image, region image.Rectangle) (*Frozen, error) {
	desktop := virtualDesktopBounds()
	if source == nil || source.Bounds().Dx() != desktop.Dx() || source.Bounds().Dy() != desktop.Dy() {
		return nil, fmt.Errorf("frozen desktop image size must be %dx%d", desktop.Dx(), desktop.Dy())
	}
	local := region.Sub(desktop.Min)
	sub, ok := source.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		return nil, fmt.Errorf("frozen desktop image does not support sub-images")
	}
	return ShowFrozenContent(source, region, sub.SubImage(local))
}

func ShowFrozenContent(desktopSource image.Image, region image.Rectangle, content image.Image) (*Frozen, error) {
	desktop := virtualDesktopBounds()
	if desktopSource == nil || desktopSource.Bounds().Dx() != desktop.Dx() || desktopSource.Bounds().Dy() != desktop.Dy() {
		return nil, fmt.Errorf("frozen desktop image size must be %dx%d", desktop.Dx(), desktop.Dy())
	}
	if region.Intersect(desktop).Empty() {
		return nil, fmt.Errorf("capture region %v is outside virtual desktop %v", region, desktop)
	}
	document, err := editor.NewDocument(content)
	if err != nil {
		return nil, err
	}
	styleUpdates := make(chan *frozenStyleRequest, 1)
	styleEvents := make(chan editor.Style, 1)
	started := make(chan frozenStart, 1)
	done := make(chan struct{})
	go runFrozenWindow(desktop, region, desktopSource, document, styleUpdates, styleEvents, started, done)
	result := <-started
	if result.err != nil {
		<-done
		return nil, result.err
	}
	state := result.state
	return &Frozen{
		window: result.hwnd, closeWindow: func() { procPostMessage.Call(result.hwnd, wmClose, 0, 0) }, done: done,
		annotate: func(ctx context.Context, tool editor.Tool, style editor.Style) error {
			return state.annotateContext(ctx, tool, style)
		},
		updateStyle: func(ctx context.Context, change editor.StyleChange) (bool, error) {
			return state.updateSelectedStyleContext(ctx, change)
		},
		styles:   styleEvents,
		rendered: document.Rendered,
	}, nil
}

func runFrozenWindow(desktop, region image.Rectangle, source image.Image, document *editor.Document, styleUpdates chan *frozenStyleRequest, styleEvents chan editor.Style, started chan<- frozenStart, done chan<- struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(done)
	defer close(styleEvents)
	instance, _, callErr := procGetModuleHandle.Call(0)
	if instance == 0 {
		started <- frozenStart{err: win32Error("GetModuleHandleW", callErr)}
		return
	}
	className, err := syscall.UTF16PtrFromString("ScreenshotWinFrozenDesktop")
	if err != nil {
		started <- frozenStart{err: err}
		return
	}
	cursor, _, callErr := procLoadCursor.Call(0, idcArrow)
	if cursor == 0 {
		started <- frozenStart{err: win32Error("LoadCursorW", callErr)}
		return
	}
	class := windowClassEx{Size: uint32(unsafe.Sizeof(windowClassEx{})), Style: 0x0008, WindowProcedure: frozenProcedure, Instance: instance, Cursor: cursor, ClassName: className}
	atom, _, callErr := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		started <- frozenStart{err: win32Error("RegisterClassExW", callErr)}
		return
	}
	defer procUnregisterClass.Call(uintptr(unsafe.Pointer(className)), instance)
	selection := newSelectionState(desktop)
	localRegion := region.Sub(desktop.Min)
	viewport := editor.Fit(document.Bounds().Size(), localRegion.Size())
	viewport.Offset = viewport.Offset.Add(localRegion.Min)
	state := &frozenState{selectionState: selection, source: source, region: localRegion, document: document, viewport: viewport, styleUpdates: styleUpdates, styleEvents: styleEvents}
	title, _ := syscall.UTF16PtrFromString("screenshot-win frozen desktop")
	hwnd, _, callErr := procCreateWindowEx.Call(wsExTopmost|wsExToolWindow|wsExLayered, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)), wsPopup, uintptr(desktop.Min.X), uintptr(desktop.Min.Y), uintptr(desktop.Dx()), uintptr(desktop.Dy()), 0, 0, instance, 0)
	if hwnd == 0 {
		started <- frozenStart{err: win32Error("CreateWindowExW", callErr)}
		return
	}
	state.hwnd = hwnd
	state.dpi = 96
	if procGetDPIForWindow.Find() == nil {
		if dpi, _, _ := procGetDPIForWindow.Call(hwnd); dpi > 0 {
			state.dpi = int(dpi)
		}
	}
	frozenStates.Store(hwnd, state)
	defer frozenStates.Delete(hwnd)
	if err := state.initializeSurface(); err != nil {
		procDestroyWindow.Call(hwnd)
		started <- frozenStart{err: err}
		return
	}
	defer state.closeSurface()
	if err := state.renderFrozenDesktop(); err != nil {
		procDestroyWindow.Call(hwnd)
		started <- frozenStart{err: err}
		return
	}
	procShowWindow.Call(hwnd, swShowNoActivate)
	started <- frozenStart{hwnd: hwnd, state: state}
	var msg message
	for {
		status, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(status) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
	state.finishRequest(fmt.Errorf("frozen overlay closed"))
	runtime.KeepAlive(state)
}

func (state *frozenState) annotateContext(ctx context.Context, tool editor.Tool, style editor.Style) error {
	if ctx == nil {
		ctx = context.Background()
	}
	request := &frozenAnnotationRequest{tool: tool, style: style, result: make(chan error, 1)}
	state.mu.Lock()
	if state.request != nil {
		state.mu.Unlock()
		return fmt.Errorf("another annotation is active")
	}
	state.request = request
	state.mu.Unlock()
	procPostMessage.Call(state.hwnd, wmFrozenAnnotate, 0, 0)
	stop := context.AfterFunc(ctx, func() { procPostMessage.Call(state.hwnd, wmFrozenCancel, 0, 0) })
	defer stop()
	err := <-request.result
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func (state *frozenState) updateSelectedStyleContext(ctx context.Context, change editor.StyleChange) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	request := &frozenStyleRequest{change: change, result: make(chan frozenStyleResult, 1)}
	select {
	case state.styleUpdates <- request:
		procPostMessage.Call(state.hwnd, wmFrozenStyle, 0, 0)
	case <-ctx.Done():
		return false, ctx.Err()
	}
	select {
	case result := <-request.result:
		return result.changed, result.err
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func (state *frozenState) applySelectedStyle(change editor.StyleChange) frozenStyleResult {
	annotation, ok := state.document.Get(state.selected)
	if !ok {
		state.selected = 0
		return frozenStyleResult{}
	}
	updated := annotation
	updated.Style = editor.ApplyStyleChange(annotation.Style, change)
	if updated.Style == annotation.Style {
		state.notifySelectedStyle(annotation.Style)
		return frozenStyleResult{}
	}
	if err := state.document.Replace(annotation.ID, updated); err != nil {
		return frozenStyleResult{err: err}
	}
	state.notifySelectedStyle(updated.Style)
	if state.hwnd != 0 {
		state.renderOrCloseFrozen()
	}
	return frozenStyleResult{changed: true}
}

func (state *frozenState) notifySelectedStyle(style editor.Style) {
	if state.styleEvents == nil {
		return
	}
	select {
	case state.styleEvents <- style:
	default:
		select {
		case <-state.styleEvents:
		default:
		}
		select {
		case state.styleEvents <- style:
		default:
		}
	}
}

func frozenWindowProcedure(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	value, found := frozenStates.Load(hwnd)
	if !found {
		result, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
		return result
	}
	state := value.(*frozenState)
	switch message {
	case wmNCHitTest:
		return htClient
	case wmFrozenAnnotate:
		if state.applyPendingToolSwitch() {
			return 0
		}
		state.applyPendingToolStyle()
		state.selected = 0
		state.transform = nil
		state.renderOrCloseFrozen()
		procSetForegroundWindow.Call(hwnd)
		procSetFocus.Call(hwnd)
		return 0
	case wmFrozenRender:
		state.renderOrCloseFrozen()
		return 0
	case wmFrozenCancel:
		state.cancelCurrentInteraction()
		return 0
	case wmFrozenToolSwap:
		state.finishCurrentToolForSwitch(editor.Tool(wParam))
		return 0
	case wmFrozenToolStyle:
		tool, style := unpackToolStyle(wParam, lParam)
		state.applyActiveToolStyle(tool, style)
		return 0
	case wmFrozenShortcut:
		state.applyShortcut(int(wParam), int(lParam))
		return 0
	case wmFrozenTextSave:
		state.finishTextEdit(true, true, false)
		return 0
	case wmFrozenTextDrop:
		state.finishTextEdit(false, true, true)
		return 0
	case wmFrozenStyle:
		select {
		case request := <-state.styleUpdates:
			request.result <- state.applySelectedStyle(request.change)
		default:
		}
		return 0
	case wmLButtonDown:
		point := mousePoint(lParam)
		if !point.In(state.region) {
			return 0
		}
		request := state.activeRequest()
		if request != nil {
			if state.beginSelectedTransform(point) {
				return 0
			}
			cleared, clearedSelection := state.clearSelectionForDrawing()
			request.clearedSelection = clearedSelection
			if clearedSelection {
				if err := state.clearSelectionOverlay(cleared); err != nil {
					state.renderErr = err
					procDestroyWindow.Call(state.hwnd)
					return 0
				}
			}
			state.beginDrawingGesture(request, point)
			state.draftPixels = nil
			state.lastDraw = time.Time{}
			procSetCapture.Call(hwnd)
			return 0
		}
		state.beginSelectionDrag(point)
		return 0
	case wmLButtonUp:
		request := state.activeRequest()
		if request != nil && request.dragging {
			point := mousePoint(lParam)
			isDrawing := state.updateDrawingGesture(request, point)
			request.dragging = false
			procReleaseCapture.Call()
			if isDrawing {
				start, end, tool, style := request.start, request.current, request.tool, request.style
				state.resetDrawingGesture(request)
				if tool == editor.ToolText {
					if err := state.beginTextEdit(0, start, "", style); err != nil {
						state.finishRequest(err)
					}
					return 0
				}
				if end != start {
					id, err := state.document.Add(editor.Annotation{Tool: tool, Start: start, End: end, Style: style})
					if err != nil {
						state.finishRequest(err)
						return 0
					}
					state.selected = id
				}
				state.renderOrCloseFrozen()
				state.continueOrFinishDrawingRequest(request)
				return 0
			}

			clearedSelection := request.clearedSelection
			tool, style := request.tool, request.style
			state.resetDrawingGesture(request)
			if state.selectAnnotationAt(point) {
				state.renderOrCloseFrozen()
				return 0
			}
			if tool == editor.ToolText && !clearedSelection {
				if err := state.beginTextEdit(0, state.imagePoint(point), "", style); err != nil {
					state.finishRequest(err)
				}
				return 0
			}
			if clearedSelection {
				if err := state.present(); err != nil {
					state.renderErr = err
					procDestroyWindow.Call(state.hwnd)
				}
			}
			return 0
		}
		if state.transform != nil {
			state.commitTransform()
		}
		return 0
	case wmLButtonDblClk:
		if state.textEdit != nil {
			return 0
		}
		point := mousePoint(lParam)
		if !point.In(state.region) {
			return 0
		}
		annotation, ok := state.document.HitTest(state.imagePoint(point), state.hitTolerance())
		if ok && annotation.Tool == editor.ToolText {
			state.cancelTransform()
			state.selected = annotation.ID
			state.notifySelectedStyle(annotation.Style)
			if err := state.beginTextEdit(annotation.ID, annotation.Start, annotation.Text, annotation.Style); err != nil {
				state.renderErr = err
			}
		}
		return 0
	case wmMButtonDown:
		state.panning = true
		state.panLast = mousePoint(lParam)
		procSetCapture.Call(hwnd)
		return 0
	case wmMouseMove:
		request := state.activeRequest()
		point := mousePoint(lParam)
		if state.panning {
			state.viewport.Pan(point.Sub(state.panLast))
			state.panLast = point
			state.renderOrCloseFrozen()
		} else if request != nil && request.dragging {
			if state.updateDrawingGesture(request, point) && request.tool != editor.ToolText {
				state.requestAnimationFrame()
			}
		} else if state.transform != nil {
			state.updateTransform(point)
		} else {
			state.updateCursor(point)
		}
		return 0
	case wmTimer:
		if wParam == frozenFrameTimer {
			state.onAnimationFrame()
			return 0
		}
	case wmMButtonUp:
		if state.panning {
			state.panning = false
			procReleaseCapture.Call()
		}
		return 0
	case wmMouseWheel:
		delta := int(int16((wParam >> 16) & 0xffff))
		screen := image.Pt(int(int16(lParam&0xffff)), int(int16((lParam>>16)&0xffff)))
		anchor := screen.Sub(state.desktop.Min)
		state.viewport.ZoomAt(anchor, powZoom(delta))
		state.renderOrCloseFrozen()
		return 0
	case wmRButtonDown:
		state.removeSelectedOrEscape()
		return 0
	case wmKeyDown:
		if command, amount, ok := frozenShortcutForKey(wParam, frozenKeyDown(vkControl), frozenKeyDown(vkShift)); ok {
			state.applyShortcut(command, amount)
			return 0
		}
	case wmClose:
		if state.textEdit != nil {
			state.finishTextEdit(false, false, true)
		}
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		state.cancelAnimationFrame()
		state.disposeTextEditor()
		procPostQuitMessage.Call(0)
		return 0
	}
	result, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}

func frozenKeyDown(key int) bool {
	value, _, _ := procFrozenGetKeyState.Call(uintptr(key))
	return int16(value&0xffff) < 0
}

func frozenShortcutForKey(key uintptr, control, shift bool) (command, amount int, ok bool) {
	switch {
	case key == vkEscape:
		return shortcutEscape, 0, true
	case control && (key == 'Y' || key == 'y'):
		return shortcutRedo, 0, true
	case control && (key == 'Z' || key == 'z') && shift:
		return shortcutRedo, 0, true
	case control && (key == 'Z' || key == 'z'):
		return shortcutUndo, 0, true
	case !control && (key == vkBack || key == vkDelete):
		return shortcutDelete, 0, true
	case !control && key == vkLeft:
		return shortcutLeft, nudgeAmount(shift), true
	case !control && key == vkUp:
		return shortcutUp, nudgeAmount(shift), true
	case !control && key == vkRight:
		return shortcutRight, nudgeAmount(shift), true
	case !control && key == vkDown:
		return shortcutDown, nudgeAmount(shift), true
	default:
		return 0, 0, false
	}
}

func nudgeAmount(shift bool) int {
	if shift {
		return 10
	}
	return 1
}

func (state *frozenState) applyShortcut(command, amount int) {
	if command == shortcutEscape {
		state.escape()
		return
	}
	request := state.activeRequest()
	if state.textEdit != nil || state.transform != nil || (request != nil && request.dragging) {
		return
	}
	switch command {
	case shortcutUndo:
		if state.document.Undo() {
			state.validateSelection()
			state.notifyCurrentSelectionStyle()
			state.renderOrCloseFrozen()
		}
	case shortcutRedo:
		if state.document.Redo() {
			state.validateSelection()
			state.notifyCurrentSelectionStyle()
			state.renderOrCloseFrozen()
		}
	case shortcutDelete:
		if state.deleteSelected() {
			state.renderOrCloseFrozen()
		}
	case shortcutLeft, shortcutUp, shortcutRight, shortcutDown:
		if amount <= 0 {
			amount = 1
		}
		delta := image.Point{}
		switch command {
		case shortcutLeft:
			delta.X = -amount
		case shortcutUp:
			delta.Y = -amount
		case shortcutRight:
			delta.X = amount
		case shortcutDown:
			delta.Y = amount
		}
		state.nudgeSelection(delta)
	}
}

func (state *frozenState) deleteSelected() bool {
	if state.selected == 0 || !state.document.Delete(state.selected) {
		return false
	}
	state.selected = 0
	return true
}

func (state *frozenState) removeSelectedOrEscape() {
	request := state.activeRequest()
	if state.textEdit != nil || state.transform != nil || (request != nil && request.dragging) {
		state.escape()
		return
	}
	if state.deleteSelected() {
		state.renderOrCloseFrozen()
		return
	}
	state.escape()
}

func (state *frozenState) validateSelection() {
	if state.selected != 0 {
		if _, ok := state.document.Get(state.selected); !ok {
			state.selected = 0
		}
	}
}

func (state *frozenState) notifyCurrentSelectionStyle() {
	if annotation, ok := state.document.Get(state.selected); ok {
		state.notifySelectedStyle(annotation.Style)
	}
}

func (state *frozenState) nudgeSelection(delta image.Point) {
	annotation, ok := state.document.Get(state.selected)
	if !ok {
		state.selected = 0
		return
	}
	moved := editor.Translate(annotation, delta, state.document.Bounds())
	if sameAnnotationGeometry(annotation, moved) {
		return
	}
	if err := state.document.Replace(annotation.ID, moved); err != nil {
		state.renderErr = err
		return
	}
	state.renderOrCloseFrozen()
}

func (state *frozenState) escape() {
	if state.textEdit != nil {
		state.finishTextEdit(false, true, true)
		return
	}
	if state.transform != nil {
		state.cancelTransform()
		return
	}
	if state.activeRequest() != nil {
		state.finishRequest(nil)
		return
	}
	if state.selected != 0 {
		state.selected = 0
		state.renderOrCloseFrozen()
		return
	}
	if toolbar := activeToolbarWindow.Load(); toolbar != 0 {
		procPostMessage.Call(toolbar, wmClose, 0, 0)
	}
}

func (state *frozenState) cancelCurrentInteraction() {
	if state.textEdit != nil {
		state.finishTextEdit(false, true, true)
	} else if state.transform != nil {
		state.cancelTransform()
	} else {
		state.finishRequest(nil)
	}
}

// finishCurrentToolForSwitch completes only the in-progress drawing-tool
// request. Text already entered is committed, while a tool that has not been
// placed yet is simply retired so the toolbar can dispatch its replacement.
// A captured drag finishes through the normal mouse-up path.
func (state *frozenState) finishCurrentToolForSwitch(expected editor.Tool) {
	request := state.activeRequest()
	if request == nil {
		state.toolSwapWait = true
		state.toolSwapFrom = expected
		return
	}
	if request.tool != expected {
		return
	}
	if request.dragging {
		state.toolSwapWait = true
		state.toolSwapFrom = expected
		return
	}
	if state.textEdit != nil {
		state.finishTextEdit(true, false, true)
		return
	}
	state.finishRequest(nil)
}

func (state *frozenState) applyPendingToolSwitch() bool {
	if !state.toolSwapWait {
		return false
	}
	expected := state.toolSwapFrom
	state.toolSwapWait = false
	request := state.activeRequest()
	if request == nil || request.tool != expected {
		return false
	}
	state.finishCurrentToolForSwitch(expected)
	return true
}

func unpackToolStyle(wParam, lParam uintptr) (editor.Tool, editor.Style) {
	packed := uint32(wParam)
	style := editor.DefaultStyle()
	style.Color.R = byte(packed >> 8)
	style.Color.G = byte(packed >> 16)
	style.Color.B = byte(packed >> 24)
	style.Width = float64(math.Float32frombits(uint32(lParam)))
	return editor.Tool(byte(packed)), style
}

func (state *frozenState) applyActiveToolStyle(expected editor.Tool, style editor.Style) {
	request := state.activeRequest()
	if state.document != nil {
		if annotation, ok := state.document.Get(state.selected); ok {
			if annotation.Style != style {
				annotation.Style = style
				if err := state.document.Replace(annotation.ID, annotation); err != nil {
					state.renderErr = err
					return
				}
				state.notifySelectedStyle(style)
				if state.hwnd != 0 {
					state.renderOrCloseFrozen()
				}
			}
			if request != nil && request.tool == expected {
				request.style = style
			}
			return
		}
	}
	if request != nil && request.tool == expected {
		request.style = style
		if state.textEdit != nil && state.textEdit.id == 0 {
			state.textEdit.style = style
		}
		return
	}
	state.toolStyleWait = true
	state.toolStyleFor = expected
	state.toolStyle = style
}

func (state *frozenState) applyPendingToolStyle() {
	if !state.toolStyleWait {
		return
	}
	expected, style := state.toolStyleFor, state.toolStyle
	state.toolStyleWait = false
	request := state.activeRequest()
	if request == nil || request.tool != expected {
		return
	}
	request.style = style
	if state.textEdit != nil && state.textEdit.id == 0 {
		state.textEdit.style = style
	}
}

func (state *frozenState) beginDrawingGesture(request *frozenAnnotationRequest, point image.Point) {
	request.start = state.imagePoint(point)
	request.current = request.start
	request.pointerStart = point
	request.dragging = true
	request.drawing = false
}

func (state *frozenState) updateDrawingGesture(request *frozenAnnotationRequest, point image.Point) bool {
	if request == nil {
		return false
	}
	request.current = state.imagePoint(point)
	if !request.drawing {
		threshold := state.drawingDragThreshold()
		request.drawing = abs(point.X-request.pointerStart.X) >= threshold || abs(point.Y-request.pointerStart.Y) >= threshold
	}
	return request.drawing
}

func (state *frozenState) drawingDragThreshold() int {
	return max(3, scaleForDPI(4, state.dpi))
}

func (state *frozenState) resetDrawingGesture(request *frozenAnnotationRequest) {
	if request == nil {
		return
	}
	request.start = image.Point{}
	request.current = image.Point{}
	request.pointerStart = image.Point{}
	request.dragging = false
	request.drawing = false
	request.clearedSelection = false
	state.draftPixels = nil
	state.lastDraw = time.Time{}
	state.cancelAnimationFrame()
}

func (state *frozenState) clearSelectionForDrawing() (editor.Annotation, bool) {
	if state.selected == 0 {
		return editor.Annotation{}, false
	}
	annotation, ok := state.document.Get(state.selected)
	state.selected = 0
	return annotation, ok
}

func (state *frozenState) beginSelectedTransform(point image.Point) bool {
	if state.selected == 0 {
		return false
	}
	selected, ok := state.document.Get(state.selected)
	if !ok {
		state.selected = 0
		return false
	}
	if handle, hit := state.handleAt(point, selected); hit {
		state.beginTransform(selected, handle, point)
		return true
	}
	annotation, hit := state.document.HitTest(state.imagePoint(point), state.hitTolerance())
	if !hit || annotation.ID != selected.ID {
		return false
	}
	state.beginTransform(selected, editor.HandleMove, point)
	return true
}

func (state *frozenState) selectAnnotationAt(point image.Point) bool {
	annotation, ok := state.document.HitTest(state.imagePoint(point), state.hitTolerance())
	if !ok {
		return false
	}
	state.selected = annotation.ID
	if request := state.activeRequest(); request != nil {
		request.style = annotation.Style
	}
	state.notifySelectedStyle(annotation.Style)
	return true
}

func (state *frozenState) beginSelectionDrag(point image.Point) {
	if state.beginSelectedTransform(point) {
		return
	}
	annotation, ok := state.document.HitTest(state.imagePoint(point), state.hitTolerance())
	if !ok {
		if state.selected != 0 {
			state.selected = 0
			state.renderOrCloseFrozen()
		}
		return
	}
	state.selected = annotation.ID
	state.notifySelectedStyle(annotation.Style)
	handle := editor.HandleMove
	if candidate, hit := state.handleAt(point, annotation); hit {
		handle = candidate
	}
	state.beginTransform(annotation, handle, point)
}

func (state *frozenState) beginTransform(annotation editor.Annotation, handle editor.TransformHandle, point image.Point) {
	state.cancelAnimationFrame()
	state.transform = &frozenTransform{
		id: annotation.ID, handle: handle, original: annotation, draft: annotation,
		previous: annotation, anchor: state.imagePoint(point),
	}
	state.lastDraw = time.Time{}
	cursorID := idcSizeAll
	for _, candidate := range state.handles(annotation) {
		if candidate.kind == handle {
			cursorID = candidate.cursor
			break
		}
	}
	if cursor, _, _ := procLoadCursor.Call(0, uintptr(cursorID)); cursor != 0 {
		procFrozenSetCursor.Call(cursor)
	}
	procSetCapture.Call(state.hwnd)
}

func (state *frozenState) updateTransform(point image.Point) {
	transform := state.transform
	if transform == nil {
		return
	}
	current := state.imagePoint(point)
	draft := transform.original
	if transform.handle == editor.HandleMove {
		draft = editor.Translate(transform.original, current.Sub(transform.anchor), state.document.Bounds())
	} else {
		draft = editor.TransformTo(transform.original, transform.handle, current, state.document.Bounds())
	}
	if sameAnnotationGeometry(draft, transform.draft) {
		return
	}
	transform.draft = draft
	state.requestAnimationFrame()
}

func (state *frozenState) commitTransform() {
	transform := state.transform
	if transform == nil {
		return
	}
	state.cancelAnimationFrame()
	procReleaseCapture.Call()
	if sameAnnotationGeometry(transform.original, transform.draft) || (transform.draft.Tool != editor.ToolText && transform.draft.Start == transform.draft.End) {
		state.transform = nil
		state.renderOrCloseFrozen()
		return
	}
	// A timer may have coalesced the last pointer movement. Paint that newest
	// geometry before committing, then retain the already-correct vector frame
	// instead of synchronously rebuilding the whole desktop on mouse-up.
	fast := transform.fastPrepared
	if fast && !sameAnnotationGeometry(transform.previous, transform.draft) {
		if err := state.renderFastVectorTransform(transform); err != nil {
			state.renderErr = err
			procDestroyWindow.Call(state.hwnd)
			return
		}
	}
	state.transform = nil
	if err := state.document.Replace(transform.id, transform.draft); err != nil {
		state.renderErr = err
	}
	if fast {
		return
	}
	state.renderOrCloseFrozen()
}

func (state *frozenState) cancelTransform() {
	if state.transform == nil {
		return
	}
	state.transform = nil
	state.cancelAnimationFrame()
	procReleaseCapture.Call()
	state.renderOrCloseFrozen()
}

func sameAnnotationGeometry(left, right editor.Annotation) bool {
	return left.ID == right.ID && left.Tool == right.Tool && left.Start == right.Start && left.End == right.End && left.Text == right.Text && left.Style == right.Style
}

type frozenHandle struct {
	kind   editor.TransformHandle
	point  image.Point
	cursor int
}

func (state *frozenState) handles(annotation editor.Annotation) []frozenHandle {
	switch annotation.Tool {
	case editor.ToolArrow:
		return []frozenHandle{
			{editor.HandleArrowStart, state.viewport.ImageToScreen(annotation.Start), idcCross},
			{editor.HandleArrowEnd, state.viewport.ImageToScreen(annotation.End), idcCross},
		}
	case editor.ToolRectangle:
		start := state.viewport.ImageToScreen(annotation.Start)
		end := state.viewport.ImageToScreen(annotation.End)
		left, right := min(start.X, end.X), max(start.X, end.X)
		top, bottom := min(start.Y, end.Y), max(start.Y, end.Y)
		middleX, middleY := (left+right)/2, (top+bottom)/2
		return []frozenHandle{
			{editor.HandleRectangleNorthWest, image.Pt(left, top), idcSizeNWSE},
			{editor.HandleRectangleNorth, image.Pt(middleX, top), idcSizeNS},
			{editor.HandleRectangleNorthEast, image.Pt(right, top), idcSizeNESW},
			{editor.HandleRectangleEast, image.Pt(right, middleY), idcSizeWE},
			{editor.HandleRectangleSouthEast, image.Pt(right, bottom), idcSizeNWSE},
			{editor.HandleRectangleSouth, image.Pt(middleX, bottom), idcSizeNS},
			{editor.HandleRectangleSouthWest, image.Pt(left, bottom), idcSizeNESW},
			{editor.HandleRectangleWest, image.Pt(left, middleY), idcSizeWE},
		}
	default:
		return nil
	}
}

func (state *frozenState) handleAt(point image.Point, annotation editor.Annotation) (editor.TransformHandle, bool) {
	radius := state.handleRadius() + scaleForDPI(3, state.dpi)
	for _, handle := range state.handles(annotation) {
		if abs(point.X-handle.point.X) <= radius && abs(point.Y-handle.point.Y) <= radius {
			return handle.kind, true
		}
	}
	return editor.HandleMove, false
}

func (state *frozenState) handleRadius() int { return max(4, scaleForDPI(5, state.dpi)) }

func (state *frozenState) hitTolerance() float64 {
	scale := state.viewport.Scale
	if scale <= 0 {
		scale = 1
	}
	return float64(max(6, scaleForDPI(8, state.dpi))) / scale
}

func (state *frozenState) updateCursor(point image.Point) {
	cursorID := idcArrow
	if state.selected != 0 {
		if selected, ok := state.document.Get(state.selected); ok {
			for _, handle := range state.handles(selected) {
				radius := state.handleRadius() + scaleForDPI(3, state.dpi)
				if abs(point.X-handle.point.X) <= radius && abs(point.Y-handle.point.Y) <= radius {
					cursorID = handle.cursor
					break
				}
			}
			if cursorID == idcArrow {
				if annotation, hit := state.document.HitTest(state.imagePoint(point), state.hitTolerance()); hit && annotation.ID == selected.ID {
					if selected.Tool == editor.ToolText {
						cursorID = idcIBeam
					} else {
						cursorID = idcSizeAll
					}
				}
			}
		}
	}
	if cursorID == idcArrow && state.activeRequest() != nil {
		cursorID = idcCross
	}
	if cursorID == idcArrow {
		if annotation, ok := state.document.HitTest(state.imagePoint(point), state.hitTolerance()); ok {
			if annotation.Tool == editor.ToolText {
				cursorID = idcIBeam
			} else {
				cursorID = idcSizeAll
			}
		}
	}
	if cursor, _, _ := procLoadCursor.Call(0, uintptr(cursorID)); cursor != 0 {
		procFrozenSetCursor.Call(cursor)
	}
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func (state *frozenState) beginTextEdit(id editor.AnnotationID, start image.Point, text string, style editor.Style) error {
	if state.textEdit != nil {
		return fmt.Errorf("another text edit is active")
	}
	characters, err := syscall.UTF16PtrFromString(text)
	if err != nil {
		return err
	}
	className, _ := syscall.UTF16PtrFromString("EDIT")
	fontHeight := max(14, int(math.Round(style.Width))*8)
	screenFontHeight := max(14, int(math.Round(float64(fontHeight)*state.viewport.Scale)))
	minimumWidth := scaleForDPI(160, state.dpi)
	textWidth := minimumWidth
	if id != 0 {
		if annotation, ok := state.document.Get(id); ok {
			bounds := editor.AnnotationBounds(annotation)
			textWidth = max(textWidth, abs(state.viewport.ImageToScreen(bounds.Max).X-state.viewport.ImageToScreen(bounds.Min).X)+scaleForDPI(24, state.dpi))
		}
	}
	height := max(scaleForDPI(28, state.dpi), screenFontHeight+scaleForDPI(10, state.dpi))
	local := state.viewport.ImageToScreen(start)
	regionScreen := state.region.Add(state.desktop.Min)
	width := min(textWidth, regionScreen.Dx())
	height = min(height, regionScreen.Dy())
	x := max(regionScreen.Min.X, min(local.X+state.desktop.Min.X, regionScreen.Max.X-width))
	y := max(regionScreen.Min.Y, min(local.Y+state.desktop.Min.Y, regionScreen.Max.Y-height))
	edit, _, callErr := procCreateWindowEx.Call(
		wsExTopmost|wsExToolWindow, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(characters)),
		wsPopup|wsBorder|wsVisible|wsTabStop|esAutoHScroll,
		uintptr(x), uintptr(y), uintptr(width), uintptr(height), state.hwnd, 0, 0, 0,
	)
	if edit == 0 {
		return win32Error("CreateWindowExW text editor", callErr)
	}
	face, _ := syscall.UTF16PtrFromString("Microsoft YaHei UI")
	font, _, _ := procFrozenCreateFont.Call(uintptr(-screenFontHeight), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(face)))
	textEdit := &frozenTextEdit{hwnd: edit, font: font, id: id, style: style, start: start}
	state.textEdit = textEdit
	frozenTextStates.Store(edit, state)
	oldProcedure, _, callErr := procFrozenSetWindowLongPtr.Call(edit, ^uintptr(3), frozenTextProcedure)
	if oldProcedure == 0 {
		frozenTextStates.Delete(edit)
		state.textEdit = nil
		procDestroyWindow.Call(edit)
		if font != 0 {
			procDeleteObject.Call(font)
		}
		return win32Error("SetWindowLongPtrW text editor", callErr)
	}
	textEdit.oldProcedure = oldProcedure
	if font != 0 {
		procSendMessage.Call(edit, wmSetFont, font, 1)
	}
	procSendMessage.Call(edit, emSetSel, 0, ^uintptr(0))
	procSetForegroundWindow.Call(edit)
	procSetFocus.Call(edit)
	return nil
}

func frozenTextWindowProcedure(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	value, found := frozenTextStates.Load(hwnd)
	if !found {
		result, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
		return result
	}
	state := value.(*frozenState)
	textEdit := state.textEdit
	if textEdit == nil || textEdit.hwnd != hwnd {
		return 0
	}
	switch message {
	case wmKeyDown:
		if wParam == vkReturn {
			procPostMessage.Call(state.hwnd, wmFrozenTextSave, 0, 0)
			return 0
		}
		if wParam == vkEscape {
			procPostMessage.Call(state.hwnd, wmFrozenTextDrop, 0, 0)
			return 0
		}
	case wmKillFocus:
		if !textEdit.closing {
			state.finishTextEdit(true, false, false)
			return 0
		}
	}
	result, _, _ := procFrozenCallWindowProc.Call(textEdit.oldProcedure, hwnd, uintptr(message), wParam, lParam)
	return result
}

func (state *frozenState) finishTextEdit(commit, restoreFocus, endTool bool) {
	textEdit := state.textEdit
	if textEdit == nil || textEdit.closing {
		return
	}
	textEdit.closing = true
	text := ""
	if commit {
		length, _, _ := procFrozenGetWindowTextLen.Call(textEdit.hwnd)
		buffer := make([]uint16, int(length)+1)
		if len(buffer) > 0 {
			procFrozenGetWindowText.Call(textEdit.hwnd, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
			text = syscall.UTF16ToString(buffer)
		}
	}
	state.disposeTextEditor()
	var editErr error
	if commit {
		if textEdit.id == 0 {
			if text != "" {
				state.selected, editErr = state.document.Add(editor.Annotation{Tool: editor.ToolText, Start: textEdit.start, Text: text, Style: textEdit.style})
			}
		} else if text == "" {
			state.document.Delete(textEdit.id)
			state.selected = 0
		} else if annotation, ok := state.document.Get(textEdit.id); ok && annotation.Text != text {
			annotation.Text = text
			editErr = state.document.Replace(textEdit.id, annotation)
		}
	}
	if editErr == nil {
		state.notifyCurrentSelectionStyle()
	}
	if restoreFocus {
		procSetForegroundWindow.Call(state.hwnd)
		procSetFocus.Call(state.hwnd)
	}
	if textEdit.id == 0 {
		state.completeNewTextPlacement(editErr, endTool)
	} else {
		if editErr != nil {
			state.renderErr = editErr
		}
		state.renderOrCloseFrozen()
	}
}

func (state *frozenState) completeNewTextPlacement(editErr error, endTool bool) {
	if editErr != nil || endTool {
		state.finishRequest(editErr)
	} else if state.hwnd != 0 {
		state.renderOrCloseFrozen()
	}
}

func (state *frozenState) disposeTextEditor() {
	textEdit := state.textEdit
	if textEdit == nil {
		return
	}
	textEdit.closing = true
	frozenTextStates.Delete(textEdit.hwnd)
	if textEdit.oldProcedure != 0 {
		procFrozenSetWindowLongPtr.Call(textEdit.hwnd, ^uintptr(3), textEdit.oldProcedure)
	}
	if textEdit.hwnd != 0 {
		procDestroyWindow.Call(textEdit.hwnd)
	}
	if textEdit.font != 0 {
		procDeleteObject.Call(textEdit.font)
	}
	state.textEdit = nil
}

func powZoom(delta int) float64 {
	result := 1.0
	steps := delta / 120
	if steps > 0 {
		for range steps {
			result *= 1.1
		}
	} else {
		for range -steps {
			result /= 1.1
		}
	}
	return result
}
func (state *frozenState) activeRequest() *frozenAnnotationRequest {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.request
}

// continueOrFinishDrawingRequest resets one completed rectangle or arrow
// placement while keeping the selected tool active. A tool switch requested
// during mouse capture takes effect immediately after that placement ends.
func (state *frozenState) continueOrFinishDrawingRequest(request *frozenAnnotationRequest) {
	state.resetDrawingGesture(request)
	if state.toolSwapWait && state.toolSwapFrom == request.tool {
		state.toolSwapWait = false
		state.finishRequest(nil)
	}
}

func (state *frozenState) finishRequest(err error) {
	procReleaseCapture.Call()
	state.mu.Lock()
	request := state.request
	state.request = nil
	state.mu.Unlock()
	if request != nil {
		request.result <- err
		procPostMessage.Call(state.hwnd, wmFrozenRender, 0, 0)
	}
}
func (state *frozenState) imagePoint(point image.Point) image.Point {
	return clampPoint(state.document.Bounds(), state.viewport.ScreenToImage(point))
}
func (state *frozenState) renderOrCloseFrozen() {
	if err := state.renderFrozenDesktop(); err != nil {
		state.renderErr = err
		procDestroyWindow.Call(state.hwnd)
	}
}

// requestAnimationFrame paints immediately when a frame is due. Otherwise it
// keeps the newest pointer geometry and arms a one-shot timer for the remaining
// part of the 60 Hz interval. Merely dropping early WM_MOUSEMOVE messages makes
// the effective rate depend on mouse-message timing and can fall well below
// 60 FPS.
func (state *frozenState) requestAnimationFrame() {
	now := time.Now()
	if state.lastDraw.IsZero() || now.Sub(state.lastDraw) >= frozenFrameInterval {
		state.cancelAnimationFrame()
		state.renderAnimationFrame()
		return
	}
	state.framePending = true
	if state.frameTimerSet {
		return
	}
	remaining := frozenFrameInterval - now.Sub(state.lastDraw)
	delayMilliseconds := max(1, int((remaining+time.Millisecond-1)/time.Millisecond))
	if timer, _, _ := procFrozenSetTimer.Call(state.hwnd, frozenFrameTimer, uintptr(delayMilliseconds), 0); timer != 0 {
		state.frameTimerSet = true
		return
	}
	// SetTimer failure is rare; drawing now is preferable to leaving the pointer
	// preview stale until another mouse message happens to arrive.
	state.framePending = false
	state.renderAnimationFrame()
}

func (state *frozenState) onAnimationFrame() {
	if state.frameTimerSet {
		procFrozenKillTimer.Call(state.hwnd, frozenFrameTimer)
		state.frameTimerSet = false
	}
	if !state.framePending {
		return
	}
	state.framePending = false
	state.renderAnimationFrame()
}

func (state *frozenState) cancelAnimationFrame() {
	if state.frameTimerSet && state.hwnd != 0 {
		procFrozenKillTimer.Call(state.hwnd, frozenFrameTimer)
	}
	state.frameTimerSet = false
	state.framePending = false
}

func (state *frozenState) renderAnimationFrame() {
	request := state.activeRequest()
	if request != nil && request.dragging && request.drawing && request.tool != editor.ToolText {
		state.lastDraw = time.Now()
		state.renderDraftOrClose(request)
		return
	}
	if state.transform != nil {
		state.lastDraw = time.Now()
		state.renderTransformOrClose(state.transform)
	}
}

func (state *frozenState) renderTransformOrClose(transform *frozenTransform) {
	if err := state.renderTransform(transform); err != nil {
		state.renderErr = err
		procDestroyWindow.Call(state.hwnd)
	} else if transform != nil {
		transform.previous = transform.draft
	}
}

func (state *frozenState) renderTransform(transform *frozenTransform) error {
	if transform == nil {
		return nil
	}
	if transform.draft.Tool == editor.ToolArrow || transform.draft.Tool == editor.ToolRectangle {
		return state.renderFastVectorTransform(transform)
	}
	dirty := state.selectionScreenBounds(transform.previous).Union(state.selectionScreenBounds(transform.draft)).Intersect(state.region)
	if dirty.Empty() {
		return nil
	}
	rendered := state.document.RenderedPreview(transform.id, &transform.draft)
	view := editor.RenderViewport(rendered, state.viewport, dirty)
	state.copyViewport(view, dirty)
	state.drawSelectionOverlay(transform.draft)
	drawOuterPixelBorder(state.pixels, state.client.Dx(), state.client, state.region)
	return state.present()
}

// renderFastVectorTransform keeps the transform cost proportional to the
// stroked vector length. The generic preview renderer scans every pixel in the
// old/new bounding rectangle, which is especially expensive for long diagonal
// arrows and is unnecessary because almost all of that rectangle is empty.
func (state *frozenState) renderFastVectorTransform(transform *frozenTransform) error {
	if !transform.fastPrepared {
		state.clearVectorTransformBase(transform.original)
		transform.fastPrepared = true
	}
	state.restoreDraftPixels()
	state.drawFastVector(transform.draft)
	state.drawTrackedSelectionOverlay(transform.draft)
	drawOuterPixelBorder(state.pixels, state.client.Dx(), state.client, state.region)
	return state.present()
}

func (state *frozenState) clearVectorTransformBase(annotation editor.Annotation) {
	state.restoreVectorSelectionPixels(annotation, state.document.RenderedWithout(annotation.ID))
	state.draftPixels = nil
}

// clearSelectionOverlay removes only the editing chrome for the previous
// selection. The document pixels underneath are already stable, so rebuilding
// the entire frozen desktop here would make every new placement pay for a
// full-screen render before drawing can begin.
func (state *frozenState) clearSelectionOverlay(annotation editor.Annotation) error {
	if annotation.Tool == editor.ToolArrow || annotation.Tool == editor.ToolRectangle {
		state.restoreVectorSelectionPixels(annotation, state.document.Rendered())
	} else {
		dirty := state.selectionScreenBounds(annotation).Intersect(state.region)
		if !dirty.Empty() {
			view := editor.RenderViewport(state.document.Rendered(), state.viewport, dirty)
			state.copyViewport(view, dirty)
		}
	}
	drawOuterPixelBorder(state.pixels, state.client.Dx(), state.client, state.region)
	return nil
}

func (state *frozenState) restoreVectorSelectionPixels(annotation editor.Annotation, base image.Image) {
	seen := make(map[int]struct{})
	clearLine := func(start, end image.Point, thickness float64) {
		radius := math.Max(.5, thickness/2) + 2
		padding := int(math.Ceil(radius + .5))
		for y := min(start.Y, end.Y) - padding; y <= max(start.Y, end.Y)+padding; y++ {
			for x := min(start.X, end.X) - padding; x <= max(start.X, end.X)+padding; x++ {
				point := image.Pt(x, y)
				if draftDistanceToSegment(point, start, end) <= radius+.5 {
					state.restoreVectorBasePixel(base, point, seen)
				}
			}
		}
	}
	start := state.viewport.ImageToScreen(annotation.Start)
	end := state.viewport.ImageToScreen(annotation.End)
	thickness := math.Max(1, annotation.Style.Width*state.viewport.Scale)
	if annotation.Tool == editor.ToolRectangle {
		left, right := min(start.X, end.X), max(start.X, end.X)
		top, bottom := min(start.Y, end.Y), max(start.Y, end.Y)
		clearLine(image.Pt(left, top), image.Pt(right, top), thickness)
		clearLine(image.Pt(right, top), image.Pt(right, bottom), thickness)
		clearLine(image.Pt(right, bottom), image.Pt(left, bottom), thickness)
		clearLine(image.Pt(left, bottom), image.Pt(left, top), thickness)
	} else {
		clearLine(start, end, thickness)
		if left, right, ok := screenArrowHead(start, end, annotation.Style.Width, state.viewport.Scale); ok {
			clearLine(end, left, thickness)
			clearLine(end, right, thickness)
		}
	}
	for _, handle := range state.handles(annotation) {
		radius := state.handleRadius() + 1
		for y := handle.point.Y - radius; y <= handle.point.Y+radius; y++ {
			for x := handle.point.X - radius; x <= handle.point.X+radius; x++ {
				state.restoreVectorBasePixel(base, image.Pt(x, y), seen)
			}
		}
	}
}

func (state *frozenState) restoreVectorBasePixel(base image.Image, point image.Point, seen map[int]struct{}) {
	if !point.In(state.region) {
		return
	}
	index := (point.Y*state.client.Dx() + point.X) * 4
	if _, exists := seen[index]; exists {
		return
	}
	seen[index] = struct{}{}
	imagePoint := state.viewport.ScreenToImage(point)
	value := color.NRGBA{}
	if imagePoint.In(base.Bounds()) {
		value = color.NRGBAModel.Convert(base.At(imagePoint.X, imagePoint.Y)).(color.NRGBA)
	}
	state.pixels[index], state.pixels[index+1], state.pixels[index+2], state.pixels[index+3] = value.B, value.G, value.R, 255
}

func (state *frozenState) drawFastVector(annotation editor.Annotation) {
	start := state.viewport.ImageToScreen(annotation.Start)
	end := state.viewport.ImageToScreen(annotation.End)
	thickness := math.Max(1, annotation.Style.Width*state.viewport.Scale)
	seen := make(map[int]int)
	if annotation.Tool == editor.ToolRectangle {
		left, right := min(start.X, end.X), max(start.X, end.X)
		top, bottom := min(start.Y, end.Y), max(start.Y, end.Y)
		state.drawDraftLine(image.Pt(left, top), image.Pt(right, top), thickness, annotation.Style, seen)
		state.drawDraftLine(image.Pt(right, top), image.Pt(right, bottom), thickness, annotation.Style, seen)
		state.drawDraftLine(image.Pt(right, bottom), image.Pt(left, bottom), thickness, annotation.Style, seen)
		state.drawDraftLine(image.Pt(left, bottom), image.Pt(left, top), thickness, annotation.Style, seen)
		return
	}
	state.drawDraftLine(start, end, thickness, annotation.Style, seen)
	if left, right, ok := screenArrowHead(start, end, annotation.Style.Width, state.viewport.Scale); ok {
		state.drawDraftLine(end, left, thickness, annotation.Style, seen)
		state.drawDraftLine(end, right, thickness, annotation.Style, seen)
	}
}

func screenArrowHead(start, end image.Point, width, scale float64) (image.Point, image.Point, bool) {
	dx, dy := float64(end.X-start.X), float64(end.Y-start.Y)
	length := math.Hypot(dx, dy)
	if length <= 0 {
		return image.Point{}, image.Point{}, false
	}
	head := math.Min((18+width*2)*scale, length*.45)
	ux, uy := dx/length, dy/length
	left := image.Pt(int(math.Round(float64(end.X)-ux*head-uy*head*.55)), int(math.Round(float64(end.Y)-uy*head+ux*head*.55)))
	right := image.Pt(int(math.Round(float64(end.X)-ux*head+uy*head*.55)), int(math.Round(float64(end.Y)-uy*head-ux*head*.55)))
	return left, right, true
}

func (state *frozenState) drawTrackedSelectionOverlay(annotation editor.Annotation) {
	seen := make(map[int]struct{}, len(state.draftPixels))
	for _, pixel := range state.draftPixels {
		seen[pixel.index] = struct{}{}
	}
	track := func(point image.Point) {
		if !point.In(state.region) {
			return
		}
		index := (point.Y*state.client.Dx() + point.X) * 4
		if _, exists := seen[index]; exists {
			return
		}
		seen[index] = struct{}{}
		state.draftPixels = append(state.draftPixels, frozenDraftPixel{
			index: index,
			bgra:  [4]byte{state.pixels[index], state.pixels[index+1], state.pixels[index+2], state.pixels[index+3]},
		})
	}
	if annotation.Tool == editor.ToolRectangle {
		start := state.viewport.ImageToScreen(annotation.Start)
		end := state.viewport.ImageToScreen(annotation.End)
		left, right := min(start.X, end.X), max(start.X, end.X)
		top, bottom := min(start.Y, end.Y), max(start.Y, end.Y)
		trackOverlayLine(image.Pt(left, top), image.Pt(right, top), track)
		trackOverlayLine(image.Pt(right, top), image.Pt(right, bottom), track)
		trackOverlayLine(image.Pt(right, bottom), image.Pt(left, bottom), track)
		trackOverlayLine(image.Pt(left, bottom), image.Pt(left, top), track)
	}
	for _, handle := range state.handles(annotation) {
		radius := state.handleRadius()
		for y := handle.point.Y - radius; y <= handle.point.Y+radius; y++ {
			for x := handle.point.X - radius; x <= handle.point.X+radius; x++ {
				track(image.Pt(x, y))
			}
		}
	}
	state.drawSelectionOverlay(annotation)
}

func trackOverlayLine(start, end image.Point, track func(image.Point)) {
	dx, dy := abs(end.X-start.X), abs(end.Y-start.Y)
	sx, sy := -1, -1
	if start.X < end.X {
		sx = 1
	}
	if start.Y < end.Y {
		sy = 1
	}
	err := dx - dy
	for {
		track(start)
		if start == end {
			return
		}
		twice := 2 * err
		if twice > -dy {
			err -= dy
			start.X += sx
		}
		if twice < dx {
			err += dx
			start.Y += sy
		}
	}
}

func (state *frozenState) renderDraftOrClose(request *frozenAnnotationRequest) {
	if err := state.renderDraft(request); err != nil {
		state.renderErr = err
		procDestroyWindow.Call(state.hwnd)
	}
}

// renderDraft restores and redraws only pixels along the temporary vector.
// This keeps diagonal arrows O(length*width), rather than rasterizing their
// potentially screen-sized axis-aligned bounding rectangle.
func (state *frozenState) renderDraft(request *frozenAnnotationRequest) error {
	if request == nil || !request.dragging || !request.drawing || request.current == request.start {
		return nil
	}
	state.restoreDraftPixels()
	start := state.viewport.ImageToScreen(request.start)
	end := state.viewport.ImageToScreen(request.current)
	thickness := math.Max(1, request.style.Width*state.viewport.Scale)
	seen := make(map[int]int)
	switch request.tool {
	case editor.ToolRectangle:
		left, right := min(start.X, end.X), max(start.X, end.X)
		top, bottom := min(start.Y, end.Y), max(start.Y, end.Y)
		state.drawDraftLine(image.Pt(left, top), image.Pt(right, top), thickness, request.style, seen)
		state.drawDraftLine(image.Pt(right, top), image.Pt(right, bottom), thickness, request.style, seen)
		state.drawDraftLine(image.Pt(right, bottom), image.Pt(left, bottom), thickness, request.style, seen)
		state.drawDraftLine(image.Pt(left, bottom), image.Pt(left, top), thickness, request.style, seen)
	case editor.ToolArrow:
		state.drawDraftLine(start, end, thickness, request.style, seen)
		if left, right, ok := screenArrowHead(start, end, request.style.Width, state.viewport.Scale); ok {
			state.drawDraftLine(end, left, thickness, request.style, seen)
			state.drawDraftLine(end, right, thickness, request.style, seen)
		}
	}
	return state.present()
}

func (state *frozenState) restoreDraftPixels() {
	for _, pixel := range state.draftPixels {
		copy(state.pixels[pixel.index:pixel.index+4], pixel.bgra[:])
	}
	state.draftPixels = state.draftPixels[:0]
}

func (state *frozenState) drawDraftLine(start, end image.Point, thickness float64, style editor.Style, seen map[int]int) {
	radius := math.Max(.5, thickness/2)
	padding := int(math.Ceil(radius + .5))
	left, right := min(start.X, end.X)-padding, max(start.X, end.X)+padding
	top, bottom := min(start.Y, end.Y)-padding, max(start.Y, end.Y)+padding
	for y := top; y <= bottom; y++ {
		for x := left; x <= right; x++ {
			if !image.Pt(x, y).In(state.region) {
				continue
			}
			coverage := math.Max(0, math.Min(1, radius+.5-draftDistanceToSegment(image.Pt(x, y), start, end)))
			if coverage <= 0 {
				continue
			}
			index := (y*state.client.Dx() + x) * 4
			pixelIndex, exists := seen[index]
			if exists && coverage <= state.draftPixels[pixelIndex].coverage {
				continue
			}
			if !exists {
				pixelIndex = len(state.draftPixels)
				seen[index] = pixelIndex
				state.draftPixels = append(state.draftPixels, frozenDraftPixel{index: index, bgra: [4]byte{state.pixels[index], state.pixels[index+1], state.pixels[index+2], state.pixels[index+3]}})
			}
			pixel := &state.draftPixels[pixelIndex]
			pixel.coverage = coverage
			alpha := uint32(math.Round(float64(style.Color.A) * coverage))
			inverse := uint32(255) - alpha
			state.pixels[index] = byte((uint32(style.Color.B)*alpha + uint32(pixel.bgra[0])*inverse) / 255)
			state.pixels[index+1] = byte((uint32(style.Color.G)*alpha + uint32(pixel.bgra[1])*inverse) / 255)
			state.pixels[index+2] = byte((uint32(style.Color.R)*alpha + uint32(pixel.bgra[2])*inverse) / 255)
			state.pixels[index+3] = 255
		}
	}
}

func draftDistanceToSegment(point, start, end image.Point) float64 {
	px, py := float64(point.X), float64(point.Y)
	ax, ay := float64(start.X), float64(start.Y)
	dx, dy := float64(end.X-start.X), float64(end.Y-start.Y)
	if dx == 0 && dy == 0 {
		return math.Hypot(px-ax, py-ay)
	}
	t := ((px-ax)*dx + (py-ay)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy))
}

func (state *frozenState) renderFrozenDesktop() error {
	if err := copyImageToBGRA(state.pixels, state.client.Dx(), state.client.Dy(), state.source); err != nil {
		return err
	}
	rendered := state.document.Rendered()
	request := state.activeRequest()
	if state.transform != nil {
		rendered = state.document.RenderedPreview(state.transform.id, &state.transform.draft)
	} else if request != nil && request.dragging && request.drawing && request.current != request.start {
		annotation := editor.Annotation{Tool: request.tool, Start: request.start, End: request.current, Style: request.style}
		rendered = state.document.RenderedPreview(0, &annotation)
	}
	view := editor.RenderViewport(rendered, state.viewport, state.region)
	state.copyViewport(view, state.region)
	state.draftPixels = nil
	if state.transform != nil {
		state.drawSelectionOverlay(state.transform.draft)
	} else if state.selected != 0 {
		if annotation, ok := state.document.Get(state.selected); ok {
			state.drawSelectionOverlay(annotation)
		}
	}
	drawOuterPixelBorder(state.pixels, state.client.Dx(), state.client, state.region)
	return state.present()
}

func (state *frozenState) selectionScreenBounds(annotation editor.Annotation) image.Rectangle {
	bounds := editor.AnnotationBounds(annotation)
	minimum := state.viewport.ImageToScreen(bounds.Min)
	maximum := state.viewport.ImageToScreen(bounds.Max)
	result := image.Rect(min(minimum.X, maximum.X), min(minimum.Y, maximum.Y), max(minimum.X, maximum.X)+1, max(minimum.Y, maximum.Y)+1)
	padding := state.handleRadius() + scaleForDPI(4, state.dpi)
	if annotation.Tool == editor.ToolArrow {
		padding += max(4, int(math.Round((18+annotation.Style.Width*2)*state.viewport.Scale)))
	}
	return image.Rect(result.Min.X-padding, result.Min.Y-padding, result.Max.X+padding, result.Max.Y+padding)
}

func (state *frozenState) drawSelectionOverlay(annotation editor.Annotation) {
	const blueR, blueG, blueB = 35, 145, 255
	switch annotation.Tool {
	case editor.ToolRectangle:
		start := state.viewport.ImageToScreen(annotation.Start)
		end := state.viewport.ImageToScreen(annotation.End)
		bounds := image.Rect(min(start.X, end.X), min(start.Y, end.Y), max(start.X, end.X), max(start.Y, end.Y))
		state.drawOverlayRectangle(bounds, blueR, blueG, blueB)
		for _, handle := range state.handles(annotation) {
			state.drawSquareHandle(handle.point, blueR, blueG, blueB)
		}
	case editor.ToolArrow:
		for _, handle := range state.handles(annotation) {
			state.drawRoundHandle(handle.point, blueR, blueG, blueB)
		}
	case editor.ToolText:
		bounds := editor.AnnotationBounds(annotation)
		start := state.viewport.ImageToScreen(bounds.Min)
		end := state.viewport.ImageToScreen(bounds.Max)
		state.drawOverlayRectangle(image.Rect(min(start.X, end.X), min(start.Y, end.Y), max(start.X, end.X), max(start.Y, end.Y)), blueR, blueG, blueB)
	}
}

func (state *frozenState) drawOverlayRectangle(bounds image.Rectangle, red, green, blue byte) {
	state.drawOverlayLine(image.Pt(bounds.Min.X, bounds.Min.Y), image.Pt(bounds.Max.X, bounds.Min.Y), red, green, blue)
	state.drawOverlayLine(image.Pt(bounds.Max.X, bounds.Min.Y), image.Pt(bounds.Max.X, bounds.Max.Y), red, green, blue)
	state.drawOverlayLine(image.Pt(bounds.Max.X, bounds.Max.Y), image.Pt(bounds.Min.X, bounds.Max.Y), red, green, blue)
	state.drawOverlayLine(image.Pt(bounds.Min.X, bounds.Max.Y), image.Pt(bounds.Min.X, bounds.Min.Y), red, green, blue)
}

func (state *frozenState) drawOverlayLine(start, end image.Point, red, green, blue byte) {
	dx, dy := abs(end.X-start.X), abs(end.Y-start.Y)
	sx, sy := -1, -1
	if start.X < end.X {
		sx = 1
	}
	if start.Y < end.Y {
		sy = 1
	}
	err := dx - dy
	for {
		state.setOverlayPixel(start, red, green, blue)
		if start == end {
			return
		}
		twice := 2 * err
		if twice > -dy {
			err -= dy
			start.X += sx
		}
		if twice < dx {
			err += dx
			start.Y += sy
		}
	}
}

func (state *frozenState) drawSquareHandle(center image.Point, red, green, blue byte) {
	radius := state.handleRadius()
	for y := center.Y - radius; y <= center.Y+radius; y++ {
		for x := center.X - radius; x <= center.X+radius; x++ {
			if x == center.X-radius || x == center.X+radius || y == center.Y-radius || y == center.Y+radius {
				state.setOverlayPixel(image.Pt(x, y), red, green, blue)
			} else {
				state.setOverlayPixel(image.Pt(x, y), 255, 255, 255)
			}
		}
	}
}

func (state *frozenState) drawRoundHandle(center image.Point, red, green, blue byte) {
	radius := state.handleRadius()
	inner := max(0, radius-2)
	for y := center.Y - radius; y <= center.Y+radius; y++ {
		for x := center.X - radius; x <= center.X+radius; x++ {
			distance := (x-center.X)*(x-center.X) + (y-center.Y)*(y-center.Y)
			if distance > radius*radius {
				continue
			}
			if distance >= inner*inner {
				state.setOverlayPixel(image.Pt(x, y), red, green, blue)
			} else {
				state.setOverlayPixel(image.Pt(x, y), 255, 255, 255)
			}
		}
	}
}

func (state *frozenState) setOverlayPixel(point image.Point, red, green, blue byte) {
	if !point.In(state.region) {
		return
	}
	index := (point.Y*state.client.Dx() + point.X) * 4
	state.pixels[index], state.pixels[index+1], state.pixels[index+2], state.pixels[index+3] = blue, green, red, 255
}

func (state *frozenState) copyViewport(view *image.NRGBA, destination image.Rectangle) {
	for y := 0; y < view.Bounds().Dy(); y++ {
		for x := 0; x < view.Bounds().Dx(); x++ {
			sourceIndex := y*view.Stride + x*4
			destinationIndex := ((destination.Min.Y+y)*state.client.Dx() + destination.Min.X + x) * 4
			state.pixels[destinationIndex] = view.Pix[sourceIndex+2]
			state.pixels[destinationIndex+1] = view.Pix[sourceIndex+1]
			state.pixels[destinationIndex+2] = view.Pix[sourceIndex]
			state.pixels[destinationIndex+3] = 255
		}
	}
}
