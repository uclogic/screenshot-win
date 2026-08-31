package screenshotwin

import (
	"image"
	"image/color"
	"math"
	"reflect"
	"testing"
)

func TestFindScrollOffset300Pixels(t *testing.T) {
	source := createTestImage(320, 1800)
	first := crop(source, 0, 600)
	second := crop(source, 300, 600)
	assertOffset(t, first, second, 300)
}

func TestFindScrollOffset527Pixels(t *testing.T) {
	source := createTestImage(320, 2400)
	first := crop(source, 100, 1200)
	second := crop(source, 627, 1200)
	assertOffset(t, first, second, 527)
}

func TestFindScrollOffsetNoScroll(t *testing.T) {
	frame := crop(createTestImage(320, 900), 100, 600)
	if offset, ok := FindScrollOffset(frame, frame); ok {
		t.Fatalf("FindScrollOffset() = (%d, true), want no match", offset)
	}
}

func TestFindScrollOffsetNoScrollOnFlatFrame(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 320, 600))
	if offset, ok := FindScrollOffset(frame, frame); ok {
		t.Fatalf("FindScrollOffset() = (%d, true), want no match", offset)
	}
}

func TestAnalyzeScrollReportsStationaryReason(t *testing.T) {
	frame := crop(createTestImage(320, 900), 100, 600)
	result, err := AnalyzeScroll(frame, frame, DefaultMatchOptions())
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched || result.Reason != RejectionStationary {
		t.Fatalf("AnalyzeScroll() = %+v, want stationary rejection", result)
	}
}

func TestAnalyzeScrollReportsSizeMismatch(t *testing.T) {
	previous := createTestImage(320, 600)
	current := createTestImage(321, 600)
	result, err := AnalyzeScroll(previous, current, DefaultMatchOptions())
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched || result.Reason != RejectionSizeMismatch {
		t.Fatalf("AnalyzeScroll() = %+v, want size mismatch", result)
	}
}

func TestAnalyzeScrollHonorsMaximumOffsetRatio(t *testing.T) {
	source := createTestImage(320, 1200)
	previous := crop(source, 0, 600)
	current := crop(source, 300, 600)
	options := DefaultMatchOptions()
	options.MaxOffsetRatio = 0.4
	result, err := AnalyzeScroll(previous, current, options)
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched {
		t.Fatalf("AnalyzeScroll() = %+v, want rejection above configured range", result)
	}
}

