package situation

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/clock"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/playbook"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store/storetest"
)

/*
§3.14's loop, against a real Postgres (§14).

The catalogue is proved in catalogue_test.go with nothing running. What is left
is the half that only exists because the player is not present: a role produces
work on days nobody was there for, the rules in force settle what they cover,
what they do not cover waits, revising a rule changes what happens next, and
none of it is random.
*/

/*
world is everything one player's delegated role needs around it.

The playbook is the real module rather than a stand-in, and deliberately: the
whole ticket is that a job is answered by the *player's own rules*, and a fake
matcher would prove the plumbing and none of the design.
*/
type world struct {
	ctx     context.Context
	tx      pgx.Tx
	clk     *clock.Stepping
	m       *Situations
	rules   *playbook.Playbooks
	account string
	job     *rung
	paid    *ledger
	// from is the next day [arise] will look at. A day is only ever produced
	// once, so a test that wants two moments has to keep moving forward.
	from int
}

// rung is a Holding a test can move up and down the ladder.
type rung struct{ tier simulation.Tier }

func (r *rung) Holds(context.Context, pgx.Tx, string) (simulation.Tier, error) { return r.tier, nil }

// here is a Where for a player who never moves. §3.15's transfer is
// internal/playbook's test; what this package needs is one environment that
// exists.
type here struct{}

func (here) Where(context.Context, pgx.Tx, string) (string, error) { return "east-side", nil }

// ledger is an Awards that records what it was handed, so a test can assert
// that an answer's authored deltas were actually paid.
type ledger struct{ grants []simulation.Grant }

func (l *ledger) Award(_ context.Context, _ pgx.Tx, _ string, grants ...simulation.Grant) error {
	l.grants = append(l.grants, grants...)
	return nil
}

func (l *ledger) credit(f simulation.Factor) float64 {
	out := 0.0
	for _, g := range l.grants {
		if g.Factor == f {
			out += g.Credit
		}
	}
	return out
}

// districts is the environment set the playbook holds rule sets for.
type districts struct{}

func (districts) Environments() []playbook.Environment {
	return []playbook.Environment{{Key: "east-side", Name: "East Side"}}
}

// with runs f inside one transaction on a fresh database, which is the shape
// every command has (§3.2) and therefore the shape every test should have.
//
// The transaction is rolled back by a cleanup rather than by a closure around
// f, because a t.Fatalf inside f ends the goroutine outright — a closure would
// never return and the connection would be held until the pool gave up.
func with(t *testing.T, f func(*world)) {
	t.Helper()
	st := storetest.New(t)
	ctx := t.Context()

	tx, err := st.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	clk := &clock.Stepping{At: time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)}
	rules := playbook.New(districts{})
	job := &rung{tier: simulation.Delegated}
	paid := &ledger{}

	f(&world{
		ctx: ctx, tx: tx, clk: clk, rules: rules, job: job, paid: paid, from: 2,
		m:       New(clk, job, rules, here{}, paid),
		account: newAccount(t, ctx, tx),
	})
}

func newAccount(t *testing.T, ctx context.Context, tx pgx.Tx) string {
	t.Helper()
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO account (provider, subject, email, display_name)
		VALUES ('google', 'situation-test', 'situation@example.com', 'Situation')
		RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("create account: %v", err)
	}
	return id
}

func day(n int) simulation.Date {
	return simulation.Date{Year: 2026, Month: time.March, Day: n}
}

func (w *world) live(t *testing.T, on simulation.Date) Arisen {
	t.Helper()
	out, err := w.m.Day(w.ctx, w.tx, w.account, on)
	if err != nil {
		t.Fatalf("day %v: %v", on, err)
	}
	return out
}

func (w *world) unlock(t *testing.T, ids ...string) {
	t.Helper()
	if err := w.rules.Unlock(w.ctx, w.tx, w.account, ids...); err != nil {
		t.Fatalf("unlock %v: %v", ids, err)
	}
}

