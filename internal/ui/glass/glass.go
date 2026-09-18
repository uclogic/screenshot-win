// Package glass renders reusable, cached glass surfaces without a windowing or
// screenshot dependency. Coordinates passed to Render are physical pixels.
package glass

import (
	"image"
	"image/color"
	"math"
)

// Theme contains material and interaction colors shared by native views.
type Theme struct {
	Tint, Ink, Hover, Selected color.NRGBA
	TintAmount                 float64
}

var Light = Theme{
	Tint: color.NRGBA{245, 249, 255, 255}, Ink: color.NRGBA{35, 45, 60, 255},
	Hover: color.NRGBA{255, 255, 255, 100}, Selected: color.NRGBA{90, 160, 245, 90},
	TintAmount: .24,
}

func Scale(n, dpi int) int { return (n*max(96, dpi) + 48) / 96 }
func Margin(dpi int) int   { return Scale(8, dpi) }
func Support(dpi int) int  { return Scale(18, dpi) }

// Crop copies screen-coordinate pixels, extending the nearest edge. A missing
// backdrop produces an opaque neutral surface. The result never aliases source.
func Crop(source image.Image, bounds image.Rectangle) *image.RGBA {
	dst := image.NewRGBA(bounds)
	rgba, _ := source.(*image.RGBA)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := color.RGBA{Light.Tint.R, Light.Tint.G, Light.Tint.B, 255}
			if source != nil && !source.Bounds().Empty() {
				b := source.Bounds()
				sx, sy := max(b.Min.X, min(x, b.Max.X-1)), max(b.Min.Y, min(y, b.Max.Y-1))
				if rgba != nil {
					c = rgba.RGBAAt(sx, sy)
				} else {
					c = color.RGBAModel.Convert(source.At(sx, sy)).(color.RGBA)
				}
			}
			dst.SetRGBA(x, y, c)
		}
	}
	return dst
}

// Distance is a signed distance to a rounded rectangle, negative inside.
func Distance(x, y float64, b image.Rectangle, radius float64) float64 {
	radius = math.Min(radius, float64(min(b.Dx(), b.Dy()))/2)
	qx := math.Abs(x-float64(b.Min.X+b.Max.X)/2) - float64(b.Dx())/2 + radius
	qy := math.Abs(y-float64(b.Min.Y+b.Max.Y)/2) - float64(b.Dy())/2 + radius
	return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - radius
}

func Contains(p image.Point, b image.Rectangle, radius float64) bool {
	return Distance(float64(p.X)+.5, float64(p.Y)+.5, b, radius) <= 0
}

// Render produces premultiplied RGBA. The material interior already contains
// its backdrop; only the antialiased contour and shadow have partial alpha.
func Render(source image.Image, bounds, body image.Rectangle, radius float64, dpi int, theme Theme) *image.RGBA {
	return render(source, bounds, body, radius, dpi, theme, false)
}

// RenderFrosted uses the theme's tint amount and a broad blur without lens distortion.
// Like Render, it composites the backdrop before presenting the native surface.
func RenderFrosted(source image.Image, bounds, body image.Rectangle, radius float64, dpi int, theme Theme) *image.RGBA {
	theme.Tint = color.NRGBA{255, 255, 255, 255}
	theme.TintAmount = math.Max(0, math.Min(1, theme.TintAmount))
	return render(source, bounds, body, radius, dpi, theme, true)
}

