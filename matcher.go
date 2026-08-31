package screenshotwin

import (
	"fmt"
	"image"
	"math"
)

const (
	coarseScale   = 4
	minimumOffset = 3
)

// RejectionReason explains why two frames were not matched.
type RejectionReason string

const (
	RejectionNone          RejectionReason = ""
	RejectionEmptyFrame    RejectionReason = "empty_frame"
	RejectionSizeMismatch  RejectionReason = "size_mismatch"
	RejectionFrameTooShort RejectionReason = "frame_too_short"
	RejectionStationary    RejectionReason = "stationary"
	RejectionScoreTooHigh  RejectionReason = "score_too_high"
	RejectionAmbiguous     RejectionReason = "ambiguous"
)

// MatchOptions controls vertical scroll matching. Use DefaultMatchOptions as
// the starting point and override only values that need tuning.
type MatchOptions struct {
	MaxOffsetRatio       float64
	MaxMeanDifference    float64
	MinimumConfidence    float64
	StationaryDifference float64
}

// DefaultMatchOptions returns the settings used by FindScrollOffset.
func DefaultMatchOptions() MatchOptions {
	return MatchOptions{
		MaxOffsetRatio:       0.5,
		MaxMeanDifference:    8.0,
		MinimumConfidence:    0.25,
		StationaryDifference: 0.5,
	}
}

// Validate checks whether the matcher settings are meaningful.
func (options MatchOptions) Validate() error {
	if !finite(options.MaxOffsetRatio) || options.MaxOffsetRatio <= 0 || options.MaxOffsetRatio >= 1 {
		return fmt.Errorf("max offset ratio must be greater than 0 and less than 1")
	}
	if !finite(options.MaxMeanDifference) || options.MaxMeanDifference < 0 || options.MaxMeanDifference > 255 {
		return fmt.Errorf("max mean difference must be between 0 and 255")
	}
	if !finite(options.MinimumConfidence) || options.MinimumConfidence < 0 || options.MinimumConfidence > 256 {
		return fmt.Errorf("minimum confidence must be between 0 and 256")
	}
	if !finite(options.StationaryDifference) || options.StationaryDifference < 0 || options.StationaryDifference > 255 {
		return fmt.Errorf("stationary difference must be between 0 and 255")
	}
	return nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

// MatchResult contains the selected offset, scores, and rejection reason.
// Reason is empty when Matched is true.
type MatchResult struct {
	Offset          int
	Matched         bool
	BestScore       float64
	SecondBestScore float64
	Reason          RejectionReason
}

// Matcher analyzes a sequence of viewport captures while caching the
// grayscale pixels of the last successfully matched frame.
type Matcher struct {
	previous []uint8
	width    int
	height   int
	options  MatchOptions
}

// NewMatcher creates a stateful matcher whose baseline is first. Rejected
// frames do not replace the baseline, so a later frame can still recover.
func NewMatcher(first image.Image, options MatchOptions) (*Matcher, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	pixels, width, height := grayscale(first)
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("first frame must not be empty")
	}
	return &Matcher{
		previous: pixels,
		width:    width,
		height:   height,
		options:  options,
	}, nil
}

// Analyze compares current with the last successfully matched frame. The
// cached baseline advances only when the result is a reliable match.
func (matcher *Matcher) Analyze(current image.Image) (MatchResult, error) {
	if matcher == nil {
		return MatchResult{}, fmt.Errorf("matcher must not be nil")
	}
	curr, width, height := grayscale(current)
	result := analyzeGrayscale(
		matcher.previous, matcher.width, matcher.height,
		curr, width, height,
		matcher.options,
	)
	if result.Matched {
		matcher.previous = curr
		matcher.width = width
		matcher.height = height
	}
	return result, nil
}

// FindScrollOffset returns how many pixels the content in current moved up
// relative to previous. It preserves the original API and default behavior.
func FindScrollOffset(previous, current image.Image) (int, bool) {
	result, err := AnalyzeScroll(previous, current, DefaultMatchOptions())
	if err != nil {
		return 0, false
	}
	return result.Offset, result.Matched
}

// AnalyzeScroll evaluates two frames using options and returns scoring details
// even when the frames cannot be matched reliably.
func AnalyzeScroll(previous, current image.Image, options MatchOptions) (MatchResult, error) {
	if err := options.Validate(); err != nil {
		return MatchResult{}, err
	}

	prev, width, height := grayscale(previous)
	curr, currentWidth, currentHeight := grayscale(current)
	return analyzeGrayscale(prev, width, height, curr, currentWidth, currentHeight, options), nil
}

