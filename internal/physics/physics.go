// Package physics is a small 2D engine for stacking axis-aligned,
// non-rotating boxes.
//
// All lengths are in pixels: one pixel is one terminal column wide and half a
// terminal row tall (the half-block renderer draws two pixels per cell), so a
// world for a cols x rows terminal is NewWorld(cols, 2*rows). The y axis
// points down, the floor is at y = H and the walls are at x = 0 and x = W.
//
// The solver is position based: bodies are integrated, overlaps are projected
// apart over several bottom-up iterations, and velocities are derived from the
// corrected positions. Contacts with the floor or a resting body move only the
// body on top, so stacks settle exactly instead of jittering. Bodies that stay
// still for a while fall asleep at integer pixel positions and cost nothing
// until an impact, a moving support or a resize wakes them.
package physics

import (
	"cmp"
	"math"
	"slices"
)

// Tunables, in pixels and seconds.
const (
	// TimeStep is the fixed step the world is meant to advance by.
	TimeStep = 1.0 / 60

	gravity     = 200.0 // px/s²
	restitution = 0.3
	friction    = 0.5
	iterations  = 8

	// maxSpeed keeps per-step motion below half a body so overlaps are
	// always resolved along the right axis.
	maxSpeed        = 180.0
	bounceThreshold = 20.0 // slower impacts don't bounce
	wakeSpeed       = 20.0 // faster impacts wake sleeping bodies
	sleepSpeed      = 2.0
	sleepDelay      = 0.25 // seconds below sleepSpeed before sleeping
	margin          = 1.0  // broadphase padding for support checks
	// tipAccel pushes a body whose centre overhangs its support off the
	// edge, so piles slump instead of balancing on corners.
	tipAccel = gravity
)

// Body is a box in the world. Its state is owned by the World; read it
// through the accessor methods.
type Body struct {
	x, y, w, h float64
	vx, vy     float64
	px, py     float64 // position at the start of the step
	asleep     bool
	still      float64 // seconds spent below sleepSpeed
	grounded   bool    // rests on something immovable this iteration
	floor      bool    // touched the floor this step
	wall       int     // -1 or +1 if it touched the left or right wall
	supLo      float64 // left end of the span held up by bodies below
	supHi      float64 // right end of that span
	leanL      bool    // a body or wall touches its left side
	leanR      bool    // a body or wall touches its right side
}

// Pos returns the body's top-left corner rounded to whole pixels.
func (b *Body) Pos() (x, y int) {
	return int(math.Round(b.x)), int(math.Round(b.y))
}

func (b *Body) wake() {
	if b.asleep {
		b.asleep = false
		b.px, b.py = b.x, b.y
	}
	b.still = 0
}

// invMass is 1 for a simulated body and 0 for a sleeping (immovable) one.
func (b *Body) invMass() float64 {
	if b.asleep {
		return 0
	}
	return 1
}

// World is a walled box of pixels with a floor, holding bodies.
type World struct {
	w, h     float64
	bodies   []*Body
	order    []*Body // scratch: bodies sorted by left edge
	pairs    []pair
	contacts []contact
}

// pair is a broadphase candidate; c indexes its contact or is -1.
type pair struct {
	a, b *Body
	c    int
}

// contact records a body-body collision resolved during a step. The normal
// points from a to b along x or y.
type contact struct {
	a, b     *Body
	vertical bool
	wa, wb   float64 // inverse masses used for the correction
	approach float64 // closing speed along the normal before solving
	depth    float64 // total correction distance this step
}

// NewWorld returns an empty world of w x h pixels.
func NewWorld(w, h int) *World {
	return &World{w: float64(w), h: float64(h)}
}

// Size returns the world's width and height in pixels.
func (w *World) Size() (width, height int) {
	return int(w.w), int(w.h)
}

// Bodies returns the bodies in insertion order. The slice must not be
// modified.
func (w *World) Bodies() []*Body {
	return w.bodies
}

// AddBody adds an awake width x height body with its top-left corner at (x, y) and a
// horizontal velocity of vx px/s. Bodies may start above the top (y < 0).
func (w *World) AddBody(x, y float64, width, height int, vx float64) *Body {
	b := &Body{x: x, y: y, w: float64(width), h: float64(height), vx: vx}
	b.px, b.py = x, y
	w.bodies = append(w.bodies, b)
	return b
}

// Clear removes all bodies.
func (w *World) Clear() {
	w.bodies = nil
}

// Resize moves the walls and floor and wakes every body. A rising floor
// lifts everything with it; bodies cut off by a closer right wall are dropped
// again in rows above the highest remaining body.
func (w *World) Resize(width, height int) {
	lift := min(0, float64(height)-w.h)
	w.w, w.h = float64(width), float64(height)
	top := 0.0
	var cut []*Body
	for _, b := range w.bodies {
		b.wake()
		b.y += lift
		if b.x+b.w > w.w {
			cut = append(cut, b)
		} else {
			top = min(top, b.y)
		}
	}
	x, bottom, rowH := 0.0, top, 0.0
	for _, b := range cut {
		if x > 0 && x+b.w > w.w {
			x, bottom, rowH = 0, bottom-rowH, 0
		}
		b.x, b.y, b.vx, b.vy = x, bottom-b.h, 0, 0
		x += b.w
		rowH = max(rowH, b.h)
	}
	for _, b := range w.bodies {
		b.px, b.py = b.x, b.y
	}
}

