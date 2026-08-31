//go:build windows

package selector

import (
	"image"
	"image/color"
	"math"
	"sync"
	"syscall"
	"unsafe"

	"screenshot-win/editor"
)

const (
	gdipOK                 = 0
	gdipUnitPixel          = 2
	gdipSmoothingAntiAlias = 4
	gdipLineCapRound       = 2
	gdipLineJoinRound      = 2
	gdipFillModeAlternate  = 0
	toolbarGlyphViewBox    = float32(24)
	toolbarGlyphPixels96   = 20
)

var (
	gdiplus = syscall.NewLazyDLL("gdiplus.dll")

	procGdiplusStartup       = gdiplus.NewProc("GdiplusStartup")
	procGdipCreateFromHDC    = gdiplus.NewProc("GdipCreateFromHDC")
	procGdipDeleteGraphics   = gdiplus.NewProc("GdipDeleteGraphics")
	procGdipSetSmoothingMode = gdiplus.NewProc("GdipSetSmoothingMode")
	procGdipCreatePath       = gdiplus.NewProc("GdipCreatePath")
	procGdipDeletePath       = gdiplus.NewProc("GdipDeletePath")
	procGdipStartPathFigure  = gdiplus.NewProc("GdipStartPathFigure")
	procGdipClosePathFigure  = gdiplus.NewProc("GdipClosePathFigure")
	procGdipAddPathLine2     = gdiplus.NewProc("GdipAddPathLine2")
	procGdipAddPathBeziers   = gdiplus.NewProc("GdipAddPathBeziers")
	procGdipCreatePen1       = gdiplus.NewProc("GdipCreatePen1")
	procGdipDeletePen        = gdiplus.NewProc("GdipDeletePen")
	procGdipSetPenStartCap   = gdiplus.NewProc("GdipSetPenStartCap")
	procGdipSetPenEndCap     = gdiplus.NewProc("GdipSetPenEndCap")
	procGdipSetPenLineJoin   = gdiplus.NewProc("GdipSetPenLineJoin")
	procGdipDrawPath         = gdiplus.NewProc("GdipDrawPath")
	procGdipCreateSolidFill  = gdiplus.NewProc("GdipCreateSolidFill")
	procGdipDeleteBrush      = gdiplus.NewProc("GdipDeleteBrush")
	procGdipFillEllipseI     = gdiplus.NewProc("GdipFillEllipseI")
	procGdipDrawEllipseI     = gdiplus.NewProc("GdipDrawEllipseI")

	toolbarGDIPlusOnce  sync.Once
	toolbarGDIPlusToken uintptr
	toolbarGDIPlusReady bool
)

type gdiplusStartupInput struct {
	Version                  uint32
	DebugEventCallback       uintptr
	SuppressBackgroundThread int32
	SuppressExternalCodecs   int32
}

type gdipPointF struct {
	X float32
	Y float32
}

type toolbarGlyphTransform struct {
	x     float32
	y     float32
	scale float32
}

func (transform toolbarGlyphTransform) point(x, y float32) gdipPointF {
	return gdipPointF{X: transform.x + x*transform.scale, Y: transform.y + y*transform.scale}
}

type toolbarIconRenderer struct {
	dc       uintptr
	graphics uintptr
}

func newToolbarIconRenderer(dc uintptr) *toolbarIconRenderer {
	renderer := &toolbarIconRenderer{dc: dc}
	if dc == 0 || !ensureToolbarGDIPlus() {
		return renderer
	}
	status, _, _ := procGdipCreateFromHDC.Call(dc, uintptr(unsafe.Pointer(&renderer.graphics)))
	if status != gdipOK || renderer.graphics == 0 {
		renderer.graphics = 0
		return renderer
	}
	procGdipSetSmoothingMode.Call(renderer.graphics, gdipSmoothingAntiAlias)
	return renderer
}

func ensureToolbarGDIPlus() bool {
	toolbarGDIPlusOnce.Do(func() {
		input := gdiplusStartupInput{Version: 1}
		status, _, _ := procGdiplusStartup.Call(
			uintptr(unsafe.Pointer(&toolbarGDIPlusToken)),
			uintptr(unsafe.Pointer(&input)),
			0,
		)
		toolbarGDIPlusReady = status == gdipOK && toolbarGDIPlusToken != 0
		// The token intentionally remains live until process teardown. Toolbars
		// can be created on several UI threads over the tray host's lifetime.
	})
	return toolbarGDIPlusReady
}

func (renderer *toolbarIconRenderer) close() {
	if renderer == nil || renderer.graphics == 0 {
		return
	}
	procGdipDeleteGraphics.Call(renderer.graphics)
	renderer.graphics = 0
}

