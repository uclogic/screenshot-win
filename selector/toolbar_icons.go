package selector

import "image/color"

// toolbarGlyphCommand is the small, runtime-independent subset of SVG path
// commands needed by the pinned Tabler toolbar icons. Coordinates stay in the
// original 24 x 24 view box; the Windows renderer scales them for the target
// monitor DPI.
type toolbarGlyphCommand struct {
	op   toolbarGlyphOp
	args [6]float32
}

type toolbarGlyphOp uint8

const (
	toolbarGlyphMove toolbarGlyphOp = iota
	toolbarGlyphLine
	toolbarGlyphCubic
	toolbarGlyphClose
)

type toolbarGlyph struct {
	source   string
	commands []toolbarGlyphCommand
}

func glyphMove(x, y float32) toolbarGlyphCommand {
	return toolbarGlyphCommand{op: toolbarGlyphMove, args: [6]float32{x, y}}
}

func glyphLine(x, y float32) toolbarGlyphCommand {
	return toolbarGlyphCommand{op: toolbarGlyphLine, args: [6]float32{x, y}}
}

func glyphCubic(x1, y1, x2, y2, x3, y3 float32) toolbarGlyphCommand {
	return toolbarGlyphCommand{op: toolbarGlyphCubic, args: [6]float32{x1, y1, x2, y2, x3, y3}}
}

func glyphClose() toolbarGlyphCommand { return toolbarGlyphCommand{op: toolbarGlyphClose} }

const circleBezier = float32(0.55228475)

func glyphCircle(cx, cy, radius float32) []toolbarGlyphCommand {
	k := radius * circleBezier
	return []toolbarGlyphCommand{
		glyphMove(cx+radius, cy),
		glyphCubic(cx+radius, cy+k, cx+k, cy+radius, cx, cy+radius),
		glyphCubic(cx-k, cy+radius, cx-radius, cy+k, cx-radius, cy),
		glyphCubic(cx-radius, cy-k, cx-k, cy-radius, cx, cy-radius),
		glyphCubic(cx+k, cy-radius, cx+radius, cy-k, cx+radius, cy),
		glyphClose(),
	}
}

func glyphRoundedRectangle(x, y, width, height, radius float32) []toolbarGlyphCommand {
	k := radius * circleBezier
	right := x + width
	bottom := y + height
	return []toolbarGlyphCommand{
		glyphMove(x+radius, y),
		glyphLine(right-radius, y),
		glyphCubic(right-radius+k, y, right, y+radius-k, right, y+radius),
		glyphLine(right, bottom-radius),
		glyphCubic(right, bottom-radius+k, right-radius+k, bottom, right-radius, bottom),
		glyphLine(x+radius, bottom),
		glyphCubic(x+radius-k, bottom, x, bottom-radius+k, x, bottom-radius),
		glyphLine(x, y+radius),
		glyphCubic(x, y+radius-k, x+radius-k, y, x+radius, y),
		glyphClose(),
	}
}

func appendGlyph(parts ...[]toolbarGlyphCommand) []toolbarGlyphCommand {
	var commands []toolbarGlyphCommand
	for _, part := range parts {
		commands = append(commands, part...)
	}
	return commands
}

var toolbarGlyphs = buildToolbarGlyphs()

