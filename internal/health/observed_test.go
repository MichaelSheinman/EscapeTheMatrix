package health

import (
	"strings"
	"testing"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
What the body says about itself (§3.6's evidence half).

Pure functions over a panel, so none of this needs a database — which is the
point of computing the readings from [Panel] rather than from a query, and is why
the thresholds are testable at all.
*/

// series builds a window of samples from one metric's values, oldest first,
// with everything else left at a plain starting body.
func series(set func(*simulation.Body, float64), values ...float64) []Sample {
	out := make([]Sample, 0, len(values))
	for i, v := range values {
		b := simulation.Starting(1.78, 78)
		set(&b, v)
		out = append(out, Sample{On: simulation.Date{}.Plus(i), Body: b})
	}
	return out
}

func slept(b *simulation.Body, v float64)   { b.LastSleep = v }
func pulse(b *simulation.Body, v float64)   { b.RestingHR = v }
func balance(b *simulation.Body, v float64) { b.HRV = v }

// says finds one observation by key.
func says(out []Observation, key string) (Observation, bool) {
	for _, o := range out {
		if o.Key == key {
			return o, true
		}
	}
	return Observation{}, false
}

// baseline is a panel with no instrumentation beyond the free tier.
func baseline(history []Sample) Panel {
	return Panel{Owned: []Tier{Baseline}, History: history}
}

/*
A run of short nights is counted, and a fortnight with two late ones is not.

The threshold is the reading's own judgement about what a finding is, and the
line matters: a player whose ordinary life produced an observation every single
day would learn to ignore the whole surface.
*/
func TestARunOfShortNightsIsSomethingToPointAt(t *testing.T) {
	t.Parallel()

	short := baseline(series(slept, 4, 4.5, 5, 4, 5.5, 4, 4.5, 8, 8, 8, 8, 8, 8, 8))
	o, ok := says(short.Observed(), "health/short-nights")
	if !ok {
		t.Fatal("seven nights under six hours said nothing")
	}
	if !strings.Contains(o.Says, "7 of the last 14") {
		t.Errorf("the reading is %q, and it should carry the player's own count", o.Says)
	}

	ordinary := baseline(series(slept, 5.5, 5.8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8))
	if _, ok := says(ordinary.Observed(), "health/short-nights"); ok {
		t.Error("a fortnight with two late nights in it was reported as a finding")
	}
}

/*
A reading nobody bought the ability to see is not produced at all.

§3.16 ties the diagnosis loop to the economy — "a player without a watch still
suffers real HRV collapse; they simply can't see the cause. That *is* the
diagnosis problem" — and this is the whole of that rule in code. The body is
identical in both panels below; only the instrumentation differs.
*/
func TestAReadingNobodyBoughtIsNotEvidence(t *testing.T) {
	t.Parallel()

	falling := series(balance, 70, 68, 66, 64, 62, 60, 58, 56, 54, 52, 50, 48, 46, 44)

	if _, ok := says(baseline(falling).Observed(), "health/hrv-falling"); ok {
		t.Error("a player with no watch was offered a heart-rate variability reading to argue from")
	}

	watched := Panel{Owned: []Tier{Baseline, Wearable}, History: falling}
	if _, ok := says(watched.Observed(), "health/hrv-falling"); !ok {
		t.Error("a player with a watch was not offered the reading they paid for")
	}
}

// A single day is not a series, and nothing can be concluded from one reading.
func TestOneDayObservesNothing(t *testing.T) {
	t.Parallel()
	if got := baseline(series(slept, 3)).Observed(); len(got) != 0 {
		t.Errorf("a single day produced %d observations", len(got))
	}
}

/*
Nothing a reading says is a verdict about it.

§3.2 in the one place it is hardest to hold: a count of short nights is a fact
and "your sleep is poor" is the game telling the player what to think, and the
two are one careless edit apart.
*/
func TestNoReadingSaysWhetherAnythingIsGood(t *testing.T) {
	t.Parallel()
	verdicts := []string{"poor", "bad", "good", "healthy", "unhealthy", "should", "too much", "too little"}

	p := Panel{
		Owned:   []Tier{Baseline, Wearable, Clinical},
		History: series(pulse, 55, 56, 57, 58, 59, 60, 61, 62, 63, 64, 65, 66, 67, 68),
	}
	for _, o := range p.Observed() {
		said := strings.ToLower(o.Says)
		for _, v := range verdicts {
			if strings.Contains(said, v) {
				t.Errorf("%q says %q, and a reading states what happened rather than what to think of it", o.Key, v)
			}
		}
	}
}

// Every key the readings can produce is in Observable, which is what the boot
// check compares the diagnosis catalogue against.
func TestObservableNamesEveryReading(t *testing.T) {
	t.Parallel()
	known := map[string]bool{}
	for _, k := range Observable() {
		known[k] = true
	}

	// A body doing everything at once, seen by everything at once, so as many
	// readings as possible fire.
	p := Panel{Owned: []Tier{Baseline, Wearable, Clinical}}
	for i := range Window {
		b := simulation.Starting(1.78, 78)
		b.LastSleep, b.RestingHR = 4, 60+float64(i)
		b.WeightKg, b.HRV = 78+float64(i)/4, 70-float64(i)
		b.Load, b.Recovery = 80, 30
		b.Glucose, b.CRP = 6.4, 5
		p.History = append(p.History, Sample{On: simulation.Date{}.Plus(i), Body: b})
	}
	for _, o := range p.Observed() {
		if !known[o.Key] {
			t.Errorf("%q is produced and is not in Observable, so a cause could speak from it and never be checked", o.Key)
		}
	}
}
