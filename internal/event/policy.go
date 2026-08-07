package event

import (
	"time"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
Whether the world should surface something *now*.

technical.md §4: "**Release is a server decision, not a clock event** — based on
activity, time away, and what would serve the player next", and "a long absence
does not dump the backlog; there is a cap on what gets surfaced, and the
notification box is where the server makes recommendations."

That is the whole of this file. [Attention] is the facts a policy may decide
from, [Policy] is the decision, and [Attentive] is the one the server runs.

# There is no recommendation surface

The last clause of §4 is the one that shapes the code. "The notification box is
where the server makes recommendations" means the recommendation *is* the offer:
the world does not tell the player what it thinks they should do, it decides
what to put in front of them and then puts it there. So nothing in this file
produces a list, a ranking, a reason, or anything else that could cross the
wire — [Policy] returns a bool, and the only observable consequence of the whole
heuristic is which moment happened and when.

That is also why the selection has to be good. A game that shows its reasoning
can be wrong out loud and be forgiven; one that shows nothing has to be right.
*/

/*
Attention is what the world knows about how much of the player's attention it
has already spent, and about what would be worth spending it on.

Everything on it is measured, and none of it is a decision. A policy that wanted
to know something not in here should be given it — the alternative is a policy
that reaches for a table, which is how the release heuristic ends up knowing
about biomarkers.
*/
type Attention struct {
	// Now is the instant being decided about, off the injected clock.
	Now time.Time
	// Standing is whether an offer is already on screen. A second one on top
	// of it is not a release policy, it is a queue jumping the queue.
	Standing bool
	// Released is how many events have been surfaced in the last [Around].
	Released int
	// Since is how long since the last one was done with — answered, walked
	// away from, or lapsed. [Never] when there has not been one, which compares
	// greater than every real gap.
	//
	// Done with rather than offered, because that is what [Quiet] is about: the
	// gap worth leaving is between a player finishing with one moment and being
	// handed the next, not between two starts.
	Since time.Duration
	// Away is how long the player had been away when this sitting began, out
	// of internal/account. Zero for somebody who has not been anywhere.
	//
	// §4 names it, and it is here rather than derived from [Since] because the
	// two are different facts: a player who has been offered nothing for a week
	// may have been playing every day.
	Away time.Duration

	// Kind is the family of the event that would be released.
	Kind Kind
	// SinceKind is the same gap, for that family alone. [Never] when nothing of
	// it has ever been put to this player, and never smaller than [Since].
	SinceKind time.Duration
	// Offered and Passed are how many of that family have been put to this
	// player, and how many of those they walked away from, inside [Lately].
	//
	// This is §4's signal, and it is deliberately handed to the policy as a
	// pair rather than as a ratio: "declined four of four" and "declined four
	// of forty" are different sentences and only one of them means the player
	// is routing around something.
	Offered int
	Passed  int

	// Lagging is which of the player's factors sit below their own average,
	// worst first, out of internal/factor. §4's "what would serve the player
	// next", as far as this codebase is prepared to compute it.
	//
	// A list of factors and no numbers. §3.2 keeps the hidden three
	// unquantified everywhere outside the package that scores them, and a
	// release heuristic is not an exception to that.
	Lagging []simulation.Factor

	// Raised is whether this candidate is there because something escalated to
	// the player rather than because the world generated it (§4's escalation
	// rule, and [Events.Raise]).
	Raised bool
}

// Never is the gap since a release that has never happened. Larger than any
// interval a policy will compare it against, and not a special case at the
// point of use.
const Never = time.Duration(1 << 62)

/*
serves is which family of event answers which factor.

Authored here, in the package that owns the families, rather than in content:
both vocabularies are closed sets in Go, and a mapping between two closed sets
that lived in YAML would be a third place for them to disagree.

**Health is deliberately absent.** The body is answered by routines (§3.4) and
by instrumentation (§3.16), not by things that happen to you in the street — and
a backlog that started offering encounters because somebody slept badly would be
the release policy prescribing, which is the one thing §3.2 will not have.
*/
var serves = map[simulation.Factor][]Kind{
	simulation.Wealth:       {Money, Work},
	simulation.Intelligence: {Opportunity},
	simulation.Stability:    {Interpersonal, Work},
	simulation.Kindness:     {Interpersonal},
}

/*
Serves reports whether the candidate's family answers one of the factors this
player is behind on.

The half of §4 that says what to *prefer*, as opposed to the caps that say what
to refuse. It is used twice below, and the second use is the interesting one:
a family the player has been walking away from is not cooled off if it is also
the family they most need — which is §4's "declining is signal" read the way the
design means it rather than the way it is easiest to implement.
*/
func (a Attention) Serves() bool {
	for _, lagging := range a.Lagging {
		for _, kind := range serves[lagging] {
			if kind == a.Kind {
				return true
			}
		}
	}
	return false
}

/*
Allowed is the most that may be surfaced in a rolling day, given how long the
player has been away.

§4's "a long absence does not dump the backlog; there is a cap on what gets
surfaced". The cap for somebody coming back is *lower* than the ordinary one,
which is the opposite of what a backlog would do left to itself: a fortnight
away accrues a deep queue, and the sentence in the design exists precisely
because emptying it is the tempting thing to do.
*/
func (a Attention) Allowed() int {
	if a.Away >= Absence {
		return Returning
	}
	return Daily
}

/*
Cooling is the least time this family must wait, given how the player has been
answering it.

§4: "declining is signal. A player routing around every interpersonal encounter
is telling the release policy something worth acting on." What is acted on is
the *rate*, and never the availability — the gap is bounded by [Quiet]+[Backing]
and there is no input that makes it infinite. A world that stopped offering what
somebody kept refusing would be a world the player could switch off one family
at a time, and the thing they are avoiding is usually the thing the game is
about.

It does not cool a family that would serve them (see [Attention.Serves]), which
is the ticket's "including things they would avoid if handed a menu" made
mechanical: the encounter you keep walking past and are furthest behind on
arrives at full rate.
*/
func (a Attention) Cooling() time.Duration {
	switch {
	case a.Serves():
		return Quiet
	case a.Offered < Noticed:
		// Too few to be a pattern. §3.15's rule about outcomes, applied to
		// refusals: one is noise, and a heuristic that acted on one would be
		// reading a coin toss.
		return Quiet
	case a.Passed*2 <= a.Offered:
		return Quiet
	}
	// How far past half the refusals have gone, as 0–1. Half is where a
	// preference becomes a pattern, so a player who takes as many as they pass
	// is not cooled at all and one who passes everything waits the full
	// [Backing].
	over := float64(a.Passed)/float64(a.Offered)*2 - 1
	return Quiet + time.Duration(over*float64(Backing))
}

/*
Policy decides whether to surface a candidate the backlog has already found
placeable and re-validated.

It runs *after* the placement and precondition checks rather than before,
because those are facts about the world and this is a judgement about the
player. A policy is therefore never able to place an event somewhere it does
not belong; the worst it can do is say "not now", which is the only answer it
should be able to give.

Pure, and it takes no context and no transaction. That is what keeps it a
policy: a release heuristic that could run a query would eventually run one.
*/
type Policy interface {
	Release(a Attention) bool
}

/*
The windows and the caps.

Constants rather than configuration, for the reason every other module's are: a
server running a different release policy would be a different game, and the
boards those populations share would not mean the same thing.
*/
const (
	// Around is the window releases are counted over.
	//
	// A rolling day rather than a calendar one, so a policy needs no timezone:
	// the player's local midnight is internal/rollover's business, and "four
	// things since this time yesterday" is the same sentence everywhere on
	// earth.
	Around = 24 * time.Hour

	// Lately is the window the decline signal is counted over.
	//
	// A fortnight, which is long enough that a pattern is a pattern and short
	// enough that a player who has changed how they play is not held to how
	// they used to.
	Lately = 14 * 24 * time.Hour

	// Daily is the most that will be surfaced in a rolling day.
	Daily = 4

	// Absence is how long away makes somebody a returning player rather than a
	// continuing one.
	//
	// Three days. Long enough that a weekend off is not an absence, short
	// enough that the person who opens the game after a fortnight is met by a
	// world with some restraint about it.
	Absence = 72 * time.Hour

	// Returning is the cap for the first rolling day back after [Absence].
	//
	// §4's cap on what a long absence dumps, as a number. Two rather than none,
	// because a world that had nothing to say to somebody who came back would
	// be answering "there is always something to do" with silence at the one
	// moment it matters.
	Returning = 2

	// Quiet is the least time between two releases.
	//
	// Walking across the city is several steps and every one of them is a
	// chance (§3.12). Without this, a single journey would empty the backlog
	// into one screen — which is the failure mode of a world that has plenty
	// to say and no sense of when to say it.
	Quiet = 30 * time.Minute

	// Backing is the most a family is slowed by having been walked away from.
	//
	// Added to [Quiet], never multiplied by anything unbounded, so the worst
	// case is a firm number a person can hold: half a day between two of a kind
	// the player refuses every time.
	Backing = 12 * time.Hour

	// Noticed is how many of a family have to have been put to somebody before
	// how they answered them is a signal rather than an anecdote.
	Noticed = 4
)

/*
Attentive is the release policy the server runs (§4).

Six rules, in the order they are asked, and the order is the argument:

 1. **Never a second offer while one is standing.** Not a heuristic. A moment on
    top of a moment is two moments, and neither of them is one.
 2. **Never two inside [Quiet] of each other**, whatever else is true. This is
    above the escalation exemption on purpose: a conflict the playbook could not
    handle is urgent, and there is still no reading of urgent that means "while
    they are still reading the last one".
 3. **An escalation goes through.** Past the daily cap and past the cooling,
    because §4's escalation rule exists so that a playbook with a hole in it
    cannot be quiet — and a cap that could silence an escalation would be the
    catch-all rule problem re-introduced one layer down.
 4. **Never more than [Attention.Allowed] in a rolling day**, which is lower for
    somebody who has just come back.
 5. **A family the player keeps walking away from is slowed**, by
    [Attention.Cooling], and never stopped.
 6. **The last release of the day goes to something that would serve them.**
    When the day's allowance is down to its final slot, only a candidate whose
    family answers a lagging factor takes it. This is the whole of "what would
    serve the player next": not a ranking, not a recommendation, not a nudge —
    one decision about which of two ordinary moments happens, made where nobody
    can see it.

What it deliberately does not do is reach for anything else. Everything above is
a function of [Attention], which is a value; a policy that could look something
up would eventually look something up.
*/
type Attentive struct{}

func (Attentive) Release(a Attention) bool {
	switch {
	case a.Standing:
		return false
	case a.Since < Quiet:
		return false
	case a.Raised:
		return true
	case a.Released >= a.Allowed():
		return false
	case a.SinceKind < a.Cooling():
		return false
	case a.Released >= a.Allowed()-1 && !a.Serves():
		return false
	default:
		return true
	}
}

/*
Steady is the floor: one at a time, [Daily] in a day, [Quiet] apart.

Not bound in the server — [Attentive] is — and kept because it is the honest
statement of what a release policy has to do *before* it is allowed to have an
opinion, and because a test about the backlog rather than about the heuristic
wants a policy with no opinions in it.
*/
type Steady struct{}

func (Steady) Release(a Attention) bool {
	switch {
	case a.Standing:
		return false
	case a.Released >= Daily:
		return false
	case a.Since < Quiet:
		return false
	default:
		return true
	}
}

// Held is a policy that never releases anything. For tests about the backlog
// rather than about the release, and for a server that wants the world quiet.
type Held struct{}

func (Held) Release(Attention) bool { return false }

// Always is a policy with no restraint at all. Tests only — bound in a server
// it would empty the backlog onto the first walk of the day.
type Always struct{}

func (Always) Release(Attention) bool { return true }
