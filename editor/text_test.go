package editor

import (
	"image"
	"image/color"
	"testing"
)

func TestTextCoverageCompositedInPreviewAndExport(t *testing.T) {
	for _, alpha := range []uint8{128, 255} {
		source := image.NewNRGBA(image.Rect(0, 0, 8, 4))
		for y := 0; y < 4; y++ {
			for x := 0; x < 8; x++ {
				source.SetNRGBA(x, y, color.NRGBA{B: 255, A: 255})
			}
		}
		document, _ := NewDocument(source)
		annotation := Annotation{Tool: ToolText, Text: "Aa中文", Start: image.Pt(2, 1), Style: Style{Color: color.NRGBA{R: 255, A: alpha}}, mask: &textMask{width: 4, height: 1, pixels: []byte{0, 64, 128, 255}}}
		preview := document.RenderedPreview(0, &annotation)
		document.annotations = []Annotation{annotation}
		for _, rendered := range []image.Image{preview, document.Rendered()} {
			for x, coverage := range []uint8{0, 64, 128, 255} {
				a := uint8((uint32(alpha)*uint32(coverage) + 127) / 255)
				want := color.NRGBA{R: a, B: 255 - a, A: 255}
				if got := rendered.At(x+2, 1); got != want {
					t.Fatalf("alpha=%d coverage=%d: got %v, want %v", alpha, coverage, got, want)
				}
			}
			if got := rendered.At(1, 1); got != (color.NRGBA{B: 255, A: 255}) {
				t.Fatalf("outside mask: %v", got)
			}
		}
	}
}
