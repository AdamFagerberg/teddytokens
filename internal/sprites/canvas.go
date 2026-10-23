package sprites

import (
	"math"
	"strconv"
	"strings"
)

// Canvas is a frame buffer of pixels that sprites are blitted into. It is
// cols pixels wide and 2*rows pixels tall, so each terminal cell covers two
// vertically stacked pixels.
type Canvas struct {
	w, h int
	pix  []Color
}

// NewCanvas returns a transparent canvas covering cols x rows terminal cells.
func NewCanvas(cols, rows int) *Canvas {
	return &Canvas{w: cols, h: 2 * rows, pix: make([]Color, cols*2*rows)}
}

// Clear makes every pixel transparent.
func (c *Canvas) Clear() {
	clear(c.pix)
}

// BlitRotated draws the opaque pixels of s turned clockwise by angle
// radians about its centre, which lands at pixel position (cx, cy), clipping
// anything outside the canvas. Each canvas pixel takes the sprite pixel under
// its centre, so whole quarter turns draw the sprite's pixels exactly.
func (c *Canvas) BlitRotated(s *Sprite, cx, cy, angle float64) {
	cos, sin := math.Cos(angle), math.Sin(angle)
	hw, hh := float64(s.W)/2, float64(s.H)/2
	ex := math.Abs(cos)*hw + math.Abs(sin)*hh
	ey := math.Abs(sin)*hw + math.Abs(cos)*hh
	x0, x1 := max(0, int(math.Floor(cx-ex))), min(c.w, int(math.Ceil(cx+ex)))
	y0, y1 := max(0, int(math.Floor(cy-ey))), min(c.h, int(math.Ceil(cy+ey)))
	for py := y0; py < y1; py++ {
		dy := float64(py) + 0.5 - cy
		for px := x0; px < x1; px++ {
			dx := float64(px) + 0.5 - cx
			// Rotate back into the sprite's frame.
			sx := int(math.Floor(cos*dx + sin*dy + hw))
			sy := int(math.Floor(-sin*dx + cos*dy + hh))
			if sx < 0 || sx >= s.W || sy < 0 || sy >= s.H {
				continue
			}
			if p := s.at(sx, sy); p.Opaque {
				c.pix[py*c.w+px] = p
			}
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
