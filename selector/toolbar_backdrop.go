package selector

import "image"

// backdropFetch keeps a potentially blocking background provider off the UI
// thread. Each window owns its fetch; closing/resizing drops the old result.
type backdropFetch struct {
	result <-chan image.Image
}

func (fetch *backdropFetch) start(source ToolbarBackground, bounds image.Rectangle, ready func()) bool {
	if source == nil || fetch.result != nil {
		return false
	}
	result := make(chan image.Image, 1)
	fetch.result = result
	go func() {
		result <- source(bounds)
		ready()
	}()
	return true
}

func (fetch *backdropFetch) poll() (image.Image, bool) {
	select {
	case source := <-fetch.result:
		fetch.result = nil
		return source, true
	default:
		return nil, false
	}
}
