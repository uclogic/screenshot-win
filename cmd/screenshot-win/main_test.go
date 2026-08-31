package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestParseArgumentsPreservesOriginalCLI(t *testing.T) {
	config, err := parseArguments([]string{"-100", "20", "1200", "800", "page.png"})
	if err != nil {
		t.Fatal(err)
	}
	if config.X != -100 || config.Y != 20 || config.Width != 1200 || config.Height != 800 {
		t.Fatalf("unexpected region: %+v", config)
	}
	if config.OutputPath != "page.png" || config.Interval != defaultCaptureInterval {
		t.Fatalf("unexpected defaults: %+v", config)
	}
}

func TestParseArgumentsUsesInteractiveSelectionWithoutCoordinates(t *testing.T) {
	config, err := parseArguments(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !config.Interactive || config.OutputPath != "result.png" {
		t.Fatalf("unexpected interactive defaults: %+v", config)
	}
}

func TestParseArgumentsUsesInteractiveSelectionWithFlags(t *testing.T) {
	config, err := parseArguments([]string{"--interval", "250ms", "--diagnostics", "events"})
	if err != nil {
		t.Fatal(err)
	}
	if !config.Interactive || config.Interval != 250*time.Millisecond || config.DiagnosticDir != "events" {
		t.Fatalf("flags were not applied to interactive mode: %+v", config)
	}
}

func TestParseLaunchArgumentsEnablesTrayWithCaptureTemplate(t *testing.T) {
	options, err := parseLaunchArguments([]string{"--tray", "--interval", "250ms", "--diagnostics", "events"})
	if err != nil {
		t.Fatal(err)
	}
	if !options.Tray || !options.Config.Interactive || options.Config.Interval != 250*time.Millisecond || options.Config.DiagnosticDir != "events" {
		t.Fatalf("unexpected tray options: %+v", options)
	}
}

func TestParseLaunchArgumentsDefaultsToTrayWithoutCoordinates(t *testing.T) {
	options, err := parseLaunchArguments(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !options.Tray || !options.Config.Interactive {
		t.Fatalf("unexpected default options: %+v", options)
	}
}

func TestParseLaunchArgumentsOncePreservesSingleInteractiveCapture(t *testing.T) {
	options, err := parseLaunchArguments([]string{"--once", "--interval", "250ms"})
	if err != nil {
		t.Fatal(err)
	}
	if options.Tray || !options.Config.Interactive || options.Config.Interval != 250*time.Millisecond {
		t.Fatalf("unexpected once options: %+v", options)
	}
}

func TestParseLaunchArgumentsRejectsConflictingHostModes(t *testing.T) {
	if _, err := parseLaunchArguments([]string{"--tray", "--once"}); err == nil {
		t.Fatal("combined --tray and --once was accepted")
	}
}

func TestParseLaunchArgumentsRejectsTrayCoordinates(t *testing.T) {
	if _, err := parseLaunchArguments([]string{"--tray", "0", "0", "800", "600"}); err == nil {
		t.Fatal("tray mode accepted capture coordinates")
	}
}

func TestParseLaunchArgumentsRejectsOnceCoordinates(t *testing.T) {
	if _, err := parseLaunchArguments([]string{"--once", "0", "0", "800", "600"}); err == nil {
		t.Fatal("once mode accepted capture coordinates")
	}
}

func TestParseArgumentsAcceptsReliabilityFlags(t *testing.T) {
	config, err := parseArguments([]string{
		"--interval", "250ms",
		"--max-scroll-ratio", "0.7",
		"--max-mean-diff", "12",
		"--min-confidence", "0.5",
		"--stationary-threshold", "0.75",
		"--diagnostics", "diagnostics",
		"--diagnostic-limit", "12",
		"0", "0", "800", "600",
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.Interval != 250*time.Millisecond || config.MatchOptions.MaxOffsetRatio != 0.7 {
		t.Fatalf("flags were not applied: %+v", config)
	}
	if config.MatchOptions.MaxMeanDifference != 12 || config.MatchOptions.MinimumConfidence != 0.5 || config.MatchOptions.StationaryDifference != 0.75 {
		t.Fatalf("matcher flags were not applied: %+v", config.MatchOptions)
	}
	if config.DiagnosticDir != "diagnostics" || config.DiagnosticMax != 12 {
		t.Fatalf("diagnostic flags were not applied: %+v", config)
	}
}

func TestSavedPreferencesProvideDefaultsAndCommandLineWins(t *testing.T) {
	saved := defaultPreferences()
	saved.LongCapture.IntervalMS = 300
	saved.LongCapture.MaxScrollRatio = 0.65
	saved.Diagnostics.Enabled = true
	saved.Diagnostics.Directory = "saved-diagnostics"
	saved.Diagnostics.Limit = 20
	options, err := parseLaunchArgumentsWithPreferences([]string{
		"--interval", "250ms",
		"--diagnostic-limit", "12",
	}, saved, filepath.Join("program", "dir"))
	if err != nil {
		t.Fatal(err)
	}
	if options.Config.Interval != 250*time.Millisecond || options.Config.MatchOptions.MaxOffsetRatio != 0.65 {
		t.Fatalf("unexpected merged capture settings: %+v", options.Config)
	}
	if options.Config.DiagnosticDir != filepath.Join("program", "dir", "saved-diagnostics") || options.Config.DiagnosticMax != 12 {
		t.Fatalf("unexpected merged diagnostics: %+v", options.Config)
	}
	if !options.Overrides.Interval || !options.Overrides.DiagnosticLimit || options.Overrides.MaxScrollRatio {
		t.Fatalf("unexpected override tracking: %+v", options.Overrides)
	}

	changed := saved
	changed.LongCapture.IntervalMS = 500
	changed.Diagnostics.Limit = 99
	recomputed := changed.apply(options.Config, options.ProgramDirectory)
	recomputed = options.Overrides.apply(recomputed, options.CommandLineConfig)
	if recomputed.Interval != 250*time.Millisecond || recomputed.DiagnosticMax != 12 || recomputed.MatchOptions.MaxOffsetRatio != 0.65 {
		t.Fatalf("command-line overrides were not retained: %+v", recomputed)
	}
}

func TestParseArgumentsRejectsInvalidSettings(t *testing.T) {
	tests := [][]string{
		{"--interval", "0s"},
		{"--interval", "0s", "0", "0", "800", "600"},
		{"--max-scroll-ratio", "1", "0", "0", "800", "600"},
		{"--max-scroll-ratio", "NaN", "0", "0", "800", "600"},
		{"--diagnostic-limit", "-1", "0", "0", "800", "600"},
		{"--diagnostic-limit", "-1"},
		{"0", "0", "0", "600"},
		{"0", "0", "800"},
	}
	for _, arguments := range tests {
		if _, err := parseArguments(arguments); err == nil {
			t.Errorf("parseArguments(%q) succeeded, want error", arguments)
		}
	}
}
