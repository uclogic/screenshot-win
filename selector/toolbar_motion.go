package selector

import (
	"image/color"
	"time"
)

const (
	toolbarHoverDuration    = 140 * time.Millisecond
	toolbarPressDuration    = 80 * time.Millisecond
	toolbarReleaseDuration  = 120 * time.Millisecond
	toolbarSemanticPeak     = 70 * time.Millisecond
	toolbarSemanticDuration = 180 * time.Millisecond
)

// toolbarEase evaluates cubic-bezier(.2, .8, .2, 1), solving x for time.
func toolbarEase(t float64) float64 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	lo, hi := 0.0, 1.0
	for i := 0; i < 24; i++ {
		u := (lo + hi) / 2
		x := 3*(1-u)*(1-u)*u*.2 + 3*(1-u)*u*u*.2 + u*u*u
		if x < t {
			lo = u
		} else {
			hi = u
		}
	}
	u := (lo + hi) / 2
	return 3*(1-u)*(1-u)*u*.8 + 3*(1-u)*u*u + u*u*u
}

type toolbarTween struct {
	from, to float64
	start    time.Time
	duration time.Duration
}

func (t toolbarTween) sample(now time.Time) (float64, bool) {
	if t.duration <= 0 || now.Sub(t.start) >= t.duration || t.from == t.to {
		return t.to, false
	}
	u := toolbarEase(float64(now.Sub(t.start)) / float64(t.duration))
	return t.from + (t.to-t.from)*u, true
}

func (t *toolbarTween) target(value float64, duration time.Duration, now time.Time) {
	if value == t.to {
		return
	}
	current, _ := t.sample(now)
	*t = toolbarTween{from: current, to: value, start: now, duration: duration}
}

type toolbarMotionInput struct{ hover, selected, pressed bool }

// State belongs to the toolbar, not its glass surface, so fallback presentation
// preserves an in-flight gesture. All values are logical (96 DPI) coordinates.
type toolbarMotion struct {
	input                            toolbarMotionInput
	scale, y, hover, selected, press toolbarTween
	semantic                         toolbarTween
	semanticStart                    time.Time
	semanticRunning                  bool
	initialized                      bool
}

type toolbarMotionFrame struct {
	scale, y, semantic     float64
	hover, selected, press float64
}

func toolbarHasSemantic(action Action) bool {
	return action == ActionCopy || action == ActionScroll || action == ActionPin
}

func (m *toolbarMotion) update(action Action, input toolbarMotionInput, now time.Time) (toolbarMotionFrame, bool) {
	if !m.initialized {
		m.scale = toolbarTween{from: 1, to: 1}
		m.initialized = true
	}
	// Complete the outward leg at its scheduled time even when a frame is late.
	if m.semanticRunning && now.Sub(m.semanticStart) >= toolbarSemanticPeak {
		m.semantic.target(0, toolbarSemanticDuration-toolbarSemanticPeak, m.semanticStart.Add(toolbarSemanticPeak))
		m.semanticRunning = false
	}
	if input != m.input {
		duration := toolbarHoverDuration
		if input.pressed {
			duration = toolbarPressDuration
		} else if m.input.pressed {
			duration = toolbarReleaseDuration
		}
		scale, y := 1.0, 0.0
		if input.hover && !input.selected {
			scale, y = 1.08, -1
		}
		if input.pressed {
			scale, y = .94, 0
		}
		m.scale.target(scale, duration, now)
		m.y.target(y, duration, now)
		m.hover.target(boolLevel(input.hover), toolbarHoverDuration, now)
		m.selected.target(boolLevel(input.selected), toolbarHoverDuration, now)
		m.press.target(boolLevel(input.pressed), duration, now)
		if !input.hover || input.selected || input.pressed {
			m.semantic.target(0, duration, now)
			m.semanticRunning = false
		} else if !m.input.hover && toolbarHasSemantic(action) {
			m.semantic.target(1, toolbarSemanticPeak, now)
			m.semanticStart, m.semanticRunning = now, true
		}
		m.input = input
	}
	var f toolbarMotionFrame
	active := m.semanticRunning
	for _, p := range []struct {
		tween toolbarTween
		value *float64
	}{
		{m.scale, &f.scale}, {m.y, &f.y}, {m.hover, &f.hover},
		{m.selected, &f.selected}, {m.press, &f.press}, {m.semantic, &f.semantic},
	} {
		value, moving := p.tween.sample(now)
		*p.value = value
		active = active || moving
	}
	return f, active
}

func boolLevel(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func toolbarMixColor(a, b color.NRGBA, amount float64) color.NRGBA {
	mix := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*amount + .5) }
	return color.NRGBA{mix(a.R, b.R), mix(a.G, b.G), mix(a.B, b.B), mix(a.A, b.A)}
}

func (f toolbarMotionFrame) ink() color.NRGBA {
	base := toolbarMixColor(color.NRGBA{35, 45, 60, 255}, color.NRGBA{20, 30, 45, 255}, f.hover)
	return toolbarMixColor(base, color.NRGBA{25, 85, 165, 255}, f.selected)
}
