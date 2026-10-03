package main

import (
	"image"
	"reflect"
	"screenshot-win/selector"
	"testing"
)

func TestToolbarEditorGeometryAtDPI(t *testing.T) {
	for _, dpi := range []int{96, 144, 192} {
		g := toolbarEditorGeometry{dpi: dpi}
		point := func(x, y int) image.Point { return image.Pt(g.scale(x), g.scale(y)) }
		ids := []string{"arrow", "copy", "cancel"}
		for _, tc := range []struct {
			x, y int
			id   string
		}{{32, 70, "arrow"}, {72, 70, "copy"}, {112, 70, "cancel"}, {32, 134, "rectangle"}} {
			got, ok := g.hit(selector.ScreenshotToolbar, ids, point(tc.x, tc.y))
			if !ok || got != tc.id {
				t.Fatalf("dpi %d hit %v: %q %v", dpi, tc, got, ok)
			}
		}
		for _, tc := range []struct {
			x, y, index int
			valid       bool
		}{{8, 70, 0, true}, {15, 70, 0, true}, {49, 70, 1, true}, {92, 70, 2, true}, {440, 70, 3, true}, {32, 134, -1, true}, {400, 100, 0, false}, {480, 70, 0, false}, {20, 10, 0, false}, {-1, 70, 0, false}} {
			got, valid := g.drop(len(ids), point(tc.x, tc.y))
			if valid != tc.valid || got != tc.index {
				t.Fatalf("dpi %d drop %v: %d %v", dpi, tc, got, valid)
			}
		}
		index, valid := g.drop(len(ids), point(440, 70))
		if !valid {
			t.Fatal("end drop rejected")
		}
		next, err := selector.ChangeToolbarLayout(selector.ScreenshotToolbar, ids, "arrow", index)
		if err != nil || !reflect.DeepEqual(next, []string{"copy", "cancel", "arrow"}) {
			t.Fatalf("end drop: %v %v", next, err)
		}
	}
}
