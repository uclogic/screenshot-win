// Package editor implements screenshot-win's platform-independent, non-destructive
// annotation document and viewport model.
package editor

import (
	"errors"
	"image"
	"image/color"
	"math"
	"sync"
)

// AnnotationID is stable for the lifetime of an annotation, including across
// replacements and undo/redo snapshots.
type AnnotationID uint64

// Tool identifies the annotation created by a drag or click.
type Tool uint8

const (
	ToolRectangle Tool = iota
	ToolArrow
	ToolText
)

// Style is stored on every annotation so color and width changes remain
// non-destructive, independently editable, and part of document history.
type Style struct {
	Color color.NRGBA
	Width float64
}

func DefaultStyle() Style { return Style{Color: color.NRGBA{R: 255, G: 55, B: 55, A: 255}, Width: 3} }

// StyleField identifies the independently selectable parts of an annotation
// style. A zero field means that no style value changed.
type StyleField uint8

const (
	StyleFieldNone StyleField = iota
	StyleFieldColor
	StyleFieldWidth
)

// StyleChange carries the complete resulting style as well as the one field
// selected by the user. Keeping the complete style on toolbar events lets a
// newly-created annotation use exactly what the toolbar displays.
type StyleChange struct {
	Field StyleField
	Style Style
}

var presetColors = [...]color.NRGBA{
	{R: 255, G: 55, B: 55, A: 255},
	{R: 22, G: 140, B: 255, A: 255},
	{R: 45, G: 190, B: 85, A: 255},
	{R: 255, G: 190, B: 25, A: 255},
	{R: 255, G: 255, B: 255, A: 255},
}

var presetWidths = [...]float64{2, 3, 5, 8}

// PresetColors returns a copy of the compact annotation palette.
func PresetColors() []color.NRGBA { return append([]color.NRGBA(nil), presetColors[:]...) }

// PresetWidths returns a copy of the supported annotation line widths.
func PresetWidths() []float64 { return append([]float64(nil), presetWidths[:]...) }

// ApplyStyleChange updates only the requested field and preserves every other
// style property. Unknown fields leave the style unchanged.
func ApplyStyleChange(current Style, change StyleChange) Style {
	switch change.Field {
	case StyleFieldColor:
		current.Color = change.Style.Color
	case StyleFieldWidth:
		current.Width = change.Style.Width
	}
	return current
}

// Annotation coordinates are always expressed in original-image pixels.
type Annotation struct {
	ID         AnnotationID
	Tool       Tool
	Start, End image.Point
	Text       string
	Style      Style
	mask       *textMask
}

// TransformHandle identifies the part of an annotation manipulated by a drag.
type TransformHandle uint8

const (
	HandleMove TransformHandle = iota
	HandleArrowStart
	HandleArrowEnd
	HandleRectangleNorthWest
	HandleRectangleNorth
	HandleRectangleNorthEast
	HandleRectangleEast
	HandleRectangleSouthEast
	HandleRectangleSouth
	HandleRectangleSouthWest
	HandleRectangleWest
)

type documentSnapshot []Annotation

// Document owns an immutable original image and a reversible annotation list.
type Document struct {
	mu          sync.RWMutex
	original    image.Image
	annotations []Annotation
	undo        []documentSnapshot
	redo        []documentSnapshot
	nextID      AnnotationID
}

func NewDocument(original image.Image) (*Document, error) {
	if original == nil || original.Bounds().Empty() {
		return nil, errors.New("editor image must not be empty")
	}
	return &Document{original: original, nextID: 1}, nil
}

func (document *Document) Bounds() image.Rectangle {
	if document == nil || document.original == nil {
		return image.Rectangle{}
	}
	return image.Rect(0, 0, document.original.Bounds().Dx(), document.original.Bounds().Dy())
}

func (document *Document) Original() image.Image { return document.original }

func (document *Document) Annotations() []Annotation {
	document.mu.RLock()
	defer document.mu.RUnlock()
	return append([]Annotation(nil), document.annotations...)
}