func (w *world) answer(t *testing.T, question string, s playbook.Sentence) {
	t.Helper()
	if _, err := w.rules.Compose(w.ctx, w.tx, w.account, "east-side", question, s); err != nil {
		t.Fatalf("compose %s: %v", question, err)
	}
}

func (w *world) waiting(t *testing.T) (Waiting, bool) {
	t.Helper()
	out, standing, err := w.m.Waiting(w.ctx, w.tx, w.account)
	if err != nil {
		t.Fatalf("waiting: %v", err)
	}
	return out, standing
}

// walk lives a run of consecutive days, returning what arose on each.
func (w *world) walk(t *testing.T, from, to int) []Arisen {
	t.Helper()
	out := []Arisen{}
	for n := from; n <= to; n++ {
		out = append(out, w.live(t, day(n)))
	}
	return out
}

/* ── it happens without you ───────────────────────────────────────────── */

/*
The ticket's first line: **higher-tier roles generate situations while the
player is elsewhere.**

Nothing in this test is a command. The days pass, and the job does its work —
which is the whole of §3.14's step 1 and the reason none of this hangs off a
button.
*/
func TestADelegatedRoleProducesWorkOnItsOwn(t *testing.T) {
	t.Parallel()
	with(t, func(w *world) {
		arose := w.live(t, day(2))
		if !arose.Arose() {
			t.Fatal("a delegated role produced nothing on a day that happened")
		}
		if _, known := Known(arose.Situation); !known {
			t.Fatalf("%q is not an authored situation", arose.Situation)
		}
		if arose.Fire == 0 {
			t.Error("nothing was written down, so there is nothing to review")
		}
	})
}

/*
And §3.14's other half of the same sentence: **low tiers are genuinely
passive — you hold the job, it pays.**

A rung below the line produces nothing, and neither does no job at all. Both
matter: the first is the design's promise about the bottom of the ladder, and
the second is what stops an unemployed player being handed a manager's Tuesday.
*/
func TestAPassiveJobProducesNothing(t *testing.T) {
	t.Parallel()
	with(t, func(w *world) {
		for _, tier := range []simulation.Tier{simulation.Unemployed, 1, 2} {
			w.job.tier = tier
			if arose := w.live(t, day(int(tier)+2)); arose.Arose() {
				t.Errorf("tier %d produced %q; it pays without being played", tier, arose.Situation)
			}
		}
		w.job.tier = simulation.Delegated
		if !w.live(t, day(9)).Arose() {
			t.Error("the first active rung produced nothing")
		}
	})
}

/*
§7, and the reason a rollover can be retried: the same player on the same day
gets the same work.

A day walked twice produces one moment, not two — the ledger walk is idempotent
in the date, the way every other part of the rollover is (§3.3).
*/
func TestTheSameDayProducesTheSameWorkOnce(t *testing.T) {
	t.Parallel()
	with(t, func(w *world) {
		first := w.live(t, day(2))
		second := w.live(t, day(2))
		if !first.Arose() {
			t.Fatal("nothing arose on the first walk")
		}
		if second.Arose() {
			t.Errorf("walking the same day again produced %q as well", second.Situation)
		}

		var n int
		if err := w.tx.QueryRow(w.ctx,
			`SELECT count(*) FROM job_situation WHERE account_id = $1`, w.account).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 1 {
			t.Errorf("%d moments were written for one day", n)
		}
	})
}

// And which moment it is, is decided rather than rolled: two players do not get
// the same morning, and one player gets the same one however often it is asked.
func TestWhichMomentArisesIsDecidedAndNotRolled(t *testing.T) {
	t.Parallel()

	pool := len(For(simulation.Delegated))
	if pool < 2 {
		t.Skipf("tier %d has %d situations, so there is nothing to choose between", simulation.Delegated, pool)
	}

	for i := range 5 {
		if pick("somebody", day(3), pool) != pick("somebody", day(3), pool) {
			t.Fatalf("the same day picked twice, differently (run %d)", i)
		}
	}
	moved := false
	for n := 1; n <= 10; n++ {
		if pick("somebody", day(n), pool) != pick("somebody", day(1), pool) {
			moved = true
		}
	}
	if !moved {
		t.Error("ten days in a row picked the same moment")
	}
	same := true
	for n := 1; n <= 12; n++ {
		if pick("somebody", day(n), pool) != pick("somebody-else", day(n), pool) {
			same = false
		}
	}
	if same {
		t.Error("two players are being handed the same twelve mornings")
	}
}

