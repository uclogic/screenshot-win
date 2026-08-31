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
	if source == nil || source.Bounds().Empty() {
		return nil, fmt.Errorf("clipboard image must not be empty")
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	pixelBytes := width * height * 4
	result := make([]byte, bitmapInfoHeaderSize+pixelBytes)
	binary.LittleEndian.PutUint32(result[0:4], bitmapInfoHeaderSize)
	binary.LittleEndian.PutUint32(result[4:8], uint32(int32(width)))
	binary.LittleEndian.PutUint32(result[8:12], uint32(int32(height)))
	binary.LittleEndian.PutUint16(result[12:14], 1)
	binary.LittleEndian.PutUint16(result[14:16], 32)
	binary.LittleEndian.PutUint32(result[20:24], uint32(pixelBytes))

	index := bitmapInfoHeaderSize
	for y := bounds.Max.Y - 1; y >= bounds.Min.Y; y-- {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			red, green, blue, _ := source.At(x, y).RGBA()
			result[index] = byte(blue >> 8)
			result[index+1] = byte(green >> 8)
			result[index+2] = byte(red >> 8)
			result[index+3] = 0xff
			index += 4
		}
	}
	return result, nil
}
