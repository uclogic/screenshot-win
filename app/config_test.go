package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"screenshot-win"
)

func TestRunnerRunContextRejectsCancelledContextWithoutStartingSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	application := New()
	runner := NewRunner(application, Runtime{})
	err := runner.RunContext(ctx, Config{Interactive: true, Interval: time.Millisecond, MatchOptions: screenshotwin.DefaultMatchOptions()})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunContext() error = %v, want context canceled", err)
	}
	if got := application.State(); got != StateIdle {
		t.Fatalf("state = %v, want idle", got)
	}
}

func validConfig() Config {
	return Config{
		Width:         800,
		Height:        600,
		Interval:      100 * time.Millisecond,
		MatchOptions:  screenshotwin.DefaultMatchOptions(),
		DiagnosticMax: 50,
	}
}

func TestConfigValidate(t *testing.T) {
	config := validConfig()
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	config.Interactive = true
	config.Width = 0
	config.Height = 0
	if err := config.Validate(); err != nil {
		t.Fatalf("interactive config with no region failed: %v", err)
	}
}

func TestConfigValidateRejectsUnsafeRuntimeValues(t *testing.T) {
	tests := []Config{
		func() Config { config := validConfig(); config.Interval = 0; return config }(),
		func() Config { config := validConfig(); config.DiagnosticMax = -1; return config }(),
		func() Config { config := validConfig(); config.Width = 0; return config }(),
		func() Config { config := validConfig(); config.LongCaptureImplementation = 99; return config }(),
	}
	for _, config := range tests {
		if err := config.Validate(); err == nil {
			t.Errorf("Config.Validate() succeeded for %+v", config)
		}
	}
}

func TestLongCaptureImplementationDefaultsToBidirectional(t *testing.T) {
	config := validConfig()
	if config.LongCaptureImplementation != LongCaptureBidirectional {
		t.Fatalf("default implementation = %v, want bidirectional", config.LongCaptureImplementation)
	}
	if LongCaptureLegacy.String() != "legacy" || LongCaptureBidirectional.String() != "bidirectional" {
		t.Fatal("unexpected long capture implementation names")
	}
}
