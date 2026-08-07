package event

import (
	"testing"
	"time"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
The release policy, with nothing running (§14).

[Attentive] is pure, which is the point of it being a value rather than a method
on the module — so every claim technical.md §4 makes about release is a table
test here rather than a database fixture. What the database tests next door prove
is that the policy is *asked* with the truth; what these prove is that it answers
§4.
*/

// settled is a player mid-session who has been offered nothing yet: no absence,
// nothing standing, nothing released, and behind on nothing.
func settled() Attention {
	return Attention{
		Now:       time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC),
		Since:     Never,
		SinceKind: Never,
		Kind:      Interpersonal,
	}
}

/*
The three floors, which are not heuristics: one thing at a time, a gap between
two of them, and never more than the day's allowance.
*/
func TestTheWorldSaysOneThingAtATimeAndLeavesAGapBetweenThem(t *testing.T) {
	t.Parallel()

	standing := settled()
	standing.Standing = true
	if (Attentive{}).Release(standing) {
		t.Error("something was released on top of something already on screen")
	}

	// Even an escalation waits for the room to be quiet. It is exempt from the
	// caps, not from the player finishing the sentence they are reading.
	standing.Raised = true
	if (Attentive{}).Release(standing) {
		t.Error("an escalation was released on top of something already on screen")
	}

	soon := settled()
	soon.Since, soon.SinceKind = Quiet-time.Minute, Quiet-time.Minute
	if (Attentive{}).Release(soon) {
		t.Errorf("two releases inside %v of each other", Quiet)
	}
	soon.Raised = true
	if (Attentive{}).Release(soon) {
		t.Error("an escalation jumped the quiet")
	}

	ok := settled()
	ok.Since, ok.SinceKind = Quiet, Quiet
	if !(Attentive{}).Release(ok) {
		t.Error("a quiet world with room in the day released nothing")
	}
}

/*
§4: "a long absence does not dump the backlog; there is a cap on what gets
surfaced."

The cap for somebody coming back is *lower* than the ordinary one, which is the
opposite of what a backlog left to itself would do — a fortnight away accrues a
deep queue, and the temptation is to spend it.
*/
func TestALongAbsenceDoesNotDumpTheBacklog(t *testing.T) {
	t.Parallel()

	back := settled()
	back.Away = Absence
	back.Since, back.SinceKind = 8*time.Hour, 8*time.Hour

	if back.Allowed() != Returning {
		t.Fatalf("somebody back after %v is allowed %d, want %d", back.Away, back.Allowed(), Returning)
	}
	for released, want := range map[int]bool{0: true, Returning: false, Returning + 1: false} {
		back.Released = released
		if got := (Attentive{}).Release(back); got != want {
			t.Errorf("with %d released today a returning player got %v, want %v", released, got, want)
		}
	}

	// A weekend is not an absence: two days away and the ordinary cap applies.
	weekend := settled()
	weekend.Away, weekend.Since, weekend.SinceKind = Absence-time.Hour, 8*time.Hour, 8*time.Hour
	if weekend.Allowed() != Daily {
		t.Errorf("a %v gap counted as an absence", weekend.Away)
	}
	weekend.Released = Returning
	if !(Attentive{}).Release(weekend) {
		t.Error("a player who was away one weekend was capped like one away a fortnight")
	}
}

/*
§4: "declining is signal. A player routing around every interpersonal encounter
is telling the release policy something worth acting on."

What is acted on is the rate. The bound is the assertion: there is no history of
refusals that makes the gap infinite, because a world you can switch off one
family at a time is a world with an opt-out from the part of it you most need.
*/
func TestWalkingAwayFromAFamilySlowsItAndNeverStopsIt(t *testing.T) {
	t.Parallel()

	// Too few to be a pattern.
	anecdote := settled()
	anecdote.Offered, anecdote.Passed = Noticed-1, Noticed-1
	if got := anecdote.Cooling(); got != Quiet {
		t.Errorf("%d refusals out of %d cooled the family to %v; that is not a pattern yet",
			anecdote.Passed, anecdote.Offered, got)
	}

	// Half taken, half not, is a person rather than a pattern.
	even := settled()
	even.Offered, even.Passed = 10, 5
	if got := even.Cooling(); got != Quiet {
		t.Errorf("taking half of them cooled the family to %v", got)
	}

	// More refused than taken slows it, and refusing everything slows it most.
	mostly := settled()
	mostly.Offered, mostly.Passed = 10, 8
	all := settled()
	all.Offered, all.Passed = 10, 10

	switch {
	case mostly.Cooling() <= Quiet:
		t.Errorf("8 of 10 walked past did not slow the family at all: %v", mostly.Cooling())
	case all.Cooling() <= mostly.Cooling():
		t.Errorf("refusing all ten (%v) is no slower than refusing eight (%v)",
			all.Cooling(), mostly.Cooling())
	case all.Cooling() > Quiet+Backing:
		t.Errorf("the gap grew to %v, past the %v it is bounded by", all.Cooling(), Quiet+Backing)
	}

	// And it is a delay rather than a ban: wait it out and the family comes
	// back at full rate.
	all.Since, all.SinceKind = Quiet+Backing, Quiet+Backing
	if !(Attentive{}).Release(all) {
		t.Error("a family the player refused every time never came round again")
	}
}

