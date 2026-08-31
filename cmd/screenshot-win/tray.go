package main

import (
	"context"
	"errors"
	"sync"
)

type trayController struct {
	mu       sync.Mutex
	run      func(context.Context) error
	report   func(error)
	cleanup  func()
	running  bool
	exiting  bool
	cancel   context.CancelFunc
	finished chan struct{}
}

func newTrayController(run func(context.Context) error, report func(error), cleanup func()) *trayController {
	return &trayController{run: run, report: report, cleanup: cleanup}
}

// Trigger starts a capture if the host is idle. Repeated triggers are ignored.
func (controller *trayController) Trigger() bool {
	controller.mu.Lock()
	if controller.exiting || controller.running || controller.run == nil {
		controller.mu.Unlock()
		return false
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	controller.running = true
	controller.cancel = cancel
	controller.finished = done
	controller.mu.Unlock()

	go func() {
		err := controller.run(ctx)
		if controller.cleanup != nil {
			controller.cleanup()
		}
		controller.mu.Lock()
		report := controller.report
		exiting := controller.exiting
		controller.mu.Unlock()
		if err != nil && !errors.Is(err, context.Canceled) && !exiting && report != nil {
			report(err)
		}
		controller.mu.Lock()
		controller.running = false
		controller.cancel = nil
		close(done)
		controller.mu.Unlock()
	}()
	return true
}

// Shutdown prevents new captures, cancels the active capture, and returns a
// channel that closes after the worker has released its resources.
func (controller *trayController) Shutdown() <-chan struct{} {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	controller.exiting = true
	if controller.cancel != nil {
		controller.cancel()
	}
	if controller.finished != nil && controller.running {
		return controller.finished
	}
	done := make(chan struct{})
	close(done)
	return done
}
