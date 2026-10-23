package physics

import (
	"math"
	"math/rand/v2"
	"testing"
)

const bw, bh = 9, 6 // sprite footprint in pixels

// add puts a width x height body into w, centred on (x, y) and turned by
// angle.
func add(w *World, width, height int, x, y, angle float64) *Rigidbody {
	b := NewRigidbody(width, height)
	b.SetCentre(x, y)
	b.setAngle(angle)
	w.Add(b)
	return b
}

// drop adds n bodies in staggered, non-overlapping slots above the world.
func drop(w *World, n int, rng *rand.Rand) {
	width, _ := w.Size()
	cols := width / (bw + 2)
	for i := range n {
		x := float64(i%cols*(bw+2)) + rng.Float64()*2 + bw/2.0
		y := -float64((i/cols + 1) * (bh + 4))
		add(w, bw, bh, x, y, 0).SetVelocity(rng.Float64()*40-20, 0)
	}
}

// overlap returns how deep a and b overlap, or a negative value if they
// don't.
func overlap(a, b *Rigidbody) float64 {
	var cs [2]contact
	depth := -1.0
	for _, c := range cs[:collide(a, b, &cs)] {
		depth = max(depth, -c.sep)
	}
	return depth
}

// checkPile fails the test if any body is awake, outside the world or
// overlapping another by more than a pixel.
func checkPile(t *testing.T, w *World) {
	t.Helper()
	width, height := w.Size()
	bodies := w.Bodies()
	for i, a := range bodies {
		if !a.asleep {
			t.Errorf("body %d at %v is awake", i, a.pos)
		}
		if lo, hi := a.aabb(0); lo.x < -1 || hi.x > float64(width)+1 || hi.y > float64(height)+1 {
			t.Errorf("body %d at %v is outside %dx%d", i, a.pos, width, height)
		}
		for j, b := range bodies[i+1:] {
			if d := overlap(a, b); d > 1 {
				t.Errorf("bodies %d %v and %d %v overlap by %.2f px", i, a.pos, i+1+j, b.pos, d)
			}
		}
	}
}

// nearQuarter reports whether a is within tol of a multiple of π/2.
func nearQuarter(a, tol float64) bool {
	return math.Abs(math.Remainder(a, math.Pi/2)) <= tol
}

func TestRestingBoxStaysPut(t *testing.T) {
	w := NewWorld(40, 40)
	b := add(w, bw, bh, 20, 40-bh/2.0, 0)
	for range 1000 {
		w.Step(TimeStep)
	}
	if d := math.Hypot(b.pos.x-20, b.pos.y-(40-bh/2.0)); d > 0.5 || math.Abs(b.angle) > 1e-3 {
		t.Errorf("resting box moved %.3f px and turned %.4f rad", d, b.angle)
	}
	if !b.asleep {
		t.Error("resting box never fell asleep")
	}
}

func TestTiltedBoxLandsFlat(t *testing.T) {
	for _, angle := range []float64{0.3, -0.7, 1.2, 2.5} {
		w := NewWorld(80, 60)
		b := add(w, bw, bh, 40, 10, angle)
		if !w.Settle(3000) {
			t.Fatalf("angle %v: did not settle", angle)
		}
		if !nearQuarter(b.angle, 0.02) {
			t.Errorf("dropped at %v, rests at %.3f rad", angle, b.angle)
		}
		checkPile(t, w)
	}
}

func TestOffCentreLandingSpins(t *testing.T) {
	for _, offset := range []float64{-6, 6} {
		w := NewWorld(120, 80)
		add(w, bw, bh, 60, 80-bh/2.0, 0)
		w.Settle(600)
		top := add(w, bw, bh, 60+offset, 20, 0)
		spun := 0.0
		for range 3000 {
			w.Step(TimeStep)
			spun = max(spun, math.Abs(top.spin))
		}
		if spun < 0.5 {
			t.Errorf("offset %v: landing never spun the body (max |ω| %.3f)", offset, spun)
		}
		if math.Abs(top.angle) < 0.05 {
			t.Errorf("offset %v: body ended upright at %.3f rad", offset, top.angle)
		}
		checkPile(t, w)
	}
}

func TestDropFiftySettles(t *testing.T) {
	w := NewWorld(120, 80)
	drop(w, 50, rand.New(rand.NewPCG(1, 2)))
	if !w.Settle(3000) {
		t.Fatal("pile did not settle within 3000 steps")
	}
	checkPile(t, w)
	if top, ok := w.HighestRestingTop(); !ok || top >= 80-bh {
		t.Errorf("HighestRestingTop() = %d, %v; want a stacked pile", top, ok)
	}
	tilted := 0
	for _, b := range w.Bodies() {
		if !nearQuarter(b.angle, 0.05) {
			tilted++
		}
	}
	t.Logf("%d of 50 bodies rest at a tilt", tilted)
}

