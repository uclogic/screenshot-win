package selector

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"sort"
	"time"
)

// CandidateMode selects automatic region suggestions. The zero value is manual.
type CandidateMode uint8

const (
	CandidateNone CandidateMode = iota
	CandidateWindowsUI
	CandidateMinimalRectangle
)

func (m CandidateMode) Valid() bool { return m <= CandidateMinimalRectangle }

// DetectRectangles returns unique, axis-aligned candidates in image-local
// coordinates, ordered by area then coordinates. Horizontal and vertical
// segments are combined without requiring a connected or closed contour.
func DetectRectangles(ctx context.Context, source image.Image) ([]image.Rectangle, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if source == nil {
		return nil, nil
	}
	b := source.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 100 || h < 80 {
		return nil, nil
	}
	started := time.Now()
	stage := started
	var edgesTime, horizontalTime, verticalTime, combineTime, sortTime, deduplicateTime time.Duration
	var rawCount, uniqueCount int
	defer func() {
		fmt.Fprintf(os.Stderr, "[minimal-rectangle] detection size=%dx%d edges=%s horizontal=%s vertical=%s combine=%s sort=%s deduplicate=%s total=%s raw=%d unique=%d context_error=%v\n", w, h, edgesTime, horizontalTime, verticalTime, combineTime, sortTime, deduplicateTime, time.Since(started), rawCount, uniqueCount, ctx.Err())
	}()
	edges, err := candidateEdges(ctx, source)
	edgesTime = time.Since(stage)
	stage = time.Now()
	if err != nil {
		return nil, err
	}
	horizontal, err := candidateLines(ctx, edges, w, h, false)
	horizontalTime = time.Since(stage)
	stage = time.Now()
	if err != nil {
		return nil, err
	}
	vertical, err := candidateLines(ctx, edges, w, h, true)
	verticalTime = time.Since(stage)
	stage = time.Now()
	if err != nil {
		return nil, err
	}
	var rectangles []image.Rectangle
	// Lines are indexed by their fixed coordinate. Restrict side lookups to
	// the overlap of the two horizontal spans, including rounded corners.
	for ti, top := range horizontal {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for _, bottom := range horizontal[ti+1:] {
			if bottom.pos-top.pos+1 < 80 {
				continue
			}
			lo, hi := max(top.start, bottom.start)-12, min(top.end, bottom.end)+12
			first := sort.Search(len(vertical), func(i int) bool { return vertical[i].pos >= lo })
			var sides []candidateLine
			for vi := first; vi < len(vertical) && vertical[vi].pos <= hi; vi++ {
				v := vertical[vi]
				if v.supports(top.pos, bottom.pos) {
					sides = append(sides, v)
				}
			}
			for li, left := range sides {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				for _, right := range sides[li+1:] {
					if right.pos-left.pos+1 < 100 || !top.supports(left.pos, right.pos) || !bottom.supports(left.pos, right.pos) {
						continue
					}
					rectangles = append(rectangles, image.Rect(left.pos, top.pos, right.pos+1, bottom.pos+1))
				}
			}
		}
	}
	combineTime = time.Since(stage)
	rawCount = len(rectangles)
	stage = time.Now()
	sort.Slice(rectangles, func(i, j int) bool {
		a, b := rectangles[i], rectangles[j]
		if a.Dx()*a.Dy() != b.Dx()*b.Dy() {
			return a.Dx()*a.Dy() < b.Dx()*b.Dy()
		}
		if a.Min.Y != b.Min.Y {
			return a.Min.Y < b.Min.Y
		}
		if a.Min.X != b.Min.X {
			return a.Min.X < b.Min.X
		}
		if a.Max.Y != b.Max.Y {
			return a.Max.Y < b.Max.Y
		}
		return a.Max.X < b.Max.X
	})
	sortTime = time.Since(stage)
	stage = time.Now()
	unique := make([]image.Rectangle, 0, len(rectangles))
	// Bucket all four coordinates so dense grids do not require comparing
	// each rectangle with every previously accepted rectangle.
	buckets := make(map[[4]int][]image.Rectangle)
	for index, r := range rectangles {
		if index%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		duplicate := false
	search:
		for x0 := (r.Min.X - 3) / 4; x0 <= (r.Min.X+3)/4; x0++ {
			for y0 := (r.Min.Y - 3) / 4; y0 <= (r.Min.Y+3)/4; y0++ {
				for x1 := (r.Max.X - 3) / 4; x1 <= (r.Max.X+3)/4; x1++ {
					for y1 := (r.Max.Y - 3) / 4; y1 <= (r.Max.Y+3)/4; y1++ {
						for _, old := range buckets[[4]int{x0, y0, x1, y1}] {
							if candidateAbs(r.Min.X-old.Min.X) <= 3 && candidateAbs(r.Min.Y-old.Min.Y) <= 3 && candidateAbs(r.Max.X-old.Max.X) <= 3 && candidateAbs(r.Max.Y-old.Max.Y) <= 3 {
								duplicate = true
								break search
							}
						}
					}
				}
			}
		}
		if !duplicate {
			unique = append(unique, r)
			key := [4]int{r.Min.X / 4, r.Min.Y / 4, r.Max.X / 4, r.Max.Y / 4}
			buckets[key] = append(buckets[key], r)
		}
	}
	deduplicateTime = time.Since(stage)
	uniqueCount = len(unique)
	return unique, ctx.Err()
}

