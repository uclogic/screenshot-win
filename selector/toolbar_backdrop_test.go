package selector

import (
	"image"
	"testing"
	"time"
)

func TestBackdropFetchDoesNotBlockUI(t *testing.T) {
	blocked := make(chan struct{})
	defer close(blocked)
	ready := make(chan struct{}, 1)
	bounds := image.Rect(-20, 10, 80, 50)
	want := image.NewRGBA(bounds)
	var fetch backdropFetch
	started := make(chan bool, 1)
	go func() {
		started <- fetch.start(func(got image.Rectangle) image.Image {
			<-blocked
			if got != bounds {
				return nil
			}
			return want
		}, bounds, func() { ready <- struct{}{} })
	}()
	select {
	case ok := <-started:
		if !ok {
			t.Fatal("request did not start")
		}
	case <-time.After(time.Second):
		t.Fatal("background request blocked the UI")
	}
	if _, ok := fetch.poll(); ok {
		t.Fatal("unfinished result returned")
	}
	if fetch.start(func(image.Rectangle) image.Image { return nil }, bounds, func() {}) {
		t.Fatal("started concurrent request for the same window")
	}
	blocked <- struct{}{}
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("completion did not notify the UI")
	}
	if got, ok := fetch.poll(); !ok || got != want {
		t.Fatal("completed background was lost")
	}
	if _, ok := fetch.poll(); ok {
		t.Fatal("result delivered twice")
	}
}
