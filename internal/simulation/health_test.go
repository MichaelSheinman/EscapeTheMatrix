package simulation

import (
	"math"
	"testing"
	"time"
)

/*
The rules, tested the way §14 says the core is tested: table-driven, no
database, milliseconds. These are the tests §17's tuning loop runs — the
biomarker formulas are explicitly listed as something to settle during
implementation, and settling them means changing a constant and running this
file, not playing the game.

The clock is a local type rather than internal/clock, so that this package's
tests import nothing outside it either.
*/

type at time.Time

func (a at) Now() time.Time { return time.Time(a) }

// noon is an arbitrary fixed instant. Which instant it is never matters, which
// is the point of injecting it.
var noon = at(time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC))

// person is the body every test starts from unless it says otherwise: 1.78m,
// 78kg, ordinary in every way.
func person() Body { return Starting(1.78, 78) }

// live runs n identical days, which is how every trend in here is produced.
func live(b Body, d Day, n int) (Body, Effects) {
	var fx Effects
	for range n {
		var day Effects
		b, day = Advance(noon, b, d)
		fx = append(fx, day...)
	}
	return b, fx
}

func hours(h float64) time.Duration { return time.Duration(h * float64(time.Hour)) }

func TestADayOfLivingMovesEachMetricTheRightWay(t *testing.T) {
	t.Parallel()

	// Fourteen days of one kind of life, against fourteen of another. A day is
	// deliberately not enough to show most of these, which is itself the point.
	good := Day{Sleep: hours(8.5), Training: Hard, Diet: Clean}
	bad := Day{Sleep: hours(5), Training: Rest, Diet: Poor}

	cases := []struct {
		name   string
		metric func(Body) float64
		// higher says which direction is the healthy one, so the table reads
		// as "this life is better for this metric" rather than as arithmetic.
		higherIsBetter bool
	}{
		{"HRV", func(b Body) float64 { return b.HRV }, true},
		{"recovery", func(b Body) float64 { return b.Recovery }, true},
		{"aerobic capacity", func(b Body) float64 { return b.Fitness }, true},
		{"resting heart rate", func(b Body) float64 { return b.RestingHR }, false},
		{"fasting glucose", func(b Body) float64 { return b.Glucose }, false},
		{"inflammation", func(b Body) float64 { return b.CRP }, false},
		{"LDL", func(b Body) float64 { return b.LDL }, false},
		{"cortisol", func(b Body) float64 { return b.Cortisol }, false},
		{"sleep debt", func(b Body) float64 { return b.SleepDebt }, false},
	}

	kept, _ := live(person(), good, 14)
	wrecked, _ := live(person(), bad, 14)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			healthy, unhealthy := c.metric(kept), c.metric(wrecked)
			if !c.higherIsBetter {
				healthy, unhealthy = -healthy, -unhealthy
			}
			if healthy <= unhealthy {
				t.Errorf("%s after a fortnight: looked-after = %.2f, neglected = %.2f — the wrong way round",
					c.name, c.metric(kept), c.metric(wrecked))
			}
		})
	}
}

// The asymmetry that makes the health system worth learning: an hour lost costs
// more than an hour recovered gives back, so a bad week is not undone by a
// weekend.
func TestSleepDebtAccruesFasterThanItIsRepaid(t *testing.T) {
	t.Parallel()

	body := person()
	body.SleepDebt = 0

	short, _ := Advance(noon, body, Day{Sleep: hours(6)})
	long, _ := Advance(noon, short, Day{Sleep: hours(10)})

	if short.SleepDebt != 2 {
		t.Errorf("a six-hour night left %.2f hours of debt, want 2", short.SleepDebt)
	}
	if long.SleepDebt <= 0 {
		t.Errorf("a ten-hour night cleared %.2f hours of debt — it should not clear two", 2-long.SleepDebt)
	}
}

// Training on a wrecked body produces the stress without the adaptation, which
// is the mechanism behind overtraining being a recovery problem.
func TestTrainingOnDeepSleepDebtBuysLessFitness(t *testing.T) {
	t.Parallel()

	rested, wrecked := person(), person()
	rested.SleepDebt = 0
	wrecked.SleepDebt = 20

	hard := Day{Sleep: hours(8), Training: Hard, Diet: Mixed}
	rested, _ = Advance(noon, rested, hard)
	wrecked, _ = Advance(noon, wrecked, hard)

	gainRested := rested.Fitness - 34
	gainWrecked := wrecked.Fitness - 34
	if gainWrecked >= gainRested {
		t.Errorf("fitness gained: rested = %.4f, wrecked = %.4f — the wrecked body should adapt less",
			gainRested, gainWrecked)
	}
}