func analyzeGrayscale(prev []uint8, width, height int, curr []uint8, currentWidth, currentHeight int, options MatchOptions) MatchResult {
	if width <= 0 || height <= 0 || currentWidth <= 0 || currentHeight <= 0 {
		return rejected(RejectionEmptyFrame, 256, 256)
	}
	if width != currentWidth || height != currentHeight {
		return rejected(RejectionSizeMismatch, 256, 256)
	}

	maxOffset := int(float64(height) * options.MaxOffsetRatio)
	if maxOffset >= height {
		maxOffset = height - 1
	}
	if maxOffset < minimumOffset {
		return rejected(RejectionFrameTooShort, 256, 256)
	}

	stationaryScore := overlapScore(prev, curr, width, height, 0, coarseScale)
	if stationaryScore <= options.StationaryDifference {
		return rejected(RejectionStationary, stationaryScore, 256)
	}

	bestCoarseOffset := 0
	bestCoarseScore := 256.0
	for offset := minimumOffset; offset <= maxOffset; offset++ {
		score := overlapScore(prev, curr, width, height, offset, coarseScale)
		if score < bestCoarseScore {
			bestCoarseScore = score
			bestCoarseOffset = offset
		}
	}

	start := bestCoarseOffset - 2
	if start < minimumOffset {
		start = minimumOffset
	}
	end := bestCoarseOffset + 2
	if end > maxOffset {
		end = maxOffset
	}

	bestOffset := 0
	bestScore := 256.0
	secondScore := 256.0
	for offset := start; offset <= end; offset++ {
		score := overlapScore(prev, curr, width, height, offset, 2)
		if score < bestScore {
			secondScore = bestScore
			bestScore = score
			bestOffset = offset
		} else if score < secondScore {
			secondScore = score
		}
	}

	if bestScore > options.MaxMeanDifference {
		return rejectedWithOffset(RejectionScoreTooHigh, bestOffset, bestScore, secondScore)
	}
	if bestScore > 0.05 && secondScore-bestScore < options.MinimumConfidence {
		return rejectedWithOffset(RejectionAmbiguous, bestOffset, bestScore, secondScore)
	}
	return MatchResult{
		Offset:          bestOffset,
		Matched:         true,
		BestScore:       bestScore,
		SecondBestScore: secondScore,
	}
}

func rejected(reason RejectionReason, bestScore, secondScore float64) MatchResult {
	return rejectedWithOffset(reason, 0, bestScore, secondScore)
}

func rejectedWithOffset(reason RejectionReason, offset int, bestScore, secondScore float64) MatchResult {
	return MatchResult{
		Offset:          offset,
		BestScore:       bestScore,
		SecondBestScore: secondScore,
		Reason:          reason,
	}
}

func grayscale(source image.Image) ([]uint8, int, int) {
	if source == nil {
		return nil, 0, 0
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, width, height
	}
	pixels := make([]uint8, width*height)
	if rgba, ok := source.(*image.RGBA); ok {
		for y := 0; y < height; y++ {
			sourceOffset := rgba.PixOffset(bounds.Min.X, bounds.Min.Y+y)
			destinationOffset := y * width
			for x := 0; x < width; x++ {
				pixelOffset := sourceOffset + x*4
				r := uint32(rgba.Pix[pixelOffset])
				g := uint32(rgba.Pix[pixelOffset+1])
				b := uint32(rgba.Pix[pixelOffset+2])
				pixels[destinationOffset+x] = uint8((257 * (299*r + 587*g + 114*b) / 1000) >> 8)
			}
		}
		return pixels, width, height
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, _ := source.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			pixels[y*width+x] = uint8((299*r + 587*g + 114*b) / 1000 >> 8)
		}
	}
	return pixels, width, height
}

func overlapScore(previous, current []uint8, width, height, offset, step int) float64 {
	var difference uint64
	var count uint64
	for y := 0; y < height-offset; y += step {
		previousRow := (y + offset) * width
		currentRow := y * width
		for x := 0; x < width; x += step {
			a := int(previous[previousRow+x])
			b := int(current[currentRow+x])
			if a > b {
				difference += uint64(a - b)
			} else {
				difference += uint64(b - a)
			}
			count++
		}
	}
	if count == 0 {
		return 256
	}
	return float64(difference) / float64(count)
}
