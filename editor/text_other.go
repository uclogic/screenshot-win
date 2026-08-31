//go:build !windows

package editor

func rasterizeText(string, int) *textMask { return nil }
