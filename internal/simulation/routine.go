package simulation

import "time"

/*
Routines: rules for the body. The player configures sleep, workout, diet and
organization and watches them move the panel over the days that follow (§3.4). A
routine is already a rule (§3.14), and the playbook is the same concept pointed
at judgement, so the two are one system with one interface language. This file is
the half with numbers in it.

A routine is a standing answer to a question about the body, chosen from a closed
vocabulary, and the day it produces is derived from that answer rather than typed
in. The sentences a player reads and picks between live in internal/routine.

A routine produces a Day for a date, and two things make that more than a lookup.

The week has a shape. "Three hard sessions a week, easy in between" is a training
plan and "train hard" is not, so the training half of a routine is seven entries
indexed by weekday, and a rest day is part of the plan rather than a day the plan
failed.

An intention is not an outcome. What happens is the intention held to whatever
the player's organization routine can hold it to, which makes the other three
routines matter more the more ambitious they are: a player who intends five hours
of sleep loses nothing by being disorganised, and one who intends eight and a
half loses most of the difference.

Both ends of the arrangement cost. A life nobody planned carries a low-grade
stress of its own and a life planned to the hour carries a larger one, so the
middle is where the trade sits — "maximise organization" has to lose to
something, or the vocabulary has a right answer in it (§3.14).

Nothing here is random. Slippage is spread evenly across the days by kept() below
rather than rolled, so the same routine over the same dates projects forward now
and applies in the rollover later with the two agreeing exactly (§3.1).
*/

// Order is how tightly a life is arranged — the organization routine.
type Order int

const (
	// Unsaid is the zero value, and means the day says nothing about how it was
	// arranged. It is what a day the player narrates by hand carries, so a Day
	// built without an Order behaves exactly as it did before organization
	// existed: nothing was intended, so nothing slipped and nothing was paid.
	Unsaid Order = iota
	// Loose is a life kept in the player's head.
	Loose
	// Listed is tomorrow written down before sleeping.
	Listed
	// Regimented is every hour of the week assigned.
	Regimented
)

// Routine is a configured set of rules for the body.
//
// The fields are mechanics rather than choices: internal/routine turns the
// player's answers into one of these, and this package never learns that the
// answers had sentences attached to them.
type Routine struct {
	// Sleep is the night the routine protects.
	Sleep time.Duration
	// Training is the week, indexed by time.Weekday — Sunday first, as the
	// standard library counts.
	Training [7]Training
	// Diet is how the routine eats.
	Diet Diet
	// Organization is how much of the above actually happens.
	Organization Order
}

// driftSleep is the night that happens when nothing is arranged: late, short,
// and ended by an alarm rather than by having slept enough. Every routine is
// measured as a distance from it, because that is what a routine is — the
// difference between what a life does on its own and what it was told to do.
const driftSleep = 5*time.Hour + 30*time.Minute

// dietPhase offsets the diet's slippage from the training's.
//
// Without it the same days slip on both, and a routine's bad days arrive as one
// catastrophe a week rather than as the scattered friction they are. Three is a
// phase shift rather than a second pattern: the spread stays even, which is the
// property kept() exists to give.
const dietPhase = 3

// On is the day this routine produces for a date.
//
// Pure in the date: the same routine and the same day are the same living,
// whether this is a projection of next fortnight or the rollover applying it for
// real. That equality is the whole reason a forecast can be shown at all.
func (r Routine) On(on Date) Day {
	held := adherence(r.Organization)
	n := on.ordinal()

	// Sleep slips by degree rather than all at once, because it does: a
	// disorganised week does not cost whole nights, it costs the last hour of
	// each of them. A routine intending less than the drift is left alone —
	// there is nothing there to fail to keep.
	sleep := r.Sleep
	if sleep > driftSleep {
		sleep = driftSleep + time.Duration(float64(sleep-driftSleep)*held)
	}

	// A session, on the other hand, either happens or does not, and a slipped
	// one becomes the easier version rather than nothing — which is what
	// actually happens to people, and keeps a missed hard day from reading as
	// the same event as a rest day the plan asked for.
	training := r.Training[on.weekday()]
	if !kept(n, held) {
		training = easier(training)
	}

	diet := r.Diet
	if !kept(n+dietPhase, held) {
		diet = worse(diet)
	}

	return Day{Sleep: sleep, Training: training, Diet: diet, Order: r.Organization}
}

// adherence is how much of an intention an arrangement holds.
//
// The returns diminish on purpose. Writing tomorrow down is most of the benefit;
// assigning every hour of the week buys the last fifteen percent and pays for it
// in orderHRV and orderCortisol below. That is the trade the organization
// routine exists to pose, and a player who finds it by over-organising and
// watching their recovery fall has learned it in the way §3.6 asks for.
func adherence(o Order) float64 {
	switch o {
	case Loose:
		return 0.50
	case Listed:
		return 0.85
	case Regimented:
		return 0.95
	default:
		// Unsaid. Nothing was intended, so nothing failed to happen.
		return 1
	}
}

// orderHRV is the autonomic cost of how a life is arranged, in milliseconds off
// the HRV target.
//
// Chaos is a stressor and so is a schedule with no slack in it. Both are real
// and both are the same axis read from opposite ends, which is why the quiet
// answer is the middle one rather than the extreme.
func orderHRV(o Order) float64 {
	switch o {
	case Loose:
		return 2
	case Regimented:
		return 5
	default:
		return 0
	}
}

