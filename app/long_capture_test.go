package app

import (
	"errors"
	"image"
	"testing"
	"time"

	"screenshot-win/selector"
)

type fakeLongCapturePreview struct {
	err       error
	updates   int
	closeCall int
}

type fakeLongCaptureVisibility struct {
	hidden   bool
	hideCall int
	showCall int
}

func (visibility *fakeLongCaptureVisibility) HideForCapture() {
	visibility.hidden = true
	visibility.hideCall++
}

func (visibility *fakeLongCaptureVisibility) RestoreAfterCapture() {
	visibility.hidden = false
	visibility.showCall++
}

func (preview *fakeLongCapturePreview) Update(image.Image) error {
	preview.updates++
	return preview.err
}

func (preview *fakeLongCapturePreview) Close() { preview.closeCall++ }

func TestDecideLongCaptureActionResumesAfterSaveDialogCancel(t *testing.T) {
	decision, err := decideLongCaptureAction(selector.ActionSaveAs, 123, time.Unix(10, 0), func(owner uintptr, now time.Time) (string, bool, error) {
		if owner != 123 || !now.Equal(time.Unix(10, 0)) {
			t.Fatalf("chooser received owner=%d, now=%v", owner, now)
		}
		return "", false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.finish || decision.action != selector.ActionCancel || decision.path != "" {
		t.Fatalf("unexpected decision after dialog cancel: %+v", decision)
	}
}

func TestDecideLongCaptureActionFinishesAfterSavePathSelected(t *testing.T) {
	decision, err := decideLongCaptureAction(selector.ActionSaveAs, 0, time.Time{}, func(uintptr, time.Time) (string, bool, error) {
		return "shot.png", true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.finish || decision.action != selector.ActionSaveAs || decision.path != "shot.png" {
		t.Fatalf("unexpected decision after choosing path: %+v", decision)
	}
}

func TestDecideLongCaptureActionStopsImmediatelyForEdit(t *testing.T) {
	decision, err := decideLongCaptureAction(selector.ActionEdit, 0, time.Time{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.finish || decision.action != selector.ActionEdit {
		t.Fatalf("unexpected edit decision: %+v", decision)
	}
}

func TestRefreshLongCapturePreviewDisablesFailedPreview(t *testing.T) {
	want := errors.New("render failed")
	preview := &fakeLongCapturePreview{err: want}
	remaining, err := refreshLongCapturePreview(preview, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	if !errors.Is(err, want) || remaining != nil {
		t.Fatalf("refreshLongCapturePreview() = (%v, %v), want (nil, %v)", remaining, err, want)
	}
	if preview.updates != 1 || preview.closeCall != 1 {
		t.Fatalf("updates=%d closes=%d, want 1 each", preview.updates, preview.closeCall)
	}
}

func TestRefreshLongCapturePreviewKeepsSuccessfulPreview(t *testing.T) {
	preview := &fakeLongCapturePreview{}
	remaining, err := refreshLongCapturePreview(preview, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	if err != nil || remaining != preview || preview.closeCall != 0 {
		t.Fatalf("refreshLongCapturePreview() = (%v, %v), closes=%d", remaining, err, preview.closeCall)
	}
}

func TestCaptureWithLongCaptureOverlaysHidesAndRestoresWindows(t *testing.T) {
	preview := &fakeLongCaptureVisibility{}
	toolbar := &fakeLongCaptureVisibility{}
	want := errors.New("capture failed")
	image, err := captureWithLongCaptureOverlays(func() (*image.RGBA, error) {
		if !preview.hidden || !toolbar.hidden {
			t.Fatal("overlays must be hidden while capturing")
		}
		return nil, want
	}, preview, toolbar, struct{}{})
	if image != nil || !errors.Is(err, want) {
		t.Fatalf("captureWithLongCaptureOverlays() = (%v, %v), want (nil, %v)", image, err, want)
	}
	if preview.hidden || toolbar.hidden || preview.hideCall != 1 || preview.showCall != 1 || toolbar.hideCall != 1 || toolbar.showCall != 1 {
		t.Fatalf("overlays not restored: preview=%+v toolbar=%+v", preview, toolbar)
	}
}
