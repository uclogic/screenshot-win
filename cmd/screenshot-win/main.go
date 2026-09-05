package main

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	application "screenshot-win/app"
	"screenshot-win/capture"
	"screenshot-win/selector"
	"strconv"
	"time"
)

const defaultCaptureInterval = 100 * time.Millisecond
const candidateDebugFlag = "--debug.candidate_mode.minimalrectangle"
const launchUsage = "usage: screenshot-win (tray), or screenshot-win --debug.candidate_mode.minimalrectangle <x> <y> <width> <height> <output.png>"

type candidateDebugOptions struct {
	Region image.Rectangle
	Output string
}
type launchOptions struct {
	Config           application.Config
	Preferences      preferences
	ProgramDirectory string
	Debug            *candidateDebugOptions
}

func main() {
	attachParentConsole()
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "screenshot-win:", err)
		os.Exit(1)
	}
}
func parseLaunchArguments(arguments []string) (launchOptions, error) {
	if len(arguments) == 0 {
		return launchOptions{}, nil
	}
	if len(arguments) != 6 || arguments[0] != candidateDebugFlag {
		return launchOptions{}, fmt.Errorf("%s", launchUsage)
	}
	var v [4]int64
	for i := range v {
		n, err := strconv.ParseInt(arguments[i+1], 10, 32)
		if err != nil {
			return launchOptions{}, fmt.Errorf("invalid coordinate %q: %w", arguments[i+1], err)
		}
		v[i] = n
	}
	if v[2] <= 0 || v[3] <= 0 || v[0]+v[2] > 2147483647 || v[1]+v[3] > 2147483647 {
		return launchOptions{}, fmt.Errorf("invalid region dimensions or coordinate overflow")
	}
	if arguments[5] == "" {
		return launchOptions{}, fmt.Errorf("output path must not be empty")
	}
	return launchOptions{Debug: &candidateDebugOptions{Region: image.Rect(int(v[0]), int(v[1]), int(v[0]+v[2]), int(v[1]+v[3])), Output: arguments[5]}}, nil
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
	if options.Debug != nil {
		return runCandidateDebug(*options.Debug)
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	settingsPath := settingsPathForExecutable(executable)
	saved, settingsErr := loadPreferences(settingsPath)
	setUILanguage(saved.General.Language)
	options.Preferences = saved
	options.ProgramDirectory = filepath.Dir(executable)
	options.Config = saved.apply(application.Config{}, options.ProgramDirectory)
	if err := options.Config.Validate(); err != nil {
		return err
	}
	runner := application.NewRunner(application.New(), application.Runtime{ChoosePNGPath: choosePNGPath, ChoosePNGPathContext: choosePNGPathContext, ShowError: showErrorMessage, Stdout: os.Stdout, Stderr: os.Stderr, Now: time.Now})
	return runTrayHost(runner, options, settingsPath, settingsErr)
}
func runCandidateDebug(options candidateDebugOptions) error {
	if !options.Region.In(selector.DesktopBounds()) {
		return fmt.Errorf("debug region %v is outside virtual desktop %v", options.Region, selector.DesktopBounds())
	}
	source, err := capture.Region(options.Region.Min.X, options.Region.Min.Y, options.Region.Dx(), options.Region.Dy())
	if err != nil {
		return err
	}
	count, err := writeCandidateDebug(context.Background(), source, options.Output)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "%d candidate rectangles; saved %s\n", count, options.Output)
	return nil
}
func writeCandidateDebug(ctx context.Context, source image.Image, path string) (int, error) {
	rectangles, err := selector.DetectRectangles(ctx, source)
	if err != nil {
		return 0, err
	}
	output := selector.DrawCandidateRectangles(source, rectangles)
	file, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	if err := png.Encode(file, output); err != nil {
		file.Close()
		return 0, err
	}
	if err := file.Close(); err != nil {
		return 0, err
	}
	return len(rectangles), nil
}
