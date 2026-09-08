package selector

import "image"

// drawPinRasterTiles bounds both temporary memory and the bitmap submitted to
// GDI. Sample in full client coordinates so clipped repaints and tile edges
// agree with a complete repaint, even for very large zoomed windows.
func drawPinRasterTiles(pixels []byte, original, size image.Point, visible image.Rectangle, draw func(image.Rectangle, []byte) error) error {
	visible = visible.Intersect(image.Rectangle{Max: size})
	if visible.Empty() {
		return nil
	}
	const edge = 256
	buffer := make([]byte, edge*edge*4)
	for y := visible.Min.Y; y < visible.Max.Y; y += edge {
		for x := visible.Min.X; x < visible.Max.X; x += edge {
			tile := image.Rect(x, y, min(x+edge, visible.Max.X), min(y+edge, visible.Max.Y))
			data := buffer[:tile.Dx()*tile.Dy()*4]
			for dy := 0; dy < tile.Dy(); dy++ {
				sy := int(int64(y+dy) * int64(original.Y) / int64(size.Y))
				for dx := 0; dx < tile.Dx(); dx++ {
					sx := int(int64(x+dx) * int64(original.X) / int64(size.X))
					src := (sy*original.X + sx) * 4
					dst := (dy*tile.Dx() + dx) * 4
					copy(data[dst:dst+4], pixels[src:src+4])
				}
			}
			if err := draw(tile, data); err != nil {
				return err
			}
		}
	}
	return nil
}
