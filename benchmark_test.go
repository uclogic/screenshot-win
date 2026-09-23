package screenshotwin

import (
	"image"
	"testing"
)

func BenchmarkMatcherSequence1200x800(b *testing.B) {
	const (
		width          = 1200
		viewportHeight = 800
		offset         = 300
		frameCount     = 12
	)
	source := createTestImage(width, viewportHeight+offset*(frameCount-1))
	frames := make([]image.Image, frameCount)
	for index := range frames {
		frames[index] = crop(source, index*offset, viewportHeight)
	}
	options := DefaultMatchOptions()

	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		matcher, err := NewMatcher(frames[0], options)
		if err != nil {
			b.Fatal(err)
		}
		for _, frame := range frames[1:] {
			result, analyzeErr := matcher.Analyze(frame)
			if analyzeErr != nil || !result.Matched {
				b.Fatalf("Analyze() = (%+v, %v)", result, analyzeErr)
			}
		}
	}
}

func BenchmarkBuilderSequence1200x800(b *testing.B) {
	const (
		width          = 1200
		viewportHeight = 800
		offset         = 300
		frameCount     = 30
	)
	source := createTestImage(width, viewportHeight+offset*(frameCount-1))
	frames := make([]image.Image, frameCount)
	for index := range frames {
		frames[index] = crop(source, index*offset, viewportHeight)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		builder, err := NewBuilder(frames[0])
		if err != nil {
			b.Fatal(err)
		}
		for _, frame := range frames[1:] {
			if appendErr := builder.Append(frame, offset); appendErr != nil {
				b.Fatal(appendErr)
			}
		}
		result := builder.Finish()
		if result.Bounds().Dy() != source.Bounds().Dy() {
			b.Fatalf("result height = %d, want %d", result.Bounds().Dy(), source.Bounds().Dy())
		}
	}
}

func BenchmarkBidirectionalSequence1200x800(b *testing.B) {
	const (
		width          = 1200
		viewportHeight = 800
		offset         = 300
		frameCount     = 30
	)
	source := createTestImage(width, viewportHeight+offset*(frameCount-1))
	frames := make([]image.Image, frameCount)
	for index := range frames {
		frames[index] = crop(source, index*offset, viewportHeight)
	}
	options := DefaultMatchOptions()

	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		stitcher, err := NewBidirectionalStitcher(frames[0], options)
		if err != nil {
			b.Fatal(err)
		}
		for _, frame := range frames[1:] {
			result, addErr := stitcher.Add(frame)
			if addErr != nil || !result.Matched {
				b.Fatalf("Add() = (%+v, %v)", result, addErr)
			}
		}
		result := stitcher.Finish()
		if result.Bounds().Dy() != source.Bounds().Dy() {
			b.Fatalf("result height = %d, want %d", result.Bounds().Dy(), source.Bounds().Dy())
		}
	}
}

// Include fallback and rejection costs, not just the ordinary scrolling path.
func BenchmarkMatcherScenarios(b *testing.B) {
	const width, height = 1200, 800
	for _, name := range []string{"stationary", "smooth_scroll", "sparse", "repeated", "descriptor_fallback", "unrelated"} {
		b.Run(name, func(b *testing.B) {
			var previous, current image.Image
			wantOffset := 0
			switch name {
			case "stationary":
				previous = createTestImage(width, height)
				current = previous
			case "smooth_scroll":
				page := motionTextPage(width, height+400)
				previous, current = crop(page, 0, height), motionFrame(page, 299, height, 20)
				wantOffset = 299
			case "sparse":
				page := createSparseTestImage(width, height+200)
				previous, current = crop(page, 0, height), crop(page, 41, height)
				wantOffset = 41
			case "repeated":
				page := repeatedPage(width, height+200, 64)
				previous, current = crop(page, 0, height), crop(page, 17, height)
			case "descriptor_fallback":
				page := descriptorCollisionPage(width, height+200)
				previous, current = crop(page, 0, height), crop(page, 179, height)
				wantOffset = 179
			case "unrelated":
				page := createTestImage(width, height*2)
				previous, current = crop(page, 0, height), crop(page, height, height)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				matcher, err := NewMatcher(previous, DefaultMatchOptions())
				if err != nil {
					b.Fatal(err)
				}
				result, err := matcher.Analyze(current)
				if err != nil || result.Matched != (wantOffset != 0) || result.Matched && result.Offset != wantOffset {
					b.Fatalf("unexpected result: %+v, %v", result, err)
				}
			}
		})
	}
}
