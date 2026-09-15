package selector

import (
	"image"
	"testing"

	"screenshot-win/internal/ui/glass"
)

func TestGlassLayoutHitRegions(t *testing.T) {
	for _, dpi := range []int{96, 120, 144, 192} {
		for _, count := range []int{9, 10} {
			size := glassToolbarSize(count, dpi)
			for i := 0; i < count; i++ {
				b := glassToolbarButton(i, dpi)
				point := b.Min.Add(b.Size().Div(2))
				index, ok := glassToolbarActionAt(point, count, dpi)
				if !ok || index != i {
					t.Fatalf("dpi=%d button=%d hit=%d %v", dpi, i, index, ok)
				}
				if !b.In(image.Rectangle{Max: size}) {
					t.Fatal("button outside surface")
				}
			}
			for _, p := range []image.Point{{0, 0}, {glass.Margin(dpi), glass.Margin(dpi)}, {size.X - 1, size.Y / 2}, {size.X / 2, size.Y - 1}} {
				if _, ok := glassToolbarActionAt(p, count, dpi); ok {
					t.Fatalf("shadow/padding accepted: %v", p)
				}
			}
		}
	}
}
