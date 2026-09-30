//go:build windows

package selector

import (
	"fmt"
	"image"
	"syscall"
	"unsafe"
)

var procCaptureGdiFlush = gdi32.NewProc("GdiFlush")

func drawCaptureSizeLabel(pixels []byte, width int, cursor image.Point, dimensions image.Point, bounds image.Rectangle, dpi int) image.Rectangle {
	if width <= 0 || dimensions.X <= 0 || dimensions.Y <= 0 {
		return image.Rectangle{}
	}
	first, _ := syscall.UTF16FromString(fmt.Sprint(dimensions.X))
	second, _ := syscall.UTF16FromString(fmt.Sprint(dimensions.Y))
	face, _ := syscall.UTF16PtrFromString("Segoe UI")
	fontSize := max(1, scaleForDPI(12, dpi))
	lineHeight := max(fontSize, scaleForDPI(13, dpi))
	font, _, _ := procFrozenCreateFont.Call(uintptr(-fontSize), 0, 0, 0, 500, 0, 0, 0, 1, 0, 0, 4, 0, uintptr(unsafe.Pointer(face)))
	if font == 0 {
		return image.Rectangle{}
	}
	defer procDeleteObject.Call(font)
	screen, _, _ := procGetDC.Call(0)
	if screen == 0 {
		return image.Rectangle{}
	}
	defer procReleaseDC.Call(0, screen)
	dc, _, _ := procCreateCompatibleDC.Call(screen)
	if dc == 0 {
		return image.Rectangle{}
	}
	defer procDeleteDC.Call(dc)
	oldFont, _, _ := procSelectObject.Call(dc, font)
	if oldFont == 0 || oldFont == ^uintptr(0) {
		return image.Rectangle{}
	}
	defer procSelectObject.Call(dc, oldFont)
	var a, b size
	procFrozenTextExtent.Call(dc, uintptr(unsafe.Pointer(&first[0])), uintptr(len(first)-1), uintptr(unsafe.Pointer(&a)))
	procFrozenTextExtent.Call(dc, uintptr(unsafe.Pointer(&second[0])), uintptr(len(second)-1), uintptr(unsafe.Pointer(&b)))
	labelWidth, labelHeight := max(int(a.Width), int(b.Width))+4, 2*lineHeight+4
	info := bitmapInfo{Header: bitmapInfoHeader{
		Size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), Width: int32(labelWidth), Height: -int32(labelHeight),
		Planes: 1, BitCount: 32, Compression: biRGB,
	}}
	var bits unsafe.Pointer
	bitmap, _, _ := procCreateDIBSection.Call(screen, uintptr(unsafe.Pointer(&info)), dibRGBColors, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bitmap == 0 || bits == nil {
		return image.Rectangle{}
	}
	defer procDeleteObject.Call(bitmap)
	oldBitmap, _, _ := procSelectObject.Call(dc, bitmap)
	if oldBitmap == 0 || oldBitmap == ^uintptr(0) {
		return image.Rectangle{}
	}
	defer procSelectObject.Call(dc, oldBitmap)
	raw := unsafe.Slice((*byte)(bits), labelWidth*labelHeight*4)
	clear(raw)
	procSetBkMode.Call(dc, 1)
	procSetTextColor.Call(dc, 0x00ffffff)
	procTextOut.Call(dc, 2, 1, uintptr(unsafe.Pointer(&first[0])), uintptr(len(first)-1))
	procTextOut.Call(dc, 2, uintptr(1+lineHeight), uintptr(unsafe.Pointer(&second[0])), uintptr(len(second)-1))
	procCaptureGdiFlush.Call()
	mask := make([]byte, labelWidth*labelHeight)
	for i := range mask {
		mask[i] = byte((uint32(raw[i*4]) + uint32(raw[i*4+1]) + uint32(raw[i*4+2]) + 1) / 3)
	}
	position := captureLabelBounds(cursor, labelWidth, labelHeight, bounds, dpi)
	height := len(pixels) / (width * 4)
	for y := 0; y < labelHeight; y++ {
		for x := 0; x < labelWidth; x++ {
			halo := byte(0)
			for dy := max(0, y-1); dy <= min(labelHeight-1, y+1); dy++ {
				for dx := max(0, x-1); dx <= min(labelWidth-1, x+1); dx++ {
					halo = max(halo, mask[dy*labelWidth+dx])
				}
			}
			if halo != 0 {
				capturePixel(pixels, width, height, position.Min.X+x, position.Min.Y+y, 255, 255, 255, byte(uint32(halo)*165/255))
			}
		}
	}
	for y := 0; y < labelHeight; y++ {
		for x := 0; x < labelWidth; x++ {
			if coverage := mask[y*labelWidth+x]; coverage != 0 {
				capturePixel(pixels, width, height, position.Min.X+x, position.Min.Y+y, 0, 0, 0, byte(uint32(coverage)*235/255))
			}
		}
	}
	return position
}
