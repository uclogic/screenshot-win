package app

import (
	"context"
	"image"

	"screenshot-win/capture"
	"screenshot-win/editor"
)

// PinClipboard creates an independent desktop pin from a clipboard snapshot.
func (runner *Runner) PinClipboard(ctx context.Context, origin image.Point) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return pinClipboard(ctx, origin, capture.ReadClipboard, editor.RenderClipboardText, runner.pinImage)
}

func pinClipboard(ctx context.Context, origin image.Point, read func() (capture.ClipboardContent, error), render func(string) (image.Image, error), pin func(image.Image, image.Point) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	content, err := read()
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	source := content.Image
	if source == nil {
		source, err = render(content.Text)
		if err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return pin(source, origin)
}
