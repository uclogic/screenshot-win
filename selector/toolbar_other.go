//go:build !windows

package selector

import (
	"context"
	"image"
)

func ShowToolbar(image.Rectangle) (Action, error) {
	return ActionCancel, errUnsupported
}

func ShowToolbarContext(context.Context, image.Rectangle, uintptr) (*ActionToolbar, error) {
	return nil, errUnsupported
}

func ShowAnnotationToolbarContext(context.Context, image.Rectangle, uintptr) (*ActionToolbar, error) {
	return nil, errUnsupported
}

func ShowCaptureToolbar(image.Rectangle) (*CaptureToolbar, error) {
	return nil, errUnsupported
}
