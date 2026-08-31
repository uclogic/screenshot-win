//go:build windows

package selector

import (
	"fmt"
	"image"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	wmPreviewUpdate        = wmUser + 77
	wdaExcludeFromCapture  = 0x00000011
	windows10Version       = 10
	windows10Build2004     = 19041
	previewFrameThickness  = 2
	previewBackgroundAlpha = 245
)

var (
	dwmapi                       = syscall.NewLazyDLL("dwmapi.dll")
	ntdll                        = syscall.NewLazyDLL("ntdll.dll")
	procDwmIsCompositionEnabled  = dwmapi.NewProc("DwmIsCompositionEnabled")
	procDwmFlush                 = dwmapi.NewProc("DwmFlush")
	procRtlGetVersion            = ntdll.NewProc("RtlGetVersion")
	procSetWindowDisplayAffinity = user32.NewProc("SetWindowDisplayAffinity")
	previewProcedure             = syscall.NewCallback(previewWindowProcedure)
	activePreview                *previewState
)

type rtlOSVersionInfoEx struct {
	Size             uint32
	MajorVersion     uint32
	MinorVersion     uint32
	BuildNumber      uint32
	PlatformID       uint32
	CSDVersion       [128]uint16
	ServicePackMajor uint16
	ServicePackMinor uint16
	SuiteMask        uint16
	ProductType      byte
	Reserved         byte
}

type previewStart struct {
	hwnd           uintptr
	hideForCapture bool
	err            error
}

type previewUpdate struct {
	source image.Image
	result chan error
}

type previewState struct {
	*selectionState
	updates chan previewUpdate
}

// ShowPreview displays a capture-excluded, click-through preview for source.
// A nil preview with a nil error means the selected region is too small for a
// safe placement.
func ShowPreview(region image.Rectangle, source image.Image) (*Preview, error) {
	if source == nil || source.Bounds().Empty() {
		return nil, fmt.Errorf("preview image must not be empty")
	}
	workArea, err := monitorWorkArea(region)
	if err != nil {
		return nil, err
	}
	provisionalSize := previewSurfaceSize(workArea, 96)
	if _, _, ok := previewWindowBounds(region, workArea, provisionalSize, previewGapDIP, previewInsideDIP); !ok {
		return nil, nil
	}

	started := make(chan previewStart, 1)
	done := make(chan struct{})
	updates := make(chan previewUpdate, 1)
	go runPreviewWindow(region, workArea, source, started, updates, done)
	result := <-started
	if result.err != nil {
		<-done
		return nil, result.err
	}
	if result.hwnd == 0 {
		<-done
		return nil, nil
	}
	preview := &Preview{done: done}
	preview.updateImage = func(source image.Image) error {
		request := previewUpdate{source: source, result: make(chan error, 1)}
		select {
		case updates <- request:
		case <-done:
			return fmt.Errorf("preview window is closed")
		}
		ok, _, callErr := procPostMessage.Call(result.hwnd, wmPreviewUpdate, 0, 0)
		if ok == 0 {
			return win32Error("PostMessageW", callErr)
		}
		select {
		case updateErr := <-request.result:
			return updateErr
		case <-done:
			return fmt.Errorf("preview window closed while updating")
		}
	}
	preview.closeWindow = func() { procPostMessage.Call(result.hwnd, wmClose, 0, 0) }
	if result.hideForCapture {
		preview.hideForCapture = func() { hideWindowForCapture(result.hwnd) }
		preview.restoreAfterCapture = func() { procShowWindow.Call(result.hwnd, swShowNoActivate) }
	}
	return preview, nil
}

