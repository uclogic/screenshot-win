package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"screenshot-win"
	application "screenshot-win/app"
	"screenshot-win/capture"
)

const defaultCaptureInterval = 100 * time.Millisecond

func main() {
	attachParentConsole()
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "screenshot-win:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	options, err := parseLaunchArguments(arguments)
	if err != nil {
		return err
	}
	if !capture.Supported() {
		return fmt.Errorf("desktop capture is currently supported only on Windows")
	}
	if err := capture.Initialize(); err != nil {
		return err
	}
	runner := application.NewRunner(application.New(), application.Runtime{
		ChoosePNGPath:        choosePNGPath,
		ChoosePNGPathContext: choosePNGPathContext,
		ShowError:            showErrorMessage,
		Stdout:               os.Stdout,
		Stderr:               os.Stderr,
		Now:                  time.Now,
	})
	if options.Tray {
		return runTrayHost(runner, options.Config)
	}
	return runner.Run(options.Config)
}

type launchOptions struct {
	Config application.Config
	Tray   bool
}

func parseArguments(arguments []string) (application.Config, error) {
	options, err := parseLaunchArguments(arguments)
	return options.Config, err
}

func parseLaunchArguments(arguments []string) (launchOptions, error) {
	config := application.Config{
		OutputPath:    "result.png",
		Interval:      defaultCaptureInterval,
		MatchOptions:  screenshotwin.DefaultMatchOptions(),
		DiagnosticMax: 50,
	}
	flags := flag.NewFlagSet("screenshot-win", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	tray := false
	once := false
	flags.BoolVar(&tray, "tray", false, "run in the Windows notification area (default without coordinates)")
	flags.BoolVar(&once, "once", false, "run one interactive capture without the tray host")
	flags.DurationVar(&config.Interval, "interval", config.Interval, "capture interval")
	flags.Float64Var(&config.MatchOptions.MaxOffsetRatio, "max-scroll-ratio", config.MatchOptions.MaxOffsetRatio, "maximum scroll as a fraction of frame height")
	flags.Float64Var(&config.MatchOptions.MaxMeanDifference, "max-mean-diff", config.MatchOptions.MaxMeanDifference, "maximum accepted mean pixel difference")
	flags.Float64Var(&config.MatchOptions.MinimumConfidence, "min-confidence", config.MatchOptions.MinimumConfidence, "minimum difference between best and second-best scores")
	flags.Float64Var(&config.MatchOptions.StationaryDifference, "stationary-threshold", config.MatchOptions.StationaryDifference, "maximum mean difference treated as stationary")
	flags.StringVar(&config.DiagnosticDir, "diagnostics", "", "directory for diagnostic events and rejected frames")
	flags.IntVar(&config.DiagnosticMax, "diagnostic-limit", config.DiagnosticMax, "maximum rejected frame pairs to save")
	if err := flags.Parse(normalizeFlagArguments(arguments)); err != nil {
		return launchOptions{}, fmt.Errorf("%v; usage: screenshot-win [--tray|--once] [flags] [<x> <y> <width> <height> [result.png]]", err)
	}
	positional := flags.Args()
	if tray && once {
		return launchOptions{}, errors.New("--tray and --once cannot be combined")
	}
	if tray && len(positional) != 0 {
		return launchOptions{}, errors.New("--tray cannot be combined with capture coordinates")
	}
	if once && len(positional) != 0 {
		return launchOptions{}, errors.New("--once cannot be combined with capture coordinates")
	}
	if len(positional) == 0 {
		config.Interactive = true
		return launchOptions{Config: config, Tray: !once}, validateConfiguration(config)
	}
	if len(positional) < 4 || len(positional) > 5 {
		return launchOptions{}, fmt.Errorf("usage: screenshot-win [--tray|--once] [flags] [<x> <y> <width> <height> [result.png]]")
	}
	values := make([]int, 4)
	for index := range values {
		value, err := strconv.Atoi(positional[index])
		if err != nil {
			return launchOptions{}, fmt.Errorf("%q is not a valid integer", positional[index])
		}
		values[index] = value
	}
	config.X, config.Y, config.Width, config.Height = values[0], values[1], values[2], values[3]
	if config.Width <= 0 || config.Height <= 0 {
		return launchOptions{}, fmt.Errorf("width and height must be positive")
	}
	if len(positional) == 5 {
		config.OutputPath = positional[4]
	}
	return launchOptions{Config: config}, validateConfiguration(config)
}

func validateConfiguration(config application.Config) error {
	return config.Validate()
}

func normalizeFlagArguments(arguments []string) []string {
	known := map[string]bool{
		"tray": false, "once": false, "interval": true, "max-scroll-ratio": true, "max-mean-diff": true,
		"min-confidence": true, "stationary-threshold": true,
		"diagnostics": true, "diagnostic-limit": true,
	}
	for index := 0; index < len(arguments); {
		argument := arguments[index]
		if argument == "--" {
			return arguments
		}
		if _, err := strconv.Atoi(argument); err == nil {
			return insertFlagTerminator(arguments, index)
		}
		if !strings.HasPrefix(argument, "-") {
			return insertFlagTerminator(arguments, index)
		}
		nameAndValue := strings.TrimLeft(argument, "-")
		name, _, hasValue := strings.Cut(nameAndValue, "=")
		requiresValue, knownFlag := known[name]
		if !knownFlag {
			return arguments
		}
		index++
		if requiresValue && !hasValue {
			if index >= len(arguments) {
				return arguments
			}
			index++
		}
	}
	return arguments
}

func insertFlagTerminator(arguments []string, index int) []string {
	normalized := make([]string, 0, len(arguments)+1)
	normalized = append(normalized, arguments[:index]...)
	normalized = append(normalized, "--")
	normalized = append(normalized, arguments[index:]...)
	return normalized
}
