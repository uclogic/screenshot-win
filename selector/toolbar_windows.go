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
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"screenshot-win/editor"
	"screenshot-win/internal/ui/glass"
)

const (
	wmPaint        = 0x000F
	wmEraseBkgnd   = 0x0014
	wmMouseLeave   = 0x02A3
	wmUser         = 0x0400
	wmToolbarReady = wmUser + 97
	wmToolbarStyle = wmUser + 98
	wmToolbarPin   = wmUser + 102
	wmToolbarRaise = wmUser + 108
	wmActivate     = 0x0006

	monitorDefaultToNearest = 2
	colorWindow             = 5
	psSolid                 = 0
	tmeLeave                = 0x00000002

	iccWin95Classes   = 0x000000FF
	ttsAlwaysTip      = 0x01
	ttsNoPrefix       = 0x02
	ttfSubclass       = 0x0010
	ttdtAutomatic     = 0
	ttmActivate       = wmUser + 1
	ttmSetDelayTime   = wmUser + 3
	ttmAddToolW       = wmUser + 50
	ttmSetMaxTipWidth = wmUser + 24
	ttmUpdateTipTextW = wmUser + 57
	tooltipDelayMS    = 300

	hwndTopmost   = ^uintptr(0)
	swpNoSize     = 0x0001
	swpNoMove     = 0x0002
	swpNoActivate = 0x0010

	wmSetFont = 0x0030
)

var (
	comctl32                   = syscall.NewLazyDLL("comctl32.dll")
	procInitCommonControlsEx   = comctl32.NewProc("InitCommonControlsEx")
	procMonitorFromRect        = user32.NewProc("MonitorFromRect")
	procGetMonitorInfo         = user32.NewProc("GetMonitorInfoW")
	procGetDPIForWindow        = user32.NewProc("GetDpiForWindow")
	procSetWindowPos           = user32.NewProc("SetWindowPos")
	procInvalidateRect         = user32.NewProc("InvalidateRect")
	procTrackMouseEvent        = user32.NewProc("TrackMouseEvent")
	procBeginPaint             = user32.NewProc("BeginPaint")
	procEndPaint               = user32.NewProc("EndPaint")
	procFillRect               = user32.NewProc("FillRect")
	procSendMessage            = user32.NewProc("SendMessageW")
	procCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procBitBlt                 = gdi32.NewProc("BitBlt")
	procCreatePen              = gdi32.NewProc("CreatePen")
	procMoveToEx               = gdi32.NewProc("MoveToEx")
	procLineTo                 = gdi32.NewProc("LineTo")
	procEllipse                = gdi32.NewProc("Ellipse")
	procSetTextColor           = gdi32.NewProc("SetTextColor")
	procSetBkMode              = gdi32.NewProc("SetBkMode")
	procTextOut                = gdi32.NewProc("TextOutW")

	toolbarProcedure    = syscall.NewCallback(toolbarWindowProcedure)
	toolbarStates       sync.Map
	activeToolbarWindow atomic.Uintptr
)

type rect struct{ Left, Top, Right, Bottom int32 }

type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

type paintStruct struct {
	DC        uintptr
	Erase     int32
	Paint     rect
	Restore   int32
	IncUpdate int32
	Reserved  [32]byte
}

type trackMouseEvent struct {
	Size      uint32
	Flags     uint32
	Track     uintptr
	HoverTime uint32
}

type initCommonControlsEx struct {
	Size uint32
	ICC  uint32
}

type toolInfo struct {
	Size      uint32
	Flags     uint32
	Window    uintptr
	ID        uintptr
	Area      rect
	Instance  uintptr
	Text      *uint16
	Parameter uintptr
	Reserved  unsafe.Pointer
}

// Version 1 ends after lpszText and is understood by every supported
// ComCtl32 version. Passing the full structure size fails when the executable
// is bound to the pre-v6 common controls library.
const toolInfoV1Size = uint32(unsafe.Offsetof(toolInfo{}.Parameter))

type toolbarState struct {
	motions        []toolbarMotion
	motionClosed   bool
	background     ToolbarBackground
	glassWindow    glassWindow
	glassEnabled   bool
	pressed        bool
	pressedAction  Action
	region         image.Rectangle
	hwnd           uintptr
	action         Action
	chosen         bool
	hover          Action
	hovering       bool
	capture        bool
	clientSize     image.Point
	actions        []Action
	labels         []string
	tooltips       []*uint16
	shortcutTarget uintptr
	persistent     bool
	ready          bool
	active         bool
	activeAction   Action
	pending        bool
	pendingAction  Action
	events         chan<- ToolbarEvent
	renderErr      error
	style          editor.Style
	pendingStyle   editor.Style
	styleMu        sync.Mutex
	tooltip        uintptr
	instance       uintptr
	workArea       image.Rectangle
	windowBounds   image.Rectangle
	dpi            int
	panel          stylePanel
}

type stylePanel struct {
	glassWindow  glassWindow
	pressedIndex int
	hwnd         uintptr
	field        editor.StyleField
	bounds       image.Rectangle
	hoverIndex   int
	keyIndex     int
}

type actionToolbarStart struct {
	hwnd  uintptr
	state *toolbarState
	err   error
}

type captureToolbarStart struct {
	hwnd           uintptr
	hideForCapture bool
	err            error
}

// ShowToolbar displays a topmost post-selection action bar and blocks until an
// action is chosen or the user cancels it.
func ShowToolbar(region image.Rectangle) (Action, error) {
	toolbar, err := ShowToolbarContext(context.Background(), region, 0)
	if err != nil {
		return ActionCancel, err
	}
	defer toolbar.Close()
	return toolbar.NextAction(context.Background())
}

// ShowToolbarContext creates a persistent toolbar for a normal screenshot.
func ShowToolbarContext(ctx context.Context, region image.Rectangle, shortcutTarget uintptr, background ...ToolbarBackground) (*ActionToolbar, error) {
	return showToolbarContext(ctx, region, selectionToolbarActions, []string{"Cancel", "Scrolling capture", "Rectangle", "Arrow", "Text", "Color", "Line width", "Pin to desktop", "Save", "Copy"}, shortcutTarget, background...)
}

func ShowAnnotationToolbarContext(ctx context.Context, region image.Rectangle, shortcutTarget uintptr, background ...ToolbarBackground) (*ActionToolbar, error) {
	return showToolbarContext(ctx, region, annotationToolbarActions, []string{"Cancel", "Rectangle", "Arrow", "Text", "Color", "Line width", "Pin to desktop", "Save", "Copy"}, shortcutTarget, background...)
}

