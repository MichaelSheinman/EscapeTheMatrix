package simulation

import (
	"math"
	"testing"
	"time"
)

/*
The tuning loop for §3.5.

§3.1 says the reason the core is pure is that "§17 lists biomarker formulas,
Karma weights, and decay curves as things to tune during implementation — and
that tuning loop must be a fast unit test, not a playthrough". This file is that
loop for the weights and the two curves, and every assertion in it is a design
statement rather than a golden number: that Karma can fall, that money flattens,
that the hidden half is half.
*/

// A body worth composing against, and the one every case below starts from.
func ordinaryBody() Body { return Starting(1.78, 78) }

func TestTheWeightsSumToOne(t *testing.T) {
	t.Parallel()

	if got := Standard().Sum(); math.Abs(got-1) > 1e-9 {
		t.Fatalf("the shipped weights sum to %v, not 1 — the composite is not on the scale it claims", got)
	}
}

// §3.5: the hidden factors are the hook. If the two revealed ones decided most
// of the composite, the board would be a health-and-wealth score with
// decoration and there would be nothing left to wonder about.
func TestHalfOfKarmaComesFromFactorsThePlayerIsNeverShown(t *testing.T) {
	t.Parallel()

	w := Standard()
	hidden := 0.0
	shown := 0.0
	for _, f := range Factors {
		if f.Shown() {
			shown += w.of(f)
			continue
		}
		hidden += w.of(f)
	}

	if hidden < shown {
		t.Fatalf("hidden factors carry %v of the composite and revealed ones carry %v — §3.5 wants the hidden half to be at least half", hidden, shown)
	}
}

// §3.5's table, asserted rather than remembered: health and money are revealed
// and the other three are not, and only the hidden three take grants.
func TestOnlyHealthAndMoneyAreEverShown(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		factor Factor
		key    string
		shown  bool
	}{
		{Health, "health", true},
		{Wealth, "money", true},
		{Intelligence, "intelligence", false},
		{Stability, "stability", false},
		{Kindness, "kindness", false},
	} {
		if tc.factor.Key() != tc.key {
			t.Errorf("%v keys as %q, want %q", tc.factor, tc.factor.Key(), tc.key)
		}
		if tc.factor.Shown() != tc.shown {
			t.Errorf("%s.Shown() is %v, want %v", tc.key, tc.factor.Shown(), tc.shown)
		}
		// The two the world already knows are never awarded; the three it does
		// not are the only ones a committed choice can move.
		if tc.factor.Granted() == tc.shown {
			t.Errorf("%s: Granted() and Shown() agree, and they are opposites by construction", tc.key)
		}
		if got := (Grant{Factor: tc.factor, Credit: 10}).Valid(); got == tc.shown {
			t.Errorf("%s: a grant naming it is %v, want %v", tc.key, got, !tc.shown)
		}
	}
}

/*
The composite is a function of its inputs and of nothing else (§3.3).

Read twice, the same standing reads the same, and no call anywhere leaves a mark
that a second call could pick up — which is what "computed on read, never stored
as a stale number" means when it is written down as a test.
*/
func TestKarmaIsComposedFromTheInputsAndNothingElse(t *testing.T) {
	t.Parallel()

	s := Standing{
		Body:   ordinaryBody(),
		Purse:  Dollars(4_000),
		Credit: Credit{Intelligence: 120, Stability: 60, Kindness: 300},
	}

	first := Karma(s)
	second := Karma(s)
	if first != second {
		t.Fatalf("the same standing composed to %+v and then to %+v", first, second)
	}

	// And the standing it was composed from is untouched — nothing in the core
	// mutates its argument.
	if s.Credit != (Credit{Intelligence: 120, Stability: 60, Kindness: 300}) {
		t.Fatalf("composing changed the standing it was handed: %+v", s.Credit)
	}
}

