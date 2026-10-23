package physics

import (
	"image"
	"math"
)

// Rigidbody is a solid box that moves and rotates under gravity and
// contacts. Create one with NewRigidbody, set where it starts with SetCentre
// and SetVelocity, then Add it to a World, which moves it from then on.
type Rigidbody struct {
	hw, hh        float64 // half width and height
	pos, vel      vec     // centre and its velocity
	angle, spin   float64 // radians and radians per second, clockwise on screen
	rot           rot     // of angle
	invMass, invI float64
	radius        float64 // half the diagonal
	asleep        bool
	still         float64 // seconds spent below sleepSpeed
	id            int     // order of Add, to break ties deterministically
	lo, hi        vec     // padded bounding box, refreshed every step
}

// NewRigidbody returns an awake, upright w x h body at the origin. Its mass
// is its area and its inertia that of a uniform box.
func NewRigidbody(w, h int) *Rigidbody {
	fw, fh := float64(w), float64(h)
	mass := fw * fh
	return &Rigidbody{
		hw: fw / 2, hh: fh / 2,
		rot:     rot{c: 1},
		invMass: 1 / mass,
		invI:    12 / (mass * (fw*fw + fh*fh)),
		radius:  math.Hypot(fw, fh) / 2,
	}
}

// SetCentre moves the body's centre to (x, y).
func (b *Rigidbody) SetCentre(x, y float64) {
	b.pos = vec{x, y}
}

// SetVelocity sets the body's velocity in px/s.
func (b *Rigidbody) SetVelocity(vx, vy float64) {
	b.vel = vec{vx, vy}
}

// Centre returns the position of the body's centre.
func (b *Rigidbody) Centre() (x, y float64) {
	return b.pos.x, b.pos.y
}

// Angle returns how far the body has turned clockwise, in radians in
// [-π, π].
func (b *Rigidbody) Angle() float64 {
	return b.angle
}

// Size returns the body's unrotated width and height.
func (b *Rigidbody) Size() (w, h int) {
	return int(2 * b.hw), int(2 * b.hh)
}

// Bounds returns the axis-aligned box around the rotated body, rounded to
// whole pixels.
func (b *Rigidbody) Bounds() image.Rectangle {
	lo, hi := b.aabb(0)
	r := func(v float64) int { return int(math.Round(v)) }
	return image.Rect(r(lo.x), r(lo.y), r(hi.x), r(hi.y))
}

// aabb returns the corners of the axis-aligned box around the body, grown
// by pad on every side.
func (b *Rigidbody) aabb(pad float64) (lo, hi vec) {
	c, s := math.Abs(b.rot.c), math.Abs(b.rot.s)
	e := vec{c*b.hw + s*b.hh + pad, s*b.hw + c*b.hh + pad}
	return b.pos.sub(e), b.pos.add(e)
}

func (b *Rigidbody) setAngle(a float64) {
	b.angle = math.Remainder(a, 2*math.Pi)
	b.rot = rot{math.Cos(b.angle), math.Sin(b.angle)}
}

// speed bounds how fast any point of the body moves.
func (b *Rigidbody) speed() float64 {
	return math.Hypot(b.vel.x, b.vel.y) + math.Abs(b.spin)*b.radius
}

// im and ii are the inverse mass and inertia the solver uses: zero for a
// sleeping body, which is immovable until woken.
func (b *Rigidbody) im() float64 {
	if b.asleep {
		return 0
	}
	return b.invMass
}

func (b *Rigidbody) ii() float64 {
	if b.asleep {
		return 0
	}
	return b.invI
}

// push applies impulse p at offset r from the centre.
func (b *Rigidbody) push(p, r vec) {
	b.vel = b.vel.add(p.mul(b.im()))
	b.spin += b.ii() * r.cross(p)
}

// velocityAt returns the velocity of the point at offset r from the centre.
func (b *Rigidbody) velocityAt(r vec) vec {
	return b.vel.add(vec{-b.spin * r.y, b.spin * r.x})
}

func (b *Rigidbody) wake() {
	b.asleep = false
	b.still = 0
}

type vec struct{ x, y float64 }

func (a vec) add(b vec) vec       { return vec{a.x + b.x, a.y + b.y} }
func (a vec) sub(b vec) vec       { return vec{a.x - b.x, a.y - b.y} }
func (a vec) mul(k float64) vec   { return vec{a.x * k, a.y * k} }
func (a vec) dot(b vec) float64   { return a.x*b.x + a.y*b.y }
func (a vec) cross(b vec) float64 { return a.x*b.y - a.y*b.x }
func (a vec) abs() vec            { return vec{math.Abs(a.x), math.Abs(a.y)} }
func (a vec) neg() vec            { return vec{-a.x, -a.y} }
func (r rot) apply(v vec) vec     { return vec{r.c*v.x - r.s*v.y, r.s*v.x + r.c*v.y} }
func (r rot) unapply(v vec) vec   { return vec{r.c*v.x + r.s*v.y, -r.s*v.x + r.c*v.y} }
func (r rot) xAxis() vec          { return vec{r.c, r.s} }
func (r rot) yAxis() vec          { return vec{-r.s, r.c} }

// towards returns v, or -v if d is negative.
func towards(v vec, d float64) vec {
	if d < 0 {
		return v.neg()
	}
	return v
}

// rot is a rotation by the angle whose cosine and sine are c and s.
type rot struct{ c, s float64 }
