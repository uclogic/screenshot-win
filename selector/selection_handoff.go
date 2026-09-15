package selector

import "image"

// selectionHandoff keeps window preparation off the selection UI thread.
// Only that thread accesses the fields; the worker publishes through result.
type selectionHandoff struct {
	result    chan error
	cancelled bool
}

func (h *selectionHandoff) pending() bool { return h.result != nil }

func (h *selectionHandoff) start(region image.Rectangle, prepare func(image.Rectangle) error, ready func()) {
	if h.pending() {
		return
	}
	result := make(chan error, 1)
	h.result = result
	go func() {
		result <- prepare(region)
		ready()
	}()
}

// finish is called only after the ready notification. Receiving the result also
// makes callback writes visible before SelectWithOptions returns to its caller.
func (h *selectionHandoff) finish() error {
	err := <-h.result
	h.result = nil
	return err
}
