//go:build windows

package editor

import (
	"image"
	"runtime"
	"testing"
	"unsafe"
)

func TestClipboardTextDrawAtTargetSize(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	source, err := RenderClipboardText("Hello 中文\n第二行")
	if err != nil {
		t.Fatal(err)
	}
	text, ok := source.(*clipboardTextImage)
	if !ok {
		t.Fatal("text lost its retained layout")
	}
	// Remove the fallback bitmap's pixels: drawing must use retained glyphs.
	text.Image = image.NewRGBA(source.Bounds())
	for _, scale := range []float64{0.5, 1, 2, 8} {
		func() {
			size := image.Pt(int(float64(source.Bounds().Dx())*scale), int(float64(source.Bounds().Dy())*scale))
			dc, _, _ := procEditorCreateCompatibleDC.Call(0)
			if dc == 0 {
				t.Fatal("CreateCompatibleDC failed")
			}
			defer procEditorDeleteDC.Call(dc)
			info := editorBitmapInfo{Header: editorBitmapInfoHeader{Size: uint32(unsafe.Sizeof(editorBitmapInfoHeader{})), Width: int32(size.X), Height: -int32(size.Y), Planes: 1, BitCount: 32}}
			var pointer unsafe.Pointer
			bitmap, _, _ := procEditorCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&pointer)), 0, 0)
			if bitmap == 0 || pointer == nil {
				t.Fatal("CreateDIBSection failed")
			}
			defer procEditorDeleteObject.Call(bitmap)
			old, _, _ := procEditorSelectObject.Call(dc, bitmap)
			defer procEditorSelectObject.Call(dc, old)
			if err := text.DrawToDC(dc, size); err != nil {
				t.Fatal(err)
			}
			procClipboardTextGDIFlush.Call()
			raw := unsafe.Slice((*byte)(pointer), size.X*size.Y*4)
			if raw[0] != 255 || raw[1] != 255 || raw[2] != 255 {
				t.Fatalf("scale %v: missing white background", scale)
			}
			ink := 0
			for i := 0; i < len(raw); i += 4 {
				if raw[i] < 250 || raw[i+1] < 250 || raw[i+2] < 250 {
					ink++
				}
			}
			if ink == 0 {
				t.Fatalf("scale %v: glyphs were not drawn", scale)
			}
		}()
	}
}

func TestRenderClipboardTextOpaqueAndWrapped(t *testing.T) {
	for _, text := range []string{"Hello 中文", "first\n\n第三行", "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz"} {
		result, err := RenderClipboardText(text)
		if err != nil {
			t.Fatal(err)
		}
		bounds := result.Bounds()
		if bounds.Dx() > 672 || bounds.Dx() <= 32 || bounds.Dy() <= 32 {
			t.Fatalf("unexpected bounds: %v", bounds)
		}
		r, g, b, a := result.At(0, 0).RGBA()
		if r != 65535 || g != 65535 || b != 65535 || a != 65535 {
			t.Fatal("background is not opaque white")
		}
		foundInk := false
		for y := 0; y < bounds.Dy(); y++ {
			for x := 0; x < bounds.Dx(); x++ {
				r, g, b, a := result.At(x, y).RGBA()
				if a != 65535 {
					t.Fatal("transparent text pixel")
				}
				if r < 65535 || g < 65535 || b < 65535 {
					foundInk = true
				}
			}
		}
		if !foundInk {
			t.Fatal("text image is blank")
		}
	}
	if _, err := RenderClipboardText(" \n\t"); err == nil {
		t.Fatal("blank content accepted")
	}
}
