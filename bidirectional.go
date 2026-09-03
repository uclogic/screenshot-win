package screenshotwin

import (
	"errors"
	"image"
	"image/draw"
	"sort"
)

const (
	anchorStep       = 8
	anchorBandHeight = 4
	maxAnchorMatches = 64
	maxRelocateTests = 24
)

// BidirectionalResult describes the placement of one frame in a
// BidirectionalStitcher. Delta is the signed change in the viewport's page Y
// coordinate: positive values move down the page and negative values move up.
type BidirectionalResult struct {
	Delta           int
	Position        int
	Matched         bool
	BestScore       float64
	SecondBestScore float64
	Reason          RejectionReason
	AddedTop        int
	AddedBottom     int
	Relocalized     bool
}

// BidirectionalStitcher locates viewport captures on one vertical page and
// preserves the first pixels captured for every page row.
type BidirectionalStitcher struct {
	pixels       []uint8
	gray         []uint8
	width        int
	frameHeight  int
	stride       int
	capacity     int
	origin       int
	height       int
	minY         int
	maxY         int
	last         []uint8
	lastPosition int
	options      MatchOptions
	anchors      map[uint64][]int
	ambiguous    map[uint64]struct{}
	indexedRows  map[int]struct{}
	finished     bool
}

// NewBidirectionalStitcher initializes a bidirectional long screenshot with
// first at page position zero.
func NewBidirectionalStitcher(first image.Image, options MatchOptions) (*BidirectionalStitcher, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	gray, width, height := grayscale(first)
	if width <= 0 || height <= 0 {
		return nil, errors.New("first image must not be empty")
	}
	capacity := height * 2
	origin := (capacity - height) / 2
	stride := width * 4
	stitcher := &BidirectionalStitcher{
		pixels:      make([]uint8, stride*capacity),
		gray:        make([]uint8, width*capacity),
		width:       width,
		frameHeight: height,
		stride:      stride,
		capacity:    capacity,
		origin:      origin,
		height:      height,
		maxY:        height,
		last:        gray,
		options:     options,
		anchors:     make(map[uint64][]int),
		ambiguous:   make(map[uint64]struct{}),
		indexedRows: make(map[int]struct{}),
	}
	canvas := stitcher.Image()
	draw.Draw(canvas, canvas.Bounds(), first, first.Bounds().Min, draw.Src)
	copy(stitcher.gray[origin*width:(origin+height)*width], gray)
	stitcher.indexRows(0, height)
	return stitcher, nil
}

// Add locates current relative to the last accepted frame or the retained
// page anchors, then adds only rows that have not previously been captured.
func (stitcher *BidirectionalStitcher) Add(current image.Image) (BidirectionalResult, error) {
	if stitcher == nil {
		return BidirectionalResult{}, errors.New("bidirectional stitcher must not be nil")
	}
	if stitcher.finished {
		return BidirectionalResult{}, errors.New("bidirectional stitcher is already finished")
	}
	curr, width, height := grayscale(current)
	if width <= 0 || height <= 0 {
		return bidirectionalRejected(RejectionEmptyFrame, 256, 256), nil
	}
	if width != stitcher.width || height != stitcher.frameHeight {
		return bidirectionalRejected(RejectionSizeMismatch, 256, 256), nil
	}

	match := analyzeSignedGrayscale(stitcher.last, curr, width, height, stitcher.options)
	position := stitcher.lastPosition + match.Delta
	relocalized := false
	if match.Matched && stitcher.extensionRows(position) > 0 {
		// Before growing either edge, check whether this frame is actually a
		// revisit of rows already on the canvas. Low-texture or repeating pages
		// can produce an equally good signed offset in the wrong direction.
		// Prefer a fully captured location when its score is no worse.
		if existing := stitcher.relocate(curr); existing.Matched && stitcher.extensionRows(existing.Position) == 0 && existing.BestScore <= match.BestScore+stationaryScoreHysteresis {
			match = existing
			position = existing.Position
			relocalized = true
		}
	}
	if !match.Matched && match.Reason != RejectionStationary {
		match = stitcher.relocate(curr)
		position = match.Position
		relocalized = match.Matched
	}
	if !match.Matched {
		return match, nil
	}

	addedTop, addedBottom, err := stitcher.place(current, curr, position)
	if err != nil {
		return BidirectionalResult{}, err
	}
	match.Position = position
	match.Delta = position - stitcher.lastPosition
	match.AddedTop = addedTop
	match.AddedBottom = addedBottom
	match.Relocalized = relocalized
	stitcher.last = curr
	stitcher.lastPosition = position
	return match, nil
}

