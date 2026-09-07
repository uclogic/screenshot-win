package selector

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"testing"
)

func TestDirectionalRectangleFixtures(t *testing.T) {
	target := image.Rect(40, 40, 300, 220)
	for _, name := range []string{"faint", "gaps", "rounded", "thick", "filled", "attached_text", "offset"} {
		t.Run(name, func(t *testing.T) {
			img := rectangleFixture(image.Rect(0, 0, 360, 280), target)
			switch name {
			case "faint":
				for y := target.Min.Y; y < target.Max.Y; y++ {
					for x := target.Min.X; x < target.Max.X; x++ {
						if img.RGBAAt(x, y).R == 0 {
							img.Set(x, y, color.Gray{235})
						}
					}
				}
			case "offset":
				for x := 60; x < 280; x++ {
					img.Set(x, 40, color.White)
					img.Set(x, 38+(x/20)%2*4, color.Black)
				}
			case "gaps":
				for x := 100; x < 106; x++ {
					img.Set(x, 40, color.White)
					img.Set(x, 219, color.White)
				}
				for y := 100; y < 106; y++ {
					img.Set(40, y, color.White)
					img.Set(299, y, color.White)
				}
			case "rounded":
				draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
				// Rasterize a one-pixel rounded border with a ten-pixel radius.
				inside := func(x, y, inset int) bool {
					r := target.Inset(inset)
					radius := 10 - inset
					if !image.Pt(x, y).In(r) {
						return false
					}
					cx := max(r.Min.X+radius, min(x, r.Max.X-1-radius))
					cy := max(r.Min.Y+radius, min(y, r.Max.Y-1-radius))
					return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= radius*radius
				}
				for y := 40; y < 220; y++ {
					for x := 40; x < 300; x++ {
						if inside(x, y, 0) && !inside(x, y, 1) {
							img.Set(x, y, color.Black)
						}
					}
				}
			case "thick":
				draw.Draw(img, target, image.NewUniform(color.Black), image.Point{}, draw.Src)
				draw.Draw(img, target.Inset(5), image.NewUniform(color.White), image.Point{}, draw.Src)
			case "filled":
				draw.Draw(img, target, image.NewUniform(color.Gray{235}), image.Point{}, draw.Src)
			case "attached_text":
				for x := 70; x < 270; x += 20 {
					draw.Draw(img, image.Rect(x, 40, x+3, 56), image.NewUniform(color.Black), image.Point{}, draw.Src)
					draw.Draw(img, image.Rect(x, 53, x+10, 56), image.NewUniform(color.Black), image.Point{}, draw.Src)
				}
			}
			got, err := DetectRectangles(context.Background(), img)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, r := range got {
				if candidateAbs(r.Min.X-target.Min.X) <= 3 && candidateAbs(r.Min.Y-target.Min.Y) <= 3 && candidateAbs(r.Max.X-target.Max.X) <= 3 && candidateAbs(r.Max.Y-target.Max.Y) <= 3 {
					found = true
				}
			}
			if !found || len(got) != 1 {
				t.Fatalf("missing %v: %v", target, got)
			}
		})
	}
}

func TestDirectionalRectangleRejectsUnrelatedEdges(t *testing.T) {
	for _, name := range []string{"three_sides", "text", "mismatched", "long_gap"} {
		t.Run(name, func(t *testing.T) {
			img := rectangleFixture(image.Rect(0, 0, 400, 300))
			line := func(r image.Rectangle) { draw.Draw(img, r, image.NewUniform(color.Black), image.Point{}, draw.Src) }
			switch name {
			case "three_sides", "long_gap":
				line(image.Rect(40, 40, 300, 41))
				line(image.Rect(40, 219, 300, 220))
				line(image.Rect(40, 40, 41, 220))
				if name == "long_gap" {
					line(image.Rect(299, 40, 300, 100))
					line(image.Rect(299, 140, 300, 220))
				}
			case "text":
				for y := 20; y < 260; y += 18 {
					for x := 20; x < 370; x += 12 {
						line(image.Rect(x, y, x+2, y+9))
						line(image.Rect(x, y+7, x+7, y+9))
					}
				}
			case "mismatched":
				line(image.Rect(40, 40, 200, 41))
				line(image.Rect(150, 219, 350, 220))
				line(image.Rect(40, 40, 41, 150))
				line(image.Rect(349, 100, 350, 220))
			}
			got, err := DetectRectangles(context.Background(), img)
			if err != nil || len(got) != 0 {
				t.Fatalf("unexpected %v, %v", got, err)
			}
		})
	}
}

