package simulation

import (
	"math"
	"time"
)

/*
The body, simulated in full.

HighLevelOverview.md §3.16: two layers — a few visible drivers, and a deep
simulated panel revealed by instrumentation. **This file is the deep panel, and
it does not know instrumentation exists.** Buying a watch does not make anyone
healthier; it makes them able to see. So every metric here is always simulated,
for every player, and what they can see of it is a separate concern living in
internal/health.

That separation is the game's whole thesis in one package boundary. A player
without a wearable still suffers the real HRV collapse — they simply cannot see
the cause, which *is* the diagnosis problem (§3.6).

# Why these metrics

Every one is a real-world concept with a real relationship behind it, because
§3.16 requires them to be teachable and tunable against reality rather than
invented dials:

  - Short sleep accrues a debt that does not clear overnight, and the debt
    suppresses heart-rate variability and raises resting heart rate.
  - HRV is the autonomic nervous system's balance sheet: it falls under
    accumulated sleep loss, training load and inflammation, and recovers slowly.
  - VO2 max rises with training and decays measurably within weeks without it —
    the strongest single predictor of all-cause mortality in the literature, and
    so the heaviest term in biological age below.
  - One short night measurably reduces insulin sensitivity the next morning,
    which is why fasting glucose here answers to sleep debt and not only to diet.
  - CRP is the inflammation marker that responds to poor diet, chronic sleep
    loss and unrecovered training load together.
  - Morning cortisol rises with sleep debt and with hard training taken without
    recovery, which is the mechanism behind "overtraining" being a stress
    problem rather than a muscle problem.

The numbers are a first tuning pass, not a claim of clinical accuracy. §17 lists
"biomarker formulas" as something to settle during implementation, and this file
is the thing that gets settled — which is exactly why it is pure and its tests
run in milliseconds.
*/

// Body is the full biomarker panel plus the hidden state driving it.
//
// One value per metric, all current state: history is a per-domain concern
// (§3.2's enumerated list, where the biomarker time series is the first entry)
// and biological age is derived on read rather than stored.
type Body struct {
	// ── the drivers, which no instrumentation shows directly ──────────

	// SleepDebt is hours owed. It accumulates on short nights and is repaid
	// slowly, which is what makes a bad week take more than one good night to
	// undo.
	SleepDebt float64
	// Fitness is VO2 max in ml/kg/min — aerobic capacity. Also a visible
	// metric on the wearable tier, and the heaviest term in biological age.
	Fitness float64
	// HeightM is a fact about the character rather than a metric. It never
	// changes here; it is carried so body composition can be read off weight.
	HeightM float64

	// ── what anyone can see, instrumented or not ──────────────────────

	// LastSleep is hours slept on the night just ended.
	LastSleep float64
	// RestingHR is beats per minute, measured at rest.
	RestingHR float64
	// WeightKg is body mass.
	WeightKg float64

	// ── what a wearable shows ─────────────────────────────────────────

	// HRV is heart-rate variability in milliseconds (RMSSD).
	HRV float64
	// Recovery is the 0–100 readiness score a wearable reports: this
	// morning's HRV against this body's own baseline, penalised by debt.
	Recovery float64
	// Load is a rolling training load, decaying with roughly a week's
	// half-life. High load is not bad in itself; high load with no recovery is.
	Load float64

	// ── what bloodwork shows ──────────────────────────────────────────

	// Glucose is fasting blood glucose in mmol/L.
	Glucose float64
	// CRP is C-reactive protein in mg/L — systemic inflammation.
	CRP float64
	// LDL is low-density lipoprotein cholesterol in mmol/L.
	LDL float64
	// Cortisol is morning serum cortisol in nmol/L — the stress axis.
	Cortisol float64
}

// Training is how hard a day was trained. A closed vocabulary rather than a
// number, because it is what a routine (§3.4) will eventually produce and
// nothing in this game takes a free field.
type Training int

const (
	Rest Training = iota
	Light
	Hard
)

// Diet is how a day was eaten. Same reasoning as Training.
type Diet int

const (
	Poor Diet = iota
	Mixed
	Clean
)

// Day is everything a day of living does to a body.
//
// It is deliberately thin: a handful of choices, each of which a player makes by
// arranging their life rather than by typing a number. Routines (§3.4) produce
// these automatically once configured, and the daily rollover (§3.3) applies
// one per player per local day.
type Day struct {
	Sleep    time.Duration
	Training Training
	Diet     Diet
	// Order is how the day was arranged — the organization routine's channel
	// into the body (routine.go). Its zero value says nothing was arranged
	// either way and costs nothing, so a day narrated by hand carries no
	// opinion about a routine it was not produced by.
	Order Order
}

