package selector

import "image"

// candidateExtent tracks the area indicated while Tab is held.
type candidateExtent struct {
	held, active    bool
	anchor, current image.Point
	chosen          image.Rectangle
}

func (e *candidateExtent) down(p image.Point) {
	if !e.held {
		e.held, e.active = true, true
		e.anchor, e.current = p, p
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
	anchor := p
	if e.active {
		anchor = e.anchor
	}
	var best image.Rectangle
	for _, r := range rectangles {
		if anchor.In(r) && p.In(r) && (best.Empty() || int64(r.Dx())*int64(r.Dy()) < int64(best.Dx())*int64(best.Dy())) {
			best = r
		}
	}
	e.chosen = best
	return best, !best.Empty()
}
