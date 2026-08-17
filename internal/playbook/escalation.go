package playbook

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

/*
Escalation: when the playbook does not handle something, it comes to the player.
Three conditions rather than one (§4) — no rule matches, two rules conflict, or a
rule fired and the outcome was poor.

The third is the load-bearing one. The cheapest way to make a demanding game
quiet is one rule broad enough to answer everything: when anyone asks then say
yes. Under a one-condition rule that player never sees another live decision —
nothing is unmatched, nothing conflicts, and the game turns into a log they
scroll past.

What that player has done is commit to a strategy, and every strategy has a
context it fails in (§3.15). So the third condition makes a catch-all rule more
likely to escalate rather than less: a rule that answers everything answers some
things badly, and every one of those comes back.

The cause is derived rather than stored. verdict carries the first two conditions
and outcome the third, so a stored cause would be a fourth copy free to disagree
with them. What is stored is raised_at — whether the world has acted on it —
which is the one thing not derivable from what happened.

This package delivers nothing. Escalating is a judgement about a playbook and
surfacing is §4's release policy, which internal/event owns, so what leaves here
is a list of moments and the reason each is on it.
*/

/*
Cause is why a moment came to the player.

Three, exactly, and named for what happened rather than for the mechanism: a
player reading their log should be able to tell "I had no answer for this" from
"my answers argued" from "my answer made it worse" without knowing what a
verdict is.
*/
type Cause string

const (
	// Unhandled is nothing in force covering the moment (verdict `silent`).
	Unhandled Cause = "unhandled"
	// Contested is two rules that both applied and disagreed (verdict `clash`).
	Contested Cause = "contested"
	// Backfired is a rule that fired and settled poorly. The condition a
	// catch-all rule cannot dodge.
	Backfired Cause = "backfired"
)

/*
Escalation is one moment on its way to the player.

It carries what the moment was and why it is here, and nothing about what to do
about it — because what to do about it is not this package's decision and the
thing that makes it needs no more than this.
*/
type Escalation struct {
	// Fire is the record this came from. What [Playbooks.Raised] names.
	Fire int64
	At   time.Time
	// Environment and Situation are where and what, by key — the same two
	// columns the record keeps, and for the same reason (§3.15).
	Environment string
	Situation   string
	Cause       Cause
	// Note is the reason in the words the log uses, empty for [Backfired]:
	// a rule that went badly has no sentence to write about the matching,
	// because the matching was fine.
	Note string
	// Says is the rule that fired, as the sentence the player wrote it as, and
	// empty when no single rule did — a moment nothing matched has no sentence
	// to name, and a clash has two. It is here so that the event which
	// interrupts somebody can say which of their own rules did this
	// (2026-08-10): the alternative was an escalation true of every escalation
	// at once, which is what "the room was not the room the rule was written
	// for" was.
	Says string
}

/*
Because reads the cause off what happened, and reports whether anything did.

The single definition of §4's three conditions in Go. The partial index in
`00016_release.sql` states the same predicate in SQL, and the two are meant to be
read together — a fourth condition added to one and not the other would be a
kind of escalation the world could decide on and never find.
*/
func Because(verdict Verdict, outcome Outcome) (Cause, bool) {
	switch {
	case verdict == Silent:
		return Unhandled, true
	case verdict == Clash:
		return Contested, true
	case outcome == Poor:
		// Deliberately not `verdict == Answered && outcome == Poor`. A moment
		// that was silent *and* went badly is already escalating for the first
		// reason, and the order of this switch is what makes "why" one answer
		// rather than a set.
		return Backfired, true
	default:
		return "", false
	}
}

/*
Escalation is the live half of the three conditions: what the matcher already
knows, before anything has been carried out.

The third condition cannot be here — a rule that fired has not yet gone
anywhere — which is why [Playbooks.Settle] returns one too.
*/
func (d Decision) Escalation() (Cause, bool) { return Because(d.Verdict, Pending) }

/*
Unraised is this player's escalations that have not been put to them, oldest
first.

Oldest first, and capped by the caller, because the point of the cap is §4's:
"a long absence does not dump the backlog". A player who was away a fortnight
while a catch-all rule went wrong every day has fourteen of these, and handing
them all over at once would be the same failure as emptying the backlog — with
the extra insult that every one of them is about something the player already
got wrong.

It reads the same three conditions [Because] does, as a predicate the partial
index matches exactly.
*/
func (m *Playbooks) Unraised(
	ctx context.Context, tx pgx.Tx, accountID string, most int,
) ([]Escalation, error) {
	if most <= 0 {
		return []Escalation{}, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT id, at, environment, situation, verdict, outcome
		  FROM playbook_fire
		 WHERE account_id = $1
		   AND raised_at IS NULL
		   AND (verdict IN ('silent', 'clash') OR outcome = 'poor')
		 ORDER BY at, id
		 LIMIT $2`, accountID, most)
	if err != nil {
		return nil, fmt.Errorf("playbook: read escalations: %w", err)
	}
	defer rows.Close()

	out := []Escalation{}
	for rows.Next() {
		var (
			e       Escalation
			verdict string
			outcome string
		)
		if err := rows.Scan(&e.Fire, &e.At, &e.Environment, &e.Situation, &verdict, &outcome); err != nil {
			return nil, fmt.Errorf("playbook: read escalations: %w", err)
		}
		cause, ok := Because(Verdict(verdict), Outcome(outcome))
		if !ok {
			// The index predicate and [Because] disagreed, which is a bug in
			// one of them rather than a state a player can reach. Skipping is
			// the honest answer: a moment nobody can say the cause of is not
			// one the world should put to somebody.
			continue
		}
		e.Cause = cause
		e.Note = Decision{Verdict: Verdict(verdict)}.Note()
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("playbook: read escalations: %w", err)
	}

	// Which rule did it, for the one cause that has a single answer to that.
	// Read after the rows are done rather than inside the loop: the connection
	// is busy until this cursor closes, and a second query on it mid-scan is
	// the deadlock that shape of code always is.
	ids := make([]int64, 0, len(out))
	for _, e := range out {
		if e.Cause == Backfired {
			ids = append(ids, e.Fire)
		}
	}
	weighed, err := m.considered(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	for i, e := range out {
		for _, c := range weighed[e.Fire] {
			if c.State == Fired {
				out[i].Says = c.Says
				break
			}
		}
	}
	return out, nil
}

/*
Raised marks an escalation as one the world has acted on.

Once. The `raised_at IS NULL` in the predicate is what makes two tabs, or two
rollovers racing each other, raise one conflict between them rather than one
each — and it reports whether this call was the one that did it, so the caller
knows whether it owns what it just queued.
*/
func (m *Playbooks) Raised(
	ctx context.Context, tx pgx.Tx, accountID string, fire int64, at time.Time,
) (bool, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE playbook_fire SET raised_at = $3
		 WHERE id = $1 AND account_id = $2 AND raised_at IS NULL`,
		fire, accountID, at)
	if err != nil {
		return false, fmt.Errorf("playbook: raise escalation: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}
