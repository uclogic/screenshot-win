package screenshotwin

import (
	"image"
	"image/color"
	"reflect"
	"testing"
)

func repeatedPage(width, height, period int) *image.RGBA {
	band := createTestImage(width, period)
	page := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		copy(page.Pix[y*page.Stride:(y+1)*page.Stride], band.Pix[(y%period)*band.Stride:(y%period+1)*band.Stride])
	}
	return page
}

func TestMatchersRejectIndependentExactRepeats(t *testing.T) {
	page := repeatedPage(160, 640, 64)
	previous, current := crop(page, 0, 384), crop(page, 17, 384)
	for _, confidence := range []float64{0, DefaultMatchOptions().MinimumConfidence} {
		options := DefaultMatchOptions()
		options.MinimumConfidence = confidence
		result, err := AnalyzeScroll(previous, current, options)
		if err != nil || result.Matched || result.Reason != RejectionAmbiguous {
			t.Fatalf("confidence %g: result=%+v err=%v", confidence, result, err)
		}
		stitcher, err := NewBidirectionalStitcher(previous, options)
		if err != nil {
			t.Fatal(err)
		}
		bidirectional, err := stitcher.Add(current)
		if err != nil || bidirectional.Matched {
			t.Fatalf("confidence %g: bidirectional=%+v err=%v", confidence, bidirectional, err)
		}
		assertImagesEqual(t, stitcher.Image(), previous)
	}
}

func TestMatchersIgnoreSmallDynamicRegion(t *testing.T) {
	page := createTestImage(320, 1000)
	for _, delta := range []int{137, -137} {
		previous := crop(page, 200, 600)
		current := crop(page, 200+delta, 600)
		for y := 96; y < 288; y++ {
			for x := 64; x < 128; x++ {
				current.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
			}
		}
		prev, w, h := grayscale(previous)
		curr, _, _ := grayscale(current)
		result := analyzeSignedGrayscale(prev, curr, w, h, DefaultMatchOptions())
		if !result.Matched || result.Delta != delta {
			t.Fatalf("delta %d: %+v", delta, result)
		}
		if delta > 0 {
			assertOffset(t, previous, current, delta)
		}
	}
}

func TestMatcherHandlesFixedHeaderInterference(t *testing.T) {
	page := createTestImage(320, 1000)
	previous, current := crop(page, 0, 600), crop(page, 137, 600)
	for y := 0; y < 32; y++ {
		copy(current.Pix[y*current.Stride:(y+1)*current.Stride], previous.Pix[y*previous.Stride:(y+1)*previous.Stride])
	}
	assertOffset(t, previous, current, 137)
}

func TestMatcherDoesNotDiscardThinUnsampledContent(t *testing.T) {
	page := image.NewRGBA(image.Rect(0, 0, 160, 800))
	for i := range page.Pix {
		page.Pix[i] = 255
	}
	for _, pt := range []image.Point{{13, 93}, {57, 151}, {101, 287}, {137, 413}, {29, 531}} {
		page.SetRGBA(pt.X, pt.Y, color.RGBA{A: 255})
	}
	assertOffset(t, crop(page, 0, 600), crop(page, 41, 600), 41)
}

func TestMatcherRejectsConflictingSparseContent(t *testing.T) {
	previous := createSparseTestImage(320, 600)
	current := createSparseTestImage(320, 600)
	// Most pixels are identical white background, but the marks do not share
	// one translation. Trimming the few marked tiles must not accept this.
	for i := range current.Pix {
		current.Pix[i] = 255
	}
	for _, pt := range []image.Point{{16, 43}, {92, 107}, {208, 199}, {44, 353}} {
		for y := pt.Y; y < pt.Y+3; y++ {
			for x := pt.X; x < pt.X+3; x++ {
				current.SetRGBA(x, y, color.RGBA{A: 255})
			}
		}
	}
	result, err := AnalyzeScroll(previous, current, DefaultMatchOptions())
	if err != nil || result.Matched {
		t.Fatalf("conflicting marks: result=%+v err=%v", result, err)
	}
}

func TestMatcherAllOffsetResidues(t *testing.T) {
	page := createTestImage(96, 700)
	for _, offset := range []int{3, 4, 5, 6, 63, 64, 65, 66, 149, 150} {
		assertOffset(t, crop(page, 0, 300), crop(page, offset, 300), offset)
	}
}

func TestMatcherFallsBackWhenRowMeansLosePosition(t *testing.T) {
	page := descriptorCollisionPage(96, 500)
	previous, current := crop(page, 0, 300), crop(page, 79, 300)
	p, w, h := grayscale(previous)
	c, _, _ := grayscale(current)
	if !reflect.DeepEqual(describeRows(p, w, h), describeRows(c, w, h)) {
		t.Fatal("fixture must have indistinguishable row descriptors")
	}
	assertOffset(t, previous, current, 79)
}

func descriptorCollisionPage(width, height int) *image.RGBA {
	page := image.NewRGBA(image.Rect(0, 0, width, height))
	for i := range page.Pix {
		page.Pix[i] = 255
	}
	var random uint32 = 13
	for y := 0; y < height; y++ {
		random = random*1664525 + 1013904223
		// Odd columns are invisible to the step-four fallback as well as
		// having identical bin means. This requires the dense fallback.
		x := width/2 + 1 + 2*(int(random>>24)%4)
		page.SetRGBA(x, y, color.RGBA{A: 255})
	}
	return page
}