// Settled reports whether every body is asleep.
func (w *World) Settled() bool {
	for _, b := range w.bodies {
		if !b.asleep {
			return false
		}
	}
	return true
}

// Settle steps the world by TimeStep until every body sleeps or maxSteps
// have run, and reports whether it settled. Use it to build a resting pile
// before the first frame.
func (w *World) Settle(maxSteps int) bool {
	for range maxSteps {
		if w.Settled() {
			return true
		}
		w.Step(TimeStep)
	}
	return w.Settled()
}

// HighestRestingTop returns the smallest top edge, in whole pixels, of any
// sleeping body, or false if none sleeps. A value <= 0 means the resting
// pile has reached the top of the world.
func (w *World) HighestRestingTop() (int, bool) {
	top, ok := math.Inf(1), false
	for _, b := range w.bodies {
		if b.asleep && b.y < top {
			top, ok = b.y, true
		}
	}
	return int(math.Round(top)), ok
}

// Step advances the simulation by dt seconds. Call it with TimeStep.
func (w *World) Step(dt float64) {
	if w.Settled() {
		return
	}
	for _, b := range w.bodies {
		if b.asleep {
			continue
		}
		b.px, b.py = b.x, b.y
		b.vy = min(b.vy+gravity*dt, maxSpeed)
		b.x += b.vx * dt
		b.y += b.vy * dt
		b.floor, b.wall = false, 0
	}
	w.findPairs()
	w.contacts = w.contacts[:0]
	for range iterations {
		w.clampAll()
		for i := range w.pairs {
			w.solve(&w.pairs[i])
		}
	}
	w.clampAll()
	w.updateVelocities(dt)
	w.wakeNeighbours()
	w.sleep(dt)
}

// findPairs collects body pairs, not both asleep, whose margin-padded boxes
// overlap, ordered bottom-up so support propagates in a single sweep.
func (w *World) findPairs() {
	w.order = append(w.order[:0], w.bodies...)
	slices.SortFunc(w.order, func(a, b *Body) int { return cmp.Compare(a.x, b.x) })
	w.pairs = w.pairs[:0]
	for i, a := range w.order {
		for _, b := range w.order[i+1:] {
			if b.x > a.x+a.w+margin {
				break
			}
			if (a.asleep && b.asleep) || b.y > a.y+a.h+margin || a.y > b.y+b.h+margin {
				continue
			}
			w.pairs = append(w.pairs, pair{a: a, b: b, c: -1})
		}
	}
	slices.SortFunc(w.pairs, func(p, q pair) int {
		return cmp.Compare(max(q.a.y+q.a.h, q.b.y+q.b.h), max(p.a.y+p.a.h, p.b.y+p.b.h))
	})
}

// clampAll keeps awake bodies inside the walls and above the floor.
func (w *World) clampAll() {
	for _, b := range w.bodies {
		if !b.asleep {
			w.clamp(b)
		}
	}
}

func (w *World) clamp(b *Body) {
	b.grounded = false
	if b.y+b.h >= w.h {
		b.y = w.h - b.h
		b.floor, b.grounded = true, true
	}
	if b.x <= 0 {
		b.x, b.wall = 0, -1
	} else if b.x+b.w >= w.w {
		b.x, b.wall = w.w-b.w, 1
	}
}

// solve pushes an overlapping pair apart along the axis of least overlap.
func (w *World) solve(p *pair) {
	a, b := p.a, p.b
	ox := min(a.x+a.w, b.x+b.w) - max(a.x, b.x)
	oy := min(a.y+a.h, b.y+b.h) - max(a.y, b.y)
	if ox <= 0 || oy <= 0 {
		return
	}
	vertical := oy <= ox
	// Orient so the normal points from a (upper or left) to b.
	if (vertical && a.y > b.y) || (!vertical && a.x > b.x) {
		a, b = b, a
	}
	approach := a.vx - b.vx
	if vertical {
		approach = a.vy - b.vy
	}
	wa, wb := a.invMass(), b.invMass()
	if vertical && b.grounded {
		wb = 0 // shock propagation: the supporting body doesn't yield
	}
	if wa+wb == 0 {
		return
	}
	depth := ox
	if vertical {
		depth = oy
	}
	da, db := depth*wa/(wa+wb), depth*wb/(wa+wb)
	if vertical {
		a.y -= da
		b.y += db
		if wb == 0 {
			a.grounded = true
		}
	} else {
		a.x -= da
		b.x += db
	}
	if p.c < 0 {
		p.c = len(w.contacts)
		w.contacts = append(w.contacts, contact{a: a, b: b, vertical: vertical, approach: approach})
	}
	c := &w.contacts[p.c]
	c.wa, c.wb = wa, wb
	c.depth += depth
}

