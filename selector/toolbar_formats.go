package selector

import (
	"image"
)

var arrowFormats = [...]struct {
	action Action
	label  string
}{
	{ActionArrow, "Arrow"},
	{ActionDoubleArrow, "Double arrow"},
	{ActionLine, "Line"},
}

func arrowFormatAction(action Action) bool {
	return action == ActionArrow || action == ActionDoubleArrow || action == ActionLine
}

func currentArrowAction(action Action) Action {
	if arrowFormatAction(action) {
		return action
	}
	return ActionArrow
}

func arrowFormatIndex(action Action) int {
	action = currentArrowAction(action)
	for i, format := range arrowFormats {
		if format.action == action {
			return i
		}
	}
	return 0
}

func arrowFormatPanelSize(dpi int) image.Point {
	return image.Pt(scaleForDPICommon(8+36*len(arrowFormats), dpi), scaleForDPICommon(44, dpi))
}

func scaleForDPICommon(value, dpi int) int {
	if dpi <= 0 {
		dpi = 96
	}
	return (value*dpi + 48) / 96
}

func arrowFormatOptionAt(point image.Point, dpi int) (int, bool) {
	padding, cell := scaleForDPICommon(4, dpi), scaleForDPICommon(36, dpi)
	if point.X < padding || point.X >= padding+cell*len(arrowFormats) || point.Y < padding || point.Y >= padding+cell {
		return 0, false
	}
	return (point.X - padding) / cell, true
}
