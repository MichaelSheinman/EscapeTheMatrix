package health

import (
	"fmt"
	"math"
)

/*
What the body says about itself, in the player's own words — the evidence half of
§3.6's diagnosis.

This is here rather than in internal/diagnosis for the reason §3.1 keeps
repeating: the module that owns the rows is the module that knows what they mean.
Whether six hours is a short night, whether four beats is a rise or a Tuesday,
and how long a run has to be before it is a run are all facts about the panel,
and a package that made a diagnosis out of them would be a second copy of this
file that drifted.

Three rules hold over everything below.

**Nothing is authored as an answer.** §3.6's first rule bans the four-option
quiz, so what comes out of here is a *reading* — a count off the series with the
player's own numbers in it. The template is written; the sentence is not.

**A reading nobody bought is not produced at all.** §3.16 ties instrumentation to
what a player can see, and §3.16 again ties that to the diagnosis loop directly:
"a player without a watch still suffers real HRV collapse; they simply can't see
the cause. That *is* the diagnosis problem." So every observation below asks
[Panel.Sees] first, and a baseline player argues from three readings while a
clinical one argues from eight. The mechanic gets sharper as the money goes in,
which is the loop §3.16 says the economy exists to close.

**Nothing here says a reading is bad.** A count is a count. "Under six hours on
nine of the last fourteen nights" is a thing a player can disagree with; "your
sleep is poor" is the game telling them what to think (§3.2).
*/

// Observation is one thing the series says, ready to be pointed at.
//
// Structurally identical to internal/diagnosis's own type and deliberately not
// that type: this package does not import the one that consumes it, so the
// dependency runs one way and the composition root does the conversion (§3.1).
type Observation struct {
	Key  string
	Says string
	From string
}

/*
Window is how far back an observation looks.

A fortnight, and it is the same number §3.15 uses for a pattern: "one bad
outcome is noise, the same rule failing every morning for a week is a pattern."
Two weeks is a run somebody could plausibly have caused, which is what makes it
arguable — a year would be a fact about a life and a day would be a fact about a
day, and neither is a thing you can conclude something from.

The series holds three weeks (see historyDays), so the window is always inside
what has been recorded.
*/
const Window = 14

/*
Observed is everything this panel currently says, in panel order.

An empty slice is a perfectly ordinary answer: a body that is doing nothing
remarkable observes nothing, and a player in that position has nothing to argue
from — which is §3.6's struggle-before-tool as an absence rather than as a
message.
*/
func (p Panel) Observed() []Observation {
	days := p.window()
	if len(days) < 2 {
		// One day is not a series. Nothing about a body can be concluded from
		// a single reading, and offering one as evidence would invite exactly
		// the conclusion §3.6 spends its length refusing to reward.
		return []Observation{}
	}

	out := []Observation{}
	for _, o := range []func([]Sample) (Observation, bool){
		p.shortNights,
		p.restingRate,
		p.weightDrift,
		p.hrvFalling,
		p.loadUnanswered,
		p.glucoseHigh,
		p.inflammationHigh,
	} {
		if said, ok := o(days); ok {
			out = append(out, said)
		}
	}
	return out
}

/*
Observable is every key this package can produce, whatever the instrumentation.

Exported so the composition root can check the diagnosis catalogue against what
the bound ledgers can actually say (§3.6). It is written out rather than derived
from the readings above, because the point of the check is to catch a key that
was typed once here and once in a YAML file and does not match — and a list
derived from the same misspelling would agree with it.
*/
func Observable() []string {
	return []string{
		"health/short-nights",
		"health/resting-rate-climbing",
		"health/weight-drifting",
		"health/hrv-falling",
		"health/load-unanswered",
		"health/glucose-high",
		"health/inflammation-high",
	}
}

// window is the last [Window] days of the series, oldest first.
func (p Panel) window() []Sample {
	if len(p.History) <= Window {
		return p.History
	}
	return p.History[len(p.History)-Window:]
}

/*
shortNights counts the nights under [Short] hours.

A count rather than a mean, because a mean of seven hours hides four nights of
four and three of ten, and it is the four that did the damage. §3.16's own
example sentence for this whole mechanic — "your recovery has been collapsing for
three weeks" — is a count of days, not an average.
*/
func (p Panel) shortNights(days []Sample) (Observation, bool) {
	m, ok := metric("sleep")
	if !ok || !p.Sees(m) {
		return Observation{}, false
	}
	short := 0
	for _, s := range days {
		if m.Read(s.Body) < Short {
			short++
		}
	}
	// Two is a fortnight with a couple of late ones in it, which is a life
	// rather than a finding.
	if short < 3 {
		return Observation{}, false
	}
	return Observation{
		Key:  "health/short-nights",
		Says: fmt.Sprintf("You slept under %g hours on %d of the last %d nights.", Short, short, len(days)),
		From: m.Tier.Name,
	}, true
}

// Short is the hours below which a night counts as a short one.
//
// Not a threshold the simulation knows about: the pure core has no notion of a
// night being short, it just carries the debt. This is the reading's word for
// it, and it lives beside the reading.
const Short float64 = 6

// restingRate is the direction the resting heart rate has moved across the
// window.
func (p Panel) restingRate(days []Sample) (Observation, bool) {
	return p.moved(days, "resting-hr", 3, "health/resting-rate-climbing",
		"Your resting heart rate is %s %s higher than it was %s ago.")
}

