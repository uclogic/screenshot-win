package editor

import (
	"image"
	"math"
)

// ScreenPoint retains fractional pixels for DPI-scaled editing chrome.
type ScreenPoint struct{ X, Y float64 }
type RectangleHandle struct {
	Kind  TransformHandle
	Point ScreenPoint
}
type RectangleSegment struct{ Start, End ScreenPoint }

// RectangleLayout is transient, allocation-free screen geometry. Only visible
// handles appear in Handles; the same layout drives painting and hit testing.
type RectangleLayout struct {
	Handles                   [8]RectangleHandle
	Count                     int
	Segments                  [8]RectangleSegment
	SegmentCount              int
	Radius, Stroke, HitRadius float64
}

func LayoutRectangle(start, end image.Point, dpi int, borderWidth float64) RectangleLayout {
	scale := float64(max(96, dpi)) / 96
	l := RectangleLayout{Radius: 4.5 * scale, Stroke: 1.5 * scale, HitRadius: 8 * scale}
	x0, x1 := float64(min(start.X, end.X)), float64(max(start.X, end.X))
	y0, y1 := float64(min(start.Y, end.Y)), float64(max(start.Y, end.Y))
	cx, cy := (x0+x1)/2, (y0+y1)/2
	wide, tall := x1-x0 >= 32*scale, y1-y0 >= 32*scale
	add := func(kind TransformHandle, x, y float64) {
		l.Handles[l.Count] = RectangleHandle{kind, ScreenPoint{x, y}}
		l.Count++
	}
	add(HandleRectangleNorthWest, x0, y0)
	if wide {
		add(HandleRectangleNorth, cx, y0)
	}
	add(HandleRectangleNorthEast, x1, y0)
	if tall {
		add(HandleRectangleEast, x1, cy)
	}
	add(HandleRectangleSouthEast, x1, y1)
	if wide {
		add(HandleRectangleSouth, cx, y1)
	}
	add(HandleRectangleSouthWest, x0, y1)
	if tall {
		add(HandleRectangleWest, x0, cy)
	}
	gap := l.Radius + l.Stroke/2 + math.Max(1, borderWidth)/2
	segment := func(x0, y0, x1, y1 float64) {
		if x1 < x0 || y1 < y0 {
			return
		}
		l.Segments[l.SegmentCount] = RectangleSegment{ScreenPoint{x0, y0}, ScreenPoint{x1, y1}}
		l.SegmentCount++
	}
	for _, y := range [2]float64{y0, y1} {
		if wide {
			segment(x0+gap, y, cx-gap, y)
			segment(cx+gap, y, x1-gap, y)
		} else {
			segment(x0+gap, y, x1-gap, y)
		}
	}
	for _, x := range [2]float64{x0, x1} {
		if tall {
			segment(x, y0+gap, x, cy-gap)
			segment(x, cy+gap, x, y1-gap)
		} else {
			segment(x, y0+gap, x, y1-gap)
		}
	}
	return l
}

func rectangleCorner(h TransformHandle) bool {
	return h == HandleRectangleNorthWest || h == HandleRectangleNorthEast || h == HandleRectangleSouthWest || h == HandleRectangleSouthEast
}

func (l RectangleLayout) Hit(p image.Point) (TransformHandle, bool) {
	best, result := l.HitRadius*l.HitRadius, HandleMove
	for i := 0; i < l.Count; i++ {
		h := l.Handles[i]
		dx, dy := float64(p.X)-h.Point.X, float64(p.Y)-h.Point.Y
		d := dx*dx + dy*dy
		if d < best || (d == best && (result == HandleMove || rectangleCorner(h.Kind) && !rectangleCorner(result))) {
			best, result = d, h.Kind
		}
	}
	return result, result != HandleMove
}

func (l *RectangleLayout) BorderCoverage(p ScreenPoint, width float64) float64 {
	coverage := 0.0
	radius := math.Max(.5, width/2)
	limit := radius + .5
	for i := 0; i < l.SegmentCount; i++ {
		s := &l.Segments[i]
		if p.X < s.Start.X-limit || p.X > s.End.X+limit || p.Y < s.Start.Y-limit || p.Y > s.End.Y+limit {
			continue
		}
		dx, dy := 0.0, 0.0
		if p.X < s.Start.X {
			dx = s.Start.X - p.X
		} else if p.X > s.End.X {
			dx = p.X - s.End.X
		}
		if p.Y < s.Start.Y {
			dy = s.Start.Y - p.Y
		} else if p.Y > s.End.Y {
			dy = p.Y - s.End.Y
		}
		distance := dx + dy
		if dx != 0 && dy != 0 {
			distance = math.Sqrt(dx*dx + dy*dy)
		}
		c := strokeCoverage(distance, radius)
		if c > coverage {
			coverage = c
		}
		if coverage >= 1 {
			return 1
		}
	}
	return coverage
}

func (l *RectangleLayout) HandleCoverage(p ScreenPoint) float64 {
	coverage := 0.0
	for i := 0; i < l.Count; i++ {
		h := l.Handles[i].Point
		d := math.Abs(math.Hypot(p.X-h.X, p.Y-h.Y) - l.Radius)
		coverage = math.Max(coverage, strokeCoverage(d, l.Stroke/2))
	}
	return coverage
}

// ResizeRectangle uses the original geometry and total pointer displacement.
// Edges cannot cross. Existing undersized rectangles never jump on mouse down.
func ResizeRectangle(a Annotation, handle TransformHandle, delta image.Point, bounds image.Rectangle, minimum image.Point) Annotation {
	if a.Tool != ToolRectangle {
		return a
	}
	x0, x1 := min(a.Start.X, a.End.X), max(a.Start.X, a.End.X)
	y0, y1 := min(a.Start.Y, a.End.Y), max(a.Start.Y, a.End.Y)
	mw, mh := min(max(1, minimum.X), x1-x0), min(max(1, minimum.Y), y1-y0)
	maxX, maxY := bounds.Max.X-1, bounds.Max.Y-1
	switch handle {
	case HandleRectangleNorthWest, HandleRectangleWest, HandleRectangleSouthWest:
		x0 = max(bounds.Min.X, min(x0+delta.X, x1-mw))
	case HandleRectangleNorthEast, HandleRectangleEast, HandleRectangleSouthEast:
		x1 = min(maxX, max(x1+delta.X, x0+mw))
	}
	switch handle {
	case HandleRectangleNorthWest, HandleRectangleNorth, HandleRectangleNorthEast:
		y0 = max(bounds.Min.Y, min(y0+delta.Y, y1-mh))
	case HandleRectangleSouthWest, HandleRectangleSouth, HandleRectangleSouthEast:
		y1 = min(maxY, max(y1+delta.Y, y0+mh))
	}
	a.Start, a.End = image.Pt(x0, y0), image.Pt(x1, y1)
	return a
}
