package simulation

import "time"

/*
Jobs: what a rung of the ladder pays, and what holding it costs a body.

HighLevelOverview.md §3.14 is the specification and it is four sentences:

	**Jobs sit in tiers.** Low tiers are genuinely passive — you hold the job,
	it pays, and it costs you health if your routines are bad. Higher tiers pay
	far more and demand real involvement. Each step up requires capability plus
	an interview (§3.4). The ladder itself teaches the trade: more money costs
	more of your attention.

§3.4 says the other half, the half that decides why income lives here rather
than on a button: **income is primarily structural.** It comes from job tier,
performance and assets rather than from repeating actions — actions build
capability, capability raises tier, tier raises income. The paid-action cap in
economy.go is the same sentence read from the other end, and the two of them
together are what stop this game becoming a clicker.

# What is in this file

Three things, and they are the three §17 lists as "job tier thresholds —
capability requirements per tier":

  - **A [Ladder]**, which is what every rung pays, what it asks of the body, and
    what capability it takes to be hired into. Numbers, in the pure core,
    tunable by a unit test that runs in milliseconds (§3.1).
  - **[Working]**, which is a day of a player's routines lived while holding a
    post. This is where "it costs you health if your routines are bad" is a
    mechanic rather than a promise.
  - **[Requirement]**, a floor on the factor readings a rung is hired against.
    It is a value rather than a function so that the capability question is
    asked once, in one place, and answered inside internal/factor's veil.

What is deliberately *not* here is a name. "Warehouse nights" and "dispatch
coordinator" are content, and they live in internal/career for the same reason
the routine vocabulary lives in internal/routine: this package is the rules.

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
Tier is a rung of §3.14's ladder. Zero is holding no job at all.

An integer rather than a named enum because the ladder is a ladder: "the tier
above this one" is arithmetic, and §3.14's own table numbers the rungs. What
each number is *called* is content.
*/
type Tier int

// Unemployed is the tier of somebody who holds no job. The zero value, which is
// what a player who has never been hired has and what leaving one returns them
// to.
const Unemployed Tier = 0

/*
Delegated is the first tier that stops being passive (§3.14).

Tiers 1 and 2 are "genuinely passive — you hold the job, it pays"; tier 3 is
where a role starts generating situations the player answers through the
playbook. The line is named here rather than left implicit because the two
halves are different tickets and the boundary between them is a rule about the
game, not a fact about a table.
*/
const Delegated Tier = 3

// Held reports whether this tier is a job at all.
func (t Tier) Held() bool { return t > Unemployed }

// Passive reports whether a tier pays without being played (§3.14).
func (t Tier) Passive() bool { return t.Held() && t < Delegated }

/*
Post is a job as the body meets it: how long the shift is, when it falls, and
whether it leaves the player eating what they meant to eat.

Three facts rather than a set of dials, and each of them is a real property of a
real job. A warehouse night is ten hours, against the body clock, eaten out of a
machine; a desk at the dispatch office is nine hours in the day with somewhere
to put your lunch. The difference between those two is the whole of what makes
the first rung punishing and the second one merely long.
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
Working is the day a routine produced, lived while holding a post.

This is §3.14's "it costs you health if your routines are bad", and every line
of it is that sentence. The post takes; the *arrangement* of the life decides
how much of it lands.

Three things come off a day, in this order:

  - **The body clock.** A night shift costs sleep on its own terms, because
    sleeping in daylight is worse sleeping. This is the one cost that does not
    depend on the shift being long, and it is why the bottom rung is the
    punishing one.
  - **The room the routine needed.** Sleep and the shift together claim most of
    the day; what is left has to hold the session, the cooking and the
    commuting. When it does not, the session becomes the easier one and the
    remainder comes out of the night, because there is nowhere else left.
  - **The eating.** A shift that does not leave the player fed is a shift they
    eat at, and badly, unless the week's food was decided before the week
    started.

Note which routine does the absorbing in every case. [Order] is what decides
whether a job is survivable, which is §3.4's trade about organization made
concrete: a life kept in somebody's head does not survive a night shift, and a
life planned to the hour survives it while paying for the planning in cortisol
(see orderCortisol). Neither answer is free, which is what keeps "maximise
organization" from being the right answer.

Pure, like everything here, and applied by two callers that must agree: the
rollover living a day that has ended, and the forecast projecting a fortnight
that has not (see [Project]). A forecast that ignored the job would be the game
lying to a player about the arc they are on.
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
Requirement is the capability a rung is hired against: a floor on the factor
readings, on the same 0–100 scale [Reading] is.

Every factor, including the three the player is never shown (§3.5). That is the
point rather than an oversight — §3.4's "actions build capability, capability
raises tier" is only true if the capability being asked for is the same set of
things the rest of the game is quietly measuring. A ladder that asked only about
health and money would be a ladder with the puzzle taken out of it.

Zero means the rung does not ask about that factor at all, in the same way
[Ceilings]' zero is no ceiling. It is a floor, so a rung asking for nothing is a
rung anybody can be hired into — which is exactly what the bottom of a ladder is.

Nothing here decides who may *see* which part of the asking. internal/factor
answers the question inside its own veil and hands back a yes or a no, and the
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

// Met reports whether a reading clears every floor this requirement sets.
func (r Requirement) Met(read Reading) bool {
	for _, f := range r.Wants() {
		if !r.Reaches(read, f) {
			return false
		}
	}
	return true
}

/*
Rung is one step of the ladder: what it pays a day, what it asks of the body,
and what capability it is hired against.

Pay is per day held rather than per action taken, and that is §3.4's whole
"income is structural" made a field. There is no path in this codebase by which
holding a job produces money faster because somebody clicked more.
*/
type Rung struct {
	Tier Tier
	// Pay is what one day of holding this is worth. Structural income (§3.4):
	// it arrives because the tier is held and the day happened, and the
	// paid-action cap in economy.go has nothing to say about it.
	Pay Money
	// Post is what the tier asks of the body.
	Post Post
	// Needs is the capability the step up is hired against (§3.14). The bottom
	// rung asks for nothing, which is what makes it the bottom rung.
	Needs Requirement
}

