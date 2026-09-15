package glass

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"
)

func TestCropCoordinatesEdgesAndOwnership(t *testing.T) {
	source := image.NewRGBA(image.Rect(-4, -2, -2, 0))
	source.SetRGBA(-4, -2, color.RGBA{10, 20, 30, 255})
	source.SetRGBA(-3, -1, color.RGBA{90, 80, 70, 255})
	copy := Crop(source, image.Rect(-6, -4, 0, 2))
	if got := copy.RGBAAt(-6, -4); got != source.RGBAAt(-4, -2) {
		t.Fatalf("negative edge: %v", got)
	}
	if got := copy.RGBAAt(-1, 1); got != source.RGBAAt(-3, -1) {
		t.Fatalf("positive edge: %v", got)
	}
	copy.SetRGBA(-4, -2, color.RGBA{})
	if source.RGBAAt(-4, -2).A != 255 {
		t.Fatal("crop aliases source")
	}
	if Crop(nil, image.Rect(0, 0, 1, 1)).RGBAAt(0, 0) != (color.RGBA{245, 249, 255, 255}) {
		t.Fatal("missing background is not neutral")
	}
}

func TestMaterialAlphaAndBackgroundResponse(t *testing.T) {
	for _, dpi := range []int{96, 144, 192} {
		bounds := image.Rect(-600, -40, -600+Scale(432, dpi), -40+Scale(64, dpi))
		body := (image.Rectangle{Max: bounds.Size()}).Inset(Margin(dpi))
		dark := image.NewUniform(color.RGBA{0, 0, 0, 255})
		light := image.NewUniform(color.RGBA{255, 255, 255, 255})
		a := Render(dark, bounds, body, float64(Scale(24, dpi)), dpi, Light)
		b := Render(light, bounds, body, float64(Scale(24, dpi)), dpi, Light)
		center := body.Min.Add(body.Size().Div(2))
		if a.RGBAAt(center.X, center.Y).A != 255 {
			t.Fatal("material interior must be opaque after backdrop composition")
		}
		if a.RGBAAt(center.X, center.Y).R >= b.RGBAAt(center.X, center.Y).R {
			t.Fatal("material ignores background")
		}
		partial := 0
		for i := 0; i < len(a.Pix); i += 4 {
			r, g, b, alpha := a.Pix[i], a.Pix[i+1], a.Pix[i+2], a.Pix[i+3]
			if r > alpha || g > alpha || b > alpha {
				t.Fatal("non-premultiplied pixel")
			}
			if alpha > 0 && alpha < 255 {
				partial++
			}
		}
		if partial == 0 || a.RGBAAt(0, 0).A != 0 {
			t.Fatal("missing soft transparent contour")
		}
		if Contains(image.Point{}, body, float64(Scale(24, dpi))) {
			t.Fatal("shadow is interactive")
		}
	}
}

func TestTransitionReversalAndIdle(t *testing.T) {
	var transition Transition
	now := time.Unix(100, 0)
	if v, active := transition.Update(false, now); v != 0 || active {
		t.Fatal("idle transition animates")
	}
	transition.Update(true, now)
	v, active := transition.Update(true, now.Add(70*time.Millisecond))
	if v < .49 || v > .51 || !active {
		t.Fatalf("midpoint = %v, %v", v, active)
	}
	reversed, _ := transition.Update(false, now.Add(70*time.Millisecond))
	if reversed != v {
		t.Fatal("reversal snapped")
	}
	if v, active := transition.Update(false, now.Add(210*time.Millisecond)); v != 0 || active {
		t.Fatal("timer would continue after transition")
	}
}

func TestBlurPreservesConstant(t *testing.T) {
	src := Crop(image.NewUniform(color.RGBA{70, 90, 110, 255}), image.Rect(-10, -10, 10, 10))
	dst := blur(src, 5)
	for i := range src.Pix {
		if src.Pix[i] != dst.Pix[i] {
			t.Fatal("blur changed a constant at its edge")
		}
	}
}

// Optional renderer-only preview for review on machines without a Windows GUI.
func TestMaterialPreview(t *testing.T) {
	path := os.Getenv("GLASS_PREVIEW")
	if path == "" {
		t.Skip("set GLASS_PREVIEW to export a material preview")
	}
	bounds := image.Rect(0, 0, 640, 240)
	bg := image.NewRGBA(bounds)
	for y := 0; y < 240; y++ {
		for x := 0; x < 640; x++ {
			c := color.RGBA{uint8(50 + x/4), uint8(70 + y/2), uint8(180 - x/8), 255}
			if x > 270 && x < 350 {
				c = color.RGBA{230, 170, 100, 255}
			}
			bg.SetRGBA(x, y, c)
		}
	}
	frameBounds := image.Rect(104, 88, 536, 152)
	frame := Render(bg, frameBounds, image.Rect(8, 8, 424, 56), 24, 96, Light)
	Overlay(frame, image.Rect(58, 14, 94, 50), 10, Light.Selected, 1)
	for y := 0; y < 64; y++ {
		for x := 0; x < 432; x++ {
			p := frame.RGBAAt(x, y)
			b := bg.RGBAAt(x+104, y+88)
			mix := func(f, v uint8) uint8 { return uint8(uint32(f) + uint32(v)*uint32(255-p.A)/255) }
			bg.SetRGBA(x+104, y+88, color.RGBA{mix(p.R, b.R), mix(p.G, b.G), mix(p.B, b.B), 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, bg); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkMaterial(b *testing.B) {
	bounds := image.Rect(0, 0, 432, 64)
	bg := Crop(nil, bounds.Inset(-Support(96)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Render(bg, bounds, image.Rect(8, 8, 424, 56), 24, 96, Light)
	}
}
