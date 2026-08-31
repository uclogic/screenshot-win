//go:build !windows

package capture

import "image"

func CopyImage(image.Image) error { return errUnsupported }