// Ladder is §3.14's table, lowest rung first.
type Ladder []Rung

/*
Rungs is the ladder the game ships with.

A function rather than a package variable, for the same reason [Standard] is:
the core is deterministic (§3.1), and a global anybody could write to is a global
that eventually differs between two players in one process.

§17 lists "job tier thresholds — capability requirements per tier" as a thing to
settle during implementation, which is exactly why every number below is here
rather than inside a query. A tuning pass is a table-driven test.

Read the three columns together, because the ladder is an argument rather than a
list. Pay roughly doubles every two rungs. The hours rise with it and never
fall — §3.14's "the ladder itself teaches the trade: more money costs more of
your attention" — and the one genuinely brutal post is the *bottom* one, because
the night shift is the job that is hardest to climb out of and that is the
thing worth teaching.
*/
func Rungs() Ladder {
	return Ladder{
		{
			// Warehouse nights. It will take anybody, which is the whole of what
			// the bottom of a ladder is, and it costs more of a body than
			// anything above it.
			Tier: 1,
			Pay:  Dollars(88),
			Post: Post{Hours: 10 * time.Hour, Nights: true},
		},
		{
			// The dispatch desk. The first step up, and the first that asks for
			// anything: a body that has held together, and enough of the two
			// hidden factors a room full of drivers and delays is answered with.
			Tier: 2,
			Pay:  Dollars(152),
			Post: Post{Hours: 9 * time.Hour},
			Needs: Requirement{
				// Low enough that a fortnight of nights lived carefully still
				// clears it, and high enough that a fortnight of nights lived
				// badly does not. That gap is the ticket: the job at the bottom
				// takes the very thing the step out of it is hired against, so
				// the player who arranged their week can climb and the player
				// who did not is held there by the job itself.
				Health:       45,
				Intelligence: 18,
				Stability:    15,
			},
		},
		{
			// Operations. §3.14's first active tier: the role runs while the
			// player is elsewhere and hands them situations to answer (#28).
			Tier: 3,
			Pay:  Dollars(268),
			Post: Post{Hours: 10 * time.Hour, Fed: true},
			Needs: Requirement{
				Health:       52,
				Money:        8,
				Intelligence: 34,
				Stability:    30,
			},
		},
		{
			Tier: 4,
			Pay:  Dollars(455),
			Post: Post{Hours: 11 * time.Hour, Fed: true},
			Needs: Requirement{
				Health:       56,
				Money:        28,
				Intelligence: 47,
				Stability:    44,
				Kindness:     28,
			},
		},
		{
			// Where §3.14's table stops naming jobs and starts naming lives:
			// politician, founder. Twelve hours, and the requirement is most of
			// the board at once.
			Tier: 5,
			Pay:  Dollars(780),
			Post: Post{Hours: 12 * time.Hour, Fed: true},
			Needs: Requirement{
				Health:       60,
				Money:        40,
				Intelligence: 58,
				Stability:    56,
				Kindness:     44,
			},
		},
	}
}

// At is the rung at a tier, and false for a tier that is not on the ladder —
// including [Unemployed], which is the absence of a rung rather than one.
func (l Ladder) At(t Tier) (Rung, bool) {
	for _, r := range l {
		if r.Tier == t {
			return r, true
		}
	}
	return Rung{}, false
}

// Above is the next rung up from a tier, and false at the top of the ladder.
//
// It takes a tier rather than a rung so that [Unemployed] has an answer: the
// step up from holding nothing is the bottom rung, which is the case the whole
// hiring path starts at.
func (l Ladder) Above(t Tier) (Rung, bool) {
	for _, r := range l {
		if r.Tier > t {
			return r, true
		}
	}
	return Rung{}, false
}

// Post is the post at a tier, and the zero post for a tier that holds none.
//
// The convenience that keeps every caller from having to decide what an
// unemployed player's day looks like: it looks like their routine, which is
// what [Working] does with a zero post.
func (l Ladder) Post(t Tier) Post {
	if r, ok := l.At(t); ok {
		return r.Post
	}
	return Post{}
}
