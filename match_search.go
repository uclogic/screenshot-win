package screenshotwin

import (
	"bytes"
	"math"
	"sort"
)

const (
	descriptorBins      = 8
	matchCandidateLimit = 8
	peakRadius          = 6
	verifyTileWidth     = 64
	verifyTileHeight    = 32
	textureThreshold    = 8
	// Texture is evidence of alignment, not a second absolute MAD limit.
	// Relative error tolerates fractional-pixel resampling of strong edges
	// while rejecting misplaced marks even on almost entirely blank pages.
	maxTextureErrorRatio = 0.45
)

// Fixed-point means retain small marks that would round away in byte means.
// Every source row and column contributes; the descriptor is only a locator.
type rowFeature [descriptorBins * 2]uint16

func describeRows(pixels []uint8, width, height int) []rowFeature {
	rows := make([]rowFeature, height)
	for y := range rows {
		row := pixels[y*width : (y+1)*width]
		for bin := 0; bin < descriptorBins; bin++ {
			start, end := bin*width/descriptorBins, (bin+1)*width/descriptorBins
			if start == end {
				continue
			}
			sum, gradient := 0, 0
			for x := start; x < end; x++ {
				sum += int(row[x])
				if x > 0 {
					gradient += absDifference(row[x], row[x-1])
				}
			}
			rows[y][bin] = uint16(sum * 16 / (end - start))
			rows[y][bin+descriptorBins] = uint16(gradient * 16 / (end - start))
		}
	}
	return rows
}

type offsetScore struct {
	offset int
	score  float64
}

func descriptorScore(previous, current []rowFeature, delta int) float64 {
	p, c, rows := overlapRows(len(previous), delta)
	var sum uint64
	for y := 0; y < rows; y++ {
		a, b := &previous[p+y], &current[c+y]
		for k := range a {
			difference := int(a[k]) - int(b[k])
			if difference < 0 {
				difference = -difference
			}
			sum += uint64(difference)
		}
	}
	return float64(sum) / float64(rows*descriptorBins*2*16)
}

func overlapRows(height, delta int) (previous, current, rows int) {
	if delta >= 0 {
		return delta, 0, height - delta
	}
	return 0, -delta, height + delta
}

func independentCandidates(scores []offsetScore) []offsetScore {
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].score != scores[j].score {
			return scores[i].score < scores[j].score
		}
		return scores[i].offset < scores[j].offset
	})
	selected := make([]offsetScore, 0, matchCandidateLimit)
	for _, candidate := range scores {
		independent := true
		for _, other := range selected {
			if samePeak(candidate.offset, other.offset) {
				independent = false
				break
			}
		}
		if independent {
			selected = append(selected, candidate)
			if len(selected) == matchCandidateLimit {
				break
			}
		}
	}
	return selected
}

func samePeak(a, b int) bool { return a-b >= -peakRadius && a-b <= peakRadius }