// updateVelocities derives velocities from the corrected positions, then
// applies restitution and Coulomb friction at the walls, floor and contacts.
func (w *World) updateVelocities(dt float64) {
	for _, b := range w.bodies {
		if b.asleep {
			continue
		}
		prevVX, prevVY := b.vx, b.vy
		b.vx, b.vy = (b.x-b.px)/dt, (b.y-b.py)/dt
		if b.floor {
			b.vy = bounce(prevVY, b.vy)
			b.vx = applyFriction(b.vx, friction*max(prevVY, 0))
		}
		if b.wall != 0 {
			b.vx = float64(b.wall) * bounce(float64(b.wall)*prevVX, float64(b.wall)*b.vx)
		}
	}
	for i := range w.contacts {
		c := &w.contacts[i]
		a, b := c.a, c.b
		an, bn, at, bt := &a.vx, &b.vx, &a.vy, &b.vy
		if c.vertical {
			an, bn, at, bt = &a.vy, &b.vy, &a.vx, &b.vx
		}
		sum := c.wa + c.wb
		if sum == 0 {
			continue
		}
		target := 0.0
		if c.approach > bounceThreshold {
			target = restitution * c.approach
		}
		if sep := *bn - *an; sep < target {
			d := target - sep
			*an -= d * c.wa / sum
			*bn += d * c.wb / sum
		}
		limit := friction * c.depth / dt
		d := max(-limit, min(limit, *bt-*at))
		*at += d * c.wa / sum
		*bt -= d * c.wb / sum
	}
	w.findSupports()
	for _, b := range w.bodies {
		if b.asleep {
			continue
		}
		if cx := b.x + b.w/2; !b.floor && b.supLo <= b.supHi {
			if cx < b.supLo && !b.leanL {
				b.vx -= tipAccel * dt
			} else if cx > b.supHi && !b.leanR {
				b.vx += tipAccel * dt
			}
		}
		b.vx = max(-maxSpeed, min(maxSpeed, b.vx))
		b.vy = max(-maxSpeed, min(maxSpeed, b.vy))
	}
}

// findSupports records, for every body, the horizontal span of the bodies
// it rests on and whether its sides touch a wall or another body. Touching
// counts, so a body bridging two corners is supported and a body leaning on
// a neighbour does not tip.
func (w *World) findSupports() {
	const touch = 0.1
	for _, b := range w.bodies {
		b.supLo, b.supHi = math.Inf(1), math.Inf(-1)
		b.leanL, b.leanR = b.x <= touch, b.x+b.w >= w.w-touch
	}
	for _, p := range w.pairs {
		a, b := p.a, p.b
		ox := min(a.x+a.w, b.x+b.w) - max(a.x, b.x)
		oy := min(a.y+a.h, b.y+b.h) - max(a.y, b.y)
		switch {
		case ox >= -touch && math.Abs(oy) <= touch:
			if a.y > b.y {
				a, b = b, a
			}
			a.supLo = min(a.supLo, max(a.x, b.x))
			a.supHi = max(a.supHi, min(a.x+a.w, b.x+b.w))
		case math.Abs(ox) <= touch && oy > margin:
			if a.x > b.x {
				a, b = b, a
			}
			a.leanR, b.leanL = true, true
		}
	}
}

// bounce returns the velocity into a static surface after contact, given the
// velocity into it before (pre) and after (post) position correction.
func bounce(pre, post float64) float64 {
	if pre > bounceThreshold {
		return -restitution * pre
	}
	return min(post, 0)
}

// applyFriction slows v toward zero by at most limit.
func applyFriction(v, limit float64) float64 {
	return v - max(-limit, min(limit, v))
}

// wakeNeighbours wakes sleeping bodies that were hit hard this step or rest
// on a body that is moving. Sleepers act as immovable during the step that
// wakes them, so impacts never pump momentum down into a pile.
func (w *World) wakeNeighbours() {
	for _, c := range w.contacts {
		if c.approach > wakeSpeed {
			c.a.wake()
			c.b.wake()
		}
	}
	for _, p := range w.pairs {
		top, below := p.a, p.b
		if top.y > below.y {
			top, below = below, top
		}
		if top.asleep && !below.asleep && math.Hypot(below.vx, below.vy) > sleepSpeed &&
			min(top.x+top.w, below.x+below.w) > max(top.x, below.x) {
			top.wake()
		}
	}
}

// sleep puts bodies that have been still for sleepDelay to rest, snapped to
// whole pixels. Rounding is monotonic and sizes are integral, so snapping
// never creates overlaps between resting bodies.
func (w *World) sleep(dt float64) {
	for _, b := range w.bodies {
		if b.asleep {
			continue
		}
		if math.Hypot(b.vx, b.vy) >= sleepSpeed {
			b.still = 0
			continue
		}
		b.still += dt
		if b.still >= sleepDelay {
			b.asleep = true
			b.vx, b.vy = 0, 0
			b.x, b.y = math.Round(b.x), math.Round(b.y)
		}
	}
}
