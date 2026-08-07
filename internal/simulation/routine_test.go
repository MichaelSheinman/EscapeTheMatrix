package simulation

import (
	"testing"
	"time"
)

/*
Routines, tested where they are decidable: no database, no calendar of the
machine's, and nothing random to average out.

Two of the assertions below are the load-bearing ones. `TestAProjectionIsExactly
TheDaysItPredicts` is what makes a forecast honest — a projection the game shows
and a fortnight the game later applies have to be the same arithmetic, or the
routine screen is telling a story the body does not follow. And
`TestOverOrganisingCostsMoreThanItBuys` is §3.14's rule about over-general
strategies made checkable: the vocabulary must not contain an option that simply
wins.
*/

// a monday, so a test that cares about the shape of the week can say which days
// it means. 2026-08-03 is one.
var monday = Date{Year: 2026, Month: time.August, Day: 3}

// The four routines a test picks between, named for what they are rather than
// for their keys — the keys live in internal/routine, which this package does
// not know about.
var (
	// drift is what an unconfigured life does: late nights, no training,
	// whatever food is in front of you, nothing written down.
	drift = Routine{
		Sleep:        driftSleep,
		Training:     [7]Training{},
		Diet:         Poor,
		Organization: Loose,
	}
	// split is the ambitious one: eight and a half hours, three hard sessions
	// and three easy ones, cooked food.
	split = Routine{
		Sleep: 8*time.Hour + 30*time.Minute,
		Training: [7]Training{
			time.Sunday: Rest, time.Monday: Hard, time.Tuesday: Light,
			time.Wednesday: Hard, time.Thursday: Light, time.Friday: Hard,
			time.Saturday: Light,
		},
		Diet:         Clean,
		Organization: Listed,
	}
)

// run walks a routine forward from a date and hands back the body it left.
func run(b Body, r Routine, from Date, days int) Body {
	days_ := Project(b, r, from, days)
	if len(days_) == 0 {
		return b
	}
	return days_[len(days_)-1].Body
}

func TestTheTrainingWeekHasAShape(t *testing.T) {
	t.Parallel()

	// Perfectly kept, so what comes out is the plan itself rather than the plan
	// after slippage — the shape is what is being asserted here.
	plan := split
	plan.Organization = Unsaid

	want := []Training{Hard, Light, Hard, Light, Hard, Light, Rest}
	on := monday
	for i, expect := range want {
		if got := plan.On(on).Training; got != expect {
			t.Errorf("day %d (%v) trained %v, want %v", i, on.weekday(), got, expect)
		}
		on = on.Next()
	}
}

// The same routine over the same dates is the same living, forever. Without
// this a forecast is a guess and a replayed rollover is a different life.
func TestARoutineProducesTheSameDaysForever(t *testing.T) {
	t.Parallel()

	on := monday
	for range 60 {
		first, second := split.On(on), split.On(on)
		if first != second {
			t.Fatalf("%v produced %+v and then %+v", on, first, second)
		}
		on = on.Next()
	}
}

// Slippage is spread rather than clustered: a routine kept four days in five
// misses one day a week, not a whole week in five.
func TestSlippageIsSpreadAcrossTheDaysRatherThanClustered(t *testing.T) {
	t.Parallel()

	plan := split
	plan.Organization = Listed // 0.80 — four days in five

	on := monday
	var missed, run int
	for range 20 {
		if plan.On(on).Training != plan.Training[on.weekday()] {
			missed++
			run++
			if run > 1 {
				t.Errorf("two slipped days in a row at %v — the spread is clustering", on)
			}
		} else {
			run = 0
		}
		on = on.Next()
	}
	if missed < 3 || missed > 5 {
		t.Errorf("%d of 20 days slipped at 0.8 adherence, want about 4", missed)
	}
}

// The organization routine's whole reason for existing: it decides how much of
// the other three actually happens.
func TestOrganizationDecidesHowMuchOfARoutineHappens(t *testing.T) {
	t.Parallel()

	slept := func(o Order) float64 {
		plan := split
		plan.Organization = o
		var total float64
		on := monday
		for range 28 {
			total += plan.On(on).Sleep.Hours()
			on = on.Next()
		}
		return total / 28
	}

	loose, listed, regimented := slept(Loose), slept(Listed), slept(Regimented)
	if !(loose < listed && listed < regimented) {
		t.Errorf("average night slept: loose = %.2f, listed = %.2f, regimented = %.2f — better arrangement must keep more of the routine",
			loose, listed, regimented)
	}
	if regimented-listed > listed-loose {
		t.Errorf("the step from listed to regimented (%.2fh) is not smaller than the step from loose to listed (%.2fh) — the returns are meant to diminish",
			regimented-listed, listed-loose)
	}
}

