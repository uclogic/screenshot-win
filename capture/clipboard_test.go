package capture

import (
	"encoding/binary"
	"image"
	"image/color"
	"testing"
)

func TestEncodeDIBHeaderAndBottomUpPixels(t *testing.T) {
	source := image.NewRGBA(image.Rect(10, 20, 12, 22))
	source.Set(10, 20, color.RGBA{R: 255, A: 255})
	source.Set(11, 20, color.RGBA{G: 255, A: 255})
	source.Set(10, 21, color.RGBA{B: 255, A: 255})
	source.Set(11, 21, color.RGBA{R: 255, G: 255, B: 255, A: 255})

	dib, err := encodeDIB(source)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(dib), bitmapInfoHeaderSize+16; got != want {
		t.Fatalf("len(DIB) = %d, want %d", got, want)
	}
	if got := binary.LittleEndian.Uint32(dib[4:8]); got != 2 {
		t.Errorf("width = %d, want 2", got)
	}
	if got := binary.LittleEndian.Uint32(dib[8:12]); got != 2 {
		t.Errorf("height = %d, want 2", got)
	}
	if got := binary.LittleEndian.Uint16(dib[14:16]); got != 32 {
		t.Errorf("bit count = %d, want 32", got)
	}
	wantPixels := []byte{
		255, 0, 0, 255, 255, 255, 255, 255,
		0, 0, 255, 255, 0, 255, 0, 255,
	}
	for index, want := range wantPixels {
		if got := dib[bitmapInfoHeaderSize+index]; got != want {
			t.Fatalf("pixel byte %d = %d, want %d", index, got, want)
		}
	}
}

func TestEncodeDIBRejectsEmptyImage(t *testing.T) {
	if _, err := encodeDIB(image.NewRGBA(image.Rectangle{})); err == nil {
		t.Fatal("encodeDIB() accepted an empty image")
	}
}
