// Package sprites holds the pixel-art characters and a half-block canvas
// that composites them into ANSI truecolor terminal output.
//
// Sprites are measured in pixels: one pixel is one terminal column wide and
// half a terminal row tall, because each cell shows two vertical pixels.
package sprites

import (
	"fmt"
	"strings"
)

// Both sprites share one footprint so toggling the skin never changes a
// physics body's size. Claude's 14x7 grid gets a transparent top row; the
// half-row gap it leaves between stacked bodies reads as a natural seam.
const claudeGrid = `
..............
..XXXXXXXXXX..
..XXEXXXXEXX..
..XXEXXXXEXX..
XXXXXXXXXXXXXX
..XXXXXXXXXX..
..XXXXXXXXXX..
...X.X..X.X...
`

const teddyGrid = `
.BBB......BBB.
BTTBBBBBBBBTTB
BTBBBBBBBBBBTB
.BBKBBBBBBKBB.
.BBKBTTTTBKBB.
.BBBTTKKTTBBB.
..BBBTTTTBBB..
....BBBBBB....
`

// Claude is the orange Claude Code mascot.
var Claude = mustParse(claudeGrid, map[byte]Color{
	'X': hex(0xD97757),
	'E': hex(0x2B2B2B),
})

// Teddy is a brown teddy bear head with a tan muzzle and inner ears.
var Teddy = mustParse(teddyGrid, map[byte]Color{
	'B': hex(0x8B5A2B),
	'T': hex(0xD2A679),
	'K': hex(0x1E1E1E),
})

// Color is a truecolor pixel. The zero value is transparent.
type Color struct {
	R, G, B uint8
	Opaque  bool
}

// hex returns the opaque color 0xRRGGBB.
func hex(rgb uint32) Color {
	return Color{R: uint8(rgb >> 16), G: uint8(rgb >> 8), B: uint8(rgb), Opaque: true}
}

// Sprite is an immutable W x H grid of pixels.
type Sprite struct {
	W, H int
	pix  []Color
}

// at returns the pixel at (x, y), which must be inside the sprite.
func (s *Sprite) at(x, y int) Color {
	return s.pix[y*s.W+x]
}

// mustParse builds a sprite from a grid of equal-length lines, where '.' is
// transparent and every other byte is looked up in palette.
func mustParse(grid string, palette map[byte]Color) *Sprite {
	lines := strings.Split(strings.Trim(grid, "\n"), "\n")
	s := &Sprite{W: len(lines[0]), H: len(lines)}
	for y, line := range lines {
		if len(line) != s.W {
			panic(fmt.Sprintf("sprites: row %d has width %d, want %d", y, len(line), s.W))
		}
		for x := range len(line) {
			c, ok := palette[line[x]]
			if !ok && line[x] != '.' {
				panic(fmt.Sprintf("sprites: unknown pixel %q at %d,%d", line[x], x, y))
			}
			s.pix = append(s.pix, c)
		}
	}
	return s
}

// Scaled returns s enlarged k times, each pixel repeated in a k x k block.
func (s *Sprite) Scaled(k int) *Sprite {
	t := &Sprite{W: s.W * k, H: s.H * k, pix: make([]Color, 0, s.W*s.H*k*k)}
	for y := range t.H {
		for x := range t.W {
			t.pix = append(t.pix, s.at(x/k, y/k))
		}
	}
	return t
}