func buildToolbarGlyphs() map[Action]toolbarGlyph {
	return map[Action]toolbarGlyph{
		ActionCancel: {
			source: "x.svg",
			commands: []toolbarGlyphCommand{
				glyphMove(18, 6), glyphLine(6, 18),
				glyphMove(6, 6), glyphLine(18, 18),
			},
		},
		ActionSave: {
			source: "device-floppy.svg",
			commands: appendGlyph(
				[]toolbarGlyphCommand{
					glyphMove(6, 4), glyphLine(16, 4), glyphLine(20, 8), glyphLine(20, 18),
					glyphCubic(20, 19.105, 19.105, 20, 18, 20), glyphLine(6, 20),
					glyphCubic(4.895, 20, 4, 19.105, 4, 18), glyphLine(4, 6),
					glyphCubic(4, 4.895, 4.895, 4, 6, 4), glyphClose(),
				},
				glyphCircle(12, 14, 2),
				[]toolbarGlyphCommand{glyphMove(14, 4), glyphLine(14, 8), glyphLine(8, 8), glyphLine(8, 4)},
			),
		},
		ActionCopy: {
			source: "copy.svg",
			commands: appendGlyph(
				glyphRoundedRectangle(7, 7, 14, 14, 2.667),
				[]toolbarGlyphCommand{
					glyphMove(4.012, 16.737), glyphCubic(3.385, 16.377, 3, 15.71, 3, 15),
					glyphLine(3, 5), glyphCubic(3, 3.9, 3.9, 3, 5, 3), glyphLine(15, 3),
					glyphCubic(15.75, 3, 16.158, 3.385, 16.5, 4),
				},
			),
		},
		ActionScroll: {
			source: "square-rounded-arrow-down.svg",
			commands: []toolbarGlyphCommand{
				glyphMove(8, 12), glyphLine(12, 16), glyphLine(16, 12),
				glyphMove(12, 8), glyphLine(12, 16),
				glyphMove(12, 3), glyphCubic(19.2, 3, 21, 4.8, 21, 12),
				glyphCubic(21, 19.2, 19.2, 21, 12, 21), glyphCubic(4.8, 21, 3, 19.2, 3, 12),
				glyphCubic(3, 4.8, 4.8, 3, 12, 3), glyphClose(),
			},
		},
		ActionSaveAs: {
			source: "file-download.svg",
			commands: []toolbarGlyphCommand{
				glyphMove(14, 3), glyphLine(14, 7), glyphCubic(14, 7.552, 14.448, 8, 15, 8), glyphLine(19, 8),
				glyphMove(17, 21), glyphLine(7, 21), glyphCubic(5.895, 21, 5, 20.105, 5, 19),
				glyphLine(5, 5), glyphCubic(5, 3.895, 5.895, 3, 7, 3), glyphLine(14, 3),
				glyphLine(19, 8), glyphLine(19, 19), glyphCubic(19, 20.105, 18.105, 21, 17, 21),
				glyphMove(12, 17), glyphLine(12, 11),
				glyphMove(9.5, 14.5), glyphLine(12, 17), glyphLine(14.5, 14.5),
			},
		},
		ActionPin: {
			source: "pin.svg",
			commands: []toolbarGlyphCommand{
				glyphMove(15, 4.5), glyphLine(11, 8.5), glyphLine(7, 10), glyphLine(5.5, 11.5),
				glyphLine(12.5, 18.5), glyphLine(14, 17), glyphLine(15.5, 13), glyphLine(19.5, 9),
				glyphMove(9, 15), glyphLine(4.5, 19.5),
				glyphMove(14.5, 4), glyphLine(20, 9.5),
			},
		},
		ActionEdit: {
			source: "edit.svg",
			commands: []toolbarGlyphCommand{
				glyphMove(7, 7), glyphLine(6, 7), glyphCubic(4.895, 7, 4, 7.895, 4, 9),
				glyphLine(4, 18), glyphCubic(4, 19.105, 4.895, 20, 6, 20), glyphLine(15, 20),
				glyphCubic(16.105, 20, 17, 19.105, 17, 18), glyphLine(17, 17),
				glyphMove(20.385, 6.585), glyphCubic(21.205, 5.765, 21.205, 4.435, 20.385, 3.615),
				glyphCubic(19.565, 2.795, 18.235, 2.795, 17.415, 3.615), glyphLine(9, 12),
				glyphLine(9, 15), glyphLine(12, 15), glyphLine(20.385, 6.585),
				glyphMove(16, 5), glyphLine(19, 8),
			},
		},
		ActionRectangle: {
			source:   "rectangle.svg",
			commands: glyphRoundedRectangle(3, 5, 18, 14, 2),
		},
		ActionArrow: {
			source: "arrow-up-right.svg",
			commands: []toolbarGlyphCommand{
				glyphMove(17, 7), glyphLine(7, 17),
				glyphMove(8, 7), glyphLine(17, 7), glyphLine(17, 16),
			},
		},
		ActionText: {
			source: "letter-t.svg",
			commands: []toolbarGlyphCommand{
				glyphMove(6, 4), glyphLine(18, 4),
				glyphMove(12, 4), glyphLine(12, 20),
			},
		},
		ActionColor: {
			source: "palette.svg",
			commands: appendGlyph(
				[]toolbarGlyphCommand{
					glyphMove(12, 21), glyphCubic(7.03, 21, 3, 16.97, 3, 12),
					glyphCubic(3, 7.03, 7.03, 3, 12, 3), glyphCubic(16.97, 3, 21, 6.582, 21, 11),
					glyphCubic(21, 12.06, 20.526, 13.078, 19.682, 13.828),
					glyphCubic(18.838, 14.578, 17.693, 15, 16.5, 15), glyphLine(14, 15),
					glyphCubic(12.895, 15, 12, 15.895, 12, 17),
					glyphCubic(12, 17.72, 12.386, 18.386, 13, 18.75),
					glyphCubic(13.9, 19.283, 13.3, 21, 12, 21),
				},
				glyphCircle(8.5, 10.5, 1), glyphCircle(12.5, 7.5, 1), glyphCircle(16.5, 10.5, 1),
			),
		},
		ActionWidth: {
			source: "line.svg",
			commands: appendGlyph(
				glyphCircle(6, 18, 2), glyphCircle(18, 6, 2),
				[]toolbarGlyphCommand{glyphMove(7.5, 16.5), glyphLine(16.5, 7.5)},
			),
		},
	}
}

func toolbarGlyphForAction(action Action) (toolbarGlyph, bool) {
	glyph, ok := toolbarGlyphs[action]
	return glyph, ok
}

func toolbarIconBaseColor(enabled bool) color.NRGBA {
	if !enabled {
		return color.NRGBA{R: 112, G: 116, B: 122, A: 255}
	}
	return color.NRGBA{R: 235, G: 238, B: 242, A: 255}
}

func toolbarIconValueColor(selected color.NRGBA, enabled bool) color.NRGBA {
	if !enabled {
		return toolbarIconBaseColor(false)
	}
	return color.NRGBA{R: selected.R, G: selected.G, B: selected.B, A: 255}
}

func toolbarIconStrokeWidth(dpi int) float32 {
	if dpi <= 0 {
		dpi = 96
	}
	return float32(2*dpi) / 96
}

func toolbarValueStrokeWidth(width float64, dpi int) float32 {
	if dpi <= 0 {
		dpi = 96
	}
	result := float32(width) * float32(dpi) / 96
	if result < 1 {
		return 1
	}
	return result
}
