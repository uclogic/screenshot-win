package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestTrayControllerRejectsRepeatedTriggerAndRecovers(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	controller := newTrayController(func(context.Context) error {
		calls.Add(1)
		<-release
		return nil
	}, nil)
	if !controller.Trigger() || controller.Trigger() {
		t.Fatal("first trigger should start and repeated trigger should be ignored")
	}
	close(release)
	waitUntil(t, func() bool {
		controller.mu.Lock()
		defer controller.mu.Unlock()
		return !controller.running
	})
	if !controller.Trigger() {
		t.Fatal("controller did not accept a trigger after capture completed")
	}
	waitUntil(t, func() bool { return calls.Load() == 2 })
	if got := calls.Load(); got != 2 {
		t.Fatalf("capture calls = %d, want 2", got)
	}
}

func TestTrayControllerShutdownCancelsAndIsIdempotent(t *testing.T) {
	started := make(chan struct{})
	controller := newTrayController(func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}, nil)
	controller.Trigger()
	<-started
	<-controller.Shutdown()
	<-controller.Shutdown()
	if controller.Trigger() {
		t.Fatal("controller accepted a trigger after shutdown")
	}
}

func TestTrayControllerReportsWorkerErrorAndRecovers(t *testing.T) {
	want := errors.New("capture failed")
	reported := make(chan error, 1)
	controller := newTrayController(func(context.Context) error { return want }, func(err error) { reported <- err })
	controller.Trigger()
	select {
	case err := <-reported:
		if !errors.Is(err, want) {
			t.Fatalf("reported error = %v, want %v", err, want)
		}
	case <-time.After(time.Second):
		t.Fatal("worker error was not reported")
	}
	waitUntil(t, func() bool {
		controller.mu.Lock()
		defer controller.mu.Unlock()
		return !controller.running
	})
	if !controller.Trigger() {
		t.Fatal("controller did not recover after worker error")
	}
}

func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition was not satisfied")
		}
		time.Sleep(time.Millisecond)
	}
}