/* ── the rules answer what they cover ─────────────────────────────────── */

// covering writes a rule that settles every question a moment puts, all one
// way, so the moment resolves without the player.
func covering(t *testing.T, w *world, a Authored, then, otherwise map[string]playbook.Ref) {
	t.Helper()
	for _, q := range a.Puts() {
		w.answer(t, q, playbook.Sentence{
			When: []playbook.Ref{conditionOf(t, q)},
			Join: playbook.And,
			Then: then[q],
			Else: otherwise[q],
		})
	}
}

// conditionOf is a condition of a question that a player starts with, so the
// sentence is composable on day one.
func conditionOf(t *testing.T, question string) playbook.Ref {
	t.Helper()
	q, ok := playbook.Vocab().Question(question)
	if !ok {
		t.Fatalf("%q is not a question", question)
	}
	for _, term := range q.Conditions() {
		if !term.Start || term.Takes() {
			continue
		}
		return playbook.Ref{Term: term.Key}
	}
	t.Fatalf("%q has no plain opening condition", question)
	return playbook.Ref{}
}

/*
The ticket's second and seventh lines at once: **playbook rules auto-resolve
situations they cover**, and **good rule authorship feeds intelligence and
emotional stability.**

The roster gap is the moment §3.14 was written about. Answering it with the
posture the content calls good settles it without the player being asked, pays
what the answer is worth, and leaves nothing waiting.
*/
func TestRulesSettleWhatTheyCoverAndPayForIt(t *testing.T) {
	t.Parallel()
	with(t, func(w *world) {
		gap, known := Known("the-roster-gap")
		if !known {
			t.Skip("the-roster-gap is not authored")
		}
		// "ask for the time back instead", either way — the bargain, which this
		// moment rewards. It is a term a player earns (§3.6) rather than one
		// they arrive with, which is the point: the answer that works here is
		// on the far side of having been through something.
		w.unlock(t, "overtime/action/time")
		covering(t, w, gap,
			map[string]playbook.Ref{"overtime": {Term: "time"}},
			map[string]playbook.Ref{"overtime": {Term: "time"}})

		arose := arise(t, w, "the-roster-gap")
		if arose.Handled != playbook.Bargain {
			t.Fatalf("the rules settled it as %q, want %q", arose.Handled, playbook.Bargain)
		}
		if arose.Waiting {
			t.Error("a moment the rules covered is waiting on the player anyway")
		}
		if _, standing := w.waiting(t); standing {
			t.Error("something is in front of the player after their rules answered it")
		}
		if w.paid.credit(simulation.Intelligence) <= 0 {
			t.Error("a well-composed answer paid nothing into judgement")
		}
	})
}

/*
The ticket's sixth line, end to end: **an authored situation makes a plausible
over-general rule cost the player something.**

The rule is the one §3.14 names — "always maximise X", written here as the
overtime answer that accepts whichever way its condition falls. It has paid
every time up to now. Here it does not.
*/
func TestAnOverGeneralRuleCostsSomething(t *testing.T) {
	t.Parallel()
	with(t, func(w *world) {
		gap, known := Known("the-roster-gap")
		if !known {
			t.Skip("the-roster-gap is not authored")
		}
		// accept it, otherwise accept it. Idle, in force, and exactly what a
		// player who has been on the bay for a fortnight writes.
		covering(t, w, gap,
			map[string]playbook.Ref{"overtime": {Term: "accept"}},
			map[string]playbook.Ref{"overtime": {Term: "accept"}})

		arose := arise(t, w, "the-roster-gap")
		if arose.Handled != playbook.Yield {
			t.Fatalf("the rules settled it as %q, want %q", arose.Handled, playbook.Yield)
		}

		var outcome string
		if err := w.tx.QueryRow(w.ctx,
			`SELECT outcome FROM job_situation WHERE account_id = $1 AND situation = $2`,
			w.account, "the-roster-gap").Scan(&outcome); err != nil {
			t.Fatalf("read outcome: %v", err)
		}
		if playbook.Outcome(outcome) != playbook.Poor {
			t.Errorf("the catch-all came to %q; §3.14 says an over-general rule must backfire", outcome)
		}
		if w.paid.credit(simulation.Stability) >= 0 {
			t.Errorf("it cost %v of stability", w.paid.credit(simulation.Stability))
		}
	})
}