// A fortnight of neglect, then one perfect night. The body is better, and
// nowhere near recovered — the lesson the panel exists to teach.
func TestOneGoodNightDoesNotUndoABadFortnight(t *testing.T) {
	t.Parallel()

	wrecked, _ := live(person(), Day{Sleep: hours(5), Diet: Poor}, 14)
	after, _ := Advance(noon, wrecked, Day{Sleep: hours(9), Diet: Clean})

	if after.Recovery <= wrecked.Recovery {
		t.Errorf("recovery did not improve at all: %.1f → %.1f", wrecked.Recovery, after.Recovery)
	}
	if after.Recovery > 60 {
		t.Errorf("recovery = %.1f after one good night on a fortnight of debt — too forgiving", after.Recovery)
	}
	if after.SleepDebt < 8 {
		t.Errorf("sleep debt = %.1f after one long night — a fortnight cannot be repaid overnight", after.SleepDebt)
	}
}

func TestBiologicalAgeIsComposedFromThePanel(t *testing.T) {
	t.Parallel()

	// Each row changes exactly one marker away from the reference body, so the
	// table reads as a statement about that marker's contribution.
	cases := []struct {
		name  string
		worse func(*Body)
	}{
		{"a lower VO2 max", func(b *Body) { b.Fitness -= 10 }},
		{"a higher resting heart rate", func(b *Body) { b.RestingHR += 12 }},
		{"a collapsed HRV", func(b *Body) { b.HRV -= 15 }},
		{"more body mass", func(b *Body) { b.WeightKg += 18 }},
		{"higher fasting glucose", func(b *Body) { b.Glucose += 1.2 }},
		{"more inflammation", func(b *Body) { b.CRP += 3 }},
		{"higher LDL", func(b *Body) { b.LDL += 1.5 }},
		{"more cortisol", func(b *Body) { b.Cortisol += 120 }},
		{"accumulated sleep debt", func(b *Body) { b.SleepDebt += 15 }},
	}

	base := BiologicalAge(person())
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := person()
			c.worse(&body)
			if got := BiologicalAge(body); got <= base {
				t.Errorf("biological age with %s = %.1f, want more than %.1f", c.name, got, base)
			}
		})
	}

	// And the whole point of the number: it moves the way a life does.
	kept, _ := live(person(), Day{Sleep: hours(8.5), Training: Hard, Diet: Clean}, 90)
	wrecked, _ := live(person(), Day{Sleep: hours(5), Training: Rest, Diet: Poor}, 90)
	if BiologicalAge(kept) >= BiologicalAge(wrecked) {
		t.Errorf("after three months: looked-after reads %.1f, neglected reads %.1f",
			BiologicalAge(kept), BiologicalAge(wrecked))
	}
	if BiologicalAge(kept) >= base {
		t.Errorf("three months of looking after yourself did not lower biological age: %.1f → %.1f",
			base, BiologicalAge(kept))
	}
}

// Nothing here is random and nothing here reads a clock, so a hundred days can
// only come out one way. This is the assertion behind "a full playthrough is
// reproducible from a seed and a fixed clock" (§14).
func TestTheSameInputProducesTheSameOutput(t *testing.T) {
	t.Parallel()

	week := []Day{
		{Sleep: hours(7.5), Training: Hard, Diet: Clean},
		{Sleep: hours(5), Training: Rest, Diet: Poor},
		{Sleep: hours(8), Training: Light, Diet: Mixed},
		{Sleep: hours(6.25), Training: Hard, Diet: Poor},
		{Sleep: hours(9), Training: Rest, Diet: Clean},
		{Sleep: hours(4), Training: Hard, Diet: Mixed},
		{Sleep: hours(8.5), Training: Light, Diet: Clean},
	}

	run := func() (Body, int) {
		body, count := person(), 0
		for i := range 100 {
			var fx Effects
			body, fx = Advance(noon, body, week[i%len(week)])
			count += len(fx)
		}
		return body, count
	}

	first, firstEffects := run()
	second, secondEffects := run()

	if first != second {
		t.Errorf("a hundred identical days produced two different bodies:\n %+v\n %+v", first, second)
	}
	if firstEffects != secondEffects {
		t.Errorf("effects: %d then %d", firstEffects, secondEffects)
	}
}

