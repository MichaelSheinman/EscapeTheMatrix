package playbook

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

/*
The records, against a real Postgres (§14).

The matcher itself is proved in match_test.go with nothing running. What is left
to prove here is the half that only exists because the player is not present: a
rule decides in the environment they are standing in and in no other, what
happened is written down precisely enough to be argued with, editing the rule
afterwards does not rewrite it, and a week of it adds up to something.
*/

// noon is a fixed instant to file records under. The module takes the caller's
// clock (§3.1), so a test decides what day it is.
var noon = time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)

// answered composes a rule so there is something in force to fire.
func answered(t *testing.T, ctx context.Context, tx pgx.Tx, m *Playbooks, account, environment, question string, s Sentence) {
	t.Helper()
	if _, err := m.Compose(ctx, tx, account, environment, question, s); err != nil {
		t.Fatalf("compose %s/%s: %v", environment, question, err)
	}
}

// aFavour is a moment somebody asks for help, paid, and legal.
func aFavour(cents int64) Situation {
	return Arises("a-favour", "help").
		Reading("help/condition/fee", cents).
		So("help/condition/legal", true)
}

/*
The ticket's last line, and §3.15's whole mechanism: only the active
environment's rule set is considered.

The failure this rules out is the quiet one — a matcher that fell back to another
set, or to all of them, would make "your old answers wait for you somewhere new"
into a sentence with no code behind it.
*/
func TestOnlyTheRuleSetWhereYouAreStandingDecides(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		answered(t, ctx, tx, m, account, "east-side", "help", startingHelp())

		here, err := m.Weigh(ctx, tx, account, "east-side", aFavour(9000))
		if err != nil {
			t.Fatalf("Weigh: %v", err)
		}
		if here.Verdict != Answered {
			t.Errorf("at home the verdict is %q, want %q", here.Verdict, Answered)
		}

		away, err := m.Weigh(ctx, tx, account, "downtown", aFavour(9000))
		if err != nil {
			t.Fatalf("Weigh: %v", err)
		}
		if away.Verdict != Silent {
			t.Errorf("somewhere else the verdict is %q; the rule was composed for east-side", away.Verdict)
		}
		if c := away.Considered[0]; c.State != Unanswered {
			t.Errorf("state %q; an unanswered question elsewhere is a gap, not a silence", c.State)
		}

		if _, err := m.Weigh(ctx, tx, account, "atlantis", aFavour(9000)); !errors.Is(err, ErrNoSuchEnvironment) {
			t.Errorf("weighing somewhere that is not an environment: %v", err)
		}
	})
}

// A moment stating something the vocabulary cannot read is refused at the seam.
// The matcher would survive it; an authoring mistake that silently stopped
// applying somebody's rules would not be survivable for the player.
func TestAMomentThisVocabularyCannotReadIsRefused(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		answered(t, ctx, tx, m, account, "east-side", "help", startingHelp())

		_, err := m.Weigh(ctx, tx, account, "east-side",
			Arises("a-favour", "help").So("help/condition/nowhere", true))
		if !errors.Is(err, ErrNoSuchFact) {
			t.Errorf("Weigh with an unreadable fact: %v, want %v", err, ErrNoSuchFact)
		}
	})
}

