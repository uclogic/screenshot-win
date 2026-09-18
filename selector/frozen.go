package selector

import (
	"context"
	"fmt"
	"image"
	"image/color"
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
	capture     func() (image.Rectangle, image.Image)
	regions     <-chan image.Rectangle
	background  ToolbarBackground
}

// Background copies the visible frozen surface on its window thread. It does
// not include toolbars and never exposes the editable surface's pixel storage.
func (frozen *Frozen) Background(bounds image.Rectangle) image.Image {
	if frozen == nil || frozen.background == nil {
		return nil
	}
	return frozen.background(bounds)
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

// Capture returns the current virtual-desktop region and the edited image
// cropped to that region. The returned image has zero-based bounds.
func (frozen *Frozen) Capture() (image.Rectangle, image.Image) {
	if frozen == nil {
		return image.Rectangle{}, nil
	}
	if frozen.capture != nil {
		return frozen.capture()
	}
	return image.Rectangle{}, frozen.Rendered()
}

// RegionChanges reports committed and in-progress capture-region changes.
// The channel is closed with the frozen overlay.
func (frozen *Frozen) RegionChanges() <-chan image.Rectangle {
	if frozen == nil {
		return nil
	}
	return frozen.regions
}
func (frozen *Frozen) AnnotateContext(ctx context.Context, tool editor.Tool, style editor.Style) error {
	if frozen == nil || frozen.annotate == nil {
		return fmt.Errorf("frozen overlay cannot annotate")
	}
	return frozen.annotate(ctx, tool, style)
}

type croppedImage struct {
	source image.Image
	area   image.Rectangle
}

func cropImage(source image.Image, area image.Rectangle) image.Image {
	if source == nil {
		return nil
	}
	area = area.Intersect(source.Bounds())
	if area.Empty() {
		return nil
	}
	return croppedImage{source: source, area: area}
}

func (cropped croppedImage) ColorModel() color.Model { return cropped.source.ColorModel() }
func (cropped croppedImage) Bounds() image.Rectangle {
	return image.Rectangle{Max: cropped.area.Size()}
}
func (cropped croppedImage) At(x, y int) color.Color {
	if !image.Pt(x, y).In(cropped.Bounds()) {
		return color.NRGBA{}
	}
	return cropped.source.At(cropped.area.Min.X+x, cropped.area.Min.Y+y)
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
	// Captures are RGBA. Read their rows directly to avoid an interface call
	// and a boxed color allocation for every pixel of the virtual desktop.
	if rgba, ok := source.(*image.RGBA); ok {
		for y := 0; y < height; y++ {
			row := rgba.Pix[y*rgba.Stride : y*rgba.Stride+width*4]
			dst := pixels[y*width*4 : (y+1)*width*4]
			for x := 0; x < len(row); x += 4 {
				dst[x], dst[x+1], dst[x+2], dst[x+3] = row[x+2], row[x+1], row[x], 255
			}
		}
		return nil
	}
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
