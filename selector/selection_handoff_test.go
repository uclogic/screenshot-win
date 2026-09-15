package selector

import (
	"errors"
	"image"
	"testing"
	"time"
)

func TestSelectionHandoffKeepsCallerAvailableAndPublishesResult(t *testing.T) {
	var h selectionHandoff
	region := image.Rect(10, 20, 100, 120)
	entered := make(chan image.Rectangle, 1)
	uiReply := make(chan struct{})
	defer close(uiReply)
	ready := make(chan struct{}, 1)
	wantErr := errors.New("window preparation failed")
	var published image.Rectangle
	h.start(region, func(got image.Rectangle) error {
		entered <- got
		// Simulate window creation waiting for the caller to process a message.
		<-uiReply
		published = got
		return wantErr
	}, func() { ready <- struct{}{} })
	select {
	case got := <-entered:
		if got != region {
			t.Fatalf("region = %v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("preparation did not start")
	}
	if !h.pending() {
		t.Fatal("handoff ended before preparation")
	}
	h.cancelled = true
	h.start(region, func(image.Rectangle) error {
		t.Error("duplicate confirmation started another window")
		return nil
	}, func() {})
	uiReply <- struct{}{}
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("preparation did not finish after caller serviced its request")
	}
	if err := h.finish(); err != wantErr {
		t.Fatalf("error = %v", err)
	}
	if h.pending() || !h.cancelled || published != region {
		t.Fatalf("handoff lost cancellation or callback writes: %+v, %v", h, published)
	}
}