/*
The ticket's fifth line: **revising a rule visibly changes subsequent
outcomes.**

The same moment, twice, with one word of the playbook changed in between — and
the two settle differently. That is the whole of §3.14's step 5, and it is only
demonstrable because the matcher is deterministic (§7): nothing else about the
world moved.
*/
func TestRevisingARuleChangesWhatHappensNext(t *testing.T) {
	t.Parallel()
	with(t, func(w *world) {
		gap, known := Known("the-roster-gap")
		if !known {
			t.Skip("the-roster-gap is not authored")
		}

		w.unlock(t, "overtime/action/time")
		covering(t, w, gap,
			map[string]playbook.Ref{"overtime": {Term: "accept"}},
			map[string]playbook.Ref{"overtime": {Term: "accept"}})
		before := arise(t, w, "the-roster-gap")

		// One word. Nothing else about the world is touched.
		covering(t, w, gap,
			map[string]playbook.Ref{"overtime": {Term: "time"}},
			map[string]playbook.Ref{"overtime": {Term: "time"}})
		after := arise(t, w, "the-roster-gap")

		if before.Handled == after.Handled {
			t.Fatalf("the same moment was settled as %q both times", before.Handled)
		}
		beforeAnswer, _ := gap.Answer(before.Handled)
		afterAnswer, _ := gap.Answer(after.Handled)
		if beforeAnswer.Outcome == afterAnswer.Outcome {
			t.Errorf("both revisions came to %q, so the edit changed nothing that happened",
				beforeAnswer.Outcome)
		}
		t.Logf("%q → %q, then %q → %q",
			before.Handled, beforeAnswer.Outcome, after.Handled, afterAnswer.Outcome)
	})
}

/* ── what the rules do not cover ──────────────────────────────────────── */

/*
The ticket's fourth line: **situations the playbook does not cover escalate as
live decisions.**

A player with no rules is handed the moment itself, with the answers the content
wrote for it — not a notification about it, and not a summary of what their
character did, because their character did not do anything.
*/
func TestWhatNothingCoveredWaitsForThePlayer(t *testing.T) {
	t.Parallel()
	with(t, func(w *world) {
		arose := w.live(t, day(2))
		if !arose.Waiting {
			t.Fatal("a player with no rules had their job answered for them")
		}
		if arose.Cause != playbook.Unhandled {
			t.Errorf("it is waiting because %q, want %q", arose.Cause, playbook.Unhandled)
		}

		standing, ok := w.waiting(t)
		if !ok {
			t.Fatal("nothing is in front of the player")
		}
		if standing.Situation.Key != arose.Situation {
			t.Errorf("%q is waiting, and %q arose", standing.Situation.Key, arose.Situation)
		}
		if len(standing.Situation.Answers()) == 0 {
			t.Error("it is waiting with nothing the player can do about it")
		}
		if standing.Note == "" {
			t.Error("nothing says why it reached them")
		}
	})
}

/*
And it is an escalation the world has already made, rather than one queued
behind §4's release policy.

The record is marked raised by this module because the moment *is* in front of
the player: a conflict raised here and again through the backlog would be the
same thing put to somebody twice under two different names.
*/
func TestAWaitingMomentIsNotAlsoQueuedAsAnEscalation(t *testing.T) {
	t.Parallel()
	with(t, func(w *world) {
		arose := w.live(t, day(2))
		if !arose.Waiting {
			t.Fatal("nothing is waiting")
		}
		left, err := w.rules.Unraised(w.ctx, w.tx, w.account, 5)
		if err != nil {
			t.Fatalf("unraised: %v", err)
		}
		if len(left) != 0 {
			t.Errorf("%d escalations are also queued for a moment already in front of the player", len(left))
		}
	})
}