/*
The ticket's second line: the record knows which rule matched, which action
fired, and what came of it.

Read back through Fires, because a record nobody can read is a write.
*/
func TestTheRecordSaysWhichConditionMatchedAndWhatWasDone(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		answered(t, ctx, tx, m, account, "east-side", "help", Sentence{
			When: []Ref{{Term: "fee", Value: "good"}, {Term: "legal"}},
			Join: And,
			Then: Ref{Term: "yes"},
			Else: Ref{Term: "no"},
		})

		// Paid under the threshold, and legal: one of two conditions.
		d, err := m.Weigh(ctx, tx, account, "east-side", aFavour(500))
		if err != nil {
			t.Fatalf("Weigh: %v", err)
		}
		written, err := m.Record(ctx, tx, account, noon, "east-side", d, Poor)
		if err != nil {
			t.Fatalf("Record: %v", err)
		}
		if written.ID == 0 {
			t.Fatal("Record returned no id; there is nothing to settle later")
		}

		log, err := m.Fires(ctx, tx, account, 1)
		if err != nil {
			t.Fatalf("Fires: %v", err)
		}
		if log.Total != 1 || len(log.Fires) != 1 {
			t.Fatalf("%d records in the log, want 1", log.Total)
		}
		f := log.Fires[0]
		if f.Situation != "a-favour" || f.Environment != "east-side" || f.Verdict != Answered {
			t.Errorf("record = %+v", f)
		}
		if f.Outcome != Poor || f.SettledAt.IsZero() {
			t.Errorf("outcome %q settled %v; an outcome known at the time is stamped at the time",
				f.Outcome, f.SettledAt)
		}
		if !f.At.Equal(noon) {
			t.Errorf("at = %v, want the caller's clock (%v)", f.At, noon)
		}

		if len(f.Considered) != 1 {
			t.Fatalf("%d questions weighed, want 1", len(f.Considered))
		}
		c := f.Considered[0]
		if c.State != Fired || c.Held != Unmet {
			t.Errorf("state %q held %q", c.State, c.Held)
		}
		if len(c.Met) != 2 || c.Met[0] != Unmet || c.Met[1] != Met {
			t.Errorf("met = %v; which condition matched is the whole point of the table", c.Met)
		}
		if c.Action.Term != "no" {
			t.Errorf("action = %+v, want the otherwise", c.Action)
		}
		// The sentence reads back whole — both branches, not only the one taken.
		if c.Says != "when they are paying me above $150 and the task is legal then say yes otherwise say no" {
			t.Errorf("says %q", c.Says)
		}
		if c.Did != "say no" {
			t.Errorf("did %q", c.Did)
		}
	})
}

/*
A record is a statement about what happened, so revising the rule afterwards must
not rewrite it.

This is the loop's own requirement: §3.14 step 5 is "you revise", and the thing
the player revises *from* is the log. A record that pointed at the current rule
would change its story the moment they acted on it.
*/
func TestRevisingARuleDoesNotRewriteWhatItAlreadyDid(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		answered(t, ctx, tx, m, account, "east-side", "help", startingHelp())

		d, err := m.Weigh(ctx, tx, account, "east-side", aFavour(9000))
		if err != nil {
			t.Fatalf("Weigh: %v", err)
		}
		if _, err := m.Record(ctx, tx, account, noon, "east-side", d, Poor); err != nil {
			t.Fatalf("Record: %v", err)
		}

		// Having read that back, the player changes their mind entirely.
		answered(t, ctx, tx, m, account, "east-side", "help", Sentence{
			When: []Ref{{Term: "legal"}}, Join: And,
			Then: Ref{Term: "no"}, Else: Ref{Term: "yes"},
		})

		log, err := m.Fires(ctx, tx, account, 1)
		if err != nil {
			t.Fatalf("Fires: %v", err)
		}
		c := log.Fires[0].Considered[0]
		if c.Says != "when they are paying me above $60 then say yes otherwise say no" {
			t.Errorf("says %q; the record holds the sentence as it stood", c.Says)
		}
	})
}

// A moment that put a question the player has no answer to is recorded as one.
// §3.14: gaps in the playbook are what demand attention, so the gap is a row.
func TestAGapInThePlaybookIsWrittenDown(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		d, err := m.Weigh(ctx, tx, account, "east-side", aFavour(9000))
		if err != nil {
			t.Fatalf("Weigh: %v", err)
		}
		if d.Verdict != Silent {
			t.Fatalf("verdict %q, want %q", d.Verdict, Silent)
		}
		if _, err := m.Record(ctx, tx, account, noon, "east-side", d, Pending); err != nil {
			t.Fatalf("Record: %v", err)
		}

		log, err := m.Fires(ctx, tx, account, 1)
		if err != nil {
			t.Fatalf("Fires: %v", err)
		}
		f := log.Fires[0]
		if f.Verdict != Silent || f.Outcome != Pending || !f.SettledAt.IsZero() {
			t.Errorf("record = %+v", f)
		}
		if len(f.Considered) != 1 || f.Considered[0].State != Unanswered {
			t.Fatalf("considered = %+v", f.Considered)
		}
		if c := f.Considered[0]; c.Says != "" || len(c.Rule.When) != 0 {
			t.Errorf("an unanswered question read back a sentence: %+v", c)
		}
		if f.Note != "nothing in your playbook covered this." {
			t.Errorf("note %q", f.Note)
		}
	})
}

