//go:build !windows

package selector

import "image"

func showPinnedWindow(image.Image, image.Point, PinEditor) (*Pin, error) { return nil, errUnsupported }
