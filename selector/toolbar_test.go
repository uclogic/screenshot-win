package selector

import (
	"context"
	"image"
	"testing"

	"screenshot-win/editor"
)

func TestActionToolbarNextActionAndCloseLifecycle(t *testing.T) {
	events := make(chan ToolbarEvent, 1)
	done := make(chan struct{})
	readyCalls := 0
	closeCalls := 0
	toolbar := &ActionToolbar{
		events: events,
		ready:  func() { readyCalls++ },
		closeWindow: func() {
			closeCalls++
			close(done)
		},
		done: done,
	}
	events <- ToolbarEvent{Action: ActionArrow}
	got, err := toolbar.NextAction(context.Background())
	if err != nil || got != ActionArrow {
		t.Fatalf("NextAction() = (%v, %v), want (arrow, nil)", got, err)
	}
	if readyCalls != 1 {
		t.Fatalf("ready calls = %d, want 1", readyCalls)
	}
	toolbar.Close()
	toolbar.Close()
	if closeCalls != 1 {
		t.Fatalf("close calls = %d, want 1", closeCalls)
	}
}

func TestToolbarBoundsPrefersBelowRight(t *testing.T) {
	got := toolbarBounds(image.Rect(100, 100, 500, 400), image.Rect(0, 0, 1920, 1080), image.Pt(208, 40))
	want := image.Rect(292, 408, 500, 448)
	if got != want {
		t.Fatalf("toolbarBounds() = %v, want %v", got, want)
	}
}

func TestToolbarBoundsMovesAboveAndClampsHorizontally(t *testing.T) {
	got := toolbarBounds(image.Rect(1850, 900, 1910, 1070), image.Rect(0, 0, 1920, 1080), image.Pt(128, 40))
	want := image.Rect(1782, 852, 1910, 892)
	if got != want {
		t.Fatalf("toolbarBounds() = %v, want %v", got, want)
	}
}

func TestToolbarBoundsSupportsNegativeMonitorCoordinates(t *testing.T) {
	got := toolbarBounds(image.Rect(-1800, -100, -1200, 700), image.Rect(-1920, -200, 0, 880), image.Pt(128, 40))
	want := image.Rect(-1328, 708, -1200, 748)
	if got != want {
		t.Fatalf("toolbarBounds() = %v, want %v", got, want)
	}
}

func TestToolbarBoundsClampsWhenNeitherSideFits(t *testing.T) {
	got := toolbarBounds(image.Rect(10, 5, 90, 95), image.Rect(0, 0, 100, 100), image.Pt(80, 40))
	want := image.Rect(10, 0, 90, 40)
	if got != want {
		t.Fatalf("toolbarBounds() = %v, want %v", got, want)
	}
}

func TestSelectionToolbarActions(t *testing.T) {
	if len(selectionToolbarActions) != 10 {
		t.Fatalf("selection toolbar has %d actions, want 10 without undo/redo buttons", len(selectionToolbarActions))
	}
	size := image.Pt(408, 40)
	tests := []struct {
		point image.Point
		want  Action
	}{
		{image.Pt(10, 20), ActionCancel},
		{image.Pt(50, 20), ActionScroll},
		{image.Pt(90, 20), ActionRectangle},
		{image.Pt(130, 20), ActionArrow},
		{image.Pt(170, 20), ActionText},
		{image.Pt(210, 20), ActionColor},
		{image.Pt(250, 20), ActionWidth},
		{image.Pt(290, 20), ActionPin},
		{image.Pt(330, 20), ActionSave},
		{image.Pt(370, 20), ActionCopy},
	}
	for _, test := range tests {
		got, ok := toolbarActionAtActions(test.point, size, selectionToolbarActions)
		if !ok || got != test.want {
			t.Errorf("selection action at %v = (%v, %v), want (%v, true)", test.point, got, ok, test.want)
		}
	}
	for _, point := range []image.Point{image.Pt(1, 20), image.Pt(20, 1), image.Pt(408, 20)} {
		if _, ok := toolbarActionAtActions(point, size, selectionToolbarActions); ok {
			t.Errorf("selection toolbar accepted outside point %v", point)
		}
	}
}

