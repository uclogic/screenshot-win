package selector

import (
	"context"
	"fmt"
	"image"
	"sync"

	"screenshot-win/editor"
)

// Action is the operation selected from the post-selection toolbar.
type Action uint8

const (
	ActionCancel Action = iota
	ActionSave
	ActionCopy
	ActionScroll
	ActionSaveAs
	ActionPin
	ActionEdit
	ActionRectangle
	ActionArrow
	ActionText
	ActionColor
	ActionWidth
)

const (
	toolbarGap = 8
)

var (
	selectionToolbarActions  = []Action{ActionCancel, ActionScroll, ActionRectangle, ActionArrow, ActionText, ActionColor, ActionWidth, ActionPin, ActionSave, ActionCopy}
	annotationToolbarActions = []Action{ActionCancel, ActionRectangle, ActionArrow, ActionText, ActionColor, ActionWidth, ActionPin, ActionSave, ActionCopy}
	captureToolbarActions    = []Action{ActionCancel, ActionEdit, ActionPin, ActionSaveAs, ActionCopy}
)

// ToolbarEvent is emitted for both commands and explicit style selections.
// Change.Field is non-zero only for a color or width choice.
type ToolbarEvent struct {
	Action Action
	Style  editor.Style
	Change editor.StyleChange
}

// ActionToolbar is the persistent post-selection toolbar used while editing.
// NextEvent arms one user action at a time. Drawing requests stay active across
// placements; a different drawing tool is queued and dispatched as soon as the
// old tool exits.
type ActionToolbar struct {
	once        sync.Once
	window      uintptr
	events      <-chan ToolbarEvent
	ready       func()
	setStyle    func(editor.Style)
	closeWindow func()
	done        <-chan struct{}
	resultError func() error
}