// SleepNeed is the nightly requirement everything else is measured against.
// Eight hours is the middle of the adult range and the number the debt is
// accrued from.
const SleepNeed = 8 * time.Hour

// debtCeiling bounds accumulated sleep debt at about four bad nights' worth.
//
// Not because a body cannot owe more, but because past that point the
// physiological consequences saturate — and an unbounded driver eventually
// dominates every other term in a way no amount of tuning can fix.
const debtCeiling = 30

// Starting is the body a new character begins with: an ordinary, unremarkable,
// slightly under-slept adult. Every number is a reference value rather than an
// ideal one, so a new player has somewhere to improve from and something to
// lose.
func Starting(heightM, weightKg float64) Body {
	b := Body{
		SleepDebt: 4,
		Fitness:   34,
		HeightM:   heightM,
		LastSleep: 7,
		RestingHR: 66,
		WeightKg:  weightKg,
		HRV:       44,
		Load:      18,
		Glucose:   5.1,
		CRP:       1.2,
		LDL:       2.9,
		Cortisol:  340,
	}
	// Read rather than stated, so a new body's score cannot disagree with the
	// panel it is a reading of. Stating it would make the player's very first
	// day report a change that never happened.
	b.Recovery = recovery(b)
	return b
}

/*
Advance lives one day and returns the body it left behind.

The clock is a parameter rather than a package-level call because that is the
whole rule of §3.1 — and because the effects returned carry the instant they
happened at, which a caller replaying a missed day needs to be able to set to
that day rather than to now.

Everything below moves *toward* a target rather than jumping to it. That is not
smoothing for its own sake: bodies have inertia, and it is what makes a single
good night visibly insufficient to undo a bad fortnight, which is the lesson the
health system exists to teach.
*/
func Advance(c Clock, b Body, d Day) (Body, Effects) {
	at := c.Now()
	before := b
	b = lived(b, d)

	fx := Effects{Sampled{At: at, Body: b}}
	fx = fx.with(crossings(at, before, b)...)
	return b, fx
}

// lived is the day itself, with no clock anywhere near it.
//
// Advance is this plus the effects, which are the only part that needs to know
// *when*. Project (routine.go) runs it a fortnight forward without an instant to
// stamp anything with, and the two have to be the same arithmetic or a forecast
// is a second model of the body that drifts from the first one.
func lived(b Body, d Day) Body {
	slept := d.Sleep.Hours()
	b.LastSleep = slept

	// Debt accrues on a short night and is repaid on a long one, but repayment
	// is slower than accrual: an hour lost costs more than an hour recovered
	// gives back, which is the actual asymmetry in the sleep literature and the
	// reason a weekend lie-in does not clear a week of five-hour nights.
	deficit := SleepNeed.Hours() - slept
	if deficit > 0 {
		b.SleepDebt = clamp(b.SleepDebt+deficit, 0, debtCeiling)
	} else {
		b.SleepDebt = clamp(b.SleepDebt+deficit*0.6, 0, debtCeiling)
	}

	// Training load, decaying with about a week's half-life. Today's session
	// lands on top of what is left of the last few.
	b.Load = clamp(b.Load*0.86+trainingLoad(d.Training), 0, 100)

	// Aerobic capacity: earned slowly, lost slowly, and not earned at all by a
	// body too wrecked to adapt — training on top of deep sleep debt produces
	// the stress without the adaptation, which is the mechanism this line is
	// here to teach.
	adapt := 1.0
	if b.SleepDebt > 12 {
		adapt = 0.35
	}
	b.Fitness = clamp(b.Fitness+trainingStimulus(d.Training)*adapt-0.035, 18, 70)

	bmi := b.WeightKg / (b.HeightM * b.HeightM)

	// HRV chases a target set by fitness and dragged down by everything that
	// stresses the system. It moves a third of the way per day: fast enough
	// that a wearable shows this morning's bad night, slow enough that the
	// trend is the real signal.
	hrvTarget := 18 + b.Fitness*0.85 - b.SleepDebt*0.9 - b.Load*0.12 - dietHRV(d.Diet) - orderHRV(d.Order)
	b.HRV = approach(b.HRV, clamp(hrvTarget, 12, 120), 0.33)

	// Resting heart rate is the same story read from the other side: lower with
	// fitness, higher with debt and unrecovered load.
	hrTarget := 82 - b.Fitness*0.55 + b.SleepDebt*0.32 + b.Load*0.05
	b.RestingHR = approach(b.RestingHR, clamp(hrTarget, 38, 110), 0.25)

	// Weight is an energy balance, moving in grams rather than kilos.
	b.WeightKg = clamp(b.WeightKg+dietWeight(d.Diet)+trainingBurn(d.Training), 35, 250)

	// One short night measurably reduces insulin sensitivity the next morning.
	// Diet is the larger term over time; debt is why a clean eater with no
	// sleep still drifts.
	glucoseTarget := 4.7 + b.SleepDebt*0.045 + dietGlucose(d.Diet) + math.Max(0, bmi-22)*0.03
	b.Glucose = approach(b.Glucose, clamp(glucoseTarget, 3.9, 9), 0.2)

	// Inflammation answers to all three at once, which is why it is the marker
	// that catches a player doing everything right except sleeping.
	crpTarget := 0.5 + b.SleepDebt*0.075 + dietCRP(d.Diet) + math.Max(0, b.Load-70)*0.02
	b.CRP = approach(b.CRP, clamp(crpTarget, 0.2, 12), 0.15)

	// Lipids move over weeks, not days. The slow constant is the point: a
	// player who cleans up their diet will not see this move for a fortnight,
	// and finding that out is part of the lesson.
	ldlTarget := 2.3 + dietLDL(d.Diet) + math.Max(0, bmi-24)*0.05
	b.LDL = approach(b.LDL, clamp(ldlTarget, 1.2, 6), 0.06)

	// The stress axis: sleep debt first, then hard days taken without recovery.
	cortisolTarget := 290 + b.SleepDebt*11 + trainingCortisol(d.Training) -
		dietCortisol(d.Diet) + orderCortisol(d.Order)
	b.Cortisol = approach(b.Cortisol, clamp(cortisolTarget, 150, 700), 0.3)

	// Recovery is not a driver — it is the wearable's reading of the drivers,
	// this morning's HRV against what this body is capable of. Recomputed
	// rather than accumulated, so it can never drift away from what produced it.
	b.Recovery = recovery(b)

	return b
}