func render(source image.Image, bounds, body image.Rectangle, radius float64, dpi int, theme Theme, frosted bool) *image.RGBA {
	back := Crop(source, bounds.Inset(-Support(dpi)))
	blurRadius := Scale(3, dpi)
	if frosted {
		// Three radius-6 passes suppress text and fine background details,
		// retaining broad color changes within the existing sampling support.
		blurRadius = Scale(6, dpi)
	}
	for i := 0; i < 3; i++ {
		back = blur(back, blurRadius)
	}
	out := image.NewRGBA(image.Rectangle{Max: bounds.Size()})
	scale := float64(max(dpi, 96)) / 96
	for y := 0; y < out.Rect.Dy(); y++ {
		for x := 0; x < out.Rect.Dx(); x++ {
			fx, fy := float64(x)+.5, float64(y)+.5
			d := Distance(fx, fy, body, radius)
			coverage := math.Max(0, math.Min(1, .5-d))
			if coverage == 0 {
				sd := math.Max(0, Distance(fx, fy-2*scale, body, radius))
				a := .18 * math.Exp(-sd*sd/(2*9*scale*scale))
				// Fade to zero before the window boundary to avoid clipped shadows.
				edge := float64(min(x, y, out.Rect.Dx()-1-x, out.Rect.Dy()-1-y))
				a *= math.Min(1, edge/(2*scale))
				out.SetRGBA(x, y, color.RGBA{0, 0, 0, uint8(a * 255)})
				continue
			}
			// A curved lens around the contour displaces the frozen background.
			// Keep the center clear so the material retains the scene's colors.
			depth := math.Abs(d) / scale
			band := math.Exp(-depth/7) * 11 * scale
			nx := Distance(fx+.5, fy, body, radius) - Distance(fx-.5, fy, body, radius)
			ny := Distance(fx, fy+.5, body, radius) - Distance(fx, fy-.5, body, radius)
			// Two smooth waves approximate low-frequency liquid distortion.
			// Fade them toward the center to preserve content readability.
			wave := math.Exp(-depth/16) * 2 * scale
			wx := math.Sin(fy/(31*scale)+fx/(67*scale)) * wave
			wy := math.Sin(fx/(37*scale)-fy/(53*scale)) * wave
			sx, sy := bounds.Min.X+x+int(math.Round(nx*band+wx)), bounds.Min.Y+y+int(math.Round(ny*band+wy))
			c := back.RGBAAt(sx, sy)
			highlight := math.Exp(-depth/1.1) * (.38 + .42*math.Max(0, -ny))
			innerShade := .10 * math.Exp(-math.Pow((depth-3)/2.5, 2)) * math.Max(0, ny)
			if frosted {
				c = back.RGBAAt(bounds.Min.X+x, bounds.Min.Y+y)
				highlight = .12 * math.Exp(-depth/0.7)
				innerShade = 0
			}
			mix := func(v, t uint8) uint8 {
				f := float64(v)*(1-theme.TintAmount) + float64(t)*theme.TintAmount
				f *= 1 - innerShade
				f += (255 - f) * highlight
				return uint8(math.Min(255, f) * coverage)
			}
			out.SetRGBA(x, y, color.RGBA{mix(c.R, theme.Tint.R), mix(c.G, theme.Tint.G), mix(c.B, theme.Tint.B), uint8(coverage * 255)})
		}
	}
	return out
}

// Overlay composites a rounded interaction highlight onto an opaque material.
func Overlay(dst *image.RGBA, bounds image.Rectangle, radius float64, c color.NRGBA, amount float64) {
	for y := max(0, bounds.Min.Y); y < min(dst.Rect.Max.Y, bounds.Max.Y); y++ {
		for x := max(0, bounds.Min.X); x < min(dst.Rect.Max.X, bounds.Max.X); x++ {
			coverage := math.Max(0, math.Min(1, .5-Distance(float64(x)+.5, float64(y)+.5, bounds, radius)))
			a := coverage * float64(c.A) / 255 * math.Max(0, math.Min(1, amount))
			p := dst.RGBAAt(x, y)
			mix := func(v, t uint8) uint8 { return uint8(float64(v)*(1-a) + float64(t)*a*float64(p.A)/255) }
			dst.SetRGBA(x, y, color.RGBA{mix(p.R, c.R), mix(p.G, c.G), mix(p.B, c.B), p.A})
		}
	}
}

// Three separable running-sum box passes approximate a Gaussian in linear time.
func blur(src *image.RGBA, radius int) *image.RGBA {
	b := src.Bounds()
	tmp, out := image.NewRGBA(b), image.NewRGBA(b)
	for axis := 0; axis < 2; axis++ {
		input, output := src, tmp
		outer, inner := b.Dy(), b.Dx()
		if axis == 1 {
			input, output = tmp, out
			outer, inner = b.Dx(), b.Dy()
		}
		for row := 0; row < outer; row++ {
			var sums [4]int
			at := func(col int) int {
				col = max(0, min(col, inner-1))
				if axis == 0 {
					return row*input.Stride + col*4
				}
				return col*input.Stride + row*4
			}
			for k := -radius; k <= radius; k++ {
				i := at(k)
				for c := 0; c < 4; c++ {
					sums[c] += int(input.Pix[i+c])
				}
			}
			for col := 0; col < inner; col++ {
				i := at(col)
				for c := 0; c < 4; c++ {
					output.Pix[i+c] = uint8(sums[c] / (2*radius + 1))
				}
				left, right := at(col-radius), at(col+radius+1)
				for c := 0; c < 4; c++ {
					sums[c] += int(input.Pix[right+c]) - int(input.Pix[left+c])
				}
			}
		}
	}
	return out
}
