//go:build !windows

package selector

import "image"

func ShowPreview(image.Rectangle, image.Image) (*Preview, error) {
	return nil, errUnsupported
}
