package editor

import (
	"image"
	"math"
)

// IsLinearTool reports whether a tool draws an editable two-endpoint stroke.
func IsLinearTool(tool Tool) bool {
	return tool == ToolArrow || tool == ToolLine || tool == ToolDoubleArrow
}

// Segment is one stroke in image or viewport coordinates.
type Segment struct{ Start, End image.Point }

// LinearSegments returns the shaft and optional arrowheads without allocating.
// Width is in original-image pixels; scale converts it to the supplied coordinates.
func LinearSegments(tool Tool, start, end image.Point, width, scale float64) ([5]Segment, int) {
	var segments [5]Segment
	if !IsLinearTool(tool) {
		return segments, 0
	}
	segments[0] = Segment{start, end}
	count := 1
	if tool != ToolLine {
		if left, right, ok := ArrowHead(start, end, width, scale); ok {
			segments[count], segments[count+1] = Segment{end, left}, Segment{end, right}
			count += 2
		}
	}
	if tool == ToolDoubleArrow {
		if left, right, ok := ArrowHead(end, start, width, scale); ok {
			segments[count], segments[count+1] = Segment{start, left}, Segment{start, right}
			count += 2
		}
	}
	return segments, count
}

// ArrowHead returns the two tips of an arrowhead at end.
func ArrowHead(start, end image.Point, width, scale float64) (image.Point, image.Point, bool) {
	dx, dy := float64(end.X-start.X), float64(end.Y-start.Y)
	length := math.Hypot(dx, dy)
	if length == 0 {
		return image.Point{}, image.Point{}, false
	}
	head := math.Min((18+width*2)*scale, length*.45)
	ux, uy := dx/length, dy/length
	left := image.Pt(int(math.Round(float64(end.X)-ux*head-uy*head*.55)), int(math.Round(float64(end.Y)-uy*head+ux*head*.55)))
	right := image.Pt(int(math.Round(float64(end.X)-ux*head+uy*head*.55)), int(math.Round(float64(end.Y)-uy*head-ux*head*.55)))
	return left, right, true
}
