package selector

import "image"

// candidateExtent tracks the area indicated while Tab is held.
type candidateExtent struct {
	held, active    bool
	current         image.Point
	path            image.Rectangle
	chosen          image.Rectangle
}

func (e *candidateExtent) down(p image.Point) {
	if !e.held {
		e.held, e.active = true, true
		e.current = p
		e.path = image.Rect(p.X, p.Y, p.X+1, p.Y+1)
	}
}

func (e *candidateExtent) at(rectangles []image.Rectangle, p image.Point) (image.Rectangle, bool) {
	if !e.held && e.active {
		if p.In(e.chosen) {
			e.current = p
			return e.chosen, true
		}
		e.active = false
	}
	e.current = p
	area := image.Rect(p.X, p.Y, p.X+1, p.Y+1)
	if e.active {
		if e.held {
			e.path = e.path.Union(area)
		}
		area = e.path
	}
	var best image.Rectangle
	for _, r := range rectangles {
		if area.In(r) && (best.Empty() || int64(r.Dx())*int64(r.Dy()) < int64(best.Dx())*int64(best.Dy())) {
			best = r
		}
	}
	e.chosen = best
	return best, !best.Empty()
}