func (toolbar *ActionToolbar) NextEvent(ctx context.Context) (ToolbarEvent, error) {
	if toolbar == nil || toolbar.events == nil {
		return ToolbarEvent{Action: ActionCancel}, fmt.Errorf("action toolbar is not available")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if toolbar.ready != nil {
		toolbar.ready()
	}
	select {
	case event := <-toolbar.events:
		return event, nil
	case <-toolbar.done:
		if toolbar.resultError != nil {
			if err := toolbar.resultError(); err != nil {
				return ToolbarEvent{Action: ActionCancel}, err
			}
		}
		if err := ctx.Err(); err != nil {
			return ToolbarEvent{Action: ActionCancel}, err
		}
		return ToolbarEvent{Action: ActionCancel}, nil
	case <-ctx.Done():
		return ToolbarEvent{Action: ActionCancel}, ctx.Err()
	}
}

// NextAction is the compatibility form for callers that do not consume style
// payloads. New annotation flows should use NextEvent.
func (toolbar *ActionToolbar) NextAction(ctx context.Context) (Action, error) {
	event, err := toolbar.NextEvent(ctx)
	return event.Action, err
}

// SetStyle refreshes the toolbar's visible current color and width. It is safe
// to call from the frozen overlay's selection-notification goroutine.
func (toolbar *ActionToolbar) SetStyle(style editor.Style) {
	if toolbar != nil && toolbar.setStyle != nil {
		toolbar.setStyle(style)
	}
}

func (toolbar *ActionToolbar) Close() {
	if toolbar == nil {
		return
	}
	toolbar.once.Do(func() {
		if toolbar.closeWindow != nil {
			toolbar.closeWindow()
		}
		if toolbar.done != nil {
			<-toolbar.done
		}
	})
}

// CaptureToolbar is the persistent action bar shown while long capture runs.
type CaptureToolbar struct {
	once                sync.Once
	window              uintptr
	actions             <-chan Action
	closeWindow         func()
	hideForCapture      func()
	restoreAfterCapture func()
	done                <-chan struct{}
}

// Actions emits the action that ended capture and then closes.
func (toolbar *CaptureToolbar) Actions() <-chan Action {
	if toolbar == nil {
		return nil
	}
	return toolbar.actions
}

// Close removes the toolbar and waits for its window resources to be released.
func (toolbar *CaptureToolbar) Close() {
	if toolbar == nil {
		return
	}
	toolbar.once.Do(func() {
		if toolbar.closeWindow != nil {
			toolbar.closeWindow()
		}
		if toolbar.done != nil {
			<-toolbar.done
		}
	})
}

// HideForCapture temporarily removes a capture toolbar that Windows could not
// exclude from capture. It is a no-op for a toolbar outside the capture region.
func (toolbar *CaptureToolbar) HideForCapture() {
	if toolbar != nil && toolbar.hideForCapture != nil {
		toolbar.hideForCapture()
	}
}

// RestoreAfterCapture shows a toolbar hidden by HideForCapture.
func (toolbar *CaptureToolbar) RestoreAfterCapture() {
	if toolbar != nil && toolbar.restoreAfterCapture != nil {
		toolbar.restoreAfterCapture()
	}
}

func toolbarBounds(region, workArea image.Rectangle, toolbarSize image.Point) image.Rectangle {
	x := region.Max.X - toolbarSize.X
	y := region.Max.Y + toolbarGap
	if y+toolbarSize.Y > workArea.Max.Y {
		y = region.Min.Y - toolbarGap - toolbarSize.Y
	}
	x = clampCoordinate(x, workArea.Min.X, workArea.Max.X-toolbarSize.X)
	y = clampCoordinate(y, workArea.Min.Y, workArea.Max.Y-toolbarSize.Y)
	return image.Rect(x, y, x+toolbarSize.X, y+toolbarSize.Y)
}

func toolbarActionAtActions(point image.Point, size image.Point, actions []Action) (Action, bool) {
	if point.X < 0 || point.Y < 0 || point.X >= size.X || point.Y >= size.Y {
		return ActionCancel, false
	}
	const padding = 4
	if len(actions) == 0 {
		return ActionCancel, false
	}
	buttonWidth := (size.X - padding*2) / len(actions)
	if buttonWidth <= 0 {
		return ActionCancel, false
	}
	if point.Y < padding || point.Y >= size.Y-padding || point.X < padding || point.X >= size.X-padding {
		return ActionCancel, false
	}
	index := (point.X - padding) / buttonWidth
	if index < 0 || index >= len(actions) {
		return ActionCancel, false
	}
	return actions[index], true
}

func toolbarActionEnabled(Action) bool { return true }

func stylePanelSize(field editor.StyleField, dpi int) image.Point {
	if dpi <= 0 {
		dpi = 96
	}
	scale := func(value int) int { return (value*dpi + 48) / 96 }
	switch field {
	case editor.StyleFieldColor:
		return image.Pt(scale(188), scale(44))
	case editor.StyleFieldWidth:
		return image.Pt(scale(152), scale(136))
	default:
		return image.Point{}
	}
}

func stylePanelOptionAt(point image.Point, field editor.StyleField, dpi int) (int, bool) {
	if dpi <= 0 {
		dpi = 96
	}
	scale := func(value int) int { return (value*dpi + 48) / 96 }
	padding := scale(4)
	switch field {
	case editor.StyleFieldColor:
		cell := scale(36)
		if point.Y < padding || point.Y >= padding+cell || point.X < padding || point.X >= padding+cell*len(editor.PresetColors()) {
			return 0, false
		}
		return (point.X - padding) / cell, true
	case editor.StyleFieldWidth:
		cell := scale(32)
		if point.X < padding || point.X >= scale(148) || point.Y < padding || point.Y >= padding+cell*len(editor.PresetWidths()) {
			return 0, false
		}
		return (point.Y - padding) / cell, true
	default:
		return 0, false
	}
}

func stylePanelBounds(anchor, workArea image.Rectangle, panelSize image.Point, gap int) image.Rectangle {
	x := anchor.Min.X
	y := anchor.Max.Y + gap
	if y+panelSize.Y > workArea.Max.Y {
		y = anchor.Min.Y - gap - panelSize.Y
	}
	x = clampCoordinate(x, workArea.Min.X, workArea.Max.X-panelSize.X)
	y = clampCoordinate(y, workArea.Min.Y, workArea.Max.Y-panelSize.Y)
	return image.Rect(x, y, x+panelSize.X, y+panelSize.Y)
}

func clampCoordinate(value, minimum, maximum int) int {
	if maximum < minimum {
		return minimum
	}
	return max(minimum, min(value, maximum))
}
