package selector

import "sync/atomic"

// PinMenuLabels contains the localized text for pinned image context menus.
type PinMenuLabels struct {
	OriginalSize string
	Close        string
}

var currentPinMenuLabels atomic.Pointer[PinMenuLabels]

// SetPinMenuLabels updates the menu text, including for existing pinned images.
// Empty labels fall back to English.
func SetPinMenuLabels(labels PinMenuLabels) {
	if labels.OriginalSize == "" {
		labels.OriginalSize = "Original size"
	}
	if labels.Close == "" {
		labels.Close = "Close"
	}
	currentPinMenuLabels.Store(&labels)
}

func pinMenuLabels() PinMenuLabels {
	if labels := currentPinMenuLabels.Load(); labels != nil {
		return *labels
	}
	return PinMenuLabels{OriginalSize: "Original size", Close: "Close"}
}