// recovery reads the 0–100 score a wearable reports.
//
// Against this body's *own* baseline rather than a population one, which is how
// the real devices work and why the number is comparable to yesterday's rather
// than to somebody else's. A body sitting exactly at its baseline reads 85, so
// there is somewhere above it to be as well as somewhere below.
//
// Sleep debt is in here only lightly, because HRV already carries most of it —
// counting it twice would let the score bottom out and stop distinguishing bad
// from catastrophic, which is exactly where a player needs the gradient.
func recovery(b Body) float64 {
	baseline := 18 + b.Fitness*0.85
	if baseline <= 0 {
		return 0
	}
	ratio := b.HRV / baseline
	return clamp(ratio*85-b.SleepDebt*0.5-math.Max(0, b.Load-70)*0.6, 0, 100)
}

/*
BiologicalAge is the headline readout, composed on read and never stored.

§3.3: everything derived is computed on read; only the inputs are persisted.
§3.5: health surfaces as a biological age rather than a score, because "your
biological age is 24" carries meaning that "health: 72" does not, and it is one
of the few numbers a player is motivated to drive downward.

The composition is a set of penalties against a reference body, each weighted by
roughly how much the real-world literature says that marker matters. Aerobic
capacity dominates; the blood markers are individually small and collectively
large, which is what makes the clinical tier worth buying without making it
mandatory.
*/
func BiologicalAge(b Body) float64 {
	// The age a body sitting at every reference value reads as.
	const reference float64 = 26

	years := reference

	// The single strongest term. A VO2 max of 42 is an unremarkable adult; each
	// point below costs about four months, each point above earns them back at
	// half the rate, because fitness protects more than it rejuvenates.
	if b.Fitness < 42 {
		years += (42 - b.Fitness) * 0.34
	} else {
		years -= (b.Fitness - 42) * 0.17
	}

	years += math.Max(0, b.RestingHR-58) * 0.13
	years += math.Max(0, 45-b.HRV) * 0.10
	years += math.Max(0, bmiOf(b)-23) * 0.45
	years += math.Max(0, b.Glucose-5.0) * 2.6
	years += math.Max(0, b.CRP-1.0) * 1.4
	years += math.Max(0, b.LDL-2.6) * 1.1
	years += math.Max(0, b.Cortisol-380) * 0.006

	// Debt is a driver rather than a marker, but it belongs in the readout: a
	// body carrying a week of lost sleep *is* older this morning, and it is the
	// term that recovers fastest, which is the encouraging half of the lesson.
	years += b.SleepDebt * 0.14

	return clamp(years, 18, 95)
}

