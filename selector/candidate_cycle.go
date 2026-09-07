package selector

import "image"

// candidateCycle retains a keyboard choice until the pointer moves.
type candidateCycle struct {
	point image.Point
	index int
}

func (c *candidateCycle) at(rectangles []image.Rectangle, p image.Point, step int) (image.Rectangle, bool) {
	if p != c.point {
		c.point, c.index = p, 0
	}
	count := 0
	for _, r := range rectangles {
		if p.In(r) {
			count++
		}
	}
	if count == 0 {
		c.index = 0
		return image.Rectangle{}, false
	}
	c.index = ((c.index+step)%count + count) % count
	index := 0
	for _, r := range rectangles {
		if p.In(r) {
			if index == c.index {
				return r, true
			}
			index++
		}
	}
	return image.Rectangle{}, false
}
