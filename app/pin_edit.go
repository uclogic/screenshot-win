package app

import (
	"context"
	"fmt"
	"image"
	"math"

	"screenshot-win/selector"
)

// ConfigurePinEditing shares the host's busy/shutdown boundary and reads the
// current toolbar layout when editing starts. Configure before creating pins.
func (runner *Runner) ConfigurePinEditing(launch func(func(context.Context) error) bool, layout func() []string) {
	runner.pins.SetEditor(func(ctx context.Context, source image.Image, bounds image.Rectangle) (image.Image, error) {
		return dispatchPinEdit(ctx, launch, func(editCtx context.Context) (image.Image, error) {
			var items []string
			if layout != nil {
				items = layout()
			}
			return runner.editPinnedImage(editCtx, source, bounds, items)
		})
	})
}

func (runner *Runner) editPinnedImage(ctx context.Context, source image.Image, bounds image.Rectangle, layout []string) (image.Image, error) {
	return runPinEdit(ctx, runner.coordinator, source, bounds, func(ctx context.Context) (selector.Action, image.Image, error) {
		size := source.Bounds().Size()
		scale := math.Min(float64(bounds.Dx())/float64(size.X), float64(bounds.Dy())/float64(size.Y))
		// Pin finishes this edit without spawning a second window. All successful
		// completion actions return the rendered image to the original pin.
		result, output, err := runner.editInlineAnnotation(ctx, source, bounds, layout, func(image.Image, image.Point) error { return nil }, scale)
		return result.action, output, err
	})
}

func runPinEdit(ctx context.Context, coordinator *App, source image.Image, bounds image.Rectangle, edit func(context.Context) (selector.Action, image.Image, error)) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if source == nil || source.Bounds().Empty() || bounds.Empty() {
		return nil, fmt.Errorf("pinned image and display bounds must not be empty")
	}
	session, err := coordinator.Begin(StateEditing)
	if err != nil {
		return nil, err
	}
	defer session.Finish()
	action, output, err := edit(ctx)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if action != selector.ActionPin && action != selector.ActionSave && action != selector.ActionCopy {
		return nil, nil
	}
	if output == nil || output.Bounds().Size() != source.Bounds().Size() {
		return nil, fmt.Errorf("editing must preserve the pinned image's original resolution")
	}
	return output, nil
}

func dispatchPinEdit(ctx context.Context, launch func(func(context.Context) error) bool, edit func(context.Context) (image.Image, error)) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if launch == nil {
		return edit(ctx)
	}
	type result struct {
		image image.Image
		err   error
	}
	finished := make(chan result, 1)
	if !launch(func(hostCtx context.Context) error {
		editCtx, cancel := context.WithCancel(hostCtx)
		stop := context.AfterFunc(ctx, cancel)
		defer stop()
		defer cancel()
		if ctx.Err() != nil {
			cancel()
		}
		output, err := edit(editCtx)
		finished <- result{image: output, err: err}
		// The pin restores itself and reports errors on its own UI thread.
		return nil
	}) {
		return nil, nil // Another capture/edit or shutdown owns the host.
	}
	// Wait for resource cleanup even if the pin closes during editing.
	resultValue := <-finished
	return resultValue.image, resultValue.err
}