func candidateAbs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Directional nonmaximum-suppressed gradients. Long-line support validates
// weak edges instead of requiring them to connect to a strong edge seed.
func candidateEdges(ctx context.Context, source image.Image) ([]uint8, error) {
	b := source.Bounds()
	w, h := b.Dx(), b.Dy()
	gray := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		if y%64 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		for x := 0; x < w; x++ {
			switch img := source.(type) {
			case *image.RGBA:
				i := img.PixOffset(b.Min.X+x, b.Min.Y+y)
				gray[y*w+x] = uint8((19595*uint32(img.Pix[i]) + 38470*uint32(img.Pix[i+1]) + 7471*uint32(img.Pix[i+2]) + 32768) >> 16)
			default:
				gray[y*w+x] = color.GrayModel.Convert(source.At(b.Min.X+x, b.Min.Y+y)).(color.Gray).Y
			}
		}
	}
	blur := make([]uint8, w*h)
	for y := 1; y < h-1; y++ {
		if y%64 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		for x := 1; x < w-1; x++ {
			i := y*w + x
			blur[i] = uint8((int(gray[i-w-1]) + 2*int(gray[i-w]) + int(gray[i-w+1]) + 2*int(gray[i-1]) + 4*int(gray[i]) + 2*int(gray[i+1]) + int(gray[i+w-1]) + 2*int(gray[i+w]) + int(gray[i+w+1])) / 16)
		}
	}
	mag := make([]uint16, w*h)
	direction := make([]uint8, w*h)
	for y := 2; y < h-2; y++ {
		if y%64 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		for x := 2; x < w-2; x++ {
			i := y*w + x
			gx := -int(blur[i-w-1]) + int(blur[i-w+1]) - 2*int(blur[i-1]) + 2*int(blur[i+1]) - int(blur[i+w-1]) + int(blur[i+w+1])
			gy := -int(blur[i-w-1]) - 2*int(blur[i-w]) - int(blur[i-w+1]) + int(blur[i+w-1]) + 2*int(blur[i+w]) + int(blur[i+w+1])
			ax, ay := candidateAbs(gx), candidateAbs(gy)
			mag[i] = uint16(ax + ay)
			if ay*1000 <= ax*414 {
				direction[i] = 0
			} else if ax*1000 <= ay*414 {
				direction[i] = 2
			} else if gx*gy > 0 {
				direction[i] = 1
			} else {
				direction[i] = 3
			}
		}
	}
	edges := make([]uint8, w*h)
	for y := 2; y < h-2; y++ {
		if y%64 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		for x := 2; x < w-2; x++ {
			i := y*w + x
			if direction[i] != 0 && direction[i] != 2 {
				continue
			}
			d := 1
			bit := uint8(1) // vertical line (horizontal gradient)
			if direction[i] == 2 {
				d = w
				bit = 2
			}
			if mag[i] >= 20 && mag[i] >= mag[i-d] && mag[i] >= mag[i+d] {
				edges[i] = bit
				if mag[i] >= 100 {
					edges[i] |= bit << 2
				}
			}
		}
	}
	return edges, ctx.Err()
}

// SmallestRectangleAt expects the sorted output of DetectRectangles.
func SmallestRectangleAt(rectangles []image.Rectangle, p image.Point) (image.Rectangle, bool) {
	for _, r := range rectangles {
		if p.In(r) {
			return r, true
		}
	}
	return image.Rectangle{}, false
}

// DrawCandidateRectangles returns an annotated copy; detection input is intact.
func DrawCandidateRectangles(source image.Image, rectangles []image.Rectangle) *image.RGBA {
	b := source.Bounds()
	result := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(result, result.Bounds(), source, b.Min, draw.Src)
	blue := image.NewUniform(color.RGBA{0x16, 0x8c, 0xff, 255})
	for _, r := range rectangles {
		r = r.Intersect(result.Bounds())
		if r.Empty() {
			continue
		}
		for _, side := range []image.Rectangle{image.Rect(r.Min.X, r.Min.Y, r.Max.X, min(r.Min.Y+2, r.Max.Y)), image.Rect(r.Min.X, max(r.Min.Y, r.Max.Y-2), r.Max.X, r.Max.Y), image.Rect(r.Min.X, r.Min.Y, min(r.Min.X+2, r.Max.X), r.Max.Y), image.Rect(max(r.Min.X, r.Max.X-2), r.Min.Y, r.Max.X, r.Max.Y)} {
			draw.Draw(result, side, blue, image.Point{}, draw.Src)
		}
	}
	return result
}