func prepareAnnotation(annotation Annotation) (Annotation, error) {
	annotation.mask = nil
	if annotation.Style.Width <= 0 {
		return Annotation{}, errors.New("annotation width must be positive")
	}
	if annotation.Style.Color.A == 0 {
		annotation.Style.Color.A = 255
	}
	if annotation.Tool != ToolText && annotation.Start == annotation.End {
		return Annotation{}, errors.New("annotation must not be empty")
	}
	if annotation.Tool == ToolText && annotation.Text == "" {
		return Annotation{}, errors.New("text annotation must not be empty")
	}
	if annotation.Tool == ToolText {
		annotation.mask = rasterizeText(annotation.Text, max(1, int(math.Round(annotation.Style.Width))))
	}
	return annotation, nil
}

func (document *Document) Add(annotation Annotation) (AnnotationID, error) {
	if document == nil {
		return 0, errors.New("editor document is nil")
	}
	var err error
	annotation, err = prepareAnnotation(annotation)
	if err != nil {
		return 0, err
	}
	document.mu.Lock()
	defer document.mu.Unlock()
	document.recordMutationLocked()
	annotation.ID = document.nextID
	document.nextID++
	document.annotations = append(document.annotations, annotation)
	return annotation.ID, nil
}

func (document *Document) Get(id AnnotationID) (Annotation, bool) {
	if document == nil || id == 0 {
		return Annotation{}, false
	}
	document.mu.RLock()
	defer document.mu.RUnlock()
	for _, annotation := range document.annotations {
		if annotation.ID == id {
			return annotation, true
		}
	}
	return Annotation{}, false
}

func (document *Document) Replace(id AnnotationID, annotation Annotation) error {
	if document == nil {
		return errors.New("editor document is nil")
	}
	if id == 0 {
		return errors.New("annotation id must not be zero")
	}
	var err error
	annotation, err = prepareAnnotation(annotation)
	if err != nil {
		return err
	}
	document.mu.Lock()
	defer document.mu.Unlock()
	for index := range document.annotations {
		if document.annotations[index].ID == id {
			document.recordMutationLocked()
			annotation.ID = id
			document.annotations[index] = annotation
			return nil
		}
	}
	return errors.New("annotation was not found")
}

func (document *Document) Delete(id AnnotationID) bool {
	if document == nil || id == 0 {
		return false
	}
	document.mu.Lock()
	defer document.mu.Unlock()
	for index := range document.annotations {
		if document.annotations[index].ID == id {
			document.recordMutationLocked()
			copy(document.annotations[index:], document.annotations[index+1:])
			document.annotations = document.annotations[:len(document.annotations)-1]
			return true
		}
	}
	return false
}

func (document *Document) Undo() bool {
	document.mu.Lock()
	defer document.mu.Unlock()
	if len(document.undo) == 0 {
		return false
	}
	document.redo = append(document.redo, cloneAnnotations(document.annotations))
	document.annotations = cloneAnnotations(document.undo[len(document.undo)-1])
	document.undo = document.undo[:len(document.undo)-1]
	return true
}

func (document *Document) Redo() bool {
	document.mu.Lock()
	defer document.mu.Unlock()
	if len(document.redo) == 0 {
		return false
	}
	document.undo = append(document.undo, cloneAnnotations(document.annotations))
	last := document.redo[len(document.redo)-1]
	document.redo = document.redo[:len(document.redo)-1]
	document.annotations = cloneAnnotations(last)
	return true
}

func (document *Document) CanUndo() bool {
	document.mu.RLock()
	defer document.mu.RUnlock()
	return len(document.undo) > 0
}

func (document *Document) recordMutationLocked() {
	document.undo = append(document.undo, cloneAnnotations(document.annotations))
	document.redo = nil
}

func cloneAnnotations(annotations []Annotation) []Annotation {
	return append([]Annotation(nil), annotations...)
}

// HitTest returns the topmost annotation touching point. Tolerance is in
// original-image pixels and is normally derived from a fixed screen distance.
func (document *Document) HitTest(point image.Point, tolerance float64) (Annotation, bool) {
	if document == nil {
		return Annotation{}, false
	}
	document.mu.RLock()
	defer document.mu.RUnlock()
	for index := len(document.annotations) - 1; index >= 0; index-- {
		annotation := document.annotations[index]
		if annotationHit(annotation, point, tolerance) {
			return annotation, true
		}
	}
	return Annotation{}, false
}
func (document *Document) CanRedo() bool {
	document.mu.RLock()
	defer document.mu.RUnlock()
	return len(document.redo) > 0
}

