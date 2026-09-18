package editor

import (
	"image"
	"image/color"
	"math"
)

// RectanglePreview snapshots document ordering once per interaction. It is an
// editing-only screen renderer: exported document pixels never contain gaps.
type RectanglePreview struct {
	source           image.Image
	annotations      []Annotation
	annotationBounds []image.Rectangle
	selected         AnnotationID
	draft            Annotation
	viewport         Viewport
	layout           RectangleLayout
	width            float64
}

func (document *Document) NewRectanglePreview(selected AnnotationID) *RectanglePreview {
	document.mu.RLock()
	defer document.mu.RUnlock()
	p := &RectanglePreview{source: document.original, annotations: cloneAnnotations(document.annotations), selected: selected}
	p.annotationBounds = make([]image.Rectangle, len(p.annotations))
	for i, a := range p.annotations {
		p.annotationBounds[i] = AnnotationBounds(a).Inset(-1)
	}
	return p
}

func (p *RectanglePreview) Update(draft Annotation, viewport Viewport, dpi int) {
	p.draft, p.viewport = draft, viewport
	p.width = math.Max(1, draft.Style.Width*viewport.scale())
	p.layout = LayoutRectangle(viewport.ImageToScreen(draft.Start), viewport.ImageToScreen(draft.End), dpi, p.width)
}

// ScreenColor preserves the selected annotation's original stacking position.
// Chrome is painted separately, above all annotations.
func (p *RectanglePreview) ScreenColor(screen image.Point) color.NRGBA {
	point := p.viewport.ScreenToImage(screen)
	b := p.source.Bounds()
	if !point.In(image.Rect(0, 0, b.Dx(), b.Dy())) {
		return color.NRGBA{A: 255}
	}
	x, y := b.Min.X+point.X, b.Min.Y+point.Y
	var base color.NRGBA
	switch source := p.source.(type) {
	case *image.RGBA:
		c := source.RGBAAt(x, y)
		if c.A == 255 {
			base = color.NRGBA{c.R, c.G, c.B, c.A}
		} else if c.A != 0 {
			alpha := uint32(c.A)
			base = color.NRGBA{uint8((uint32(c.R) * 65535 / alpha) >> 8), uint8((uint32(c.G) * 65535 / alpha) >> 8), uint8((uint32(c.B) * 65535 / alpha) >> 8), c.A}
		}
	case *image.NRGBA:
		base = source.NRGBAAt(x, y)
	default:
		base = color.NRGBAModel.Convert(source.At(x, y)).(color.NRGBA)
	}
	for i, a := range p.annotations {
		var coverage float64
		if a.ID == p.selected {
			a = p.draft
			coverage = p.layout.BorderCoverage(ScreenPoint{float64(screen.X), float64(screen.Y)}, p.width)
		} else {
			if !point.In(p.annotationBounds[i]) {
				continue
			}
			coverage = annotationCoverage(a, point)
		}
		if coverage > 0 {
			base = blendCoverage(base, a.Style.Color, coverage)
		}
	}
	return base
}
