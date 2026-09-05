package capture

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/bits"
	"strings"
	"unicode/utf16"
)

// ClipboardContent is a snapshot independent of the system clipboard handles.
// Image takes priority; Text is populated only when no supported image decodes.
type ClipboardContent struct {
	Image image.Image
	Text  string
}

const maximumClipboardBytes = 256 << 20
const maximumClipboardPixels = 64 << 20

func checkedClipboardDimensions(width, height int64) error {
	if width <= 0 || height <= 0 || width > maximumClipboardPixels || height > maximumClipboardPixels || width*height > maximumClipboardPixels {
		return fmt.Errorf("clipboard image dimensions are invalid or too large: %dx%d", width, height)
	}
	return nil
}

func decodeClipboardPNG(data []byte) (image.Image, error) {
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if err := checkedClipboardDimensions(int64(config.Width), int64(config.Height)); err != nil {
		return nil, err
	}
	return png.Decode(bytes.NewReader(data))
}

// decodeClipboardDIB accepts uncompressed Windows DIBs, including palettes and
// explicit RGB(A) masks. Every offset is checked before accessing clipboard data.
func decodeClipboardDIB(data []byte) (image.Image, error) {
	fail := func() (image.Image, error) { return nil, fmt.Errorf("unsupported or malformed clipboard bitmap") }
	if len(data) < 40 {
		return fail()
	}
	u32 := func(offset int) uint32 { return binary.LittleEndian.Uint32(data[offset : offset+4]) }
	header := int64(u32(0))
	if header != 40 && header != 52 && header != 56 && header != 108 && header != 124 || header > int64(len(data)) {
		return fail()
	}
	width, signedHeight := int64(int32(u32(4))), int64(int32(u32(8)))
	height := signedHeight
	if height < 0 {
		height = -height
	}
	if err := checkedClipboardDimensions(width, height); err != nil {
		return nil, err
	}
	depth := int(binary.LittleEndian.Uint16(data[14:16]))
	compression := u32(16)
	if binary.LittleEndian.Uint16(data[12:14]) != 1 || compression != 0 && compression != 3 && compression != 6 {
		return fail()
	}
	if depth != 1 && depth != 4 && depth != 8 && depth != 16 && depth != 24 && depth != 32 {
		return fail()
	}
	if compression != 0 && depth != 16 && depth != 32 {
		return fail()
	}
	offset := header
	masks := [4]uint32{}
	if depth == 16 {
		masks = [4]uint32{0x7c00, 0x3e0, 0x1f, 0}
	}
	if depth == 32 {
		masks = [4]uint32{0xff0000, 0xff00, 0xff, 0}
	}
	if compression != 0 {
		count := 3
		if compression == 6 || header >= 56 {
			count = 4
		}
		maskOffset := int64(40)
		if header == 40 {
			maskOffset = offset
			offset += int64(count * 4)
		}
		if maskOffset+int64(count*4) > int64(len(data)) || header != 40 && maskOffset+int64(count*4) > header {
			return fail()
		}
		var used uint32
		for i := 0; i < count; i++ {
			mask := u32(int(maskOffset) + i*4)
			if i < 3 && mask == 0 || used&mask != 0 || depth == 16 && mask > 0xffff {
				return fail()
			}
			if mask != 0 {
				shifted := mask >> bits.TrailingZeros32(mask)
				if shifted&(shifted+1) != 0 {
					return fail()
				}
			}
			masks[i] = mask
			used |= mask
		}
	}
	paletteCount := int64(u32(32))
	if depth <= 8 {
		if paletteCount == 0 {
			paletteCount = 1 << depth
		}
		if paletteCount > 1<<depth {
			return fail()
		}
	}
	paletteOffset := offset
	offset += paletteCount * 4
	if offset > int64(len(data)) {
		return fail()
	}
	if header == 124 {
		profileOffset, profileSize := int64(u32(112)), int64(u32(116))
		if profileSize != 0 {
			if profileOffset < offset || profileOffset+profileSize > int64(len(data)) {
				return fail()
			}
			if profileOffset == offset {
				offset += profileSize
			}
		}
	}
	stride := ((width*int64(depth) + 31) / 32) * 4
	if stride*height > int64(len(data))-offset {
		return fail()
	}
	result := image.NewNRGBA(image.Rect(0, 0, int(width), int(height)))
	channel := func(value, mask uint32) uint8 {
		if mask == 0 {
			return 255
		}
		shift := bits.TrailingZeros32(mask)
		maximum := mask >> shift
		return uint8((uint64((value&mask)>>shift)*255 + uint64(maximum)/2) / uint64(maximum))
	}
	for y := 0; y < int(height); y++ {
		sourceY := int64(y)
		if signedHeight > 0 {
			sourceY = height - 1 - sourceY
		}
		row := data[int(offset+sourceY*stride):int(offset+(sourceY+1)*stride)]
		for x := 0; x < int(width); x++ {
			var c color.NRGBA
			switch depth {
			case 1, 4, 8:
				index := (int(row[x*depth/8]) >> (8 - depth - (x * depth % 8))) & ((1 << depth) - 1)
				if int64(index) >= paletteCount {
					return fail()
				}
				p := int(paletteOffset) + index*4
				c = color.NRGBA{data[p+2], data[p+1], data[p], 255}
			case 24:
				p := x * 3
				c = color.NRGBA{row[p+2], row[p+1], row[p], 255}
			default:
				var value uint32
				if depth == 16 {
					value = uint32(binary.LittleEndian.Uint16(row[x*2:]))
				} else {
					value = binary.LittleEndian.Uint32(row[x*4:])
				}
				c = color.NRGBA{channel(value, masks[0]), channel(value, masks[1]), channel(value, masks[2]), channel(value, masks[3])}
			}
			result.SetNRGBA(x, y, c)
		}
	}
	return result, nil
}

func usableClipboardText(text string) bool { return strings.TrimSpace(text) != "" }

// Clipboard bytes are copied while the clipboard is open and decoded only
// after it is closed, so expensive image work cannot block other applications.
type clipboardImageData struct {
	png  bool
	data []byte
}

func decodeClipboardSnapshot(images []clipboardImageData, textData []byte, lastErr error) (ClipboardContent, error) {
	for _, candidate := range images {
		var source image.Image
		var err error
		if candidate.png {
			source, err = decodeClipboardPNG(candidate.data)
		} else {
			source, err = decodeClipboardDIB(candidate.data)
		}
		if err == nil {
			return ClipboardContent{Image: source}, nil
		}
		lastErr = err
	}
	if len(textData) > 0 {
		if len(textData)%2 != 0 {
			return ClipboardContent{}, fmt.Errorf("invalid Unicode clipboard text")
		}
		units := make([]uint16, 0, len(textData)/2)
		terminated := false
		for i := 0; i < len(textData); i += 2 {
			value := binary.LittleEndian.Uint16(textData[i:])
			if value == 0 {
				terminated = true
				break
			}
			units = append(units, value)
		}
		if !terminated {
			return ClipboardContent{}, fmt.Errorf("unterminated Unicode clipboard text")
		}
		text := string(utf16.Decode(units))
		if usableClipboardText(text) {
			return ClipboardContent{Text: text}, nil
		}
	}
	if lastErr != nil {
		return ClipboardContent{}, fmt.Errorf("读取剪贴板失败：%w", lastErr)
	}
	return ClipboardContent{}, fmt.Errorf("剪贴板中没有可贴出的图片或文字")
}
