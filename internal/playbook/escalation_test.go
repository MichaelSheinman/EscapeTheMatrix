package playbook

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

/*
Escalation, against a real Postgres (§14).

One paragraph of technical.md §4 is the whole specification and the second
sentence of it is the one worth writing tests about: "escalating only on 'no
match' lets one catch-all rule silence the game forever". That is a claim about
a *player strategy*, not about a function, so the tests below play the strategy.
*/

// silent is a moment nothing in force covers.
func silent(t *testing.T, ctx context.Context, tx pgx.Tx, m *Playbooks, account string, at time.Time) Fire {
	t.Helper()
	d, err := m.Weigh(ctx, tx, account, "downtown", aFavour(9000))
	if err != nil {
		t.Fatalf("Weigh: %v", err)
	}
	written, err := m.Record(ctx, tx, account, at, "downtown", d, Pending)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	return written
}

// clashing is a moment two rules both reach and disagree about.
func clashing(t *testing.T, ctx context.Context, tx pgx.Tx, m *Playbooks, account string, at time.Time) Fire {
	t.Helper()
	answered(t, ctx, tx, m, account, "east-side", "help", Sentence{
		When: []Ref{{Term: "legal"}}, Join: And, Then: Ref{Term: "yes"}, Else: Ref{Term: "no"},
	})
	answered(t, ctx, tx, m, account, "east-side", "overtime", Sentence{
		When: []Ref{{Term: "already"}}, Join: And, Then: Ref{Term: "refuse"}, Else: Ref{Term: "accept"},
	})
	d, err := m.Weigh(ctx, tx, account, "east-side",
		Arises("a-late-favour", "help", "overtime").
			So("help/condition/legal", true).
			So("overtime/condition/already", true))
	if err != nil {
		t.Fatalf("Weigh: %v", err)
	}
	written, err := m.Record(ctx, tx, account, at, "east-side", d, Pending)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	return written
}

/*
All three of §4's conditions escalate, and the third one only when it is
answered.

The ordering matters as much as the set. A moment that was silent escalated the
instant it was recorded; the same moment settling badly later is not a second
escalation, because it is the same conflict with more known about it.
*/
func TestEscalationFiresOnAllThreeConditions(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		// One: nothing matched.
		nothing := silent(t, ctx, tx, m, account, noon)
		if nothing.Cause != Unhandled {
			t.Errorf("an unmatched moment escalated as %q, want %q", nothing.Cause, Unhandled)
		}

		// Two: two rules disagreed.
		both := clashing(t, ctx, tx, m, account, noon.Add(time.Hour))
		if both.Cause != Contested {
			t.Errorf("a disagreement escalated as %q, want %q", both.Cause, Contested)
		}

		// Three: a rule fired and the outcome was poor. Not known when it was
		// written, which is the whole reason the condition is separate.
		d, err := m.Weigh(ctx, tx, account, "east-side", aFavour(9000))
		if err != nil {
			t.Fatalf("Weigh: %v", err)
		}
		fired, err := m.Record(ctx, tx, account, noon.Add(2*time.Hour), "east-side", d, Pending)
		if err != nil {
			t.Fatalf("Record: %v", err)
		}
		if fired.Verdict != Answered || fired.Cause != "" {
			t.Fatalf("a rule that fired escalated on the spot: %+v", fired)
		}

		badly, escalated, err := m.Settle(ctx, tx, account, fired.ID, noon.Add(72*time.Hour), Poor)
		if err != nil {
			t.Fatalf("Settle: %v", err)
		}
		if !escalated || badly.Cause != Backfired {
			t.Fatalf("a poor outcome did not escalate: %+v, %v", badly, escalated)
		}
		if badly.Situation != "a-favour" || badly.Environment != "east-side" {
			t.Errorf("escalation lost the moment it came from: %+v", badly)
		}

		// A good outcome is not news, and neither is settling a moment that was
		// already escalating for another reason.
		good, err := m.Weigh(ctx, tx, account, "east-side", aFavour(9000))
		if err != nil {
			t.Fatalf("Weigh: %v", err)
		}
		well, err := m.Record(ctx, tx, account, noon.Add(3*time.Hour), "east-side", good, Pending)
		if err != nil {
			t.Fatalf("Record: %v", err)
		}
		if _, again, err := m.Settle(ctx, tx, account, well.ID, noon.Add(96*time.Hour), Good); err != nil || again {
			t.Errorf("a good outcome escalated: %v, %v", again, err)
		}
		if _, again, err := m.Settle(ctx, tx, account, nothing.ID, noon.Add(96*time.Hour), Poor); err != nil || again {
			t.Errorf("an already-escalating moment escalated twice: %v, %v", again, err)
		}
	})
}