func showToolbarContext(ctx context.Context, region image.Rectangle, actions []Action, labels []string, shortcutTarget uintptr, background ...ToolbarBackground) (*ActionToolbar, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	started := make(chan actionToolbarStart, 1)
	done := make(chan struct{})
	events := make(chan ToolbarEvent, 1)
	var source ToolbarBackground
	if len(background) > 0 {
		source = background[0]
	}
	go runActionToolbarWindow(ctx, region, actions, labels, shortcutTarget, events, started, done, source)
	result := <-started
	if result.err != nil {
		<-done
		return nil, result.err
	}
	state := result.state
	return &ActionToolbar{
		window: result.hwnd, events: events, done: done,
		ready:       func() { procPostMessage.Call(result.hwnd, wmToolbarReady, 0, 0) },
		setStyle:    func(style editor.Style) { state.queueStyle(style) },
		closeWindow: func() { procPostMessage.Call(result.hwnd, wmClose, 0, 0) },
		resultError: func() error { return state.renderErr },
	}, nil
}

func runActionToolbarWindow(ctx context.Context, region image.Rectangle, actions []Action, labels []string, shortcutTarget uintptr, events chan<- ToolbarEvent, started chan<- actionToolbarStart, done chan<- struct{}, background ToolbarBackground) {
	defer close(done)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	workArea, err := monitorWorkArea(region)
	if err != nil {
		started <- actionToolbarStart{err: err}
		return
	}
	instance, _, callErr := procGetModuleHandle.Call(0)
	if instance == 0 {
		started <- actionToolbarStart{err: win32Error("GetModuleHandleW", callErr)}
		return
	}
	className, _ := syscall.UTF16PtrFromString("ScreenshotWinActionToolbar")
	arrow, _, callErr := procLoadCursor.Call(0, 32512)
	if arrow == 0 {
		started <- actionToolbarStart{err: win32Error("LoadCursorW", callErr)}
		return
	}
	class := windowClassEx{
		Size: uint32(unsafe.Sizeof(windowClassEx{})), WindowProcedure: toolbarProcedure,
		Instance: instance, Cursor: arrow, Background: colorWindow + 1, ClassName: className,
	}
	atom, _, callErr := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		started <- actionToolbarStart{err: win32Error("RegisterClassExW", callErr)}
		return
	}
	defer procUnregisterClass.Call(uintptr(unsafe.Pointer(className)), instance)

	state := &toolbarState{
		action: ActionCancel, clientSize: glassToolbarSize(len(actions), 96),
		background: background, glassEnabled: true, region: region,
		actions: actions, labels: labels, shortcutTarget: shortcutTarget,
		persistent: true, events: events, style: editor.DefaultStyle(), instance: instance, workArea: workArea, dpi: 96,
	}
	bounds := toolbarBounds(region, workArea, state.clientSize)
	state.windowBounds = bounds
	title, _ := syscall.UTF16PtrFromString("screenshot-win actions")
	hwnd, _, callErr := procCreateWindowEx.Call(
		wsExTopmost|wsExToolWindow|wsExLayered,
		uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)), wsPopup,
		uintptr(bounds.Min.X), uintptr(bounds.Min.Y), uintptr(bounds.Dx()), uintptr(bounds.Dy()),
		// Do not attach the toolbar's input queue to the frozen UI thread.
		// The drawing window requests z-order updates with wmToolbarRaise.
		0, 0, instance, 0,
	)
	if hwnd == 0 {
		started <- actionToolbarStart{err: win32Error("CreateWindowExW", callErr)}
		return
	}
	state.hwnd = hwnd
	stopCancellation := context.AfterFunc(ctx, func() {
		procPostMessage.Call(hwnd, wmClose, 0, 0)
	})
	defer stopCancellation()
	toolbarStates.Store(hwnd, state)
	defer toolbarStates.Delete(hwnd)
	activeToolbarWindow.Store(hwnd)
	defer activeToolbarWindow.Store(0)

	dpi := uintptr(96)
	if procGetDPIForWindow.Find() == nil {
		dpi, _, _ = procGetDPIForWindow.Call(hwnd)
	}
	if dpi > 96 {
		state.dpi = int(dpi)
		state.clientSize = glassToolbarSize(len(actions), int(dpi))
		bounds = toolbarBounds(region, workArea, state.clientSize)
		state.windowBounds = bounds
		procSetWindowPos.Call(hwnd, 0, uintptr(bounds.Min.X), uintptr(bounds.Min.Y), uintptr(bounds.Dx()), uintptr(bounds.Dy()), 0x0014)
	}
	if err := state.createTooltips(instance); err != nil {
		procDestroyWindow.Call(hwnd)
		started <- actionToolbarStart{err: err}
		return
	}
	state.paintGlassOrFallback(hwnd, false)
	procShowWindow.Call(hwnd, swShow)
	procSetForegroundWindow.Call(hwnd)
	procSetFocus.Call(hwnd)
	started <- actionToolbarStart{hwnd: hwnd, state: state}

	var msg message
	for {
		status, _, callErr := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(status) == -1 {
			state.renderErr = win32Error("GetMessageW", callErr)
			procDestroyWindow.Call(hwnd)
			break
		}
		if status == 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
	runtime.KeepAlive(state)
}

// ShowCaptureToolbar displays the persistent long-capture action bar without
// taking focus from the window being scrolled.
func ShowCaptureToolbar(region image.Rectangle) (*CaptureToolbar, error) {
	started := make(chan captureToolbarStart, 1)
	done := make(chan struct{})
	actions := make(chan Action, 1)
	go runCaptureToolbarWindow(region, started, actions, done)
	result := <-started
	if result.err != nil {
		<-done
		return nil, result.err
	}
	return &CaptureToolbar{
		window:      result.hwnd,
		actions:     actions,
		closeWindow: func() { procPostMessage.Call(result.hwnd, wmClose, 0, 0) },
		hideForCapture: func() {
			if result.hideForCapture {
				hideWindowForCapture(result.hwnd)
			}
		},
		restoreAfterCapture: func() {
			if result.hideForCapture {
				procShowWindow.Call(result.hwnd, swShowNoActivate)
			}
		},
		done: done,
	}, nil
}

