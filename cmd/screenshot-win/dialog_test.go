package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestChoosePNGPathContextRejectsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := choosePNGPathContext(ctx, 0, time.Time{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("choosePNGPathContext() error = %v, want context canceled", err)
	}
}

func TestSuggestedScreenshotName(t *testing.T) {
	now := time.Date(2026, time.August, 14, 1, 2, 3, 0, time.Local)
	if got, want := suggestedScreenshotName(now), "Screenshot_20260814_010203.png"; got != want {
		t.Fatalf("suggestedScreenshotName() = %q, want %q", got, want)
	}
}
