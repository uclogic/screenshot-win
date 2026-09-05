package main

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestLaunchModes(t *testing.T) {
	options, err := parseLaunchArguments(nil)
	if err != nil || options.Debug != nil {
		t.Fatalf("default: %+v %v", options, err)
	}
	options, err = parseLaunchArguments([]string{candidateDebugFlag, "-100", "150", "1200", "700", "page.png"})
	if err != nil {
		t.Fatal(err)
	}
	if options.Debug.Region != image.Rect(-100, 150, 1100, 850) || options.Debug.Output != "page.png" {
		t.Fatal(options.Debug)
	}
	for _, args := range [][]string{
		{"--tray"}, {"--once"}, {"100", "150", "1200", "700", "page.png"}, {"--interval", "100ms"}, {"--max-scroll-ratio", "0.5"}, {"--max-mean-diff", "8"}, {"--min-confidence", "0.25"}, {"--stationary-threshold", "0.5"}, {"--diagnostics", "out"}, {"--diagnostic-limit", "50"},
		{candidateDebugFlag, "0", "0", "0", "80", "x.png"}, {candidateDebugFlag, "0", "0", "100", "-1", "x.png"}, {candidateDebugFlag, "2147483647", "0", "100", "80", "x.png"}, {candidateDebugFlag, "x", "0", "100", "80", "x.png"}, {candidateDebugFlag, "0", "0", "100", "80"}, {candidateDebugFlag, "0", "0", "100", "80", ""},
	} {
		if _, err := parseLaunchArguments(args); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}

func TestDebugDrawsAllQualifyingRectangles(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 500, 350))
	draw.Draw(source, source.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	for _, r := range []image.Rectangle{image.Rect(20, 20, 220, 170), image.Rect(270, 180, 470, 330), image.Rect(300, 30, 320, 50)} {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				if x == r.Min.X || x == r.Max.X-1 || y == r.Min.Y || y == r.Max.Y-1 {
					source.Set(x, y, color.Black)
				}
			}
		}
	}
	path := filepath.Join(t.TempDir(), "all.png")
	count, err := writeCandidateDebug(context.Background(), source, path)
	if err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	result, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []image.Point{{20, 20}, {270, 180}} {
		if color.RGBAModel.Convert(result.At(p.X, p.Y)) != (color.RGBA{22, 140, 255, 255}) {
			t.Errorf("no border at %v", p)
		}
	}
	if color.RGBAModel.Convert(source.At(20, 20)) != (color.RGBA{0, 0, 0, 255}) {
		t.Fatal("source mutated")
	}
}
func TestDebugWritesUnchangedImageWithoutCandidates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.png")
	source := image.NewRGBA(image.Rect(0, 0, 120, 90))
	count, err := writeCandidateDebug(context.Background(), source, path)
	if err != nil || count != 0 {
		t.Fatalf("%d %v", count, err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	got, err := png.Decode(file)
	if err != nil || got.Bounds() != source.Bounds() {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := writeCandidateDebug(context.Background(), source, filepath.Join(t.TempDir(), "missing", "x.png")); err == nil {
		t.Fatal("missing directory accepted")
	}
}
