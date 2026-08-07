package simulation

import "time"

/*
Money, and the one rule V1 has about earning it.

HighLevelOverview.md §3.4: income is primarily *structural* — it comes from job
tier, performance and assets rather than from repeating actions. Some actions do
pay directly, but **the number of money-producing actions per real day is
capped**, starting at three. The cap applies to earning only, never to play: a
player can take unlimited non-earning actions in a day.

The cap is what stops the game becoming a clicker, which would quietly replace
its own thesis — a life that improves because you arranged it well — with the
opposite one.
*/

// Money is an amount in cents.
//
// Integer cents rather than a float, because money is counted rather than
// measured and a rounding error in a balance is a bug the player finds before
// anyone else does.
type Money int64

// Cents and Dollars build a Money from the two units it is written in.
func Cents(n int64) Money    { return Money(n) }
func Dollars(n int64) Money  { return Money(n * 100) }
func (m Money) Cents() int64 { return int64(m) }

// PaidActionsPerDay is §3.4's cap. Three is the design's stated starting point.
const PaidActionsPerDay = 3

/*
Earn reports what an action offering `offer` actually pays, as the nth paid
action of the player's day.

§17 lists the shape past the cap — hard stop or steep drop-off — as a tuning
question left open. This is the hard stop, which is the simpler of the two to
reason about while everything above it is being built, and it is one function to
change when the call is made. Nothing else in the codebase decides this.
*/
func Earn(offer Money, takenToday int) Money {
	if takenToday >= PaidActionsPerDay {
		return 0
	}
	return offer
}

// EarningsLeft is how many paid actions a player has left today.
//
// Derived rather than stored (§3.3): what is persisted is how many have been
// taken and which local day that count belongs to, and this is read from those
// two. A stored "remaining" would be wrong every midnight.
func EarningsLeft(takenToday int) int {
	left := PaidActionsPerDay - takenToday
	if left < 0 {
		return 0
	}
	return left
}

/*
Date is a calendar day in somebody's own timezone.

One game day is one real calendar day (§3.1) and the boundary is the *player's*
local midnight (§3.3), so "today" is a question that cannot be answered without
knowing where they are. Holding it as a calendar date rather than an instant is
what makes "is this count still today's" a comparison rather than a subtraction
with a timezone hidden in it.
*/
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// Today is the player's current local day, read from the injected clock.
//
// A nil zone means UTC, which is the sane reading of "we do not know where this
// player is yet" — it is a real timezone rather than a silent local default
// that would differ between a laptop and a server.
func Today(c Clock, zone *time.Location) Date {
	if zone == nil {
		zone = time.UTC
	}
	return On(c.Now(), zone)
}

// On is the calendar date an instant falls on, in a given timezone.
func On(t time.Time, zone *time.Location) Date {
	if zone == nil {
		zone = time.UTC
	}
	y, m, d := t.In(zone).Date()
	return Date{Year: y, Month: m, Day: d}
}
