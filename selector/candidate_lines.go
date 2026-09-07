package selector

import (
	"context"
	"sort"
)

// start/end are inclusive. Prefix counts permit constant-time side checks.
// breaks records the endpoint of every unsupported run longer than eight.
type candidateLine struct {
	pos, start, end int
	hits, strong    []int
	breaks          []int
}

func (l candidateLine) supports(start, end int) bool {
	if start < l.start-12 || end > l.end+12 {
		return false
	}
	a, b := start+12, end-12
	if a > b || a < l.start || b > l.end {
		return false
	}
	i, j := a-l.start, b-l.start+1
	n := j - i
	coverage := 75
	if (l.strong[j]-l.strong[i])*100 < n*25 {
		coverage = 85
	}
	if (l.hits[j]-l.hits[i])*100 < n*coverage {
		return false
	}
	k := sort.SearchInts(l.breaks, a+8)
	return k == len(l.breaks) || l.breaks[k] > b
}

func candidateLines(ctx context.Context, edges []uint8, w, h int, vertical bool) ([]candidateLine, error) {
	across, along, bit, minimum := h, w, uint8(2), 76
	if vertical {
		across, along, bit, minimum = w, h, 1, 56
	}
	at := func(p, t int) uint8 {
		if vertical {
			return edges[t*w+p]
		}
		return edges[p*w+t]
	}
	type segment struct{ lo, hi, start, end int }
	var merged []segment
	// Find long runs with two pixels of perpendicular tolerance. Local gaps are allowed;
	// adjacent traces of the same border are merged by matching endpoints.
	// The 12-pixel merge band allows an eight-pixel trace separation plus
	// the two-pixel scan tolerance on either side.
	for p := 2; p < across-2; p++ {
		if p%32 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		start, last := -1, -1
		flush := func() {
			if start < 0 || last-start+1 < minimum {
				return
			}
			for i := len(merged) - 1; i >= 0; i-- {
				old := &merged[i]
				if p-old.lo > 12 {
					break
				}
				if p-old.lo <= 12 && p-old.hi <= 4 && candidateAbs(start-old.start) <= 8 && candidateAbs(last-old.end) <= 8 {
					old.hi = p
					old.start = min(old.start, start)
					old.end = max(old.end, last)
					return
				}
			}
			merged = append(merged, segment{p, p, start, last})
		}
		for t := 2; t < along-2; t++ {
			value := uint8(0)
			for q := max(2, p-2); q <= min(across-3, p+2); q++ {
				value |= at(q, t)
			}
			if value&bit != 0 {
				if start < 0 {
					start = t
				}
				last = t
			} else if start >= 0 && t-last > 8 {
				flush()
				start = -1
			}
		}
		flush()
	}
	lines := make([]candidateLine, 0, len(merged))
	for index, s := range merged {
		if index%32 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		l := candidateLine{pos: (s.lo + s.hi) / 2, start: s.start, end: s.end}
		l.hits = make([]int, s.end-s.start+2)
		l.strong = make([]int, len(l.hits))
		gap := 0
		for t := s.start; t <= s.end; t++ {
			value := uint8(0)
			for p := max(2, s.lo); p <= min(across-3, s.hi); p++ {
				value |= at(p, t)
			}
			i := t - s.start
			l.hits[i+1] = l.hits[i]
			l.strong[i+1] = l.strong[i]
			if value&bit != 0 {
				l.hits[i+1]++
				gap = 0
			} else {
				gap++
				if gap > 8 {
					l.breaks = append(l.breaks, t)
				}
			}
			if value&(bit<<2) != 0 {
				l.strong[i+1]++
			}
		}
		lines = append(lines, l)
	}
	sort.Slice(lines, func(i, j int) bool {
		if lines[i].pos != lines[j].pos {
			return lines[i].pos < lines[j].pos
		}
		return lines[i].start < lines[j].start
	})
	return lines, ctx.Err()
}