func runCaptureToolbarWindow(region image.Rectangle, started chan<- captureToolbarStart, selected chan<- Action, done chan<- struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(done)
	defer close(selected)

	workArea, err := monitorWorkArea(region)
	if err != nil {
		started <- captureToolbarStart{err: err}
		return
	}
	instance, _, callErr := procGetModuleHandle.Call(0)
	if instance == 0 {
		started <- captureToolbarStart{err: win32Error("GetModuleHandleW", callErr)}
		return
	}
	className, _ := syscall.UTF16PtrFromString("ScreenshotWinCaptureToolbar")
	arrow, _, callErr := procLoadCursor.Call(0, 32512)
	if arrow == 0 {
		started <- captureToolbarStart{err: win32Error("LoadCursorW", callErr)}
		return
	}
	class := windowClassEx{
		Size: uint32(unsafe.Sizeof(windowClassEx{})), WindowProcedure: toolbarProcedure,
		Instance: instance, Cursor: arrow, Background: colorWindow + 1, ClassName: className,
	}
	atom, _, callErr := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		started <- captureToolbarStart{err: win32Error("RegisterClassExW", callErr)}
		return
	}
	defer procUnregisterClass.Call(uintptr(unsafe.Pointer(className)), instance)

	state := &toolbarState{
		action: ActionCancel, capture: true, clientSize: image.Pt(208, 40),
		actions: captureToolbarActions, labels: []string{"Cancel", "Stop and annotate", "Pin to desktop", "Save as", "Copy"},
		style: editor.DefaultStyle(), instance: instance, workArea: workArea, dpi: 96,
	}
	bounds := toolbarBounds(region, workArea, state.clientSize)
	state.windowBounds = bounds
	title, _ := syscall.UTF16PtrFromString("screenshot-win capture controls")
	hwnd, _, callErr := procCreateWindowEx.Call(
		wsExTopmost|wsExToolWindow|wsExNoActivate,
		uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)), wsPopup,
		uintptr(bounds.Min.X), uintptr(bounds.Min.Y), uintptr(bounds.Dx()), uintptr(bounds.Dy()),
		0, 0, instance, 0,
	)
	if hwnd == 0 {
		started <- captureToolbarStart{err: win32Error("CreateWindowExW", callErr)}
		return
	}
	state.hwnd = hwnd
	toolbarStates.Store(hwnd, state)
	defer toolbarStates.Delete(hwnd)
	activeToolbarWindow.Store(hwnd)
	defer activeToolbarWindow.Store(0)

	dpi := uintptr(96)
	if procGetDPIForWindow.Find() == nil {
		dpi, _, _ = procGetDPIForWindow.Call(hwnd)
	}
	if dpi > 96 {
		state.dpi = int(dpi)
		state.clientSize = image.Pt(scaleForDPI(208, int(dpi)), scaleForDPI(40, int(dpi)))
		bounds = toolbarBounds(region, workArea, state.clientSize)
		state.windowBounds = bounds
		if ok, _, callErr := procSetWindowPos.Call(hwnd, 0, uintptr(bounds.Min.X), uintptr(bounds.Min.Y), uintptr(bounds.Dx()), uintptr(bounds.Dy()), 0x0014); ok == 0 {
			procDestroyWindow.Call(hwnd)
			started <- captureToolbarStart{err: win32Error("SetWindowPos", callErr)}
			return
		}
	}
	if err := state.createTooltips(instance); err != nil {
		procDestroyWindow.Call(hwnd)
		started <- captureToolbarStart{err: err}
		return
	}
	procShowWindow.Call(hwnd, swShowNoActivate)
	overlapsCapture := !bounds.Intersect(region).Empty()
	var captureExcluded uintptr
	if ensureCaptureExclusionSupported() == nil {
		captureExcluded, _, _ = procSetWindowDisplayAffinity.Call(hwnd, wdaExcludeFromCapture)
	}
	started <- captureToolbarStart{hwnd: hwnd, hideForCapture: overlapsCapture && captureExcluded == 0}

	var msg message
	for {
		status, _, callErr := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(status) == -1 {
			state.renderErr = win32Error("GetMessageW", callErr)
			procDestroyWindow.Call(hwnd)
			break
		}
		if status == 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
	if state.chosen {
		selected <- state.action
	}
	runtime.KeepAlive(state)
}

func scaleForDPI(value, dpi int) int { return (value*dpi + 48) / 96 }

func monitorWorkArea(region image.Rectangle) (image.Rectangle, error) {
	area := rect{int32(region.Min.X), int32(region.Min.Y), int32(region.Max.X), int32(region.Max.Y)}
	monitor, _, callErr := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&area)), monitorDefaultToNearest)
	if monitor == 0 {
		return image.Rectangle{}, win32Error("MonitorFromRect", callErr)
	}
	info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	ok, _, callErr := procGetMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return image.Rectangle{}, win32Error("GetMonitorInfoW", callErr)
	}
	return image.Rect(int(info.Work.Left), int(info.Work.Top), int(info.Work.Right), int(info.Work.Bottom)), nil
}

// TriggerPin posts the Pin button action to the current capture toolbar.
// Posting keeps all toolbar and annotation state on its owning UI thread.
func TriggerPin() bool {
	hwnd := activeToolbarWindow.Load()
	if hwnd == 0 {
		return false
	}
	ok, _, _ := procPostMessage.Call(hwnd, wmToolbarPin, 0, 0)
	return ok != 0
}

