// Package clock is where the wall clock is read, and the only place it is.
// The interface is declared by each consumer that needs one; this package holds
// the implementations.
package clock

import "time"

// System is the real clock. It is the only time.Now() in the codebase outside a
// test.
type System struct{}

func (System) Now() time.Time { return time.Now().UTC() }

// Fixed is a clock stopped at an instant, for tests and for replaying a day
// that has already happened.
type Fixed struct{ At time.Time }

func (f Fixed) Now() time.Time { return f.At }

// Stepping is a clock a test moves by hand.
type Stepping struct{ At time.Time }

func (s *Stepping) Now() time.Time { return s.At }

// Advance moves the clock forward, and returns the new time for convenience.
func (s *Stepping) Advance(d time.Duration) time.Time {
	s.At = s.At.Add(d)
	return s.At
}