func runPreviewWindow(region, workArea image.Rectangle, source image.Image, started chan<- previewStart, updates chan previewUpdate, done chan<- struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(done)

	instance, _, callErr := procGetModuleHandle.Call(0)
	if instance == 0 {
		started <- previewStart{err: win32Error("GetModuleHandleW", callErr)}
		return
	}
	className, err := syscall.UTF16PtrFromString("ScreenshotWinLivePreview")
	if err != nil {
		started <- previewStart{err: err}
		return
	}
	class := windowClassEx{
		Size: uint32(unsafe.Sizeof(windowClassEx{})), WindowProcedure: previewProcedure,
		Instance: instance, ClassName: className,
	}
	atom, _, callErr := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		started <- previewStart{err: win32Error("RegisterClassExW", callErr)}
		return
	}
	defer procUnregisterClass.Call(uintptr(unsafe.Pointer(className)), instance)

	initialSize := previewSurfaceSize(workArea, 96)
	initialBounds, _, _ := previewWindowBounds(region, workArea, initialSize, previewGapDIP, previewInsideDIP)
	state := &previewState{updates: updates}
	activePreview = state
	defer func() { activePreview = nil }()
	title, _ := syscall.UTF16PtrFromString("screenshot-win live preview")
	hwnd, _, callErr := procCreateWindowEx.Call(
		wsExTopmost|wsExToolWindow|wsExLayered|wsExTransparent|wsExNoActivate,
		uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)), wsPopup,
		uintptr(initialBounds.Min.X), uintptr(initialBounds.Min.Y), uintptr(initialBounds.Dx()), uintptr(initialBounds.Dy()),
		0, 0, instance, 0,
	)
	if hwnd == 0 {
		started <- previewStart{err: win32Error("CreateWindowExW", callErr)}
		return
	}

	dpi := 96
	if procGetDPIForWindow.Find() == nil {
		value, _, _ := procGetDPIForWindow.Call(hwnd)
		if value > 0 {
			dpi = int(value)
		}
	}
	surfaceSize := previewSurfaceSize(workArea, dpi)
	gap := scaleForDPI(previewGapDIP, dpi)
	insidePadding := scaleForDPI(previewInsideDIP, dpi)
	bounds, _, fits := previewWindowBounds(region, workArea, surfaceSize, gap, insidePadding)
	if !fits {
		procDestroyWindow.Call(hwnd)
		started <- previewStart{}
		return
	}
	if ok, _, callErr := procSetWindowPos.Call(hwnd, 0, uintptr(bounds.Min.X), uintptr(bounds.Min.Y), uintptr(bounds.Dx()), uintptr(bounds.Dy()), 0x0014); ok == 0 {
		procDestroyWindow.Call(hwnd)
		started <- previewStart{err: win32Error("SetWindowPos", callErr)}
		return
	}
	state.selectionState = newSelectionState(bounds)
	state.hwnd = hwnd
	if err := state.initializeSurface(); err != nil {
		procDestroyWindow.Call(hwnd)
		started <- previewStart{err: err}
		return
	}
	defer state.closeSurface()
	if err := state.renderPreview(source); err != nil {
		procDestroyWindow.Call(hwnd)
		started <- previewStart{err: err}
		return
	}
	procShowWindow.Call(hwnd, swShowNoActivate)
	overlapsCapture := !bounds.Intersect(region).Empty()
	var captureExcluded uintptr
	if ensureCaptureExclusionSupported() == nil {
		captureExcluded, _, _ = procSetWindowDisplayAffinity.Call(hwnd, wdaExcludeFromCapture)
	}
	started <- previewStart{hwnd: hwnd, hideForCapture: overlapsCapture && captureExcluded == 0}

	var msg message
	for {
		status, _, callErr := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(status) == -1 {
			state.renderErr = win32Error("GetMessageW", callErr)
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

// hideWindowForCapture waits for DWM after hiding so an immediate BitBlt does
// not receive a stale composed frame containing the window.
func hideWindowForCapture(hwnd uintptr) {
	procShowWindow.Call(hwnd, swHide)
	procDwmFlush.Call()
}

func previewWindowProcedure(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	state := activePreview
	if state == nil {
		result, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
		return result
	}
	switch message {
	case wmPreviewUpdate:
		select {
		case request := <-state.updates:
			request.result <- state.renderPreview(request.source)
		default:
		}
		return 0
	case wmNCHitTest:
		return htTransparent
	case wmClose:
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	result, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}

func previewSurfaceSize(workArea image.Rectangle, dpi int) image.Point {
	width := scaleForDPI(previewWidthDIP, dpi)
	height := workArea.Dy() * previewMaxHeightRatio / 100
	return image.Pt(max(1, width), max(1, height))
}

func ensureCaptureExclusionSupported() error {
	version := rtlOSVersionInfoEx{Size: uint32(unsafe.Sizeof(rtlOSVersionInfoEx{}))}
	status, _, _ := procRtlGetVersion.Call(uintptr(unsafe.Pointer(&version)))
	if status != 0 {
		return fmt.Errorf("RtlGetVersion failed (NTSTATUS=0x%x)", status)
	}
	if version.MajorVersion < windows10Version || version.MajorVersion == windows10Version && version.BuildNumber < windows10Build2004 {
		return fmt.Errorf("capture-excluded preview requires Windows 10 version 2004 or later")
	}
	var enabled int32
	hresult, _, _ := procDwmIsCompositionEnabled.Call(uintptr(unsafe.Pointer(&enabled)))
	if hresult != 0 {
		return fmt.Errorf("DwmIsCompositionEnabled failed (HRESULT=0x%x)", hresult)
	}
	if enabled == 0 {
		return fmt.Errorf("capture-excluded preview requires desktop composition")
	}
	return nil
}

func (state *previewState) renderPreview(source image.Image) error {
	if source == nil || source.Bounds().Empty() {
		return fmt.Errorf("preview image must not be empty")
	}
	clear(state.pixels)
	client := state.client
	availableWidth := max(1, client.Dx()-previewFrameThickness*2)
	availableHeight := max(1, client.Dy()-previewFrameThickness*2)
	sourceBounds := source.Bounds()
	drawWidth := availableWidth
	drawHeight := max(1, sourceBounds.Dy()*drawWidth/sourceBounds.Dx())
	if drawHeight > availableHeight {
		drawHeight = availableHeight
		drawWidth = max(1, sourceBounds.Dx()*drawHeight/sourceBounds.Dy())
	}
	frame := image.Rect(
		(client.Dx()-drawWidth)/2-previewFrameThickness,
		0,
		(client.Dx()+drawWidth)/2+previewFrameThickness,
		drawHeight+previewFrameThickness*2,
	).Intersect(client)
	fillPreviewRectangle(state.pixels, client.Dx(), frame, 42, 45, 50, previewBackgroundAlpha)
	destination := image.Rect(
		(client.Dx()-drawWidth)/2,
		previewFrameThickness,
		(client.Dx()-drawWidth)/2+drawWidth,
		previewFrameThickness+drawHeight,
	)
	for y := 0; y < drawHeight; y++ {
		sourceY := sourceBounds.Min.Y + y*sourceBounds.Dy()/drawHeight
		for x := 0; x < drawWidth; x++ {
			sourceX := sourceBounds.Min.X + x*sourceBounds.Dx()/drawWidth
			red, green, blue, alpha := source.At(sourceX, sourceY).RGBA()
			index := ((destination.Min.Y+y)*client.Dx() + destination.Min.X + x) * 4
			state.pixels[index] = byte(blue >> 8)
			state.pixels[index+1] = byte(green >> 8)
			state.pixels[index+2] = byte(red >> 8)
			state.pixels[index+3] = byte(alpha >> 8)
		}
	}
	return state.present()
}

func fillPreviewRectangle(pixels []byte, width int, area image.Rectangle, red, green, blue, alpha byte) {
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			index := (y*width + x) * 4
			pixels[index], pixels[index+1], pixels[index+2], pixels[index+3] = blue, green, red, alpha
		}
	}
}