// Rendered returns a lazy, original-sized image. Pixels are composited on
// demand, allowing encoders to process very tall screenshots one scanline at a
// time without allocating another full-size bitmap.
func (document *Document) Rendered() image.Image {
	document.mu.RLock()
	defer document.mu.RUnlock()
	return &renderedImage{source: document.original, bounds: document.Bounds(), annotations: cloneAnnotations(document.annotations)}
}

// RenderedPreview returns a lazy render in which replaceID is replaced by
// draft. A zero replaceID appends the draft. The document and its history are
// not modified.
func (document *Document) RenderedPreview(replaceID AnnotationID, draft *Annotation) image.Image {
	document.mu.RLock()
	defer document.mu.RUnlock()
	annotations := cloneAnnotations(document.annotations)
	if draft != nil {
		preview := *draft
		if preview.Tool == ToolText && preview.mask == nil && preview.Text != "" {
			preview.mask = rasterizeText(preview.Text, max(1, int(math.Round(preview.Style.Width))))
		}
		replaced := false
		if replaceID != 0 {
			for index := range annotations {
				if annotations[index].ID == replaceID {
					preview.ID = replaceID
					annotations[index] = preview
					replaced = true
					break
				}
			}
		}
		if !replaced && replaceID == 0 {
			annotations = append(annotations, preview)
		}
	}
	return &renderedImage{source: document.original, bounds: document.Bounds(), annotations: annotations}
}

// RenderedWithout returns a lazy render with one annotation omitted. It is
// used by interactive transforms to restore the pixels beneath a moving
// vector without rasterizing its entire axis-aligned bounding box.
func (document *Document) RenderedWithout(id AnnotationID) image.Image {
	document.mu.RLock()
	defer document.mu.RUnlock()
	annotations := make([]Annotation, 0, len(document.annotations))
	for _, annotation := range document.annotations {
		if annotation.ID != id {
			annotations = append(annotations, annotation)
		}
	}
	return &renderedImage{source: document.original, bounds: document.Bounds(), annotations: annotations}
}

type renderedImage struct {
	source      image.Image
	bounds      image.Rectangle
	annotations []Annotation
}

func (rendered *renderedImage) Bounds() image.Rectangle { return rendered.bounds }
func (rendered *renderedImage) ColorModel() color.Model { return color.NRGBAModel }
func (rendered *renderedImage) At(x, y int) color.Color {
	if !(image.Pt(x, y).In(rendered.bounds)) {
		return color.NRGBA{}
	}
	sb := rendered.source.Bounds()
	base := color.NRGBAModel.Convert(rendered.source.At(sb.Min.X+x, sb.Min.Y+y)).(color.NRGBA)
	for _, annotation := range rendered.annotations {
		if coverage := annotationCoverage(annotation, image.Pt(x, y)); coverage > 0 {
			base = blendCoverage(base, annotation.Style.Color, coverage)
		}
	}
	return base
}

func annotationCoverage(annotation Annotation, point image.Point) float64 {
	radius := math.Max(.5, annotation.Style.Width/2)
	switch annotation.Tool {
	case ToolRectangle:
		r := normalized(annotation.Start, annotation.End)
		return maxCoverage(
			strokeCoverage(distanceToSegment(point, r.Min, image.Pt(r.Max.X, r.Min.Y)), radius),
			strokeCoverage(distanceToSegment(point, image.Pt(r.Max.X, r.Min.Y), r.Max), radius),
			strokeCoverage(distanceToSegment(point, r.Max, image.Pt(r.Min.X, r.Max.Y)), radius),
			strokeCoverage(distanceToSegment(point, image.Pt(r.Min.X, r.Max.Y), r.Min), radius),
		)
	case ToolArrow:
		coverage := strokeCoverage(distanceToSegment(point, annotation.Start, annotation.End), radius)
		left, right, ok := arrowHead(annotation.Start, annotation.End, annotation.Style.Width)
		if !ok {
			return coverage
		}
		return maxCoverage(coverage,
			strokeCoverage(distanceToSegment(point, annotation.End, left), radius),
			strokeCoverage(distanceToSegment(point, annotation.End, right), radius),
		)
	case ToolText:
		if annotation.mask != nil {
			if annotation.mask.at(point.X-annotation.Start.X, point.Y-annotation.Start.Y) {
				return 1
			}
			return 0
		}
		if textPixel(annotation.Text, point.X-annotation.Start.X, point.Y-annotation.Start.Y, max(1, int(math.Round(annotation.Style.Width)))) {
			return 1
		}
		return 0
	default:
		return 0
	}
}

