package screenshotwin

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
)

var ErrNoReliableOverlap = errors.New("no reliable overlap between frames")

// Builder incrementally assembles a long screenshot in a geometrically
// growing RGBA buffer. It avoids copying the full output on every append.
type Builder struct {
	pixels   []uint8
	width    int
	height   int
	capacity int
	stride   int
	finished bool
}

// NewBuilder initializes a growing long-image buffer with first.
func NewBuilder(first image.Image) (*Builder, error) {
	if first == nil || first.Bounds().Dx() <= 0 || first.Bounds().Dy() <= 0 {
		return nil, errors.New("first image must not be empty")
	}
	bounds := first.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	stride := width * 4
	pixels := make([]uint8, stride*height)
	destination := &image.RGBA{
		Pix:    pixels,
		Stride: stride,
		Rect:   image.Rect(0, 0, width, height),
	}
	draw.Draw(destination, destination.Bounds(), first, bounds.Min, draw.Src)
	return &Builder{
		pixels:   pixels,
		width:    width,
		height:   height,
		capacity: height,
		stride:   stride,
	}, nil
}

// Height returns the number of completed rows currently in the output.
func (builder *Builder) Height() int {
	if builder == nil {
		return 0
	}
	return builder.height
}

// Image returns a read-only view of the rows assembled so far. The view is
// valid only until the next call to Append or Finish; callers must not retain
// or modify it.
func (builder *Builder) Image() *image.RGBA {
	if builder == nil || builder.width <= 0 || builder.height <= 0 || builder.stride <= 0 {
		return nil
	}
	return &image.RGBA{
		Pix:    builder.pixels[:builder.stride*builder.height],
		Stride: builder.stride,
		Rect:   image.Rect(0, 0, builder.width, builder.height),
	}
}

// Append adds the newly revealed bottom offset rows from current.
func (builder *Builder) Append(current image.Image, offset int) error {
	if builder == nil {
		return errors.New("builder must not be nil")
	}
	if builder.finished {
		return errors.New("builder is already finished")
	}
	if builder.width <= 0 || builder.height <= 0 || builder.capacity <= 0 {
		return errors.New("builder is not initialized")
	}
	if current == nil {
		return errors.New("image must not be nil")
	}
	currentBounds := current.Bounds()
	if builder.width != currentBounds.Dx() {
		return errors.New("images must have equal widths")
	}
	if offset <= 0 || offset > currentBounds.Dy() {
		return fmt.Errorf("offset %d is outside frame height %d", offset, currentBounds.Dy())
	}

	requiredHeight := builder.height + offset
	if requiredHeight > builder.capacity {
		capacity := builder.capacity
		for capacity < requiredHeight {
			capacity *= 2
		}
		pixels := make([]uint8, builder.stride*capacity)
		copy(pixels, builder.pixels[:builder.stride*builder.height])
		builder.pixels = pixels
		builder.capacity = capacity
	}

	destination := &image.RGBA{
		Pix:    builder.pixels,
		Stride: builder.stride,
		Rect:   image.Rect(0, 0, builder.width, requiredHeight),
	}
	destinationRect := image.Rect(0, builder.height, builder.width, requiredHeight)
	sourceStart := image.Pt(currentBounds.Min.X, currentBounds.Max.Y-offset)
	draw.Draw(destination, destinationRect, current, sourceStart, draw.Src)
	builder.height = requiredHeight
	return nil
}

// Finish returns the completed long screenshot without copying its pixels.
// The builder cannot be appended to after Finish is called.
func (builder *Builder) Finish() *image.RGBA {
	if builder == nil {
		return nil
	}
	builder.finished = true
	return builder.Image()
}

// AppendFrame returns output with the newly revealed bottom offset rows from
// current appended. Inputs are not modified.
func AppendFrame(output, current image.Image, offset int) (*image.RGBA, error) {
	if output == nil || current == nil {
		return nil, errors.New("images must not be nil")
	}
	outBounds, currentBounds := output.Bounds(), current.Bounds()
	if outBounds.Dx() != currentBounds.Dx() {
		return nil, errors.New("images must have equal widths")
	}
	if offset <= 0 || offset > currentBounds.Dy() {
		return nil, fmt.Errorf("offset %d is outside frame height %d", offset, currentBounds.Dy())
	}

	result := image.NewRGBA(image.Rect(0, 0, outBounds.Dx(), outBounds.Dy()+offset))
	draw.Draw(result, image.Rect(0, 0, outBounds.Dx(), outBounds.Dy()), output, outBounds.Min, draw.Src)
	sourceStartY := currentBounds.Max.Y - offset
	destination := image.Rect(0, outBounds.Dy(), currentBounds.Dx(), outBounds.Dy()+offset)
	draw.Draw(result, destination, current, image.Pt(currentBounds.Min.X, sourceStartY), draw.Src)
	return result, nil
}

// Stitch combines an ordered sequence of equally sized viewport captures.
func Stitch(frames []image.Image) (*image.RGBA, error) {
	if len(frames) == 0 || frames[0] == nil {
		return nil, errors.New("at least one frame is required")
	}
	matcher, err := NewMatcher(frames[0], DefaultMatchOptions())
	if err != nil {
		return nil, err
	}
	builder, err := NewBuilder(frames[0])
	if err != nil {
		return nil, err
	}
	for index, current := range frames[1:] {
		match, analyzeErr := matcher.Analyze(current)
		if analyzeErr != nil {
			return nil, fmt.Errorf("frame %d: %w", index+1, analyzeErr)
		}
		if !match.Matched {
			return nil, fmt.Errorf("frame %d: %w", index+1, ErrNoReliableOverlap)
		}
		if appendErr := builder.Append(current, match.Offset); appendErr != nil {
			return nil, fmt.Errorf("frame %d: %w", index+1, appendErr)
		}
	}
	return builder.Finish(), nil
}
