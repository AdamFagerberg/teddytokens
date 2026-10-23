package sprites

import (
	"math"
	"slices"
	"testing"
)

func TestSpritesShareFootprint(t *testing.T) {
	if Claude.W != 9 || Claude.H != 6 {
		t.Errorf("Claude is %dx%d, want 9x6", Claude.W, Claude.H)
	}
	if Teddy.W != Claude.W || Teddy.H != Claude.H {
		t.Errorf("Teddy is %dx%d, want %dx%d", Teddy.W, Teddy.H, Claude.W, Claude.H)
	}
	for _, s := range []*Sprite{Claude, Teddy} {
		if len(s.pix) != s.W*s.H {
			t.Errorf("sprite has %d pixels, want %d", len(s.pix), s.W*s.H)
		}
	}
}

func TestRender(t *testing.T) {
	red, blue := hex(0xFF0000), hex(0x0000FF)
	s := &Sprite{W: 3, H: 2, pix: []Color{
		red, red, {},
		red, blue, blue,
	}}
	c := NewCanvas(5, 1)
	c.BlitRotated(s, 2.5, 1, 0)
	c.BlitRotated(s, -0.5, 1, 0) // clipped to a single blue pixel at (0, 1)

	const (
		fgRed  = "\x1b[38;2;255;0;0m"
		fgBlue = "\x1b[38;2;0;0;255m"
		bgBlue = "\x1b[48;2;0;0;255m"
	)
	want := fgBlue + "▄" + fgRed + "█" + bgBlue + "▀" + fgBlue + "\x1b[49m▄ \x1b[0m"
	if got := c.Render(); got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}

	c.Clear()
	if got := NewCanvas(2, 2).Render(); got != "  \n  " {
		t.Errorf("empty Render() = %q", got)
	}
	if got := c.Render(); got != "     " {
		t.Errorf("cleared Render() = %q", got)
	}
}

func TestTintedStaysInRange(t *testing.T) {
	for _, f := range []float64{-1, -MaxVariation, -0.03, 0, 0.05, MaxVariation, 1} {
		tinted := Teddy.Tinted(f)
		for i, c := range Teddy.pix {
			got := tinted.pix[i]
			if got.Opaque != c.Opaque {
				t.Fatalf("Tinted(%v) changed transparency of pixel %d", f, i)
			}
			if !c.Opaque {
				continue
			}
			_, _, l0 := toHSL(c)
			_, _, l1 := toHSL(got)
			if d := math.Abs(l1 - l0); d > MaxVariation+0.01 {
				t.Errorf("Tinted(%v) shifted lightness by %.3f", f, d)
			}
		}
	}
	if Claude.Tinted(0).pix[16] != Claude.pix[16] {
		t.Error("Tinted(0) should round-trip colors exactly")
	}
}

func TestScaled(t *testing.T) {
	red, blue := hex(0xFF0000), hex(0x0000FF)
	s := &Sprite{W: 2, H: 1, pix: []Color{red, blue}}
	got := s.Scaled(2)
	want := []Color{red, red, blue, blue, red, red, blue, blue}
	if got.W != 4 || got.H != 2 || !slices.Equal(got.pix, want) {
		t.Errorf("Scaled(2) = %dx%d %v, want 4x2 %v", got.W, got.H, got.pix, want)
	}
	if big := Claude.Scaled(5); big.W != 45 || big.H != 30 || big.at(44, 29) != Claude.at(8, 5) {
		t.Errorf("Claude.Scaled(5) is %dx%d", big.W, big.H)
	}
}

// blit draws s at angle on a w x h canvas centred on (cx, cy) and returns
// the canvas's pixels.
func blit(s *Sprite, w, h int, cx, cy, angle float64) []Color {
	c := &Canvas{w: w, h: h, pix: make([]Color, w*h)}
	c.BlitRotated(s, cx, cy, angle)
	return c.pix
}

func TestBlitRotatedQuarterTurns(t *testing.T) {
	s := Claude
	w, h := s.W, s.H
	// Expected pixels of s turned clockwise q quarter turns, filling a
	// canvas of the turned size exactly.
	turned := func(q int) []Color {
		var out []Color
		tw, th := w, h
		if q%2 != 0 {
			tw, th = h, w
		}
		for y := range th {
			for x := range tw {
				var p Color
				switch q {
				case 0:
					p = s.at(x, y)
				case 1:
					p = s.at(y, h-1-x)
				case 2:
					p = s.at(w-1-x, h-1-y)
				case 3:
					p = s.at(w-1-y, x)
				}
				out = append(out, p)
			}
		}
		return out
	}
	for q, angle := range []float64{0, math.Pi / 2, math.Pi, -math.Pi / 2} {
		tw, th := w, h
		if q%2 != 0 {
			tw, th = h, w
		}
		got := blit(s, tw, th, float64(tw)/2, float64(th)/2, angle)
		if !slices.Equal(got, turned(q)) {
			t.Errorf("BlitRotated at %v rad does not match a %d-quarter turn", angle, q)
		}
	}
	// A whole-pixel offset moves the sprite by whole pixels.
	if got := blit(s, w+3, h+2, float64(w)/2+3, float64(h)/2+2, 0); got[(h+1)*(w+3)+w+2] != s.at(w-1, h-1) {
		t.Error("offset blit misplaced the sprite")
	}
}

func TestBlitRotated45(t *testing.T) {
	red := hex(0xFF0000)
	s := &Sprite{W: 4, H: 4, pix: slices.Repeat([]Color{red}, 16)}
	got := blit(s, 8, 8, 4, 4, math.Pi/4)
	// A square turned 45° is a diamond: the centre and the tips along the
	// axes are filled, the corners of its bounding box are not.
	for _, p := range [][2]int{{3, 3}, {4, 4}, {2, 3}, {5, 4}, {3, 2}, {4, 5}} {
		if !got[p[1]*8+p[0]].Opaque {
			t.Errorf("pixel %v of the diamond is empty", p)
		}
	}
	for _, p := range [][2]int{{1, 1}, {6, 1}, {1, 6}, {6, 6}} {
		if got[p[1]*8+p[0]].Opaque {
			t.Errorf("corner pixel %v is filled", p)
		}
	}
}
