package selector

import "image"

// candidateGesture is independent of Win32 so click/drag decisions are testable.
type candidateGesture struct {
	pressed, manual bool
	anchor, current image.Point
	locked          image.Rectangle
	threshold       int
}

func (g *candidateGesture) down(p image.Point, candidate image.Rectangle) {
	g.pressed, g.manual = true, false
	g.anchor, g.current, g.locked = p, p, candidate
}

func (g *candidateGesture) move(p image.Point) {
	if !g.pressed {
		return
	}
	g.current = p
	if candidateAbs(p.X-g.anchor.X) >= max(1, g.threshold) || candidateAbs(p.Y-g.anchor.Y) >= max(1, g.threshold) {
		g.manual = true
	}
}

func (g *candidateGesture) up(p image.Point, bounds image.Rectangle) (image.Rectangle, bool) {
	if !g.pressed {
		return image.Rectangle{}, false
	}
	g.move(p)
	g.pressed = false
	r := g.locked
	if g.manual {
		r = dragRectangle(bounds, g.anchor, g.current)
	}
	return r, !r.Empty()
}
