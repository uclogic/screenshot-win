package editor

import (
	"image"
	"image/color"
	"testing"
)

func TestDocumentUndoRedoAndRedoInvalidation(t *testing.T) {
	document, _ := NewDocument(image.NewNRGBA(image.Rect(0, 0, 20, 20)))
	first := Annotation{Tool: ToolRectangle, Start: image.Pt(1, 1), End: image.Pt(10, 10), Style: DefaultStyle()}
	if _, err := document.Add(first); err != nil {
		t.Fatal(err)
	}
	if !document.Undo() || !document.CanRedo() || len(document.Annotations()) != 0 {
		t.Fatal("undo did not move annotation to redo stack")
	}
	if !document.Redo() || len(document.Annotations()) != 1 {
		t.Fatal("redo did not restore annotation")
	}
	document.Undo()
	second := first
	second.End = image.Pt(15, 15)
	_, _ = document.Add(second)
	if document.CanRedo() {
		t.Fatal("new edit should clear redo stack")
	}
}

func TestViewportRoundTripAndAnchorZoom(t *testing.T) {
	viewport := Viewport{Scale: .5, Offset: image.Pt(10, 20)}
	point := image.Pt(80, 60)
	if got := viewport.ScreenToImage(viewport.ImageToScreen(point)); got != point {
		t.Fatalf("round trip = %v, want %v", got, point)
	}
	anchor := image.Pt(100, 100)
	before := viewport.ScreenToImage(anchor)
	viewport.ZoomAt(anchor, 2)
	if after := viewport.ScreenToImage(anchor); after != before {
		t.Fatalf("zoom moved anchor image point from %v to %v", before, after)
	}
}

func TestRenderedUsesOriginalBoundsAndOverlaysAnnotation(t *testing.T) {
	source := image.NewNRGBA(image.Rect(5, 7, 25, 27))
	source.Set(15, 17, color.NRGBA{B: 255, A: 255})
	document, _ := NewDocument(source)
	style := DefaultStyle()
	style.Color = color.NRGBA{R: 255, A: 255}
	style.Width = 3
	_, _ = document.Add(Annotation{Tool: ToolArrow, Start: image.Pt(0, 10), End: image.Pt(19, 10), Style: style})
	rendered := document.Rendered()
	if rendered.Bounds() != image.Rect(0, 0, 20, 20) {
		t.Fatalf("bounds = %v", rendered.Bounds())
	}
	red, _, blue, _ := rendered.At(10, 10).RGBA()
	if red <= blue {
		t.Fatalf("annotation was not composited: red=%d blue=%d", red, blue)
	}
}

func TestRenderedArrowAntialiasesDiagonalEdges(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 32, 24))
	for y := 0; y < source.Bounds().Dy(); y++ {
		for x := 0; x < source.Bounds().Dx(); x++ {
			source.SetNRGBA(x, y, color.NRGBA{A: 255})
		}
	}
	document, _ := NewDocument(source)
	style := DefaultStyle()
	style.Color = color.NRGBA{R: 255, A: 255}
	style.Width = 2
	_, _ = document.Add(Annotation{Tool: ToolArrow, Start: image.Pt(3, 4), End: image.Pt(27, 16), Style: style})

	rendered := document.Rendered()
	foundPartial := false
	foundOpaque := false
	for y := 0; y < rendered.Bounds().Dy(); y++ {
		for x := 0; x < rendered.Bounds().Dx(); x++ {
			red, _, _, _ := rendered.At(x, y).RGBA()
			value := uint8(red >> 8)
			foundPartial = foundPartial || value > 0 && value < 255
			foundOpaque = foundOpaque || value == 255
		}
	}
	if !foundPartial || !foundOpaque {
		t.Fatalf("antialiased arrow pixels: partial=%v opaque=%v", foundPartial, foundOpaque)
	}
}

func TestRenderedWithoutOmitsOnlyRequestedAnnotation(t *testing.T) {
	document, err := NewDocument(image.NewNRGBA(image.Rect(0, 0, 30, 20)))
	if err != nil {
		t.Fatal(err)
	}
	red := DefaultStyle()
	red.Color = color.NRGBA{R: 255, A: 255}
	blue := DefaultStyle()
	blue.Color = color.NRGBA{B: 255, A: 255}
	removed, _ := document.Add(Annotation{Tool: ToolArrow, Start: image.Pt(2, 5), End: image.Pt(25, 5), Style: red})
	_, _ = document.Add(Annotation{Tool: ToolArrow, Start: image.Pt(2, 15), End: image.Pt(25, 15), Style: blue})

	rendered := document.RenderedWithout(removed)
	if got := color.NRGBAModel.Convert(rendered.At(10, 5)).(color.NRGBA); got.R != 0 {
		t.Fatalf("omitted annotation still rendered: %+v", got)
	}
	if got := color.NRGBAModel.Convert(rendered.At(10, 15)).(color.NRGBA); got.B == 0 {
		t.Fatalf("unrelated annotation was omitted: %+v", got)
	}
}

