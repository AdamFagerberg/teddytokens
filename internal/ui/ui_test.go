package ui

import (
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/AdamFagerberg/teddytokens/internal/physics"
	"github.com/AdamFagerberg/teddytokens/internal/sprites"
)

func TestFormatTokens(t *testing.T) {
	for n, want := range map[int64]string{
		0:             "0",
		999:           "999",
		1000:          "1.0k",
		12_345:        "12.3k",
		999_949:       "999.9k",
		999_950:       "1.00M",
		1_240_000:     "1.24M",
		999_994_999:   "999.99M",
		999_995_000:   "1.00B",
		3_504_000_000: "3.50B",
	} {
		if got := formatTokens(n); got != want {
			t.Errorf("formatTokens(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestSpawnerAdd(t *testing.T) {
	s := spawner{per: 10_000}
	s.add(9_999)
	if s.queue != 0 || s.progress() != 0.9999 {
		t.Fatalf("after 9999: queue %d, progress %v", s.queue, s.progress())
	}
	s.add(1)
	if s.queue != 1 || s.carry != 0 {
		t.Fatalf("after 10000: queue %d, carry %d", s.queue, s.carry)
	}
	s.add(205_000) // a 200k burst plus change
	if s.queue != 21 || s.carry != 5_000 {
		t.Fatalf("after burst: queue %d, carry %d", s.queue, s.carry)
	}
}

func TestSpawnerRelease(t *testing.T) {
	s := spawner{per: 1}
	if n := s.release(1); n != 0 {
		t.Fatalf("empty queue released %d", n)
	}
	s.add(20)
	if n := s.release(0); n != 1 {
		t.Fatalf("first of a burst released %d, want 1 immediately", n)
	}
	got := 0
	for range 4 { // one second
		got += s.release(0.25)
	}
	if got != 20/burstTime { // a burst of 20 falls in burstTime seconds
		t.Fatalf("released %d in one second, want %v", got, 20/burstTime)
	}
	for s.queue > 0 {
		s.release(0.1)
	}
	if n := s.release(10); n != 0 || s.credit != 1 {
		t.Fatalf("drained queue released %d, credit %v", n, s.credit)
	}
}

func TestPileCapacity(t *testing.T) {
	if c := pileCapacity(1, 1); c != 1 {
		t.Fatalf("tiny capacity = %d, want 1", c)
	}
}

type noSource struct{}

func (noSource) Poll() (int64, error) { return 0, nil }

func newTestModel(initial int64) *Model {
	m := New(Config{Source: noSource{}, Initial: initial, Per: 10_000, Scope: "test", Seed: 1})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m
}

func TestStartupPileIsSettled(t *testing.T) {
	m := newTestModel(205_000)
	if n := len(m.world.Bodies()); n != 20 || len(m.chars) != n {
		t.Fatalf("got %d bodies, %d skins, want 20", n, len(m.chars))
	}
	if !m.world.Settled() {
		t.Fatal("startup pile is still moving")
	}
	if m.spawn.queue != 0 || m.spawn.carry != 5_000 {
		t.Fatalf("startup tokens queued: queue %d, carry %d", m.spawn.queue, m.spawn.carry)
	}
}

func TestFreshStartsEmpty(t *testing.T) {
	m := New(Config{Source: noSource{}, Initial: 50_000_000, Fresh: true, Per: 10_000, Scope: "test", Seed: 1})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if len(m.chars) != 0 || m.giants != 0 || m.spawn.queue != 0 || m.total != 50_000_000 {
		t.Fatalf("chars %d, giants %d, queue %d, total %d", len(m.chars), m.giants, m.spawn.queue, m.total)
	}
	m.Update(pollMsg{delta: 25_000})
	if m.spawn.queue != 2 {
		t.Fatalf("queue %d after 25k new tokens, want 2", m.spawn.queue)
	}
}

func TestHugeHistoryBecomesBigsAndGiants(t *testing.T) {
	capacity := pileCapacity(120, 2*39)
	m := newTestModel(10_000 * int64(7*capacity+5)) // 7 piles: 2 giants and a big
	var n [len(tierScale)]int
	for _, c := range m.chars {
		n[c.tier]++
	}
	if m.giants != 2 || n != [...]int{small: 5, big: 1, giant: 1} {
		t.Fatalf("giants %d, characters per tier %v", m.giants, n)
	}
	if !m.world.Settled() {
		t.Fatal("startup pile is still moving")
	}
	checkChars(t, m, true)
}

func TestKeys(t *testing.T) {
	m := newTestModel(50_000)
	press := func(r rune) tea.Cmd {
		_, cmd := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		return cmd
	}
	before := m.chars[0].skin
	press('t')
	if !m.teddy || m.chars[0].skin != before || !m.world.Settled() {
		t.Fatal("t should re-skin in place without touching physics")
	}
	m.merge = &merge{}
	press('s')
	press('r')
	if !m.world.Settled() {
		t.Fatal("s and r should be ignored during a merge")
	}
	m.merge = nil
	press('s')
	if m.world.Settled() {
		t.Fatal("s should wake the pile")
	}
	press('r')
	if len(m.world.Bodies()) != 5 || m.chars[0].skin != before {
		t.Fatal("r should drop the same characters again")
	}
	for _, b := range m.world.Bodies() {
		if r := b.Bounds(); r.Max.Y > 0 || b.Angle() != 0 {
			t.Fatalf("replayed body starts on screen or tilted: %v, %v rad", r, b.Angle())
		}
	}
	if cmd := press('q'); cmd == nil || cmd() != tea.Quit() {
		t.Fatal("q should quit")
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); cmd == nil || cmd() != tea.Quit() {
		t.Fatal("ctrl+c should quit")
	}
}

// addChar adds a character of tier with its top-left corner at (x, y).
func addChar(m *Model, tier int, x, y float64) *char {
	c := m.newChar(tier)
	m.addBody(c, x, y, 0)
	m.chars = append(m.chars, c)
	return c
}

// runUntil steps frames of 1/fps seconds until done or a minute has passed.
func runUntil(t *testing.T, m *Model, done func() bool) {
	t.Helper()
	now := time.Unix(0, 0)
	for range 60 * fps {
		if done() {
			return
		}
		now = now.Add(time.Second / fps)
		m.frame(now)
	}
	t.Fatal("timed out")
}

func TestPileAtTopMergesIntoBig(t *testing.T) {
	m := newTestModel(0)
	_, floor := m.world.Size()
	h := sprites.Claude.H
	for i := range floor/h + 1 { // a column one character taller than the world
		addChar(m, small, 0, float64(floor-(i+1)*h))
	}
	m.world.Settle(600)
	runUntil(t, m, func() bool { return m.merge != nil })
	runUntil(t, m, func() bool { return m.merge == nil })
	if len(m.chars) != 1 || m.chars[0].tier != big || m.giants != 0 {
		t.Fatalf("got %d characters, giants %d; want one big", len(m.chars), m.giants)
	}
	runUntil(t, m, m.world.Settled)
	if r := m.chars[0].body.Bounds(); r.Max.Y != floor || r.Dy() != 5*h {
		t.Fatalf("big rests at %v, want upright on the floor at %d", r, floor)
	}
}

func TestResizeDuringMergeStaysInside(t *testing.T) {
	m := newTestModel(0)
	_, floor := m.world.Size()
	h := sprites.Claude.H
	for i := range floor/h + 1 {
		addChar(m, small, 0, float64(floor-(i+1)*h))
	}
	m.world.Settle(600)
	runUntil(t, m, func() bool { return m.merge != nil })
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	if m.merge != nil {
		t.Fatal("resize should cancel the merge")
	}
	runUntil(t, m, func() bool { return m.merge == nil && m.world.Settled() })
	checkChars(t, m, true)
}

func TestThreeBigsMakeOneGiant(t *testing.T) {
	m := newTestModel(0)
	for round := 1; round <= 2; round++ {
		for i := range 3 {
			addChar(m, big, float64(i*40), -30)
		}
		runUntil(t, m, func() bool { return m.giants == round && m.world.Settled() })
		if len(m.chars) != 1 || m.chars[0].tier != giant {
			t.Fatalf("round %d: %d characters, want the one giant", round, len(m.chars))
		}
	}
}

func TestFourBigsLeaveOneOver(t *testing.T) {
	m := newTestModel(0)
	for i := range 4 {
		addChar(m, big, float64(i%3*44), float64(-30-i/3*30))
	}
	runUntil(t, m, func() bool { return m.giants == 1 && m.merge == nil })
	n := map[int]int{}
	for _, c := range m.chars {
		n[c.tier]++
	}
	if n[giant] != 1 || n[big] != 1 {
		t.Fatalf("got %v, want one giant and one big", n)
	}
}

func TestBigBurstBecomesPiles(t *testing.T) {
	m := newTestModel(0)
	capacity := pileCapacity(m.world.Size())
	// 10 piles and 3: rainPiles of them rain, the other 7 make 2 giants and a big.
	m.Update(pollMsg{delta: 10_000 * int64(10*capacity+3)})
	m.frame(time.Unix(0, 0))
	var n [len(tierScale)]int
	for _, c := range m.chars {
		n[c.tier]++
	}
	if m.giants != 2 || n[giant] != 1 || n[big] != 1 {
		t.Fatalf("giants %d, characters per tier %v", m.giants, n)
	}
	if raining := m.spawn.queue + n[small]; raining != rainPiles*capacity+3 {
		t.Fatalf("%d characters raining, want %d", raining, rainPiles*capacity+3)
	}
}

func TestResizeRescalesToFit(t *testing.T) {
	m := newTestModel(10_000 * int64(3*pileCapacity(120, 2*39))) // one giant
	// 8x would be taller than half of the 78-pixel world.
	if w, _ := m.chars[0].size(); w != 6*sprites.Claude.W {
		t.Fatalf("giant is %d wide at 120x40", w)
	}
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	if w, h := m.chars[0].size(); w > 60 || h > 19 {
		t.Fatalf("giant is %dx%d in a 60x38 world", w, h)
	}
	runUntil(t, m, m.world.Settled)
	checkChars(t, m, true)
}

// checkChars fails the test if there is more than one giant, a character is
// outside the walls or below the floor or, when settled is set, overlaps
// another by more than a pixel.
func checkChars(t *testing.T, m *Model, settled bool) {
	t.Helper()
	ww, wh := m.world.Size()
	giants := 0
	for i, a := range m.chars {
		if a.tier == giant {
			if giants++; giants > 1 {
				t.Fatal("more than one giant")
			}
		}
		if r := a.body.Bounds(); r.Min.X < -1 || r.Max.X > ww+1 || r.Max.Y > wh+1 {
			t.Fatalf("character %d at %v is outside %dx%d", i, r, ww, wh)
		}
		for _, b := range m.chars[i+1:] {
			if d := overlap(a.body, b.body); settled && d > 1 {
				t.Fatalf("characters at %v and %v overlap by %.2f px", a.body.Bounds(), b.body.Bounds(), d)
			}
		}
	}
}

// overlap returns how deep two bodies overlap, or a negative value if they
// don't, by the separating axis test.
func overlap(a, b *physics.Rigidbody) float64 {
	ca, cb := corners(a), corners(b)
	depth := math.Inf(1)
	for _, body := range []*physics.Rigidbody{a, b} {
		cos, sin := math.Cos(body.Angle()), math.Sin(body.Angle())
		for _, axis := range [][2]float64{{cos, sin}, {-sin, cos}} {
			loA, hiA := project(ca, axis)
			loB, hiB := project(cb, axis)
			depth = min(depth, min(hiA, hiB)-max(loA, loB))
		}
	}
	return depth
}

func corners(b *physics.Rigidbody) [4][2]float64 {
	x, y := b.Centre()
	w, h := b.Size()
	cos, sin := math.Cos(b.Angle()), math.Sin(b.Angle())
	var cs [4][2]float64
	for i, d := range [4][2]float64{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
		dx, dy := d[0]*float64(w)/2, d[1]*float64(h)/2
		cs[i] = [2]float64{x + cos*dx - sin*dy, y + sin*dx + cos*dy}
	}
	return cs
}

func project(cs [4][2]float64, axis [2]float64) (lo, hi float64) {
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, c := range cs {
		p := c[0]*axis[0] + c[1]*axis[1]
		lo, hi = min(lo, p), max(hi, p)
	}
	return lo, hi
}

// TestSmallTerminalKeepsMerging rains characters for ten minutes while the
// terminal changes size, down to too small, and checks the pile never gets
// stuck or leaves the world.
func TestSmallTerminalKeepsMerging(t *testing.T) {
	m := New(Config{Source: noSource{}, Per: 1, Scope: "test", Seed: 2})
	sizes := [][2]int{{30, 10}, {60, 20}, {30, 10}, {29, 10}, {120, 40}, {30, 9}}
	now := time.Unix(0, 0)
	for i := range 600 * fps {
		if i%(100*fps) == 0 {
			sz := sizes[i/(100*fps)]
			m.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		}
		if i%(fps/2) == 0 {
			m.Update(pollMsg{delta: 1}) // two characters a second
		}
		now = now.Add(time.Second / fps)
		m.frame(now)
		checkChars(t, m, m.merge == nil && m.world.Settled())
		_ = m.render()
	}
	if m.giants < 10 || len(m.chars) > 30 {
		t.Fatalf("after ten minutes: %d giants, %d characters", m.giants, len(m.chars))
	}
}

func TestCyclePer(t *testing.T) {
	m := newTestModel(0)
	m.spawn.per, m.spawn.carry = 1_000, 900
	var got []int64
	for range 4 {
		m.cyclePer()
		got = append(got, m.spawn.per)
	}
	if !slices.Equal(got, []int64{10_000, 100, 500, 1_000}) {
		t.Fatalf("per cycles through %v", got)
	}
	if m.spawn.queue != 9 { // 900 carried tokens become 9 characters at 100 each
		t.Fatalf("queue %d after dropping to 100/char, want 9", m.spawn.queue)
	}
	m.spawn.per = 2_000 // a custom --per jumps to the next step up
	if m.cyclePer(); m.spawn.per != 10_000 {
		t.Fatalf("2000 cycled to %d, want 10000", m.spawn.per)
	}
}

func TestEmptyPileShowsHint(t *testing.T) {
	m := newTestModel(0)
	if !strings.Contains(m.render(), "waiting for new tokens") {
		t.Fatal("empty pile should show the hint")
	}
	m.spawnBody()
	if strings.Contains(m.render(), "waiting for new tokens") {
		t.Fatal("hint still shown with a character on screen")
	}
}
