package conflict

import (
	"fmt"
	"sort"
	"strings"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/playbook"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
What may be said to somebody, given everything already said to them.

Until this file, a pathway's only gate was its [Strain]: the day found whoever
was most overdue, read the pressure they were under, and drew from every pathway
in the population authored for that pressure. Two things followed, and both of
them read on the phone as the game not knowing who anybody was.

**A moment belonged to nobody.** `the-stairwell` is a rota and a boiler man —
Ruthie's whole life, per `content/environments/wilmington.yaml` — and it was
equally available to Bez behind the counter and Kyle at Kestrel & Vane.
Worse than a mismatch: `short-at-the-counter` is somebody caught short *at* the
counter, which is
the one thing Bez cannot be, and he had a one-in-six chance of sending it.

**A moment had no position.** `the-favour-back` opens "I know I still owe you
for the allotment afternoon, and here I am asking again", and could be the first
thing a stranger ever texted you. There was no allotment afternoon. The strain
gate does not help here, because a strain is a state — "I owe them something" —
and a state says nothing about which of the four conversations that could
follow from it has already been had.

# Two fields, and the second one is the thread

  - [Authored.With] is who may send it, and it is required. Nothing is
    person-agnostic by default any more, because the default is what produced
    the above: fourteen files silent on the question and thirteen of them
    wrong. A moment that genuinely fits everybody says so by naming everybody,
    which costs six words and means somebody read all six and agreed.
  - [Authored.After] is what must already have passed *with that same person*.
    Per-person is the whole of it: Ruthie answering `the-quick-hand` unlocks
    nothing with Tomasz, because the allotment afternoon the sequel remembers
    was Tomasz's afternoon.

# Gated at selection, so there is no gate to get past

[Reached] is consulted where the pool is drawn rather than where the row is
written. A pathway whose thread has not reached it is not a candidate — not a
candidate that is then refused — so there is no code path, in the game or in
the developer's toolbar, along which B can arrive before A. The rule cannot be
bypassed because there is nothing to bypass.

# An answer is part of the requirement

`after: [the-quick-hand]` alone would still produce the incoherence one level
up: the sequel says "I know I still owe you", which is only true if the player
actually turned up. So a requirement may name the posture it needs, and a
person's thread branches on how they were treated rather than merely on what
they were asked. A requirement with no stance is satisfied by any of them,
which is the right default for the case where only the subject matters.

# What repeats, and how often

Ordering implies exhaustion: if `the-fourth-ask` could arise again it would not
be the fourth ask. So an answered pathway is spent, and a population played for
a fortnight would go silent — every thread run out, nobody with anything left
to say.

[Authored.Every] is the way back. A pathway with a cadence comes round again
that many days after it was last answered, which is what most of living beside
somebody actually is: Tomasz asks about the car most weeks, Bez is short at the
till about once a month, and neither is a story with an ending. So a person's
writing is a few threads that run once and a few standing habits that do not,
and the standing habits are what keep them talking after the threads are done.
*/

/*
Requirement is one thing that must already have passed with this person.

Stance empty means any answer will do — the requirement is that the
conversation happened, not that it went a particular way.
*/
type Requirement struct {
	Conflict string
	Stance   playbook.Stance
}

// Requires reports whether this requirement is satisfied by one past
// conversation.
func (r Requirement) Requires(p Passed) bool {
	if p.Conflict != r.Conflict {
		return false
	}
	return r.Stance == "" || r.Stance == p.Stance
}

// String is the requirement as an author would have written it, used by the
// loader's failures and the toolbar's refusals so that a message about a gate
// names the gate in the file's own words.
func (r Requirement) String() string {
	if r.Stance == "" {
		return r.Conflict
	}
	return r.Conflict + " (" + string(r.Stance) + ")"
}

/*
Passed is one conversation that has already been settled with somebody.

Read off the `conflict` table, which has carried every field of it since 00021
— the person, the pathway, the posture it was met with and the day. So the
ordering rule needs no column of its own: what has passed between two people
is already written down, and a second record of it could only ever disagree
with the first.

Withdrawn conflicts are absent rather than excluded: [Conflicts.Withdraw]
deletes the row, so a moment the toolbar took back never happened and unlocks
nothing.
*/
type Passed struct {
	Conflict string
	Stance   playbook.Stance
	On       simulation.Date
}

// History is everything that has passed between the player and each person of
// a population, by person key, oldest first.
type History map[string][]Passed

/*
Reached reports whether a pathway may arise with somebody today.

Three questions, and a pathway has to answer all of them:

  - **Is it theirs?** [Authored.With] names who may send it.
  - **Has the thread got here?** Every [Requirement] must be satisfied by
    something already settled with this same person.
  - **Is it due?** Never said before, or said far enough back that its cadence
    has come round again. A pathway with no cadence is said once and is then
    spent for good.

`on` is the day being generated rather than the wall clock, so a rollover
catching up on a week of missed days asks the question as each of those days
would have asked it.
*/
func Reached(a Authored, person string, past []Passed, on simulation.Date) bool {
	if !a.Sends(person) {
		return false
	}
	for _, need := range a.After {
		if !satisfied(need, past) {
			return false
		}
	}
	return due(a, past, on)
}

// Wanting is every requirement of a pathway that this history does not yet
// satisfy.
//
// The same walk [Reached] makes, keeping what it finds rather than only whether
// it found everything. Nothing in the game calls it: it exists so the
// developer's toolbar can say *which* conversation is missing rather than that
// one is, which is the difference between a refusal somebody can act on and a
// closed door.
func Wanting(a Authored, past []Passed) []Requirement {
	var out []Requirement
	for _, need := range a.After {
		if !satisfied(need, past) {
			out = append(out, need)
		}
	}
	return out
}

// satisfied reports whether anything in a history answers one requirement.
func satisfied(need Requirement, past []Passed) bool {
	for _, p := range past {
		if need.Requires(p) {
			return true
		}
	}
	return false
}

/*
due reports whether enough has elapsed for a pathway to be said again.

The last time it was said rather than the first: a weekly ask is weekly from
the most recent one, and a history is walked whole rather than assumed sorted,
because the query that fills it is free to change its order without changing
what a cadence means.
*/
func due(a Authored, past []Passed, on simulation.Date) bool {
	said := false
	var last simulation.Date
	for _, p := range past {
		if p.Conflict != a.Key {
			continue
		}
		if !said || last.Before(p.On) {
			said, last = true, p.On
		}
	}
	switch {
	case !said:
		return true
	case a.Every <= 0:
		// Said once and spent. A thread has an end, and this is it.
		return false
	default:
		return last.Until(on) >= a.Every
	}
}

/*
Openers is every pathway a person may be met with before anything has passed
between them at all.

The floor the loader holds every population to. A person whose writing is all
sequel is a person the day can never reach: [Conflicts.pathway] would filter
their pool to nothing under every pressure including [Easy], and somebody the
player lives two doors from would simply never speak. That is a hole in the
content rather than a quiet day, so it is a failed build.
*/
func Openers(among, person string) []Authored { return openers(catalogue, among, person) }

// openers is [Openers] over a catalogue that is not the shipped one yet, which
// is the state the loader's own check runs in.
func openers(all []Authored, among, person string) []Authored {
	var out []Authored
	for _, a := range all {
		if a.Among == among && a.Sends(person) && len(a.After) == 0 {
			out = append(out, a)
		}
	}
	return out
}

/*
ordering checks every rule about `with:` and `after:` that the selection then
gets to assume, across a whole parsed catalogue.

Separate from the per-file checks in [pathway] because none of these can be
answered by one file: whether a requirement names something real, whether the
two ends of it share a person, whether a thread eventually closes, and whether
everybody has somewhere to start are all questions about the population.

`people` is the population by environment key. Taken as an argument rather than
read from internal/environment for the reason [parse] takes it: the failures
here are the ones that decide whether the mechanic has holes in it, and they
have to be reachable from a test with an invented population.
*/
func ordering(all []Authored, people map[string][]string) error {
	byKey := map[string]Authored{}
	for _, a := range all {
		byKey[a.Among+"/"+a.Key] = a
	}

	for _, a := range all {
		seen := map[string]bool{}
		for _, need := range a.After {
			if seen[need.Conflict] {
				return fmt.Errorf("%q waits on %q twice", a.Key, need.Conflict)
			}
			seen[need.Conflict] = true

			before, written := byKey[a.Among+"/"+need.Conflict]
			if !written {
				return fmt.Errorf("%q waits on %q, which %s has not got",
					a.Key, need.Conflict, a.Among)
			}
			// The two ends of a thread have to be sayable by one person, or the
			// sequel is unreachable by construction: the requirement is read
			// against what has passed with *this* person, and nobody who can
			// send this can ever have been sent that.
			if shared(a.With, before.With) == nil {
				return fmt.Errorf("%q waits on %q, and nobody who sends %q can have been sent %q",
					a.Key, need.Conflict, a.Key, need.Conflict)
			}
			// A posture nobody could have taken is a gate that never opens. The
			// answers are checked against the question they put in [pathway],
			// so this only has to ask whether the earlier moment was ever met
			// that way.
			if need.Stance != "" {
				if _, answers := before.Answer(need.Stance); !answers {
					return fmt.Errorf("%q waits on %q being met with %q, which %q has no answer for",
						a.Key, need.Conflict, need.Stance, need.Conflict)
				}
			}
		}
	}

	if err := acyclic(all, byKey); err != nil {
		return err
	}

	// Everybody has somewhere to start, under the one pressure everybody is
	// always under. [Standing.Under] ends in [Easy] for every standing there
	// is, so an opener that follows from anything else is an opener only for a
	// person who happens to be in that state — and a stranger is in none of
	// them.
	for among, population := range people {
		for _, person := range population {
			opens := false
			for _, a := range openers(all, among, person) {
				if a.Follows == Easy {
					opens = true
					break
				}
			}
			if !opens {
				return fmt.Errorf("nobody can open a conversation with %q of %s: every pathway they send "+
					"waits on another, so they would never speak", person, among)
			}
		}
	}
	return nil
}

// acyclic refuses a thread that waits on itself, directly or round a loop.
//
// A cycle passes every check above — every requirement names a real pathway
// and shares a person with it — and produces a set of moments none of which can
// ever arise. It would read as content that was written and never shipped.
func acyclic(all []Authored, byKey map[string]Authored) error {
	const (
		open = 1
		shut = 2
	)
	state := map[string]int{}

	var walk func(a Authored, trail []string) error
	walk = func(a Authored, trail []string) error {
		id := a.Among + "/" + a.Key
		switch state[id] {
		case shut:
			return nil
		case open:
			return fmt.Errorf("%s waits on itself: %s", a.Key, strings.Join(append(trail, a.Key), " → "))
		}
		state[id] = open
		for _, need := range a.After {
			if before, written := byKey[a.Among+"/"+need.Conflict]; written {
				if err := walk(before, append(trail, a.Key)); err != nil {
					return err
				}
			}
		}
		state[id] = shut
		return nil
	}

	for _, a := range all {
		if err := walk(a, nil); err != nil {
			return err
		}
	}
	return nil
}

// shared is a person both pathways may be sent by, or nil for none.
func shared(a, b []string) []string {
	in := map[string]bool{}
	for _, key := range a {
		in[key] = true
	}
	var out []string
	for _, key := range b {
		if in[key] {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}
