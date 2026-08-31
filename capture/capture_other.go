//go:build !windows

package capture

import (
	"errors"
	"image"
)

var errUnsupported = errors.New("desktop capture is currently supported only on Windows")

func Supported() bool { return false }

func Initialize() error { return errUnsupported }

func EscapePressed() bool { return false }

func Region(x, y, width, height int) (*image.RGBA, error) {
	return nil, errUnsupported
}
