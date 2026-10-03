package app

import (
	"context"
	"image"

	"screenshot-win/editor"
	"screenshot-win/selector"
	"testing"
)

func TestArrowFormatActionsOnlyAffectNewAnnotations(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 120, 100))
	doc, _ := editor.NewDocument(source)
	originalID, _ := doc.Add(editor.Annotation{Tool: editor.ToolArrow, Start: image.Pt(20, 10), End: image.Pt(100, 10), Style: editor.DefaultStyle()})
	original, _ := doc.Get(originalID)
	style := editor.DefaultStyle()
	style.Color = editor.PresetColors()[1]
	style.Width = 8
	actions := []selector.Action{selector.ActionDoubleArrow, selector.ActionLine, selector.ActionArrow, selector.ActionCopy}
	tools := []editor.Tool{editor.ToolDoubleArrow, editor.ToolLine, editor.ToolArrow}
	events, placements, styleChanges := 0, 0, 0
	result, err := runActionMenu(source.Bounds(), source, interactiveOperations{
		showToolbarEvent: func(image.Rectangle) (selector.ToolbarEvent, error) {
			if events >= len(actions) {
				t.Fatal("unexpected toolbar request")
			}
			action := actions[events]
			events++
			return selector.ToolbarEvent{Action: action, Style: style}, nil
		},
		annotate: func(tool editor.Tool, got editor.Style) error {
			if tool != tools[placements] || got != style {
				t.Fatalf("placement %d tool=%v style=%v", placements, tool, got)
			}
			placements++
			_, err := doc.Add(editor.Annotation{Tool: tool, Start: image.Pt(20, 20+placements*15), End: image.Pt(100, 20+placements*15), Style: got})
			return err
		},
		updateSelectedStyle: func(context.Context, editor.StyleChange) (bool, error) { styleChanges++; return false, nil },
		rendered:            doc.Rendered,
		copy: func(output image.Image) error {
			if len(doc.Annotations()) != 4 {
				t.Fatal("missing annotations at copy")
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := doc.Get(originalID)
	if after != original || styleChanges != 0 || result.action != selector.ActionCopy || placements != 3 {
		t.Fatalf("original=%+v after=%+v styleChanges=%d result=%+v placements=%d", original, after, styleChanges, result, placements)
	}
}