func (renderer *toolbarIconRenderer) draw(action Action, button image.Rectangle, enabled bool, style editor.Style, dpi int) {
	if renderer == nil {
		return
	}
	glyph, ok := toolbarGlyphForAction(action)
	if !ok {
		return
	}
	if dpi <= 0 {
		dpi = 96
	}
	size := float32(scaleForDPI(toolbarGlyphPixels96, dpi))
	maximum := float32(min(button.Dx(), button.Dy()) - scaleForDPI(4, dpi))
	if maximum < size {
		size = maximum
	}
	if size <= 0 {
		return
	}
	transform := toolbarGlyphTransform{
		x:     float32(button.Min.X+button.Max.X)/2 - size/2,
		y:     float32(button.Min.Y+button.Max.Y)/2 - size/2,
		scale: size / toolbarGlyphViewBox,
	}
	stroke := toolbarIconStrokeWidth(dpi)
	base := toolbarIconBaseColor(enabled)
	if renderer.graphics == 0 || !renderer.drawGlyph(glyph, transform, base, stroke) {
		renderer.drawFallbackGlyph(glyph, transform, base, max(1, int(math.Round(float64(stroke)))))
	}

	switch action {
	case ActionColor:
		renderer.drawSelectedColor(transform, style.Color, enabled, dpi)
	case ActionWidth:
		renderer.drawSelectedWidth(transform, style, enabled, dpi)
	}
}

func (renderer *toolbarIconRenderer) drawGlyph(glyph toolbarGlyph, transform toolbarGlyphTransform, strokeColor color.NRGBA, strokeWidth float32) bool {
	if renderer.graphics == 0 {
		return false
	}
	var path uintptr
	status, _, _ := procGdipCreatePath.Call(gdipFillModeAlternate, uintptr(unsafe.Pointer(&path)))
	if status != gdipOK || path == 0 {
		return false
	}
	defer procGdipDeletePath.Call(path)

	var current gdipPointF
	hasCurrent := false
	for _, command := range glyph.commands {
		switch command.op {
		case toolbarGlyphMove:
			current = transform.point(command.args[0], command.args[1])
			hasCurrent = true
			if result, _, _ := procGdipStartPathFigure.Call(path); result != gdipOK {
				return false
			}
		case toolbarGlyphLine:
			if !hasCurrent {
				return false
			}
			next := transform.point(command.args[0], command.args[1])
			points := [2]gdipPointF{current, next}
			if result, _, _ := procGdipAddPathLine2.Call(path, uintptr(unsafe.Pointer(&points[0])), uintptr(len(points))); result != gdipOK {
				return false
			}
			current = next
		case toolbarGlyphCubic:
			if !hasCurrent {
				return false
			}
			points := [4]gdipPointF{
				current,
				transform.point(command.args[0], command.args[1]),
				transform.point(command.args[2], command.args[3]),
				transform.point(command.args[4], command.args[5]),
			}
			if result, _, _ := procGdipAddPathBeziers.Call(path, uintptr(unsafe.Pointer(&points[0])), uintptr(len(points))); result != gdipOK {
				return false
			}
			current = points[3]
		case toolbarGlyphClose:
			if result, _, _ := procGdipClosePathFigure.Call(path); result != gdipOK {
				return false
			}
			hasCurrent = false
		default:
			return false
		}
	}

	pen, ok := createToolbarGDIPlusPen(strokeColor, strokeWidth)
	if !ok {
		return false
	}
	defer procGdipDeletePen.Call(pen)
	result, _, _ := procGdipDrawPath.Call(renderer.graphics, pen, path)
	return result == gdipOK
}

func createToolbarGDIPlusPen(value color.NRGBA, width float32) (uintptr, bool) {
	if width < 1 {
		width = 1
	}
	var pen uintptr
	status, _, _ := procGdipCreatePen1.Call(
		uintptr(toolbarARGB(value)),
		uintptr(math.Float32bits(width)),
		gdipUnitPixel,
		uintptr(unsafe.Pointer(&pen)),
	)
	if status != gdipOK || pen == 0 {
		return 0, false
	}
	procGdipSetPenStartCap.Call(pen, gdipLineCapRound)
	procGdipSetPenEndCap.Call(pen, gdipLineCapRound)
	procGdipSetPenLineJoin.Call(pen, gdipLineJoinRound)
	return pen, true
}

func toolbarARGB(value color.NRGBA) uint32 {
	alpha := value.A
	if alpha == 0 {
		alpha = 255
	}
	return uint32(alpha)<<24 | uint32(value.R)<<16 | uint32(value.G)<<8 | uint32(value.B)
}