func TestCaptureToolbarActions(t *testing.T) {
	size := image.Pt(208, 40)
	tests := []struct {
		point image.Point
		want  Action
	}{
		{image.Pt(10, 20), ActionCancel},
		{image.Pt(50, 20), ActionEdit},
		{image.Pt(90, 20), ActionPin},
		{image.Pt(130, 20), ActionSaveAs},
		{image.Pt(170, 20), ActionCopy},
	}
	for _, test := range tests {
		got, ok := toolbarActionAtActions(test.point, size, captureToolbarActions)
		if !ok || got != test.want {
			t.Errorf("capture action at %v = (%v, %v), want (%v, true)", test.point, got, ok, test.want)
		}
	}
	if _, ok := toolbarActionAtActions(image.Pt(1, 20), size, captureToolbarActions); ok {
		t.Fatal("capture toolbar accepted a point outside its buttons")
	}
	if !toolbarActionEnabled(ActionPin) {
		t.Fatal("pin action should be enabled")
	}
	for _, action := range []Action{ActionCancel, ActionSaveAs, ActionCopy} {
		if !toolbarActionEnabled(action) {
			t.Fatalf("action %v should be enabled", action)
		}
	}
}

func TestAnnotationToolbarOmitsLongCapture(t *testing.T) {
	if len(annotationToolbarActions) != 9 {
		t.Fatalf("annotation toolbar has %d actions, want 9 without undo/redo buttons", len(annotationToolbarActions))
	}
	for _, action := range annotationToolbarActions {
		if action == ActionScroll || action == ActionEdit {
			t.Fatalf("annotation toolbar contains terminal action %v", action)
		}
	}
	for _, required := range []Action{ActionRectangle, ActionArrow, ActionText, ActionColor, ActionWidth} {
		found := false
		for _, action := range annotationToolbarActions {
			if action == required {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("annotation toolbar is missing %v", required)
		}
	}
	if got := annotationToolbarActions[len(annotationToolbarActions)-2:]; got[0] != ActionSave || got[1] != ActionCopy {
		t.Fatalf("annotation toolbar trailing actions = %v, want save and copy", got)
	}
}

func TestStylePanelOptionMappingAtSupportedDPI(t *testing.T) {
	for _, dpi := range []int{96, 144, 192} {
		colorSize := stylePanelSize(editor.StyleFieldColor, dpi)
		for index := range editor.PresetColors() {
			point := image.Pt((4+index*36+18)*dpi/96, 22*dpi/96)
			got, ok := stylePanelOptionAt(point, editor.StyleFieldColor, dpi)
			if !ok || got != index {
				t.Fatalf("color option dpi=%d point=%v = (%d,%v), want %d", dpi, point, got, ok, index)
			}
		}
		if _, ok := stylePanelOptionAt(image.Pt(colorSize.X, colorSize.Y/2), editor.StyleFieldColor, dpi); ok {
			t.Fatalf("color panel accepted right edge at dpi %d", dpi)
		}
		for index := range editor.PresetWidths() {
			point := image.Pt(20*dpi/96, (4+index*32+16)*dpi/96)
			got, ok := stylePanelOptionAt(point, editor.StyleFieldWidth, dpi)
			if !ok || got != index {
				t.Fatalf("width option dpi=%d point=%v = (%d,%v), want %d", dpi, point, got, ok, index)
			}
		}
	}
}

func TestStylePanelBoundsFlipsAndClamps(t *testing.T) {
	workArea := image.Rect(-1920, -200, 0, 880)
	size := image.Pt(188, 136)
	below := stylePanelBounds(image.Rect(-500, 100, -460, 140), workArea, size, 4)
	if below != image.Rect(-500, 144, -312, 280) {
		t.Fatalf("below bounds = %v", below)
	}
	above := stylePanelBounds(image.Rect(-80, 820, -40, 860), workArea, size, 4)
	if above != image.Rect(-188, 680, 0, 816) {
		t.Fatalf("above/clamped bounds = %v", above)
	}
}