// orderCortisol is the same cost on the stress axis, in nmol/L.
func orderCortisol(o Order) float64 {
	switch o {
	case Loose:
		return 25
	case Regimented:
		return 55
	default:
		return 0
	}
}

/*
Projected is one day of a forecast: the date, the day the routine produced, and
the body it left behind.
*/
type Projected struct {
	On   Date
	Day  Day
	Body Body
}

/*
Project runs a routine forward from a body and reports what it would do.

This is what makes a routine legible on the day it is configured rather than a
fortnight later. §3.4 wants the player to *watch* a routine move their
biomarkers, and the honest version of that takes real days — so the game shows
both: the days as they are lived, and the arc they are on if nothing changes.

It is composed on read and never stored (§3.3). It has to be: a projection is a
statement about a routine that has not happened yet, and the moment one is
persisted it is a second answer waiting to disagree with the first.

No clock, because a forecast has no instant — the caller says which day it
starts from, and the days after it follow from the calendar rather than from
anything this package could read.

The post is a parameter rather than an assumption, and it is not optional. A
player holding a job is living a different fortnight from the one their routines
describe (§3.14, work.go), and a forecast that projected the routine alone would
be the game quietly promising a body the rollover is never going to produce. The
zero [Post] is somebody who holds no job, which costs nothing.
*/
func Project(b Body, r Routine, p Post, from Date, days int) []Projected {
	if days <= 0 {
		return nil
	}
	out := make([]Projected, 0, days)
	on := from
	for range days {
		d := Working(r.On(on), p)
		b = lived(b, d)
		out = append(out, Projected{On: on, Day: d, Body: b})
		on = on.Next()
	}
	return out
}

/* ── the even spread ──────────────────────────────────────────────────── */

// kept reports whether the nth day of a stretch holds an intention kept at
// `rate`.
//
// Bresenham rather than a coin toss. Nothing in this package is random (§3.1),
// and an even spread is also the truer picture: a routine kept four days in five
// misses one day a week, not five days in a row and then twenty perfect ones.
// The line-drawing test — has the running total crossed another whole unit —
// gives exactly that, in one comparison and with no state.
func kept(n int, rate float64) bool {
	switch {
	case rate >= 1:
		return true
	case rate <= 0:
		return false
	}
	return int(float64(n+1)*rate) > int(float64(n)*rate)
}

// easier is a session downgraded one step: what a hard day becomes when it does
// not happen.
func easier(t Training) Training {
	if t > Rest {
		return t - 1
	}
	return Rest
}

// worse is a day's eating downgraded one step, for the same reason.
func worse(d Diet) Diet {
	if d > Poor {
		return d - 1
	}
	return Poor
}

/* ── the calendar ─────────────────────────────────────────────────────── */

// Next is the day after this one.
//
// Exported because the rollover (§3.3) walks the calendar with it: a player who
// was away for three days has three days to live, in order, and "the day after"
// is the only arithmetic that gets that right across a month boundary.
func (d Date) Next() Date { return On(d.at().AddDate(0, 0, 1), time.UTC) }

// Plus is the date n days after this one, and n days before it when n is
// negative.
//
// The rollover uses it to skip: a player who has been away for a season is
// caught up from a bounded window rather than from wherever they stopped, and
// the start of that window is this arithmetic (§3.3).
func (d Date) Plus(n int) Date { return On(d.at().AddDate(0, 0, n), time.UTC) }

// Before reports whether this date falls strictly before another.
//
// The rollover's guard, and the reason it is here rather than in a caller: a
// Date is three numbers and comparing them by hand is three chances to get a
// month boundary wrong.
func (d Date) Before(o Date) bool { return d.at().Before(o.at()) }

// Until counts the days from this date to another — negative when o is the
// earlier of the two, and zero when they are the same day.
//
// Whole days by construction: both instants are midday UTC, where no
// daylight-saving rule applies, so the difference is always an exact multiple
// of twenty-four hours.
func (d Date) Until(o Date) int { return int(o.at().Sub(d.at()) / (24 * time.Hour)) }

// At is the date as an instant — midday UTC, the same one every other piece of
// calendar arithmetic in this package uses.
//
// Exported because a caller replaying a missed day has to stamp its effects
// with the day they belong to rather than with now, which is the thing Advance's
// injected clock exists to make possible (§3.3).
func (d Date) At() time.Time { return d.at() }

// weekday is which day of the week this is. A fact about the calendar rather
// than about the player, so it is read in UTC and means the same everywhere.
func (d Date) weekday() time.Weekday { return d.at().Weekday() }

// ordinal numbers a date, so that consecutive days are consecutive integers.
//
// Days since the Unix epoch, which is an arbitrary origin and is allowed to be:
// nothing depends on where the counting starts, only on it advancing by one a
// day. Midday rather than midnight so that no daylight-saving rule anywhere can
// make two calendar dates land on the same count.
func (d Date) ordinal() int {
	days := int(d.at().Unix() / 86400)
	if days < 0 {
		// The game dates nothing before 1970. A floor rather than a negative
		// keeps the spread in kept() well defined instead of reflected about
		// zero, which would make a routine slip differently before the epoch.
		return 0
	}
	return days
}

// at is the date as an instant, at midday UTC. Constructing a time.Time is not
// reading the clock — §3.1 forbids time.Now, and uses time's types everywhere.
func (d Date) at() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 12, 0, 0, 0, time.UTC)
}
