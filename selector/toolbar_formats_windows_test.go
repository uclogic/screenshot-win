//go:build windows

package selector

import (
	"image"
	"image/color"
	"screenshot-win/editor"
	"testing"
)

func TestArrowButtonDispatchesRememberedFormat(t *testing.T) {
	for _, format := range arrowFormats {
		events := make(chan ToolbarEvent, 1)
		state := &toolbarState{persistent: true, ready: true, events: events, arrowAction: format.action, style: editor.DefaultStyle()}
		if !state.handlePersistentAction(ActionArrow) {
			t.Fatal("arrow action was not emitted")
		}
		event := <-events
		tool, ok := drawingToolForAction(event.Action)
		if !ok || !editor.IsLinearTool(tool) || event.Action != format.action || event.Style != state.style {
			t.Fatalf("event=%+v tool=%v", event, tool)
		}
		if !state.motionInput(ActionArrow).selected {
			t.Fatal("arrow button lost selected appearance")
		}
		if state.tooltipLabel(ActionArrow, "Arrow") != "Arrow" {
			t.Fatal("arrow tooltip contains a format instruction")
		}
	}
}

func TestArrowFormatsSwitchWhileDrawingAndRetainStyle(t *testing.T) {
	events := make(chan ToolbarEvent, 1)
	style := editor.DefaultStyle()
	style.Width = 8
	state := &toolbarState{persistent: true, active: true, activeAction: ActionArrow, events: events, style: style}
	if !state.handlePersistentAction(ActionDoubleArrow) || !state.pending || state.pendingAction != ActionDoubleArrow {
		t.Fatal("double arrow switch was not queued")
	}
	if !state.handlePersistentAction(ActionLine) || state.pendingAction != ActionLine {
		t.Fatal("latest format should win")
	}
	state.rearmPersistentActions()
	event := <-events
	if event.Action != ActionLine || event.Style != style || !state.active || state.pending {
		t.Fatalf("event=%+v state=%+v", event, state)
	}
	packed, width := packToolStyle(editor.ToolLine, style)
	tool, roundTrip := unpackToolStyle(packed, width)
	if tool != editor.ToolLine || roundTrip != style {
		t.Fatalf("packed tool/style=%v %+v", tool, roundTrip)
	}
}

func TestArrowFormatSelectionIsIndependentOfStyleSync(t *testing.T) {
	state := &toolbarState{hwnd: uintptr(0x7ffffff0), arrowAction: ActionLine, panel: stylePanel{format: true}, style: editor.DefaultStyle()}
	state.pendingStyle = editor.Style{Color: editor.PresetColors()[2], Width: 5}
	state.applyQueuedStyle()
	if state.arrowAction != ActionLine || state.currentPanelIndex() != 2 || state.panelOptionCount() != 3 {
		t.Fatal("selected annotation style reset format")
	}
	for _, tool := range []editor.Tool{editor.ToolLine, editor.ToolDoubleArrow} {
		overlay := &frozenState{selectionState: &selectionState{}, viewport: editor.Viewport{Scale: 1}}
		annotation := editor.Annotation{Tool: tool, Start: image.Pt(10, 20), End: image.Pt(90, 20), Style: editor.DefaultStyle()}
		if overlay.handles(annotation).count != 2 {
			t.Fatalf("tool %v has no endpoint handles", tool)
		}
	}
}

func TestArrowFormatMenuChoiceStartsDrawing(t *testing.T) {
	for index, format := range arrowFormats {
		events := make(chan ToolbarEvent, 1)
		state := &toolbarState{
			hwnd: uintptr(0x7ffffff0), persistent: true, ready: true, events: events,
			arrowAction: ActionLine, style: editor.DefaultStyle(), panel: stylePanel{format: true},
		}
		state.choosePanelOption(index)
		select {
		case event := <-events:
			if event.Action != format.action || event.Change.Field != editor.StyleFieldNone || event.Style != state.style || state.arrowAction != format.action {
				t.Fatalf("menu choice %d event=%+v", index, event)
			}
		default:
			t.Fatal("menu did not start drawing")
		}
	}
}

func TestLinearFastVectorMatchesExportAndRestoresFootprint(t *testing.T) {
	bounds := image.Rect(0, 0, 120, 100)
	for _, tool := range []editor.Tool{editor.ToolArrow, editor.ToolLine, editor.ToolDoubleArrow} {
		source := image.NewRGBA(bounds)
		for i := 0; i < len(source.Pix); i += 4 {
			source.Pix[i], source.Pix[i+1], source.Pix[i+2], source.Pix[i+3] = 20, 40, 60, 255
		}
		doc, _ := editor.NewDocument(source)
		annotation := editor.Annotation{Tool: tool, Start: image.Pt(20, 25), End: image.Pt(95, 75), Style: editor.DefaultStyle()}
		id, err := doc.Add(annotation)
		if err != nil {
			t.Fatal(err)
		}
		annotation, _ = doc.Get(id)
		state := &frozenState{
			selectionState: &selectionState{client: bounds, pixels: make([]byte, len(source.Pix))},
			document:       doc, region: bounds, viewport: editor.Viewport{Scale: 1},
		}
		if err := copyImageToBGRA(state.pixels, 120, 100, source); err != nil {
			t.Fatal(err)
		}
		assertFrame := func(want image.Image) {
			t.Helper()
			for y := 0; y < 100; y++ {
				for x := 0; x < 120; x++ {
					i := (y*120 + x) * 4
					got := color.NRGBA{state.pixels[i+2], state.pixels[i+1], state.pixels[i], state.pixels[i+3]}
					expected := color.NRGBAModel.Convert(want.At(x, y)).(color.NRGBA)
					if got != expected {
						t.Fatalf("tool %v pixel (%d,%d)=%v, want %v", tool, x, y, got, expected)
					}
				}
			}
		}
		state.drawFastVector(annotation)
		assertFrame(doc.Rendered())
		state.clearVectorTransformBase(annotation)
		assertFrame(source)
		moved := editor.Translate(annotation, image.Pt(0, -10), bounds)
		if err := doc.Replace(id, moved); err != nil {
			t.Fatal(err)
		}
		state.drawFastVector(moved)
		assertFrame(doc.Rendered())
	}
}