func TestAnalyzeScrollReportsScoreTooHigh(t *testing.T) {
	previous := image.NewRGBA(image.Rect(0, 0, 40, 40))
	current := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			current.SetRGBA(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	result, err := AnalyzeScroll(previous, current, DefaultMatchOptions())
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched || result.Reason != RejectionScoreTooHigh {
		t.Fatalf("AnalyzeScroll() = %+v, want score-too-high rejection", result)
	}
}

func TestAnalyzeScrollHonorsConfidenceThreshold(t *testing.T) {
	source := createTestImage(320, 1200)
	previous := crop(source, 0, 600)
	current := crop(source, 200, 600)
	// Keep the real overlap but introduce a small difference so the exact-match
	// shortcut does not bypass the configured confidence threshold.
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			current.SetRGBA(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	options := DefaultMatchOptions()
	options.MinimumConfidence = 256
	result, err := AnalyzeScroll(previous, current, options)
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched || result.Reason != RejectionAmbiguous {
		t.Fatalf("AnalyzeScroll() = %+v, want ambiguous rejection", result)
	}
}

func TestMatchOptionsValidation(t *testing.T) {
	for _, ratio := range []float64{0, 1, math.NaN(), math.Inf(1)} {
		options := DefaultMatchOptions()
		options.MaxOffsetRatio = ratio
		if _, err := AnalyzeScroll(createTestImage(10, 10), createTestImage(10, 10), options); err == nil {
			t.Errorf("AnalyzeScroll() accepted maximum offset ratio %v", ratio)
		}
	}
}

func TestMatcherMatchesStatelessResults(t *testing.T) {
	source := createTestImage(320, 1800)
	frames := []image.Image{
		crop(source, 0, 600),
		crop(source, 200, 600),
		crop(source, 475, 600),
		crop(source, 700, 600),
	}
	matcher, err := NewMatcher(frames[0], DefaultMatchOptions())
	if err != nil {
		t.Fatal(err)
	}
	previous := frames[0]
	for index, current := range frames[1:] {
		want, analyzeErr := AnalyzeScroll(previous, current, DefaultMatchOptions())
		if analyzeErr != nil {
			t.Fatal(analyzeErr)
		}
		got, analyzeErr := matcher.Analyze(current)
		if analyzeErr != nil {
			t.Fatal(analyzeErr)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("frame %d: Matcher.Analyze() = %+v, want %+v", index+1, got, want)
		}
		previous = current
	}
}

func TestMatcherKeepsBaselineAfterRejectedFrames(t *testing.T) {
	source := createTestImage(320, 1200)
	first := crop(source, 0, 600)
	matcher, err := NewMatcher(first, DefaultMatchOptions())
	if err != nil {
		t.Fatal(err)
	}

	stationary, err := matcher.Analyze(first)
	if err != nil || stationary.Reason != RejectionStationary {
		t.Fatalf("stationary result = (%+v, %v)", stationary, err)
	}
	sizeMismatch, err := matcher.Analyze(createTestImage(319, 600))
	if err != nil || sizeMismatch.Reason != RejectionSizeMismatch {
		t.Fatalf("size mismatch result = (%+v, %v)", sizeMismatch, err)
	}

	matched, err := matcher.Analyze(crop(source, 300, 600))
	if err != nil {
		t.Fatal(err)
	}
	if !matched.Matched || matched.Offset != 300 {
		t.Fatalf("recovery result = %+v, want 300px match", matched)
	}
}

func TestNewMatcherRejectsInvalidInitialFrame(t *testing.T) {
	if _, err := NewMatcher(nil, DefaultMatchOptions()); err == nil {
		t.Fatal("NewMatcher(nil) succeeded, want error")
	}
	options := DefaultMatchOptions()
	options.MaxOffsetRatio = 1
	if _, err := NewMatcher(createTestImage(10, 10), options); err == nil {
		t.Fatal("NewMatcher() accepted invalid options")
	}
}

func TestRGBAFastGrayscaleMatchesGenericConversion(t *testing.T) {
	source := createTestImage(80, 100)
	subImage := source.SubImage(image.Rect(10, 20, 70, 90)).(*image.RGBA)
	fast, fastWidth, fastHeight := grayscale(subImage)
	generic, genericWidth, genericHeight := grayscale(genericImage{subImage})
	if fastWidth != genericWidth || fastHeight != genericHeight || !reflect.DeepEqual(fast, generic) {
		t.Fatal("RGBA grayscale fast path differs from generic image conversion")
	}
}

func TestBuilderGrowsAndRestoresOriginal(t *testing.T) {
	const (
		width          = 320
		sourceHeight   = 3000
		viewportHeight = 800
	)
	source := createTestImage(width, sourceHeight)
	positions := []int{0, 300, 650, 930, 1327, 1700, 2000, 2200}
	builder, err := NewBuilder(crop(source, positions[0], viewportHeight))
	if err != nil {
		t.Fatal(err)
	}
	for index := 1; index < len(positions); index++ {
		offset := positions[index] - positions[index-1]
		if err := builder.Append(crop(source, positions[index], viewportHeight), offset); err != nil {
			t.Fatalf("append %d: %v", index, err)
		}
	}
	if builder.Height() != sourceHeight {
		t.Fatalf("Builder.Height() = %d, want %d", builder.Height(), sourceHeight)
	}
	assertImagesEqual(t, builder.Finish(), source)
}

func TestBuilderSupportsNonZeroSourceBounds(t *testing.T) {
	source := createTestImage(360, 1000)
	first := source.SubImage(image.Rect(20, 100, 340, 600))
	current := source.SubImage(image.Rect(20, 300, 340, 800))
	builder, err := NewBuilder(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.Append(current, 200); err != nil {
		t.Fatal(err)
	}
	want := image.NewRGBA(image.Rect(0, 0, 320, 700))
	for y := 0; y < 700; y++ {
		for x := 0; x < 320; x++ {
			want.Set(x, y, source.At(x+20, y+100))
		}
	}
	assertImagesEqual(t, builder.Finish(), want)
}

func TestBuilderImageReturnsCurrentViewWithoutFinishing(t *testing.T) {
	source := createTestImage(40, 80)
	builder, err := NewBuilder(crop(source, 0, 40))
	if err != nil {
		t.Fatal(err)
	}
	if got := builder.Image(); got == nil {
		t.Fatal("Builder.Image() returned nil")
	} else if got.Bounds() != image.Rect(0, 0, 40, 40) {
		t.Fatalf("Builder.Image() bounds = %v, want (0,0)-(40,40)", got.Bounds())
	}
	if err := builder.Append(crop(source, 20, 40), 20); err != nil {
		t.Fatalf("Append after Image: %v", err)
	}
	view := builder.Image()
	if view.Bounds() != image.Rect(0, 0, 40, 60) {
		t.Fatalf("Builder.Image() bounds = %v, want (0,0)-(40,60)", view.Bounds())
	}
	assertImagesEqual(t, view, crop(source, 0, 60))
	assertImagesEqual(t, builder.Finish(), crop(source, 0, 60))
}

func TestBuilderImageHandlesNilAndZeroValue(t *testing.T) {
	var builder *Builder
	if builder.Image() != nil {
		t.Fatal("nil Builder.Image() returned a non-nil image")
	}
	if (&Builder{}).Image() != nil {
		t.Fatal("zero-value Builder.Image() returned a non-nil image")
	}
}

func TestBuilderValidatesAppendAndFinishState(t *testing.T) {
	if _, err := NewBuilder(nil); err == nil {
		t.Fatal("NewBuilder(nil) succeeded, want error")
	}
	if err := (&Builder{}).Append(createTestImage(40, 40), 10); err == nil {
		t.Fatal("zero-value Builder.Append() succeeded, want error")
	}
	builder, err := NewBuilder(createTestImage(40, 40))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		image  image.Image
		offset int
	}{
		{nil, 10},
		{createTestImage(41, 40), 10},
		{createTestImage(40, 40), 0},
		{createTestImage(40, 40), 41},
	} {
		if err := builder.Append(test.image, test.offset); err == nil {
			t.Fatalf("Builder.Append(%v, %d) succeeded, want error", test.image, test.offset)
		}
	}
	builder.Finish()
	if err := builder.Append(createTestImage(40, 40), 10); err == nil {
		t.Fatal("Builder.Append() after Finish succeeded, want error")
	}
}

func TestFullStitchRestoresOriginal(t *testing.T) {
	const (
		width          = 320
		sourceHeight   = 3000
		viewportHeight = 800
	)
	source := createTestImage(width, sourceHeight)
	positions := []int{0, 300, 650, 930, 1327, 1700, 2000, 2200}
	frames := make([]image.Image, len(positions))
	for index, position := range positions {
		frames[index] = crop(source, position, viewportHeight)
	}

	result, err := Stitch(frames)
	if err != nil {
		t.Fatal(err)
	}
	assertImagesEqual(t, result, source)
}

func assertImagesEqual(t *testing.T, got, want image.Image) {
	t.Helper()
	if got.Bounds().Dx() != want.Bounds().Dx() || got.Bounds().Dy() != want.Bounds().Dy() {
		t.Fatalf("result size = %v, want %v", got.Bounds(), want.Bounds())
	}
	gotBounds, wantBounds := got.Bounds(), want.Bounds()
	for y := 0; y < wantBounds.Dy(); y++ {
		for x := 0; x < wantBounds.Dx(); x++ {
			gotColor := color.RGBAModel.Convert(got.At(gotBounds.Min.X+x, gotBounds.Min.Y+y)).(color.RGBA)
			wantColor := color.RGBAModel.Convert(want.At(wantBounds.Min.X+x, wantBounds.Min.Y+y)).(color.RGBA)
			if gotColor != wantColor {
				t.Fatalf("pixel (%d, %d) differs: got %v, want %v", x, y, gotColor, wantColor)
			}
		}
	}
}

type genericImage struct {
	image.Image
}

func assertOffset(t *testing.T, previous, current image.Image, expected int) {
	t.Helper()
	offset, ok := FindScrollOffset(previous, current)
	if !ok || offset != expected {
		t.Fatalf("FindScrollOffset() = (%d, %t), want (%d, true)", offset, ok, expected)
	}
}

func createTestImage(width, height int) *image.RGBA {
	result := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Mix multiple non-power-of-two periods so false vertical matches are
			// very unlikely while keeping test fixtures deterministic.
			result.SetRGBA(x, y, color.RGBA{
				R: uint8((x*17 + y*13 + (y/37)*29) % 256),
				G: uint8((x*7 + y*19 + (x*y)%251) % 256),
				B: uint8((x*23 + y*5 + (y/53)*41) % 256),
				A: 255,
			})
		}
	}
	return result
}

func crop(source *image.RGBA, y, height int) *image.RGBA {
	result := image.NewRGBA(image.Rect(0, 0, source.Bounds().Dx(), height))
	for row := 0; row < height; row++ {
		copy(result.Pix[row*result.Stride:row*result.Stride+result.Stride], source.Pix[(y+row)*source.Stride:(y+row)*source.Stride+source.Stride])
	}
	return result
}