// The lesson the mechanic is for: organization matters in proportion to what
// you were trying to do. A life that intends nothing loses nothing by being
// disorganised.
func TestAmbitionIsWhatMakesOrganizationMatter(t *testing.T) {
	t.Parallel()

	spread := func(r Routine) float64 {
		loose, regimented := r, r
		loose.Organization = Loose
		regimented.Organization = Regimented
		return run(person(), regimented, monday, 60).Fitness - run(person(), loose, monday, 60).Fitness
	}

	modest, ambitious := spread(drift), spread(split)
	if ambitious <= modest {
		t.Errorf("organization is worth %.3f VO2 max on an ambitious routine and %.3f on an idle one — it should matter more the more you intended",
			ambitious, modest)
	}
}

/*
Assigning every hour of the week is not free, and the vocabulary must not
contain an option that simply wins (§3.14).

The last tenth of adherence is bought with the stress the arrangement itself is,
and the trade lands where a trade should: three months of a regimented week
reads a hair younger and recovers measurably worse than three months of a
written-down one. Neither answer dominates, which is what makes it a question.
*/
func TestARegimentedWeekIsPaidForInRecovery(t *testing.T) {
	t.Parallel()

	listed, regimented := split, split
	listed.Organization = Listed
	regimented.Organization = Regimented

	a := run(person(), listed, monday, 90)
	b := run(person(), regimented, monday, 90)

	if b.Cortisol <= a.Cortisol {
		t.Errorf("cortisol after three months: listed = %.0f, regimented = %.0f — a life scheduled to the hour is its own load",
			a.Cortisol, b.Cortisol)
	}
	if b.HRV >= a.HRV {
		t.Errorf("HRV after three months: listed = %.1f, regimented = %.1f — the same stress read from the other side",
			a.HRV, b.HRV)
	}
	if b.Recovery >= a.Recovery {
		t.Errorf("recovery after three months: listed = %.0f, regimented = %.0f — the number the player watches must show the cost",
			a.Recovery, b.Recovery)
	}
}

/*
Organization is an amplifier, not a virtue.

It decides how much of a routine happens, and says nothing about whether the
routine was any good — so a punishing one kept faithfully is *worse* than the
same one you keep failing to follow. Training hard on a body carrying a
fortnight of sleep debt produces the stress without the adaptation (health.go
says so in the fitness line), and organization is what makes sure it happens.

This is the failure a player is meant to be able to diagnose: the routine was
wrong, and getting better at keeping it made it worse.
*/
func TestOrganizationAmplifiesWhicheverRoutineItKeeps(t *testing.T) {
	t.Parallel()

	// Training taken seriously, on top of no sleep routine and no food routine.
	punishing := split
	punishing.Sleep = driftSleep
	punishing.Diet = Poor

	loose, regimented := punishing, punishing
	loose.Organization = Loose
	regimented.Organization = Regimented

	a := run(person(), loose, monday, 90)
	b := run(person(), regimented, monday, 90)

	if BiologicalAge(b) <= BiologicalAge(a) {
		t.Errorf("biological age after three months of a bad routine: loosely kept = %.2f, kept faithfully = %.2f — keeping a wrong rule better has to cost more",
			BiologicalAge(a), BiologicalAge(b))
	}
	if b.Cortisol <= a.Cortisol {
		t.Errorf("cortisol: loosely kept = %.0f, kept faithfully = %.0f", a.Cortisol, b.Cortisol)
	}
}

// A life nobody arranged carries a cost of its own, so the trade is a middle
// rather than a floor.
func TestBeingDisorganisedIsItsOwnStress(t *testing.T) {
	t.Parallel()

	// The same days either way, so what is left is the arrangement alone.
	day := Day{Sleep: hours(7), Training: Light, Diet: Mixed}
	loose, _ := live(person(), Day{Sleep: day.Sleep, Training: day.Training, Diet: day.Diet, Order: Loose}, 60)
	listed, _ := live(person(), Day{Sleep: day.Sleep, Training: day.Training, Diet: day.Diet, Order: Listed}, 60)

	if loose.Cortisol <= listed.Cortisol {
		t.Errorf("cortisol: loose = %.0f, listed = %.0f — keeping it all in your head is a stressor",
			loose.Cortisol, listed.Cortisol)
	}
}

// A day the player narrated by hand says nothing about arrangement, and must
// therefore be charged nothing for it.
func TestADayNarratedByHandIsChargedNoArrangement(t *testing.T) {
	t.Parallel()

	said := Day{Sleep: hours(7), Training: Light, Diet: Mixed}
	planned := said
	planned.Order = Listed

	if lived(person(), said) != lived(person(), planned) {
		t.Error("a day with no Order behaved differently from a planned one — the zero value must be free")
	}
}