/*
§3.5: the composite "can fall, which keeps it from being a level bar".

Two ways, both of them ordinary. A fortnight of five-hour nights and hard
training is a body that reads older; money spent is money no longer held. Karma
is a reading of what is true now, not a tally of what was ever achieved.
*/
func TestKarmaFalls(t *testing.T) {
	t.Parallel()

	t.Run("when the body is wrecked", func(t *testing.T) {
		before := Standing{Body: ordinaryBody(), Purse: Dollars(1_200)}

		wrecked := before.Body
		for range 14 {
			wrecked = lived(wrecked, Day{Sleep: 5 * time.Hour, Training: Hard, Diet: Poor})
		}
		after := Standing{Body: wrecked, Purse: before.Purse}

		if Karma(after).Karma >= Karma(before).Karma {
			t.Fatalf("a fortnight of no sleep, hard training and bad food did not lower Karma: %v then %v",
				Karma(before).Karma, Karma(after).Karma)
		}
	})

	t.Run("when the money is spent", func(t *testing.T) {
		rich := Standing{Body: ordinaryBody(), Purse: Dollars(80_000)}
		poor := Standing{Body: ordinaryBody(), Purse: Dollars(900)}

		if Karma(poor).Karma >= Karma(rich).Karma {
			t.Fatalf("spending down to $900 did not lower Karma: %v then %v",
				Karma(rich).Karma, Karma(poor).Karma)
		}
	})
}

// The scale is the scale. An empty player is not below the floor and a player
// at the top of everything is exactly at the ceiling.
func TestKarmaIsBoundedByItsOwnScale(t *testing.T) {
	t.Parallel()

	nothing := Karma(Standing{})
	if nothing.Karma < 0 {
		t.Fatalf("a standing with nothing in it reads %v", nothing.Karma)
	}

	// Everything at the top at once: a body far younger than the reference, an
	// unreachable fortune, and enormous credit in all three hidden factors.
	best := ordinaryBody()
	best.Fitness = 70
	best.RestingHR = 40
	best.HRV = 110
	best.SleepDebt = 0
	best.Glucose = 4.2
	best.CRP = 0.3
	best.LDL = 1.5
	best.Cortisol = 200
	best.WeightKg = 68

	top := Karma(Standing{
		Body:   best,
		Purse:  Dollars(500_000_000),
		Credit: Credit{Intelligence: 1e6, Stability: 1e6, Kindness: 1e6},
	})
	if top.Karma > KarmaFull {
		t.Fatalf("the best standing the simulation can produce reads %v, above the scale's %v", top.Karma, KarmaFull)
	}
	if top.Karma <= nothing.Karma {
		t.Fatalf("the best standing (%v) does not beat the empty one (%v)", top.Karma, nothing.Karma)
	}
}

/*
§3.5's money curve, which is the acceptance criterion written as arithmetic.

Three things have to be true and all three are design statements: the curve
rises everywhere below the flip (which is V2), each dollar is worth less than the
one before it, and the fall in what a dollar is worth is sharp at the plateau
rather than gradual — otherwise the threshold is not a threshold.
*/
func TestMoneyRisesWithDiminishingReturnsAboveThePlateau(t *testing.T) {
	t.Parallel()

	// It rises, all the way up. V1 has no downslope (§3.5: the flip is V2).
	last := -1.0
	for _, m := range []Money{
		0, Dollars(1), Dollars(100), Dollars(1_000), Dollars(10_000), Dollars(100_000),
		Dollars(500_000), MoneyPlateau, Dollars(2_000_000), Dollars(10_000_000), Dollars(100_000_000),
	} {
		got := MoneyStanding(m)
		if got <= last {
			t.Fatalf("money standing did not rise at %v: %v after %v", m, got, last)
		}
		if got > 100 {
			t.Fatalf("money standing at %v is %v, above the 0–100 every factor is on", m, got)
		}
		last = got
	}

	// The marginal value of a dollar falls monotonically. Measured as the rise
	// across a fixed $10,000 step, taken at widening balances.
	const step = Money(10_000_00)
	marginal := func(at Money) float64 { return MoneyStanding(at+step) - MoneyStanding(at) }

	previous := math.Inf(1)
	for _, at := range []Money{
		Dollars(0), Dollars(50_000), Dollars(200_000), Dollars(900_000),
		Dollars(2_000_000), Dollars(20_000_000),
	} {
		got := marginal(at)
		if got >= previous {
			t.Fatalf("a $10k step at %v was worth %v, not less than the %v it was worth lower down", at, got, previous)
		}
		previous = got
	}

	// And the plateau is a plateau: the same step is worth a small fraction
	// above the threshold of what it was worth just below it.
	below := marginal(MoneyPlateau - step)
	above := marginal(MoneyPlateau)
	if above/below > 0.3 {
		t.Fatalf("crossing $1M only cost %.0f%% of the return on a dollar — §3.5 asks for diminishing returns above the threshold, not a bend",
			100*(1-above/below))
	}

	// The two halves meet, so a player does not see a step in their composite
	// on the day they cross a million.
	if got := MoneyStanding(MoneyPlateau); math.Abs(got-plateauStanding) > 1e-9 {
		t.Fatalf("the curve is discontinuous at the plateau: %v against %v", got, plateauStanding)
	}
}

