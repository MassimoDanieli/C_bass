package app

import (
	"bytes"
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/gofont/goregular"
)

// The colours: a dark room, a rosewood neck, and one amber light for whatever is being played.
var (
	colBack    = rgb(0x16, 0x15, 0x18)
	colPanel   = rgb(0x20, 0x1e, 0x22)
	colRaised  = rgb(0x2c, 0x29, 0x2e)
	colHover   = rgb(0x39, 0x35, 0x3a)
	colLine    = rgb(0x45, 0x41, 0x46)
	colText    = rgb(0xee, 0xe7, 0xd8)
	colDim     = rgb(0x9a, 0x93, 0x86)
	colFaint   = rgb(0x62, 0x5d, 0x57)
	colAccent  = rgb(0xf4, 0xa9, 0x3a)
	colOnLight = rgb(0x1c, 0x14, 0x0a)
	colDanger  = rgb(0xe0, 0x5d, 0x4b)
	colWood    = rgb(0x3a, 0x24, 0x1d)
	colWoodLit = rgb(0x4a, 0x2f, 0x25)
	colFret    = rgb(0xb4, 0xb0, 0xa6)
	colNut     = rgb(0xe9, 0xe2, 0xcf)
	colString  = rgb(0xd9, 0xd2, 0xc2)
	colInlay   = rgb(0x8f, 0x86, 0x78)
)

func rgb(r, g, b uint8) color.RGBA { return color.RGBA{r, g, b, 0xff} }

// fade returns the colour at a fraction of its strength, for drawing over what is there.
func fade(c color.RGBA, alpha float64) color.RGBA {
	return color.RGBA{uint8(float64(c.R) * alpha), uint8(float64(c.G) * alpha), uint8(float64(c.B) * alpha), uint8(255 * alpha)}
}

type weight int

const (
	regular weight = iota
	medium
	bold
)

var fontSources = map[weight]*text.GoTextFaceSource{}

func init() {
	for w, data := range map[weight][]byte{regular: goregular.TTF, medium: gomedium.TTF, bold: gobold.TTF} {
		source, err := text.NewGoTextFaceSource(bytes.NewReader(data))
		if err != nil {
			panic(err)
		}
		fontSources[w] = source
	}
}

type align int

const (
	left align = iota
	centre
	right
)

// rect is a rectangle in the units of the window, not of the screen.
type rect struct{ x, y, w, h float32 }

func (r rect) has(x, y float32) bool { return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h }
func (r rect) inset(d float32) rect  { return rect{r.x + d, r.y + d, r.w - 2*d, r.h - 2*d} }

// pointer is what the mouse did since the last update.
type pointer struct {
	x, y              float32
	pressed, released bool // just now
	down              bool
	wheel             float32
}

// canvas draws in the units of the window and scales to the pixels of the screen. The same
// code lays a screen out twice: once to act on the mouse (dst is nil), once to paint
// (in is nil).
type canvas struct {
	dst   *ebiten.Image
	in    *pointer
	mx    float32 // where the mouse is, for showing what is under it
	my    float32
	scale float32
	w, h  float32
}

func (c *canvas) painting() bool { return c.dst != nil }

// clip returns a canvas that paints only inside r.
func (c *canvas) clip(r rect) *canvas {
	if c.dst == nil {
		return c
	}
	out := *c
	bounds := image.Rect(int(math.Round(float64(r.x*c.scale))), int(math.Round(float64(r.y*c.scale))), int(math.Round(float64((r.x+r.w)*c.scale))), int(math.Round(float64((r.y+r.h)*c.scale))))
	out.dst = c.dst.SubImage(bounds.Intersect(c.dst.Bounds())).(*ebiten.Image)
	return &out
}

func (c *canvas) fill(r rect, col color.Color) {
	if c.dst == nil || r.w <= 0 || r.h <= 0 {
		return
	}
	vector.FillRect(c.dst, r.x*c.scale, r.y*c.scale, r.w*c.scale, r.h*c.scale, col, false)
}