// Image returns the assembled page rows captured so far. The returned view is
// valid only until Add is called again and must not be modified.
func (stitcher *BidirectionalStitcher) Image() *image.RGBA {
	if stitcher == nil {
		return nil
	}
	if stitcher.width <= 0 || stitcher.height <= 0 || stitcher.origin < 0 || stitcher.origin+stitcher.height > stitcher.capacity {
		return nil
	}
	start := stitcher.origin * stitcher.stride
	end := (stitcher.origin + stitcher.height) * stitcher.stride
	return &image.RGBA{
		Pix:    stitcher.pixels[start:end],
		Stride: stitcher.stride,
		Rect:   image.Rect(0, 0, stitcher.width, stitcher.height),
	}
}

// Finish returns the completed image and prevents additional frames.
func (stitcher *BidirectionalStitcher) Finish() *image.RGBA {
	if stitcher == nil {
		return nil
	}
	stitcher.finished = true
	return stitcher.Image()
}

func analyzeSignedGrayscale(previous, current []uint8, width, height int, options MatchOptions) BidirectionalResult {
	maxOffset := int(float64(height) * options.MaxOffsetRatio)
	if maxOffset >= height {
		maxOffset = height - 1
	}
	if maxOffset < minimumOffset {
		return bidirectionalRejected(RejectionFrameTooShort, 256, 256)
	}
	stationaryScore := signedOverlapScore(previous, current, width, height, 0, coarseScale)
	if stationaryScore == 0 {
		return bidirectionalRejected(RejectionStationary, stationaryScore, 256)
	}

	bestCoarseDelta := 0
	bestCoarseScore := 256.0
	for delta := -maxOffset; delta <= maxOffset; delta += coarseScale {
		if delta > -minimumOffset && delta < minimumOffset {
			continue
		}
		score := signedOverlapScore(previous, current, width, height, delta, coarseScale)
		if score < bestCoarseScore {
			bestCoarseScore = score
			bestCoarseDelta = delta
		}
	}

	bestDelta := 0
	bestScore := 256.0
	secondScore := 256.0
	for delta := bestCoarseDelta - coarseScale; delta <= bestCoarseDelta+coarseScale; delta++ {
		if delta < -maxOffset || delta > maxOffset || (delta > -minimumOffset && delta < minimumOffset) {
			continue
		}
		score := signedOverlapScore(previous, current, width, height, delta, 2)
		if score < bestScore {
			secondScore = bestScore
			bestScore = score
			bestDelta = delta
		} else if score < secondScore {
			secondScore = score
		}
	}
	if stationaryScore <= options.StationaryDifference && stationaryScore <= bestScore+stationaryScoreHysteresis {
		return bidirectionalRejected(RejectionStationary, stationaryScore, bestScore)
	}
	if bestScore > options.MaxMeanDifference {
		return bidirectionalRejectedWithDelta(RejectionScoreTooHigh, bestDelta, bestScore, secondScore)
	}
	if bestScore > 0.05 && secondScore-bestScore < options.MinimumConfidence {
		return bidirectionalRejectedWithDelta(RejectionAmbiguous, bestDelta, bestScore, secondScore)
	}
	return BidirectionalResult{Delta: bestDelta, Matched: true, BestScore: bestScore, SecondBestScore: secondScore}
}