// A routine visibly moves the panel over the days that follow, which is the
// whole of what §3.4 asks the early game to teach.
func TestKeepingARoutineMovesThePanel(t *testing.T) {
	t.Parallel()

	kept := run(person(), split, monday, 60)
	unarranged := run(person(), drift, monday, 60)

	if BiologicalAge(kept) >= BiologicalAge(unarranged) {
		t.Errorf("after two months: routine reads %.1f, drift reads %.1f",
			BiologicalAge(kept), BiologicalAge(unarranged))
	}
	if kept.SleepDebt >= unarranged.SleepDebt {
		t.Errorf("sleep debt: routine = %.1f, drift = %.1f", kept.SleepDebt, unarranged.SleepDebt)
	}
	if kept.Fitness <= unarranged.Fitness {
		t.Errorf("aerobic capacity: routine = %.1f, drift = %.1f", kept.Fitness, unarranged.Fitness)
	}
}

/*
The forecast is not a model of its own.

A projection is shown the moment a routine is configured, and the same routine
is applied for real one day at a time afterwards. If the two were different code
the screen would eventually be lying, and it would be lying about the one thing
the early game is for. So they are the same code, and this is the assertion that
keeps them that way.
*/
func TestAProjectionIsExactlyTheDaysItPredicts(t *testing.T) {
	t.Parallel()

	const days = 45
	forecast := Project(person(), split, monday, days)
	if len(forecast) != days {
		t.Fatalf("projected %d days, want %d", len(forecast), days)
	}

	body := person()
	on := monday
	for i, want := range forecast {
		if want.On != on {
			t.Fatalf("day %d is dated %v, want %v", i, want.On, on)
		}
		var fx Effects
		body, fx = Advance(noon, body, split.On(on))
		if len(fx) == 0 {
			t.Fatalf("day %d produced no effects", i)
		}
		if body != want.Body {
			t.Fatalf("day %d: lived %+v, projected %+v", i, body, want.Body)
		}
		on = on.Next()
	}
}

func TestAProjectionOfNothingIsNothing(t *testing.T) {
	t.Parallel()

	if got := Project(person(), split, monday, 0); got != nil {
		t.Errorf("Project(…, 0) = %v, want nil", got)
	}
	if got := Project(person(), split, monday, -3); got != nil {
		t.Errorf("Project(…, -3) = %v, want nil", got)
	}
}

// The calendar arithmetic a rollover walks with, across the boundaries that
// break naive versions of it.
func TestTheDayAfterCrossesEveryBoundary(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		from, want Date
	}{
		{"an ordinary day", Date{2026, time.August, 6}, Date{2026, time.August, 7}},
		{"the end of a month", Date{2026, time.August, 31}, Date{2026, time.September, 1}},
		{"the end of a year", Date{2026, time.December, 31}, Date{2027, time.January, 1}},
		{"a leap day", Date{2028, time.February, 28}, Date{2028, time.February, 29}},
		{"the day after a leap day", Date{2028, time.February, 29}, Date{2028, time.March, 1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.from.Next(); got != c.want {
				t.Errorf("%v.Next() = %v, want %v", c.from, got, c.want)
			}
			if a, b := c.from.ordinal(), c.want.ordinal(); b != a+1 {
				t.Errorf("ordinals %d then %d — consecutive days must be consecutive numbers", a, b)
			}
		})
	}
}

// The rest of the calendar the rollover walks with: how far apart two days are,
// which of them came first, and where a jump lands.
//
// Together these are what bounds a catch-up. A player who has been away for a
// season is caught up from a window, and the window's first day is Plus on a
// negative number — arithmetic that has to survive month, year and leap-day
// boundaries in both directions.
func TestTheCalendarCountsAndComparesDays(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		from Date
		days int
		want Date
	}{
		{"forward within a month", Date{2026, time.August, 6}, 5, Date{2026, time.August, 11}},
		{"back across a month", Date{2026, time.September, 3}, -5, Date{2026, time.August, 29}},
		{"back across a year", Date{2027, time.January, 2}, -3, Date{2026, time.December, 30}},
		{"across a leap day", Date{2028, time.February, 27}, 3, Date{2028, time.March, 1}},
		{"a season back", Date{2026, time.August, 6}, -90, Date{2026, time.May, 8}},
		{"nowhere", Date{2026, time.August, 6}, 0, Date{2026, time.August, 6}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.from.Plus(c.days)
			if got != c.want {
				t.Fatalf("%v.Plus(%d) = %v, want %v", c.from, c.days, got, c.want)
			}
			// Until is Plus read backwards, and the two have to agree or a
			// catch-up would walk a different number of days than it counted.
			if n := c.from.Until(got); n != c.days {
				t.Errorf("%v.Until(%v) = %d, want %d", c.from, got, n, c.days)
			}
			switch {
			case c.days > 0 && !c.from.Before(got):
				t.Errorf("%v is not before %v", c.from, got)
			case c.days < 0 && !got.Before(c.from):
				t.Errorf("%v is not before %v", got, c.from)
			case c.days == 0 && (c.from.Before(got) || got.Before(c.from)):
				t.Errorf("%v is not the same day as itself", c.from)
			}
		})
	}
}
