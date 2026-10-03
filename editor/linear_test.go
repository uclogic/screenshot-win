package editor

import (
	"image"
	"image/color"
	"testing"
)

func TestLinearFormatsRenderAndHitExpectedHeads(t *testing.T) {
	for _, tc := range []struct {
		name               string
		tool               Tool
		startHead, endHead bool
	}{
		{"arrow", ToolArrow, false, true},
		{"double arrow", ToolDoubleArrow, true, true},
		{"line", ToolLine, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, _ := NewDocument(image.NewRGBA(image.Rect(0, 0, 120, 100)))
			annotation := Annotation{Tool: tc.tool, Start: image.Pt(20, 50), End: image.Pt(100, 50), Style: DefaultStyle()}
			if _, err := doc.Add(annotation); err != nil {
				t.Fatal(err)
			}
			rendered := doc.Rendered()
			for _, point := range []struct {
				point   image.Point
				painted bool
			}{
				{image.Pt(60, 50), true},
				{image.Pt(44, 37), tc.startHead},
				{image.Pt(76, 37), tc.endHead},
				{image.Pt(60, 20), false},
			} {
				got := color.NRGBAModel.Convert(rendered.At(point.point.X, point.point.Y)).(color.NRGBA)
				if (got == annotation.Style.Color) != point.painted {
					t.Errorf("pixel %v = %v, painted=%v", point.point, got, point.painted)
				}
				_, hit := doc.HitTest(point.point, 0)
				if hit != point.painted {
					t.Errorf("hit %v = %v, want %v", point.point, hit, point.painted)
				}
				if point.painted && !point.point.In(AnnotationBounds(annotation)) {
					t.Errorf("bounds omit %v", point.point)
				}
			}
		})
	}
}

func TestLinearFormatsTransformAndHistory(t *testing.T) {
	for _, tool := range []Tool{ToolArrow, ToolLine, ToolDoubleArrow} {
		doc, _ := NewDocument(image.NewRGBA(image.Rect(0, 0, 120, 100)))
		original := Annotation{Tool: tool, Start: image.Pt(20, 50), End: image.Pt(100, 50), Style: DefaultStyle()}
		id, err := doc.Add(original)
		if err != nil {
			t.Fatal(err)
		}
		original, _ = doc.Get(id)
		changed := TransformTo(original, HandleArrowEnd, image.Pt(500, -20), doc.Bounds())
		if changed.End != image.Pt(119, 0) || changed.Start != original.Start || changed.Tool != tool || changed.Style != original.Style {
			t.Fatalf("tool %v transform=%+v", tool, changed)
		}
		if err := doc.Replace(id, changed); err != nil {
			t.Fatal(err)
		}
		if !doc.Undo() {
			t.Fatal("missing undo")
		}
		restored, _ := doc.Get(id)
		if restored != original {
			t.Fatalf("undo=%+v, want %+v", restored, original)
		}
		if !doc.Redo() {
			t.Fatal("missing redo")
		}
		restored, _ = doc.Get(id)
		if restored != changed {
			t.Fatalf("redo=%+v, want %+v", restored, changed)
		}
		moved := Translate(original, image.Pt(500, 500), doc.Bounds())
		if !AnnotationBounds(moved).In(doc.Bounds()) || moved.End.Sub(moved.Start) != original.End.Sub(original.Start) {
			t.Fatalf("translation=%+v", moved)
		}
	}
}

func TestLinearFormatsReverseDiagonalAndShortStrokes(t *testing.T) {
	for _, tool := range []Tool{ToolArrow, ToolDoubleArrow, ToolLine} {
		for _, pair := range [][2]image.Point{
			{image.Pt(100, 80), image.Pt(20, 20)},
			{image.Pt(20, 20), image.Pt(100, 80)},
			{image.Pt(40, 40), image.Pt(43, 41)},
		} {
			doc, _ := NewDocument(image.NewRGBA(image.Rect(0, 0, 120, 100)))
			annotation := Annotation{Tool: tool, Start: pair[0], End: pair[1], Style: DefaultStyle()}
			if _, err := doc.Add(annotation); err != nil {
				t.Fatal(err)
			}
			rendered := doc.Rendered()
			bounds := AnnotationBounds(annotation)
			for y := 0; y < 100; y++ {
				for x := 0; x < 120; x++ {
					c := color.NRGBAModel.Convert(rendered.At(x, y)).(color.NRGBA)
					if c.R == 0 {
						continue
					}
					point := image.Pt(x, y)
					if !point.In(bounds) {
						t.Fatalf("tool %v pixel %v outside %v", tool, point, bounds)
					}
					if _, hit := doc.HitTest(point, 2); !hit {
						t.Fatalf("painted stroke cannot be selected at %v", point)
					}
				}
			}
		}
	}
}

func TestLinearHeadScalingAndEmptyAnnotation(t *testing.T) {
	left, right, ok := ArrowHead(image.Pt(20, 50), image.Pt(100, 50), 3, .5)
	if !ok || left != image.Pt(88, 57) || right != image.Pt(88, 43) {
		t.Fatalf("scaled head=%v %v %v", left, right, ok)
	}
	for _, tool := range []Tool{ToolArrow, ToolLine, ToolDoubleArrow} {
		doc, _ := NewDocument(image.NewRGBA(image.Rect(0, 0, 10, 10)))
		if _, err := doc.Add(Annotation{Tool: tool, Style: DefaultStyle()}); err == nil {
			t.Fatalf("tool %v accepted zero-length stroke", tool)
		}
	}
}