/*
§3.14's "good rules buy time, and gaps in the playbook are what demand
attention", as a mechanic rather than a slogan.

A role that is waiting on the player produces nothing new. It is what bounds a
long absence (§4: "a long absence does not dump the backlog") — a fortnight away
with an empty playbook is one decision waiting rather than fourteen — and it is
what makes composing rules pay: the job only keeps moving while they cover it.
*/
func TestARoleThatIsWaitingProducesNothingNew(t *testing.T) {
	t.Parallel()
	with(t, func(w *world) {
		arose := w.walk(t, 2, 15)

		produced := 0
		for _, a := range arose {
			if a.Arose() {
				produced++
			}
		}
		if produced != 1 {
			t.Errorf("a fortnight away produced %d moments, want the one nobody has dealt with", produced)
		}

		standing, ok := w.waiting(t)
		if !ok {
			t.Fatal("nothing is waiting after a fortnight")
		}

		// And once it is answered, the job starts again.
		if _, err := w.m.Answer(w.ctx, w.tx, w.account, standing.ID,
			standing.Situation.Answers()[0].Stance); err != nil {
			t.Fatalf("answer: %v", err)
		}
		if !w.live(t, day(16)).Arose() {
			t.Error("the job produced nothing after the player caught up with it")
		}
	})
}

/*
Answering one by hand, which is §3.14's step 4 done by the player.

What follows has to be the same kind of consequence the rules would have
produced — the deltas paid, the record settled, the moment closed — or the log
would be counting two different things and §3.6's pattern would be built on
sand.
*/
func TestAnsweringOneByHandSettlesItLikeARuleWould(t *testing.T) {
	t.Parallel()
	with(t, func(w *world) {
		arose := w.live(t, day(2))
		standing, ok := w.waiting(t)
		if !ok {
			t.Fatal("nothing is waiting")
		}

		chosen := standing.Situation.Answers()[0]
		answer, err := w.m.Answer(w.ctx, w.tx, w.account, standing.ID, chosen.Stance)
		if err != nil {
			t.Fatalf("answer: %v", err)
		}
		if answer.Came != chosen.Came {
			t.Errorf("it came to %q, want the authored %q", answer.Came, chosen.Came)
		}
		if len(chosen.Moves) > 0 && len(w.paid.grants) == 0 {
			t.Error("the answer's authored deltas were not paid")
		}

		var (
			stance  string
			outcome string
			waiting bool
		)
		if err := w.tx.QueryRow(w.ctx,
			`SELECT stance, outcome, waiting FROM job_situation WHERE id = $1`, standing.ID,
		).Scan(&stance, &outcome, &waiting); err != nil {
			t.Fatalf("read the moment: %v", err)
		}
		if playbook.Stance(stance) != chosen.Stance || playbook.Outcome(outcome) != chosen.Outcome || waiting {
			t.Errorf("the moment reads stance=%q outcome=%q waiting=%v", stance, outcome, waiting)
		}

		// The record that was written pending when it arose is settled now.
		read, err := w.rules.Fires(w.ctx, w.tx, w.account, 1)
		if err != nil {
			t.Fatalf("fires: %v", err)
		}
		if len(read.Fires) != 1 {
			t.Fatalf("%d records, want the one this moment wrote", len(read.Fires))
		}
		if read.Fires[0].Outcome != chosen.Outcome {
			t.Errorf("the record says %q and the moment came to %q", read.Fires[0].Outcome, chosen.Outcome)
		}
		if read.Fires[0].Situation != arose.Situation {
			t.Errorf("the record is filed under %q", read.Fires[0].Situation)
		}

		// Twice is refused. A moment answered by two tabs is one moment.
		if _, err := w.m.Answer(w.ctx, w.tx, w.account, standing.ID, chosen.Stance); err == nil {
			t.Error("the same moment was answered twice")
		}
		if _, err := w.m.Answer(w.ctx, w.tx, w.account, standing.ID+1000, chosen.Stance); err == nil {
			t.Error("a moment that is not this player's was answered")
		}
	})
}

