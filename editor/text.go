package editor

import "unicode/utf8"

type textMask struct {
	width, height int
	pixels        []byte
}

func (mask *textMask) at(x, y int) bool {
	return mask != nil && x >= 0 && y >= 0 && x < mask.width && y < mask.height && mask.pixels[y*mask.width+x] != 0
}

// textPixel is a compact deterministic bitmap renderer shared by preview and
// export. ASCII glyphs use a readable 5x7 font; other Unicode code points use
// a framed glyph with a distinguishing bit pattern instead of being dropped.
func textPixel(text string, x, y, scale int) bool {
	if x < 0 || y < 0 || scale < 1 {
		return false
	}
	cellW, cellH := 6*scale, 8*scale
	column, row := x/cellW, y/cellH
	if row > 0 {
		return false
	}
	runes := []rune(text)
	if column >= len(runes) {
		return false
	}
	gx, gy := (x%cellW)/scale, (y%cellH)/scale
	if gx >= 5 || gy >= 7 {
		return false
	}
	pattern := glyph(runes[column])
	return pattern[gy]&(1<<uint(4-gx)) != 0
}

func glyph(r rune) [7]byte {
	if r >= 'a' && r <= 'z' {
		r -= 'a' - 'A'
	}
	if rows, ok := glyphs[r]; ok {
		return rows
	}
	// A visible stable fallback for all valid Unicode characters.
	rows := [7]byte{31, 17, 17, 17, 17, 17, 31}
	if r != utf8.RuneError {
		rows[2] |= byte(r&7) << 1
		rows[4] |= byte((r>>3)&7) << 1
	}
	return rows
}

var glyphs = map[rune][7]byte{
	' ': {}, '0': {14, 17, 19, 21, 25, 17, 14}, '1': {4, 12, 4, 4, 4, 4, 14}, '2': {14, 17, 1, 2, 4, 8, 31},
	'3': {30, 1, 1, 14, 1, 1, 30}, '4': {2, 6, 10, 18, 31, 2, 2}, '5': {31, 16, 30, 1, 1, 17, 14},
	'6': {6, 8, 16, 30, 17, 17, 14}, '7': {31, 1, 2, 4, 8, 8, 8}, '8': {14, 17, 17, 14, 17, 17, 14},
	'9': {14, 17, 17, 15, 1, 2, 12}, 'A': {14, 17, 17, 31, 17, 17, 17}, 'B': {30, 17, 17, 30, 17, 17, 30},
	'C': {14, 17, 16, 16, 16, 17, 14}, 'D': {30, 17, 17, 17, 17, 17, 30}, 'E': {31, 16, 16, 30, 16, 16, 31},
	'F': {31, 16, 16, 30, 16, 16, 16}, 'G': {14, 17, 16, 23, 17, 17, 15}, 'H': {17, 17, 17, 31, 17, 17, 17},
	'I': {14, 4, 4, 4, 4, 4, 14}, 'J': {7, 2, 2, 2, 2, 18, 12}, 'K': {17, 18, 20, 24, 20, 18, 17},
	'L': {16, 16, 16, 16, 16, 16, 31}, 'M': {17, 27, 21, 21, 17, 17, 17}, 'N': {17, 25, 21, 19, 17, 17, 17},
	'O': {14, 17, 17, 17, 17, 17, 14}, 'P': {30, 17, 17, 30, 16, 16, 16}, 'Q': {14, 17, 17, 17, 21, 18, 13},
	'R': {30, 17, 17, 30, 20, 18, 17}, 'S': {15, 16, 16, 14, 1, 1, 30}, 'T': {31, 4, 4, 4, 4, 4, 4},
	'U': {17, 17, 17, 17, 17, 17, 14}, 'V': {17, 17, 17, 17, 17, 10, 4}, 'W': {17, 17, 17, 21, 21, 21, 10},
	'X': {17, 17, 10, 4, 10, 17, 17}, 'Y': {17, 17, 10, 4, 4, 4, 4}, 'Z': {31, 1, 2, 4, 8, 16, 31},
	'-': {0, 0, 0, 31, 0, 0, 0}, '_': {0, 0, 0, 0, 0, 0, 31}, '.': {0, 0, 0, 0, 0, 12, 12}, ':': {0, 12, 12, 0, 12, 12, 0},
}
