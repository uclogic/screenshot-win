//go:build windows

package selector

import (
	"image"
	"image/color"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"screenshot-win/editor"
)

var (
	frozenIMM32                 = syscall.NewLazyDLL("imm32.dll")
	procFrozenImmGetContext     = frozenIMM32.NewProc("ImmGetContext")
	procFrozenImmReleaseContext = frozenIMM32.NewProc("ImmReleaseContext")
	procFrozenImmGetComposition = frozenIMM32.NewProc("ImmGetCompositionStringW")
	procFrozenTextExtent        = gdi32.NewProc("GetTextExtentPoint32W")
)

func frozenComposition(hwnd uintptr) string {
	context, _, _ := procFrozenImmGetContext.Call(hwnd)
	if context == 0 {
		return ""
	}
	defer procFrozenImmReleaseContext.Call(hwnd, context)
	length, _, _ := procFrozenImmGetComposition.Call(context, 8, 0, 0)
	if int32(length) <= 0 {
		return ""
	}
	buffer := make([]uint16, int(length)/2)
	procFrozenImmGetComposition.Call(context, 8, uintptr(unsafe.Pointer(&buffer[0])), length)
	return string(utf16.Decode(buffer))
}

func frozenInputText(hwnd uintptr) string {
	length, _, _ := procFrozenGetWindowTextLen.Call(hwnd)
	buffer := make([]uint16, int(length)+1)
	procFrozenGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	return syscall.UTF16ToString(buffer)
}

// Paint with exactly the same document rasterizer and viewport as committed
// annotations. The native EDIT only supplies editing, caret and IME behavior;
// its opaque background, font rendering and border never reach the screen.
func (state *frozenState) paintTextEditor() {
	edit := state.textEdit
	if edit == nil {
		return
	}
	var paint paintStruct
	dc, _, _ := procBeginPaint.Call(edit.hwnd, uintptr(unsafe.Pointer(&paint)))
	if dc == 0 {
		return
	}
	defer procEndPaint.Call(edit.hwnd, uintptr(unsafe.Pointer(&paint)))
	text := frozenInputText(edit.hwnd)
	var selectionStart, selectionEnd uint32
	procSendMessage.Call(edit.hwnd, 0x00B0, uintptr(unsafe.Pointer(&selectionStart)), uintptr(unsafe.Pointer(&selectionEnd)))
	if edit.composition != "" {
		units := utf16.Encode([]rune(text))
		start, end := min(int(selectionStart), len(units)), min(int(selectionEnd), len(units))
		text = string(utf16.Decode(units[:start])) + edit.composition + string(utf16.Decode(units[end:]))
	}
	view := state.renderTextPreview(text)
	// Native selection positions retain standard mouse/keyboard selection behavior.
	if selectionStart != selectionEnd && !edit.composing {
		left, _, _ := procSendMessage.Call(edit.hwnd, 0x00D6, uintptr(selectionStart), 0)
		right, _, _ := procSendMessage.Call(edit.hwnd, 0x00D6, uintptr(selectionEnd), 0)
		if int32(right) == -1 {
			// EM_POSFROMCHAR does not accept the position after the final character.
			units := utf16.Encode([]rune(text))
			var extent struct{ X, Y int32 }
			if len(units) > 0 && edit.font != 0 {
				old, _, _ := procSelectObject.Call(dc, edit.font)
				procFrozenTextExtent.Call(dc, uintptr(unsafe.Pointer(&units[0])), uintptr(len(units)), uintptr(unsafe.Pointer(&extent)))
				procSelectObject.Call(dc, old)
			}
			right = uintptr(extent.X + 1)
		}
		x1, x2 := int(int16(left)), int(int16(right))
		for y := 0; y < min(view.Bounds().Dy(), max(1, int(float64(max(14, int(edit.style.Width)*8))*state.viewport.Scale))); y++ {
			for x := max(0, x1); x < min(view.Bounds().Dx(), x2); x++ {
				index := y*view.Stride + x*4
				view.Pix[index] = byte((uint32(view.Pix[index])*3 + 35) / 4)
				view.Pix[index+1] = byte((uint32(view.Pix[index+1])*3 + 145) / 4)
				view.Pix[index+2] = byte((uint32(view.Pix[index+2])*3 + 255) / 4)
			}
		}
	}
	// A persistent, quiet baseline identifies the editable area even when empty.
	// It is UI chrome only and is never included in the committed annotation.
	for x := 0; x < view.Bounds().Dx(); x++ {
		if x%4 < 2 {
			view.SetNRGBA(x, view.Bounds().Dy()-1, color.NRGBA{R: 75, G: 151, B: 225, A: 255})
		}
	}
	pixels := make([]byte, view.Bounds().Dx()*view.Bounds().Dy()*4)
	if err := copyImageToBGRA(pixels, view.Bounds().Dx(), view.Bounds().Dy(), view); err != nil {
		return
	}
	size := image.Pt(view.Bounds().Dx(), view.Bounds().Dy())
	info := bitmapInfo{Header: bitmapInfoHeader{Size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), Width: int32(size.X), Height: -int32(size.Y), Planes: 1, BitCount: 32, Compression: biRGB}}
	procPinStretchDIBits.Call(dc, 0, 0, uintptr(size.X), uintptr(size.Y), 0, 0, uintptr(size.X), uintptr(size.Y), uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&info)), dibRGBColors, rasterSourceCopy)
}

func (state *frozenState) renderTextPreview(text string) *image.NRGBA {
	edit := state.textEdit
	draft := editor.Annotation{Tool: editor.ToolText, Start: edit.start, Text: text, Style: edit.style}
	return editor.RenderViewport(state.document.RenderedPreview(edit.id, &draft), state.viewport, edit.bounds)
}

// Size before showing and after edits so an empty editor never spans the canvas.
func (state *frozenState) resizeTextEditor() {
	edit := state.textEdit
	if edit == nil || edit.closing {
		return
	}
	text := frozenInputText(edit.hwnd) + edit.composition
	units := utf16.Encode([]rune(text))
	var extent struct{ X, Y int32 }
	dc, _, _ := procGetDC.Call(edit.hwnd)
	if dc == 0 {
		return
	}
	old, _, _ := procSelectObject.Call(dc, edit.font)
	if len(units) > 0 {
		procFrozenTextExtent.Call(dc, uintptr(unsafe.Pointer(&units[0])), uintptr(len(units)), uintptr(unsafe.Pointer(&extent)))
	}
	procSelectObject.Call(dc, old)
	procReleaseDC.Call(edit.hwnd, dc)
	width := max(1, min(state.annotationCanvas().Max.X-edit.bounds.Min.X, max(120, int(extent.X)+24)))
	if width == edit.bounds.Dx() {
		return
	}
	edit.bounds.Max.X = edit.bounds.Min.X + width
	procSetWindowPos.Call(edit.hwnd, 0, 0, 0, uintptr(width), uintptr(edit.bounds.Dy()), 0x0016) // no move, z-order or activation
}
