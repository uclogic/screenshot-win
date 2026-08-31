package selector

import (
	"errors"
	"image"
	"sync"
	"sync/atomic"
)

const (
	previewWidthDIP       = 120
	previewGapDIP         = 8
	previewInsideDIP      = 8
	previewMaxHeightRatio = 35
)

type previewPlacement uint8

const (
	previewRight previewPlacement = iota
	previewLeft
	previewInside
)

// Preview is a live, click-through view of an in-progress long screenshot.
type Preview struct {
	once                sync.Once
	closed              atomic.Bool
	updateImage         func(image.Image) error
	closeWindow         func()
	hideForCapture      func()
	restoreAfterCapture func()
	done                <-chan struct{}
}

// Update synchronously redraws the preview before returning.
func (preview *Preview) Update(source image.Image) error {
	if preview == nil {
		return nil
	}
	if preview.closed.Load() {
		return errors.New("preview is closed")
	}
	if source == nil || source.Bounds().Empty() {
		return errors.New("preview image must not be empty")
	}
	if preview.updateImage == nil {
		return errors.New("preview cannot be updated")
	}
	return preview.updateImage(source)
}

// Close removes the preview and waits for its window resources to be released.
func (preview *Preview) Close() {
	if preview == nil {
		return
	}
	preview.once.Do(func() {
		preview.closed.Store(true)
		if preview.closeWindow != nil {
			preview.closeWindow()
		}
		if preview.done != nil {
			<-preview.done
		}
	})
}

// HideForCapture temporarily removes a preview that Windows could not exclude
// from capture. It is a no-op when display affinity is active or the preview
// does not overlap the captured region.
func (preview *Preview) HideForCapture() {
	if preview != nil && !preview.closed.Load() && preview.hideForCapture != nil {
		preview.hideForCapture()
	}
}

// RestoreAfterCapture shows a preview hidden by HideForCapture.
func (preview *Preview) RestoreAfterCapture() {
	if preview != nil && !preview.closed.Load() && preview.restoreAfterCapture != nil {
		preview.restoreAfterCapture()
	}
}

func previewWindowBounds(region, workArea image.Rectangle, size image.Point, gap, insidePadding int) (image.Rectangle, previewPlacement, bool) {
	if region.Empty() || workArea.Empty() || size.X <= 0 || size.Y <= 0 {
		return image.Rectangle{}, previewRight, false
	}
	size.X = min(size.X, workArea.Dx())
	size.Y = min(size.Y, workArea.Dy())
	y := clampCoordinate(region.Min.Y, workArea.Min.Y, workArea.Max.Y-size.Y)
	if x := region.Max.X + gap; x+size.X <= workArea.Max.X {
		return image.Rect(x, y, x+size.X, y+size.Y), previewRight, true
	}
	if x := region.Min.X - gap - size.X; x >= workArea.Min.X {
		return image.Rect(x, y, x+size.X, y+size.Y), previewLeft, true
	}
	insideWidth := region.Dx() - insidePadding*2
	insideHeight := region.Dy() - insidePadding*2
	if insideWidth < size.X || insideHeight <= 0 {
		return image.Rectangle{}, previewInside, false
	}
	size.Y = min(size.Y, insideHeight)
	x := region.Min.X + (region.Dx()-size.X)/2
	y = region.Min.Y + insidePadding
	bounds := image.Rect(x, y, x+size.X, y+size.Y)
	if !bounds.In(workArea) {
		return image.Rectangle{}, previewInside, false
	}
	return bounds, previewInside, true
}