// strokeCoverage converts distance from a pixel center to a stroke into a
// smooth one-pixel transition at the edge.
func strokeCoverage(distance, radius float64) float64 {
	return math.Max(0, math.Min(1, radius+.5-distance))
}

func maxCoverage(values ...float64) float64 {
	maximum := 0.0
	for _, value := range values {
		maximum = math.Max(maximum, value)
	}
	return maximum
}

func annotationHit(annotation Annotation, point image.Point, tolerance float64) bool {
	radius := math.Max(math.Max(.5, annotation.Style.Width/2), tolerance)
	switch annotation.Tool {
	case ToolRectangle:
		r := normalized(annotation.Start, annotation.End)
		return distanceToSegment(point, r.Min, image.Pt(r.Max.X, r.Min.Y)) <= radius ||
			distanceToSegment(point, image.Pt(r.Max.X, r.Min.Y), r.Max) <= radius ||
			distanceToSegment(point, r.Max, image.Pt(r.Min.X, r.Max.Y)) <= radius ||
			distanceToSegment(point, image.Pt(r.Min.X, r.Max.Y), r.Min) <= radius
	case ToolArrow:
		if distanceToSegment(point, annotation.Start, annotation.End) <= radius {
			return true
		}
		left, right, ok := arrowHead(annotation.Start, annotation.End, annotation.Style.Width)
		if !ok {
			return false
		}
		return distanceToSegment(point, annotation.End, left) <= radius || distanceToSegment(point, annotation.End, right) <= radius
	case ToolText:
		return point.In(expandRectangle(AnnotationBounds(annotation), int(math.Ceil(radius))))
	default:
		return false
	}
}

func arrowHead(start, end image.Point, width float64) (image.Point, image.Point, bool) {
	dx, dy := float64(end.X-start.X), float64(end.Y-start.Y)
	length := math.Hypot(dx, dy)
	if length == 0 {
		return image.Point{}, image.Point{}, false
	}
	head := math.Min(18+width*2, length*.45)
	ux, uy := dx/length, dy/length
	left := image.Pt(int(math.Round(float64(end.X)-ux*head-uy*head*.55)), int(math.Round(float64(end.Y)-uy*head+ux*head*.55)))
	right := image.Pt(int(math.Round(float64(end.X)-ux*head+uy*head*.55)), int(math.Round(float64(end.Y)-uy*head-ux*head*.55)))
	return left, right, true
}

// AnnotationBounds returns the original-image bounding box of an annotation.
func AnnotationBounds(annotation Annotation) image.Rectangle {
	switch annotation.Tool {
	case ToolText:
		if annotation.mask != nil {
			return image.Rect(annotation.Start.X, annotation.Start.Y, annotation.Start.X+annotation.mask.width, annotation.Start.Y+annotation.mask.height)
		}
		scale := max(1, int(math.Round(annotation.Style.Width)))
		return image.Rect(annotation.Start.X, annotation.Start.Y, annotation.Start.X+len([]rune(annotation.Text))*6*scale, annotation.Start.Y+8*scale)
	case ToolArrow:
		points := []image.Point{annotation.Start, annotation.End}
		if left, right, ok := arrowHead(annotation.Start, annotation.End, annotation.Style.Width); ok {
			points = append(points, left, right)
		}
		minimum, maximum := points[0], points[0]
		for _, point := range points[1:] {
			minimum.X, minimum.Y = min(minimum.X, point.X), min(minimum.Y, point.Y)
			maximum.X, maximum.Y = max(maximum.X, point.X), max(maximum.Y, point.Y)
		}
		radius := int(math.Ceil(math.Max(.5, annotation.Style.Width/2)))
		return image.Rect(minimum.X-radius, minimum.Y-radius, maximum.X+radius+1, maximum.Y+radius+1)
	case ToolRectangle:
		radius := int(math.Ceil(math.Max(.5, annotation.Style.Width/2)))
		minimum := image.Pt(min(annotation.Start.X, annotation.End.X)-radius, min(annotation.Start.Y, annotation.End.Y)-radius)
		maximum := image.Pt(max(annotation.Start.X, annotation.End.X)+radius+1, max(annotation.Start.Y, annotation.End.Y)+radius+1)
		return image.Rectangle{Min: minimum, Max: maximum}
	default:
		return image.Rectangle{}
	}
}

