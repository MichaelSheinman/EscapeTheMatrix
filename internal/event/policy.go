package event

import "time"

/*
Whether the world should surface something *now*.

technical.md §4: "**Release is a server decision, not a clock event** — based on
activity, time away, and what would serve the player next." That sentence is a
whole ticket of its own (#22), and this file is the seam it lands in: an
interface, a small honest implementation bound to it today, and a struct
carrying the facts a policy is allowed to decide from.

Naming it here rather than leaving it out is the same decision internal/rollover
took with its World and internal/character with its Encounters. A backlog that
releases whatever it finds is indistinguishable, from the outside, from one that
chose — and the difference is a ticket.

**What #22 will want that is not here yet.** [Attention] carries what this
module can answer out of its own tables (§3.1): whether something is already on
screen, how much has been released lately, and — the field §4 asks for by name —
how often this player has walked away from this kind of thing. Time away and
"what would serve the player next" need internal/account's activity and
whatever the diagnosis surface (§3.6) turns out to be, which means widening this
struct and binding a second interface. Widening it is a change to one struct and
one implementation, which is why it is a struct rather than six arguments.
*/

/*
Attention is what the world knows about how much of the player's attention it
has already spent.

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
	// Since is how long since the last release. [Never] when there has not
	// been one, which compares greater than every real gap.
	Since time.Duration

	// Kind is the family of the event that would be released.
	Kind Kind
	// Offered and Passed are how many of that family have been put to this
	// player, and how many of those they walked away from, inside [Lately].
	//
	// This is §4's signal, and it is deliberately handed to the policy as a
	// pair rather than as a ratio: "declined four of four" and "declined four
	// of forty" are different sentences and only one of them means the player
	// is routing around something.
	Offered int
	Passed  int
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
Around is the window [Steady] counts releases over, and [Lately] the window the
decline signal is counted over.

A rolling day rather than a calendar one, so a policy needs no timezone: the
player's local midnight is internal/rollover's business, and "four things since
this time yesterday" is the same sentence everywhere on earth.
*/
const (
	Around = 24 * time.Hour
	Lately = 14 * 24 * time.Hour
)

/*
Steady is the policy the server runs until #22 replaces it.

Three rules, all of them floors rather than heuristics:

  - never a second offer while one is standing;
  - never more than [Daily] in a rolling day;
  - never two inside [Quiet] of each other.

What it deliberately does *not* do is act on the decline signal, on time away,
or on what would serve the player next. §4 asks for all three and this answers
none of them — which is honest, and is visible here rather than in a backlog:
the fields are on [Attention], unread, waiting for the ticket that reads them.
*/
type Steady struct{}

const (
	// Daily is the most Steady will surface in a rolling day.
	//
	// §4 caps what a long absence dumps on somebody, and this is that cap in
	// its simplest form: a fortnight away accrues a deep backlog, and coming
	// back gets you four things rather than forty.
	Daily = 4

	// Quiet is the least time between two releases.
	//
	// Walking across the city is several steps and every one of them is a
	// chance (§3.12). Without this, a single journey would empty the backlog
	// into one screen — which is the failure mode of a world that has plenty
	// to say and no sense of when to say it.
	Quiet = 30 * time.Minute
)

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
