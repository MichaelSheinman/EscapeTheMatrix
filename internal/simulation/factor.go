package simulation

import "math"

/*
Progression, and why there is no level bar.

HighLevelOverview.md §3.5: **no level.** Progression is a set of factors, most of
them hidden from the player, mixed into one composite the player understands as
growth. The composite gets an evocative name — Karma — and it replaces the level
bar entirely.

	| Factor              | Grown by                                    | Shown |
	| Health              | the whole biomarker panel, as biological age | yes  |
	| Money               | amount held                                  | yes  |
	| Intelligence        | activities that demonstrate it               | no   |
	| Emotional stability | behaviour in interactions                    | no   |
	| Kindness            | how helpful the player is                    | no   |

Three properties of this file are the design rather than an implementation
detail, and each of them is asserted in factor_test.go:

 1. **Karma is composed, never accumulated.** There is no counter anywhere that
    Karma is added to. It is a function of a body, a balance and three credit
    totals, so it cannot be stale and cannot disagree with the things it is a
    reading of — §3.3's "everything derived is computed on read".

 2. **It can fall.** A body that has been wrecked for a fortnight reads lower;
    money spent is money no longer held. That is what keeps it from being a
    level bar wearing a spiritual name (§3.2), and it is why the composite is
    defensible as *Karma* rather than as a score.

 3. **Money's contribution flattens.** §3.5 draws the curve: money genuinely
    helps early, and past an ultra-wealthy point the relationship inverts. V1
    builds the rising side and the plateau — diminishing returns above $1M —
    and V2 builds the flip. The plateau is here; the downslope is not.

The formula is never stated to the player, and this file is the whole of it.
*/

/*
Factor is one component of the composite (§3.5).

Wealth is the identifier for the money factor because [Money] is already the
type of an amount in this package. Its key and its name are "money"; nothing
outside this file sees the Go spelling.
*/
type Factor int

const (
	Health Factor = iota
	Wealth
	Intelligence
	Stability
	Kindness
)

// Factors is the board, in the order §3.5 tabulates it: the two that are shown,
// then the three that are not.
var Factors = []Factor{Health, Wealth, Intelligence, Stability, Kindness}

// Key is the factor's stable identifier — what a database row and a wire field
// are keyed by. Never rename one without a migration.
func (f Factor) Key() string {
	switch f {
	case Health:
		return "health"
	case Wealth:
		return "money"
	case Intelligence:
		return "intelligence"
	case Stability:
		return "stability"
	default:
		return "kindness"
	}
}

func (f Factor) String() string { return f.Key() }

/*
Shown reports whether the player is ever told this factor exists by name.

Health and Money are revealed; the rest are not, and **that is the hook** (§3.5):
the questions "what are the hidden factors, and what grows them" are the thing
the game is asking the player to work out. Nothing that is not Shown may have
its name or its value cross the wire — internal/factor is where that is enforced,
in the same way internal/health enforces instrumentation, on the way out.
*/
func (f Factor) Shown() bool { return f == Health || f == Wealth }

/*
Granted reports whether this factor is scored from committed choices.

The two revealed factors are not: Health is read off the body and Money off the
balance, both of which are already true about the world and neither of which
anybody awards. The three hidden ones accumulate credit from choices the player
has committed to — an option taken in a challenge (§5.1), a conflict answered
(§4), an endorsement received (§3.5) — and never from anything typed. §7 is
explicit: hidden factors are scored from committed choices, not from text.
*/
func (f Factor) Granted() bool { return !f.Shown() }

/*
Credit is what the three hidden factors have accumulated.

A running total per factor rather than a history: §3.2 enumerates the history
surfaces the game keeps, and a factor ledger is not one of them — the *deltas*
are recorded by whatever awarded them (a challenge outcome record), and what is
current state is the total. Two timelines of the same thing is how you end up
unable to render either.

The numbers are credit, not scores. What a credit total reads as on the board is
[creditStanding], which saturates, so a total is unbounded above and the reading
it produces is not.
*/
type Credit struct {
	Intelligence float64
	Stability    float64
	Kindness     float64
}

// Of reads one factor's credit. Zero for the two that are not granted.
func (c Credit) Of(f Factor) float64 {
	switch f {
	case Intelligence:
		return c.Intelligence
	case Stability:
		return c.Stability
	case Kindness:
		return c.Kindness
	default:
		return 0
	}
}