func TestRenderViewportAllocationDoesNotDependOnSourceHeight(t *testing.T) {
	source := image.NewUniform(color.White)
	view := RenderViewport(source, Viewport{Scale: 1}, image.Rect(0, 0, 80, 40))
	if view.Bounds().Size() != image.Pt(80, 40) || len(view.Pix) != 80*40*4 {
		t.Fatalf("unexpected viewport allocation %v / %d", view.Bounds(), len(view.Pix))
	}
}

func TestPresetStylesAndPartialChanges(t *testing.T) {
	colors := PresetColors()
	widths := PresetWidths()
	if len(colors) != 5 || len(widths) != 4 {
		t.Fatalf("preset counts = %d colors, %d widths", len(colors), len(widths))
	}
	if colors[0] != DefaultStyle().Color || widths[1] != DefaultStyle().Width {
		t.Fatalf("default style is not represented by presets: %+v", DefaultStyle())
	}
	original := Style{Color: colors[1], Width: widths[3]}
	colorChanged := ApplyStyleChange(original, StyleChange{Field: StyleFieldColor, Style: Style{Color: colors[2]}})
	if colorChanged.Color != colors[2] || colorChanged.Width != original.Width {
		t.Fatalf("color change = %+v, want color changed and width preserved", colorChanged)
	}
	widthChanged := ApplyStyleChange(original, StyleChange{Field: StyleFieldWidth, Style: Style{Width: widths[0]}})
	if widthChanged.Width != widths[0] || widthChanged.Color != original.Color {
		t.Fatalf("width change = %+v, want width changed and color preserved", widthChanged)
	}
	colors[0] = color.NRGBA{}
	if PresetColors()[0] != DefaultStyle().Color {
		t.Fatal("PresetColors returned mutable shared storage")
	}
}

func TestTextAnnotationProducesVisiblePixels(t *testing.T) {
	document, _ := NewDocument(image.NewNRGBA(image.Rect(0, 0, 120, 40)))
	style := DefaultStyle()
	if _, err := document.Add(Annotation{Tool: ToolText, Start: image.Pt(2, 2), Text: "文字 A1", Style: style}); err != nil {
		t.Fatal(err)
	}
	rendered := document.Rendered()
	visible := false
	for y := 0; y < 35 && !visible; y++ {
		for x := 0; x < 115; x++ {
			red, _, _, _ := rendered.At(x, y).RGBA()
			if red > 0 {
				visible = true
				break
			}
		}
	}
	if !visible {
		t.Fatal("text annotation rendered no visible pixels")
	}
}

func TestDocumentStableIDsReplaceDeleteAndHistory(t *testing.T) {
	document, _ := NewDocument(image.NewNRGBA(image.Rect(0, 0, 100, 80)))
	firstID, err := document.Add(Annotation{Tool: ToolRectangle, Start: image.Pt(5, 5), End: image.Pt(30, 20), Style: DefaultStyle()})
	if err != nil || firstID == 0 {
		t.Fatalf("Add() id=%d err=%v", firstID, err)
	}
	secondID, _ := document.Add(Annotation{Tool: ToolArrow, Start: image.Pt(10, 40), End: image.Pt(60, 40), Style: DefaultStyle()})
	if secondID <= firstID {
		t.Fatalf("ids are not monotonic: first=%d second=%d", firstID, secondID)
	}

	first, ok := document.Get(firstID)
	if !ok {
		t.Fatal("Get() did not find first annotation")
	}
	first.End = image.Pt(45, 30)
	if err := document.Replace(firstID, first); err != nil {
		t.Fatal(err)
	}
	if got, _ := document.Get(firstID); got.ID != firstID || got.End != first.End {
		t.Fatalf("Replace() = %+v", got)
	}
	if !document.Undo() {
		t.Fatal("Undo() did not revert replacement")
	}
	if got, _ := document.Get(firstID); got.End == first.End {
		t.Fatal("Undo() retained replacement geometry")
	}
	if !document.Redo() {
		t.Fatal("Redo() did not restore replacement")
	}
	if got, _ := document.Get(firstID); got.End != first.End {
		t.Fatal("Redo() did not restore replacement geometry")
	}

	if !document.Delete(secondID) {
		t.Fatal("Delete() returned false")
	}
	if _, ok := document.Get(secondID); ok {
		t.Fatal("deleted annotation is still present")
	}
	if !document.Undo() {
		t.Fatal("Undo() did not restore deleted annotation")
	}
	if restored, ok := document.Get(secondID); !ok || restored.ID != secondID {
		t.Fatalf("restored annotation = %+v, %v", restored, ok)
	}
}

func TestDocumentHitTestUsesToleranceAndTopmostOrder(t *testing.T) {
	document, _ := NewDocument(image.NewNRGBA(image.Rect(0, 0, 100, 100)))
	bottom, _ := document.Add(Annotation{Tool: ToolRectangle, Start: image.Pt(10, 10), End: image.Pt(80, 80), Style: DefaultStyle()})
	top, _ := document.Add(Annotation{Tool: ToolArrow, Start: image.Pt(5, 13), End: image.Pt(90, 13), Style: DefaultStyle()})
	if hit, ok := document.HitTest(image.Pt(40, 16), 4); !ok || hit.ID != top {
		t.Fatalf("HitTest() = %+v, %v; want top id %d", hit, ok, top)
	}
	if hit, ok := document.HitTest(image.Pt(10, 50), 2); !ok || hit.ID != bottom {
		t.Fatalf("HitTest() = %+v, %v; want rectangle id %d", hit, ok, bottom)
	}
	if _, ok := document.HitTest(image.Pt(50, 50), 2); ok {
		t.Fatal("HitTest() selected the empty rectangle interior")
	}
}

