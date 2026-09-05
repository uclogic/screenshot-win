package selector

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"testing"
)

func rectangleFixture(bounds image.Rectangle, rectangles ...image.Rectangle) *image.RGBA {
	img := image.NewRGBA(bounds)
	draw.Draw(img, bounds, image.NewUniform(color.White), image.Point{}, draw.Src)
	for _, r := range rectangles {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				if x == r.Min.X || x == r.Max.X-1 || y == r.Min.Y || y == r.Max.Y-1 {
					img.Set(x, y, color.Black)
				}
			}
		}
	}
	return img
}

func TestDetectNestedRectangles(t *testing.T) {
	outer := image.Rect(20, 20, 580, 420)
	inner := image.Rect(140, 110, 400, 310)
	img := rectangleFixture(image.Rect(0, 0, 600, 450), outer, inner, image.Rect(180, 160, 220, 185))
	got, err := DetectRectangles(context.Background(), img)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("rectangles=%v", got)
	}
	r, ok := SmallestRectangleAt(got, image.Pt(200, 170))
	if !ok || candidateAbs(r.Min.X-inner.Min.X) > 3 || candidateAbs(r.Max.Y-inner.Max.Y) > 3 {
		t.Fatalf("inner=%v %v", r, ok)
	}
	r, ok = SmallestRectangleAt(got, image.Pt(50, 50))
	if !ok || candidateAbs(r.Min.X-outer.Min.X) > 3 {
		t.Fatalf("outer=%v %v", r, ok)
	}
	if _, ok := SmallestRectangleAt(got, image.Pt(0, 0)); ok {
		t.Fatal("outside accepted")
	}
}

func TestCandidateImageOriginAndDrawing(t *testing.T) {
	img := rectangleFixture(image.Rect(-30, 40, 370, 340), image.Rect(0, 70, 300, 300))
	rects, err := DetectRectangles(context.Background(), img)
	if err != nil || len(rects) != 1 {
		t.Fatalf("%v %v", rects, err)
	}
	if candidateAbs(rects[0].Min.X-30) > 3 || candidateAbs(rects[0].Min.Y-30) > 3 {
		t.Fatal(rects)
	}
	out := DrawCandidateRectangles(img, rects)
	if out.Bounds() != image.Rect(0, 0, 400, 300) {
		t.Fatal(out.Bounds())
	}
	if got := out.RGBAAt(rects[0].Min.X, rects[0].Min.Y); got != (color.RGBA{22, 140, 255, 255}) {
		t.Fatal(got)
	}
	if img.RGBAAt(100, 150) != (color.RGBA{255, 255, 255, 255}) {
		t.Fatal("source changed")
	}
}

func TestCandidateBlankNonRectangleAndCancellation(t *testing.T) {
	img := rectangleFixture(image.Rect(0, 0, 400, 300))
	for y := 40; y < 260; y++ {
		for x := 50; x < 350; x++ {
			dx, dy := x-200, y-150
			if dx*dx*10000+dy*dy*22500 < 225000000 {
				img.Set(x, y, color.Black)
			}
		}
	}
	got, err := DetectRectangles(context.Background(), img)
	if err != nil || len(got) != 0 {
		t.Fatalf("ellipse: %v %v", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := DetectRectangles(ctx, img); err != context.Canceled {
		t.Fatal(err)
	}
}

func TestCandidateMinimumDimensionsAndGap(t *testing.T) {
	for _, size := range []image.Point{{100, 80}, {99, 80}, {100, 79}, {80, 60}} {
		img := rectangleFixture(image.Rect(0, 0, 250, 200), image.Rect(30, 30, 30+size.X, 30+size.Y))
		rects, err := DetectRectangles(context.Background(), img)
		if err != nil {
			t.Fatal(err)
		}
		want := size.X >= 100 && size.Y >= 80
		if (len(rects) > 0) != want {
			t.Errorf("size %v: %v", size, rects)
		}
	}
	img := rectangleFixture(image.Rect(0, 0, 400, 300), image.Rect(30, 30, 350, 250))
	img.Set(180, 30, color.White)
	rects, err := DetectRectangles(context.Background(), img)
	if err != nil || len(rects) != 1 {
		t.Fatalf("gap: %v %v", rects, err)
	}
}

func TestFrozenCandidatePixels(t *testing.T) {
	source := rectangleFixture(image.Rect(-10, -20, 110, 80))
	r := image.Rect(20, 20, 100, 90)
	pixels := make([]byte, 120*100*4)
	drawFrozenCandidate(pixels, source, r)
	if pixels[0] != 207 || pixels[3] != 255 {
		t.Fatal("background not frozen and shaded")
	}
	i := (50*120 + 50) * 4
	if pixels[i] != 255 || pixels[i+3] != 255 {
		t.Fatal("candidate interior changed")
	}
	i = (20*120 + 20) * 4
	if pixels[i] != 255 || pixels[i+1] != 140 || pixels[i+2] != 22 {
		t.Fatal("missing border")
	}
}

func TestSmallestRectangleHalfOpen(t *testing.T) {
	r := image.Rect(0, 0, 100, 80)
	for _, p := range []image.Point{{100, 40}, {50, 80}} {
		if _, ok := SmallestRectangleAt([]image.Rectangle{r}, p); ok {
			t.Fatal(p)
		}
	}
}

func TestCandidateGesture(t *testing.T) {
	bounds := image.Rect(0, 0, 800, 600)
	r := image.Rect(20, 20, 200, 180)
	g := candidateGesture{threshold: 4}
	g.down(image.Pt(100, 100), r)
	got, ok := g.up(image.Pt(102, 101), bounds)
	if !ok || got != r {
		t.Fatalf("click=%v %v", got, ok)
	}
	g.down(image.Pt(100, 100), r)
	g.move(image.Pt(95, 95))
	got, ok = g.up(image.Pt(90, 80), bounds)
	if !ok || got != image.Rect(90, 80, 100, 100) {
		t.Fatal(got)
	}
	g.down(image.Pt(100, 100), r)
	g.move(image.Pt(120, 120))
	got, ok = g.up(image.Pt(100, 100), bounds)
	if ok || !got.Empty() {
		t.Fatal("drag turned into click")
	}
}

func BenchmarkDetectRectangles(b *testing.B) {
	for _, size := range []image.Point{{2560, 1440}, {3840, 2160}} {
		b.Run(size.String(), func(b *testing.B) {
			img := rectangleFixture(image.Rectangle{Max: size}, image.Rect(100, 100, size.X-100, size.Y-100), image.Rect(200, 200, 800, 700))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := DetectRectangles(context.Background(), img); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkCandidateQuery(b *testing.B) {
	rects := make([]image.Rectangle, 1000)
	for i := range rects {
		rects[i] = image.Rect(i, i, i+100, i+80)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		SmallestRectangleAt(rects, image.Pt(900, 910))
	}
}
