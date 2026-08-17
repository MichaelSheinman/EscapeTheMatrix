package simulation

import "time"

/*
Jobs: what holding one costs a body, and what one is hired against.

Progression is lateral (§3.15), so each job is its own thing: it pays what it
pays, it asks what it asks, and what it leads to is authored on the job.

§3.4 decides why income lives in this package rather than on a button: income is
structural, coming from the job held rather than from repeating actions. The
paid-action cap in economy.go is the same rule from the other end, and together
they stop this game becoming a clicker.

Two things are here:

  - [Working], a day of a player's routines lived while holding a post. This is
    where "it costs you health if your routines are bad" is a mechanic rather
    than a promise.
  - [Requirement], a floor on the factor readings a job is hired against. A value
    rather than a function, so the capability question is asked once, in one
    place, and answered inside internal/factor.

Both are shapes rather than tables. What a particular job pays, how long its
shift is and which floors it sets are authored in content/jobs and parsed by
internal/career. Names like "warehouse nights" are content too.

# Why the toll speaks the routine's own language

[Working] takes the [Day] a routine produced and hands back the day that
actually happened, expressed in the same four terms — sleep, training, diet, and
how the week was arranged. It adds no field to [Body] and no term to
[BiologicalAge].

That is the design rather than economy. A job is not a separate axis of harm; it
is a claim on the same hours the routine was spending, and the honest way to say
"the night shift wrecks your sleep" is to take the sleep. It also means the
whole existing simulation applies to it unchanged: a player on nights accrues
real debt, adapts to training at 0.35 because of it, and reads older — and
every one of those consequences was already tuned before jobs existed.
*/

/*
Post is a job as the body meets it: how long the shift is, when it falls, and
whether it leaves the player eating what they meant to eat.

Three facts rather than a set of dials, and each of them is a real property of a
real job. A warehouse night is ten hours, against the body clock, eaten out of a
machine; a desk at the dispatch office is nine hours in the day with somewhere
to put your lunch. The difference between those two is the whole of what makes
one job punishing and another one merely long.
*/
type Post struct {
	// Hours is the shift. Together with the night the routine asked for it is
	// what claims the day, and what is left has to hold everything else — see
	// [Spare].
	Hours time.Duration
	// Nights is whether it is worked against the body clock. Daytime sleep is
	// worse sleep, so a night shift costs more than its hours.
	Nights bool
	// Fed is whether the shift leaves the player's eating alone.
	Fed bool
}

// Zero reports whether this is nobody's job — the post of a player who holds
// none, which costs nothing and pays nothing.
func (p Post) Zero() bool { return p.Hours == 0 && !p.Nights && !p.Fed }

const (
	// Whole is the day, all of it. Sleep, the shift, and whatever is left.
	Whole = 24 * time.Hour
	// Spare is how much waking time a life needs outside its job for the
	// routine to actually happen: getting there and back, cooking what the
	// diet says, the session, and the people. A day with less than this in it
	// is a day the routine loses something to.
	//
	// Six hours is deliberately generous. The claim being made is not that a
	// nine-hour job is exhausting — it is that a *twelve*-hour one leaves
	// nothing, and the threshold has to sit where that becomes true rather than
	// where every job does.
	Spare = 6 * time.Hour
	// NightToll is what working against the body clock costs, whatever else is
	// true of the shift: daylight sleep is shorter and lighter, and no amount
	// of arranging makes a bedroom dark at eleven in the morning.
	NightToll = 90 * time.Minute
	// ShortestNight is the floor. Not a body being protected — the debt ceiling
	// saturates long before this bites — but the arithmetic being kept
	// meaningful at the bottom.
	ShortestNight = 3 * time.Hour
)

