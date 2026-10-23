// Package physics is a small 2D rigid body engine for boxes that fall,
// tumble and pile up.
//
// All lengths are in pixels: one pixel is one terminal column wide and half a
// terminal row tall (the half-block renderer draws two pixels per cell), so a
// world for a cols x rows terminal is NewWorld(cols, 2*rows). The y axis
// points down, the floor is at y = H and the walls are at x = 0 and x = W.
// Angles are in radians, clockwise on screen.
//
// The solver uses sequential impulses in the style of Box2D-Lite: contacts
// between boxes come from the separating axis test and face clipping, each
// step applies normal and friction impulses at every contact point several
// times, warm started from the previous step's impulses, so off-centre
// impacts spin bodies and piles come to rest at any angle. Bodies that stay
// still for a while fall asleep, keep their exact pose and act as immovable
// until an impact, a moving support, a removal or a resize wakes them.
package physics

import (
	"cmp"
	"math"
	"math/rand/v2"
	"slices"
)

// Tunables, in pixels and seconds.
const (
	// TimeStep is the fixed step the world is meant to advance by.
	TimeStep = 1.0 / 60

	gravity     = 200.0 // px/s²
	iterations  = 10
	friction    = 0.5
	restitution = 0.3

	// maxSpeed caps how fast any point of a body moves, keeping per-step
	// motion below margin and half the smallest body.
	maxSpeed = 150.0
	// margin is how far apart surfaces get contacts, so they can't pass
	// through each other within a step.
	margin          = 3.0
	slop            = 0.5 // overlap left alone, so resting contacts persist
	baumgarte       = 0.2 // fraction of deeper overlap removed per step
	bounceThreshold = 20.0
	linearDamping   = 0.05 // per second
	angularDamping  = 0.5  // per second
	wakeSpeed       = 20.0 // faster impacts wake sleeping bodies
	sleepSpeed      = 2.0
	sleepDelay      = 0.5 // seconds below sleepSpeed before sleeping
	shakeSpeed      = 140.0
	shakeSpin       = 6.0 // rad/s
)

// World is a walled box of pixels with a floor, holding rigid bodies.
type World struct {
	w, h   float64
	bodies []*Rigidbody
	nextID int
	ground Rigidbody // the immovable other side of floor and wall contacts

	order            []*Rigidbody // scratch: bodies sorted by left edge
	arbs, prev       []arbiter
	index, prevIndex map[pairKey]int // arbiter of each touching pair in arbs, prev
}

// arbiter holds the contacts between a pair of bodies, or between a body
// and the floor or a wall.
type arbiter struct {
	a, b *Rigidbody
	n    int
	c    [2]contact
}

// pairKey names an arbiter; plane tells floor and walls apart when b is
// the ground.
type pairKey struct {
	a, b  *Rigidbody
	plane int
}

// NewWorld returns an empty world of w x h pixels.
func NewWorld(w, h int) *World {
	world := &World{w: float64(w), h: float64(h), index: map[pairKey]int{}, prevIndex: map[pairKey]int{}}
	world.ground.asleep = true
	return world
}

// Size returns the world's width and height in pixels.
func (w *World) Size() (width, height int) {
	return int(w.w), int(w.h)
}

// Bodies returns the bodies in the order they were added. The slice must
// not be modified.
func (w *World) Bodies() []*Rigidbody {
	return w.bodies
}

// Add puts b into the world. Bodies may start above the top (y < 0).
func (w *World) Add(b *Rigidbody) {
	b.id = w.nextID
	w.nextID++
	w.bodies = append(w.bodies, b)
}

// Remove takes b out of the world and wakes the bodies touching it, since
// anything it held up must fall.
func (w *World) Remove(b *Rigidbody) {
	w.bodies = slices.DeleteFunc(w.bodies, func(o *Rigidbody) bool { return o == b })
	lo, hi := b.aabb(margin)
	for _, o := range w.bodies {
		if olo, ohi := o.aabb(0); olo.x < hi.x && lo.x < ohi.x && olo.y < hi.y && lo.y < ohi.y {
			o.wake()
		}
	}
}

// Clear removes all bodies.
func (w *World) Clear() {
	w.bodies = nil
	w.arbs = w.arbs[:0]
	clear(w.index)
}

