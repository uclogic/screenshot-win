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
	}, nil, nil)
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
	var cleaned atomic.Bool
	controller := newTrayController(func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}, nil, func() { cleaned.Store(true) })
	controller.Trigger()
	<-started
	<-controller.Shutdown()
	<-controller.Shutdown()
	if !cleaned.Load() {
		t.Fatal("shutdown completed before cleanup")
	}
	if controller.Trigger() {
		t.Fatal("controller accepted a trigger after shutdown")
	}
}

func TestTrayControllerReportsWorkerErrorAndRecovers(t *testing.T) {
	want := errors.New("capture failed")
	reported := make(chan error, 1)
	var cleaned atomic.Bool
	controller := newTrayController(func(context.Context) error { return want }, func(err error) {
		if !cleaned.Load() {
			reported <- errors.New("error was reported before cleanup")
			return
		}
		reported <- err
	}, func() { cleaned.Store(true) })
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

func TestTrayControllerCleansUpBeforeBecomingIdle(t *testing.T) {
	cleanupStarted := make(chan struct{})
	cleanupRelease := make(chan struct{})
	controller := newTrayController(func(context.Context) error { return nil }, nil, func() {
		close(cleanupStarted)
		<-cleanupRelease
	})
	if !controller.Trigger() {
		t.Fatal("trigger was rejected")
	}
	<-cleanupStarted
	if controller.Trigger() {
		t.Fatal("controller accepted a trigger while cleanup was running")
	}
	shutdown := controller.Shutdown()
	select {
	case <-shutdown:
		t.Fatal("shutdown completed before cleanup")
	default:
	}
	close(cleanupRelease)
	select {
	case <-shutdown:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not complete after cleanup")
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

func TestTrayControllerSerializesCaptureAndPin(t *testing.T) {
	release := make(chan struct{})
	controller := newTrayController(func(context.Context) error { <-release; return nil }, nil, nil)
	if !controller.Trigger() {
		t.Fatal("capture did not start")
	}
	if controller.TriggerTask(func(context.Context) error { t.Error("pin ran during capture"); return nil }) {
		t.Fatal("pin was accepted during capture")
	}
	close(release)
	waitUntil(t, func() bool { controller.mu.Lock(); defer controller.mu.Unlock(); return !controller.running })
	pinStarted := make(chan struct{})
	if !controller.TriggerTask(func(ctx context.Context) error { close(pinStarted); <-ctx.Done(); return ctx.Err() }) {
		t.Fatal("pin did not start")
	}
	<-pinStarted
	if controller.Trigger() {
		t.Fatal("capture was accepted during pin")
	}
	<-controller.Shutdown()
	if controller.TriggerTask(func(context.Context) error { return nil }) {
		t.Fatal("pin accepted after shutdown")
	}
}
