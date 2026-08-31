//go:build !windows

package selector

import "image"

func ShowMouseShield(image.Rectangle) (*MouseShield, error) {
	return nil, nil
}