func TestRenderedPreviewReplacesWithoutMutatingDocument(t *testing.T) {
	document, _ := NewDocument(image.NewNRGBA(image.Rect(0, 0, 60, 30)))
	style := DefaultStyle()
	style.Color = color.NRGBA{R: 255, A: 255}
	id, _ := document.Add(Annotation{Tool: ToolArrow, Start: image.Pt(2, 5), End: image.Pt(50, 5), Style: style})
	original, _ := document.Get(id)
	draft := Translate(original, image.Pt(0, 10), document.Bounds())
	preview := document.RenderedPreview(id, &draft)
	redOld, _, _, _ := preview.At(25, 5).RGBA()
	redNew, _, _, _ := preview.At(25, 15).RGBA()
	if redOld != 0 || redNew == 0 {
		t.Fatalf("preview old/new red = %d/%d", redOld, redNew)
	}
	if got, _ := document.Get(id); got.Start != original.Start || got.End != original.End {
		t.Fatal("RenderedPreview mutated the document")
	}
}

func TestTranslateAndTransformStayInsideBounds(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 80)
	rectangle := Annotation{Tool: ToolRectangle, Start: image.Pt(10, 10), End: image.Pt(30, 30), Style: DefaultStyle()}
	moved := Translate(rectangle, image.Pt(200, 200), bounds)
	if got := AnnotationBounds(moved); got.Max.X > bounds.Max.X || got.Max.Y > bounds.Max.Y {
		t.Fatalf("translated bounds %v exceed %v", got, bounds)
	}
	resized := TransformTo(rectangle, HandleRectangleNorthWest, image.Pt(90, 70), bounds)
	if resized.Start != image.Pt(30, 30) || resized.End != image.Pt(90, 70) {
		t.Fatalf("crossed resize = %v..%v", resized.Start, resized.End)
	}
	arrow := Annotation{Tool: ToolArrow, Start: image.Pt(5, 5), End: image.Pt(50, 50), Style: DefaultStyle()}
	arrow = TransformTo(arrow, HandleArrowEnd, image.Pt(500, -20), bounds)
	if arrow.End != image.Pt(99, 0) {
		t.Fatalf("clamped arrow end = %v", arrow.End)
	}
}

func TestRectangleAllResizeHandles(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 80)
	rectangle := Annotation{Tool: ToolRectangle, Start: image.Pt(20, 20), End: image.Pt(60, 50), Style: DefaultStyle()}
	tests := []struct {
		handle TransformHandle
		point  image.Point
		start  image.Point
		end    image.Point
	}{
		{HandleRectangleNorthWest, image.Pt(5, 7), image.Pt(5, 7), image.Pt(60, 50)},
		{HandleRectangleNorth, image.Pt(0, 7), image.Pt(20, 7), image.Pt(60, 50)},
		{HandleRectangleNorthEast, image.Pt(75, 7), image.Pt(20, 7), image.Pt(75, 50)},
		{HandleRectangleEast, image.Pt(75, 0), image.Pt(20, 20), image.Pt(75, 50)},
		{HandleRectangleSouthEast, image.Pt(75, 65), image.Pt(20, 20), image.Pt(75, 65)},
		{HandleRectangleSouth, image.Pt(0, 65), image.Pt(20, 20), image.Pt(60, 65)},
		{HandleRectangleSouthWest, image.Pt(5, 65), image.Pt(5, 20), image.Pt(60, 65)},
		{HandleRectangleWest, image.Pt(5, 0), image.Pt(5, 20), image.Pt(60, 50)},
	}
	for _, test := range tests {
		got := TransformTo(rectangle, test.handle, test.point, bounds)
		if got.Start != test.start || got.End != test.end {
			t.Errorf("handle %d = %v..%v, want %v..%v", test.handle, got.Start, got.End, test.start, test.end)
		}
	}
}

func TestReplaceTextRegeneratesBounds(t *testing.T) {
	document, _ := NewDocument(image.NewNRGBA(image.Rect(0, 0, 300, 80)))
	id, _ := document.Add(Annotation{Tool: ToolText, Start: image.Pt(2, 2), Text: "A", Style: DefaultStyle()})
	annotation, _ := document.Get(id)
	before := AnnotationBounds(annotation)
	annotation.Text = "A much longer label"
	if err := document.Replace(id, annotation); err != nil {
		t.Fatal(err)
	}
	after, _ := document.Get(id)
	if AnnotationBounds(after).Dx() <= before.Dx() {
		t.Fatalf("text bounds did not grow: before=%v after=%v", before, AnnotationBounds(after))
	}
}