// searchScroll is shared by the stateful, stateless and signed matchers.
// Descriptors select candidates; only pixel verification can accept them.
func searchScroll(previous, current []uint8, width, height int, prevRows, currRows []rowFeature, signed bool, options MatchOptions) MatchResult {
	maximum := minInt(height-1, int(float64(height)*options.MaxOffsetRatio))
	if maximum < minimumOffset {
		return rejected(RejectionFrameTooShort, 256, 256)
	}
	// A sampled zero difference is not proof of a duplicate: thin text may
	// live entirely between the sampled rows or columns.
	if bytes.Equal(previous, current) {
		return rejected(RejectionStationary, 0, 256)
	}
	if prevRows == nil {
		prevRows = describeRows(previous, width, height)
	}
	if currRows == nil {
		currRows = describeRows(current, width, height)
	}
	minimum := minimumOffset
	if signed {
		minimum = -maximum
	}
	allowed := func(delta int) bool {
		return delta >= minimum && delta <= maximum && (delta <= -minimumOffset || delta >= minimumOffset)
	}
	scores := make([]offsetScore, 0, maximum-minimum+1)
	for delta := minimum; delta <= maximum; delta++ {
		if allowed(delta) {
			scores = append(scores, offsetScore{delta, descriptorScore(prevRows, currRows, delta)})
		}
	}
	// Refinement uses 32 columns across the viewport. The final tile verifier
	// remains denser; sampling here can only propose, never accept, a shift.
	columns := make([]int, minInt(width, 32))
	for i := range columns {
		columns[i] = i * width / len(columns)
	}
	verify := func(candidates []offsetScore, refine bool) MatchResult {
		verified := make([]verifiedOffset, 0, len(candidates))
		seen := make(map[int]bool, len(candidates))
		for _, candidate := range candidates {
			delta := candidate.offset
			if refine {
				// Keep the descriptor winner on pixel-score ties. This matters for
				// thin marks which coarse pixel sampling may not see at all.
				best := refinementScore(previous, current, width, height, delta, columns)
				for nearby := candidate.offset - peakRadius; nearby <= candidate.offset+peakRadius; nearby++ {
					if !allowed(nearby) || nearby == delta {
						continue
					}
					score := refinementScore(previous, current, width, height, nearby, columns)
					if score < best {
						best, delta = score, nearby
					}
				}
			}
			// Verify the original locator as well if coarse refinement moved it;
			// local animation must not erase a good descriptor candidate.
			for _, position := range []int{delta, candidate.offset} {
				if !seen[position] {
					p, c, rows := overlapRows(height, position)
					quality := verifyOverlap(previous[p*width:(p+rows)*width], current[c*width:(c+rows)*width], width, rows)
					verified = append(verified, verifiedOffset{position, quality})
					seen[position] = true
				}
			}
		}
		return chooseVerified(verified, options)
	}
	result := verify(independentCandidates(scores), true)
	weakDescriptor := scores[len(scores)-1].score-scores[0].score < 1.0/16
	if !result.Matched && result.Reason != RejectionAmbiguous {
		// Search every allowed displacement in grayscale. Dense input such as
		// unrelated screenshots does not need every pixel of every overlap.
		for i := range scores {
			scores[i].score = signedOverlapScore(previous, current, width, height, scores[i].offset, coarseScale)
		}
		result = verify(independentCandidates(scores), true)
		if !result.Matched && result.Reason != RejectionAmbiguous && weakDescriptor {
			// With indistinguishable descriptors even the grayscale sampling
			// lattice can miss all the evidence. Reserve dense search for this
			// exceptional case, and still require the usual pixel verification.
			for i := range scores {
				scores[i].score = signedOverlapScore(previous, current, width, height, scores[i].offset, 1)
			}
			result = verify(independentCandidates(scores), false)
		}
	}
	stationary := signedOverlapScore(previous, current, width, height, 0, 1)
	if stationary <= options.StationaryDifference {
		zero := verifyOverlap(previous, current, width, height)
		if (zero.accepts(options) || zero.informative == 0) && zero.score <= result.BestScore+stationaryScoreHysteresis {
			return rejected(RejectionStationary, stationary, result.BestScore)
		}
	}
	return result
}

func refinementScore(previous, current []uint8, width, height, delta int, columns []int) float64 {
	p, c, rows := overlapRows(height, delta)
	var difference uint64
	for y := 0; y < rows; y += 2 {
		a, b := previous[(p+y)*width:], current[(c+y)*width:]
		for _, x := range columns {
			difference += uint64(absDifference(a[x], b[x]))
		}
	}
	return float64(difference) / float64(((rows+1)/2)*len(columns))
}

type overlapQuality struct {
	score       float64
	informative int
	inliers     int
}

func (q overlapQuality) accepts(options MatchOptions) bool {
	return q.informative > 0 && q.inliers*5 >= q.informative*4 && q.score <= options.MaxMeanDifference
}

type verifiedOffset struct {
	offset  int
	quality overlapQuality
}

// chooseVerified also serves history relocation, so exact repeats cannot
// bypass ambiguity checking by taking a different matching path.
func chooseVerified(candidates []verifiedOffset, options MatchOptions) MatchResult {
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.quality.accepts(options) != b.quality.accepts(options) {
			return a.quality.accepts(options)
		}
		if a.quality.score != b.quality.score {
			return a.quality.score < b.quality.score
		}
		return a.offset < b.offset
	})
	if len(candidates) == 0 {
		return rejected(RejectionScoreTooHigh, 256, 256)
	}
	best := candidates[0]
	second := 256.0
	for _, candidate := range candidates[1:] {
		if !samePeak(best.offset, candidate.offset) && candidate.quality.accepts(options) {
			second = math.Min(second, candidate.quality.score)
		}
	}
	if !best.quality.accepts(options) {
		return rejectedWithOffset(RejectionScoreTooHigh, best.offset, best.quality.score, second)
	}
	gap := second - best.quality.score
	// 256 denotes a missing competitor, not a measured confidence gap.
	// Actual pixel scores cannot establish a gap of 256.
	if gap <= 1e-9 || gap < options.MinimumConfidence || options.MinimumConfidence == 256 {
		return rejectedWithOffset(RejectionAmbiguous, best.offset, best.quality.score, second)
	}
	return MatchResult{Offset: best.offset, Matched: true, BestScore: best.quality.score, SecondBestScore: second}
}

