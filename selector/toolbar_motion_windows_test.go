//go:build windows

package selector

import (
	"testing"
	"time"

	"screenshot-win/editor"
)

func TestPanelMotionHoverPressAndLeave(t *testing.T) {
	for _, panel := range []stylePanel{
		{format: true},
		{field: editor.StyleFieldColor},
		{field: editor.StyleFieldWidth},
	} {
		state := &toolbarState{panel: panel, style: editor.DefaultStyle()}
		state.panel.hoverIndex, state.panel.pressedIndex = -1, -1
		selected := state.currentPanelIndex()
		target := (selected + 1) % state.panelOptionCount()
		now := time.Unix(100, 0)
		frames, active := state.panelMotionFrames(now)
		if active || frames[selected].selected != 1 || frames[target].scale != 1 {
			t.Fatalf("initial panel state: frames=%+v active=%v", frames, active)
		}
		state.panel.hoverIndex = target
		state.panelMotionFrames(now)
		now = now.Add(toolbarHoverDuration)
		frames, active = state.panelMotionFrames(now)
		if active || frames[target].scale != 1.08 || frames[target].y != -1 || frames[target].hover != 1 {
			t.Fatalf("hover endpoint: %+v active=%v", frames[target], active)
		}
		// Switching presentation must not reset an in-flight interaction.
		state.glassEnabled = true
		state.panel.pressedIndex = target
		before, _ := state.panelMotionFrames(now)
		state.glassEnabled = false
		after, _ := state.panelMotionFrames(now)
		if before[target] != after[target] {
			t.Fatal("glass fallback reset panel motion")
		}
		now = now.Add(toolbarPressDuration)
		frames, _ = state.panelMotionFrames(now)
		if frames[target].scale != .94 || frames[target].press != 1 {
			t.Fatalf("press endpoint: %+v", frames[target])
		}
		state.panel.hoverIndex, state.panel.pressedIndex = -1, -1
		state.panelMotionFrames(now)
		frames, active = state.panelMotionFrames(now.Add(toolbarHoverDuration))
		if active || frames[target].scale != 1 || frames[target].y != 0 || frames[target].hover != 0 || frames[target].press != 0 || frames[selected].selected != 1 {
			t.Fatalf("leave endpoint: frames=%+v active=%v", frames, active)
		}
	}
}
