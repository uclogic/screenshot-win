package app

import (
	"fmt"
	"time"

	"screenshot-win"
	"screenshot-win/selector"
)

// LongCaptureImplementation selects the scrolling-capture engine. The zero
// value intentionally selects the more stable one-way implementation.
type LongCaptureImplementation uint8

const (
	LongCaptureLegacy LongCaptureImplementation = iota
	LongCaptureBidirectional
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
	Interval                  time.Duration
	MatchOptions              screenshotwin.MatchOptions
	DiagnosticDir             string
	DiagnosticMax             int
	CandidateMode             selector.CandidateMode
	LongCaptureImplementation LongCaptureImplementation
}

func (config Config) Validate() error {
	if config.Interval <= 0 {
		return fmt.Errorf("interval must be positive")
	}
	if config.DiagnosticMax < 0 {
		return fmt.Errorf("diagnostic limit must not be negative")
	}
	if !config.CandidateMode.Valid() {
		return fmt.Errorf("unknown candidate mode %d", config.CandidateMode)
	}
	if !config.LongCaptureImplementation.valid() {
		return fmt.Errorf("unknown long capture implementation %d", config.LongCaptureImplementation)
	}
	return config.MatchOptions.Validate()
}
