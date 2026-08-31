package selector

import (
	"context"
	"fmt"
	"image"
	"sync"

	"screenshot-win/editor"
)

// Frozen is the captured desktop overlay used by the inline annotation flow.
// It owns one non-destructive document displayed inside the original region.
type Frozen struct {
	once        sync.Once
	window      uintptr
	closeWindow func()
	done        <-chan struct{}
	annotate    func(context.Context, editor.Tool, editor.Style) error
	updateStyle func(context.Context, editor.StyleChange) (bool, error)
	styles      <-chan editor.Style
	rendered    func() image.Image
}

func (frozen *Frozen) WindowHandle() uintptr {
	if frozen == nil {
		return 0
	}
	return frozen.window
}
func (frozen *Frozen) Rendered() image.Image {
	if frozen == nil || frozen.rendered == nil {
		return nil
	}
	return frozen.rendered()
}
func (frozen *Frozen) AnnotateContext(ctx context.Context, tool editor.Tool, style editor.Style) error {
	if frozen == nil || frozen.annotate == nil {
		return fmt.Errorf("frozen overlay cannot annotate")
	}
	return frozen.annotate(ctx, tool, style)
}

// SelectedStyles reports the full style whenever an annotation becomes
// selected or its style changes. Deselection intentionally retains the last
// style as the default for the next annotation.
func (frozen *Frozen) SelectedStyles() <-chan editor.Style {
	if frozen == nil {
		return nil
	}
	return frozen.styles
}

// UpdateSelectedStyleContext applies one color or width choice to the current
// selection. It returns false when no annotation is selected.
func (frozen *Frozen) UpdateSelectedStyleContext(ctx context.Context, change editor.StyleChange) (bool, error) {
	if frozen == nil || frozen.updateStyle == nil {
		return false, fmt.Errorf("frozen overlay cannot update annotation style")
	}
	return frozen.updateStyle(ctx, change)
}
func (frozen *Frozen) Close() {
	if frozen == nil {
		return
	}
	frozen.once.Do(func() {
		if frozen.closeWindow != nil {
			frozen.closeWindow()
		}
		if frozen.done != nil {
			<-frozen.done
		}
	})
}

func copyImageToBGRA(pixels []byte, width, height int, source image.Image) error {
	if source == nil || source.Bounds().Dx() != width || source.Bounds().Dy() != height {
		return fmt.Errorf("source image size must be %dx%d", width, height)
	}
	if len(pixels) != width*height*4 {
		return fmt.Errorf("pixel buffer size is %d, want %d", len(pixels), width*height*4)
	}
	bounds := source.Bounds()
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			red, green, blue, _ := source.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			index := (y*width + x) * 4
			pixels[index] = byte(blue >> 8)
			pixels[index+1] = byte(green >> 8)
			pixels[index+2] = byte(red >> 8)
			pixels[index+3] = 0xff
		}
	}
	return nil
}
