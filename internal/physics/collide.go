package physics

import "math"

// Box-box collision follows Box2D-Lite: the separating axis test finds the
// face of least penetration, the incident box's most anti-parallel edge is
// clipped against the reference face's side planes, and the clipped points
// become up to two contacts. Contacts are also made for boxes up to margin
// apart (speculative contacts), so fast bodies stop at the surface instead
// of sinking in first.

type contact struct {
	pos    vec     // contact point
	normal vec     // unit normal from body a to body b
	sep    float64 // separation along normal; negative when overlapping

	pn, pt          float64 // accumulated normal and tangent impulses
	massN, massT    float64
	bias            float64 // normal velocity the solver aims for
	approach        float64 // closing speed before solving
	ra, rb, tangent vec     // offsets from each centre, and the tangent
}

// clipSegment keeps the part of segment in on the inner side of the line
// n·v = offset and reports whether a segment (two points) is left.
func clipSegment(in [2]vec, n vec, offset float64) (out [2]vec, ok bool) {
	d0, d1 := n.dot(in[0])-offset, n.dot(in[1])-offset
	k := 0
	if d0 <= 0 {
		out[k] = in[0]
		k++
	}
	if d1 <= 0 {
		out[k] = in[1]
		k++
	}
	if d0*d1 < 0 {
		out[k] = in[0].add(in[1].sub(in[0]).mul(d0 / (d0 - d1)))
		k++
	}
	return out, k == 2
}

// incidentEdge returns the edge of box b whose outward normal points most
// against normal, in world space.
func incidentEdge(b *Rigidbody, normal vec) [2]vec {
	n := b.rot.unapply(normal).neg()
	x, y := b.hw, b.hh
	var e [2]vec
	switch na := n.abs(); {
	case na.x > na.y && n.x > 0:
		e = [2]vec{{x, -y}, {x, y}}
	case na.x > na.y:
		e = [2]vec{{-x, y}, {-x, -y}}
	case n.y > 0:
		e = [2]vec{{x, y}, {-x, y}}
	default:
		e = [2]vec{{-x, -y}, {x, -y}}
	}
	return [2]vec{b.pos.add(b.rot.apply(e[0])), b.pos.add(b.rot.apply(e[1]))}
}

// collide writes the contacts between boxes a and b, at most margin apart,
// to out and returns how many there are.
func collide(a, b *Rigidbody, out *[2]contact) int {
	hA, hB := vec{a.hw, a.hh}, vec{b.hw, b.hh}
	dp := b.pos.sub(a.pos)
	dA, dB := a.rot.unapply(dp), b.rot.unapply(dp)
	// The rotation from b's frame to a's; its absolute matrix is symmetric.
	cd := a.rot.c*b.rot.c + a.rot.s*b.rot.s
	sd := a.rot.c*b.rot.s - a.rot.s*b.rot.c
	ac, as := math.Abs(cd), math.Abs(sd)
	absC := func(v vec) vec { return vec{ac*v.x + as*v.y, as*v.x + ac*v.y} }
	faceA := dA.abs().sub(hA).sub(absC(hB))
	faceB := dB.abs().sub(absC(hA)).sub(hB)
	if max(faceA.x, faceA.y, faceB.x, faceB.y) > margin {
		return 0
	}

	// Pick the axis of greatest separation, preferring a's faces and then
	// x axes so the choice doesn't flicker between nearly equal ones.
	const tol = 0.1
	axis, sep, normal := 0, faceA.x, towards(a.rot.xAxis(), dA.x)
	if faceA.y > sep+tol {
		axis, sep, normal = 1, faceA.y, towards(a.rot.yAxis(), dA.y)
	}
	if faceB.x > sep+tol {
		axis, sep, normal = 2, faceB.x, towards(b.rot.xAxis(), dB.x)
	}
	if faceB.y > sep+tol {
		axis, normal = 3, towards(b.rot.yAxis(), dB.y)
	}

	// Set up the reference face and its side planes, and find the incident
	// edge on the other box.
	ref, inc, front := a, b, normal
	if axis >= 2 {
		ref, inc, front = b, a, normal.neg()
	}
	hx, hy, side := ref.hw, ref.hh, ref.rot.yAxis()
	if axis == 1 || axis == 3 {
		hx, hy, side = ref.hh, ref.hw, ref.rot.xAxis()
	}
	frontOff := ref.pos.dot(front) + hx
	sideOff := ref.pos.dot(side)

	clip, ok := clipSegment(incidentEdge(inc, front), side.neg(), -sideOff+hy)
	if !ok {
		return 0
	}
	if clip, ok = clipSegment(clip, side, sideOff+hy); !ok {
		return 0
	}
	n := 0
	for _, v := range clip {
		s := front.dot(v) - frontOff
		if s > margin {
			continue
		}
		// Slide the point onto the reference face.
		out[n] = contact{pos: v.sub(front.mul(s)), normal: normal, sep: s}
		n++
	}
	return n
}

// plane is the outside of the world beyond a floor or wall: points p with
// normal·p > offset, the normal pointing out of the world.
type plane struct {
	normal vec
	offset float64
}

// planeContacts writes contacts for the (at most two) deepest of b's corners
// within margin of p to out and returns how many there are.
func planeContacts(b *Rigidbody, p plane, out *[2]contact) int {
	n := 0
	for i := range 4 {
		corner := vec{b.hw, b.hh}
		if i&1 != 0 {
			corner.x = -corner.x
		}
		if i&2 != 0 {
			corner.y = -corner.y
		}
		v := b.pos.add(b.rot.apply(corner))
		s := p.offset - p.normal.dot(v)
		if s > margin {
			continue
		}
		c := contact{pos: v, normal: p.normal, sep: s}
		if n < 2 {
			out[n] = c
			n++
			continue
		}
		j := 0 // replace the shallower contact if this corner is deeper
		if out[1].sep > out[0].sep {
			j = 1
		}
		if s < out[j].sep {
			out[j] = c
		}
	}
	return n
}
