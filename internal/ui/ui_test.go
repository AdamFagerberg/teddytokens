package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

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
	if got != int(releaseRate) {
		t.Fatalf("released %d in one second, want %v", got, releaseRate)
	}
	for s.queue > 0 {
		s.release(0.1)
	}
	if n := s.release(10); n != 0 || s.credit != 1 {
		t.Fatalf("drained queue released %d, credit %v", n, s.credit)
	}
}

func TestSplitPiles(t *testing.T) {
	if p, r := splitPiles(170, 78); p != 2 || r != 14 {
		t.Fatalf("splitPiles = %d, %d", p, r)
	}
	if c := pileCapacity(1, 1); c != 1 {
		t.Fatalf("tiny capacity = %d, want 1", c)
	}
}

func TestSquashedTop(t *testing.T) {
	const h, floor = 8, 80
	if got := squashedTop(10, h, floor, 1); got != 10 {
		t.Errorf("scale 1 moved body to %d", got)
	}
	if got := squashedTop(10, h, floor, 0); got != floor {
		t.Errorf("scale 0 left body at %d, want hidden at %d", got, floor)
	}
	if got := squashedTop(10, h, floor, -0.2); got < floor {
		t.Errorf("overshoot brought body back to %d", got)
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
	if n := len(m.world.Bodies()); n != 20 || len(m.skins) != n {
		t.Fatalf("got %d bodies, %d skins, want 20", n, len(m.skins))
	}
	if !m.world.Settled() {
		t.Fatal("startup pile is still moving")
	}
	if m.spawn.queue != 0 || m.spawn.carry != 5_000 {
		t.Fatalf("startup tokens queued: queue %d, carry %d", m.spawn.queue, m.spawn.carry)
	}
}

func TestHugeHistoryCountsClearedPiles(t *testing.T) {
	m := newTestModel(10_000 * 1_000)
	capacity := pileCapacity(120, 2*39)
	if m.cleared != 1000/capacity || len(m.world.Bodies()) != 1000%capacity {
		t.Fatalf("cleared %d with %d bodies, capacity %d", m.cleared, len(m.world.Bodies()), capacity)
	}
}

func TestKeys(t *testing.T) {
	m := newTestModel(50_000)
	press := func(r rune) tea.Cmd {
		_, cmd := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		return cmd
	}
	before := m.skins[0]
	press('t')
	if !m.teddy || m.skins[0] != before || !m.world.Settled() {
		t.Fatal("t should re-skin in place without touching physics")
	}
	press('r')
	if len(m.world.Bodies()) != 5 || m.skins[0] != before {
		t.Fatal("r should drop the same characters again")
	}
	for _, b := range m.world.Bodies() {
		if _, y := b.Pos(); y >= 0 {
			t.Fatalf("replayed body starts on screen at y=%d", y)
		}
	}
	if cmd := press('q'); cmd == nil || cmd() != tea.Quit() {
		t.Fatal("q should quit")
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); cmd == nil || cmd() != tea.Quit() {
		t.Fatal("ctrl+c should quit")
	}
}

func TestPileAtTopSquashesAndClears(t *testing.T) {
	m := newTestModel(0)
	_, floor := m.world.Size()
	h := sprites.Claude.H
	m.world.AddBody(0, float64(h), sprites.Claude.W, floor-h, 0) // a pillar up to the top
	m.skins = append(m.skins, skin{})
	m.addBody(0, 0, 0)
	m.world.Settle(600)
	now := time.Unix(0, 0)
	for i := 0; i < 300 && m.cleared == 0; i++ {
		now = now.Add(time.Second / fps)
		m.frame(now)
	}
	if m.cleared != 1 || len(m.world.Bodies()) != 0 || m.squash != nil {
		t.Fatalf("cleared %d, %d bodies left, squashing %v", m.cleared, len(m.world.Bodies()), m.squash != nil)
	}
}
