package app

import (
	"context"
	"errors"
	"image"
	"io"
	"time"

	"screenshot-win/selector"
)

// Runtime supplies the platform-owned operations used by capture workflows.
// Keeping these at the boundary lets the CLI and tray host share Runner.
type Runtime struct {
	ChoosePNGPath        func(owner uintptr, now time.Time) (path string, selected bool, err error)
	ChoosePNGPathContext func(context.Context, uintptr, time.Time) (path string, selected bool, err error)
	ShowError            func(owner uintptr, err error)
	Stdout               io.Writer
	Stderr               io.Writer
	Now                  func() time.Time
}

// Runner executes capture workflows using one shared session coordinator.
type Runner struct {
	coordinator *App
	runtime     Runtime
	pins        *selector.PinManager
}

func NewRunner(coordinator *App, runtime Runtime) *Runner {
	if coordinator == nil {
		coordinator = New()
	}
	if runtime.Stdout == nil {
		runtime.Stdout = io.Discard
	}
	if runtime.Stderr == nil {
		runtime.Stderr = io.Discard
	}
	if runtime.Now == nil {
		runtime.Now = time.Now
	}
	return &Runner{coordinator: coordinator, runtime: runtime, pins: selector.NewPinManager()}
}

// Run performs one interactive or coordinate capture session.
func (runner *Runner) Run(config Config) error {
	err := runner.RunContext(context.Background(), config)
	if err == nil && runner != nil && runner.pins != nil {
		runner.pins.Wait()
	}
	return err
}

// RunContext performs one capture session and cancels it when ctx is done.
func (runner *Runner) RunContext(ctx context.Context, config Config) error {
	if runner == nil || runner.coordinator == nil {
		return errors.New("capture runner is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := config.Validate(); err != nil {
		return err
	}
	if config.Interactive {
		return runner.runInteractive(ctx, config)
	}
	session, err := runner.coordinator.Begin(StateScrolling)
	if err != nil {
		return err
	}
	defer session.Finish()
	return runner.runLongCapture(ctx, config, true, session)
}

func (runner *Runner) State() State {
	if runner == nil || runner.coordinator == nil {
		return StateIdle
	}
	return runner.coordinator.State()
}

// ClosePinnedImages closes all desktop pins owned by this runner. It is used
// when a long-lived host exits; finishing a capture session does not call it.
func (runner *Runner) ClosePinnedImages() {
	if runner != nil && runner.pins != nil {
		runner.pins.CloseAll()
	}
}

func (runner *Runner) pinImage(source image.Image, origin image.Point) error {
	if runner == nil || runner.pins == nil {
		return errors.New("pin manager is nil")
	}
	_, err := runner.pins.Show(source, origin)
	return err
}

func (runner *Runner) choosePNGPath(ctx context.Context, owner uintptr, now time.Time) (string, bool, error) {
	if runner.runtime.ChoosePNGPathContext != nil {
		return runner.runtime.ChoosePNGPathContext(ctx, owner, now)
	}
	if runner.runtime.ChoosePNGPath != nil {
		return runner.runtime.ChoosePNGPath(owner, now)
	}
	return "", false, errors.New("runtime operation \"choose PNG path\" is not configured")
}
