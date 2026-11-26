package event

import "time"

// [Claude fable5] Whether the world should surface something now. [Attention]
// is the facts a policy may decide from, [Policy] is the decision, and
// [Attentive] is the one the server runs.

// [Claude fable5] Attention is what the world knows about what the player is
// paying attention to.
type Attention struct {
	// [Claude fable5] Now is the instant being decided about, off the injected
	// clock.
	Now time.Time
	// [Claude fable5] Standing is whether an offer is already on screen.
	Standing bool
	// [Claude fable5] Released is how many events have been surfaced in the
	// last [Around].
	Released int
	// [Claude fable5] Since is how long since the last one was done with —
	// answered, walked away from, or lapsed. [Never] when there has not been
	// one, which compares greater than every real gap.
	Since time.Duration

	// [Claude fable5] Raised is whether this candidate is there because
	// something escalated to the player rather than because the world generated
	// it.
	Raised bool
}

// [Claude fable5] Never is the gap since a release that has never happened.
// Larger than any interval a policy will compare it against.
const Never = time.Duration(1 << 62)

// [Claude fable5] Policy decides whether to surface a candidate the backlog has
// already found placeable and re-validated. It runs after the placement and
// precondition checks, so the worst it can say is "not now". Pure, and it takes
// no context and no transaction.
type Policy interface {
	Release(a Attention) bool
}

// [Claude fable5] The windows and the caps. Constants rather than
// configuration.
const (
	// [Claude fable5] Around is the window releases are counted over. A rolling
	// day rather than a calendar one, so a policy needs no timezone.
	Around = 24 * time.Hour

	// [Claude fable5] Daily is the most that will be surfaced in a rolling day.
	Daily = 4

	// [Claude fable5] Quiet is the least time between two releases.
	Quiet = 30 * time.Minute
)

// [Claude fable5] Attentive is the release policy the server runs.
type Attentive struct{}

func (Attentive) Release(a Attention) bool {
	switch {
	// [Claude fable5] One at a time, and this one is above the escalation.
	case a.Standing:
		return false
	// [Claude fable5] Everything below this line is the world's sense of
	// timing, and an escalation is exempt from all of it.
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
