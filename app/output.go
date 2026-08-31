package app

import (
	"image"
	"image/png"
	"os"
)

func savePNG(path string, source image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(file, source); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
