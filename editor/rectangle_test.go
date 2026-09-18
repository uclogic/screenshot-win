package editor

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestRectangleLayoutDPIAndHollowCenters(t *testing.T) {
	for _, dpi := range []int{96, 120, 144, 168, 192} {
		for _, width := range []float64{2, 3, 5, 8, 32} {
			l := LayoutRectangle(image.Pt(20, 20), image.Pt(400, 300), dpi, width)
			if l.Count != 8 || l.SegmentCount != 8 {
				t.Fatalf("dpi=%d layout=%+v", dpi, l)
			}
			if l.Radius != 4.5*float64(dpi)/96 {
				t.Fatal("radius is not DPI scaled")
			}
			for i := 0; i < l.Count; i++ {
				h := l.Handles[i]
				p := image.Pt(int(h.Point.X), int(h.Point.Y))
				if got, ok := l.Hit(p); !ok || got != h.Kind {
					t.Fatalf("handle %v hit=%v %v", h.Kind, got, ok)
				}
				for y := -2; y <= 2; y++ {
					for x := -2; x <= 2; x++ {
						q := ScreenPoint{h.Point.X + float64(x), h.Point.Y + float64(y)}
						if l.BorderCoverage(q, width) != 0 || l.HandleCoverage(q) != 0 {
							t.Fatalf("dpi=%d width=%v handle=%v center is painted", dpi, width, h.Kind)
						}
					}
				}
			}
		}
	}
}

func TestRectangleSmallHandlesAndRoundHitRegions(t *testing.T) {
	for _, tc := range []struct{ w, h, count int }{{80, 60, 8}, {20, 60, 6}, {80, 20, 6}, {16, 16, 4}, {1, 1, 4}} {
		l := LayoutRectangle(image.Pt(0, 0), image.Pt(tc.w, tc.h), 96, 3)
		if l.Count != tc.count {
			t.Fatalf("size=%dx%d count=%d", tc.w, tc.h, l.Count)
		}
		for i := 0; i < l.SegmentCount; i++ {
			s := l.Segments[i]
			if s.End.X < s.Start.X || s.End.Y < s.Start.Y {
				t.Fatal("reversed short segment")
			}
		}
	}
	l := LayoutRectangle(image.Pt(20, 20), image.Pt(120, 100), 96, 3)
	if _, ok := l.Hit(image.Pt(27, 27)); ok {
		t.Fatal("square hit region instead of circle")
	}
	if got, ok := l.Hit(image.Pt(28, 20)); !ok || got != HandleRectangleNorthWest {
		t.Fatal("hit radius not expanded")
	}
	small := LayoutRectangle(image.Pt(0, 0), image.Pt(16, 16), 96, 3)
	if got, ok := small.Hit(image.Pt(8, 0)); !ok || got != HandleRectangleNorthWest {
		t.Fatal("hidden midpoint remained hittable or tie unstable")
	}
	if got, ok := small.Hit(image.Pt(10, 0)); !ok || got != HandleRectangleNorthEast {
		t.Fatal("did not choose nearest handle")
	}
}

func TestResizeRectangleDeltaAndConstraints(t *testing.T) {
	a := Annotation{Tool: ToolRectangle, Start: image.Pt(20, 20), End: image.Pt(60, 60), Style: DefaultStyle()}
	b := image.Rect(0, 0, 100, 100)
	for _, tc := range []struct {
		h          TransformHandle
		start, end image.Point
	}{
		{HandleRectangleNorthWest, image.Pt(25, 27), image.Pt(60, 60)},
		{HandleRectangleNorth, image.Pt(20, 27), image.Pt(60, 60)},
		{HandleRectangleNorthEast, image.Pt(20, 27), image.Pt(65, 60)},
		{HandleRectangleEast, image.Pt(20, 20), image.Pt(65, 60)},
		{HandleRectangleSouthEast, image.Pt(20, 20), image.Pt(65, 67)},
		{HandleRectangleSouth, image.Pt(20, 20), image.Pt(60, 67)},
		{HandleRectangleSouthWest, image.Pt(25, 20), image.Pt(60, 67)},
		{HandleRectangleWest, image.Pt(25, 20), image.Pt(60, 60)},
	} {
		got := ResizeRectangle(a, tc.h, image.Pt(5, 7), b, image.Pt(16, 16))
		if got.Start != tc.start || got.End != tc.end {
			t.Fatalf("handle=%v result=%v..%v", tc.h, got.Start, got.End)
		}
		if still := ResizeRectangle(a, tc.h, image.Point{}, b, image.Pt(16, 16)); still != a {
			t.Fatal("pointer down changed geometry")
		}
	}
	for _, h := range []TransformHandle{HandleRectangleNorthWest, HandleRectangleNorthEast, HandleRectangleSouthEast, HandleRectangleSouthWest} {
		for _, delta := range []image.Point{{1000, 1000}, {-1000, -1000}} {
			got := ResizeRectangle(a, h, delta, b, image.Pt(16, 16))
			if got.End.X-got.Start.X < 16 || got.End.Y-got.Start.Y < 16 || !got.Start.In(b) || !got.End.In(b) {
				t.Fatalf("constraint failed: %+v", got)
			}
		}
	}
	a.End = image.Pt(24, 26)
	if got := ResizeRectangle(a, HandleRectangleNorthWest, image.Point{}, b, image.Pt(16, 16)); got != a {
		t.Fatal("old small rectangle jumped")
	}
	if got := ResizeRectangle(a, HandleRectangleSouthEast, image.Pt(20, 20), b, image.Pt(16, 16)); got.End != image.Pt(44, 46) {
		t.Fatal("old small rectangle cannot grow")
	}
}

