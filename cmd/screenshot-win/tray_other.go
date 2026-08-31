//go:build !windows

package main

import (
	"fmt"

	application "screenshot-win/app"
)

func runTrayHost(*application.Runner, launchOptions, string, error) error {
	return fmt.Errorf("notification-area host is supported only on Windows")
}
