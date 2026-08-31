package app

import (
	"context"
	"errors"
	"image"
	"testing"
	"time"

	"screenshot-win/editor"
	"screenshot-win/selector"
)

func TestRunActionMenuReturnsAfterSave(t *testing.T) {
	savedPath := ""
	result, err := runActionMenu(image.Rect(0, 0, 20, 10), image.NewRGBA(image.Rect(0, 0, 20, 10)), interactiveOperations{
		showToolbar: func(image.Rectangle) (selector.Action, error) { return selector.ActionSave, nil },
		choosePath:  func(time.Time) (string, bool, error) { return "shot.png", true, nil },
		save: func(path string, _ image.Image) error {
			savedPath = path
			return nil
		},
		copy:      func(image.Image) error { return nil },
		now:       time.Now,
		showError: func(error) { t.Fatal("unexpected error dialog") },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.action != selector.ActionSave || result.path != "shot.png" || savedPath != "shot.png" {
		t.Fatalf("unexpected result: %+v, saved path %q", result, savedPath)
	}
}

func TestRunActionMenuReturnsToToolbarAfterDialogCancel(t *testing.T) {
	actions := []selector.Action{selector.ActionSave, selector.ActionCopy}
	toolbarCalls := 0
	copyCalls := 0
	result, err := runActionMenu(image.Rectangle{}, image.NewRGBA(image.Rect(0, 0, 1, 1)), interactiveOperations{
		showToolbar: func(image.Rectangle) (selector.Action, error) {
			action := actions[toolbarCalls]
			toolbarCalls++
			return action, nil
		},
		choosePath: func(time.Time) (string, bool, error) { return "", false, nil },
		save:       func(string, image.Image) error { t.Fatal("save should not be called"); return nil },
		copy:       func(image.Image) error { copyCalls++; return nil },
		now:        time.Now,
		showError:  func(error) { t.Fatal("unexpected error dialog") },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.action != selector.ActionCopy || toolbarCalls != 2 || copyCalls != 1 {
		t.Fatalf("result=%+v toolbarCalls=%d copyCalls=%d", result, toolbarCalls, copyCalls)
	}
}

func TestRunActionMenuReturnsToToolbarAfterOperationFailure(t *testing.T) {
	actions := []selector.Action{selector.ActionCopy, selector.ActionScroll}
	toolbarCalls := 0
	shownErrors := 0
	result, err := runActionMenu(image.Rectangle{}, image.NewRGBA(image.Rect(0, 0, 1, 1)), interactiveOperations{
		showToolbar: func(image.Rectangle) (selector.Action, error) {
			action := actions[toolbarCalls]
			toolbarCalls++
			return action, nil
		},
		choosePath: func(time.Time) (string, bool, error) { return "", false, nil },
		save:       func(string, image.Image) error { return nil },
		copy:       func(image.Image) error { return errors.New("clipboard busy") },
		now:        time.Now,
		showError:  func(error) { shownErrors++ },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.action != selector.ActionScroll || shownErrors != 1 || toolbarCalls != 2 {
		t.Fatalf("result=%+v shownErrors=%d toolbarCalls=%d", result, shownErrors, toolbarCalls)
	}
}

func TestRunActionMenuPropagatesToolbarFailure(t *testing.T) {
	want := errors.New("toolbar failed")
	_, err := runActionMenu(image.Rectangle{}, image.NewRGBA(image.Rect(0, 0, 1, 1)), interactiveOperations{
		showToolbar: func(image.Rectangle) (selector.Action, error) { return selector.ActionCancel, want },
	})
	if !errors.Is(err, want) {
		t.Fatalf("runActionMenu() error = %v, want %v", err, want)
	}
}

func TestRunActionMenuPinsAtCaptureOrigin(t *testing.T) {
	region := image.Rect(-20, 30, 80, 90)
	var pinnedAt image.Point
	result, err := runActionMenu(region, image.NewRGBA(image.Rect(0, 0, 100, 60)), interactiveOperations{
		showToolbar: func(image.Rectangle) (selector.Action, error) { return selector.ActionPin, nil },
		pin:         func(_ image.Image, origin image.Point) error { pinnedAt = origin; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.action != selector.ActionPin || pinnedAt != region.Min {
		t.Fatalf("result=%+v pinnedAt=%v", result, pinnedAt)
	}
}

func TestRunActionMenuRetriesAfterPinFailure(t *testing.T) {
	actions := []selector.Action{selector.ActionPin, selector.ActionCancel}
	calls, errorsShown := 0, 0
	result, err := runActionMenu(image.Rectangle{}, image.NewRGBA(image.Rect(0, 0, 1, 1)), interactiveOperations{
		showToolbar: func(image.Rectangle) (selector.Action, error) { action := actions[calls]; calls++; return action, nil },
		pin:         func(image.Image, image.Point) error { return errors.New("pin failed") },
		showError:   func(error) { errorsShown++ },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.action != selector.ActionCancel || calls != 2 || errorsShown != 1 {
		t.Fatalf("result=%+v calls=%d errors=%d", result, calls, errorsShown)
	}
}

func TestRunActionMenuAnnotatesInPlaceThenReturnsToToolbar(t *testing.T) {
	actions := []selector.Action{selector.ActionArrow, selector.ActionSave}
	calls, annotations := 0, 0
	result, err := runActionMenu(image.Rectangle{}, image.NewRGBA(image.Rect(0, 0, 1, 1)), interactiveOperations{
		showToolbar: func(image.Rectangle) (selector.Action, error) { action := actions[calls]; calls++; return action, nil },
		choosePath:  func(time.Time) (string, bool, error) { return "marked.png", true, nil },
		save:        func(string, image.Image) error { return nil },
		annotate: func(tool editor.Tool, _ editor.Style) error {
			if tool != editor.ToolArrow {
				t.Fatalf("tool=%v", tool)
			}
			annotations++
			return nil
		},
		now: time.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.action != selector.ActionSave || calls != 2 || annotations != 1 {
		t.Fatalf("result=%+v calls=%d annotations=%d", result, calls, annotations)
	}
}

func TestRunActionMenuExportsRenderedAnnotations(t *testing.T) {
	marked := image.NewRGBA(image.Rect(0, 0, 3, 2))
	copied := image.Image(nil)
	result, err := runActionMenu(image.Rectangle{}, image.NewRGBA(image.Rect(0, 0, 1, 1)), interactiveOperations{
		showToolbar: func(image.Rectangle) (selector.Action, error) { return selector.ActionCopy, nil },
		copy:        func(source image.Image) error { copied = source; return nil },
		rendered:    func() image.Image { return marked },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.action != selector.ActionCopy || copied != marked {
		t.Fatalf("result=%+v copied=%T", result, copied)
	}
}

func TestRunActionMenuAppliesExplicitStyleAndUsesItForNextAnnotation(t *testing.T) {
	style := editor.DefaultStyle()
	style.Color = editor.PresetColors()[2]
	events := []selector.ToolbarEvent{
		{Action: selector.ActionColor, Style: style, Change: editor.StyleChange{Field: editor.StyleFieldColor, Style: style}},
		{Action: selector.ActionArrow, Style: style},
		{Action: selector.ActionCopy, Style: style},
	}
	call := 0
	var applied editor.StyleChange
	var annotated editor.Style
	result, err := runActionMenu(image.Rectangle{}, image.NewRGBA(image.Rect(0, 0, 5, 5)), interactiveOperations{
		showToolbarEvent: func(image.Rectangle) (selector.ToolbarEvent, error) {
			event := events[call]
			call++
			return event, nil
		},
		updateSelectedStyle: func(_ context.Context, change editor.StyleChange) (bool, error) {
			applied = change
			return true, nil
		},
		annotate: func(tool editor.Tool, got editor.Style) error {
			if tool != editor.ToolArrow {
				t.Fatalf("tool = %v", tool)
			}
			annotated = got
			return nil
		},
		copy: func(image.Image) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.action != selector.ActionCopy || applied.Field != editor.StyleFieldColor || annotated != style {
		t.Fatalf("result=%+v applied=%+v annotated=%+v", result, applied, annotated)
	}
}
