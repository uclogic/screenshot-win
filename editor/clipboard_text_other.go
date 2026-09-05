//go:build !windows

package editor

import (
	"fmt"
	"image"
)

func RenderClipboardText(string) (image.Image, error) {
	return nil, fmt.Errorf("clipboard text rendering is supported only on Windows")
}
