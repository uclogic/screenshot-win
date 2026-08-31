//go:build !windows

package main

import (
	"context"
	"fmt"
	"time"
)

func choosePNGPath(uintptr, time.Time) (string, bool, error) {
	return "", false, fmt.Errorf("save dialog is supported only on Windows")
}

func choosePNGPathContext(ctx context.Context, owner uintptr, now time.Time) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	return choosePNGPath(owner, now)
}

func suggestedScreenshotName(now time.Time) string {
	return "Screenshot_" + now.Format("20060102_150405") + ".png"
}

func showErrorMessage(uintptr, error) {}
