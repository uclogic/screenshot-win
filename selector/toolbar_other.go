//go:build !windows

package selector

import (
	"context"
	"image"
)

func ShowToolbar(image.Rectangle) (Action, error) {
	return ActionCancel, errUnsupported
}

func ShowToolbarContext(context.Context, image.Rectangle, uintptr, ...ToolbarBackground) (*ActionToolbar, error) {
	return nil, errUnsupported
}

func ShowAnnotationToolbarContext(context.Context, image.Rectangle, uintptr, ...ToolbarBackground) (*ActionToolbar, error) {
	return nil, errUnsupported
}

func ShowCaptureToolbar(image.Rectangle) (*CaptureToolbar, error) {
	return nil, errUnsupported
}

func ShowToolbarContextWithLayout(context.Context, image.Rectangle, uintptr, []string, ...ToolbarBackground) (*ActionToolbar, error) {
	return nil, errUnsupported
}
func ShowAnnotationToolbarContextWithLayout(context.Context, image.Rectangle, uintptr, []string, ...ToolbarBackground) (*ActionToolbar, error) {
	return nil, errUnsupported
}
func ShowCaptureToolbarWithLayout(image.Rectangle, []string) (*CaptureToolbar, error) {
	return nil, errUnsupported
}
