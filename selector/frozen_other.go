//go:build !windows

package selector

import (
	"image"
)

func ShowFrozenDesktop(image.Image, image.Rectangle) (*Frozen, error) {
	return nil, errUnsupported
}

func ShowFrozenContent(image.Image, image.Rectangle, image.Image, ...float64) (*Frozen, error) {
	return nil, errUnsupported
}
