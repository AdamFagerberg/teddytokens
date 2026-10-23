package ui

const (
	// releaseRate is the slowest that queued characters fall, per second.
	releaseRate = 4.0
	// burstTime is about how long a burst takes to fall: bigger bursts
	// fall faster.
	burstTime = 2.0
)

// spawner turns token deltas into characters and releases queued
// characters at a steady rate, so a large burst of tokens rains in over a
// few seconds instead of all at once.
type spawner struct {
	per    int64   // tokens per character
	carry  int64   // tokens counted toward the next character
	queue  int     // characters waiting to fall
	credit float64 // release allowance accumulated while the queue is busy
	rate   float64 // characters per second for the current burst
}

// add counts tokens and queues one character per full `per` tokens.
func (s *spawner) add(tokens int64) {
	s.carry += tokens
	s.queue += int(s.carry / s.per)
	s.carry %= s.per
	s.rate = max(releaseRate, float64(s.queue)/burstTime)
}

// progress is the fraction of the way to the next character, in [0, 1).
func (s *spawner) progress() float64 {
	return float64(s.carry) / float64(s.per)
}

// release returns how many queued characters to spawn after dt seconds.
// The first character of a burst is released immediately.
func (s *spawner) release(dt float64) int {
	if s.queue == 0 {
		s.credit = 1
		return 0
	}
	s.credit += dt * s.rate
	n := min(s.queue, int(s.credit))
	s.credit -= float64(n)
	s.queue -= n
	return n
}
