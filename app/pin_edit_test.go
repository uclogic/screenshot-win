package app

import (
	"context"
	"errors"
	"image"
	"testing"
	"time"

	"screenshot-win/selector"
)

func TestPinEditCompletionAndCancellation(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 200, 100))
	output := image.NewRGBA(source.Bounds())
	// Screen bounds can be zoomed or partially outside the desktop; source and
	// output still have their full, original pixel dimensions.
	bounds := image.Rect(-25, 30, 75, 80)
	for _, action := range []selector.Action{selector.ActionPin, selector.ActionCopy, selector.ActionSave, selector.ActionCancel} {
		coordinator := New()
		got, err := runPinEdit(context.Background(), coordinator, source, bounds, func(context.Context) (selector.Action, image.Image, error) {
			if coordinator.State() != StateEditing {
				t.Fatal("pin editor did not acquire an exclusive editing session")
			}
			if _, err := coordinator.Begin(StateSelecting); !errors.Is(err, ErrSessionActive) {
				t.Fatal("capture was allowed during pin editing")
			}
			return action, output, nil
		})
		if err != nil || coordinator.State() != StateIdle {
			t.Fatalf("completion failed to release session: %v", err)
		}
		if action == selector.ActionCancel && got != nil || action != selector.ActionCancel && got != output {
			t.Fatalf("action %v returned wrong edit result", action)
		}
	}
}

func TestPinEditFailuresRetainOriginalAndReleaseSession(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 100, 50))
	for _, stage := range []string{"busy", "error", "cancel-before", "cancel-after", "resized", "missing-output"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			coordinator := New()
			if stage == "busy" {
				session, _ := coordinator.Begin(StateSelecting)
				defer session.Finish()
			}
			if stage == "cancel-before" {
				cancel()
			}
			called := false
			got, err := runPinEdit(ctx, coordinator, source, source.Bounds(), func(context.Context) (selector.Action, image.Image, error) {
				called = true
				if stage == "error" {
					return selector.ActionCancel, nil, errors.New("editor failed")
				}
				if stage == "cancel-after" {
					cancel()
				}
				var output image.Image = source
				if stage == "resized" {
					output = image.NewRGBA(image.Rect(0, 0, 50, 25))
				} else if stage == "missing-output" {
					output = nil
				}
				return selector.ActionPin, output, nil
			})
			if got != nil || err == nil {
				t.Fatal("failure returned an image that could replace the original")
			}
			if (stage == "busy" || stage == "cancel-before") && called {
				t.Fatal("opened pin editor despite busy host/cancellation")
			}
			if stage == "busy" {
				if coordinator.State() != StateSelecting {
					t.Fatal("rejected pin edit released another capture session")
				}
			} else if coordinator.State() != StateIdle {
				t.Fatal("failed pin edit retained its session")
			}
		})
	}
}

func TestDispatchPinEditRejectsBusyHost(t *testing.T) {
	called := false
	output, err := dispatchPinEdit(context.Background(), func(func(context.Context) error) bool { return false }, func(context.Context) (image.Image, error) {
		called = true
		return nil, nil
	})
	if called || output != nil || err != nil {
		t.Fatal("busy host opened or applied a pin edit")
	}
}

func TestDispatchPinEditCancelsAndWaitsForCleanup(t *testing.T) {
	for _, closePin := range []bool{false, true} {
		t.Run(map[bool]string{false: "host shutdown", true: "pin closed"}[closePin], func(t *testing.T) {
			pinCtx, cancelPin := context.WithCancel(context.Background())
			hostCtx, cancelHost := context.WithCancel(context.Background())
			defer cancelPin()
			defer cancelHost()
			started, cleaned := make(chan struct{}), make(chan struct{})
			finished := make(chan error, 1)
			go func() {
				_, err := dispatchPinEdit(pinCtx, func(task func(context.Context) error) bool { go task(hostCtx); return true }, func(ctx context.Context) (image.Image, error) {
					close(started)
					<-ctx.Done()
					close(cleaned)
					return nil, ctx.Err()
				})
				finished <- err
			}()
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("pin editor did not start")
			}
			if closePin {
				cancelPin()
			} else {
				cancelHost()
			}
			select {
			case err := <-finished:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation = %v", err)
				}
				select {
				case <-cleaned:
				default:
					t.Fatal("returned before editor cleanup")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("editor did not stop after cancellation")
			}
		})
	}
}
