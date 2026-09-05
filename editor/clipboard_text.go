package editor

import (
	"fmt"
	"strings"
	"unicode"
)

// wrapClipboardText preserves explicit newlines and wraps even long unbroken
// words. Measurement is supplied by the actual platform font.
func wrapClipboardText(text string, maxWidth, maxLines int, measure func(string) (int, error)) ([]string, error) {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	text = strings.ReplaceAll(text, "\t", "    ")
	var lines []string
	appendLine := func(line string) error {
		if len(lines) >= maxLines {
			return fmt.Errorf("文字过长，无法生成贴图")
		}
		lines = append(lines, line)
		return nil
	}
	for _, paragraph := range strings.Split(text, "\n") {
		runes := []rune(paragraph)
		if len(runes) == 0 {
			if err := appendLine(""); err != nil {
				return nil, err
			}
			continue
		}
		for len(runes) > 0 {
			low, high := 1, len(runes)
			// Exponential probing keeps measurements bounded for very long paragraphs.
			for probe := 1; probe < high; probe *= 2 {
				width, err := measure(string(runes[:probe]))
				if err != nil {
					return nil, err
				}
				if width > maxWidth {
					high = probe
					break
				}
				low = probe
			}
			best := 0
			for low <= high {
				mid := (low + high) / 2
				width, err := measure(string(runes[:mid]))
				if err != nil {
					return nil, err
				}
				if width <= maxWidth {
					best = mid
					low = mid + 1
				} else {
					high = mid - 1
				}
			}
			if best == 0 {
				return nil, fmt.Errorf("文字字符超出贴图宽度")
			}
			split := best
			if best < len(runes) {
				for i := best - 1; i >= 0; i-- {
					if unicode.IsSpace(runes[i]) {
						split = i + 1
						break
					}
				}
			}
			if err := appendLine(string(runes[:split])); err != nil {
				return nil, err
			}
			runes = runes[split:]
		}
	}
	return lines, nil
}
