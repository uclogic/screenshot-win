//go:build !windows

package selector

import (
	"context"
	"errors"
	"image"
)

var errUnsupported = errors.New("region selection is currently supported only on Windows")

func Select() (image.Rectangle, bool, error) {
	return image.Rectangle{}, false, errUnsupported
}

func SelectContext(context.Context) (image.Rectangle, bool, error) {
	return image.Rectangle{}, false, errUnsupported
}

func DesktopBounds() image.Rectangle { return image.Rectangle{} }