func roundedPath(r rect, radius, s float32) *vector.Path {
	radius = min(radius, r.w/2, r.h/2)
	x0, y0, x1, y1, q := r.x*s, r.y*s, (r.x+r.w)*s, (r.y+r.h)*s, radius*s
	var path vector.Path
	path.MoveTo(x0+q, y0)
	path.LineTo(x1-q, y0)
	path.ArcTo(x1, y0, x1, y0+q, q)
	path.LineTo(x1, y1-q)
	path.ArcTo(x1, y1, x1-q, y1, q)
	path.LineTo(x0+q, y1)
	path.ArcTo(x0, y1, x0, y1-q, q)
	path.LineTo(x0, y0+q)
	path.ArcTo(x0, y0, x0+q, y0, q)
	path.Close()
	return &path
}

func pathOptions(col color.Color) *vector.DrawPathOptions {
	options := &vector.DrawPathOptions{AntiAlias: true}
	options.ColorScale.ScaleWithColor(col)
	return options
}

// round fills a rectangle with rounded corners.
func (c *canvas) round(r rect, radius float32, col color.Color) {
	if c.dst == nil || r.w <= 0 || r.h <= 0 {
		return
	}
	vector.FillPath(c.dst, roundedPath(r, radius, c.scale), nil, pathOptions(col))
}

// outline strokes a rectangle with rounded corners.
func (c *canvas) outline(r rect, radius, width float32, col color.Color) {
	if c.dst == nil || r.w <= 0 || r.h <= 0 {
		return
	}
	vector.StrokePath(c.dst, roundedPath(r, radius, c.scale), &vector.StrokeOptions{Width: width * c.scale, LineJoin: vector.LineJoinRound}, pathOptions(col))
}

func (c *canvas) line(x0, y0, x1, y1, width float32, col color.Color) {
	if c.dst == nil {
		return
	}
	vector.StrokeLine(c.dst, x0*c.scale, y0*c.scale, x1*c.scale, y1*c.scale, width*c.scale, col, true)
}

func (c *canvas) disc(x, y, radius float32, col color.Color) {
	if c.dst == nil {
		return
	}
	vector.FillCircle(c.dst, x*c.scale, y*c.scale, radius*c.scale, col, true)
}

func (c *canvas) ring(x, y, radius, width float32, col color.Color) {
	if c.dst == nil {
		return
	}
	vector.StrokeCircle(c.dst, x*c.scale, y*c.scale, radius*c.scale, width*c.scale, col, true)
}

// polygon fills a shape given by its corners.
func (c *canvas) polygon(col color.Color, points ...float32) {
	if c.dst == nil {
		return
	}
	var path vector.Path
	path.MoveTo(points[0]*c.scale, points[1]*c.scale)
	for i := 2; i+1 < len(points); i += 2 {
		path.LineTo(points[i]*c.scale, points[i+1]*c.scale)
	}
	path.Close()
	vector.FillPath(c.dst, &path, nil, pathOptions(col))
}

func (c *canvas) face(size float32, w weight) *text.GoTextFace {
	return &text.GoTextFace{Source: fontSources[w], Size: float64(size * c.scale)}
}

// label writes one line of text: x is its left edge, centre or right edge according to a,
// and y its vertical middle.
func (c *canvas) label(s string, x, y, size float32, w weight, col color.Color, a align) {
	if c.dst == nil || s == "" {
		return
	}
	options := &text.DrawOptions{}
	options.GeoM.Translate(float64(x*c.scale), float64(y*c.scale))
	options.ColorScale.ScaleWithColor(col)
	options.PrimaryAlign = map[align]text.Align{left: text.AlignStart, centre: text.AlignCenter, right: text.AlignEnd}[a]
	options.SecondaryAlign = text.AlignCenter
	text.Draw(c.dst, s, c.face(size, w), options)
}

// width of a line of text, in the units of the window.
func (c *canvas) width(s string, size float32, w weight) float32 {
	width, _ := text.Measure(s, &text.GoTextFace{Source: fontSources[w], Size: float64(size)}, 0)
	return float32(width)
}

// fit shortens a line of text to a width, ending it with an ellipsis.
func (c *canvas) fit(s string, size float32, w weight, room float32) string {
	if c.width(s, size, w) <= room {
		return s
	}
	runes := []rune(s)
	for len(runes) > 1 {
		runes = runes[:len(runes)-1]
		if candidate := string(runes) + "…"; c.width(candidate, size, w) <= room {
			return candidate
		}
	}
	return "…"
}