// The hidden factors saturate, so no amount of grinding one of them can drown
// the other four — and early credit in an untouched factor is worth the most.
func TestHiddenFactorsSaturate(t *testing.T) {
	t.Parallel()

	first := creditStanding(CreditHalf/10, 0)
	half := creditStanding(CreditHalf, 0)
	huge := creditStanding(CreditHalf*1000, 0)

	if math.Abs(half-50) > 1e-9 {
		t.Fatalf("CreditHalf reads %v, not the halfway point it is named for", half)
	}
	if huge > 100 {
		t.Fatalf("a thousand times the half-point reads %v, above the scale", huge)
	}
	// Concave: the first tenth is worth more than a tenth of the halfway score.
	if first <= 5 {
		t.Fatalf("the first tenth of a factor's credit is worth %v — early credit is meant to be the cheapest", first)
	}

	// A negative total cannot exist, and a total driven at cannot go below the
	// floor (§5.1's options are not all rewards, but they are not a hole).
	c := Credit{Kindness: 10}.Plus(Kindness, -400)
	if c.Kindness != 0 {
		t.Fatalf("a punishing choice drove kindness to %v", c.Kindness)
	}
}

// §3.15: emotional stability and kindness are capped by the variety of
// situations an environment offers — applied to the reading, because the player
// genuinely learned what they learned and what ran out is somewhere to show it.
func TestAnEnvironmentCapsWhatAHiddenFactorCanRead(t *testing.T) {
	t.Parallel()

	credit := Credit{Intelligence: 900, Stability: 900, Kindness: 900}
	open := Karma(Standing{Body: ordinaryBody(), Credit: credit})
	capped := Karma(Standing{
		Body:     ordinaryBody(),
		Credit:   credit,
		Ceilings: Ceilings{Intelligence: 30, Stability: 30, Kindness: 30},
	})

	if capped.Intelligence != 30 {
		t.Fatalf("a ceiling of 30 read as %v", capped.Intelligence)
	}
	if capped.Karma >= open.Karma {
		t.Fatalf("ceilings did not lower the composite: %v against %v", capped.Karma, open.Karma)
	}

	// A zero ceiling is no ceiling, not a ceiling of nothing (§3.15, #29).
	if open.Kindness == 0 {
		t.Fatal("an unauthored ceiling flattened the factor it was never given")
	}
}

/*
§3.15's decay, checked against the shape the design itself draws.

The illustration is a weekly return of 240, 180, 95, 40, 12, 4 — an environment
whose rewards fall away as it is mastered, "same effort, shrinking return". What
matters is not matching those six numbers but reproducing the property they
demonstrate: steady effort in one place pays a small fraction of what it did
within about six weeks, and it never pays nothing.
*/
func TestAnEnvironmentPaysLessTheMoreItIsWorked(t *testing.T) {
	t.Parallel()

	const weekly = 240.0

	worked := 0.0
	days := 0
	var week []float64
	for range 6 {
		week = append(week, Pays(weekly, Familiarity(days, worked)))
		worked += weekly
		days += 7
	}

	for i := 1; i < len(week); i++ {
		if week[i] >= week[i-1] {
			t.Fatalf("week %d paid %v, not less than week %d's %v", i+1, week[i], i, week[i-1])
		}
	}
	if week[0] != weekly {
		t.Fatalf("the first week in a new environment paid %v rather than the whole %v offered", week[0], weekly)
	}
	if week[5] > weekly/10 {
		t.Fatalf("after six weeks of the same effort the environment still pays %v of %v — the pressure to travel is not there", week[5], weekly)
	}
	if week[5] <= 0 {
		t.Fatalf("week six paid %v — §3.15: returns decay but never go negative", week[5])
	}

	// Relocating restores the rate: familiarity is a fact about a place, and a
	// place nobody has worked is worth all of what it offers.
	if got := Familiarity(0, 0); got != 1 {
		t.Fatalf("a brand-new environment pays %v of what it offers", got)
	}

	// Elapsed time alone does not push anybody out. A player who lived
	// somewhere for a season without engaging still has most of it to earn.
	if got := Familiarity(90, 0); got < 0.7 {
		t.Fatalf("ninety days of barely engaging left familiarity at %v — §3.15 wants decay to track mastery, not the calendar", got)
	}

	// And an exhausted environment still pays something.
	if got := Familiarity(10_000, 1e9); got != ReturnFloor {
		t.Fatalf("an environment worked to death pays %v, not the floor %v", got, ReturnFloor)
	}
}

