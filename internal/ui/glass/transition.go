package glass

import "time"

const TransitionDuration = 140 * time.Millisecond

// Transition retargets from the current value rather than snapping when the
// pointer reverses direction. Callers stop their timer once active is false.
type Transition struct {
	from, to float64
	start    time.Time
}

func (t *Transition) Update(on bool, now time.Time) (value float64, active bool) {
	u := float64(now.Sub(t.start)) / float64(TransitionDuration)
	u = max(0, min(1, u))
	value = t.from + (t.to-t.from)*(u*u*(3-2*u))
	target := 0.0
	if on {
		target = 1
	}
	if target != t.to {
		t.from, t.to, t.start = value, target, now
		u = 0
	}
	return value, u < 1 && t.from != t.to
}