// bmiOf is body mass index — the one composite the panel reads off two metrics
// rather than measuring.
func bmiOf(b Body) float64 {
	if b.HeightM <= 0 {
		return 22
	}
	return b.WeightKg / (b.HeightM * b.HeightM)
}

/* ── effects ──────────────────────────────────────────────────────────── */

// Sampled says a day of living produced a data point.
//
// The caller writes it to the biomarker time series (§3.2), which is the
// evidence a diagnosis is later argued from (§3.6). The core does not know
// there is a table; it knows the day happened and when.
type Sampled struct {
	At   time.Time
	Body Body
}

func (Sampled) effect() {}

// Band is a named region of a metric — the vocabulary a player is given for
// what a number means, since a number alone teaches nothing.
type Band int

const (
	Poorly Band = iota
	Fair
	Good
)

func (b Band) String() string {
	switch b {
	case Poorly:
		return "poor"
	case Fair:
		return "fair"
	default:
		return "good"
	}
}

// Crossed says a metric moved between bands today.
//
// This is what makes the simulation *legible* without narrating it. A crossing
// is the raw material of an event release (§4) and of the evidence a diagnosis
// points at (§3.6) — and deciding which of those it becomes is the caller's
// business, not this package's.
type Crossed struct {
	At     time.Time
	Metric string
	From   Band
	To     Band
}

func (Crossed) effect() {}

// crossings reports every band a day moved a metric across.
//
// Only the metrics whose bands are meaningful to a player are watched. A
// crossing on every marker would be noise, and noise is what stops a player
// noticing the one that matters.
func crossings(at time.Time, before, after Body) Effects {
	var fx Effects
	watch := []struct {
		metric string
		band   func(Body) Band
	}{
		{"recovery", func(b Body) Band { return bandOf(b.Recovery, 34, 67) }},
		{"sleep debt", func(b Body) Band { return bandOf(-b.SleepDebt, -16, -6) }},
		{"inflammation", func(b Body) Band { return bandOf(-b.CRP, -3, -1.5) }},
	}
	for _, w := range watch {
		from, to := w.band(before), w.band(after)
		if from != to {
			fx = append(fx, Crossed{At: at, Metric: w.metric, From: from, To: to})
		}
	}
	return fx
}

// bandOf places a value against two thresholds. Metrics where lower is better
// are passed negated, so there is one comparison rule rather than two.
func bandOf(v, poor, fair float64) Band {
	switch {
	case v < poor:
		return Poorly
	case v < fair:
		return Fair
	default:
		return Good
	}
}

/* ── the vocabulary's numbers ─────────────────────────────────────────── */

func trainingLoad(t Training) float64 {
	switch t {
	case Light:
		return 12
	case Hard:
		return 32
	default:
		return 0
	}
}

func trainingStimulus(t Training) float64 {
	switch t {
	case Light:
		return 0.045
	case Hard:
		return 0.11
	default:
		return 0
	}
}

func trainingBurn(t Training) float64 {
	switch t {
	case Light:
		return -0.02
	case Hard:
		return -0.05
	default:
		return 0
	}
}

func trainingCortisol(t Training) float64 {
	switch t {
	case Light:
		return 8
	case Hard:
		return 34
	default:
		return 0
	}
}

func dietHRV(d Diet) float64 {
	switch d {
	case Poor:
		return 4
	case Clean:
		return -2
	default:
		return 0
	}
}

func dietWeight(d Diet) float64 {
	switch d {
	case Poor:
		return 0.06
	case Clean:
		return -0.025
	default:
		return 0
	}
}

func dietGlucose(d Diet) float64 {
	switch d {
	case Poor:
		return 0.85
	case Clean:
		return -0.1
	default:
		return 0.25
	}
}

func dietCRP(d Diet) float64 {
	switch d {
	case Poor:
		return 1.15
	case Clean:
		return -0.15
	default:
		return 0.35
	}
}

func dietLDL(d Diet) float64 {
	switch d {
	case Poor:
		return 1.05
	case Clean:
		return -0.35
	default:
		return 0.2
	}
}

func dietCortisol(d Diet) float64 {
	switch d {
	case Clean:
		return 14
	default:
		return 0
	}
}

/* ── arithmetic ───────────────────────────────────────────────────────── */

// approach moves a value a fraction of the way to a target. The one shape every
// biomarker here uses, because bodies have inertia and step changes are a tell
// that a number is a dial rather than a measurement.
func approach(now, target, rate float64) float64 {
	return now + (target-now)*rate
}

func clamp(v, lo, hi float64) float64 {
	return math.Min(math.Max(v, lo), hi)
}