func toolbarWindowProcedure(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	value, found := toolbarStates.Load(hwnd)
	if !found {
		result, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
		return result
	}
	state := value.(*toolbarState)
	// Only the same-thread style popup leaves focus on the toolbar. The
	// toolbar itself must activate normally before capturing button input.
	if message == 0x0021 && state.panel.hwnd == hwnd { // WM_MOUSEACTIVATE
		return 3 // MA_NOACTIVATE
	}
	switch message {
	case wmMouseMove, wmMouseLeave, wmLButtonDown, wmLButtonUp, wmToolbarReady, wmToolbarStyle, wmToolbarPin, wmKeyDown, wmRButtonDown, 0x0215:
		state.refreshMotion()
		defer state.refreshMotion()
	}
	if message == wmToolbarBackgroundReady {
		procInvalidateRect.Call(hwnd, 0, 0)
		return 0
	}
	if state.panel.hwnd == hwnd {
		return state.panelWindowProcedure(hwnd, message, wParam, lParam)
	}
	switch message {
	case wmToolbarRaise:
		if wParam == state.shortcutTarget && state.persistent {
			procSetWindowPos.Call(hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
			if state.panel.hwnd != 0 {
				procSetWindowPos.Call(state.panel.hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
			}
		}
		return 0
	case wmActivate:
		if wParam&0xffff == 0 {
			state.pressed, state.hovering = false, false
			state.closeStylePanel()
			procReleaseCapture.Call()
			procInvalidateRect.Call(hwnd, 0, 0)
		}
		// DefWindowProc gives the newly activated toolbar keyboard focus.
	case wmCancelMode:
		state.pressed, state.hovering = false, false
		state.closeStylePanel()
		procReleaseCapture.Call()
		procInvalidateRect.Call(hwnd, 0, 0)
		return 0
	case wmToolbarBackdrop:
		if wParam == state.shortcutTarget && state.glassEnabled {
			state.glassWindow.dirty = true
			state.panel.glassWindow.dirty = true
			procInvalidateRect.Call(hwnd, 0, 0)
			if state.panel.hwnd != 0 {
				procInvalidateRect.Call(state.panel.hwnd, 0, 0)
			}
		}
		return 0
	case wmTimer:
		if wParam == glassFrameTimer {
			procFrozenKillTimer.Call(hwnd, glassFrameTimer)
			procInvalidateRect.Call(hwnd, 0, 0)
			if state.panel.hwnd != 0 {
				procInvalidateRect.Call(state.panel.hwnd, 0, 0)
			}
			return 0
		}
	case 0x02E0: // WM_DPICHANGED
		if state.persistent {
			state.updateGlassDPI(int(wParam & 0xffff))
			return 0
		}
	case wmNCHitTest:
		if state.glassEnabled && !state.glassContains(mousePoint(lParam).Sub(state.windowBounds.Min), false) {
			return ^uintptr(0)
		}
	case wmLButtonDown:
		if state.persistent {
			state.pressedAction, state.pressed = state.actionAt(mousePoint(lParam))
			state.hover, state.hovering = state.pressedAction, state.pressed
			if state.pressed {
				procSetCapture.Call(hwnd)
				procInvalidateRect.Call(hwnd, 0, 0)
			}
			return 0
		}
	case 0x0215: // WM_CAPTURECHANGED
		state.pressed = false
		procInvalidateRect.Call(hwnd, 0, 0)
	case wmToolbarReady:
		if state.persistent {
			state.rearmPersistentActions()
			procInvalidateRect.Call(hwnd, 0, 0)
		}
		return 0
	case wmToolbarStyle:
		state.applyQueuedStyle()
		return 0
	case wmMouseMove:
		point := mousePoint(lParam)
		hover, hovering := state.actionAt(point)
		hovering = hovering && toolbarActionEnabled(hover)
		if hover != state.hover || hovering != state.hovering {
			state.hover = hover
			state.hovering = hovering
			procInvalidateRect.Call(hwnd, 0, 0)
		}
		tracking := trackMouseEvent{Size: uint32(unsafe.Sizeof(trackMouseEvent{})), Flags: tmeLeave, Track: hwnd}
		procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tracking)))
		return 0
	case wmMouseLeave:
		state.hover = ActionCancel
		state.hovering = false
		procInvalidateRect.Call(hwnd, 0, 0)
		return 0
	case wmLButtonUp, wmToolbarPin:
		action, ok := state.actionAt(mousePoint(lParam))
		if state.persistent && message == wmLButtonUp {
			// Existing external callers may synthesize an up message, so retain
			// the established activation behavior while showing press feedback.
			state.pressed = false
			state.hover, state.hovering = action, ok
			procReleaseCapture.Call()
		}
		if message == wmToolbarPin {
			action, ok = ActionPin, false
			for _, available := range state.actions {
				if available == ActionPin {
					ok = true
					break
				}
			}
		}
		if ok && toolbarActionEnabled(action) {
			if state.persistent && (action == ActionColor || action == ActionWidth) {
				state.toggleStylePanel(action)
				return 0
			}
			if state.persistent {
				state.focusDrawingSurface(action)
				if state.handlePersistentAction(action) {
					procInvalidateRect.Call(hwnd, 0, 0)
				}
				return 0
			}
			state.action = action
			state.chosen = true
			procDestroyWindow.Call(hwnd)
		}
		return 0
	case wmRButtonDown:
		if state.persistent {
			if state.handlePersistentAction(ActionCancel) {
				procInvalidateRect.Call(hwnd, 0, 0)
			}
			return 0
		}
		state.action = ActionCancel
		state.chosen = true
		procDestroyWindow.Call(hwnd)
		return 0
	case wmClose:
		state.closeStylePanel()
		procDestroyWindow.Call(hwnd)
		return 0
	case wmKeyDown:
		if state.panel.hwnd != 0 {
			if wParam == vkEscape {
				state.closeStylePanel()
				return 0
			}
			if state.handlePanelKey(wParam) {
				return 0
			}
		}
		if state.shortcutTarget != 0 {
			if command, amount, ok := frozenShortcutForKey(wParam, frozenKeyDown(vkControl), frozenKeyDown(vkShift)); ok {
				procPostMessage.Call(state.shortcutTarget, wmFrozenShortcut, uintptr(command), uintptr(amount))
				return 0
			}
		}
		if wParam == vkEscape {
			if state.capture {
				return 0
			}
			state.action = ActionCancel
			state.chosen = true
			procDestroyWindow.Call(hwnd)
			return 0
		}
	case wmPaint:
		if err := state.paint(hwnd); err != nil {
			state.renderErr = err
			procDestroyWindow.Call(hwnd)
		}
		return 0
	case wmEraseBkgnd:
		return 1
	case wmDestroy:
		state.motionClosed = true
		procFrozenKillTimer.Call(hwnd, glassFrameTimer)
		state.motions = nil
		state.glassWindow.close()
		state.closeStylePanel()
		procPostQuitMessage.Call(0)
		return 0
	}
	result, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}

