//go:build !windows

package capture

func ReadClipboard() (ClipboardContent, error) { return ClipboardContent{}, errUnsupported }
