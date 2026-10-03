//go:build windows

package selector

import (
	"image"

	"screenshot-win/editor"
)

func (state *toolbarState) panelOptionCount() int {
	if state.panel.format {
		return len(arrowFormats)
	}
	if state.panel.field == editor.StyleFieldWidth {
		return len(editor.PresetWidths())
	}
	return len(editor.PresetColors())
}

func (state *toolbarState) drawToolbarIcon(renderer *toolbarIconRenderer, action Action, button image.Rectangle, enabled bool) {
	displayed := action
	if action == ActionArrow {
		displayed = currentArrowAction(state.arrowAction)
	}
	renderer.draw(displayed, button, enabled, state.style, state.dpi)
}
