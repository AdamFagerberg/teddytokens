package sprites

import "math"

// MaxVariation is the largest lightness shift Tinted applies, as a fraction
// of the full HSL lightness range.
const MaxVariation = 0.08

// Tinted returns a copy of s with every color's HSL lightness shifted by f,
// clamped to [-MaxVariation, MaxVariation]. Callers should cache the result
// per character rather than tinting every frame.
func (s *Shape) Tinted(f float64) *Shape {
	f = max(-MaxVariation, min(MaxVariation, f))
	t := *s
	t.palette = make([]Color, len(s.palette))
	for i, c := range s.palette {
		h, sat, l := toHSL(c)
		t.palette[i] = fromHSL(h, sat, max(0, min(1, l+f)))
	}
	return &t
}

// toHSL converts c to hue in [0, 6), saturation and lightness in [0, 1].
func toHSL(c Color) (h, s, l float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	hi, lo := max(r, g, b), min(r, g, b)
	l = (hi + lo) / 2
	d := hi - lo
	if d == 0 {
		return 0, 0, l
	}
	s = d / (1 - math.Abs(2*l-1))
	switch hi {
	case r:
		h = math.Mod((g-b)/d+6, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h, s, l
}

// fromHSL is the inverse of toHSL and returns an opaque color.
func fromHSL(h, s, l float64) Color {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h, 2)-1))
	var r, g, b float64
	switch int(h) {
	case 0:
		r, g = c, x
	case 1:
		r, g = x, c
	case 2:
		g, b = c, x
	case 3:
		g, b = x, c
	case 4:
		r, b = x, c
	default:
		r, b = c, x
	}
	m := l - c/2
	ch := func(v float64) uint8 { return uint8(math.Round((v + m) * 255)) }
	return Color{R: ch(r), G: ch(g), B: ch(b), Opaque: true}
}
