package app

import (
	"fmt"
	"image"

	"screenshot-win"
)

type longCaptureFrameResult struct {
	matched         bool
	offset          int
	position        int
	bestScore       float64
	secondBestScore float64
	reason          screenshotwin.RejectionReason
	addedTop        int
	addedBottom     int
	relocalized     bool
}

type longCaptureEngine interface {
	Add(image.Image) (longCaptureFrameResult, error)
	Image() *image.RGBA
	Finish() *image.RGBA
}

func newLongCaptureEngine(implementation LongCaptureImplementation, first image.Image, options screenshotwin.MatchOptions) (longCaptureEngine, error) {
	switch implementation {
	case LongCaptureBidirectional:
		stitcher, err := screenshotwin.NewBidirectionalStitcher(first, options)
		if err != nil {
			return nil, err
		}
		return &bidirectionalLongCaptureEngine{stitcher: stitcher}, nil
	case LongCaptureLegacy:
		matcher, err := screenshotwin.NewMatcher(first, options)
		if err != nil {
			return nil, err
		}
		builder, err := screenshotwin.NewBuilder(first)
		if err != nil {
			return nil, err
		}
		return &legacyLongCaptureEngine{matcher: matcher, builder: builder}, nil
	default:
		return nil, fmt.Errorf("unknown long capture implementation %d", implementation)
	}
}

type legacyLongCaptureEngine struct {
	matcher  *screenshotwin.Matcher
	builder  *screenshotwin.Builder
	position int
}

func (engine *legacyLongCaptureEngine) Add(current image.Image) (longCaptureFrameResult, error) {
	result, err := engine.matcher.Analyze(current)
	frame := longCaptureFrameResult{
		matched:         result.Matched,
		offset:          result.Offset,
		position:        engine.position,
		bestScore:       result.BestScore,
		secondBestScore: result.SecondBestScore,
		reason:          result.Reason,
	}
	if err != nil || !result.Matched {
		return frame, err
	}
	if err := engine.builder.Append(current, result.Offset); err != nil {
		return longCaptureFrameResult{}, err
	}
	engine.position += result.Offset
	frame.position = engine.position
	frame.addedBottom = result.Offset
	return frame, nil
}

func (engine *legacyLongCaptureEngine) Image() *image.RGBA  { return engine.builder.Image() }
func (engine *legacyLongCaptureEngine) Finish() *image.RGBA { return engine.builder.Finish() }

type bidirectionalLongCaptureEngine struct {
	stitcher *screenshotwin.BidirectionalStitcher
}

func (engine *bidirectionalLongCaptureEngine) Add(current image.Image) (longCaptureFrameResult, error) {
	result, err := engine.stitcher.Add(current)
	return longCaptureFrameResult{
		matched:         result.Matched,
		offset:          result.Delta,
		position:        result.Position,
		bestScore:       result.BestScore,
		secondBestScore: result.SecondBestScore,
		reason:          result.Reason,
		addedTop:        result.AddedTop,
		addedBottom:     result.AddedBottom,
		relocalized:     result.Relocalized,
	}, err
}

func (engine *bidirectionalLongCaptureEngine) Image() *image.RGBA  { return engine.stitcher.Image() }
func (engine *bidirectionalLongCaptureEngine) Finish() *image.RGBA { return engine.stitcher.Finish() }
