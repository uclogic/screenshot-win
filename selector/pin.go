package selector

import (
	"fmt"
	"image"
	"math"
	"sync"
)

const (
	pinMinimumScale = 0.1
	pinMaximumScale = 8.0
	pinZoomStep     = 1.1
)

// Pin is one independently managed desktop image window.
type Pin struct {
	once        sync.Once
	closeWindow func()
	done        <-chan struct{}
}

// Close removes this pinned image and releases its window resources.
func (pin *Pin) Close() {
	if pin == nil {
		return
	}
	pin.once.Do(func() {
		if pin.closeWindow != nil {
			pin.closeWindow()
		}
		if pin.done != nil {
			<-pin.done
		}
	})
}

// PinManager owns all pinned image windows for one application process.
type PinManager struct {
	mu     sync.Mutex
	pins   map[*Pin]struct{}
	closed bool
	wake   chan struct{}
}

func NewPinManager() *PinManager {
	return &PinManager{pins: make(map[*Pin]struct{}), wake: make(chan struct{})}
}

// Show creates a topmost pinned image with its top-left corner at origin.
func (manager *PinManager) Show(source image.Image, origin image.Point) (*Pin, error) {
	if manager == nil {
		return nil, fmt.Errorf("pin manager is nil")
	}
	if source == nil || source.Bounds().Empty() {
		return nil, fmt.Errorf("pinned image must not be empty")
	}
	manager.mu.Lock()
	if manager.closed {
		manager.mu.Unlock()
		return nil, fmt.Errorf("pin manager is closed")
	}
	manager.mu.Unlock()
	pin, err := showPinnedWindow(source, origin)
	if err != nil {
		return nil, err
	}
	manager.mu.Lock()
	if manager.closed {
		manager.mu.Unlock()
		pin.Close()
		return nil, fmt.Errorf("pin manager is closed")
	}
	manager.pins[pin] = struct{}{}
	manager.signalLocked()
	manager.mu.Unlock()
	go func() {
		<-pin.done
		manager.mu.Lock()
		delete(manager.pins, pin)
		manager.signalLocked()
		manager.mu.Unlock()
	}()
	return pin, nil
}

func (manager *PinManager) Count() int {
	if manager == nil {
		return 0
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return len(manager.pins)
}

// Wait blocks until every currently managed pinned image has closed.
func (manager *PinManager) Wait() {
	if manager == nil {
		return
	}
	for {
		manager.mu.Lock()
		if len(manager.pins) == 0 {
			manager.mu.Unlock()
			return
		}
		wake := manager.wake
		manager.mu.Unlock()
		<-wake
	}
}

// CloseAll prevents new pins, closes every existing pin, and waits for them.
func (manager *PinManager) CloseAll() {
	if manager == nil {
		return
	}
	manager.mu.Lock()
	manager.closed = true
	pins := make([]*Pin, 0, len(manager.pins))
	for pin := range manager.pins {
		pins = append(pins, pin)
	}
	manager.mu.Unlock()
	for _, pin := range pins {
		pin.Close()
	}
	manager.Wait()
}

func (manager *PinManager) signalLocked() {
	close(manager.wake)
	manager.wake = make(chan struct{})
}

func pinSize(original image.Point, scale float64) image.Point {
	if original.X <= 0 || original.Y <= 0 {
		return image.Point{}
	}
	scale = math.Max(pinMinimumScale, math.Min(pinMaximumScale, scale))
	return scaledPinSize(original, scale)
}

func scaledPinSize(original image.Point, scale float64) image.Point {
	return image.Pt(max(1, int(math.Round(float64(original.X)*scale))), max(1, int(math.Round(float64(original.Y)*scale))))
}

func pinScaleForSize(original, current image.Point) float64 {
	if original.X <= 0 || original.Y <= 0 || current.X <= 0 || current.Y <= 0 {
		return 1
	}
	return math.Min(float64(current.X)/float64(original.X), float64(current.Y)/float64(original.Y))
}

// pinZoomBounds keeps the image point beneath cursor fixed on screen.
func pinZoomBounds(bounds image.Rectangle, original image.Point, cursor image.Point, wheelDelta int) (image.Rectangle, float64) {
	if bounds.Empty() || original.X <= 0 || original.Y <= 0 || wheelDelta == 0 {
		return bounds, pinScaleForSize(original, bounds.Size())
	}
	oldScale := pinScaleForSize(original, bounds.Size())
	steps := float64(wheelDelta) / 120
	minimumScale := math.Min(pinMinimumScale, oldScale)
	newScale := math.Max(minimumScale, math.Min(pinMaximumScale, oldScale*math.Pow(pinZoomStep, steps)))
	newSize := scaledPinSize(original, newScale)
	rx := float64(cursor.X-bounds.Min.X) / float64(bounds.Dx())
	ry := float64(cursor.Y-bounds.Min.Y) / float64(bounds.Dy())
	newMin := image.Pt(cursor.X-int(math.Round(rx*float64(newSize.X))), cursor.Y-int(math.Round(ry*float64(newSize.Y))))
	return image.Rectangle{Min: newMin, Max: newMin.Add(newSize)}, newScale
}

func pinResetBounds(bounds image.Rectangle, original image.Point) image.Rectangle {
	center := image.Pt((bounds.Min.X+bounds.Max.X)/2, (bounds.Min.Y+bounds.Max.Y)/2)
	minPoint := image.Pt(center.X-original.X/2, center.Y-original.Y/2)
	return image.Rectangle{Min: minPoint, Max: minPoint.Add(original)}
}

func pinInitialBounds(original, origin image.Point, workArea image.Rectangle) image.Rectangle {
	if original.X <= 0 || original.Y <= 0 || workArea.Empty() {
		return image.Rectangle{}
	}
	maximum := image.Pt(max(1, workArea.Dx()*4/5), max(1, workArea.Dy()*4/5))
	scale := math.Min(1, math.Min(float64(maximum.X)/float64(original.X), float64(maximum.Y)/float64(original.Y)))
	size := scaledPinSize(original, scale)
	x := clampCoordinate(origin.X, workArea.Min.X, workArea.Max.X-size.X)
	y := clampCoordinate(origin.Y, workArea.Min.Y, workArea.Max.Y-size.Y)
	return image.Rect(x, y, x+size.X, y+size.Y)
}
