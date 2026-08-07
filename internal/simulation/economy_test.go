package simulation

import (
	"testing"
	"time"
)

func TestThePaidActionCapStopsEarningAndNothingElse(t *testing.T) {
	t.Parallel()

	offer := Dollars(40)
	cases := []struct {
		takenToday int
		pays       Money
		left       int
	}{
		{0, offer, 3},
		{1, offer, 2},
		{2, offer, 1},
		{3, 0, 0},
		{9, 0, 0},
	}

	for _, c := range cases {
		if got := Earn(offer, c.takenToday); got != c.pays {
			t.Errorf("with %d paid actions taken, an offer of %d pays %d, want %d",
				c.takenToday, offer, got, c.pays)
		}
		if got := EarningsLeft(c.takenToday); got != c.left {
			t.Errorf("with %d taken, %d actions left, want %d", c.takenToday, got, c.left)
		}
	}
}

// The day boundary is the player's local midnight (§3.3), which is the whole
// reason this is a calendar date rather than a duration since something.
func TestTodayIsTheDayWhereThePlayerIs(t *testing.T) {
	t.Parallel()

	auckland, err := time.LoadLocation("Pacific/Auckland")
	if err != nil {
		t.Skipf("no timezone database on this machine: %v", err)
	}
	losAngeles, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skipf("no timezone database on this machine: %v", err)
	}

	// 2026-08-06 21:00 UTC. Already the 7th in Auckland; still the 6th in
	// California — the same instant, two different game days.
	now := at(time.Date(2026, 8, 6, 21, 0, 0, 0, time.UTC))

	if got := Today(now, auckland); got != (Date{2026, time.August, 7}) {
		t.Errorf("Auckland reads %v, want 2026-08-07", got)
	}
	if got := Today(now, losAngeles); got != (Date{2026, time.August, 6}) {
		t.Errorf("Los Angeles reads %v, want 2026-08-06", got)
	}
	if got := Today(now, nil); got != (Date{2026, time.August, 6}) {
		t.Errorf("an unknown timezone reads %v, want UTC's 2026-08-06", got)
	}
}