// TestRestingPileDoesNotDrift wakes a settled pile and checks it holds still.
// Waking drops the warm-start impulses, so the pile sags by a fraction of a
// pixel while the solver finds them again; anything near a pixel would show.
func TestRestingPileDoesNotDrift(t *testing.T) {
	w := NewWorld(120, 80)
	drop(w, 50, rand.New(rand.NewPCG(3, 4)))
	w.Settle(3000)
	var before []vec
	for _, b := range w.Bodies() {
		before = append(before, b.pos)
	}
	w.Resize(120, 80) // wakes every body without moving it
	for range 1000 {
		w.Step(TimeStep)
	}
	checkPile(t, w)
	worst := 0.0
	for i, b := range w.Bodies() {
		worst = max(worst, math.Hypot(b.pos.x-before[i].x, b.pos.y-before[i].y))
	}
	if worst > 1 {
		t.Errorf("a body drifted %.2f px", worst)
	}
	t.Logf("largest drift %.3f px", worst)
}

func TestTallStackIsStable(t *testing.T) {
	w := NewWorld(40, 100)
	for i := range 10 {
		add(w, bw, bh, 15, 100-(float64(i)+0.5)*bh, 0)
	}
	if !w.Settle(1000) {
		t.Fatal("stack did not settle")
	}
	checkPile(t, w)
	for i, b := range w.Bodies() {
		if d := math.Hypot(b.pos.x-15, b.pos.y-(100-(float64(i)+0.5)*bh)); d > 0.5 {
			t.Errorf("body %d moved %.2f px", i, d)
		}
	}
}

func TestResizeKeepsBodiesInside(t *testing.T) {
	w := NewWorld(160, 80)
	drop(w, 40, rand.New(rand.NewPCG(5, 6)))
	w.Settle(3000)

	w.Resize(90, 70)
	if w.Settled() {
		t.Error("Resize should wake bodies")
	}
	if !w.Settle(5000) {
		t.Fatal("pile did not resettle after resize")
	}
	checkPile(t, w)
}

func TestImpactWakesSleeper(t *testing.T) {
	w := NewWorld(40, 40)
	base := add(w, bw, bh, 15, 40-bh/2.0, 0)
	w.Settle(100)
	add(w, bw, bh, 15, 5, 0)
	for range 200 {
		w.Step(TimeStep)
		if !base.asleep {
			return
		}
	}
	t.Error("a falling body should wake the body it lands on")
}

func TestShakeLaunchesAndResettles(t *testing.T) {
	w := NewWorld(120, 80)
	rng := rand.New(rand.NewPCG(5, 6))
	drop(w, 30, rng)
	w.Settle(3000)
	before, _ := w.HighestRestingTop()
	w.Shake(rng)
	if w.Settled() {
		t.Fatal("shake left bodies asleep")
	}
	w.Step(TimeStep)
	for i, b := range w.Bodies() {
		if b.vel.y >= 0 {
			t.Fatalf("body %d not thrown upward: vy=%v", i, b.vel.y)
		}
	}
	if !w.Settle(3000) {
		t.Fatal("pile did not resettle after a shake")
	}
	checkPile(t, w)
	if after, _ := w.HighestRestingTop(); after < before-3*bh {
		t.Errorf("pile top went from %d to %d after a shake", before, after)
	}
}

func TestMixedSizesStack(t *testing.T) {
	w := NewWorld(120, 120)
	add(w, 8*bw, 8*bh, 50, -30, 0) // giant
	add(w, 5*bw, 5*bh, 55, -90, 0) // big, lands on the giant
	drop(w, 30, rand.New(rand.NewPCG(9, 10)))
	for _, b := range w.Bodies()[2:] {
		b.pos.y -= 120 // above the giant and the big
	}
	if !w.Settle(5000) {
		t.Fatal("mixed pile did not settle")
	}
	checkPile(t, w)
}

func TestRemoveDropsWhatItHeld(t *testing.T) {
	w := NewWorld(40, 40)
	base := add(w, bw, bh, 15, 40-bh/2.0, 0)
	top := add(w, bw, bh, 15, 40-1.5*bh, 0)
	w.Settle(100)
	w.Remove(base)
	if len(w.Bodies()) != 1 || w.Settled() {
		t.Fatal("Remove should delete the body and wake what it held")
	}
	w.Settle(600)
	if r := top.Bounds(); r.Max.Y != 40 {
		t.Errorf("body left resting with its bottom at y=%d, want the floor", r.Max.Y)
	}
}

func TestQuarterTurnedBodyBounds(t *testing.T) {
	w := NewWorld(40, 40)
	b := add(w, bw, bh, 20, 20, math.Pi/2)
	if r := b.Bounds(); r.Dx() != bh || r.Dy() != bw {
		t.Errorf("quarter-turned bounds %v, want %dx%d", r, bh, bw)
	}
}

func BenchmarkSettle500(b *testing.B) {
	for b.Loop() {
		w := NewWorld(400, 200)
		drop(w, 500, rand.New(rand.NewPCG(7, 8)))
		if !w.Settle(20000) {
			b.Fatal("did not settle")
		}
	}
}

func BenchmarkStep500Awake(b *testing.B) {
	w := NewWorld(400, 200)
	drop(w, 500, rand.New(rand.NewPCG(7, 8)))
	w.Settle(20000)
	for b.Loop() {
		w.Resize(400, 200)
		w.Step(TimeStep)
	}
}
