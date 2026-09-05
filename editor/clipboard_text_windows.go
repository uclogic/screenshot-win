//go:build windows

package editor

import (
	"fmt"
	"image"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

var (
	procClipboardTextGDIFlush   = editorGDI32.NewProc("GdiFlush")
	procClipboardSaveDC         = editorGDI32.NewProc("SaveDC")
	procClipboardRestoreDC      = editorGDI32.NewProc("RestoreDC")
	procClipboardGraphicsMode   = editorGDI32.NewProc("SetGraphicsMode")
	procClipboardWorldTransform = editorGDI32.NewProc("SetWorldTransform")
	procClipboardPatBlt         = editorGDI32.NewProc("PatBlt")
)

// clipboardTextImage keeps the original layout and glyphs alongside a bitmap
// for image.Image consumers. Pins draw the glyphs directly at their target size.
type clipboardTextImage struct {
	image.Image
	lines      []string
	lineHeight int
}

// DrawToDC paints retained text into a Windows device context. The world
// transform scales font outlines and positions together without rewrapping.
func (text *clipboardTextImage) DrawToDC(dc uintptr, size image.Point) error {
	if size.X <= 0 || size.Y <= 0 {
		return nil
	}
	fail := func() error { return fmt.Errorf("无法绘制文字贴图") }
	saved, _, _ := procClipboardSaveDC.Call(dc)
	if saved == 0 {
		return fail()
	}
	defer procClipboardRestoreDC.Call(dc, saved)
	if ok, _, _ := procClipboardPatBlt.Call(dc, 0, 0, uintptr(size.X), uintptr(size.Y), 0x00FF0062 /* WHITENESS */); ok == 0 {
		return fail()
	}
	if ok, _, _ := procClipboardGraphicsMode.Call(dc, 2 /* GM_ADVANCED */); ok == 0 {
		return fail()
	}
	transform := [6]float32{float32(size.X) / float32(text.Bounds().Dx()), 0, 0, float32(size.Y) / float32(text.Bounds().Dy()), 0, 0}
	if ok, _, _ := procClipboardWorldTransform.Call(dc, uintptr(unsafe.Pointer(&transform))); ok == 0 {
		return fail()
	}
	face, _ := syscall.UTF16PtrFromString("Microsoft YaHei UI")
	height := int32(-18)
	font, _, _ := procEditorCreateFont.Call(uintptr(height), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, editorCleartype, 0, uintptr(unsafe.Pointer(face)))
	if font == 0 {
		return fail()
	}
	defer procEditorDeleteObject.Call(font)
	oldFont, _, _ := procEditorSelectObject.Call(dc, font)
	if oldFont == 0 || oldFont == ^uintptr(0) {
		return fail()
	}
	defer procEditorSelectObject.Call(dc, oldFont)
	procEditorSetBkMode.Call(dc, editorTransparent)
	procEditorSetTextColor.Call(dc, 0)
	for index, line := range text.lines {
		if line == "" {
			continue
		}
		chars, err := syscall.UTF16FromString(line)
		if err != nil {
			return err
		}
		if ok, _, _ := procEditorTextOut.Call(dc, 16, uintptr(16+index*text.lineHeight), uintptr(unsafe.Pointer(&chars[0])), uintptr(len(chars)-1)); ok == 0 {
			return fail()
		}
	}
	return nil
}

// RenderClipboardText renders plain Unicode text to an opaque white image.
func RenderClipboardText(text string) (image.Image, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("剪贴板文字为空")
	}
	if len(text) > 4<<20 {
		return nil, fmt.Errorf("文字过长，无法生成贴图")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const fontHeight = 18
	const padding = 16
	const maxWidth = 640
	const maxPixels = 64 << 20
	fail := func() (image.Image, error) { return nil, fmt.Errorf("无法渲染剪贴板文字") }
	screen, _, _ := procEditorGetDC.Call(0)
	if screen == 0 {
		return fail()
	}
	defer procEditorReleaseDC.Call(0, screen)
	dc, _, _ := procEditorCreateCompatibleDC.Call(screen)
	if dc == 0 {
		return fail()
	}
	defer procEditorDeleteDC.Call(dc)
	face, _ := syscall.UTF16PtrFromString("Microsoft YaHei UI")
	height := int32(-fontHeight)
	font, _, _ := procEditorCreateFont.Call(uintptr(height), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, editorCleartype, 0, uintptr(unsafe.Pointer(face)))
	if font == 0 {
		return fail()
	}
	defer procEditorDeleteObject.Call(font)
	oldFont, _, _ := procEditorSelectObject.Call(dc, font)
	if oldFont == 0 || oldFont == ^uintptr(0) {
		return fail()
	}
	defer procEditorSelectObject.Call(dc, oldFont)
	lineHeight := fontHeight
	measure := func(text string) (int, error) {
		if text == "" {
			return 0, nil
		}
		chars, err := syscall.UTF16FromString(text)
		if err != nil {
			return 0, err
		}
		var size editorTextSize
		ok, _, _ := procEditorGetTextExtentPoint32.Call(dc, uintptr(unsafe.Pointer(&chars[0])), uintptr(len(chars)-1), uintptr(unsafe.Pointer(&size)))
		if ok == 0 {
			return 0, fmt.Errorf("无法测量剪贴板文字")
		}
		if int(size.Height) > lineHeight {
			lineHeight = int(size.Height)
		}
		return int(size.Width), nil
	}
	if _, err := measure("Ag中文"); err != nil {
		return nil, err
	}
	lines, err := wrapClipboardText(text, maxWidth, maxPixels/((maxWidth+padding*2)*(lineHeight+4)), measure)
	if err != nil {
		return nil, err
	}
	lineHeight += 4
	width := 1
	for _, line := range lines {
		w, err := measure(line)
		if err != nil {
			return nil, err
		}
		if w > width {
			width = w
		}
	}
	width += padding * 2
	imageHeight := len(lines)*lineHeight + padding*2
	if imageHeight <= 0 || int64(width)*int64(imageHeight) > maxPixels {
		return nil, fmt.Errorf("文字过长，无法生成贴图")
	}
	info := editorBitmapInfo{Header: editorBitmapInfoHeader{Size: uint32(unsafe.Sizeof(editorBitmapInfoHeader{})), Width: int32(width), Height: -int32(imageHeight), Planes: 1, BitCount: 32}}
	var pointer unsafe.Pointer
	bitmap, _, _ := procEditorCreateDIBSection.Call(screen, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&pointer)), 0, 0)
	if bitmap == 0 || pointer == nil {
		return fail()
	}
	defer procEditorDeleteObject.Call(bitmap)
	oldBitmap, _, _ := procEditorSelectObject.Call(dc, bitmap)
	if oldBitmap == 0 || oldBitmap == ^uintptr(0) {
		return fail()
	}
	defer procEditorSelectObject.Call(dc, oldBitmap)
	raw := unsafe.Slice((*byte)(pointer), width*imageHeight*4)
	for i := range raw {
		raw[i] = 255
	}
	procEditorSetBkMode.Call(dc, editorTransparent)
	procEditorSetTextColor.Call(dc, 0)
	for index, line := range lines {
		if line == "" {
			continue
		}
		chars, _ := syscall.UTF16FromString(line)
		ok, _, _ := procEditorTextOut.Call(dc, padding, uintptr(padding+index*lineHeight), uintptr(unsafe.Pointer(&chars[0])), uintptr(len(chars)-1))
		if ok == 0 {
			return fail()
		}
	}
	procClipboardTextGDIFlush.Call()
	result := image.NewRGBA(image.Rect(0, 0, width, imageHeight))
	for i := 0; i < len(raw); i += 4 {
		result.Pix[i] = raw[i+2]
		result.Pix[i+1] = raw[i+1]
		result.Pix[i+2] = raw[i]
		result.Pix[i+3] = 255
	}
	return &clipboardTextImage{Image: result, lines: lines, lineHeight: lineHeight}, nil
}