// Two answers that disagree, recorded as a disagreement: both rows applied,
// neither fired, and the log says why.
func TestADisagreementIsRecordedAsOne(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
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
		if _, err := m.Record(ctx, tx, account, noon, "east-side", d, Pending); err != nil {
			t.Fatalf("Record: %v", err)
		}

		log, err := m.Fires(ctx, tx, account, 1)
		if err != nil {
			t.Fatalf("Fires: %v", err)
		}
		f := log.Fires[0]
		if f.Verdict != Clash || len(f.Considered) != 2 {
			t.Fatalf("record = %+v", f)
		}
		for _, c := range f.Considered {
			if c.State != Weighed {
				t.Errorf("%q is %q; both halves of a disagreement were weighed and neither fired",
					c.Question, c.State)
			}
		}
		if f.Note != "answer 1 said to go along with it; answer 4 said to refuse." {
			t.Errorf("note %q", f.Note)
		}
	})
}

/*
An outcome the world answers later.

technical.md §4's third escalation condition is "a rule fired and the outcome was
poor", and §3.4's active jobs have consequences that run for weeks — so the
record is written when the rule fires and settled when there is something to say.
Once, because evidence that could be re-graded is not evidence.
*/
func TestAnOutcomeIsSettledOnceAndNotAgain(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		answered(t, ctx, tx, m, account, "east-side", "help", startingHelp())

		d, err := m.Weigh(ctx, tx, account, "east-side", aFavour(9000))
		if err != nil {
			t.Fatalf("Weigh: %v", err)
		}
		written, err := m.Record(ctx, tx, account, noon, "east-side", d, Pending)
		if err != nil {
			t.Fatalf("Record: %v", err)
		}

		later := noon.Add(72 * time.Hour)
		if err := m.Settle(ctx, tx, account, written.ID, later, Poor); err != nil {
			t.Fatalf("Settle: %v", err)
		}
		if err := m.Settle(ctx, tx, account, written.ID, later, Good); !errors.Is(err, ErrNoSuchFire) {
			t.Errorf("settling twice: %v, want %v", err, ErrNoSuchFire)
		}
		if err := m.Settle(ctx, tx, account, written.ID, later, Pending); !errors.Is(err, ErrNoSuchOutcome) {
			t.Errorf("settling back to pending: %v, want %v", err, ErrNoSuchOutcome)
		}

		log, err := m.Fires(ctx, tx, account, 1)
		if err != nil {
			t.Fatalf("Fires: %v", err)
		}
		if f := log.Fires[0]; f.Outcome != Poor || !f.SettledAt.Equal(later) {
			t.Errorf("outcome %q settled %v, want %q at %v", f.Outcome, f.SettledAt, Poor, later)
		}
	})
}

/*
§3.6's pattern: one bad outcome is noise, the same answer failing all week is
something to act on.

Counted in one query rather than by reading the log, because the counting is a
game rule and rules are the server's (§10.2).
*/
func TestAWeekOfTheSameAnswerAddsUpToAPattern(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		answered(t, ctx, tx, m, account, "east-side", "help", startingHelp())

		for day := 0; day < 5; day++ {
			at := noon.Add(time.Duration(day) * 24 * time.Hour)
			d, err := m.Weigh(ctx, tx, account, "east-side", aFavour(9000))
			if err != nil {
				t.Fatalf("Weigh: %v", err)
			}
			outcome := Poor
			if day == 4 {
				outcome = Good
			}
			if _, err := m.Record(ctx, tx, account, at, "east-side", d, outcome); err != nil {
				t.Fatalf("Record: %v", err)
			}
		}
		// One more moment the player has no answer to at all.
		d, err := m.Weigh(ctx, tx, account, "east-side", Arises("a-shout", "voice"))
		if err != nil {
			t.Fatalf("Weigh: %v", err)
		}
		if _, err := m.Record(ctx, tx, account, noon, "east-side", d, Pending); err != nil {
			t.Fatalf("Record: %v", err)
		}

		trends, err := m.Evidence(ctx, tx, account, noon.Add(-24*time.Hour))
		if err != nil {
			t.Fatalf("Evidence: %v", err)
		}
		by := map[string]Trend{}
		for _, tr := range trends {
			by[tr.Question] = tr
		}
		if got := by["help"]; got.Put != 5 || got.Fired != 5 || got.Poor != 4 || got.Good != 1 {
			t.Errorf("help = %+v; want five moments, all decided, four of them badly", got)
		}
		if got := by["voice"]; got.Put != 1 || got.Unanswered != 1 || got.Fired != 0 {
			t.Errorf("voice = %+v; want one moment with no answer to it", got)
		}
		if by["help"].N != 1 || by["voice"].N != 2 {
			t.Errorf("answers are cited by their authored numbers: %+v", trends)
		}

		// A window that starts after everything happened counts nothing, which
		// is what makes "this week" mean this week.
		empty, err := m.Evidence(ctx, tx, account, noon.Add(365*24*time.Hour))
		if err != nil {
			t.Fatalf("Evidence: %v", err)
		}
		if len(empty) != 0 {
			t.Errorf("%d trends outside the window", len(empty))
		}
	})
}

