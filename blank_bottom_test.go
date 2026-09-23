package screenshotwin

import (
	"image"
	"image/color"
	"testing"
)

func TestMatcherMovingContentAboveBlankBottom(t *testing.T) {
	page := image.NewRGBA(image.Rect(0, 0, 320, 1200))
	for i := range page.Pix {
		page.Pix[i] = 255
	}
	// A smooth illustration near the top; the rest of the page is blank.
	for y := 0; y < 120; y++ {
		for x := 0; x < 320; x++ {
			v := uint8(40 + (x%16)*5 + y)
			page.SetRGBA(x, y, color.RGBA{v, v, v, 255})
		}
	}
	assertOffset(t, crop(page, 0, 900), crop(page, 41, 900), 41)
}

func TestStitchTextAboveBlankBottom(t *testing.T) {
	const height = 900
	page := motionTextPage(320, 1500)
	for y := 240; y < 1500; y++ {
		for x := 0; x < 320; x++ {
			page.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	positions := []int{0, 41, 82, 123}
	frames := make([]image.Image, len(positions))
	for i, top := range positions {
		frames[i] = crop(page, top, height)
	}
	result, err := Stitch(frames)
	if err != nil {
		t.Fatal(err)
	}
	assertImagesEqual(t, result, crop(page, 0, height+positions[len(positions)-1]))
	matcher, err := NewMatcher(frames[len(frames)-1], DefaultMatchOptions())
	if err != nil {
		t.Fatal(err)
	}
	match, err := matcher.Analyze(frames[len(frames)-1])
	if err != nil || match.Matched || match.Reason != RejectionStationary {
		t.Fatalf("stationary frame: %+v, %v", match, err)
	}
}