func TestRectanglePreviewPreservesBackgroundOrderAndExport(t *testing.T) {
	source := image.NewNRGBA(image.Rect(7, 9, 167, 129))
	for y := source.Rect.Min.Y; y < source.Rect.Max.Y; y++ {
		for x := source.Rect.Min.X; x < source.Rect.Max.X; x++ {
			source.SetNRGBA(x, y, color.NRGBA{uint8(x), uint8(y), 70, 255})
		}
	}
	doc, _ := NewDocument(source)
	bottom := Annotation{Tool: ToolArrow, Start: image.Pt(5, 20), End: image.Pt(100, 20), Style: Style{Color: color.NRGBA{0, 180, 0, 255}, Width: 3}}
	doc.Add(bottom)
	id, _ := doc.Add(Annotation{Tool: ToolRectangle, Start: image.Pt(20, 20), End: image.Pt(120, 100), Style: DefaultStyle()})
	doc.Add(Annotation{Tool: ToolArrow, Start: image.Pt(100, 5), End: image.Pt(100, 110), Style: Style{Color: color.NRGBA{0, 0, 255, 255}, Width: 3}})
	a, _ := doc.Get(id)
	preview := doc.NewRectanglePreview(id)
	v := Viewport{Scale: 1}
	preview.Update(a, v, 96)
	without := doc.RenderedWithout(id)
	for _, point := range []image.Point{{20, 20}, {70, 20}, {120, 60}, {70, 100}} {
		want := color.NRGBAModel.Convert(without.At(point.X, point.Y)).(color.NRGBA)
		if got := preview.ScreenColor(point); got != want {
			t.Fatalf("center %v got=%v want=%v", point, got, want)
		}
	}
	if got := preview.ScreenColor(image.Pt(100, 20)); got.B != 255 {
		t.Fatalf("selected rectangle covered higher annotation: %v", got)
	}
	if got := color.NRGBAModel.Convert(doc.Rendered().At(70, 20)).(color.NRGBA); got != a.Style.Color {
		t.Fatalf("export contains gap: %v", got)
	}
	// Updating the transient preview must not mutate document geometry/history.
	moved := a
	moved.Start.X += 5
	moved.End.X += 5
	preview.Update(moved, v, 192)
	if original, _ := doc.Get(id); original != a {
		t.Fatal("preview mutated document")
	}
	if !doc.Undo() {
		t.Fatal("missing history")
	}
	if len(doc.Annotations()) != 2 {
		t.Fatal("preview added an undo record")
	}
}

func TestRectangleGeometryAndPreviewNoFrameAllocations(t *testing.T) {
	doc, _ := NewDocument(image.NewNRGBA(image.Rect(0, 0, 500, 300)))
	id, _ := doc.Add(Annotation{Tool: ToolRectangle, Start: image.Pt(20, 20), End: image.Pt(400, 200), Style: DefaultStyle()})
	a, _ := doc.Get(id)
	p := doc.NewRectanglePreview(id)
	allocs := testing.AllocsPerRun(100, func() {
		p.Update(a, Viewport{Scale: 1.25}, 144)
		p.ScreenColor(image.Pt(30, 25))
		l := LayoutRectangle(a.Start, a.End, 144, 3)
		l.Hit(image.Pt(20, 20))
		l.HandleCoverage(ScreenPoint{20, 20})
		ResizeRectangle(a, HandleRectangleSouthEast, image.Pt(2, 3), doc.Bounds(), image.Pt(16, 16))
	})
	if allocs != 0 {
		t.Fatalf("allocations per frame=%v", allocs)
	}
}

func BenchmarkRectanglePreview4K(b *testing.B) {
	doc, _ := NewDocument(image.NewRGBA(image.Rect(0, 0, 3840, 2160)))
	id, _ := doc.Add(Annotation{Tool: ToolRectangle, Start: image.Pt(200, 200), End: image.Pt(3600, 1900), Style: DefaultStyle()})
	a, _ := doc.Get(id)
	p := doc.NewRectanglePreview(id)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Update(a, Viewport{Scale: 1}, 144)
		// A 4K-wide edge band, representative of local rectangle repaint work.
		for y := 190; y < 210; y++ {
			for x := 190; x < 3610; x++ {
				p.ScreenColor(image.Pt(x, y))
			}
		}
	}
}

func TestFractionalHandleCenters(t *testing.T) {
	l := LayoutRectangle(image.Pt(10, 10), image.Pt(91, 91), 120, 3)
	if math.Abs(l.Handles[1].Point.X-50.5) > 0.001 {
		t.Fatal("fractional midpoint lost")
	}
}