func (state *toolbarState) createTooltips(instance uintptr) error {
	controls := initCommonControlsEx{Size: uint32(unsafe.Sizeof(initCommonControlsEx{})), ICC: iccWin95Classes}
	if ok, _, callErr := procInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&controls))); ok == 0 {
		return win32Error("InitCommonControlsEx", callErr)
	}
	className, _ := syscall.UTF16PtrFromString("tooltips_class32")
	tooltip, _, callErr := procCreateWindowEx.Call(
		wsExTopmost, uintptr(unsafe.Pointer(className)), 0, wsPopup|ttsAlwaysTip|ttsNoPrefix,
		0, 0, 0, 0, state.hwnd, 0, instance, 0,
	)
	if tooltip == 0 {
		return win32Error("CreateWindowExW tooltip", callErr)
	}
	state.tooltip = tooltip
	if ok, _, callErr := procSetWindowPos.Call(tooltip, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate); ok == 0 {
		return win32Error("SetWindowPos tooltip", callErr)
	}
	procSendMessage.Call(tooltip, ttmActivate, 1, 0)
	procSendMessage.Call(tooltip, ttmSetDelayTime, ttdtAutomatic, tooltipDelayMS)
	procSendMessage.Call(tooltip, ttmSetMaxTipWidth, 0, 240)
	state.tooltips = make([]*uint16, len(state.labels))
	for index, label := range state.labels {
		button := state.buttonRect(index)
		label = state.tooltipLabel(state.actions[index], label)
		state.tooltips[index], _ = syscall.UTF16PtrFromString(label)
		info := toolInfo{
			Size: toolInfoV1Size, Flags: ttfSubclass, Window: state.hwnd, ID: uintptr(index + 1),
			Area:     rect{int32(button.Min.X), int32(button.Min.Y), int32(button.Max.X), int32(button.Max.Y)},
			Instance: instance, Text: state.tooltips[index],
		}
		if ok, _, _ := procSendMessage.Call(tooltip, ttmAddToolW, 0, uintptr(unsafe.Pointer(&info))); ok == 0 {
			return fmt.Errorf("TTM_ADDTOOLW failed for toolbar action %d", state.actions[index])
		}
	}
	return nil
}

func (state *toolbarState) tooltipLabel(action Action, fallback string) string {
	switch action {
	case ActionColor:
		return "Color: " + annotationColorName(state.style.Color)
	case ActionWidth:
		return fmt.Sprintf("Line width: %g px", state.style.Width)
	default:
		return fallback
	}
}

func (state *toolbarState) updateStyleTooltips() {
	if state.tooltip == 0 {
		return
	}
	for index, action := range state.actions {
		if action != ActionColor && action != ActionWidth {
			continue
		}
		label := state.tooltipLabel(action, state.labels[index])
		button := state.buttonRect(index)
		state.tooltips[index], _ = syscall.UTF16PtrFromString(label)
		info := toolInfo{
			Size: toolInfoV1Size, Flags: ttfSubclass, Window: state.hwnd, ID: uintptr(index + 1),
			Area:     rect{int32(button.Min.X), int32(button.Min.Y), int32(button.Max.X), int32(button.Max.Y)},
			Instance: state.instance, Text: state.tooltips[index],
		}
		procSendMessage.Call(state.tooltip, ttmUpdateTipTextW, 0, uintptr(unsafe.Pointer(&info)))
	}
}

func annotationColorName(value color.NRGBA) string {
	names := []string{"Red", "Blue", "Green", "Yellow", "White"}
	for index, candidate := range editor.PresetColors() {
		if candidate == value {
			return names[index]
		}
	}
	return fmt.Sprintf("#%02X%02X%02X", value.R, value.G, value.B)
}

func (state *toolbarState) queueStyle(style editor.Style) {
	state.styleMu.Lock()
	state.pendingStyle = style
	state.styleMu.Unlock()
	procPostMessage.Call(state.hwnd, wmToolbarStyle, 0, 0)
}

func (state *toolbarState) applyQueuedStyle() {
	state.styleMu.Lock()
	style := state.pendingStyle
	state.styleMu.Unlock()
	if style.Width <= 0 {
		return
	}
	state.style = style
	state.updateStyleTooltips()
	procInvalidateRect.Call(state.hwnd, 0, 0)
	if state.panel.hwnd != 0 {
		state.panel.keyIndex = state.currentPanelIndex()
		procInvalidateRect.Call(state.panel.hwnd, 0, 0)
	}
}

func (state *toolbarState) currentPanelIndex() int {
	switch state.panel.field {
	case editor.StyleFieldColor:
		for index, value := range editor.PresetColors() {
			if value == state.style.Color {
				return index
			}
		}
	case editor.StyleFieldWidth:
		for index, value := range editor.PresetWidths() {
			if value == state.style.Width {
				return index
			}
		}
	}
	return 0
}

func (state *toolbarState) actionButtonBounds(action Action) (image.Rectangle, bool) {
	for index, candidate := range state.actions {
		if candidate == action {
			local := state.buttonRect(index)
			return local.Add(state.windowBounds.Min), true
		}
	}
	return image.Rectangle{}, false
}

func (state *toolbarState) toggleStylePanel(action Action) {
	field := editor.StyleFieldColor
	if action == ActionWidth {
		field = editor.StyleFieldWidth
	}
	if state.panel.hwnd != 0 && state.panel.field == field {
		state.closeStylePanel()
		return
	}
	state.closeStylePanel()
	anchor, ok := state.actionButtonBounds(action)
	if !ok {
		return
	}
	size := stylePanelSize(field, state.dpi)
	size = size.Add(image.Pt(2*glass.Margin(state.dpi), 2*glass.Margin(state.dpi)))
	bounds := stylePanelBounds(anchor, state.workArea, size, scaleForDPI(4, state.dpi))
	className, _ := syscall.UTF16PtrFromString("ScreenshotWinActionToolbar")
	title, _ := syscall.UTF16PtrFromString("screenshot-win style choices")
	exStyle := uintptr(wsExTopmost | wsExToolWindow)
	if state.glassEnabled {
		exStyle |= wsExLayered
	}
	hwnd, _, callErr := procCreateWindowEx.Call(
		exStyle,
		uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)), wsPopup,
		uintptr(bounds.Min.X), uintptr(bounds.Min.Y), uintptr(bounds.Dx()), uintptr(bounds.Dy()),
		state.hwnd, 0, state.instance, 0,
	)
	if hwnd == 0 {
		state.renderErr = win32Error("CreateWindowExW style panel", callErr)
		return
	}
	state.panel = stylePanel{hwnd: hwnd, field: field, bounds: bounds, hoverIndex: -1, pressedIndex: -1}
	state.panel.keyIndex = state.currentPanelIndex()
	toolbarStates.Store(hwnd, state)
	if state.glassEnabled {
		state.paintGlassOrFallback(hwnd, true)
	}
	procShowWindow.Call(hwnd, swShowNoActivate)
	procSetCapture.Call(hwnd)
	procInvalidateRect.Call(state.hwnd, 0, 0)
}

