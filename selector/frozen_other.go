//go:build !windows

package selector

import (
	"image"
)

func ShowFrozenDesktop(image.Image, image.Rectangle) (*Frozen, error) {
	return nil, errUnsupported
}

func ShowFrozenContent(image.Image, image.Rectangle, image.Image) (*Frozen, error) {
	return nil, errUnsupported
}
