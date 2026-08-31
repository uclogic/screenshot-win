//go:build !windows

package main

import "os"

func replaceSettingsFile(source, destination string) error {
	return os.Rename(source, destination)
}
