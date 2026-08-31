package app

import (
	"fmt"
	"time"

	"screenshot-win"
)

// Config contains the inputs for one capture session.
type Config struct {
	X, Y          int
	Width, Height int
	OutputPath    string
	Interval      time.Duration
	MatchOptions  screenshotwin.MatchOptions
	DiagnosticDir string
	DiagnosticMax int
	Interactive   bool
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
	return config.MatchOptions.Validate()
}
