package selector

import (
	"image"
	"screenshot-win/internal/ui/glass"
)

func glassToolbarSize(count, dpi int) image.Point {
	m := glass.Margin(dpi)
	return image.Pt(glass.Scale(count*40, dpi)+2*m, glass.Scale(40, dpi)+2*m)
}

func glassToolbarButton(index, dpi int) image.Rectangle {
	m := glass.Margin(dpi)
	return image.Rect(m+glass.Scale(index*40, dpi), m, m+glass.Scale((index+1)*40, dpi), m+glass.Scale(40, dpi))
}

func glassToolbarActionAt(p image.Point, count, dpi int) (int, bool) {
	for i := 0; i < count; i++ {
		if p.In(glassToolbarButton(i, dpi)) {
			return i, true
		}
	}
	return 0, false
}
