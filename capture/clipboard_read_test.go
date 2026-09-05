package capture

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
	"unicode/utf16"
)

func testDIB(width, height int32, depth uint16, compression uint32, extra []byte) []byte {
	data := make([]byte, 40)
	binary.LittleEndian.PutUint32(data, 40)
	binary.LittleEndian.PutUint32(data[4:], uint32(width))
	binary.LittleEndian.PutUint32(data[8:], uint32(height))
	binary.LittleEndian.PutUint16(data[12:], 1)
	binary.LittleEndian.PutUint16(data[14:], depth)
	binary.LittleEndian.PutUint32(data[16:], compression)
	return append(data, extra...)
}

func TestDecodeClipboardDIBRoundTripAndOrientation(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 2, 2))
	source.Set(0, 0, color.RGBA{R: 255, A: 255})
	source.Set(1, 0, color.RGBA{G: 255, A: 255})
	source.Set(0, 1, color.RGBA{B: 255, A: 255})
	source.Set(1, 1, color.White)
	data, err := encodeDIB(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, topDown := range []bool{false, true} {
		if topDown {
			binary.LittleEndian.PutUint32(data[8:], uint32(0xfffffffe))
			row := append([]byte(nil), data[40:48]...)
			copy(data[40:48], data[48:56])
			copy(data[48:56], row)
		}
		decoded, err := decodeClipboardDIB(data)
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < 2; y++ {
			for x := 0; x < 2; x++ {
				if color.NRGBAModel.Convert(decoded.At(x, y)) != color.NRGBAModel.Convert(source.At(x, y)) {
					t.Fatalf("wrong pixel at %d,%d", x, y)
				}
			}
		}
	}
}

func TestDecodeClipboardDIBPalettePaddingAndMasks(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want color.NRGBA
	}{
		{"24-bit padded", testDIB(1, 1, 24, 0, []byte{0, 0, 255, 0}), color.NRGBA{R: 255, A: 255}},
		{"16-bit RGB", testDIB(1, 1, 16, 0, []byte{0, 0x7c, 0, 0}), color.NRGBA{R: 255, A: 255}},
		{"32-bit unused alpha", testDIB(1, 1, 32, 0, []byte{0, 0, 255, 0}), color.NRGBA{R: 255, A: 255}},
	}
	for _, depth := range []uint16{1, 4, 8} {
		palette := []byte{0, 0, 0, 0, 0, 0, 255, 0}
		data := testDIB(1, 1, depth, 0, append(palette, byte(1<<(8-depth)), 0, 0, 0))
		binary.LittleEndian.PutUint32(data[32:], 2)
		cases = append(cases, struct {
			name string
			data []byte
			want color.NRGBA
		}{"palette", data, color.NRGBA{R: 255, A: 255}})
	}
	masks := make([]byte, 12)
	binary.LittleEndian.PutUint32(masks, 0xf800)
	binary.LittleEndian.PutUint32(masks[4:], 0x7e0)
	binary.LittleEndian.PutUint32(masks[8:], 0x1f)
	cases = append(cases, struct {
		name string
		data []byte
		want color.NRGBA
	}{"565 bitfields", testDIB(1, 1, 16, 3, append(masks, 0, 0xf8, 0, 0)), color.NRGBA{R: 255, A: 255}})
	v5 := make([]byte, 124+4)
	copy(v5, testDIB(1, 1, 32, 3, nil))
	binary.LittleEndian.PutUint32(v5, 124)
	for i, mask := range []uint32{0xff0000, 0xff00, 0xff, 0xff000000} {
		binary.LittleEndian.PutUint32(v5[40+i*4:], mask)
	}
	copy(v5[124:], []byte{0, 0, 255, 128})
	cases = append(cases, struct {
		name string
		data []byte
		want color.NRGBA
	}{"V5 alpha", v5, color.NRGBA{R: 255, A: 128}})
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result, err := decodeClipboardDIB(test.data)
			if err != nil {
				t.Fatal(err)
			}
			if got := color.NRGBAModel.Convert(result.At(0, 0)); got != test.want {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
}

func TestDecodeClipboardDIBRejectsMalformedData(t *testing.T) {
	valid := testDIB(1, 1, 24, 0, []byte{0, 0, 255, 0})
	tests := [][]byte{nil, valid[:39], valid[:43], testDIB(-1, 1, 24, 0, nil), testDIB(1, -2147483648, 24, 0, nil), testDIB(1<<30, 1<<30, 32, 0, nil), testDIB(1, 1, 24, 1, nil), testDIB(1, 1, 16, 3, make([]byte, 4))}
	badPalette := testDIB(1, 1, 8, 0, []byte{0, 0, 0, 0, 2, 0, 0, 0})
	binary.LittleEndian.PutUint32(badPalette[32:], 1)
	tests = append(tests, badPalette)
	for i, data := range tests {
		if _, err := decodeClipboardDIB(data); err == nil {
			t.Errorf("accepted malformed DIB %d", i)
		}
	}
}

func unicodeClipboard(text string) []byte {
	units := append(utf16.Encode([]rune(text)), 0)
	data := make([]byte, len(units)*2)
	for i, value := range units {
		binary.LittleEndian.PutUint16(data[i*2:], value)
	}
	return data
}

func TestClipboardSnapshotPriorityAndTextFallback(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 1, 1))
	source.Set(0, 0, color.RGBA{R: 255, A: 255})
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, source); err != nil {
		t.Fatal(err)
	}
	dib := testDIB(1, 1, 24, 0, []byte{255, 0, 0, 0})
	content, err := decodeClipboardSnapshot([]clipboardImageData{{true, pngData.Bytes()}, {false, dib}}, unicodeClipboard("文字"), nil)
	if err != nil || content.Image == nil || content.Text != "" {
		t.Fatalf("image priority failed: %+v %v", content, err)
	}
	r, _, _, _ := content.Image.At(0, 0).RGBA()
	if r != 65535 {
		t.Fatal("PNG did not take priority")
	}
	content, err = decodeClipboardSnapshot([]clipboardImageData{{true, []byte("broken")}, {false, dib}}, nil, nil)
	if err != nil || content.Image == nil {
		t.Fatal("DIB fallback failed", err)
	}
	content, err = decodeClipboardSnapshot([]clipboardImageData{{true, []byte("broken")}}, unicodeClipboard("中文\n😀"), nil)
	if err != nil || content.Text != "中文\n😀" {
		t.Fatal("Unicode fallback failed", err)
	}
	for _, data := range [][]byte{nil, unicodeClipboard(" \n\t"), {1}, {65, 0}} {
		if _, err := decodeClipboardSnapshot(nil, data, nil); err == nil {
			t.Fatal("empty or malformed text accepted")
		}
	}
}

func FuzzDecodeClipboardDIB(f *testing.F) {
	f.Add(testDIB(1, 1, 24, 0, []byte{0, 0, 255, 0}))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = decodeClipboardDIB(data) })
}
