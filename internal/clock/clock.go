// Package clock is where the wall clock is read, and the only place it is.
//
// technical.md §3.1 makes time an injected dependency everywhere: a Clock
// interface, never a bare time.Now(). That is not purism. One game day is one
// real calendar day and the day boundary is the *player's* local midnight
// (§3.3), so "what happens at the rollover in Auckland" is a test that gets
// written over and over — and it cannot be written at all against a function
// that asks the operating system what time it is.
//
// The interface itself is declared by each consumer that needs one, Go-style —
// internal/simulation declares its own so that it imports nothing at all. This
// package holds the two implementations: the real one, and the one tests use to
// stand still.
package clock

import "time"

// System is the real clock. It is the only time.Now() in the codebase outside a
// test, which is what makes "no direct clock reads" a property you can check by
// grepping rather than a habit.
type System struct{}

func (System) Now() time.Time { return time.Now().UTC() }

// Fixed is a clock stopped at an instant, for tests and for replaying a day
// that has already happened.
type Fixed struct{ At time.Time }

func (f Fixed) Now() time.Time { return f.At }

// Stepping is a clock a test moves by hand.
//
// Rollover, sleep debt and biomarker trends are all things that only happen
// across days, and the alternative to this is a test that sleeps.
type Stepping struct{ At time.Time }

func (s *Stepping) Now() time.Time { return s.At }

// Advance moves the clock forward, and returns the new time for convenience.
func (s *Stepping) Advance(d time.Duration) time.Time {
	s.At = s.At.Add(d)
	return s.At
}