// Plus adds an amount to one factor and returns the new total.
//
// A total may not go below zero: a delta is allowed to be negative (§5.1's
// authored options are not all rewards) but a factor that has been driven to
// nothing has nowhere further to fall, and an unbounded negative would make one
// bad afternoon unrecoverable.
func (c Credit) Plus(f Factor, n float64) Credit {
	switch f {
	case Intelligence:
		c.Intelligence = math.Max(0, c.Intelligence+n)
	case Stability:
		c.Stability = math.Max(0, c.Stability+n)
	case Kindness:
		c.Kindness = math.Max(0, c.Kindness+n)
	}
	return c
}

/*
Ceilings are the highest a factor may read where the player is now.

§3.15's table, and all four of the rows that have an effect in V1:

	| Intelligence        | capped. Plateaus once the rule set works           |
	| Money               | capped by that environment's economic ceiling      |
	| Emotional stability | capped by situational variety                      |
	| Kindness            | same — capped by the range of chances to help      |
	| Health              | no environment effect in V1                        |

The environment does not *cause* any of them — it limits the exposure they grow
from, and a single population has a finite repertoire of situations to respond
to and a finite amount of money moving through it. Which is why every cap here
is applied to the *reading* rather than to what it is a reading of: the player
has genuinely learned what they learned and genuinely holds what they hold, and
what has run out is anywhere new to demonstrate it or anyone here it is worth
anything to.

Three of the four are properties of the *place* and are authored per population
in internal/environment. The fourth is not, and the difference is the whole of
§3.15's intelligence row: adaptive capacity "plateaus once the rule set works"
and "rises as the player rebuilds a rule set for different people", so what caps
it is how many rule sets the player has built rather than which one they are
standing in. [Reach] is that cap, and a caller fills this field from it.

Zero means no ceiling, in the same way [Order]'s zero value says nothing was
arranged: an environment nobody has authored a ceiling for is not an environment
with a ceiling of nothing. internal/environment is what authors them, and its
`Nowhere` is the zero this sentence describes.
*/
type Ceilings struct {
	// Intelligence is [Reach] of the player's breadth, not a fact about the
	// population. See above.
	Intelligence float64
	Stability    float64
	Kindness     float64
	// Money is the environment's economic ceiling, as an amount held rather
	// than as a score — "a Green setting affords less" in the design's terms,
	// stated in the only unit an author can reason about. It caps what money is
	// worth *here*, and it caps nothing else: a balance is the character
	// module's and no reading of it may write one.
	Money Money
}

func (c Ceilings) of(f Factor) float64 {
	switch f {
	case Intelligence:
		return c.Intelligence
	case Stability:
		return c.Stability
	case Kindness:
		return c.Kindness
	default:
		return 0
	}
}

/*
Standing is everything Karma is composed from: the inputs, and only the inputs.

Note what is *not* in here — a Karma, a total, a level, a last-computed-at. The
composite is the return value of a function over this struct and is stored
nowhere (§3.3).
*/
type Standing struct {
	// Body is the whole biomarker panel. The health factor is read from its
	// biological age, which is itself composed on read (§3.5).
	Body Body
	// Purse is money held. Not money earned: §3.5's factor is the amount held,
	// which is why spending lowers it and why the game can assert something
	// about hoarding at all.
	Purse Money
	// Credit is the three hidden totals.
	Credit Credit
	// Ceilings are where the player is standing, expressed as caps (§3.15).
	Ceilings Ceilings
}

/*
Reading is one composition of a Standing: the composite, and the per-factor
scores it was made of.

Every score is 0–100. The scores for the hidden factors are in here because this
is the package that tunes them and a tuning test has to be able to see them —
**not** because anything downstream may show them. internal/factor is the
boundary that strips them, and it is the only thing that reads this type.
*/
type Reading struct {
	// Karma is the composite, 0–[KarmaFull].
	Karma float64

	Health       float64
	Money        float64
	Intelligence float64
	Stability    float64
	Kindness     float64
}