/*
Working is the day a routine produced, lived while holding a post — §3.14's "it
costs you health if your routines are bad". The post takes, and the arrangement
of the life decides how much of it lands.

Three things come off a day, in this order:

  - The body clock. A night shift costs sleep on its own terms, because sleeping
    in daylight is worse sleeping. The one cost that does not depend on the shift
    being long, and why the jobs that take anybody are the punishing ones.
  - The room the routine needed. Sleep and the shift claim most of the day, and
    what is left has to hold the session, the cooking and the commuting. When it
    does not, the session becomes the easier one and the remainder comes out of
    the night.
  - The eating. A shift that does not leave the player fed is one they eat at
    badly, unless the week's food was decided before the week started.

[Order] does the absorbing in every case, so it is what decides whether a job is
survivable: a life kept in somebody's head does not survive a night shift, and a
life planned to the hour survives it while paying for the planning in cortisol
(see orderCortisol). Neither answer is free.

Pure, and applied by two callers that have to agree: the rollover living a day
that has ended, and the forecast projecting a fortnight that has not (see
[Project]).
*/
func Working(d Day, p Post) Day {
	if p.Zero() {
		return d
	}
	held := 1 - absorbed(d.Order)

	var lost time.Duration
	if p.Nights {
		lost = time.Duration(float64(NightToll) * held)
	}

	// What the day has left once the night the routine asked for and the shift
	// are both out of it, against what a life needs in order to be lived.
	if short := Spare - (Whole - d.Sleep - p.Hours); short > 0 {
		// A session on the far side of a day with no room in it. It becomes the
		// easier one rather than nothing, exactly as a slipped session in a
		// routine does — a job erodes a training week, it does not delete one.
		if !arranged(d.Order) {
			d.Training = easier(d.Training)
		}
		lost += time.Duration(float64(short) * held)
	}

	if !p.Fed && !arranged(d.Order) {
		d.Diet = worse(d.Diet)
	}

	if lost > 0 {
		d.Sleep -= lost
		if d.Sleep < ShortestNight {
			d.Sleep = ShortestNight
		}
	}
	return d
}

/*
absorbed is how much of a shift's encroachment an arrangement takes.

The same diminishing shape adherence() has, and for the same reason: writing the
week down is most of the benefit, and assigning every hour buys the rest and
pays for it elsewhere. Nothing arranged absorbs nothing, which is the correct
reading of [Unsaid] — a day nobody planned is a day the shift takes whole.

It never reaches one. A job you have arranged your entire life around is still a
job, and a routine that could cancel a night shift outright would make the
ladder's trade disappear.
*/
func absorbed(o Order) float64 {
	switch o {
	case Loose:
		return 0.15
	case Listed:
		return 0.40
	case Regimented:
		return 0.60
	default:
		// Unsaid. Nothing was arranged, so nothing was arranged around.
		return 0
	}
}

// arranged reports whether the week was decided in advance — the threshold at
// which a shift stops taking the session and the eating with it.
func arranged(o Order) bool { return o >= Listed }

/*
Requirement is the capability a job is hired against: a floor on the factor
readings, on the same 0–100 scale [Reading] is.

Every factor, including the three the player is never shown (§3.5). That is the
point rather than an oversight — §3.4's "actions build capability, capability
raises income" is only true if the capability being asked for is the same set of
things the rest of the game is quietly measuring. A job that asked only about
health and money would be a job with the puzzle taken out of it.

Zero means the job does not ask about that factor at all, in the same way
[Ceilings]' zero is no ceiling. It is a floor, so a job asking for nothing is a
job anybody can be hired into — which is what an entry-level posting is.

Nothing here decides who may *see* which part of the asking. internal/factor
answers the question and hands back a yes or a no, and the
shown half of the requirement is the only half anything downstream can name.
*/
type Requirement struct {
	Health       float64
	Money        float64
	Intelligence float64
	Stability    float64
	Kindness     float64
}

// Of is the floor this requirement puts on one factor, and zero for a factor it
// does not ask about.
func (r Requirement) Of(f Factor) float64 {
	switch f {
	case Health:
		return r.Health
	case Wealth:
		return r.Money
	case Intelligence:
		return r.Intelligence
	case Stability:
		return r.Stability
	default:
		return r.Kindness
	}
}

// Asks reports whether this requirement has an opinion about a factor.
func (r Requirement) Asks(f Factor) bool { return r.Of(f) > 0 }

// Wants is every factor this requirement asks about, in the pure core's own
// order. Stable, so the chips a player reads do not move about between reads.
func (r Requirement) Wants() []Factor {
	out := make([]Factor, 0, len(Factors))
	for _, f := range Factors {
		if r.Asks(f) {
			out = append(out, f)
		}
	}
	return out
}

// Reaches reports whether one factor of a reading clears this requirement's
// floor for it.
func (r Requirement) Reaches(read Reading, f Factor) bool { return read.Of(f) >= r.Of(f) }

// Empty reports whether this requirement asks for nothing at all — an
// entry-level posting, which every environment has to have at least one of.
func (r Requirement) Empty() bool { return len(r.Wants()) == 0 }

// Met reports whether a reading clears every floor this requirement sets.
func (r Requirement) Met(read Reading) bool {
	for _, f := range r.Wants() {
		if !r.Reaches(read, f) {
			return false
		}
	}
	return true
}
