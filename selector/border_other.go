//go:build !windows

package selector

import "image"

func ShowBorder(region image.Rectangle) (*Border, error) {
	return nil, errUnsupported
}
