package selector

import (
	"context"
	"errors"
	"image"
	"testing"
	"time"
)

func TestPinEditPreservesSourceAndScreenBounds(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 400, 200))
	bounds := image.Rect(-40, 50, 160, 150)
	for _, cancelled := range []bool{false, true} {
		notified := make(chan struct{}, 1)
		session := startPinEdit(func(ctx context.Context, got image.Image, at image.Rectangle) (image.Image, error) {
			if got != source || at != bounds {
				t.Error("pin edit resampled the original image or changed its screen bounds")
			}
			if cancelled {
				return nil, nil
			}
			return image.NewRGBA(source.Bounds()), nil
		}, source, bounds, func() { notified <- struct{}{} })
		select {
		case <-notified:
		case <-time.After(2 * time.Second):
			t.Fatal("pin edit did not notify its window")
		}
		session.close()
		result := <-session.result
		if result.err != nil || (result.image == nil) != cancelled {
			t.Fatalf("wrong edit completion: %+v", result)
		}
	}
}

func TestClosingPinCancelsEditorAndDiscardsLateResult(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 5, 5))
	notified := make(chan struct{}, 1)
	started := make(chan struct{})
	session := startPinEdit(func(ctx context.Context, _ image.Image, _ image.Rectangle) (image.Image, error) {
		close(started)
		<-ctx.Done()
		return source, nil // A late successful result must not revive a closed pin.
	}, source, source.Bounds(), func() { notified <- struct{}{} })
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("editor did not start")
	}
	session.close()
	result := <-session.result
	if result.image != nil || !errors.Is(result.err, context.Canceled) {
		t.Fatalf("closing pin accepted a late result: %+v", result)
	}
	select {
	case <-notified:
		t.Fatal("posted completion to a closed pin window")
	default:
	}
}