// verifyOverlap measures both all pixels and pixels with local structure.
// Blank tiles therefore cannot outvote the small amount of actual text.
func verifyOverlap(previous, current []uint8, width, height int) overlapQuality {
	quality := measureTiles(previous, current, width, height, 2, true)
	if quality.informative < 5 {
		// Sparse content needs every pixel and no trimming of its evidence.
		return measureTiles(previous, current, width, height, 1, false)
	}
	if quality.score == 0 && !bytes.Equal(previous, current) {
		// Zero sampled error can hide changes entirely between sample points.
		return measureTiles(previous, current, width, height, 1, true)
	}
	return quality
}

func measureTiles(previous, current []uint8, width, height, step int, allowTrim bool) overlapQuality {
	all := make([]float64, 0, ((width+verifyTileWidth-1)/verifyTileWidth)*((height+verifyTileHeight-1)/verifyTileHeight))
	quality := overlapQuality{}
	threshold := textureThreshold
	if step == 1 {
		// Dense fallback must also retain low-contrast text and smooth edges.
		threshold = 1
	}
	for top := 0; top < height; top += verifyTileHeight {
		bottom := minInt(height, top+verifyTileHeight)
		for left := 0; left < width; left += verifyTileWidth {
			right := minInt(width, left+verifyTileWidth)
			sum, count, textureSum, textureWeight := 0, 0, 0, 0
			for y := top; y < bottom; y += step {
				for x := left; x < right; x += step {
					i := y*width + x
					difference := absDifference(previous[i], current[i])
					sum += difference
					count++
					edgeError, edgeWeight, contrast := textureDifference(previous, current, width, x, y)
					if contrast >= threshold {
						textureSum += edgeError
						textureWeight += edgeWeight
					}
				}
			}
			mean := float64(sum) / float64(count)
			all = append(all, mean)
			if textureWeight > 0 {
				quality.informative++
				if float64(textureSum) <= maxTextureErrorRatio*float64(textureWeight) {
					quality.inliers++
				}
			}
		}
	}
	trim := allowTrim && quality.informative >= 5
	// Keep MaxMeanDifference in its original whole-overlap pixel units.
	// Applying that same threshold to edge-only MAD made ordinary animated
	// text fail verification even when the correct displacement was found.
	trimCount := 0
	if trim {
		// Blank tiles must not increase the budget for discarding content.
		trimCount = quality.informative / 5
	}
	quality.score = trimmedMean(all, trimCount)
	return quality
}

func trimmedMean(scores []float64, trimCount int) float64 {
	if len(scores) == 0 {
		return 0
	}
	if trimCount > 0 {
		sort.Float64s(scores)
		scores = scores[:len(scores)-trimCount]
	}
	var total float64
	for _, score := range scores {
		total += score
	}
	return total / float64(len(scores))
}

// Compare signed gradients rather than the absolute intensity at edge pixels.
// Weighting by the two frames' combined contrast keeps the test independent
// of font color and prevents unchanged white neighbors from diluting errors.
func textureDifference(previous, current []uint8, width, x, y int) (difference, weight, contrast int) {
	i := y*width + x
	if x > 0 {
		p, c := int(previous[i])-int(previous[i-1]), int(current[i])-int(current[i-1])
		difference += absInt(p - c)
		weight += absInt(p) + absInt(c)
		contrast = maxInt(absInt(p), absInt(c))
	}
	if y > 0 {
		p, c := int(previous[i])-int(previous[i-width]), int(current[i])-int(current[i-width])
		difference += absInt(p - c)
		weight += absInt(p) + absInt(c)
		contrast = maxInt(contrast, maxInt(absInt(p), absInt(c)))
	}
	return difference, weight, contrast
}

func absDifference(a, b uint8) int {
	return absInt(int(a) - int(b))
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
