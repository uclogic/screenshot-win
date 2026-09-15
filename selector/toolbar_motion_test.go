package selector

import (
	"math"
	"reflect"
	"testing"
	"time"
)

func TestToolbarMotionGesture(t *testing.T) {
	var m toolbarMotion
	now := time.Unix(100, 0)
	hover := toolbarMotionInput{hover: true}
	if f, active := m.update(ActionText, toolbarMotionInput{}, now); f.scale != 1 || active {
		t.Fatal("initial state must be still", f)
	}
	m.update(ActionText, hover, now)
	f, active := m.update(ActionText, hover, now.Add(toolbarHoverDuration))
	if f.scale != 1.08 || f.y != -1 || f.hover != 1 || active {
		t.Fatal("hover endpoint", f, active)
	}
	now = now.Add(time.Second)
	press := toolbarMotionInput{hover: true, pressed: true}
	m.update(ActionText, press, now)
	f, _ = m.update(ActionText, press, now.Add(toolbarPressDuration))
	if f.scale != .94 || f.y != 0 || f.press != 1 {
		t.Fatal("press endpoint", f)
	}
	now = now.Add(time.Second)
	m.update(ActionText, hover, now)
	f, active = m.update(ActionText, hover, now.Add(toolbarReleaseDuration))
	if f.scale != 1.08 || f.y != -1 || f.press != 0 || active {
		t.Fatal("release endpoint", f, active)
	}
	now = now.Add(time.Second)
	m.update(ActionText, toolbarMotionInput{}, now)
	f, active = m.update(ActionText, toolbarMotionInput{}, now.Add(toolbarHoverDuration))
	if f.scale != 1 || f.y != 0 || f.hover != 0 || active {
		t.Fatal("leave endpoint", f, active)
	}
}

func TestToolbarMotionInterruptionsAreContinuous(t *testing.T) {
	for _, next := range []toolbarMotionInput{{}, {hover: true, pressed: true}, {hover: true, selected: true}} {
		var m toolbarMotion
		now := time.Unix(100, 0)
		hover := toolbarMotionInput{hover: true}
		m.update(ActionCopy, hover, now)
		now = now.Add(35 * time.Millisecond)
		before, _ := m.update(ActionCopy, hover, now)
		after, _ := m.update(ActionCopy, next, now)
		if before != after {
			t.Fatalf("interruption snapped: %+v -> %+v", before, after)
		}
		f, active := m.update(ActionCopy, next, now.Add(time.Second))
		if f.semantic != 0 || active {
			t.Fatal("interrupted motion did not settle", f)
		}
	}
}

func TestToolbarSemanticPlaysOnceAndHandlesLateFrames(t *testing.T) {
	for _, action := range []Action{ActionCopy, ActionScroll, ActionPin} {
		var m toolbarMotion
		now := time.Unix(100, 0)
		hover := toolbarMotionInput{hover: true}
		m.update(action, hover, now)
		f, active := m.update(action, hover, now.Add(toolbarSemanticPeak))
		if f.semantic != 1 || !active {
			t.Fatal("semantic peak", action, f)
		}
		f, active = m.update(action, hover, now.Add(toolbarSemanticDuration))
		if f.semantic != 0 || active {
			t.Fatal("semantic end", action, f)
		}
		f, active = m.update(action, hover, now.Add(time.Second))
		if f.semantic != 0 || active {
			t.Fatal("hover replayed", action, f)
		}
		var late toolbarMotion
		late.update(action, hover, now)
		f, active = late.update(action, hover, now.Add(time.Second))
		if f.semantic != 0 || active {
			t.Fatal("late frame queued animation", action, f)
		}
		m.update(action, toolbarMotionInput{}, now.Add(time.Second))
		m.update(action, hover, now.Add(2*time.Second))
		f, _ = m.update(action, hover, now.Add(2*time.Second+toolbarSemanticPeak))
		if f.semantic != 1 {
			t.Fatal("new entry did not play", action, f)
		}
	}
}

func TestToolbarSelectedSuppressesHoverMotion(t *testing.T) {
	var m toolbarMotion
	now := time.Unix(100, 0)
	selected := toolbarMotionInput{selected: true}
	m.update(ActionPin, selected, now)
	now = now.Add(time.Second)
	selected.hover = true
	m.update(ActionPin, selected, now)
	f, active := m.update(ActionPin, selected, now.Add(time.Second))
	if f.scale != 1 || f.y != 0 || f.semantic != 0 || f.selected != 1 || active {
		t.Fatal("selected icon moved", f)
	}
	selected.pressed = true
	m.update(ActionPin, selected, now.Add(2*time.Second))
	f, _ = m.update(ActionPin, selected, now.Add(2*time.Second+toolbarPressDuration))
	if f.scale != .94 {
		t.Fatal("selected icon lost press feedback", f)
	}
}

func TestToolbarEaseMonotonicAndBounded(t *testing.T) {
	previous := 0.0
	for i := 0; i <= 1000; i++ {
		v := toolbarEase(float64(i) / 1000)
		if v < previous || v < 0 || v > 1 || math.IsNaN(v) {
			t.Fatal("invalid easing", i, v)
		}
		previous = v
	}
	// At Bezier parameter .5, x=.275 and y=.8.
	if math.Abs(toolbarEase(.275)-.8) > 1e-6 {
		t.Fatal("curve does not solve x for time")
	}
}

func TestToolbarSemanticPartsPreserveGeometry(t *testing.T) {
	for action, glyph := range toolbarGlyphs {
		var commands []toolbarGlyphCommand
		parts := toolbarGlyphParts(action, glyph)
		for _, part := range parts {
			if part.glyph.commands[0].op != toolbarGlyphMove {
				t.Fatal("split inside subpath", action)
			}
			commands = append(commands, part.glyph.commands...)
		}
		if !reflect.DeepEqual(commands, glyph.commands) {
			t.Fatal("split changed geometry", action)
		}
		if action == ActionCopy || action == ActionScroll {
			if len(parts) != 2 || parts[1].dx != 0 || parts[1].dy != 0 {
				t.Fatal("stationary part moved", action)
			}
		} else if len(parts) != 1 {
			t.Fatal("unexpected split", action)
		}
	}
}
