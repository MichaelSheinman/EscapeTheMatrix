package event

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/clock"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
Escalation and the facts the release policy is decided from, against a real
Postgres (§14).

policy_test.go proves what [Attentive] does with an [Attention]. What is left is
the half a pure test cannot reach: that the [Attention] it is handed is the
truth, and that an escalation arrives the one way it is allowed to.
*/

// escalation is the authored event the world raises when a moment escalated
// because nothing in the playbook covered it. Named here so a test breaks when
// the content is renamed rather than quietly stopping testing anything.
const escalation = "nothing-covered-it"

// asked is a Policy that remembers what it was asked and answers yes.
type asked struct {
	last Attention
	seen int
}

func (a *asked) Release(at Attention) bool {
	a.last, a.seen = at, a.seen+1
	return true
}

// elsewhere is a Bearings a test sets by hand: how long they had been away and
// what they are behind on.
type elsewhere struct {
	away    time.Duration
	lagging []simulation.Factor
}

func (e *elsewhere) Away(context.Context, pgx.Tx, string) (time.Duration, error) {
	return e.away, nil
}

func (e *elsewhere) Lagging(context.Context, pgx.Tx, string) ([]simulation.Factor, error) {
	return e.lagging, nil
}

/*
An escalation is never generated. It arrives because internal/playbook decided it
had to, and by no other route.

The failure this rules out is the one that would be hardest to see: the world
inventing a conflict the player's rules never had an answer to, which is
everything the word "escalation" means, invented.
*/
func TestAnEscalationIsNeverGenerated(t *testing.T) {
	world(t, func(ctx context.Context, tx pgx.Tx, e *Events, _ *clock.Stepping, who string, _ *wallet) {
		// Fill it several times over, clearing between, so "it just did not fit"
		// is not an available explanation.
		for range 5 {
			if err := e.Replenish(ctx, tx, who, today); err != nil {
				t.Fatalf("replenish: %v", err)
			}
			if _, err := tx.Exec(ctx,
				`UPDATE event_backlog SET state = 'done', settled_at = now() WHERE account_id = $1`, who,
			); err != nil {
				t.Fatalf("clear the backlog: %v", err)
			}
		}

		var raised int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM event_backlog
			 WHERE account_id = $1 AND event = ANY($2)`, who, raisedKeys(),
		).Scan(&raised); err != nil {
			t.Fatalf("count the raised events: %v", err)
		}
		if raised != 0 {
			t.Errorf("the generator produced %d escalation(s) nobody escalated", raised)
		}
	})
}

/*
Raise is the only door, it is refused to anything that is not an escalation, and
raising the same one twice is one escalation.
*/
func TestOnlyAnEscalationCanBeRaised(t *testing.T) {
	world(t, func(ctx context.Context, tx pgx.Tx, e *Events, _ *clock.Stepping, who string, _ *wallet) {
		if _, err := e.Raise(ctx, tx, who, "somebody-asking-for-directions"); !errors.Is(err, ErrNotRaised) {
			t.Errorf("raising an ordinary event: %v, want %v", err, ErrNotRaised)
		}
		if _, err := e.Raise(ctx, tx, who, "no-such-event"); !errors.Is(err, ErrNotRaised) {
			t.Errorf("raising nothing at all: %v, want %v", err, ErrNotRaised)
		}

		queued, err := e.Raise(ctx, tx, who, escalation)
		if err != nil || !queued {
			t.Fatalf("Raise: %v, %v", queued, err)
		}
		again, err := e.Raise(ctx, tx, who, escalation)
		if err != nil || again {
			t.Errorf("raising the same conflict twice queued it twice: %v, %v", again, err)
		}
	})
}

/*
An escalation outranks whatever else the world had ready, and it does so past the
backlog's depth.

§4's depth caps what is *generated*; a full backlog is not a reason for a
conflict the player's own rules could not handle to go unmentioned. And it beats
weight rather than carrying a large one, because weight is an authoring
judgement between things the world thought of and this is not one of those.
*/
func TestAnEscalationIsOfferedBeforeAnythingElseWaiting(t *testing.T) {
	world(t, func(ctx context.Context, tx pgx.Tx, e *Events, _ *clock.Stepping, who string, _ *wallet) {
		if err := e.Replenish(ctx, tx, who, today); err != nil {
			t.Fatalf("replenish: %v", err)
		}
		if _, err := e.Raise(ctx, tx, who, escalation); err != nil {
			t.Fatalf("Raise: %v", err)
		}

		var waiting int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM event_backlog WHERE account_id = $1 AND state <> 'done'`, who,
		).Scan(&waiting); err != nil {
			t.Fatalf("count the backlog: %v", err)
		}
		if waiting != Depth+1 {
			t.Errorf("the backlog holds %d; an escalation goes in past the depth", waiting)
		}

		offer := standingAt(t, ctx, e, tx, who, "east-side")
		if offer == nil {
			t.Fatal("nothing was offered with an escalation waiting")
		}
		if offer.Event.Key != escalation {
			t.Errorf("the world offered %q with an escalation waiting", offer.Event.Key)
		}
		if !offer.Event.Raised {
			t.Errorf("%q was offered as an escalation and is not one", offer.Event.Key)
		}
	})
}

