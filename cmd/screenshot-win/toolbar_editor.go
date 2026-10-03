package main

import (
	"image"
	"math"
	"screenshot-win/selector"
)

// Geometry is shared by painting, hit testing and drop targeting.
type toolbarEditorGeometry struct{ dpi int }

func (g toolbarEditorGeometry) scale(v int) int { return (v*g.dpi + 48) / 96 }
func (g toolbarEditorGeometry) rect(x, y, w, h int) image.Rectangle {
	return image.Rect(g.scale(x), g.scale(y), g.scale(x+w), g.scale(y+h))
}
func (g toolbarEditorGeometry) row(candidate bool) image.Rectangle {
	y := 50
	if candidate {
		y = 114
	}
	return g.rect(8, y, 460, 40)
}
func (g toolbarEditorGeometry) cell(index int, candidate bool) image.Rectangle {
	y := 50
	if candidate {
		y = 114
	}
	return g.rect(12+40*index, y, 40, 40)
}
func (g toolbarEditorGeometry) hit(kind selector.ToolbarKind, ids []string, p image.Point) (string, bool) {
	for _, candidate := range []bool{false, true} {
		items := ids
		if candidate {
			items = selector.ToolbarCandidates(kind, ids)
		}
		for i, id := range items {
			if p.In(g.cell(i, candidate)) {
				return id, true
			}
		}
	}
	return "", false
}
func (g toolbarEditorGeometry) drop(count int, p image.Point) (int, bool) {
	if p.In(g.row(true)) {
		return -1, true
	}
	if !p.In(g.row(false)) {
		return 0, false
	}
	index := int(math.Round(float64(p.X-g.scale(12)) / float64(g.scale(40))))
	if index < 0 {
		index = 0
	}
	if index > count {
		index = count
	}
	return index, true
}