// Resize moves the walls and floor and wakes every body. A rising floor
// lifts everything with it; bodies cut off by a closer right wall are dropped
// again upright, in rows above the highest remaining body.
func (w *World) Resize(width, height int) {
	lift := min(0, float64(height)-w.h)
	w.w, w.h = float64(width), float64(height)
	top := 0.0
	var cut []*Rigidbody
	for _, b := range w.bodies {
		b.wake()
		b.pos.y += lift
		if lo, hi := b.aabb(0); hi.x > w.w+slop {
			cut = append(cut, b)
		} else {
			top = min(top, lo.y)
		}
	}
	x, bottom, rowH := 0.0, top, 0.0
	for _, b := range cut {
		bw, bh := 2*b.hw, 2*b.hh
		if x > 0 && x+bw > w.w {
			x, bottom, rowH = 0, bottom-rowH, 0
		}
		b.pos, b.vel, b.spin = vec{x + b.hw, bottom - b.hh}, vec{}, 0
		b.setAngle(0)
		x += bw
		rowH = max(rowH, bh)
	}
}

// Shake wakes every body and throws it upward and sideways with a random
// spin, so the whole pile jumps, tumbles and falls back.
func (w *World) Shake(r *rand.Rand) {
	for _, b := range w.bodies {
		b.wake()
		b.vel = vec{(r.Float64()*2 - 1) * shakeSpeed / 2, -shakeSpeed * (0.5 + r.Float64()/2)}
		b.spin = (r.Float64()*2 - 1) * shakeSpin
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

// HighestRestingTop returns the smallest top edge, in whole pixels, of the
// bounding box of any sleeping body, or false if none sleeps. A value <= 0
// means the resting pile has reached the top of the world.
func (w *World) HighestRestingTop() (int, bool) {
	top, ok := math.Inf(1), false
	for _, b := range w.bodies {
		if lo, _ := b.aabb(0); b.asleep && lo.y < top {
			top, ok = lo.y, true
		}
	}
	return int(math.Round(top)), ok
}

// Step advances the simulation by dt seconds. Call it with TimeStep.
func (w *World) Step(dt float64) {
	if w.Settled() {
		return
	}
	w.findContacts()
	for _, b := range w.bodies {
		if b.asleep {
			continue
		}
		b.vel.y += gravity * dt
		b.vel = b.vel.mul(1 / (1 + dt*linearDamping))
		b.spin /= 1 + dt*angularDamping
	}
	// Measure every contact before any warm start changes the velocities,
	// so a warm-start impulse never looks like an impact.
	for i := range w.arbs {
		w.arbs[i].preStep(dt)
	}
	for i := range w.arbs {
		w.arbs[i].warmStart()
	}
	for range iterations {
		for i := range w.arbs {
			w.arbs[i].solve()
		}
	}
	for _, b := range w.bodies {
		if b.asleep {
			continue
		}
		if s := b.speed(); s > maxSpeed {
			b.vel, b.spin = b.vel.mul(maxSpeed/s), b.spin*maxSpeed/s
		}
		b.pos = b.pos.add(b.vel.mul(dt))
		b.setAngle(b.angle + b.spin*dt)
		// The floor and walls don't give: move anything pushed into them
		// further than slop back out. The bounding box's edges are the
		// deepest corners.
		lo, hi := b.aabb(-slop)
		b.pos.x += max(0, -lo.x) - max(0, hi.x-w.w)
		b.pos.y -= max(0, hi.y-w.h)
	}
	w.wakeTouched()
	w.sleep(dt)
}

// findContacts rebuilds the arbiters: floor and wall contacts for awake
// bodies, then box contacts for pairs, not both asleep, whose padded
// bounding boxes overlap (sort and sweep along x). Contacts that persist
// from the previous step keep their impulses.
func (w *World) findContacts() {
	w.prev, w.arbs = w.arbs, w.prev[:0]
	w.prevIndex, w.index = w.index, w.prevIndex
	clear(w.index)

	planes := [...]plane{{vec{0, 1}, w.h}, {vec{-1, 0}, 0}, {vec{1, 0}, w.w}}
	var cs [2]contact
	for _, b := range w.bodies {
		b.lo, b.hi = b.aabb(margin / 2)
		if b.asleep {
			continue
		}
		for i, p := range planes {
			if n := planeContacts(b, p, &cs); n > 0 {
				w.addArbiter(pairKey{b, &w.ground, i}, cs[:n])
			}
		}
	}

	w.order = append(w.order[:0], w.bodies...)
	slices.SortFunc(w.order, func(a, b *Rigidbody) int {
		return cmp.Or(cmp.Compare(a.lo.x, b.lo.x), cmp.Compare(a.id, b.id))
	})
	for i, a := range w.order {
		for _, b := range w.order[i+1:] {
			if b.lo.x > a.hi.x {
				break
			}
			if (a.asleep && b.asleep) || b.lo.y > a.hi.y || a.lo.y > b.hi.y {
				continue
			}
			p, q := a, b // in order of Add, so a pair keeps its key
			if p.id > q.id {
				p, q = q, p
			}
			if n := collide(p, q, &cs); n > 0 {
				w.addArbiter(pairKey{p, q, 0}, cs[:n])
			}
		}
	}
}

// addArbiter records the contacts cs for k. A contact within a pixel of
// one the pair had last step takes over its impulses (warm starting).
// Matching by position rather than by the edges that made a contact keeps
// working for equal boxes stacked flush, whose clipped edges keep swapping.
func (w *World) addArbiter(k pairKey, cs []contact) {
	ar := arbiter{a: k.a, b: k.b, n: len(cs)}
	copy(ar.c[:], cs)
	if i, ok := w.prevIndex[k]; ok {
		old := &w.prev[i]
		for j := range ar.c[:ar.n] {
			for _, oc := range old.c[:old.n] {
				if d := oc.pos.sub(ar.c[j].pos); d.dot(d) < 1 {
					ar.c[j].pn, ar.c[j].pt = oc.pn, oc.pt
				}
			}
		}
	}
	w.index[k] = len(w.arbs)
	w.arbs = append(w.arbs, ar)
}

// preStep computes each contact's effective masses and target velocity.
func (ar *arbiter) preStep(dt float64) {
	a, b := ar.a, ar.b
	for i := range ar.c[:ar.n] {
		c := &ar.c[i]
		c.ra, c.rb = c.pos.sub(a.pos), c.pos.sub(b.pos)
		c.tangent = vec{c.normal.y, -c.normal.x}
		mass := func(d vec) float64 {
			ra, rb := c.ra.cross(d), c.rb.cross(d)
			return 1 / (a.im() + b.im() + a.ii()*ra*ra + b.ii()*rb*rb)
		}
		c.massN, c.massT = mass(c.normal), mass(c.tangent)

		vn := b.velocityAt(c.rb).sub(a.velocityAt(c.ra)).dot(c.normal)
		c.approach = -vn
		if c.sep > 0 {
			// Speculative: allow closing the gap within this step.
			c.bias = -c.sep / dt
		} else {
			c.bias = baumgarte / dt * max(0, -c.sep-slop)
		}
		if c.approach > bounceThreshold && c.sep < c.approach*dt {
			c.bias = max(c.bias, restitution*c.approach)
		}
	}
}

// warmStart applies the impulses carried over from the previous step.
func (ar *arbiter) warmStart() {
	for _, c := range ar.c[:ar.n] {
		p := c.normal.mul(c.pn).add(c.tangent.mul(c.pt))
		ar.a.push(p.neg(), c.ra)
		ar.b.push(p, c.rb)
	}
}

// solve applies one round of normal and friction impulses.
func (ar *arbiter) solve() {
	a, b := ar.a, ar.b
	for i := range ar.c[:ar.n] {
		c := &ar.c[i]
		dv := b.velocityAt(c.rb).sub(a.velocityAt(c.ra))
		pn := max(c.pn+c.massN*(c.bias-dv.dot(c.normal)), 0)
		p := c.normal.mul(pn - c.pn)
		c.pn = pn
		a.push(p.neg(), c.ra)
		b.push(p, c.rb)

		dv = b.velocityAt(c.rb).sub(a.velocityAt(c.ra))
		limit := friction * c.pn
		pt := max(-limit, min(limit, c.pt-c.massT*dv.dot(c.tangent)))
		p = c.tangent.mul(pt - c.pt)
		c.pt = pt
		a.push(p.neg(), c.ra)
		b.push(p, c.rb)
	}
}

// wakeTouched wakes sleeping bodies that were hit hard this step or rest
// on a body that is moving. Sleepers act as immovable during the step that
// wakes them, so impacts never pump momentum down into a pile.
func (w *World) wakeTouched() {
	for i := range w.arbs {
		ar := &w.arbs[i]
		if ar.a.asleep == ar.b.asleep || ar.b == &w.ground {
			continue
		}
		// below > 0 when the awake body is under the sleeper.
		sleeper, mover, below := ar.a, ar.b, ar.c[0].normal.y
		if ar.b.asleep {
			sleeper, mover, below = ar.b, ar.a, -below
		}
		hit := false
		for _, c := range ar.c[:ar.n] {
			hit = hit || c.approach > wakeSpeed
		}
		if hit || below > 0.5 && mover.speed() > sleepSpeed {
			sleeper.wake()
		}
	}
}

// sleep puts bodies that have been still for sleepDelay to rest where they
// are.
func (w *World) sleep(dt float64) {
	for _, b := range w.bodies {
		if b.asleep {
			continue
		}
		if b.speed() >= sleepSpeed {
			b.still = 0
			continue
		}
		if b.still += dt; b.still >= sleepDelay {
			b.asleep = true
			b.vel, b.spin = vec{}, 0
		}
	}
}
