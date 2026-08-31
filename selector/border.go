package selector

import "sync"

// Border is a visible marker around the capture region. Close removes it.
type Border struct {
	once        sync.Once
	window      uintptr
	closeWindow func()
	done        <-chan struct{}
}

// WindowHandle returns the native window handle used to own modal dialogs.
func (border *Border) WindowHandle() uintptr {
	if border == nil {
		return 0
	}
	return border.window
}

// Close removes the border and waits for its window resources to be released.
func (border *Border) Close() {
	if border == nil {
		return
	}
	border.once.Do(func() {
		if border.closeWindow != nil {
			border.closeWindow()
		}
		if border.done != nil {
			<-border.done
		}
	})
}