/*
The policy is asked with the truth.

Every field on [Attention] is a fact about the player, and a heuristic decided
from a fact that was quietly wrong would be indistinguishable, from the outside,
from a heuristic that was wrong. So this walks a player into an offer, has them
walk away from it, and then reads back what the policy was told the second time.
*/
func TestThePolicyIsAskedWithWhatTheWorldKnows(t *testing.T) {
	t.Parallel()
	policy := &asked{}
	bearings := &elsewhere{away: 9 * 24 * time.Hour, lagging: []simulation.Factor{simulation.Kindness}}

	worldKnowing(t, policy, bearings, func(ctx context.Context, tx pgx.Tx, e *Events, clk *clock.Stepping, who string, _ *wallet) {
		if err := e.Replenish(ctx, tx, who, today); err != nil {
			t.Fatalf("replenish: %v", err)
		}

		if first := standingAt(t, ctx, e, tx, who, "east-side"); first == nil {
			t.Fatal("nothing was offered by a policy that says yes to everything")
		}
		if policy.last.Since != Never || policy.last.SinceKind != Never {
			t.Errorf("the first release of a player's life reported a previous one: %+v", policy.last)
		}
		if policy.last.Away != bearings.away {
			t.Errorf("time away reached the policy as %v, want %v", policy.last.Away, bearings.away)
		}
		if len(policy.last.Lagging) != 1 || policy.last.Lagging[0] != simulation.Kindness {
			t.Errorf("what would serve reached the policy as %v", policy.last.Lagging)
		}
		if policy.last.Raised {
			t.Error("an ordinary event was put to the policy as an escalation")
		}

		// Let it lapse, decline the next one, and come back in the morning.
		// Two things were put to this player and they answered neither.
		clk.At = clk.At.Add(Window + time.Minute)
		second := standingAt(t, ctx, e, tx, who, "east-side")
		if second == nil {
			t.Fatal("nothing was offered after the first lapsed")
		}
		if _, err := e.Decline(ctx, tx, who, second.ID); err != nil {
			t.Fatalf("decline: %v", err)
		}

		clk.At = clk.At.Add(8 * time.Hour)
		if _, err := e.Arrived(ctx, tx, who, "east-side"); err != nil {
			t.Fatalf("walk back: %v", err)
		}

		switch {
		case policy.last.Released != 2:
			t.Errorf("%d released in the last day, want the two they did not answer", policy.last.Released)
		case policy.last.Since == Never || policy.last.Since < 8*time.Hour:
			t.Errorf("the gap since the last release read %v", policy.last.Since)
		case policy.last.SinceKind < policy.last.Since:
			t.Errorf("the last of this family (%v) is more recent than the last of anything (%v)",
				policy.last.SinceKind, policy.last.Since)
		case policy.last.Offered != policy.last.Passed:
			t.Errorf("§4's signal reads %d of the %q family offered and %d passed; both were passed",
				policy.last.Offered, policy.last.Kind, policy.last.Passed)
		}
		if policy.last.Offered == 0 && policy.last.SinceKind != Never {
			t.Error("a family with nothing offered reported a last release")
		}
	})
}

// An escalation reaches the policy marked as one, which is what earns it the
// exemption from the caps.
func TestAnEscalationReachesThePolicyAsOne(t *testing.T) {
	t.Parallel()
	policy := &asked{}
	worldKnowing(t, policy, Here{}, func(ctx context.Context, tx pgx.Tx, e *Events, _ *clock.Stepping, who string, _ *wallet) {
		if _, err := e.Raise(ctx, tx, who, escalation); err != nil {
			t.Fatalf("Raise: %v", err)
		}
		if offer := standingAt(t, ctx, e, tx, who, "east-side"); offer == nil {
			t.Fatal("a raised escalation was not offered")
		}
		if !policy.last.Raised {
			t.Error("an escalation was put to the policy as an ordinary moment")
		}
	})
}

// raisedKeys is every authored escalation, read off the catalogue rather than
// listed — a fourth one added to content is one this test covers for free.
func raisedKeys() []string {
	out := []string{}
	for _, e := range All() {
		if e.Raised {
			out = append(out, e.Key)
		}
	}
	return out
}