func expandRectangle(bounds image.Rectangle, amount int) image.Rectangle {
	return image.Rect(bounds.Min.X-amount, bounds.Min.Y-amount, bounds.Max.X+amount, bounds.Max.Y+amount)
}

// Translate moves an annotation by delta while keeping it inside bounds when
// the annotation fits on that axis.
func Translate(annotation Annotation, delta image.Point, bounds image.Rectangle) Annotation {
	area := AnnotationBounds(annotation)
	delta.X = clampedTranslation(area.Min.X, area.Max.X, delta.X, bounds.Min.X, bounds.Max.X)
	delta.Y = clampedTranslation(area.Min.Y, area.Max.Y, delta.Y, bounds.Min.Y, bounds.Max.Y)
	annotation.Start = annotation.Start.Add(delta)
	annotation.End = annotation.End.Add(delta)
	return annotation
}

func clampedTranslation(minimum, maximum, delta, boundMinimum, boundMaximum int) int {
	size, boundSize := maximum-minimum, boundMaximum-boundMinimum
	if size >= boundSize {
		return boundMinimum - minimum
	}
	target := minimum + delta
	target = max(boundMinimum, min(target, boundMaximum-size))
	return target - minimum
}

// TransformTo moves one resize handle to point. Arrow handles move one end;
// rectangle handles modify one or two normalized edges.
func TransformTo(annotation Annotation, handle TransformHandle, point image.Point, bounds image.Rectangle) Annotation {
	point = clampTransformPoint(bounds, point)
	switch annotation.Tool {
	case ToolArrow:
		if handle == HandleArrowStart {
			annotation.Start = point
		} else if handle == HandleArrowEnd {
			annotation.End = point
		}
	case ToolRectangle:
		left, right := min(annotation.Start.X, annotation.End.X), max(annotation.Start.X, annotation.End.X)
		top, bottom := min(annotation.Start.Y, annotation.End.Y), max(annotation.Start.Y, annotation.End.Y)
		switch handle {
		case HandleRectangleNorthWest:
			left, top = point.X, point.Y
		case HandleRectangleNorth:
			top = point.Y
		case HandleRectangleNorthEast:
			right, top = point.X, point.Y
		case HandleRectangleEast:
			right = point.X
		case HandleRectangleSouthEast:
			right, bottom = point.X, point.Y
		case HandleRectangleSouth:
			bottom = point.Y
		case HandleRectangleSouthWest:
			left, bottom = point.X, point.Y
		case HandleRectangleWest:
			left = point.X
		}
		annotation.Start = image.Pt(min(left, right), min(top, bottom))
		annotation.End = image.Pt(max(left, right), max(top, bottom))
	}
	return annotation
}

func clampTransformPoint(bounds image.Rectangle, point image.Point) image.Point {
	maximum := image.Pt(max(bounds.Min.X, bounds.Max.X-1), max(bounds.Min.Y, bounds.Max.Y-1))
	return image.Pt(max(bounds.Min.X, min(point.X, maximum.X)), max(bounds.Min.Y, min(point.Y, maximum.Y)))
}

func normalized(a, b image.Point) image.Rectangle {
	return image.Rect(min(a.X, b.X), min(a.Y, b.Y), max(a.X, b.X), max(a.Y, b.Y))
}

func distanceToSegment(p, a, b image.Point) float64 {
	px, py := float64(p.X), float64(p.Y)
	ax, ay := float64(a.X), float64(a.Y)
	bx, by := float64(b.X), float64(b.Y)
	dx, dy := bx-ax, by-ay
	if dx == 0 && dy == 0 {
		return math.Hypot(px-ax, py-ay)
	}
	t := ((px-ax)*dx + (py-ay)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy))
}

func blend(destination, source color.NRGBA) color.NRGBA {
	a := uint32(source.A)
	inverse := 255 - a
	return color.NRGBA{R: uint8((uint32(source.R)*a + uint32(destination.R)*inverse) / 255), G: uint8((uint32(source.G)*a + uint32(destination.G)*inverse) / 255), B: uint8((uint32(source.B)*a + uint32(destination.B)*inverse) / 255), A: 255}
}

func blendCoverage(destination, source color.NRGBA, coverage float64) color.NRGBA {
	source.A = uint8(math.Round(float64(source.A) * math.Max(0, math.Min(1, coverage))))
	return blend(destination, source)
}