func (renderer *toolbarIconRenderer) drawSelectedColor(transform toolbarGlyphTransform, selected color.NRGBA, enabled bool, dpi int) {
	value := toolbarIconValueColor(selected, enabled)
	outline := color.NRGBA{R: 22, G: 24, B: 28, A: 255}
	if !enabled {
		outline = color.NRGBA{R: 80, G: 82, B: 86, A: 255}
	}
	center := transform.point(18, 18)
	radius := float32(3) * transform.scale
	left := int32(math.Round(float64(center.X - radius)))
	top := int32(math.Round(float64(center.Y - radius)))
	diameter := int32(math.Round(float64(radius * 2)))
	if diameter < 2 {
		diameter = 2
	}
	if renderer.graphics == 0 {
		renderer.drawFallbackColorDot(left, top, diameter, value, outline)
		return
	}
	var brush uintptr
	status, _, _ := procGdipCreateSolidFill.Call(uintptr(toolbarARGB(value)), uintptr(unsafe.Pointer(&brush)))
	if status != gdipOK || brush == 0 {
		return
	}
	defer procGdipDeleteBrush.Call(brush)
	procGdipFillEllipseI.Call(renderer.graphics, brush, uintptr(left), uintptr(top), uintptr(diameter), uintptr(diameter))
	pen, ok := createToolbarGDIPlusPen(outline, max(float32(1), float32(dpi)/96))
	if !ok {
		return
	}
	defer procGdipDeletePen.Call(pen)
	procGdipDrawEllipseI.Call(renderer.graphics, pen, uintptr(left), uintptr(top), uintptr(diameter), uintptr(diameter))
}

func (renderer *toolbarIconRenderer) drawSelectedWidth(transform toolbarGlyphTransform, style editor.Style, enabled bool, dpi int) {
	value := toolbarIconValueColor(style.Color, enabled)
	width := toolbarValueStrokeWidth(style.Width, dpi)
	overlay := toolbarGlyph{
		commands: []toolbarGlyphCommand{glyphMove(7.5, 16.5), glyphLine(16.5, 7.5)},
	}
	if renderer.graphics != 0 && renderer.drawGlyph(overlay, transform, value, width) {
		return
	}
	renderer.drawFallbackGlyph(overlay, transform, value, max(1, int(math.Round(float64(width)))))
}

func (renderer *toolbarIconRenderer) drawFallbackGlyph(glyph toolbarGlyph, transform toolbarGlyphTransform, strokeColor color.NRGBA, strokeWidth int) {
	if renderer.dc == 0 {
		return
	}
	pen, _, _ := procCreatePen.Call(psSolid, uintptr(max(1, strokeWidth)), rgb(strokeColor.R, strokeColor.G, strokeColor.B))
	if pen == 0 {
		return
	}
	defer procDeleteObject.Call(pen)
	oldPen, _, _ := procSelectObject.Call(renderer.dc, pen)
	defer procSelectObject.Call(renderer.dc, oldPen)

	var current image.Point
	var start image.Point
	hasCurrent := false
	for _, command := range glyph.commands {
		switch command.op {
		case toolbarGlyphMove:
			current = roundedGlyphPoint(transform.point(command.args[0], command.args[1]))
			start = current
			hasCurrent = true
			procMoveToEx.Call(renderer.dc, uintptr(current.X), uintptr(current.Y), 0)
		case toolbarGlyphLine:
			if !hasCurrent {
				continue
			}
			current = roundedGlyphPoint(transform.point(command.args[0], command.args[1]))
			procLineTo.Call(renderer.dc, uintptr(current.X), uintptr(current.Y))
		case toolbarGlyphCubic:
			if !hasCurrent {
				continue
			}
			current = roundedGlyphPoint(transform.point(command.args[4], command.args[5]))
			procLineTo.Call(renderer.dc, uintptr(current.X), uintptr(current.Y))
		case toolbarGlyphClose:
			if hasCurrent {
				procLineTo.Call(renderer.dc, uintptr(start.X), uintptr(start.Y))
			}
			hasCurrent = false
		}
	}
}

func roundedGlyphPoint(point gdipPointF) image.Point {
	return image.Pt(int(math.Round(float64(point.X))), int(math.Round(float64(point.Y))))
}

func (renderer *toolbarIconRenderer) drawFallbackColorDot(left, top, diameter int32, fill, outline color.NRGBA) {
	if renderer.dc == 0 {
		return
	}
	brush, _, _ := procCreateSolidBrush.Call(rgb(fill.R, fill.G, fill.B))
	if brush == 0 {
		return
	}
	defer procDeleteObject.Call(brush)
	oldBrush, _, _ := procSelectObject.Call(renderer.dc, brush)
	defer procSelectObject.Call(renderer.dc, oldBrush)
	pen, _, _ := procCreatePen.Call(psSolid, 1, rgb(outline.R, outline.G, outline.B))
	if pen == 0 {
		return
	}
	defer procDeleteObject.Call(pen)
	oldPen, _, _ := procSelectObject.Call(renderer.dc, pen)
	defer procSelectObject.Call(renderer.dc, oldPen)
	procEllipse.Call(renderer.dc, uintptr(left), uintptr(top), uintptr(left+diameter), uintptr(top+diameter))
}