// A posture this moment was not written an answer for is refused rather than
// applied as nothing.
func TestAnAnswerHasToBeOneTheMomentHas(t *testing.T) {
	t.Parallel()
	with(t, func(w *world) {
		w.live(t, day(2))
		standing, ok := w.waiting(t)
		if !ok {
			t.Fatal("nothing is waiting")
		}

		missing := playbook.Stance("")
		for _, s := range playbook.Stances {
			if _, written := standing.Situation.Answer(s); !written {
				missing = s
			}
		}
		if missing == "" {
			t.Skipf("%s answers every posture there is", standing.Situation.Key)
		}
		if _, err := w.m.Answer(w.ctx, w.tx, w.account, standing.ID, missing); err == nil {
			t.Errorf("%q was accepted and %s has no answer for it", missing, standing.Situation.Key)
		}
	})
}

/* ── you review ───────────────────────────────────────────────────────── */

/*
The ticket's third line: **the player reviews a log — the situation, which rule
fired, the action, the consequence.**

Two of those four are internal/playbook's and two are this module's, which is
why they are joined rather than duplicated: the record knows which rule fired
and on which condition, and this knows what was happening and what came of it.
*/
func TestTheLogCanBeToldWhatWasHappening(t *testing.T) {
	t.Parallel()
	with(t, func(w *world) {
		gap, known := Known("the-roster-gap")
		if !known {
			t.Skip("the-roster-gap is not authored")
		}
		covering(t, w, gap,
			map[string]playbook.Ref{"overtime": {Term: "accept"}},
			map[string]playbook.Ref{"overtime": {Term: "accept"}})
		arose := arise(t, w, "the-roster-gap")

		told, err := w.m.Told(w.ctx, w.tx, w.account, []int64{arose.Fire})
		if err != nil {
			t.Fatalf("told: %v", err)
		}
		line, ok := told[arose.Fire]
		if !ok {
			t.Fatalf("the record has nothing said about it")
		}
		if line.Name != gap.Name || line.Line != gap.Line {
			t.Errorf("the log reads %q / %q", line.Name, line.Line)
		}
		yielded, _ := gap.Answer(playbook.Yield)
		if line.Did != yielded.Did || line.Came != yielded.Came {
			t.Errorf("the log says the character %q and it came to %q", line.Did, line.Came)
		}

		// And nothing for a record that did not come from a job, which is every
		// other thing that ever weighs a rule.
		if _, ok := told[arose.Fire+9999]; ok {
			t.Error("a record with no moment behind it was given one")
		}
		if empty, err := w.m.Told(w.ctx, w.tx, w.account, nil); err != nil || len(empty) != 0 {
			t.Errorf("Told(nothing) = %v, %v", empty, err)
		}
	})
}

/*
arise lives the first day that produces the named situation, and only that day.

Which moment a day produces is a pure function of who and when (see [pick]), so
a test that wants a particular one finds the day rather than walking towards it
— which keeps every other situation in the pool out of the ledger and out of the
record, and keeps these tests to one moment each.
*/
func arise(t *testing.T, w *world, key string) Arisen {
	t.Helper()
	pool := For(w.job.tier)
	want := -1
	for i, a := range pool {
		if a.Key == key {
			want = i
		}
	}
	if want < 0 {
		t.Fatalf("%q is not authored for tier %d", key, w.job.tier)
	}
	for n := w.from; n <= 60; n++ {
		on := day(n)
		if pick(w.account, on, len(pool)) != want {
			continue
		}
		w.from = n + 1
		arose := w.live(t, on)
		if arose.Situation != key {
			t.Fatalf("day %d produced %q, want %q", n, arose.Situation, key)
		}
		return arose
	}
	t.Fatalf("%q does not come up again before day 60", key)
	return Arisen{}
}
