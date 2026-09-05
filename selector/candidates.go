package selector

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"sort"
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
// coordinates, ordered by area then coordinates. Both edge components and
// enclosed regions are examined so nested rectangles survive contour retrieval.
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
	edges, err := candidateEdges(ctx, source)
	if err != nil {
		return nil, err
	}
	// Bridge only short horizontal/vertical gaps, without joining distant text.
	closed := append([]bool(nil), edges...)
	for y := 1; y < h-1; y++ {
		if y%64 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		for x := 1; x < w-1; x++ {
			i := y*w + x
			if edges[i] {
				continue
			}
			closed[i] = edges[i-1] && edges[i+1] || edges[i-w] && edges[i+w]
		}
	}
	seen := make([]bool, w*h)
	queue := make([]int32, 0, w*h)
	var rectangles []image.Rectangle
	for seed := range closed {
		if seed%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if seen[seed] {
			continue
		}
		kind := closed[seed]
		seen[seed] = true
		queue = append(queue[:0], int32(seed))
		left, top, right, bottom := seed%w, seed/w, seed%w, seed/w
		touchesBorder := false
		for head := 0; head < len(queue); head++ {
			if head%4096 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			i := int(queue[head])
			x, y := i%w, i/w
			left, top, right, bottom = min(left, x), min(top, y), max(right, x), max(bottom, y)
			touchesBorder = touchesBorder || x == 0 || y == 0 || x == w-1 || y == h-1
			for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				nx, ny := x+d[0], y+d[1]
				if nx < 0 || nx >= w || ny < 0 || ny >= h {
					continue
				}
				n := ny*w + nx
				if !seen[n] && closed[n] == kind {
					seen[n] = true
					queue = append(queue, int32(n))
				}
			}
		}
		if !kind && touchesBorder {
			continue
		}
		r := image.Rect(left, top, right+1, bottom+1)
		if r.Dx() < 96 || r.Dy() < 76 {
			continue
		}
		if !rectangleSupported(closed, w, h, r) {
			continue
		}
		// Canny traces the two sides of a thin border. Map the outside
		// component / enclosed interior back to the border center before
		// applying the minimum size, so a 99px box does not become 101px.
		if kind {
			r = r.Inset(1)
		} else {
			r = r.Inset(-2)
		}
		r = r.Intersect(image.Rect(0, 0, w, h))
		if r.Dx() < 100 || r.Dy() < 80 {
			continue
		}
		rectangles = append(rectangles, r)
	}
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
	unique := make([]image.Rectangle, 0, len(rectangles))
	for _, r := range rectangles {
		duplicate := false
		for _, old := range unique {
			if candidateAbs(r.Min.X-old.Min.X) <= 3 && candidateAbs(r.Min.Y-old.Min.Y) <= 3 && candidateAbs(r.Max.X-old.Max.X) <= 3 && candidateAbs(r.Max.Y-old.Max.Y) <= 3 {
				duplicate = true
				break
			}
		}
		if !duplicate {
			unique = append(unique, r)
		}
	}
	return unique, ctx.Err()
}

func candidateAbs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Validate all four sides, rather than accepting an arbitrary contour's box.
func rectangleSupported(edges []bool, w, h int, r image.Rectangle) bool {
	hit := func(x, y int, vertical bool) bool {
		for d := -2; d <= 2; d++ {
			nx, ny := x, y
			if vertical {
				nx += d
			} else {
				ny += d
			}
			if nx >= 0 && nx < w && ny >= 0 && ny < h && edges[ny*w+nx] {
				return true
			}
		}
		return false
	}
	for _, x := range []int{r.Min.X, r.Max.X - 1} {
		count := 0
		for y := r.Min.Y; y < r.Max.Y; y++ {
			if hit(x, y, true) {
				count++
			}
		}
		if count*100 < r.Dy()*85 {
			return false
		}
	}
	for _, y := range []int{r.Min.Y, r.Max.Y - 1} {
		count := 0
		for x := r.Min.X; x < r.Max.X; x++ {
			if hit(x, y, false) {
				count++
			}
		}
		if count*100 < r.Dx()*85 {
			return false
		}
	}
	return true
}

// Canny: Gaussian smoothing, Sobel gradient, nonmaximum suppression and
// hysteresis. Fixed thresholds are in 8-bit intensity-gradient units.
func candidateEdges(ctx context.Context, source image.Image) ([]bool, error) {
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
	weak := make([]bool, w*h)
	edges := make([]bool, w*h)
	queue := make([]int, 0)
	for y := 2; y < h-2; y++ {
		for x := 2; x < w-2; x++ {
			i := y*w + x
			offsets := [4]int{1, w + 1, w, w - 1}
			d := offsets[direction[i]]
			if mag[i] >= 40 && mag[i] >= mag[i-d] && mag[i] >= mag[i+d] {
				weak[i] = true
				if mag[i] >= 100 {
					edges[i] = true
					queue = append(queue, i)
				}
			}
		}
	}
	for head := 0; head < len(queue); head++ {
		if head%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		i := queue[head]
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				n := i + dy*w + dx
				if n >= 0 && n < len(edges) && weak[n] && !edges[n] {
					edges[n] = true
					queue = append(queue, n)
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
