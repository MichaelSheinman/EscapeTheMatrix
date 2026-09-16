package event

import "time"

/*
Whether the world should surface something *now*.

technical.md §4: "**Release is a server decision, not a clock event**", and
"there is a cap on what gets surfaced, and the notification box is where the
server makes recommendations."

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
Attention is what the world knows about what player is paying attention to, in order to determine what would be worthwhile next.
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
	Since time.Duration

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
	// Around is the window releases are counted over. A rolling day rather than
	// a calendar one, so a policy needs no timezone.
	Around = 24 * time.Hour

	// Daily is the most that will be surfaced in a rolling day.
	Daily = 4

	// Quiet is the least time between two releases.
	Quiet = 30 * time.Minute
)

/*
Attentive is the release policy the server runs (§4). Four rules, in the order
they are asked:

 1. Never a second offer while one is standing.
 2. Never two inside [Quiet] of each other, whatever else is true. Above the
    escalation exemption on purpose: a conflict the playbook could not handle is
    urgent, and urgent does not mean "while they are still reading the last one".
 3. An escalation goes through, past the daily cap. §4's escalation rule exists
    so a playbook with a hole in it cannot be quiet, and a cap that could silence
    one would re-introduce the catch-all rule problem.
 4. Never more than [Daily] in a rolling day.

Everything above is a function of [Attention], which is a value, so nothing here
can look anything up.
*/
type Attentive struct{}

func (Attentive) Release(a Attention) bool {
	switch {
	// One at a time, and this one is above the escalation. A second offer on
	// top of the one on screen is not a release policy however urgent the
	// second one is — the player would answer the wrong thing.
	case a.Standing:
		return false
	// Everything below this line is the world's sense of timing, and an
	// escalation is exempt from all of it. It sits above [Quiet] because a quiet
	// period that could swallow an escalation would let one catch-all rule
	// silence the game (§4).
	case a.Raised:
		return true
	case a.Since < Quiet:
		return false
	case a.Released >= Daily:
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