/*
The ticket's own sentence: the policy reads what the player has been declining
"including things they would avoid if handed a menu".

A family that is both the one they walk past and the one they are furthest
behind on is not cooled at all. That is the whole design argument in one branch:
the world is not a menu, and the thing being avoided is usually the thing the
game is about.
*/
func TestTheFamilyTheyAvoidAndNeedMostIsNotSlowedDown(t *testing.T) {
	t.Parallel()

	avoided := settled()
	avoided.Offered, avoided.Passed = 10, 10
	if avoided.Cooling() == Quiet {
		t.Fatal("the fixture is wrong: this should be a cooled family")
	}

	needed := avoided
	needed.Lagging = []simulation.Factor{simulation.Kindness}
	if !needed.Serves() {
		t.Fatalf("%q does not answer a lagging Kindness", needed.Kind)
	}
	if got := needed.Cooling(); got != Quiet {
		t.Errorf("the family they avoid and are furthest behind on waits %v, want %v", got, Quiet)
	}
}

/*
§4's "what would serve the player next", and the only place it changes an
outcome: the day's last release goes to a family that answers something the
player is behind on.

Not a ranking and not a recommendation — one decision between two ordinary
moments, made where nobody can see it.
*/
func TestTheLastReleaseOfTheDayGoesToWhatWouldServe(t *testing.T) {
	t.Parallel()

	last := settled()
	last.Released, last.Since, last.SinceKind = Daily-1, 8*time.Hour, 8*time.Hour

	if (Attentive{}).Release(last) {
		t.Error("the day's last release went to a family that answers nothing")
	}

	last.Lagging = []simulation.Factor{simulation.Kindness}
	if !(Attentive{}).Release(last) {
		t.Error("the day's last release was refused to the family that would serve them")
	}

	// Earlier in the day it makes no difference — the policy prefers, it does
	// not prescribe.
	early := settled()
	early.Released, early.Since, early.SinceKind = 1, 8*time.Hour, 8*time.Hour
	if !(Attentive{}).Release(early) {
		t.Error("an ordinary moment was refused with the day barely started")
	}
}

/*
The body is not answered by things that happen to you in the street.

§3.16 puts health behind routines and instrumentation, and a backlog that
started offering encounters because somebody slept badly would be the release
policy prescribing — which is the one thing §3.2 will not have.
*/
func TestNothingInTheBacklogAnswersALaggingBody(t *testing.T) {
	t.Parallel()
	for _, kind := range Kinds {
		a := settled()
		a.Kind, a.Lagging = kind, []simulation.Factor{simulation.Health}
		if a.Serves() {
			t.Errorf("%q was offered as an answer to a lagging body", kind)
		}
	}

	// Every other factor has one, so "what would serve" is never a dead end for
	// a player who is behind on something the world can act on.
	for _, which := range simulation.Factors {
		if which == simulation.Health {
			continue
		}
		if len(serves[which]) == 0 {
			t.Errorf("nothing in the backlog answers a lagging %v", which)
		}
	}
}

/*
An escalation is exempt from the caps and the cooling, and from nothing else.

§4's escalation rule exists so that a playbook with a hole in it cannot make the
game quiet. A daily cap that could swallow one would be the catch-all rule
problem re-introduced a layer down — write one rule that answers everything, and
let the release policy eat the escalations it produces.
*/
func TestAnEscalationIsNotCappedAndIsNotCooled(t *testing.T) {
	t.Parallel()

	spent := settled()
	spent.Released, spent.Since, spent.SinceKind = Daily+2, 8*time.Hour, time.Minute
	spent.Away = 30 * 24 * time.Hour
	spent.Offered, spent.Passed = 20, 20

	if (Attentive{}).Release(spent) {
		t.Fatal("the fixture is wrong: an ordinary event should be refused here")
	}
	spent.Raised = true
	if !(Attentive{}).Release(spent) {
		t.Error("an escalation was refused by a cap; a catch-all rule could then silence the game")
	}
}

// Steady is still the floor it always was: it reads none of the new signals, and
// a test that wants a policy with no opinions can still have one.
func TestTheFloorPolicyReadsNothingItWasNotGiven(t *testing.T) {
	t.Parallel()

	a := settled()
	a.Since, a.SinceKind = 8*time.Hour, time.Minute
	a.Away, a.Offered, a.Passed = 30*24*time.Hour, 20, 20
	a.Released = Daily - 1

	if !(Steady{}).Release(a) {
		t.Error("the floor policy acted on a signal it does not read")
	}
	if (Held{}).Release(a) || !(Always{}).Release(a) {
		t.Error("the two policies with no opinions grew one")
	}
}
