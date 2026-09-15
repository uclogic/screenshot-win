//go:build windows

package editor

import "testing"

func TestWindowsTextRetainsAntialiasing(t *testing.T) {
	for _, text := range []string{"Hello World", "中文标注"} {
		mask := rasterizeText(text, 3)
		if mask == nil {
			t.Fatalf("rasterizeText(%q) failed", text)
		}
		partial := false
		for _, coverage := range mask.pixels {
			if coverage > 0 && coverage < 255 {
				partial = true
				break
			}
		}
		if !partial {
			t.Errorf("%q has no antialiased edge pixels", text)
		}
	}
}