// A cost is not a reward, and does not get cheaper because the place is worn
// out. Otherwise the safest environment in the game would be the exhausted one.
func TestCostsDoNotDecayWithTheEnvironment(t *testing.T) {
	t.Parallel()

	worn := Familiarity(200, 5_000)
	if worn >= 0.2 {
		t.Fatalf("this case wanted a thoroughly worn environment; familiarity is %v", worn)
	}
	if got := Pays(-40, worn); got != -40 {
		t.Fatalf("a cost of 40 was discounted to %v in a worn-out environment", got)
	}
}

// The health factor is the biological age and not a second opinion about the
// body. One statement in the codebase about what a healthy body is (§3.5).
func TestTheHealthFactorIsTheBiologicalAge(t *testing.T) {
	t.Parallel()

	young := ordinaryBody()
	young.Fitness = 58
	young.SleepDebt = 0

	old := ordinaryBody()
	old.Fitness = 24
	old.SleepDebt = 22

	if BiologicalAge(young) >= BiologicalAge(old) {
		t.Fatal("this case wanted the fitter body to read younger")
	}
	if healthStanding(young) <= healthStanding(old) {
		t.Fatalf("the younger body scored %v against the older one's %v", healthStanding(young), healthStanding(old))
	}
	// The reference body sits somewhere worth improving from, at neither end.
	if got := healthStanding(ordinaryBody()); got < 25 || got > 90 {
		t.Fatalf("a new player's health factor reads %v — there has to be room in both directions", got)
	}
}

// The weights are a value, so an alternative weighting is a struct literal
// rather than an edit — which is what makes tuning a test rather than a
// playthrough (§3.1, §17).
func TestTheWeightingIsTunableWithoutTouchingTheComposition(t *testing.T) {
	t.Parallel()

	s := Standing{
		Body:   ordinaryBody(),
		Purse:  Dollars(2_000),
		Credit: Credit{Intelligence: 400, Stability: 400, Kindness: 400},
	}

	onlyMoney := Weights{Money: 1}.Read(s)
	onlyKindness := Weights{Kindness: 1}.Read(s)

	// Same standing, different questions asked of it — and the per-factor
	// scores are the same either way, because a weighting decides how they are
	// mixed and never what they are.
	if onlyMoney.Kindness != onlyKindness.Kindness {
		t.Fatalf("the weighting changed a factor's own score: %v against %v", onlyMoney.Kindness, onlyKindness.Kindness)
	}
	if onlyMoney.Karma == onlyKindness.Karma {
		t.Fatal("two different weightings composed the same standing to the same Karma")
	}
}