func TestMatcherRejectedContentKeepsBothCaches(t *testing.T) {
	page := createTestImage(160, 800)
	first := crop(page, 0, 400)
	matcher, err := NewMatcher(first, DefaultMatchOptions())
	if err != nil {
		t.Fatal(err)
	}
	before := append([]rowFeature(nil), matcher.rows...)
	bad := image.NewRGBA(first.Bounds())
	for i := 3; i < len(bad.Pix); i += 4 {
		bad.Pix[i] = 255
	}
	result, err := matcher.Analyze(bad)
	if err != nil || result.Matched {
		t.Fatalf("bad frame accepted: %+v, %v", result, err)
	}
	if !reflect.DeepEqual(before, matcher.rows) {
		t.Fatal("rejected frame replaced descriptor cache")
	}
	result, err = matcher.Analyze(crop(page, 137, 400))
	if err != nil || !result.Matched || result.Offset != 137 {
		t.Fatalf("recovery: %+v, %v", result, err)
	}
}

func TestBidirectionalRejectsCorruptionWithoutChangingCanvas(t *testing.T) {
	page := createTestImage(160, 1000)
	first := crop(page, 0, 400)
	stitcher, err := NewBidirectionalStitcher(first, DefaultMatchOptions())
	if err != nil {
		t.Fatal(err)
	}
	current := crop(page, 101, 400)
	for y := 0; y < 240; y++ {
		for x := 0; x < 160; x++ {
			current.SetRGBA(x, y, color.RGBA{A: 255})
		}
	}
	result, err := stitcher.Add(current)
	if err != nil || result.Matched {
		t.Fatalf("corrupt frame accepted: %+v, %v", result, err)
	}
	assertImagesEqual(t, stitcher.Image(), first)
	result, err = stitcher.Add(crop(page, 101, 400))
	if err != nil || !result.Matched || result.Delta != 101 {
		t.Fatalf("recovery: %+v, %v", result, err)
	}
}

func TestRelocationRejectsExactRepeatsDeterministically(t *testing.T) {
	page := repeatedPage(160, 1000, 64)
	stitcher, err := NewBidirectionalStitcher(crop(page, 0, 600), DefaultMatchOptions())
	if err != nil {
		t.Fatal(err)
	}
	current, _, _ := grayscale(crop(page, 17, 600))
	first := stitcher.relocate(current)
	if first.Matched || first.Reason != RejectionAmbiguous {
		t.Fatalf("repeated history accepted: %+v", first)
	}
	for i := 0; i < 5; i++ {
		if got := stitcher.relocate(current); got != first {
			t.Fatalf("relocation changed across identical calls: %+v != %+v", got, first)
		}
	}
}

func TestMatcherRetainsLowContrastSparseText(t *testing.T) {
	page := createSparseTestImage(320, 1000)
	for i := 0; i < len(page.Pix); i += 4 {
		if page.Pix[i] == 0 {
			page.Pix[i], page.Pix[i+1], page.Pix[i+2] = 252, 252, 252
		}
	}
	assertOffset(t, crop(page, 0, 600), crop(page, 41, 600), 41)
}

func TestMatcherStationaryWithSmallNoise(t *testing.T) {
	previous := createTestImage(96, 300)
	current := crop(previous, 0, 300)
	for i := 0; i < len(current.Pix); i += 32 {
		for c := 0; c < 3; c++ {
			if current.Pix[i+c] < 255 {
				current.Pix[i+c]++
			}
		}
	}
	result, err := AnalyzeScroll(previous, current, DefaultMatchOptions())
	if err != nil || result.Matched || result.Reason != RejectionStationary {
		t.Fatalf("stationary noise: %+v, %v", result, err)
	}
}

func TestMatchersSupportNarrowAndNonZeroBounds(t *testing.T) {
	for _, width := range []int{1, 3, 7, 65} {
		page := createTestImage(width+4, 300)
		previous := page.SubImage(image.Rect(2, 13, width+2, 173))
		current := page.SubImage(image.Rect(2, 54, width+2, 214))
		assertOffset(t, previous, current, 41)
		assertOffset(t, genericImage{previous}, genericImage{current}, 41)
	}
}

func TestMatcherDoesNotTreatAdjacentOffsetsAsIndependentPeaks(t *testing.T) {
	page := image.NewRGBA(image.Rect(0, 0, 80, 700))
	for y := 0; y < 700; y++ {
		for x := 0; x < 80; x++ {
			value := uint8(80 + y/8)
			if (x+y)%8 == 0 {
				value++
			}
			page.SetRGBA(x, y, color.RGBA{value, value, value, 255})
		}
	}
	previous, current := crop(page, 0, 400), crop(page, 73, 400)
	for i := 0; i < len(current.Pix); i += 64 {
		current.Pix[i]++
		current.Pix[i+1]++
		current.Pix[i+2]++
	}
	options := DefaultMatchOptions()
	options.MinimumConfidence = 0.5
	result, err := AnalyzeScroll(previous, current, options)
	if err != nil || !result.Matched || result.Offset != 73 || result.SecondBestScore-result.BestScore < options.MinimumConfidence {
		t.Fatalf("broad single peak: %+v, %v", result, err)
	}
}