// weightDrift is the same reading on the scale, and it is the one metric where
// the direction is not the point — a body losing three kilos in a fortnight is
// as much a finding as one gaining them.
func (p Panel) weightDrift(days []Sample) (Observation, bool) {
	m, ok := metric("weight")
	if !ok || !p.Sees(m) {
		return Observation{}, false
	}
	first, last := m.Read(days[0].Body), m.Read(days[len(days)-1].Body)
	if math.Abs(last-first) < 1.5 {
		return Observation{}, false
	}
	way := "up"
	if last < first {
		way = "down"
	}
	return Observation{
		Key: "health/weight-drifting",
		Says: fmt.Sprintf("Your weight has gone %s %s %s in %s.",
			way, round(math.Abs(last-first), m.Decimals), m.Unit, span(len(days)-1)),
		From: m.Tier.Name,
	}, true
}

// hrvFalling is the wearable reading §3.16 names as the diagnosis problem's
// worked example, and a baseline player cannot see it.
func (p Panel) hrvFalling(days []Sample) (Observation, bool) {
	return p.moved(days, "hrv", -4, "health/hrv-falling",
		"Your heart-rate variability is %s %s lower than it was %s ago.")
}

/*
loadUnanswered is the one observation that reads two metrics against each other.

Training load is not a finding on its own — the metric's own sentence says so:
"not bad in itself; bad without recovery underneath it." So the reading is the
pair, and it is produced only when both are visible, which makes it the most
expensive sentence in the game to be able to say.
*/
func (p Panel) loadUnanswered(days []Sample) (Observation, bool) {
	load, okL := metric("load")
	rec, okR := metric("recovery")
	if !okL || !okR || !p.Sees(load) || !p.Sees(rec) {
		return Observation{}, false
	}
	hard := 0
	for _, s := range days {
		// A day the body was asked for something while it was not ready for
		// it. Both halves are the player's own doing, which is what makes the
		// count arguable rather than unlucky.
		if load.Read(s.Body) > Working && rec.Read(s.Body) < Ready {
			hard++
		}
	}
	if hard < 3 {
		return Observation{}, false
	}
	return Observation{
		Key: "health/load-unanswered",
		Says: fmt.Sprintf("On %d of the last %d days you trained on a recovery under %g.",
			hard, len(days), Ready),
		From: load.Tier.Name,
	}, true
}

// Working is the load above which a day counts as having been trained, and Ready
// is the recovery below which the body had not answered the last one. Both are
// this reading's words, like [Short].
const (
	Working float64 = 40
	Ready   float64 = 60
)

func (p Panel) glucoseHigh(days []Sample) (Observation, bool) {
	return p.over(days, "glucose", 5.6, "health/glucose-high",
		"Your fasting glucose has been over %s on %d of the last %d readings.")
}

func (p Panel) inflammationHigh(days []Sample) (Observation, bool) {
	return p.over(days, "crp", 3, "health/inflammation-high",
		"Your inflammation marker has been over %s on %d of the last %d readings.")
}

/*
moved is the shared shape of a metric that has drifted one way across the window.

`by` is signed: positive is a rise worth remarking on, negative a fall. The
threshold is the reading's judgement about noise and nothing else — it does not
say which direction is healthy, because [Metric.Lower] already does and repeating
it here is how the two would come apart.
*/
func (p Panel) moved(days []Sample, key string, by float64, id, says string) (Observation, bool) {
	m, ok := metric(key)
	if !ok || !p.Sees(m) {
		return Observation{}, false
	}
	first, last := m.Read(days[0].Body), m.Read(days[len(days)-1].Body)
	change := last - first
	if by > 0 && change < by {
		return Observation{}, false
	}
	if by < 0 && change > by {
		return Observation{}, false
	}
	return Observation{
		Key:  id,
		Says: fmt.Sprintf(says, round(math.Abs(change), m.Decimals), m.Unit, span(len(days)-1)),
		From: m.Tier.Name,
	}, true
}

// span writes a span the way a person would say it.
//
// Trivial, and it is here because the alternative shipped: a window narrower
// than the fortnight — which is every player in their first fortnight — reads
// "1 days ago", and a reading whose grammar is wrong is a reading somebody
// stops trusting the arithmetic of.
func span(n int) string {
	if n == 1 {
		return "a day"
	}
	return fmt.Sprintf("%d days", n)
}

// over counts the readings above a line.
func (p Panel) over(days []Sample, key string, line float64, id, says string) (Observation, bool) {
	m, ok := metric(key)
	if !ok || !p.Sees(m) {
		return Observation{}, false
	}
	high := 0
	for _, s := range days {
		if m.Read(s.Body) > line {
			high++
		}
	}
	if high < 3 {
		return Observation{}, false
	}
	return Observation{
		Key:  id,
		Says: fmt.Sprintf(says, round(line, m.Decimals), high, len(days)),
		From: m.Tier.Name,
	}, true
}

// metric finds one by key. A miss is a metric renamed out of the panel, and the
// observation that read it simply stops being produced rather than panicking on
// a screen somebody is looking at.
func metric(key string) (Metric, bool) {
	for _, m := range Metrics {
		if m.Key == key {
			return m, true
		}
	}
	return Metric{}, false
}

// round formats a number to a metric's own resolution, so a reading never claims
// more precision than the instrument that took it.
func round(v float64, decimals int) string {
	return fmt.Sprintf("%.*f", decimals, v)
}