// State in, new state out — never mutation in place (§3.1). A caller holding
// yesterday's body still holds yesterday's body.
func TestAdvanceDoesNotTouchTheBodyItWasGiven(t *testing.T) {
	t.Parallel()

	body := person()
	before := body

	Advance(noon, body, Day{Sleep: hours(4), Training: Hard, Diet: Poor})

	if body != before {
		t.Errorf("the argument was modified:\n %+v\n %+v", before, body)
	}
}

// The one place the injected clock is visible from outside: a day replayed for
// last Tuesday is stamped last Tuesday, not now.
func TestADayIsStampedByTheClockItWasGiven(t *testing.T) {
	t.Parallel()

	tuesday := at(time.Date(2026, 7, 28, 3, 30, 0, 0, time.UTC))
	_, fx := Advance(tuesday, person(), Day{Sleep: hours(8)})

	if len(fx) == 0 {
		t.Fatal("a day of living produced no effects at all")
	}
	sample, ok := fx[0].(Sampled)
	if !ok {
		t.Fatalf("first effect is %T, want a Sampled", fx[0])
	}
	if !sample.At.Equal(time.Time(tuesday)) {
		t.Errorf("sample stamped %s, want %s", sample.At, time.Time(tuesday))
	}
}

// A crossing is how the simulation says something happened without narrating
// it — the raw material of an event release (§4) and of diagnosis evidence.
func TestCollapsingRecoveryIsReportedAsACrossing(t *testing.T) {
	t.Parallel()

	body := person()
	body.SleepDebt = 0
	body.HRV = 60
	body.Recovery = 90

	var crossings []Crossed
	for range 10 {
		var fx Effects
		body, fx = Advance(noon, body, Day{Sleep: hours(4), Training: Hard, Diet: Poor})
		for _, e := range fx {
			if c, ok := e.(Crossed); ok {
				crossings = append(crossings, c)
			}
		}
	}

	var fell bool
	for _, c := range crossings {
		if c.Metric == "recovery" && c.To < c.From {
			fell = true
		}
	}
	if !fell {
		t.Errorf("ten wrecked days reported no fall in recovery; crossings = %+v", crossings)
	}

	// And a day where nothing moves across a threshold says nothing, which is
	// what keeps a crossing worth listening to.
	steady := person()
	steady, fx := Advance(noon, steady, Day{Sleep: hours(8), Diet: Mixed})
	_ = steady
	for _, e := range fx {
		if c, ok := e.(Crossed); ok {
			t.Errorf("an unremarkable day reported %+v", c)
		}
	}
}

// Every value stays inside the range its unit makes sense in, whatever is done
// to it. A biomarker that can go negative is a biomarker that will, three
// tuning passes from now.
func TestNoMetricCanLeaveItsPhysicalRange(t *testing.T) {
	t.Parallel()

	extremes := []Day{
		{Sleep: 0, Training: Hard, Diet: Poor},
		{Sleep: 24 * time.Hour, Training: Rest, Diet: Clean},
	}

	for _, day := range extremes {
		body, _ := live(person(), day, 400)
		checks := []struct {
			name     string
			v        float64
			lo, hi   float64
			describe string
		}{
			{"HRV", body.HRV, 5, 200, "ms"},
			{"recovery", body.Recovery, 0, 100, "score"},
			{"resting heart rate", body.RestingHR, 30, 130, "bpm"},
			{"fitness", body.Fitness, 15, 80, "ml/kg/min"},
			{"weight", body.WeightKg, 35, 250, "kg"},
			{"glucose", body.Glucose, 3, 15, "mmol/L"},
			{"CRP", body.CRP, 0, 20, "mg/L"},
			{"LDL", body.LDL, 0.5, 8, "mmol/L"},
			{"cortisol", body.Cortisol, 100, 900, "nmol/L"},
			{"sleep debt", body.SleepDebt, 0, debtCeiling, "hours"},
			{"biological age", BiologicalAge(body), 18, 95, "years"},
		}
		for _, c := range checks {
			if math.IsNaN(c.v) || c.v < c.lo || c.v > c.hi {
				t.Errorf("after 400 days of %v, %s = %.2f %s, outside [%.0f, %.0f]",
					day, c.name, c.v, c.describe, c.lo, c.hi)
			}
		}
	}
}
