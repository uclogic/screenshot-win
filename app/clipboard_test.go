package app

import (
	"context"
	"errors"
	"image"
	"testing"

	"screenshot-win/capture"
)

func TestPinClipboardImageAndText(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 3, 2))
	origin := image.Pt(-50, 100)
	for _, useText := range []bool{false, true} {
		rendered, pinned := false, false
		err := pinClipboard(context.Background(), origin, func() (capture.ClipboardContent, error) {
			if useText {
				return capture.ClipboardContent{Text: "中文"}, nil
			}
			return capture.ClipboardContent{Image: source, Text: "ignored"}, nil
		}, func(text string) (image.Image, error) {
			rendered = true
			if text != "中文" {
				t.Fatal(text)
			}
			return source, nil
		}, func(got image.Image, at image.Point) error {
			pinned = true
			if got != source || at != origin {
				t.Fatal("pin content or origin changed")
			}
			return nil
		})
		if err != nil || !pinned || rendered != useText {
			t.Fatalf("unexpected outcome rendered=%v pinned=%v err=%v", rendered, pinned, err)
		}
	}
}

func TestPinClipboardFailuresAndCancellation(t *testing.T) {
	failure := errors.New("failure")
	for _, stage := range []string{"read", "render", "pin", "cancel-before", "cancel-after-read", "cancel-after-render"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if stage == "cancel-before" {
				cancel()
			}
			readCalled, pinCalled := false, false
			err := pinClipboard(ctx, image.Point{}, func() (capture.ClipboardContent, error) {
				readCalled = true
				if stage == "read" {
					return capture.ClipboardContent{}, failure
				}
				if stage == "cancel-after-read" {
					cancel()
				}
				return capture.ClipboardContent{Text: "text"}, nil
			}, func(string) (image.Image, error) {
				if stage == "render" {
					return nil, failure
				}
				if stage == "cancel-after-render" {
					cancel()
				}
				return image.NewRGBA(image.Rect(0, 0, 1, 1)), nil
			}, func(image.Image, image.Point) error { pinCalled = true; return failure })
			if err == nil {
				t.Fatal("failure ignored")
			}
			if stage == "cancel-before" && readCalled {
				t.Fatal("read clipboard after cancellation")
			}
			if stage != "pin" && pinCalled {
				t.Fatal("created pin after error or cancellation")
			}
		})
	}
}
