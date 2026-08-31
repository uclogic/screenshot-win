package selector

import "sync"

// MouseShield prevents mouse movement over a capture region from reaching the
// window underneath while still relaying wheel input for scrolling.
type MouseShield struct {
	once        sync.Once
	closeWindow func()
	done        <-chan struct{}
}

// Close removes the input shield and waits for its window resources to be
// released.
func (shield *MouseShield) Close() {
	if shield == nil {
		return
	}
	shield.once.Do(func() {
		if shield.closeWindow != nil {
			shield.closeWindow()
		}
		if shield.done != nil {
			<-shield.done
		}
	})
}
