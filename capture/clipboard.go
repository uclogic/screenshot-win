package capture

import (
	"encoding/binary"
	"fmt"
	"image"
)

const bitmapInfoHeaderSize = 40

// encodeDIB converts an image into a bottom-up 32-bit BI_RGB DIB suitable for
// the Windows CF_DIB clipboard format.
func encodeDIB(source image.Image) ([]byte, error) {
	size, err := dibSize(source)
	if err != nil {
		return nil, err
	}
	result := make([]byte, size)
	if err := writeDIB(result, source); err != nil {
		return nil, err
	}
	return result, nil
}

func dibSize(source image.Image) (int, error) {
	if source == nil || source.Bounds().Empty() {
		return 0, fmt.Errorf("clipboard image must not be empty")
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	const maximumDIBDimension = int64(1<<31 - 1)
	if int64(width) > maximumDIBDimension || int64(height) > maximumDIBDimension {
		return 0, fmt.Errorf("clipboard image dimensions %dx%d exceed the DIB limit", width, height)
	}
	pixelBytes := uint64(width) * uint64(height) * 4
	maximumInt := uint64(^uint(0) >> 1)
	if pixelBytes > maximumInt-bitmapInfoHeaderSize || pixelBytes > uint64(^uint32(0)) {
		return 0, fmt.Errorf("clipboard image %dx%d is too large", width, height)
	}
	return bitmapInfoHeaderSize + int(pixelBytes), nil
}

func writeDIB(destination []byte, source image.Image) error {
	size, err := dibSize(source)
	if err != nil {
		return err
	}
	if len(destination) != size {
		return fmt.Errorf("clipboard DIB buffer has size %d, want %d", len(destination), size)
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	pixelBytes := uint32(size - bitmapInfoHeaderSize)
	clear(destination[:bitmapInfoHeaderSize])
	binary.LittleEndian.PutUint32(destination[0:4], bitmapInfoHeaderSize)
	binary.LittleEndian.PutUint32(destination[4:8], uint32(int32(width)))
	binary.LittleEndian.PutUint32(destination[8:12], uint32(int32(height)))
	binary.LittleEndian.PutUint16(destination[12:14], 1)
	binary.LittleEndian.PutUint16(destination[14:16], 32)
	binary.LittleEndian.PutUint32(destination[20:24], pixelBytes)

	index := bitmapInfoHeaderSize
	for y := bounds.Max.Y - 1; y >= bounds.Min.Y; y-- {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			red, green, blue, _ := source.At(x, y).RGBA()
			destination[index] = byte(blue >> 8)
			destination[index+1] = byte(green >> 8)
			destination[index+2] = byte(red >> 8)
			destination[index+3] = 0xff
			index += 4
		}
	}
	return nil
}