func TestDirectionalRectangleGridAndAdjacent(t *testing.T) {
	img := rectangleFixture(image.Rect(0, 0, 450, 350))
	for x := 30; x <= 390; x += 120 {
		draw.Draw(img, image.Rect(x, 30, x+1, 301), image.NewUniform(color.Black), image.Point{}, draw.Src)
	}
	for y := 30; y <= 300; y += 90 {
		draw.Draw(img, image.Rect(30, y, 391, y+1), image.NewUniform(color.Black), image.Point{}, draw.Src)
	}
	got, err := DetectRectangles(context.Background(), img)
	if err != nil {
		t.Fatal(err)
	}
	for row := 0; row < 3; row++ {
		for col := 0; col < 3; col++ {
			want := image.Rect(30+col*120, 30+row*90, 151+col*120, 121+row*90)
			r, ok := SmallestRectangleAt(got, want.Min.Add(image.Pt(20, 20)))
			if !ok || r != want {
				t.Fatalf("want %v, got %v (%v)", want, r, got)
			}
		}
	}
}

func BenchmarkDirectionalRectangles(b *testing.B) {
	for _, size := range []image.Point{{1920, 1080}, {3840, 2160}} {
		for _, dense := range []bool{false, true} {
			b.Run(fmt.Sprintf("%dx%d/dense=%t", size.X, size.Y, dense), func(b *testing.B) {
				img := rectangleFixture(image.Rectangle{Max: size}, image.Rect(40, 40, size.X-40, size.Y-40), image.Rect(100, 100, 700, 500))
				if dense {
					for x := 30; x < size.X-30; x += 120 {
						draw.Draw(img, image.Rect(x, 30, x+1, size.Y-30), image.NewUniform(color.Black), image.Point{}, draw.Src)
					}
					for y := 30; y < size.Y-30; y += 90 {
						draw.Draw(img, image.Rect(30, y, size.X-30, y+1), image.NewUniform(color.Black), image.Point{}, draw.Src)
					}
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := DetectRectangles(context.Background(), img); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// Cancel after pixel reads have started, rather than before detection begins.
type cancelCandidateImage struct {
	image.Image
	cancel context.CancelFunc
	reads  int
}

func (img *cancelCandidateImage) At(x, y int) color.Color {
	img.reads++
	if img.reads == 1000 {
		img.cancel()
	}
	return img.Image.At(x, y)
}

func TestDirectionalRectangleCancellationDuringDetection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source := &cancelCandidateImage{Image: rectangleFixture(image.Rect(0, 0, 400, 300), image.Rect(30, 30, 350, 250)), cancel: cancel}
	if _, err := DetectRectangles(ctx, source); err != context.Canceled {
		t.Fatalf("got %v", err)
	}
}

func TestDirectionalRectangleAdjacent(t *testing.T) {
	left, right := image.Rect(30, 30, 180, 200), image.Rect(190, 30, 340, 200)
	source := rectangleFixture(image.Rect(0, 0, 400, 250), left, right)
	got, err := DetectRectangles(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []image.Rectangle{left, right} {
		r, ok := SmallestRectangleAt(got, want.Min.Add(image.Pt(20, 20)))
		if !ok || r != want {
			t.Fatalf("want %v, got %v (%v)", want, r, got)
		}
	}
}