/*
The log is paginated, newest first, and a page past the end is clamped rather
than refused — the same bargain the leaderboards make. A player who has been
going a year has a year of moments and no reason to be handed all of them.
*/
func TestTheLogIsAPageOfTheMostRecentMoments(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		answered(t, ctx, tx, m, account, "east-side", "help", startingHelp())

		total := LogPage + 3
		for i := 0; i < total; i++ {
			d, err := m.Weigh(ctx, tx, account, "east-side", aFavour(9000))
			if err != nil {
				t.Fatalf("Weigh: %v", err)
			}
			if _, err := m.Record(ctx, tx, account,
				noon.Add(time.Duration(i)*time.Hour), "east-side", d, Pending); err != nil {
				t.Fatalf("Record: %v", err)
			}
		}

		first, err := m.Fires(ctx, tx, account, 1)
		if err != nil {
			t.Fatalf("Fires: %v", err)
		}
		if first.Total != total || first.Pages != 2 || len(first.Fires) != LogPage {
			t.Fatalf("page one = %d of %d over %d pages", len(first.Fires), first.Total, first.Pages)
		}
		if !first.Fires[0].At.After(first.Fires[1].At) {
			t.Error("the log is not newest first")
		}

		last, err := m.Fires(ctx, tx, account, 99)
		if err != nil {
			t.Fatalf("Fires: %v", err)
		}
		if last.Page != 2 || len(last.Fires) != 3 {
			t.Errorf("page 99 answered as page %d with %d records", last.Page, len(last.Fires))
		}
	})
}

// A moment that put no question the vocabulary has is not an empty log entry.
// Nothing was weighed, so there is nothing to be evidence of.
func TestAMomentThatPutNothingIsNotRecorded(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		d, err := m.Weigh(ctx, tx, account, "east-side", Arises("a-nothing", "a-question-nobody-asks"))
		if err != nil {
			t.Fatalf("Weigh: %v", err)
		}
		written, err := m.Record(ctx, tx, account, noon, "east-side", d, Pending)
		if err != nil {
			t.Fatalf("Record: %v", err)
		}
		if written.ID != 0 {
			t.Errorf("record %d written for a moment nothing was weighed for", written.ID)
		}
		log, err := m.Fires(ctx, tx, account, 1)
		if err != nil {
			t.Fatalf("Fires: %v", err)
		}
		if log.Total != 0 {
			t.Errorf("%d records in the log", log.Total)
		}
	})
}

// The static audit, through the module: the same analysis, scoped to one rule
// set, so a screen asks about where the player is standing rather than about
// everything they have ever written.
func TestTheAuditIsScopedToTheRuleSetItIsAskedAbout(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		answered(t, ctx, tx, m, account, "east-side", "help", Sentence{
			When: []Ref{{Term: "legal"}}, Join: And, Then: Ref{Term: "yes"}, Else: Ref{Term: "no"},
		})
		answered(t, ctx, tx, m, account, "east-side", "voice", Sentence{
			When: []Ref{{Term: "above"}}, Join: And, Then: Ref{Term: "back"}, Else: Ref{Term: "quiet"},
		})

		here, err := m.Foresaw(ctx, tx, account, "east-side", "help", "voice")
		if err != nil {
			t.Fatalf("Foresaw: %v", err)
		}
		if len(here.Conflicts) != 1 || here.Conflicts[0].Questions != [2]string{"help", "voice"} {
			t.Errorf("conflicts = %+v", here.Conflicts)
		}

		away, err := m.Foresaw(ctx, tx, account, "downtown", "help", "voice")
		if err != nil {
			t.Fatalf("Foresaw: %v", err)
		}
		if len(away.Conflicts) != 0 {
			t.Errorf("conflicts somewhere with no rules: %+v", away.Conflicts)
		}
	})
}
