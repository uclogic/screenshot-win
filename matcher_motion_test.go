package screenshotwin

import (
	"image"
	"image/color"
	"testing"
)

// Text-like strokes on a mostly white page, with nonrepeating line contents.
// Smooth scrolling can change the rasterization of every stroke at once.
func motionTextPage(width, height int) *image.RGBA {
	page := image.NewRGBA(image.Rect(0, 0, width, height))
	for i := range page.Pix {
		page.Pix[i] = 255
	}
	for y := 0; y < height; y++ {
		if y%24 >= 11 {
			continue
		}
		for x := 12; x < width-12; x++ {
			if x%9 >= 6 {
				continue
			}
			seed := uint32(y/24+1)*2654435761 ^ uint32(x/9+1)*2246822519
			if seed>>uint((x%9+y%24*3)%24)&1 != 0 {
				page.SetRGBA(x, y, color.RGBA{R: 40, G: 40, B: 40, A: 255})
			}
		}
	}
	return page
}

func motionFrame(page *image.RGBA, top, height, fraction int) *image.RGBA {
	frame := crop(page, top, height)
	for y := 0; y < height; y++ {
		for x := 0; x < frame.Bounds().Dx(); x++ {
			for channel := 0; channel < 3; channel++ {
				i := y*frame.Stride + x*4 + channel
				next := (top+y+1)*page.Stride + x*4 + channel
				frame.Pix[i] = byte((int(frame.Pix[i])*(100-fraction) + int(page.Pix[next])*fraction + 50) / 100)
			}
		}
	}
	return frame
}

func TestMatcherSmoothScrollTextAtLargerOffsets(t *testing.T) {
	page := motionTextPage(320, 1500)
	for _, offset := range []int{41, 137, 239, 299} {
		for _, fraction := range []int{10, 20} {
			previous := crop(page, 0, 600)
			current := motionFrame(page, offset, 600, fraction)
			result, err := AnalyzeScroll(previous, current, DefaultMatchOptions())
			if err != nil || !result.Matched || result.Offset != offset {
				t.Errorf("offset=%d subpixel=%d%%: %+v, %v", offset, fraction, result, err)
			}
		}
	}
}

func TestStitchSmoothScrollTextKeepsUpWithConsecutiveFrames(t *testing.T) {
	const height = 600
	page := motionTextPage(320, 2400)
	positions := []int{0, 137, 376, 675, 910, 1205}
	fractions := []int{0, 20, 10, 25, 10, 20}
	frames := make([]image.Image, len(positions))
	for i, position := range positions {
		frames[i] = motionFrame(page, position, height, fractions[i])
	}
	result, err := Stitch(frames)
	if err != nil {
		t.Fatal(err)
	}
	if result.Bounds().Dy() != positions[len(positions)-1]+height {
		t.Fatalf("stitched height = %d, want %d", result.Bounds().Dy(), positions[len(positions)-1]+height)
	}
	// Verify each appended strip came from the intended captured frame.
	for i := 1; i < len(frames); i++ {
		added := positions[i] - positions[i-1]
		frame := frames[i].(*image.RGBA)
		want := frame.SubImage(image.Rect(0, height-added, 320, height))
		got := result.SubImage(image.Rect(0, positions[i-1]+height, 320, positions[i]+height))
		assertImagesEqual(t, got, want)
	}
}

func TestBidirectionalSmoothScrollText(t *testing.T) {
	page := motionTextPage(320, 2400)
	positions := []int{600, 361, 660, 523, 224}
	fractions := []int{0, 20, 10, 25, 10}
	stitcher, err := NewBidirectionalStitcher(motionFrame(page, positions[0], 600, fractions[0]), DefaultMatchOptions())
	if err != nil {
		t.Fatal(err)
	}
	for i, position := range positions[1:] {
		result, err := stitcher.Add(motionFrame(page, position, 600, fractions[i+1]))
		if err != nil || !result.Matched || result.Delta != position-positions[i] {
			t.Fatalf("frame %d: %+v, %v", i+1, result, err)
		}
	}
}
