// Package icon draws the program's icon.
package icon

import (
	"image"
	"image/color"
	"math"
)

// Draw makes the icon at a given size: four strings on a dark rounded square, and the amber
// note being played on one of them. Margin is the empty border around the square, as a
// fraction of the size: macOS wants about a tenth, a window's title bar none.
func Draw(size int, margin float64) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	const sub = 4 // samples per pixel each way, for smooth edges
	n := float64(size) * (1 - 2*margin)
	offset := float64(size) * margin
	radius := n * 0.225
	shade := func(x, y float64) (color.RGBA, bool) {
		x, y = x-offset, y-offset
		if x < 0 || y < 0 || x > n || y > n {
			return color.RGBA{}, false
		}
		// the rounded square
		dx := math.Max(0, math.Max(radius-x, x-(n-radius)))
		dy := math.Max(0, math.Max(radius-y, y-(n-radius)))
		if dx*dx+dy*dy > radius*radius {
			return color.RGBA{}, false
		}
		out := color.RGBA{0x20, 0x1e, 0x22, 0xff}
		for i := 0; i < 4; i++ {
			at := n * (0.26 + 0.16*float64(i))
			thickness := n * (0.008 + 0.005*float64(i))
			if math.Abs(y-at) < thickness {
				out = color.RGBA{0xd9, 0xd2, 0xc2, 0xff}
			}
		}
		if d := math.Hypot(x-n*0.60, y-n*0.58); d < n*0.135 {
			out = color.RGBA{0xf4, 0xa9, 0x3a, 0xff}
		} else if d < n*0.165 {
			out = color.RGBA{0x20, 0x1e, 0x22, 0xff}
		}
		return out, true
	}
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var r, g, b, a float64
			for sy := 0; sy < sub; sy++ {
				for sx := 0; sx < sub; sx++ {
					if col, ok := shade(float64(px)+(float64(sx)+0.5)/sub, float64(py)+(float64(sy)+0.5)/sub); ok {
						r, g, b, a = r+float64(col.R), g+float64(col.G), b+float64(col.B), a+255
					}
				}
			}
			const samples = sub * sub
			img.SetRGBA(px, py, color.RGBA{uint8(r / samples), uint8(g / samples), uint8(b / samples), uint8(a / samples)})
		}
	}
	return img
}
