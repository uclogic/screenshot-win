//go:build windows

package editor

import (
	"syscall"
	"unsafe"
)

const (
	editorDIBRGBColors = 0
	editorBIRGB        = 0
	editorTransparent  = 1
	editorCleartype    = 5
)

var (
	editorUser32                   = syscall.NewLazyDLL("user32.dll")
	editorGDI32                    = syscall.NewLazyDLL("gdi32.dll")
	procEditorGetDC                = editorUser32.NewProc("GetDC")
	procEditorReleaseDC            = editorUser32.NewProc("ReleaseDC")
	procEditorCreateCompatibleDC   = editorGDI32.NewProc("CreateCompatibleDC")
	procEditorDeleteDC             = editorGDI32.NewProc("DeleteDC")
	procEditorCreateDIBSection     = editorGDI32.NewProc("CreateDIBSection")
	procEditorCreateFont           = editorGDI32.NewProc("CreateFontW")
	procEditorSelectObject         = editorGDI32.NewProc("SelectObject")
	procEditorDeleteObject         = editorGDI32.NewProc("DeleteObject")
	procEditorGetTextExtentPoint32 = editorGDI32.NewProc("GetTextExtentPoint32W")
	procEditorSetBkMode            = editorGDI32.NewProc("SetBkMode")
	procEditorSetTextColor         = editorGDI32.NewProc("SetTextColor")
	procEditorTextOut              = editorGDI32.NewProc("TextOutW")
)

type editorBitmapInfoHeader struct {
	Size                         uint32
	Width, Height                int32
	Planes, BitCount             uint16
	Compression, SizeImage       uint32
	XPelsPerMeter, YPelsPerMeter int32
	ClrUsed, ClrImportant        uint32
}
type editorBitmapInfo struct {
	Header editorBitmapInfoHeader
	Colors [1]uint32
}
type editorTextSize struct{ Width, Height int32 }

func rasterizeText(text string, scale int) *textMask {
	characters, err := syscall.UTF16FromString(text)
	if err != nil || len(characters) <= 1 {
		return nil
	}
	face, _ := syscall.UTF16PtrFromString("Microsoft YaHei UI")
	fontHeight := max(14, scale*8)
	screen, _, _ := procEditorGetDC.Call(0)
	if screen == 0 {
		return nil
	}
	defer procEditorReleaseDC.Call(0, screen)
	dc, _, _ := procEditorCreateCompatibleDC.Call(screen)
	if dc == 0 {
		return nil
	}
	defer procEditorDeleteDC.Call(dc)
	font, _, _ := procEditorCreateFont.Call(uintptr(-fontHeight), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, editorCleartype, 0, uintptr(unsafe.Pointer(face)))
	if font == 0 {
		return nil
	}
	defer procEditorDeleteObject.Call(font)
	oldFont, _, _ := procEditorSelectObject.Call(dc, font)
	if oldFont == 0 || oldFont == ^uintptr(0) {
		return nil
	}
	defer procEditorSelectObject.Call(dc, oldFont)
	var dimensions editorTextSize
	if ok, _, _ := procEditorGetTextExtentPoint32.Call(dc, uintptr(unsafe.Pointer(&characters[0])), uintptr(len(characters)-1), uintptr(unsafe.Pointer(&dimensions))); ok == 0 {
		return nil
	}
	width, height := max(1, int(dimensions.Width)+2), max(fontHeight, int(dimensions.Height)+2)
	info := editorBitmapInfo{Header: editorBitmapInfoHeader{Size: uint32(unsafe.Sizeof(editorBitmapInfoHeader{})), Width: int32(width), Height: -int32(height), Planes: 1, BitCount: 32, Compression: editorBIRGB, SizeImage: uint32(width * height * 4)}}
	var bits unsafe.Pointer
	bitmap, _, _ := procEditorCreateDIBSection.Call(screen, uintptr(unsafe.Pointer(&info)), editorDIBRGBColors, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bitmap == 0 || bits == nil {
		return nil
	}
	defer procEditorDeleteObject.Call(bitmap)
	oldBitmap, _, _ := procEditorSelectObject.Call(dc, bitmap)
	if oldBitmap == 0 || oldBitmap == ^uintptr(0) {
		return nil
	}
	defer procEditorSelectObject.Call(dc, oldBitmap)
	procEditorSetBkMode.Call(dc, editorTransparent)
	procEditorSetTextColor.Call(dc, 0x00ffffff)
	if ok, _, _ := procEditorTextOut.Call(dc, 1, 0, uintptr(unsafe.Pointer(&characters[0])), uintptr(len(characters)-1)); ok == 0 {
		return nil
	}
	raw := unsafe.Slice((*byte)(bits), width*height*4)
	mask := &textMask{width: width, height: height, pixels: make([]byte, width*height)}
	for index := range mask.pixels {
		if raw[index*4] != 0 || raw[index*4+1] != 0 || raw[index*4+2] != 0 {
			mask.pixels[index] = 255
		}
	}
	return mask
}
