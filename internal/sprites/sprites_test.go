package sprites

import (
	"maps"
	"math"
	"slices"
	"strings"
	"testing"
)

// The pixel-art grids the shapes were drawn from, where '.' is transparent.
// Upright, the SVGs must still draw exactly these.
var fixtures = []struct {
	name    string
	shape   *Shape
	grid    string
	palette map[byte]Color
	eye     Color
	eyes    int // eye-colored pixels at any angle
}{
	{"claude", Claude, `
.XXXXXXX.
.XEXXXEX.
XXXXXXXXX
.XXXXXXX.
.XXXXXXX.
.X.X.X.X.
`, map[byte]Color{'X': hex(0xD97757), 'E': hex(0x2B2B2B)}, hex(0x2B2B2B), 2},
	{"teddy", Teddy, `
BB.....BB
BTBBBBBTB
.BKBBBKB.
.BBTTTBB.
.BBTKTBB.
..BBBBB..
`, map[byte]Color{'B': hex(0x8B5A2B), 'T': hex(0xD2A679), 'K': hex(0x1E1E1E)}, hex(0x1E1E1E), 3},
}

// pixels returns a fixture's grid as a row-major slice and its width.
func pixels(grid string, palette map[byte]Color) ([]Color, int) {
	lines := strings.Fields(grid)
	var pix []Color
	for _, l := range lines {
		for i := range len(l) {
			pix = append(pix, palette[l[i]])
		}
	}
	return pix, len(lines[0])
}

// draw paints s on a w x h canvas and returns the canvas's pixels.
func draw(s *Shape, w, h int, cx, cy, angle float64, scale int) []Color {
	c := &Canvas{w: w, h: h, pix: make([]Color, w*h)}
	c.Draw(s, cx, cy, angle, scale)
	return c.pix
}

func TestParse(t *testing.T) {
	for _, f := range fixtures {
		if f.shape.W != 9 || f.shape.H != 6 {
			t.Errorf("%s is %dx%d, want 9x6", f.name, f.shape.W, f.shape.H)
		}
		for _, c := range f.shape.palette {
			if !slices.Contains(slices.Collect(maps.Values(f.palette)), c) {
				t.Errorf("%s has unexpected color %v", f.name, c)
			}
		}
		if len(f.shape.palette) != len(f.palette) {
			t.Errorf("%s has %d colors, want %d", f.name, len(f.shape.palette), len(f.palette))
		}
	}
	bad := []string{
		`<svg viewBox="0 0 2 2"><circle r="1"/></svg>`,
		`<svg viewBox="0 0 2 2"><rect x="0" y="0" width="1" height="1" fill="red"/></svg>`,
		`<svg viewBox="0 0 2 2"><rect x="1" y="0" width="2" height="1" fill="#FF0000"/></svg>`,
		`<svg viewBox="0 0 2 2"><rect x="0" y="0" width="1" fill="#FF0000"/></svg>`,
		`<svg viewBox="1 0 2 2"><rect x="0" y="0" width="1" height="1" fill="#FF0000"/></svg>`,
		`<svg viewBox="0 0 2 2"></svg>`,
		`<svg viewBox="0 0 2 2"><rect`,
	}
	for _, b := range bad {
		if _, err := parse([]byte(b)); err == nil {
			t.Errorf("parse(%s) accepted a malformed shape", b)
		}
	}
}

func TestUprightMatchesPixelArt(t *testing.T) {
	for _, f := range fixtures {
		want, w := pixels(f.grid, f.palette)
		h := len(want) / w
		at := func(x, y int) Color { return want[y*w+x] }
		for _, k := range []int{1, 2, 5, 8} {
			for q, angle := range []float64{0, math.Pi / 2, math.Pi, -math.Pi / 2} {
				// The fixture turned clockwise q quarter turns and
				// enlarged k times, filling a canvas of that size.
				tw, th := w, h
				if q%2 != 0 {
					tw, th = h, w
				}
				var turned []Color
				for y := range th * k {
					for x := range tw * k {
						x, y := x/k, y/k
						var p Color
						switch q {
						case 0:
							p = at(x, y)
						case 1:
							p = at(y, h-1-x)
						case 2:
							p = at(w-1-x, h-1-y)
						case 3:
							p = at(w-1-y, x)
						}
						turned = append(turned, p)
					}
				}
				got := draw(f.shape, tw*k, th*k, float64(tw*k)/2, float64(th*k)/2, angle, k)
				if !slices.Equal(got, turned) {
					t.Errorf("%s at %dx, %v rad does not match the pixel art", f.name, k, angle)
				}
			}
		}
		// A whole-pixel offset moves the shape by whole pixels.
		got := draw(f.shape, w+3, h+2, float64(w)/2+3, float64(h)/2+2, 0, 1)
		for y := range h {
			if !slices.Equal(got[(y+2)*(w+3)+3:][:w], want[y*w:][:w]) {
				t.Errorf("%s offset draw misplaced row %d", f.name, y)
			}
		}
	}
}

