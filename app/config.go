package app

import (
	"fmt"
	"time"

	"screenshot-win"
)

// LongCaptureImplementation selects the scrolling-capture engine. The zero
// value intentionally selects the bidirectional implementation.
type LongCaptureImplementation uint8

const (
	LongCaptureBidirectional LongCaptureImplementation = iota
	LongCaptureLegacy
)

func (implementation LongCaptureImplementation) valid() bool {
	return implementation == LongCaptureBidirectional || implementation == LongCaptureLegacy
}

func (implementation LongCaptureImplementation) String() string {
	if implementation == LongCaptureLegacy {
		return "legacy"
	}
	if implementation == LongCaptureBidirectional {
		return "bidirectional"
	}
	return fmt.Sprintf("unknown(%d)", implementation)
}

// Config contains the inputs for one capture session.
type Config struct {
	X, Y                      int
	Width, Height             int
	OutputPath                string
	Interval                  time.Duration
	MatchOptions              screenshotwin.MatchOptions
	DiagnosticDir             string
	DiagnosticMax             int
	Interactive               bool
	LongCaptureImplementation LongCaptureImplementation
}

func (config Config) Validate() error {
	if config.Interval <= 0 {
		return fmt.Errorf("interval must be positive")
	}
	if config.DiagnosticMax < 0 {
		return fmt.Errorf("diagnostic limit must not be negative")
	}
	if !config.Interactive && (config.Width <= 0 || config.Height <= 0) {
		return fmt.Errorf("width and height must be positive")
	}
	if !config.LongCaptureImplementation.valid() {
		return fmt.Errorf("unknown long capture implementation %d", config.LongCaptureImplementation)
	}
	return config.MatchOptions.Validate()
}
