//go:build windows

package selector

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"screenshot-win/editor"
)

func TestInlineTextPreviewMatchesCommit(t *testing.T) {
	for _, scale := range []float64{0.5, 1, 1.75} {
		for _, existing := range []bool{false, true} {
			source := image.NewNRGBA(image.Rect(0, 0, 320, 100))
			for y := 0; y < 100; y++ {
				for x := 0; x < 320; x++ {
					source.SetNRGBA(x, y, color.NRGBA{uint8(x), uint8(y), 80, 255})
				}
			}
			doc, err := editor.NewDocument(source)
			if err != nil {
				t.Fatal(err)
			}
			style := editor.DefaultStyle()
			start := image.Pt(37, 23)
			var id editor.AnnotationID
			if existing {
				id, err = doc.Add(editor.Annotation{Tool: editor.ToolText, Start: start, Text: "Original", Style: style})
				if err != nil {
					t.Fatal(err)
				}
			}
			state := &frozenState{document: doc, viewport: editor.Viewport{Scale: scale, Offset: image.Pt(11, 7)}, textEdit: &frozenTextEdit{id: id, start: start, style: style, bounds: image.Rect(0, 0, 320, 100)}}
			text := "中文 Abc 123"
			preview := state.renderTextPreview(text)
			if existing {
				original, _ := doc.Get(id)
				if original.Text != "Original" {
					t.Fatal("preview mutated document")
				}
			}
			draft := editor.Annotation{Tool: editor.ToolText, Start: start, Text: text, Style: style}
			if existing {
				err = doc.Replace(id, draft)
			} else {
				_, err = doc.Add(draft)
			}
			if err != nil {
				t.Fatal(err)
			}
			committed := editor.RenderViewport(doc.Rendered(), state.viewport, state.textEdit.bounds)
			if !bytes.Equal(preview.Pix, committed.Pix) {
				t.Fatalf("preview differs from commit: scale=%v existing=%v", scale, existing)
			}
		}
	}
}