func TestRotatedKeepsDetails(t *testing.T) {
	for _, f := range fixtures {
		want, _ := pixels(f.grid, f.palette)
		area := 0
		for _, p := range want {
			if p.Opaque {
				area++
			}
		}
		for deg := 0; deg < 360; deg += 5 {
			for _, off := range []float64{0, 0.3, 0.5} {
				angle := float64(deg) * math.Pi / 180
				const n = 20
				cx, cy := n/2+off, n/2+off/2
				got := draw(f.shape, n, n, cx, cy, angle, 1)
				eyes, opaque := 0, 0
				for i, p := range got {
					if !p.Opaque {
						continue
					}
					opaque++
					if p == f.eye {
						eyes++
					}
					// Every pixel drawn overlaps the turned shape's bounding box.
					x, y := float64(i%n)+0.5, float64(i/n)+0.5
					ex := math.Abs(math.Cos(angle))*4.5 + math.Abs(math.Sin(angle))*3
					ey := math.Abs(math.Sin(angle))*4.5 + math.Abs(math.Cos(angle))*3
					if math.Abs(x-cx) >= ex+0.5 || math.Abs(y-cy) >= ey+0.5 {
						t.Errorf("%s at %d° drew pixel %d,%d outside its box", f.name, deg, i%n, i/n)
					}
				}
				if eyes != f.eyes {
					t.Errorf("%s at %d°+%v has %d eye pixels, want %d", f.name, deg, off, eyes, f.eyes)
				}
				if d := math.Abs(float64(opaque-area)) / float64(area); d > 0.25 {
					t.Errorf("%s at %d°+%v covers %d pixels, want about %d", f.name, deg, off, opaque, area)
				}
			}
		}
	}
}

func TestRotated45IsADiamond(t *testing.T) {
	red := hex(0xFF0000)
	s := &Shape{W: 4, H: 4, rects: []rect{{0, 0, 4, 4, 0}}, palette: []Color{red}}
	got := draw(s, 8, 8, 4, 4, math.Pi/4, 1)
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

func TestRender(t *testing.T) {
	red, blue := hex(0xFF0000), hex(0x0000FF)
	// .. red red .
	// .. red blue blue
	s := &Shape{W: 3, H: 2, rects: []rect{{0, 0, 2, 2, 0}, {1, 1, 2, 1, 1}}, palette: []Color{red, blue}}
	c := NewCanvas(5, 1)
	c.Draw(s, 2.5, 1, 0, 1)
	c.Draw(s, -0.5, 1, 0, 1) // clipped to a single blue pixel at (0, 1)

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
		for i, c := range Teddy.palette {
			_, _, l0 := toHSL(c)
			_, _, l1 := toHSL(tinted.palette[i])
			if d := math.Abs(l1 - l0); d > MaxVariation+0.01 {
				t.Errorf("Tinted(%v) shifted lightness by %.3f", f, d)
			}
		}
	}
	if !slices.Equal(Claude.Tinted(0).palette, Claude.palette) {
		t.Error("Tinted(0) should round-trip colors exactly")
	}
}

// BenchmarkFrame draws a busy frame: 300 small characters, a big and a
// giant, all at 45°.
func BenchmarkFrame(b *testing.B) {
	c := NewCanvas(200, 50)
	for b.Loop() {
		c.Clear()
		for i := range 300 {
			s := Claude
			if i%2 == 1 {
				s = Teddy
			}
			c.Draw(s, float64(i%20*10+5), float64(i/20*6+5), math.Pi/4, 1)
		}
		c.Draw(Teddy, 60, 50, math.Pi/4, 5)
		c.Draw(Teddy, 140, 50, math.Pi/4, 8)
	}
}
