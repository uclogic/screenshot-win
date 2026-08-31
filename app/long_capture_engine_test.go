package app

import (
	"image"
	"image/color"
	"testing"

	"screenshot-win"
)

func TestLongCaptureEngineSelection(t *testing.T) {
	first := engineTestCrop(engineTestImage(80, 240), 80, 80)
	legacy, err := newLongCaptureEngine(LongCaptureLegacy, first, screenshotwin.DefaultMatchOptions())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := legacy.(*legacyLongCaptureEngine); !ok {
		t.Fatalf("legacy selection returned %T", legacy)
	}
	bidirectional, err := newLongCaptureEngine(LongCaptureBidirectional, first, screenshotwin.DefaultMatchOptions())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := bidirectional.(*bidirectionalLongCaptureEngine); !ok {
		t.Fatalf("bidirectional selection returned %T", bidirectional)
	}
	if _, err := newLongCaptureEngine(99, first, screenshotwin.DefaultMatchOptions()); err == nil {
		t.Fatal("unknown implementation succeeded")
	}
}

func TestLegacyAndBidirectionalEnginesKeepSeparateDirectionSemantics(t *testing.T) {
	source := engineTestImage(80, 240)
	first := engineTestCrop(source, 80, 80)
	up := engineTestCrop(source, 40, 80)

	legacy, err := newLongCaptureEngine(LongCaptureLegacy, first, screenshotwin.DefaultMatchOptions())
	if err != nil {
		t.Fatal(err)
	}
	legacyResult, err := legacy.Add(up)
	if err != nil {
		t.Fatal(err)
	}
	if legacyResult.matched {
		t.Fatalf("legacy engine unexpectedly matched upward movement: %+v", legacyResult)
	}

	bidirectional, err := newLongCaptureEngine(LongCaptureBidirectional, first, screenshotwin.DefaultMatchOptions())
	if err != nil {
		t.Fatal(err)
	}
	bidirectionalResult, err := bidirectional.Add(up)
	if err != nil {
		t.Fatal(err)
	}
	if !bidirectionalResult.matched || bidirectionalResult.offset != -40 || bidirectionalResult.addedTop != 40 {
		t.Fatalf("bidirectional engine result = %+v", bidirectionalResult)
	}
}

func engineTestImage(width, height int) *image.RGBA {
	result := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			result.SetRGBA(x, y, color.RGBA{
				R: uint8((x*17 + y*13 + (y/7)*29) % 256),
				G: uint8((x*7 + y*19 + x*y) % 256),
				B: uint8((x*23 + y*5 + (y/11)*41) % 256),
				A: 255,
			})
		}
	}
	return result
}

func engineTestCrop(source *image.RGBA, y, height int) *image.RGBA {
	result := image.NewRGBA(image.Rect(0, 0, source.Bounds().Dx(), height))
	for row := 0; row < height; row++ {
		copy(result.Pix[row*result.Stride:(row+1)*result.Stride], source.Pix[(y+row)*source.Stride:(y+row+1)*source.Stride])
	}
	return result
}
