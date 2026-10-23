package ui

import "math/rand/v2"

// Demo is a fake token source for trying the app without spending tokens.
// It grows in small steps with an occasional large burst.
type Demo struct {
	rng   *rand.Rand
	total int64
}

// NewDemo returns a demo source that starts with a small existing pile.
func NewDemo(seed uint64) *Demo {
	return &Demo{rng: rand.New(rand.NewPCG(seed, seed)), total: 180_000}
}

// Total returns the tokens "spent" so far.
func (d *Demo) Total() int64 { return d.total }

// Poll returns the tokens added since the last call.
func (d *Demo) Poll() (int64, error) {
	var n int64
	switch r := d.rng.Float64(); {
	case r < 0.04:
		n = 120_000 + d.rng.Int64N(130_000) // a big tool result or long answer
	case r < 0.75:
		n = 1_000 + d.rng.Int64N(8_000)
	}
	d.total += n
	return n, nil
}
