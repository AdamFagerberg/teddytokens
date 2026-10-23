package physics

import (
	"math/rand/v2"
	"testing"
)

const bw, bh = 14, 8 // sprite footprint in pixels

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
		if ax < 0 || ay < 0 || ax+bw > width || ay+bh > height {
			t.Errorf("body %d at %d,%d is outside %dx%d", i, ax, ay, width, height)
		}
		for j, b := range bodies[i+1:] {
			bx, by := b.Pos()
			if ax < bx+bw && bx < ax+bw && ay < by+bh && by < ay+bh {
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
