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