/*
§3.15's money row: "capped by that environment's economic ceiling — a Green
setting affords less."

Three properties, and all three are the design rather than the arithmetic.

The cap bites where the design says it does: a player holding more than the
place can use reads as though they held exactly what it can use. It is applied
to the *reading* and not to the balance, so the money is still theirs — which is
what makes relocating restore it rather than refund it. And a place with no
authored ceiling caps nothing, because a zero is the absence of a ceiling and not
a ceiling of zero.
*/
func TestAnEnvironmentCapsWhatMoneyIsWorthAndNotWhatIsHeld(t *testing.T) {
	t.Parallel()

	const ceiling = Money(12_000_00) // $12,000, a place with very little in it
	rich := Money(400_000_00)

	poor := Karma(Standing{Body: ordinaryBody(), Purse: ceiling,
		Ceilings: Ceilings{Money: ceiling}})
	capped := Karma(Standing{Body: ordinaryBody(), Purse: rich,
		Ceilings: Ceilings{Money: ceiling}})
	open := Karma(Standing{Body: ordinaryBody(), Purse: rich})

	if capped.Money != poor.Money {
		t.Fatalf("$12,000 and $400,000 read as %v and %v under the same ceiling", poor.Money, capped.Money)
	}
	if open.Money <= capped.Money {
		t.Fatalf("the ceiling did not lower the reading: %v capped against %v open", capped.Money, open.Money)
	}
	// Somewhere that affords more is worth more of the same money, which is
	// §3.15's "moving: more options" with nothing having been earned.
	richer := Karma(Standing{Body: ordinaryBody(), Purse: rich,
		Ceilings: Ceilings{Money: Money(2_500_000_00)}})
	if richer.Money != open.Money {
		t.Fatalf("a ceiling above the balance changed the reading: %v against %v", richer.Money, open.Money)
	}

	// Nothing was taken. The balance is an input and no reading of it may edit
	// one — asserted here because this is the package that would have to.
	held := Standing{Body: ordinaryBody(), Purse: rich, Ceilings: Ceilings{Money: ceiling}}
	Karma(held)
	if held.Purse != rich {
		t.Fatalf("composing a standing changed the balance to %v", held.Purse)
	}
}

/*
§3.15's intelligence row, which is the one cap that is a fact about the player.

"Capped — plateaus once the rule set works" and "rises as the player rebuilds a
rule set for different people." So the assertions are: one rule set is a real
plateau well short of the top, each further one raises it, the raises get
smaller, and the scale is never reached — there is always somewhere else.

The last case is the load-bearing one. Having built no rule set anywhere must not
read as *no ceiling*, because zero means no ceiling everywhere else in this file
and a player who has been nowhere is the last person who should be uncapped.
*/
func TestIntelligenceIsCappedByHowManyRuleSetsHaveBeenBuilt(t *testing.T) {
	t.Parallel()

	one, two, three, four := Reach(1), Reach(2), Reach(3), Reach(4)

	if one >= 60 {
		t.Fatalf("one rule set reaches %v, which is not a plateau anybody would leave", one)
	}
	if !(one < two && two < three && three < four) {
		t.Fatalf("rebuilding did not raise the ceiling: %v, %v, %v, %v", one, two, three, four)
	}
	if two-one <= three-two {
		t.Fatalf("the second set paid %v and the third %v; the returns do not fall", two-one, three-two)
	}
	if Reach(40) >= 100 {
		t.Fatalf("forty rule sets reach %v, and the scale is meant to be unreachable", Reach(40))
	}
	if Reach(0) != one || Reach(-3) != one {
		t.Fatalf("having built nothing reads as %v/%v rather than as one place", Reach(0), Reach(-3))
	}

	// And the consequence, which is the whole mechanic: grinding one population
	// stops moving the factor, and arriving somewhere new moves it before a
	// single new choice has been made.
	stuck := Ceilings{Intelligence: Reach(1)}
	moved := Ceilings{Intelligence: Reach(2)}
	worked := Karma(Standing{Body: ordinaryBody(), Credit: Credit{Intelligence: 600}, Ceilings: stuck})
	ground := Karma(Standing{Body: ordinaryBody(), Credit: Credit{Intelligence: 6000}, Ceilings: stuck})
	elsewhere := Karma(Standing{Body: ordinaryBody(), Credit: Credit{Intelligence: 600}, Ceilings: moved})

	if ground.Intelligence != worked.Intelligence {
		t.Fatalf("ten times the credit in one place read as %v against %v", ground.Intelligence, worked.Intelligence)
	}
	if elsewhere.Intelligence <= worked.Intelligence {
		t.Fatalf("a second rule set did not raise the reading: %v against %v",
			elsewhere.Intelligence, worked.Intelligence)
	}
}
