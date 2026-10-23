package physics

import (
	"math/rand/v2"
	"testing"
)

const bw, bh = 9, 6 // sprite footprint in pixels

// drop adds n bodies in staggered, non-overlapping slots above the world.
func drop(w *World, n int, rng *rand.Rand) {
	width, _ := w.Size()
	cols := width / (bw + 2)
	for i := range n {
		x := float64(i%cols*(bw+2)) + rng.Float64()*2
		y := -float64((i/cols + 1) * (bh + 4))
		w.AddBody(x, y, bw, bh, rng.Float64()*40-20)
	}
}

// checkPile fails the test if any body is awake, outside the world or
// overlapping another body.
func checkPile(t *testing.T, w *World) {
	t.Helper()
	width, height := w.Size()
	bodies := w.Bodies()
	for i, a := range bodies {
		ax, ay := a.Pos()
		if !a.asleep {
			t.Errorf("body %d at %d,%d is awake", i, ax, ay)
		}
		aw, ah := int(a.w), int(a.h)
		if ax < 0 || ay < 0 || ax+aw > width || ay+ah > height {
			t.Errorf("body %d at %d,%d is outside %dx%d", i, ax, ay, width, height)
		}
		for j, b := range bodies[i+1:] {
			bx, by := b.Pos()
			if ax < bx+int(b.w) && bx < ax+aw && ay < by+int(b.h) && by < ay+ah {
				t.Errorf("bodies %d (%d,%d) and %d (%d,%d) overlap", i, ax, ay, i+1+j, bx, by)
			}
		}
	}
}

func positions(w *World) [][2]int {
	var ps [][2]int
	for _, b := range w.Bodies() {
		x, y := b.Pos()
		ps = append(ps, [2]int{x, y})
	}
	return ps
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
}

func TestRestingPileDoesNotDrift(t *testing.T) {
	w := NewWorld(120, 80)
	drop(w, 50, rand.New(rand.NewPCG(3, 4)))
	w.Settle(3000)
	before := positions(w)

	w.Resize(120, 80) // wakes every body without moving it
	for range 1000 {
		w.Step(TimeStep)
	}
	checkPile(t, w)
	for i, p := range positions(w) {
		if p != before[i] {
			t.Errorf("body %d drifted from %v to %v", i, before[i], p)
		}
	}
}

func TestTallStackIsStable(t *testing.T) {
	w := NewWorld(40, 100)
	for i := range 10 {
		w.AddBody(10, float64(100-(i+1)*bh), bw, bh, 0)
	}
	before := positions(w)
	for range 1000 {
		w.Step(TimeStep)
	}
	checkPile(t, w)
	for i, p := range positions(w) {
		if p != before[i] {
			t.Errorf("body %d moved from %v to %v", i, before[i], p)
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
	base := w.AddBody(10, 40-bh, bw, bh, 0)
	w.Settle(100)
	w.AddBody(10, 0, bw, bh, 0)
	for range 200 {
		w.Step(TimeStep)
		if !base.asleep {
			return
		}
	}
	t.Error("a falling body should wake the body it lands on")
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
		if b.vy >= 0 {
			t.Fatalf("body %d not thrown upward: vy=%v", i, b.vy)
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
	w.AddBody(20, -40, 5*bw, 5*bh, 0) // giant
	w.AddBody(30, -70, 3*bw, 3*bh, 0) // big, lands on the giant
	drop(w, 30, rand.New(rand.NewPCG(9, 10)))
	for _, b := range w.Bodies()[2:] {
		b.y -= 80 // above the giant and the big
	}
	if !w.Settle(5000) {
		t.Fatal("mixed pile did not settle")
	}
	checkPile(t, w)
}

func TestRemoveDropsWhatItHeld(t *testing.T) {
	w := NewWorld(40, 40)
	base := w.AddBody(10, 40-bh, bw, bh, 0)
	top := w.AddBody(10, 40-2*bh, bw, bh, 0)
	w.Settle(100)
	w.Remove(base)
	if len(w.Bodies()) != 1 || w.Settled() {
		t.Fatal("Remove should delete the body and wake the rest")
	}
	w.Settle(600)
	if _, y := top.Pos(); y != 40-bh {
		t.Errorf("body left resting at y=%d, want the floor", y)
	}
}

func TestOffCentreLandingSkidsSideways(t *testing.T) {
	for _, tc := range []struct {
		name   string
		offset float64
		dir    int
	}{{"left", -3, -1}, {"centred", 0, 0}, {"right", 3, 1}} {
		w := NewWorld(120, 80)
		w.AddBody(50, 80-bh, bw, bh, 0)
		w.Settle(600)
		top := w.AddBody(50+tc.offset, 20, bw, bh, 0)
		w.Settle(3000)
		x, _ := top.Pos()
		moved := x - int(50+tc.offset)
		if tc.dir == 0 && moved != 0 || tc.dir*moved < 2 && tc.dir != 0 {
			t.Errorf("%s landing moved %d px sideways", tc.name, moved)
		}
	}
}

func TestOverhangingLandingTipsOntoItsSide(t *testing.T) {
	for _, dir := range []int{-1, 1} {
		w := NewWorld(120, 80)
		w.AddBody(50, 80-bh, bw, bh, 0)
		w.Settle(600)
		top := w.AddBody(50+float64(dir*6), 20, bw, bh, 0)
		if !w.Settle(3000) {
			t.Fatal("did not settle")
		}
		if gw, gh := top.Size(); top.Turn() != dir || gw != bh || gh != bw {
			t.Errorf("dir %d: turn %d, size %dx%d; want tipped %dx%d", dir, top.Turn(), gw, gh, bh, bw)
		}
		checkPile(t, w)
	}
}

func TestBlockedTipSlidesUpright(t *testing.T) {
	w := NewWorld(120, 80)
	w.AddBody(50, 80-bh, bw, bh, 0)
	// A low box beside the support fills the space the tipped body would
	// need without holding it up.
	w.AddBody(50+bw+1, 80-3, bw, 3, 0)
	w.Settle(600)
	top := w.AddBody(50+6, 20, bw, bh, 0)
	w.Settle(3000)
	if top.Turn() != 0 {
		t.Fatalf("tipped into an occupied space, turn %d", top.Turn())
	}
	if x, _ := top.Pos(); x <= 56 {
		t.Fatalf("overhanging body stayed at x=%d instead of sliding off", x)
	}
	checkPile(t, w)
}