func signedOverlapScore(previous, current []uint8, width, height, delta, step int) float64 {
	previousStart, currentStart := 0, 0
	rows := height
	if delta >= 0 {
		previousStart = delta
		rows -= delta
	} else {
		currentStart = -delta
		rows += delta
	}
	var difference uint64
	var count uint64
	for row := 0; row < rows; row += step {
		previousRow := (previousStart + row) * width
		currentRow := (currentStart + row) * width
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

func (stitcher *BidirectionalStitcher) relocate(current []uint8) BidirectionalResult {
	votes := make(map[int]int)
	for localY := 0; localY+anchorBandHeight < stitcher.frameHeight; localY++ {
		signature := rowSignature(current, stitcher.width, stitcher.frameHeight, localY)
		for _, pageY := range stitcher.anchors[signature] {
			position := pageY - localY
			if stitcher.validCandidate(position) {
				votes[position]++
			}
		}
	}
	type candidate struct {
		position int
		votes    int
	}
	candidates := make([]candidate, 0, len(votes))
	for position, count := range votes {
		if count >= 2 {
			candidates = append(candidates, candidate{position: position, votes: count})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].votes > candidates[j].votes })
	if len(candidates) > maxRelocateTests {
		candidates = candidates[:maxRelocateTests]
	}

	bestPosition := 0
	bestScore := 256.0
	secondScore := 256.0
	for _, candidate := range candidates {
		score := stitcher.canvasScore(current, candidate.position, 2)
		if score < bestScore {
			secondScore = bestScore
			bestScore = score
			bestPosition = candidate.position
		} else if score < secondScore {
			secondScore = score
		}
	}
	if bestScore > stitcher.options.MaxMeanDifference {
		return bidirectionalRejected(RejectionScoreTooHigh, bestScore, secondScore)
	}
	if bestScore > 0.05 && secondScore-bestScore < stitcher.options.MinimumConfidence {
		return bidirectionalRejected(RejectionAmbiguous, bestScore, secondScore)
	}
	return BidirectionalResult{
		Delta:           bestPosition - stitcher.lastPosition,
		Position:        bestPosition,
		Matched:         true,
		BestScore:       bestScore,
		SecondBestScore: secondScore,
		Relocalized:     true,
	}
}

func (stitcher *BidirectionalStitcher) validCandidate(position int) bool {
	overlapStart := maxInt(position, stitcher.minY)
	overlapEnd := minInt(position+stitcher.frameHeight, stitcher.maxY)
	minimumOverlap := stitcher.frameHeight - int(float64(stitcher.frameHeight)*stitcher.options.MaxOffsetRatio)
	return overlapEnd-overlapStart >= minimumOverlap
}

func (stitcher *BidirectionalStitcher) extensionRows(position int) int {
	return maxInt(0, stitcher.minY-position) + maxInt(0, position+stitcher.frameHeight-stitcher.maxY)
}

func (stitcher *BidirectionalStitcher) canvasScore(current []uint8, position, step int) float64 {
	start := maxInt(position, stitcher.minY)
	end := minInt(position+stitcher.frameHeight, stitcher.maxY)
	canvasGray := stitcher.grayView()
	var difference uint64
	var count uint64
	for pageY := start; pageY < end; pageY += step {
		canvasRow := (pageY - stitcher.minY) * stitcher.width
		currentRow := (pageY - position) * stitcher.width
		for x := 0; x < stitcher.width; x += step {
			a := int(canvasGray[canvasRow+x])
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

func (stitcher *BidirectionalStitcher) place(current image.Image, currentGray []uint8, position int) (int, int, error) {
	if current == nil || current.Bounds().Dx() != stitcher.width || current.Bounds().Dy() != stitcher.frameHeight {
		return 0, 0, errors.New("image dimensions changed during bidirectional capture")
	}
	oldMin, oldMax := stitcher.minY, stitcher.maxY
	newMin := minInt(oldMin, position)
	newMax := maxInt(oldMax, position+stitcher.frameHeight)
	addedTop := oldMin - newMin
	addedBottom := newMax - oldMax
	if addedTop == 0 && addedBottom == 0 {
		return 0, 0, nil
	}

	newHeight := newMax - newMin
	if stitcher.origin < addedTop || stitcher.capacity-(stitcher.origin+stitcher.height) < addedBottom {
		oldBottomSpace := stitcher.capacity - (stitcher.origin + stitcher.height)
		capacity := stitcher.capacity
		for capacity < newHeight+stitcher.frameHeight {
			capacity *= 2
		}
		origin := (capacity - stitcher.height) / 2
		if addedTop == 0 {
			origin = stitcher.origin
		} else if addedBottom == 0 {
			origin = capacity - stitcher.height - oldBottomSpace
		}
		origin = maxInt(origin, addedTop)
		origin = minInt(origin, capacity-stitcher.height-addedBottom)
		pixels := make([]uint8, stitcher.stride*capacity)
		copy(pixels[origin*stitcher.stride:(origin+stitcher.height)*stitcher.stride], stitcher.Image().Pix)
		gray := make([]uint8, stitcher.width*capacity)
		copy(gray[origin*stitcher.width:(origin+stitcher.height)*stitcher.width], stitcher.grayView())
		stitcher.pixels = pixels
		stitcher.gray = gray
		stitcher.capacity = capacity
		stitcher.origin = origin
	}
	stitcher.origin -= addedTop
	stitcher.height = newHeight
	canvas := stitcher.Image()
	currentBounds := current.Bounds()
	if addedTop > 0 {
		draw.Draw(canvas, image.Rect(0, 0, stitcher.width, addedTop), current, currentBounds.Min, draw.Src)
		copy(stitcher.gray[stitcher.origin*stitcher.width:(stitcher.origin+addedTop)*stitcher.width], currentGray[:addedTop*stitcher.width])
	}
	if addedBottom > 0 {
		destinationStart := newHeight - addedBottom
		sourceStart := image.Pt(currentBounds.Min.X, currentBounds.Max.Y-addedBottom)
		draw.Draw(canvas, image.Rect(0, destinationStart, stitcher.width, newHeight), current, sourceStart, draw.Src)
		graySourceStart := (stitcher.frameHeight - addedBottom) * stitcher.width
		grayDestinationStart := (stitcher.origin + newHeight - addedBottom) * stitcher.width
		copy(stitcher.gray[grayDestinationStart:(stitcher.origin+newHeight)*stitcher.width], currentGray[graySourceStart:])
	}

	stitcher.minY = newMin
	stitcher.maxY = newMax
	if addedTop > 0 {
		stitcher.indexRows(newMin, minInt(newMax, oldMin+anchorBandHeight+1))
	}
	if addedBottom > 0 {
		stitcher.indexRows(maxInt(newMin, oldMax-anchorBandHeight), newMax)
	}
	return addedTop, addedBottom, nil
}

func (stitcher *BidirectionalStitcher) indexRows(start, end int) {
	first := start
	if remainder := first % anchorStep; remainder != 0 {
		if remainder < 0 {
			first -= remainder
		} else {
			first += anchorStep - remainder
		}
	}
	for pageY := first; pageY+anchorBandHeight < end; pageY += anchorStep {
		if _, exists := stitcher.indexedRows[pageY]; exists {
			continue
		}
		localY := pageY - stitcher.minY
		signature := rowSignature(stitcher.grayView(), stitcher.width, stitcher.maxY-stitcher.minY, localY)
		if _, ignored := stitcher.ambiguous[signature]; !ignored {
			positions := append(stitcher.anchors[signature], pageY)
			if len(positions) > maxAnchorMatches {
				delete(stitcher.anchors, signature)
				stitcher.ambiguous[signature] = struct{}{}
			} else {
				stitcher.anchors[signature] = positions
			}
		}
		stitcher.indexedRows[pageY] = struct{}{}
	}
}

func (stitcher *BidirectionalStitcher) grayView() []uint8 {
	start := stitcher.origin * stitcher.width
	return stitcher.gray[start : start+stitcher.height*stitcher.width]
}

func rowSignature(pixels []uint8, width, height, row int) uint64 {
	if width <= 1 || row < 0 || row+anchorBandHeight >= height {
		return 0
	}
	var signature uint64
	for index := 0; index < 32; index++ {
		x := index * (width - 1) / 32
		nextX := (index + 1) * (width - 1) / 32
		if pixels[row*width+x] < pixels[row*width+nextX] {
			signature |= uint64(1) << index
		}
		if pixels[row*width+x] < pixels[(row+anchorBandHeight)*width+x] {
			signature |= uint64(1) << (index + 32)
		}
	}
	return signature
}

func bidirectionalRejected(reason RejectionReason, bestScore, secondScore float64) BidirectionalResult {
	return bidirectionalRejectedWithDelta(reason, 0, bestScore, secondScore)
}

func bidirectionalRejectedWithDelta(reason RejectionReason, delta int, bestScore, secondScore float64) BidirectionalResult {
	return BidirectionalResult{Delta: delta, BestScore: bestScore, SecondBestScore: secondScore, Reason: reason}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