func (state *toolbarState) closeStylePanel() {
	hwnd := state.panel.hwnd
	if hwnd == 0 {
		return
	}
	state.panel.hwnd = 0
	state.panel.glassWindow.close()
	procReleaseCapture.Call()
	toolbarStates.Delete(hwnd)
	procDestroyWindow.Call(hwnd)
	procInvalidateRect.Call(state.hwnd, 0, 0)
}

func (state *toolbarState) panelWindowProcedure(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmLButtonDown:
		state.panel.pressedIndex = -1
		if index, ok := state.panelOptionAt(mousePoint(lParam)); ok {
			state.panel.pressedIndex = index
		}
		procInvalidateRect.Call(hwnd, 0, 0)
		return 0
	case wmNCHitTest:
		if state.glassEnabled && !state.glassContains(mousePoint(lParam).Sub(state.panel.bounds.Min), true) {
			return ^uintptr(0)
		}
	case wmMouseMove:
		index, ok := state.panelOptionAt(mousePoint(lParam))
		if !ok {
			index = -1
		}
		if index != state.panel.hoverIndex {
			state.panel.hoverIndex = index
			procInvalidateRect.Call(hwnd, 0, 0)
		}
		return 0
	case wmLButtonUp:
		state.panel.pressedIndex = -1
		point := mousePoint(lParam)
		if index, ok := state.panelOptionAt(point); ok {
			state.choosePanelOption(index)
			return 0
		}
		screen := point.Add(state.panel.bounds.Min)
		if screen.In(state.windowBounds) {
			local := screen.Sub(state.windowBounds.Min)
			action, ok := state.actionAt(local)
			state.closeStylePanel()
			if ok && toolbarActionEnabled(action) {
				if action == ActionColor || action == ActionWidth {
					state.toggleStylePanel(action)
				} else {
					state.focusDrawingSurface(action)
					state.handlePersistentAction(action)
				}
			}
			return 0
		}
		state.closeStylePanel()
		return 0
	case wmRButtonDown, wmClose:
		state.closeStylePanel()
		return 0
	case wmPaint:
		if err := state.paintStylePanel(hwnd); err != nil {
			state.renderErr = err
			state.closeStylePanel()
		}
		return 0
	case wmEraseBkgnd:
		return 1
	case wmDestroy:
		return 0
	}
	result, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}

func (state *toolbarState) handlePanelKey(key uintptr) bool {
	count := len(editor.PresetColors())
	if state.panel.field == editor.StyleFieldWidth {
		count = len(editor.PresetWidths())
	}
	switch key {
	case vkLeft, vkUp:
		state.panel.keyIndex = (state.panel.keyIndex + count - 1) % count
	case vkRight, vkDown:
		state.panel.keyIndex = (state.panel.keyIndex + 1) % count
	case vkReturn:
		state.choosePanelOption(state.panel.keyIndex)
		return true
	default:
		return false
	}
	procInvalidateRect.Call(state.panel.hwnd, 0, 0)
	return true
}

func (state *toolbarState) choosePanelOption(index int) {
	change := editor.StyleChange{Field: state.panel.field, Style: state.style}
	action := ActionColor
	switch state.panel.field {
	case editor.StyleFieldColor:
		values := editor.PresetColors()
		if index < 0 || index >= len(values) {
			return
		}
		change.Style.Color = values[index]
	case editor.StyleFieldWidth:
		values := editor.PresetWidths()
		if index < 0 || index >= len(values) {
			return
		}
		change.Style.Width = values[index]
		action = ActionWidth
	default:
		return
	}
	if state.active && !state.ready {
		tool, ok := drawingToolForAction(state.activeAction)
		if !ok {
			return
		}
		state.style = change.Style
		state.updateStyleTooltips()
		state.closeStylePanel()
		if state.shortcutTarget != 0 {
			packedTool, packedWidth := packToolStyle(tool, state.style)
			procPostMessage.Call(state.shortcutTarget, wmFrozenToolStyle, packedTool, packedWidth)
		}
		procInvalidateRect.Call(state.hwnd, 0, 0)
		return
	}
	if !state.ready || state.events == nil {
		return
	}
	select {
	case state.events <- ToolbarEvent{Action: action, Style: change.Style, Change: change}:
		state.style = change.Style
		state.ready = false
		state.updateStyleTooltips()
		state.closeStylePanel()
		procInvalidateRect.Call(state.hwnd, 0, 0)
	default:
	}
}

func (state *toolbarState) paintStylePanel(hwnd uintptr) error {
	if state.glassEnabled {
		return state.paintGlassOrFallback(hwnd, true)
	}
	return paintToolbarBuffered(hwnd, state.panel.bounds.Size(), state.drawStylePanel)
}

func (state *toolbarState) drawStylePanel(dc uintptr) error {
	background, _, _ := procCreateSolidBrush.Call(rgb(245, 249, 255))
	if background == 0 {
		return fmt.Errorf("CreateSolidBrush failed for style panel")
	}
	defer procDeleteObject.Call(background)
	area := rect{0, 0, int32(state.panel.bounds.Dx()), int32(state.panel.bounds.Dy())}
	procFillRect.Call(dc, uintptr(unsafe.Pointer(&area)), background)
	count := len(editor.PresetColors())
	if state.panel.field == editor.StyleFieldWidth {
		count = len(editor.PresetWidths())
	}
	for index := 0; index < count; index++ {
		state.drawStyleOption(dc, index)
	}
	return nil
}

func (state *toolbarState) styleOptionRect(index int) image.Rectangle {
	padding := scaleForDPI(4, state.dpi)
	if state.panel.field == editor.StyleFieldColor {
		cell := scaleForDPI(36, state.dpi)
		return image.Rect(padding+index*cell, padding, padding+(index+1)*cell, padding+cell).Add(image.Pt(glass.Margin(state.dpi), glass.Margin(state.dpi)))
	}
	cell := scaleForDPI(32, state.dpi)
	return image.Rect(padding, padding+index*cell, state.panel.bounds.Dx()-2*glass.Margin(state.dpi)-padding, padding+(index+1)*cell).Add(image.Pt(glass.Margin(state.dpi), glass.Margin(state.dpi)))
}

