package sprites

import (
	"math"
	"slices"
	"strconv"
	"strings"
)

// Canvas is a frame buffer of pixels that shapes are drawn into. It is
// cols pixels wide and 2*rows pixels tall, so each terminal cell covers two
// vertically stacked pixels.
type Canvas struct {
	w, h int
	pix  []Color

	// Scratch space for Draw, kept to avoid allocating every frame.
	votes  []*rect
	counts []int
}

// NewCanvas returns a transparent canvas covering cols x rows terminal cells.
func NewCanvas(cols, rows int) *Canvas {
	return &Canvas{w: cols, h: 2 * rows, pix: make([]Color, cols*2*rows)}
}

// Clear makes every pixel transparent.
func (c *Canvas) Clear() {
	clear(c.pix)
}

// samples is how many points per axis Draw looks at in each canvas pixel.
const samples = 4

// Draw paints s enlarged scale times and turned clockwise by angle radians
// about its centre, which lands at pixel position (cx, cy), clipping
// anything outside the canvas. Colors are never blended, since the
// terminal's background is unknown.
//
// Each canvas pixel maps a grid of samples x samples points inside it back
// into the shape. The pixel stays transparent unless at least half the
// points land on a rect, and otherwise takes the color most of them land on,
// so edges and color boundaries fall where the shape's area does.
//
// That vote would lose small features: a rect no bigger than one canvas
// pixel (an eye, a leg) spreads over up to four pixels when turned and may
// win none of them. Such a rect is a detail. It does not vote, and is drawn
// instead as exactly the one pixel under its centre, unless a later rect
// covers that point. So details neither vanish nor grow at any angle.
//
// Upright or at quarter turns, with the centre on whole pixels, all points
// of a pixel fall in the same rect and each detail's centre in its own
// pixel, so the result is exactly the pixel art.
func (c *Canvas) Draw(s *Shape, cx, cy, angle float64, scale int) {
	k := float64(scale)
	cos, sin := math.Cos(angle), math.Sin(angle)
	w, h := float64(s.W), float64(s.H)
	hw, hh := w/2, h/2
	ex := (math.Abs(cos)*hw + math.Abs(sin)*hh) * k
	ey := (math.Abs(sin)*hw + math.Abs(cos)*hh) * k
	x0, x1 := max(0, int(math.Floor(cx-ex))), min(c.w, int(math.Ceil(cx+ex)))
	y0, y1 := max(0, int(math.Floor(cy-ey))), min(c.h, int(math.Ceil(cy+ey)))

	detail := 1 / (k * k) // largest area of a detail, in shape units
	c.votes = c.votes[:0]
	for i := len(s.rects) - 1; i >= 0; i-- { // topmost first
		if r := &s.rects[i]; r.w*r.h > detail {
			c.votes = append(c.votes, r)
		}
	}
	c.counts = slices.Grow(c.counts[:0], len(s.palette))[:len(s.palette)]
	counts := c.counts

	// One step between samples moves this far in the shape, along a
	// canvas row (ux, vx) and down a canvas column (uy, vy).
	const step = 1.0 / samples
	ux, vx := cos/k*step, -sin/k*step
	uy, vy := sin/k*step, cos/k*step
	for py := y0; py < y1; py++ {
		dy := float64(py) + step/2 - cy
		for px := x0; px < x1; px++ {
			dx := float64(px) + step/2 - cx
			// The pixel's first sample, rotated back into the shape.
			u0 := (cos*dx+sin*dy)/k + hw
			v0 := (-sin*dx+cos*dy)/k + hh
			clear(counts)
			hits := 0
			for j := range samples {
				u, v := u0+float64(j)*uy, v0+float64(j)*vy
				for range samples {
					if u >= 0 && u < w && v >= 0 && v < h {
						for _, r := range c.votes {
							if r.contains(u, v) {
								counts[r.c]++
								hits++
								break
							}
						}
					}
					u, v = u+ux, v+vx
				}
			}
			if 2*hits < samples*samples {
				continue
			}
			best := 0
			for i, n := range counts {
				if n >= counts[best] { // ties go to the later color
					best = i
				}
			}
			c.pix[py*c.w+px] = s.palette[best]
		}
	}

	for i := range s.rects {
		r := &s.rects[i]
		u, v := r.x+r.w/2, r.y+r.h/2
		if r.w*r.h > detail || s.topmost(u, v) != i {
			continue
		}
		du, dv := (u-hw)*k, (v-hh)*k
		px := int(math.Floor(cx + cos*du - sin*dv))
		py := int(math.Floor(cy + sin*du + cos*dv))
		if px >= 0 && px < c.w && py >= 0 && py < c.h {
			c.pix[py*c.w+px] = s.palette[r.c]
		}
	}
}

// Render returns the canvas as newline-separated terminal lines using
// half-block glyphs and truecolor escapes. Escapes are only emitted when the
// active color changes, fully transparent cells are plain spaces on the
// terminal's own background, and each line resets its attributes at the end.
func (c *Canvas) Render() string {
	var b strings.Builder
	for row := range c.h / 2 {
		if row > 0 {
			b.WriteByte('\n')
		}
		c.renderLine(&b, row)
	}
	return b.String()
}

func (c *Canvas) renderLine(b *strings.Builder, row int) {
	var fg, bg Color // active colors; transparent means terminal default
	setFG := func(col Color) {
		if col != fg {
			writeColor(b, 38, col)
			fg = col
		}
	}
	setBG := func(col Color) {
		if col != bg {
			if col.Opaque {
				writeColor(b, 48, col)
			} else {
				b.WriteString("\x1b[49m")
			}
			bg = col
		}
	}
	top := c.pix[2*row*c.w:][:c.w]
	bot := c.pix[(2*row+1)*c.w:][:c.w]
	for x := range c.w {
		t, u := top[x], bot[x]
		switch {
		case !t.Opaque && !u.Opaque:
			setBG(Color{})
			b.WriteByte(' ')
		case t == u:
			setFG(t)
			b.WriteString("█")
		case !u.Opaque:
			setFG(t)
			setBG(Color{})
			b.WriteString("▀")
		case !t.Opaque:
			setFG(u)
			setBG(Color{})
			b.WriteString("▄")
		case fg == u || bg == t:
			// Reuse the active colors with the lower half block.
			setFG(u)
			setBG(t)
			b.WriteString("▄")
		default:
			setFG(t)
			setBG(u)
			b.WriteString("▀")
		}
	}
	if fg.Opaque || bg.Opaque {
		b.WriteString("\x1b[0m")
	}
}

// writeColor writes an SGR truecolor escape; layer is 38 (fg) or 48 (bg).
func writeColor(b *strings.Builder, layer int, c Color) {
	b.WriteString("\x1b[")
	b.WriteString(strconv.Itoa(layer))
	b.WriteString(";2;")
	b.WriteString(strconv.Itoa(int(c.R)))
	b.WriteByte(';')
	b.WriteString(strconv.Itoa(int(c.G)))
	b.WriteByte(';')
	b.WriteString(strconv.Itoa(int(c.B)))
	b.WriteByte('m')
}