/*
The sentence the whole three-condition rule exists for.

The strategy under test is the one a player actually reaches for: write one rule
broad enough that it answers everything, and never be asked anything again. Under
a one-condition escalation it works — nothing is unmatched, so nothing escalates,
and the game goes quiet for good.

What this asserts is that it does not work here. The catch-all answers every
moment, so `silent` and `clash` never happen again; and every one of those
answers that turns out badly comes back, which is more escalation than the
player was getting before rather than less.
*/
func TestACatchAllRuleCannotSilenceTheGame(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		// `when legal then yes otherwise no` answers every favour that states
		// whether it is legal, however the fee reads: there is no moment it
		// leaves unanswered and no second rule for it to argue with.
		answered(t, ctx, tx, m, account, "east-side", "help", Sentence{
			When: []Ref{{Term: "legal"}}, Join: And, Then: Ref{Term: "yes"}, Else: Ref{Term: "no"},
		})

		var poor int
		for i := range 5 {
			at := noon.Add(time.Duration(i) * 24 * time.Hour)
			d, err := m.Weigh(ctx, tx, account, "east-side", aFavour(int64(1000*(i+1))))
			if err != nil {
				t.Fatalf("Weigh: %v", err)
			}
			if d.Verdict != Answered || d.Escalates() {
				t.Fatalf("day %d: the catch-all left a gap: %+v", i, d.Verdict)
			}
			written, err := m.Record(ctx, tx, account, at, "east-side", d, Pending)
			if err != nil {
				t.Fatalf("Record: %v", err)
			}
			if written.Cause != "" {
				t.Fatalf("day %d escalated before anything came of it", i)
			}

			// The rule is right about some rooms and wrong about others (§3.15).
			outcome := Good
			if i%2 == 0 {
				outcome = Poor
			}
			_, escalated, err := m.Settle(ctx, tx, account, written.ID, at.Add(6*time.Hour), outcome)
			if err != nil {
				t.Fatalf("Settle: %v", err)
			}
			if escalated != (outcome == Poor) {
				t.Errorf("day %d settled %q and escalated = %v", i, outcome, escalated)
			}
			if escalated {
				poor++
			}
		}

		if poor == 0 {
			t.Fatal("a week of a catch-all rule produced no escalation at all")
		}
		waiting, err := m.Unraised(ctx, tx, account, 10)
		if err != nil {
			t.Fatalf("Unraised: %v", err)
		}
		if len(waiting) != poor {
			t.Fatalf("%d escalations waiting, want the %d that went badly", len(waiting), poor)
		}
		for _, e := range waiting {
			if e.Cause != Backfired {
				t.Errorf("a catch-all escalated as %q; the only condition it can meet is %q",
					e.Cause, Backfired)
			}
		}
	})
}

/*
Unraised is oldest first and capped by the caller — §4's "a long absence does not
dump the backlog", applied to the half of it that is about the player's own
mistakes.

Somebody away a fortnight while a rule went wrong every morning is owed a
conversation about it, not fourteen.
*/
func TestEscalationsQueueUpOldestFirstAndAreHandedOverAFewAtATime(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		var made []Fire
		for i := range 4 {
			made = append(made, silent(t, ctx, tx, m, account, noon.Add(time.Duration(i)*24*time.Hour)))
		}

		none, err := m.Unraised(ctx, tx, account, 0)
		if err != nil || len(none) != 0 {
			t.Fatalf("asking for none returned %d: %v", len(none), err)
		}

		first, err := m.Unraised(ctx, tx, account, 1)
		if err != nil {
			t.Fatalf("Unraised: %v", err)
		}
		if len(first) != 1 || first[0].Fire != made[0].ID {
			t.Fatalf("the cap handed back %+v, want only the oldest (%d)", first, made[0].ID)
		}
		if first[0].Note != "nothing in your playbook covered this." {
			t.Errorf("note %q; an escalation carries the reason in the log's own words", first[0].Note)
		}

		// Raising it takes it out of the queue, once and for one caller.
		mine, err := m.Raised(ctx, tx, account, made[0].ID, noon.Add(time.Minute))
		if err != nil || !mine {
			t.Fatalf("Raised: %v, %v", mine, err)
		}
		again, err := m.Raised(ctx, tx, account, made[0].ID, noon.Add(2*time.Minute))
		if err != nil || again {
			t.Fatalf("raising the same escalation twice: %v, %v", again, err)
		}

		rest, err := m.Unraised(ctx, tx, account, 10)
		if err != nil {
			t.Fatalf("Unraised: %v", err)
		}
		if len(rest) != 3 || rest[0].Fire != made[1].ID {
			t.Fatalf("after raising one, %d wait and the first is %d", len(rest), rest[0].Fire)
		}

		// And the log says it was put to them, without changing what happened.
		log, err := m.Fires(ctx, tx, account, 1)
		if err != nil {
			t.Fatalf("Fires: %v", err)
		}
		for _, f := range log.Fires {
			if f.Cause != Unhandled {
				t.Errorf("record %d reads as %q", f.ID, f.Cause)
			}
			if raised := !f.RaisedAt.IsZero(); raised != (f.ID == made[0].ID) {
				t.Errorf("record %d raised = %v", f.ID, raised)
			}
		}
	})
}

// The cause is derived from what happened, in one place, and the order of the
// three conditions is what makes "why" one answer rather than a set.
func TestTheCauseIsReadOffWhatHappened(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		verdict Verdict
		outcome Outcome
		want    Cause
		is      bool
	}{
		{Answered, Pending, "", false},
		{Answered, Good, "", false},
		{Answered, Mixed, "", false},
		{Answered, Poor, Backfired, true},
		{Silent, Pending, Unhandled, true},
		{Silent, Poor, Unhandled, true},
		{Clash, Pending, Contested, true},
		{Clash, Poor, Contested, true},
	} {
		got, is := Because(tc.verdict, tc.outcome)
		if got != tc.want || is != tc.is {
			t.Errorf("Because(%q, %q) = %q, %v; want %q, %v",
				tc.verdict, tc.outcome, got, is, tc.want, tc.is)
		}
	}
}
