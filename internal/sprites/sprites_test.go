package sprites

import (
	"math"
	"testing"
)

func TestSpritesShareFootprint(t *testing.T) {
	if Claude.W != 14 || Claude.H != 8 {
		t.Errorf("Claude is %dx%d, want 14x8", Claude.W, Claude.H)
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
	red, blue := Hex(0xFF0000), Hex(0x0000FF)
	s := &Sprite{W: 3, H: 2, pix: []Color{
		red, red, {},
		red, blue, blue,
	}}
	c := NewCanvas(5, 1)
	c.Blit(s, 1, 0)
	c.Blit(s, -2, 0) // clipped to a single blue pixel at (0, 1)

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