func (state *toolbarState) drawStyleOption(dc uintptr, index int) {
	option := state.styleOptionRect(index)
	selected := index == state.currentIndexForField()
	if !state.glassEnabled && (selected || index == state.panel.hoverIndex || index == state.panel.keyIndex) {
		fill := rgb(225, 234, 245)
		if selected {
			fill = rgb(195, 218, 249)
		}
		brush, _, _ := procCreateSolidBrush.Call(fill)
		if brush != 0 {
			area := rect{int32(option.Min.X), int32(option.Min.Y), int32(option.Max.X), int32(option.Max.Y)}
			procFillRect.Call(dc, uintptr(unsafe.Pointer(&area)), brush)
			procDeleteObject.Call(brush)
		}
	}
	center := image.Pt((option.Min.X+option.Max.X)/2, (option.Min.Y+option.Max.Y)/2)
	if state.panel.field == editor.StyleFieldColor {
		value := editor.PresetColors()[index]
		radius := scaleForDPI(9, state.dpi)
		brush, _, _ := procCreateSolidBrush.Call(rgb(value.R, value.G, value.B))
		pen, _, _ := procCreatePen.Call(psSolid, 1, rgb(22, 24, 28))
		if brush != 0 && pen != 0 {
			oldBrush, _, _ := procSelectObject.Call(dc, brush)
			oldPen, _, _ := procSelectObject.Call(dc, pen)
			procEllipse.Call(dc, uintptr(center.X-radius), uintptr(center.Y-radius), uintptr(center.X+radius), uintptr(center.Y+radius))
			procSelectObject.Call(dc, oldPen)
			procSelectObject.Call(dc, oldBrush)
		}
		if pen != 0 {
			procDeleteObject.Call(pen)
		}
		if brush != 0 {
			procDeleteObject.Call(brush)
		}
	} else {
		value := editor.PresetWidths()[index]
		lineLeft := option.Min.X + scaleForDPI(12, state.dpi)
		lineRight := option.Min.X + scaleForDPI(86, state.dpi)
		height := max(1, scaleForDPI(int(value), state.dpi))
		brush, _, _ := procCreateSolidBrush.Call(rgb(state.style.Color.R, state.style.Color.G, state.style.Color.B))
		if brush != 0 {
			lineArea := rect{int32(lineLeft), int32(center.Y - height/2), int32(lineRight), int32(center.Y - height/2 + height)}
			// Pale strokes need a contour against the light material so the
			// white preset still communicates its actual thickness.
			if state.style.Color.R > 200 && state.style.Color.G > 200 && state.style.Color.B > 200 {
				outline, _, _ := procCreateSolidBrush.Call(rgb(135, 148, 166))
				if outline != 0 {
					border := rect{lineArea.Left - 1, lineArea.Top - 1, lineArea.Right + 1, lineArea.Bottom + 1}
					procFillRect.Call(dc, uintptr(unsafe.Pointer(&border)), outline)
					procDeleteObject.Call(outline)
				}
			}
			procFillRect.Call(dc, uintptr(unsafe.Pointer(&lineArea)), brush)
			procDeleteObject.Call(brush)
		}
		label, _ := syscall.UTF16FromString(fmt.Sprintf("%g px", value))
		procSetBkMode.Call(dc, 1)
		ink := state.inkColor()
		procSetTextColor.Call(dc, rgb(ink.R, ink.G, ink.B))
		procTextOut.Call(dc, uintptr(option.Min.X+scaleForDPI(96, state.dpi)), uintptr(center.Y-scaleForDPI(8, state.dpi)), uintptr(unsafe.Pointer(&label[0])), uintptr(len(label)-1))
	}
	if selected {
		ink := state.inkColor()
		pen, _, _ := procCreatePen.Call(psSolid, uintptr(max(2, scaleForDPI(2, state.dpi))), rgb(ink.R, ink.G, ink.B))
		if pen != 0 {
			oldPen, _, _ := procSelectObject.Call(dc, pen)
			x := option.Max.X - scaleForDPI(10, state.dpi)
			y := option.Min.Y + scaleForDPI(9, state.dpi)
			line(dc, x-scaleForDPI(5, state.dpi), y+scaleForDPI(5, state.dpi), x-scaleForDPI(1, state.dpi), y+scaleForDPI(9, state.dpi))
			line(dc, x-scaleForDPI(1, state.dpi), y+scaleForDPI(9, state.dpi), x+scaleForDPI(6, state.dpi), y)
			procSelectObject.Call(dc, oldPen)
			procDeleteObject.Call(pen)
		}
	}
}

func (state *toolbarState) currentIndexForField() int {
	field := state.panel.field
	state.panel.field = field
	return state.currentPanelIndex()
}

func (state *toolbarState) emitPersistentAction(action Action) bool {
	if !state.persistent || !state.ready || state.events == nil {
		return false
	}
	select {
	case state.events <- ToolbarEvent{Action: action, Style: state.style}:
		state.ready = false
		state.active = action == ActionRectangle || action == ActionArrow || action == ActionText
		state.activeAction = action
		return true
	default:
		return false
	}
}

// Request activation on the drawing window's own thread. Owned windows can
// share an input queue across threads, making direct activation synchronous.
func (state *toolbarState) focusDrawingSurface(action Action) {
	if target := state.drawingSurfaceTarget(action); target != 0 {
		procPostMessage.Call(target, wmFrozenFocus, 0, 0)
	}
}

func (state *toolbarState) drawingSurfaceTarget(action Action) uintptr {
	if !drawingToolAction(action) {
		return 0
	}
	return state.shortcutTarget
}

func drawingToolAction(action Action) bool {
	_, ok := drawingToolForAction(action)
	return ok
}

func drawingToolForAction(action Action) (editor.Tool, bool) {
	switch action {
	case ActionRectangle:
		return editor.ToolRectangle, true
	case ActionArrow:
		return editor.ToolArrow, true
	case ActionText:
		return editor.ToolText, true
	default:
		return 0, false
	}
}

func packToolStyle(tool editor.Tool, style editor.Style) (uintptr, uintptr) {
	packedTool := uint32(tool) | uint32(style.Color.R)<<8 | uint32(style.Color.G)<<16 | uint32(style.Color.B)<<24
	packedWidth := math.Float32bits(float32(style.Width))
	return uintptr(packedTool), uintptr(packedWidth)
}

