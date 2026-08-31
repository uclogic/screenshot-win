package selector

import "image"

func dragRectangle(bounds image.Rectangle, start, end image.Point) image.Rectangle {
	start = clampPoint(bounds, start)
	end = clampPoint(bounds, end)
	minimum := image.Pt(min(start.X, end.X), min(start.Y, end.Y))
	maximum := image.Pt(max(start.X, end.X), max(start.Y, end.Y))
	return image.Rectangle{Min: minimum, Max: maximum}
}

func desktopRectangle(desktop image.Rectangle, start, end image.Point) image.Rectangle {
	client := image.Rect(0, 0, desktop.Dx(), desktop.Dy())
	return dragRectangle(client, start, end).Add(desktop.Min)
}

func clampPoint(bounds image.Rectangle, point image.Point) image.Point {
	return image.Pt(
		max(bounds.Min.X, min(point.X, bounds.Max.X)),
		max(bounds.Min.Y, min(point.Y, bounds.Max.Y)),
	)
}
