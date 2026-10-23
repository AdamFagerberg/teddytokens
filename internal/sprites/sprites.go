// Package sprites holds the characters and a half-block canvas that
// composites them into ANSI truecolor terminal output.
//
// The characters are vector shapes loaded from the SVG files in this
// directory. They are measured in pixels: one pixel is one terminal column
// wide and half a terminal row tall, because each cell shows two vertical
// pixels.
package sprites

import (
	_ "embed"
	"encoding/xml"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Both shapes share one 9x6 footprint so toggling the skin never changes a
// physics body's size.
var (
	//go:embed claude.svg
	claudeSVG []byte
	//go:embed teddy.svg
	teddySVG []byte
)

// Claude is the orange Claude Code mascot.
var Claude = mustParse("claude.svg", claudeSVG)

// Teddy is a brown teddy bear head with a tan muzzle and inner ears.
var Teddy = mustParse("teddy.svg", teddySVG)

// Color is a truecolor pixel. The zero value is transparent.
type Color struct {
	R, G, B uint8
	Opaque  bool
}

// hex returns the opaque color 0xRRGGBB.
func hex(rgb uint32) Color {
	return Color{R: uint8(rgb >> 16), G: uint8(rgb >> 8), B: uint8(rgb), Opaque: true}
}

// Shape is an immutable W x H vector image: axis-aligned rectangles painted
// in order, later ones on top.
type Shape struct {
	W, H    int
	rects   []rect
	palette []Color // distinct rect colors, in order of first use
}

// rect is a filled rectangle in shape units; c indexes the palette.
type rect struct {
	x, y, w, h float64
	c          int
}

// contains reports whether the point (u, v) is inside r.
func (r *rect) contains(u, v float64) bool {
	return u >= r.x && u < r.x+r.w && v >= r.y && v < r.y+r.h
}

// svg is the subset of SVG the shapes are written in: a viewBox and <rect>
// children with a #RRGGBB fill. Anything else is rejected rather than
// silently dropped.
type svg struct {
	ViewBox string `xml:"viewBox,attr"`
	Rects   []struct {
		X      string `xml:"x,attr"`
		Y      string `xml:"y,attr"`
		Width  string `xml:"width,attr"`
		Height string `xml:"height,attr"`
		Fill   string `xml:"fill,attr"`
	} `xml:"rect"`
	Other []struct {
		XMLName xml.Name
	} `xml:",any"`
}

// mustParse loads an embedded SVG and panics if it is not valid, since a
// broken asset is a build mistake.
func mustParse(name string, data []byte) *Shape {
	s, err := parse(data)
	if err != nil {
		panic(fmt.Sprintf("sprites: %s: %v", name, err))
	}
	return s
}

func parse(data []byte) (*Shape, error) {
	var doc svg
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if len(doc.Other) > 0 {
		return nil, fmt.Errorf("unsupported element <%s>, only <rect> is allowed", doc.Other[0].XMLName.Local)
	}
	vb := strings.Fields(strings.ReplaceAll(doc.ViewBox, ",", " "))
	if len(vb) != 4 || vb[0] != "0" || vb[1] != "0" {
		return nil, fmt.Errorf("viewBox %q must be \"0 0 W H\"", doc.ViewBox)
	}
	s := &Shape{}
	var err1, err2 error
	s.W, err1 = strconv.Atoi(vb[2])
	s.H, err2 = strconv.Atoi(vb[3])
	if err1 != nil || err2 != nil || s.W <= 0 || s.H <= 0 {
		return nil, fmt.Errorf("viewBox %q needs a whole positive width and height", doc.ViewBox)
	}
	for i, e := range doc.Rects {
		var v [4]float64
		for j, a := range []string{e.X, e.Y, e.Width, e.Height} {
			f, err := strconv.ParseFloat(a, 64)
			if err != nil {
				return nil, fmt.Errorf("rect %d: %v", i, err)
			}
			v[j] = f
		}
		r := rect{x: v[0], y: v[1], w: v[2], h: v[3]}
		if r.w <= 0 || r.h <= 0 || r.x < 0 || r.y < 0 || r.x+r.w > float64(s.W) || r.y+r.h > float64(s.H) {
			return nil, fmt.Errorf("rect %d is empty or outside the viewBox", i)
		}
		rgb, err := strconv.ParseUint(strings.TrimPrefix(e.Fill, "#"), 16, 32)
		if len(e.Fill) != 7 || e.Fill[0] != '#' || err != nil {
			return nil, fmt.Errorf("rect %d: fill %q is not #RRGGBB", i, e.Fill)
		}
		c := hex(uint32(rgb))
		r.c = slices.Index(s.palette, c)
		if r.c < 0 {
			r.c = len(s.palette)
			s.palette = append(s.palette, c)
		}
		s.rects = append(s.rects, r)
	}
	if len(s.rects) == 0 {
		return nil, fmt.Errorf("no <rect> elements")
	}
	return s, nil
}

// topmost returns the index of the last rect containing (u, v), or -1.
func (s *Shape) topmost(u, v float64) int {
	for i := len(s.rects) - 1; i >= 0; i-- {
		if s.rects[i].contains(u, v) {
			return i
		}
	}
	return -1
}