// handlePersistentAction either emits an armed action immediately or remembers
// the latest command selected while a drawing request is active. The frozen
// overlay is asked to finish the old request; NextEvent subsequently rearms the
// toolbar and flushes the remembered command. This lets save, copy, scroll,
// pin, cancel, and the other drawing tools all be clicked without first
// cancelling the active drawing tool.
func (state *toolbarState) handlePersistentAction(action Action) bool {
	if state.emitPersistentAction(action) {
		return true
	}
	if !state.persistent || state.ready || !state.active || action == state.activeAction {
		return false
	}
	if state.pending {
		state.pendingAction = action
		state.activeAction = action
		return true
	}
	previousTool, ok := drawingToolForAction(state.activeAction)
	if !ok {
		return false
	}
	state.pending = true
	state.pendingAction = action
	state.activeAction = action
	if state.shortcutTarget != 0 {
		procPostMessage.Call(state.shortcutTarget, wmFrozenToolSwap, uintptr(previousTool), 0)
	}
	return true
}

func (state *toolbarState) rearmPersistentActions() {
	state.ready = true
	state.active = false
	if !state.pending {
		return
	}
	action := state.pendingAction
	state.pending = false
	if state.emitPersistentAction(action) {
		return
	}
	// The event channel is buffered and should be empty when NextEvent rearms
	// the toolbar. Retain the choice if that invariant is ever temporarily
	// violated rather than dropping the user's switch.
	state.pending = true
	state.pendingAction = action
	state.ready = false
	state.active = true
	state.activeAction = action
}

func (state *toolbarState) paint(hwnd uintptr) error {
	if state.glassEnabled {
		return state.paintGlassOrFallback(hwnd, false)
	}
	return paintToolbarBuffered(hwnd, state.clientSize, state.draw)
}

// Render a complete frame offscreen so background fills and individual glyphs
// never become visible before the rest of the toolbar is ready.
func paintToolbarBuffered(hwnd uintptr, size image.Point, draw func(uintptr) error) error {
	var paint paintStruct
	windowDC, _, callErr := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&paint)))
	if windowDC == 0 {
		return win32Error("BeginPaint", callErr)
	}
	defer procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&paint)))
	if size.X <= 0 || size.Y <= 0 {
		return nil
	}
	dc, _, callErr := procCreateCompatibleDC.Call(windowDC)
	if dc == 0 {
		return win32Error("CreateCompatibleDC toolbar", callErr)
	}
	defer procDeleteDC.Call(dc)
	bitmap, _, callErr := procCreateCompatibleBitmap.Call(windowDC, uintptr(size.X), uintptr(size.Y))
	if bitmap == 0 {
		return win32Error("CreateCompatibleBitmap toolbar", callErr)
	}
	defer procDeleteObject.Call(bitmap)
	previous, _, callErr := procSelectObject.Call(dc, bitmap)
	if previous == 0 || previous == ^uintptr(0) {
		return win32Error("SelectObject toolbar bitmap", callErr)
	}
	defer procSelectObject.Call(dc, previous)
	if err := draw(dc); err != nil {
		return err
	}
	const srccopy = 0x00CC0020
	if ok, _, err := procBitBlt.Call(windowDC, 0, 0, uintptr(size.X), uintptr(size.Y), dc, 0, 0, srccopy); ok == 0 {
		return win32Error("BitBlt toolbar", err)
	}
	return nil
}

func (state *toolbarState) draw(dc uintptr) error {
	var frames []toolbarMotionFrame
	if state.persistent {
		var active bool
		frames, active = state.motionFrames(time.Now())
		if active {
			procFrozenSetTimer.Call(state.hwnd, glassFrameTimer, 16, 0)
		}
	}
	backgroundColor := rgb(42, 45, 50)
	if state.persistent {
		backgroundColor = rgb(245, 249, 255)
	}
	background, _, _ := procCreateSolidBrush.Call(backgroundColor)
	if background == 0 {
		return fmt.Errorf("CreateSolidBrush failed for toolbar background")
	}
	defer procDeleteObject.Call(background)
	area := rect{0, 0, int32(state.clientSize.X), int32(state.clientSize.Y)}
	procFillRect.Call(dc, uintptr(unsafe.Pointer(&area)), background)
	iconRenderer := newToolbarIconRenderer(dc)
	if state.persistent {
		ink := state.inkColor()
		iconRenderer.ink = &ink
	}
	defer iconRenderer.close()

	for index, action := range state.actions {
		button := state.buttonRect(index)
		enabled := toolbarActionEnabled(action)
		if state.persistent {
			frame := frames[index]
			state.drawMotionBackground(dc, index, frame)
			ink := frame.ink()
			iconRenderer.ink, iconRenderer.motion = &ink, &frame
			iconRenderer.draw(action, button, enabled, state.style, state.dpi)
			continue
		}
		if (state.active && state.activeAction == action) || (state.hovering && state.hover == action) {
			hoverColor, selectedColor := rgb(67, 73, 81), rgb(48, 94, 150)
			if state.persistent {
				hoverColor, selectedColor = rgb(225, 234, 245), rgb(195, 218, 249)
			}
			brush, _, _ := procCreateSolidBrush.Call(hoverColor)
			if state.active && state.activeAction == action {
				if activeBrush, _, _ := procCreateSolidBrush.Call(selectedColor); activeBrush != 0 {
					if brush != 0 {
						procDeleteObject.Call(brush)
					}
					brush = activeBrush
				}
			}
			if brush != 0 {
				hoverArea := rect{int32(button.Min.X), int32(button.Min.Y), int32(button.Max.X), int32(button.Max.Y)}
				procFillRect.Call(dc, uintptr(unsafe.Pointer(&hoverArea)), brush)
				procDeleteObject.Call(brush)
			}
		}
		iconRenderer.draw(action, button, enabled, state.style, state.dpi)
	}
	return nil
}

func line(dc uintptr, x1, y1, x2, y2 int) {
	procMoveToEx.Call(dc, uintptr(x1), uintptr(y1), 0)
	procLineTo.Call(dc, uintptr(x2), uintptr(y2))
}

func rgb(red, green, blue byte) uintptr {
	return uintptr(red) | uintptr(green)<<8 | uintptr(blue)<<16
}