// Of reads one factor's score.
func (r Reading) Of(f Factor) float64 {
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

// KarmaFull is what a Standing at the top of every factor at once reads.
//
// A round thousand rather than a hundred, so that a day of ordinary living moves
// the number visibly without the scores underneath it having to be integers —
// and so that the composite never looks like a percentage, which is what a level
// bar looks like.
const KarmaFull = 1000

/*
Weights are how the factors combine.

§17 lists this as a thing to settle during implementation, which is precisely why
it is a value in the pure core rather than a set of literals inside a function:
a tuning pass is `Weights{...}.Read(standing)` in a table-driven test that runs
in milliseconds, and the shipped weighting is [Standard].

The hidden three carry half the composite between them. That is the design, not
an accident of tuning — if the two revealed factors decided most of Karma, the
board would read as a health-and-wealth score with decoration, and the question
the game is asking ("what else is being measured?") would answer itself.

There is deliberately **no breadth multiplier**. §3.15 is explicit that breadth
pays through the individual factors — through intelligence rising only when a
rule set is rebuilt elsewhere, and through the ceilings above — rather than
through a global bonus. A versatility term here would be a second, louder
statement of a thesis the factors already encode.
*/
type Weights struct {
	Health       float64
	Money        float64
	Intelligence float64
	Stability    float64
	Kindness     float64
}

// Standard is the weighting the game ships with.
//
// A function rather than a package variable so that it cannot be reassigned at
// runtime: the core is deterministic (§3.1), and a global anybody could write to
// is a global that eventually differs between two players in the same process.
func Standard() Weights {
	return Weights{
		// Health leads, because it is the factor the player can watch move and
		// the one the whole simulation underneath already exists to produce.
		Health: 0.26,
		// Money is the smallest of the five on purpose. It is the factor with
		// the loudest feedback in an ordinary life and the one §3.5 is arguing
		// against over-weighting; it is also the only one that plateaus.
		Money: 0.18,
		// Adaptive capacity — the factor §3.15 says cannot grow without moving.
		Intelligence: 0.20,
		Stability:    0.18,
		Kindness:     0.18,
	}
}

func (w Weights) of(f Factor) float64 {
	switch f {
	case Health:
		return w.Health
	case Wealth:
		return w.Money
	case Intelligence:
		return w.Intelligence
	case Stability:
		return w.Stability
	default:
		return w.Kindness
	}
}

// Sum is what the weights add up to. One, or the composite is not on the scale
// it claims to be — asserted rather than assumed.
func (w Weights) Sum() float64 {
	return w.Health + w.Money + w.Intelligence + w.Stability + w.Kindness
}

/*
Karma composes the standing under the shipped weighting.

The one function the rest of the backend calls. Everything it reads is an input
that was persisted; everything it returns is derived and is thrown away.
*/
func Karma(s Standing) Reading { return Standard().Read(s) }

// Read composes the standing under this weighting.
func (w Weights) Read(s Standing) Reading {
	r := Reading{
		Health:       healthStanding(s.Body),
		Money:        moneyStanding(s.Purse, s.Ceilings.Money),
		Intelligence: creditStanding(s.Credit.Intelligence, s.Ceilings.of(Intelligence)),
		Stability:    creditStanding(s.Credit.Stability, s.Ceilings.of(Stability)),
		Kindness:     creditStanding(s.Credit.Kindness, s.Ceilings.of(Kindness)),
	}

	total := 0.0
	for _, f := range Factors {
		total += w.of(f) * r.Of(f)
	}
	r.Karma = clamp(total*KarmaFull/100, 0, KarmaFull)
	return r
}

/*
The health factor: the whole panel, read as one number.

Not a second model of the body. It is [BiologicalAge] — already the composition
of every biomarker, already computed on read, already the thing §3.5 says health
surfaces as — mapped onto the 0–100 the other factors are on. So there is exactly
one statement in the codebase about what a healthy body is, and tuning it moves
both the readout and the composite together.

The window is deliberately wide at the top: a body reading 20 is a remarkable
one and there has to be somewhere above the reference to go, or the strongest
players stop being able to move the factor at all.
*/
const (
	// bestAge reads 100. Younger than any body this simulation produces.
	bestAge = 20.0
	// worstAge reads 0.
	worstAge = 62.0
)

func healthStanding(b Body) float64 {
	return clamp((worstAge-BiologicalAge(b))/(worstAge-bestAge)*100, 0, 100)
}

/*
MoneyPlateau is §3.5's threshold: the point above which more money buys less
Karma.

$1M, and the design says so in as many words — "V1 builds the rising side and the
plateau: diminishing returns above $1M." The flip past ultra-wealth, where the
relationship inverts and further accumulation *lowers* Karma, is V2 and is
deliberately absent below.
*/
const MoneyPlateau = Money(1_000_000_00) // $1,000,000

// plateauStanding is what the threshold itself reads.
//
// Not 100. There is real ground above the plateau — §3.5's curve keeps rising,
// it simply stops paying — and a player who has arrived at a million has to have
// somewhere left to go, or the factor becomes a completed achievement.
const plateauStanding = 72.0

/*
MoneyStanding is the money factor: how much is held, read on §3.5's curve.

Below the plateau, a rising concave curve — money genuinely helps, and it helps
most when there is least of it, which is the honest shape of what money buys a
poor person versus a comfortable one. Above the plateau, an exponential approach
to the top: the curve still rises and each dollar is worth a fraction of what the
same dollar was worth below.

The two halves meet at the plateau by construction, so there is no step in the
composite at a million dollars — what changes is the slope, which is the whole
of what the design asks for. §3.15 calls returns falling "the game's consistent
teaching language": nothing is explained, the number simply stops answering.
*/
func MoneyStanding(m Money) float64 {
	if m <= 0 {
		// Debt is not modelled (§3.4 has no borrowing), so the floor is having
		// nothing rather than owing something.
		return 0
	}

	d := float64(m) / float64(MoneyPlateau)
	if d <= 1 {
		// The exponent is what makes the first thousand dollars worth more than
		// the hundred-thousandth. §17 tuning.
		return plateauStanding * math.Pow(d, 0.55)
	}

	// Above the plateau: what is left of the scale, approached and never
	// reached. `aboveScale` is how many further plateaus it takes to cover
	// about two thirds of the remaining distance.
	const aboveScale = 3.0
	return plateauStanding + (100-plateauStanding)*(1-math.Exp(-(d-1)/aboveScale))
}

/*
moneyStanding is the same curve, read where the player is standing.

§3.15: money is "capped by that environment's economic ceiling — a Green setting
affords less". The cap is applied to the amount before the curve rather than to
the score after it, which is what makes an authored ceiling a number an author
can reason about: it is *the most money there is worth having here*, in dollars,
and what it reads as is whatever the one money curve says it reads as. A second
number stated on the 0–100 scale would be a second opinion about the curve.

The balance is untouched. §3.15's ceilings limit exposure rather than cause
anything, so a player who has more than the place can use still *has* it — they
carry it with them, and the day they live somewhere with a higher ceiling it is
worth what it always was. That is the mechanic: an environment can run out of
what money is good for without anybody being robbed.
*/
func moneyStanding(m, ceiling Money) float64 {
	if ceiling > 0 && m > ceiling {
		m = ceiling
	}
	return MoneyStanding(m)
}

/*
RuleSet is the credit an environment has to have *offered* a player before they
count as having built a rule set for its people.

Offered rather than paid, for the same reason [Familiarity] is measured that way:
it is the closest honest proxy the mechanics have for effort spent, and counting
the decayed payout would make a well-worn environment progressively harder to
have been understood. Against [MasteryScale] it is a little under half, so a
place counts as worked out somewhat before it stops paying — a player who has
built a working rule set should be credited with it while there is still a reason
to be there.
*/
const RuleSet = 200.0

/*
Reach is the highest intelligence may read for a player who has built this many
rule sets.

§3.15's intelligence row, which is the one cap that is not a fact about a place:
"capped — plateaus once the rule set works" where the player is, and "rises as
the player rebuilds a rule set for different people" when they move. So the
ceiling is a function of *breadth*, and it is the whole of what the design means
by "Karma should reward breadth over depth" — several working rule sets beat one
perfect one, paid through the intelligence factor rather than through a global
versatility bonus that would say the same thing twice (see [Weights]).

One rule set caps the factor at [reachStep] short of the top, which is a real
plateau a player can feel and cannot grind past: more credit in the same
population reads as the same number. Each further set closes a fixed fraction of
what is left, so the fourth is worth much less than the second and the scale is
never reached — there is always somewhere else.

Fewer than one is treated as one. A player is always somewhere, and a zero would
mean *no ceiling* under this package's convention, which is the opposite of what
having been nowhere should say.
*/
const reachStep = 0.55

func Reach(sets int) float64 {
	if sets < 1 {
		sets = 1
	}
	return 100 * (1 - math.Pow(reachStep, float64(sets)))
}

/*
CreditHalf is the credit at which a hidden factor reads halfway up its scale.

The saturating curve below is doing the same job the money curve does, for the
same reason: early credit in a factor the player has barely touched is worth a
great deal, and the thousandth act of ordinary kindness moves the board by
almost nothing. It is also what makes the factor unbounded in its input and
bounded in its output, so no amount of grinding one channel can drown the other
four.
*/
const CreditHalf = 240.0

func creditStanding(credit, ceiling float64) float64 {
	if credit <= 0 {
		return 0
	}
	v := 100 * credit / (credit + CreditHalf)
	// §3.15's cap, applied to the reading and not to what was learned.
	if ceiling > 0 && v > ceiling {
		return ceiling
	}
	return v
}

/*
Familiarity is how much of an offered reward an environment still pays.

§3.15: Karma gains from an environment decay the longer the player stays and the
more thoroughly they have mastered it — same effort, shrinking return — and that
is the pressure that makes travel the progression vehicle rather than a side
activity. The design's own illustration is a weekly return falling 240 → 180 →
95 → 40 → 12 → 4, which is roughly geometric, which is why this is an exponential
in effort rather than a linear taper.

`worked` is the raw credit this environment has been *offered*, before any
decay — not the credit actually paid out. That distinction is the design note
attached to it: "decay should track mastery, not merely elapsed time; otherwise a
player who barely engages gets pushed out as fast as one who genuinely mastered
the place." Effort spent is the closest honest proxy for mastery the mechanics
have, and counting the decayed payout instead would make the curve flatten itself
into a stall.

`days` is the weak half, on a scale long enough that it cannot push anybody out
on its own. It is there because §3.15 asks for both, and because a player who
lives somewhere for a year without engaging has still exhausted its novelty.

The floor is not zero. §3.15: returns decay but never go negative — "staying may
be unrewarding; it should not be punitive."
*/
const (
	// MasteryScale is the raw credit an environment absorbs before it pays
	// about a third of what it did. §17 tuning.
	MasteryScale = 420.0
	// TenureScale is the same for days lived there, and is deliberately long.
	TenureScale = 400.0
	// ReturnFloor is the least an exhausted environment pays.
	ReturnFloor = 0.02
)

/*
Elsewhere reports whether an environment has decayed far enough that somebody in
it starts mentioning somewhere else.

§3.15 asks for two pressures working together and is explicit that neither has
to be heavy-handed on its own: decay is the pressure, and an in-app
recommendation is the pointer. This is where the second one is allowed to speak,
and it is stated as a point on the first one's curve rather than as a rule of its
own — the game does not suggest leaving until leaving is what the numbers have
already been saying, so the suggestion is never ahead of the player's own
evidence.

Half. An environment that has paid out half of what it once did is one the
player has genuinely worked out — a little past [RuleSet], which is where a
rule set counts as built — and it is still paying enough that going is a choice
rather than a rescue. A threshold at the [ReturnFloor] would arrive long after
anybody needed it; one near the top would have the world telling people to leave
a place they had barely met.

What it emphatically does not decide is *what is said*. The recommendation
suggests the trip and never explains the mechanism (§3.15), and the pure core has
no business holding a sentence.
*/
func Elsewhere(familiarity float64) bool { return familiarity <= elsewhereAt }

const elsewhereAt = 0.5

func Familiarity(days int, worked float64) float64 {
	if days < 0 {
		days = 0
	}
	if worked < 0 {
		worked = 0
	}
	worn := worked/MasteryScale + float64(days)/TenureScale
	return clamp(math.Exp(-worn), ReturnFloor, 1)
}

/*
Grant is an authored factor delta: a committed choice, and what it is worth.

§5.1 and §7: every challenge option and every escalated daily conflict carries
authored deltas, and the hidden factors are scored from those and from nothing
else. There is no path from a text box to this type, and there is deliberately no
command that produces one — a factor a player could award themselves is not a
measurement of anything.

Only the hidden factors take grants. Health and Money are read from the world
(see [Standing]); a Grant naming either is a programming error and [Grant.Valid]
is how a caller finds out at the boundary rather than in a player's total.
*/
type Grant struct {
	Factor Factor
	// Credit is the authored amount, before the environment's familiarity is
	// applied. Negative is allowed: not every authored option is a reward.
	Credit float64
}

// Valid reports whether this grant names a factor that can be granted.
func (g Grant) Valid() bool { return g.Factor.Granted() }

/*
Pays is what a grant is actually worth in an environment this worn.

The asymmetry is deliberate. A reward decays with familiarity — that is the whole
mechanism — and a cost does not: an environment you have worn out pays you less
for what it has already taught you, but it does not charge you less for the
opposite. Discounting costs alongside rewards would make the safest place in the
game the one you had already exhausted.
*/
func Pays(offer, familiarity float64) float64 {
	if offer <= 0 {
		return offer
	}
	return offer * familiarity
}
