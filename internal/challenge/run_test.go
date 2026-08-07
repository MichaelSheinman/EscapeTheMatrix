package challenge

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/clock"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store/storetest"
)

/*
Runs, against a real Postgres (§14).

Two rules of §5.1 are what these tests are for, and neither of them is provable
against a mock: **once ever, per player**, which is a unique constraint doing the
work, and **save and resume**, which is a checkpoint written at a scene boundary
and read back by somebody who closed the tab.

The third thing worth proving is that the authored deltas are paid **on
completion** and not as the player goes — because paying per choice would make
abandoning part way a way of taking the credit and leaving the consequences.
*/

/* ── the harness ───────────────────────────────────────────────────────── */

// paid records what a finished run handed to internal/factor.
type paid struct {
	calls  int
	grants []simulation.Grant
}

func (p *paid) Award(_ context.Context, _ pgx.Tx, _ string, grants ...simulation.Grant) error {
	p.calls++
	p.grants = append(p.grants, grants...)
	return nil
}

// settled records the event a finished run closed.
type settled struct {
	calls int
	id    int64
	at    time.Time
}

func (s *settled) Resolve(_ context.Context, _ pgx.Tx, _ string, id int64, at time.Time) error {
	s.calls, s.id, s.at = s.calls+1, id, at
	return nil
}

type harness struct {
	ctx     context.Context
	tx      pgx.Tx
	m       *Challenges
	account string
	clk     *clock.Stepping
	paid    *paid
	settled *settled
}

// the code every test in this file runs.
const trial = Code("resolve_conflict/a")

/*
with runs f inside one transaction on a fresh database, with a catalogue holding
one challenge.

The catalogue is swapped rather than loaded from files: what these tests are
about is the run, and a definition assembled in Go is the same definition the
loader would have produced — it went through [Plan.check] to get here.

The transaction is rolled back by a cleanup rather than by a closure, because a
t.Fatalf inside f ends the goroutine outright and a closure would never return.
*/
func with(t *testing.T, f func(h harness)) {
	t.Helper()
	withCatalogue(t, []Definition{authored(t, "a")}, f)
}

func withCatalogue(t *testing.T, defined []Definition, f func(h harness)) {
	t.Helper()

	held := catalogue.Load()
	t.Cleanup(func() { catalogue.Store(held) })
	swap(defined)

	st := storetest.New(t)
	ctx := t.Context()

	tx, err := st.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	h := harness{
		ctx:     ctx,
		tx:      tx,
		account: newAccount(t, ctx, tx),
		clk:     &clock.Stepping{At: time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)},
		paid:    &paid{},
		settled: &settled{},
	}
	h.m = New(h.clk, h.paid, h.settled)

	if err := h.m.Publish(ctx, tx); err != nil {
		t.Fatalf("publish: %v", err)
	}
	f(h)
}

// authored is the definition the tests run, as the loader would have made it.
func authored(t *testing.T, variant string) Definition {
	t.Helper()
	plan := twoScenes()
	if err := plan.check(); err != nil {
		t.Fatalf("the test plan is not runnable: %v", err)
	}
	return Definition{
		Family:   "resolve_conflict",
		Variant:  variant,
		Code:     Of("resolve_conflict", variant),
		Mechanic: "staged",
		Name:     "The argument on the stoop",
		Opens:    "a-shift-going-spare",
		Plan:     plan,
	}
}

func newAccount(t *testing.T, ctx context.Context, tx pgx.Tx) string {
	t.Helper()
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO account (provider, subject, email, display_name)
		VALUES ('google', 'challenge-test', 'challenge@example.com', 'Challenge')
		RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("create an account: %v", err)
	}
	return id
}

/* ── the catalog table ─────────────────────────────────────────────────── */

/*
§5.2: definitions are upserted into a catalog table **so foreign keys work**,
and the database stores runs, never definitions.

Both halves are here: the row carries §5.3's three identity columns and the code
derived from two of them, and it carries nothing else — no name, no scene, no
delta — while a run naming something unpublished is refused by the database
rather than by anything in Go.
*/
func TestTheCatalogueIsPublishedSoARunCanReferenceIt(t *testing.T) {
	with(t, func(h harness) {
		var family, variant, mechanic, code string
		var retired *time.Time
		if err := h.tx.QueryRow(h.ctx, `
			SELECT family, variant, mechanic, code, retired_at FROM challenge`).
			Scan(&family, &variant, &mechanic, &code, &retired); err != nil {
			t.Fatalf("read the catalog: %v", err)
		}
		if family != "resolve_conflict" || variant != "a" || mechanic != "staged" {
			t.Errorf("catalog row = %q/%q on %q, want the three identity columns", family, variant, mechanic)
		}
		if code != "resolve_conflict/a" {
			t.Errorf("code = %q, want it derived from the two columns", code)
		}
		if retired != nil {
			t.Error("a published challenge is retired")
		}

		// The table holds identity and provenance and nothing else. A column
		// carrying content would be content that goes stale.
		rows, err := h.tx.Query(h.ctx, `
			SELECT column_name FROM information_schema.columns WHERE table_name = 'challenge'`)
		if err != nil {
			t.Fatalf("read the catalog's shape: %v", err)
		}
		defer rows.Close()
		want := map[string]bool{
			"family": true, "variant": true, "mechanic": true, "code": true,
			"published_at": true, "retired_at": true,
		}
		for rows.Next() {
			var column string
			if err := rows.Scan(&column); err != nil {
				t.Fatalf("read the catalog's shape: %v", err)
			}
			if !want[column] {
				t.Errorf("the catalog carries %q, which is a definition rather than an identity", column)
			}
		}

		// And the foreign key is real.
		_, err = h.tx.Exec(h.ctx, `
			INSERT INTO challenge_run (account_id, challenge, state, scene, began_at, moved_at)
			VALUES ($1, 'jury_duty/a', 'running', 'court', now(), now())`, h.account)
		if err == nil {
			t.Fatal("a run of an unpublished challenge was accepted")
		}
	})
}

// A challenge withdrawn from content has runs behind it. Retiring rather than
// deleting is what keeps a player's history from being removed by an author
// tidying up.
func TestWithdrawnContentIsRetiredRatherThanDeleted(t *testing.T) {
	withCatalogue(t, []Definition{authored(t, "a"), authored(t, "b")}, func(h harness) {
		if _, err := h.m.Begin(h.ctx, h.tx, h.account, "resolve_conflict/b", 0); err != nil {
			t.Fatalf("begin: %v", err)
		}

		swap([]Definition{authored(t, "a")})
		if err := h.m.Publish(h.ctx, h.tx); err != nil {
			t.Fatalf("re-publish: %v", err)
		}

		var retired *time.Time
		if err := h.tx.QueryRow(h.ctx,
			`SELECT retired_at FROM challenge WHERE code = 'resolve_conflict/b'`).Scan(&retired); err != nil {
			t.Fatalf("the withdrawn challenge is gone from the catalog: %v", err)
		}
		if retired == nil {
			t.Error("a withdrawn challenge is still published")
		}

		var runs int
		if err := h.tx.QueryRow(h.ctx,
			`SELECT count(*) FROM challenge_run WHERE challenge = 'resolve_conflict/b'`).Scan(&runs); err != nil {
			t.Fatalf("count the runs: %v", err)
		}
		if runs != 1 {
			t.Errorf("%d runs of the withdrawn challenge survived, want 1", runs)
		}

		// Publishing it again un-retires it rather than making a second row.
		swap([]Definition{authored(t, "a"), authored(t, "b")})
		if err := h.m.Publish(h.ctx, h.tx); err != nil {
			t.Fatalf("re-publish: %v", err)
		}
		if err := h.tx.QueryRow(h.ctx,
			`SELECT retired_at FROM challenge WHERE code = 'resolve_conflict/b'`).Scan(&retired); err != nil {
			t.Fatalf("read the catalog: %v", err)
		}
		if retired != nil {
			t.Error("re-publishing left it retired")
		}
	})
}

/* ── beginning, checkpointing, resuming ────────────────────────────────── */

func TestARunBeginsAtTheOpeningScene(t *testing.T) {
	with(t, func(h harness) {
		run, err := h.m.Begin(h.ctx, h.tx, h.account, trial, 0)
		if err != nil {
			t.Fatalf("Begin: %v", err)
		}
		if run.Scene.Key != "stoop" {
			t.Errorf("Scene = %q, want the plan's opening", run.Scene.Key)
		}
		if len(run.Trail) != 0 || run.Done {
			t.Errorf("a fresh run is %+v, want nothing committed and not done", run)
		}
		if len(run.Scene.Options) != 2 {
			t.Errorf("%d options at the opening, want the authored two", len(run.Scene.Options))
		}
	})
}

/*
§5.1's checkpoint, and the whole of why it is at scene boundaries.

The row moves *because* the run left a scene — not on a timer, and not on a
schedule — so what a resume lands on is somewhere the content decided was a
place to stand.
*/
func TestTheCheckpointMovesWhenASceneIsLeft(t *testing.T) {
	with(t, func(h harness) {
		if _, err := h.m.Begin(h.ctx, h.tx, h.account, trial, 0); err != nil {
			t.Fatalf("Begin: %v", err)
		}

		before := h.clk.Now()
		h.clk.Advance(90 * time.Second)
		run, err := h.m.Choose(h.ctx, h.tx, h.account, trial, "stoop", "listen")
		if err != nil {
			t.Fatalf("Choose: %v", err)
		}
		if run.Scene.Key != "kitchen" {
			t.Errorf("Scene = %q, want where the option led", run.Scene.Key)
		}
		if len(run.Trail) != 1 || run.Trail[0] != (Step{Scene: "stoop", Option: "listen"}) {
			t.Errorf("Trail = %+v, want the one committed choice with its scene", run.Trail)
		}
		if !run.Moved.After(before) {
			t.Errorf("Moved = %v, want the moment the scene was left", run.Moved)
		}

		var scene string
		if err := h.tx.QueryRow(h.ctx,
			`SELECT scene FROM challenge_run WHERE account_id = $1`, h.account).Scan(&scene); err != nil {
			t.Fatalf("read the checkpoint: %v", err)
		}
		if scene != "kitchen" {
			t.Errorf("the stored checkpoint is %q, want the scene the run is standing at", scene)
		}
	})
}

/*
§5.1: "even at 6 minutes, players abandon partway." Coming back is the same call
as opening it, and it lands on the scene the run was left at with the trail
intact — not at the beginning, and not in the middle of a sentence.
*/
func TestAbandoningPartwayResumesWhereItWasLeft(t *testing.T) {
	with(t, func(h harness) {
		if _, err := h.m.Begin(h.ctx, h.tx, h.account, trial, 0); err != nil {
			t.Fatalf("Begin: %v", err)
		}
		if _, err := h.m.Choose(h.ctx, h.tx, h.account, trial, "stoop", "listen"); err != nil {
			t.Fatalf("Choose: %v", err)
		}

		// A week later, a different tab, a different day.
		h.clk.Advance(7 * 24 * time.Hour)
		back, err := h.m.Begin(h.ctx, h.tx, h.account, trial, 0)
		if err != nil {
			t.Fatalf("Begin again: %v", err)
		}
		if back.Scene.Key != "kitchen" {
			t.Errorf("resumed at %q, want the checkpoint", back.Scene.Key)
		}
		if len(back.Trail) != 1 {
			t.Errorf("Trail = %+v, want what was committed before", back.Trail)
		}
		if back.Done {
			t.Error("resuming finished the run")
		}

		// And Of finds it without being told which challenge.
		open, running, err := h.m.Of(h.ctx, h.tx, h.account)
		if err != nil || !running {
			t.Fatalf("Of = %v, %v; want the run in progress", running, err)
		}
		if open.Scene.Key != "kitchen" {
			t.Errorf("Of landed at %q, want the checkpoint", open.Scene.Key)
		}
	})
}

/*
§5.1: "once ever, per player." The unique constraint is the mechanism, not a
safety net — and finishing is what closes the door, so a run in progress is
still resumable.
*/
func TestAChallengeIsPlayedOnceEver(t *testing.T) {
	with(t, func(h harness) {
		finish(t, h)

		if _, err := h.m.Begin(h.ctx, h.tx, h.account, trial, 0); !errors.Is(err, ErrDone) {
			t.Fatalf("Begin after finishing = %v, want ErrDone", err)
		}
		if _, err := h.m.Choose(h.ctx, h.tx, h.account, trial, "kitchen", "side-with-her"); !errors.Is(err, ErrNotRunning) {
			t.Fatalf("Choose after finishing = %v, want ErrNotRunning", err)
		}

		played, err := h.m.Played(h.ctx, h.tx, h.account, trial)
		if err != nil || !played {
			t.Fatalf("Played = %v, %v; want true", played, err)
		}
		_, running, err := h.m.Of(h.ctx, h.tx, h.account)
		if err != nil || running {
			t.Fatalf("Of = %v, %v; want nothing in progress", running, err)
		}
	})
}

/* ── choosing ──────────────────────────────────────────────────────────── */

/*
A stale tab is the ordinary case, not an attack: somebody answered, the request
was resent, or a second window still shows the scene they have left. It is
refused with its own error, so a caller can say "you have already moved on"
rather than applying the choice twice.
*/
func TestAChoiceAtASceneTheRunHasLeftIsRefused(t *testing.T) {
	with(t, func(h harness) {
		if _, err := h.m.Begin(h.ctx, h.tx, h.account, trial, 0); err != nil {
			t.Fatalf("Begin: %v", err)
		}
		if _, err := h.m.Choose(h.ctx, h.tx, h.account, trial, "stoop", "listen"); err != nil {
			t.Fatalf("Choose: %v", err)
		}

		_, err := h.m.Choose(h.ctx, h.tx, h.account, trial, "stoop", "walk-on")
		if !errors.Is(err, ErrMoved) {
			t.Fatalf("Choose at the scene just left = %v, want ErrMoved", err)
		}

		var trailed []Step
		run, _, err := h.m.held(h.ctx, h.tx, h.account, authored(t, "a"), false)
		if err != nil {
			t.Fatalf("read the run: %v", err)
		}
		trailed = run.Trail
		if len(trailed) != 1 {
			t.Errorf("Trail = %+v, want the refusal to have committed nothing", trailed)
		}
	})
}

func TestAnOptionThatIsNotOnOfferIsRefused(t *testing.T) {
	with(t, func(h harness) {
		if _, err := h.m.Begin(h.ctx, h.tx, h.account, trial, 0); err != nil {
			t.Fatalf("Begin: %v", err)
		}
		// Real, but at the other scene: an option key is only unique within
		// the scene that offers it.
		_, err := h.m.Choose(h.ctx, h.tx, h.account, trial, "stoop", "side-with-her")
		if !errors.Is(err, ErrNoOption) {
			t.Fatalf("Choose = %v, want ErrNoOption", err)
		}
	})
}

func TestARunThatWasNeverBegunCannotBeChosenIn(t *testing.T) {
	with(t, func(h harness) {
		_, err := h.m.Choose(h.ctx, h.tx, h.account, trial, "stoop", "listen")
		if !errors.Is(err, ErrNotRunning) {
			t.Fatalf("Choose = %v, want ErrNotRunning", err)
		}
	})
}

func TestAChallengeNobodyAuthoredIsRefused(t *testing.T) {
	with(t, func(h harness) {
		if _, err := h.m.Begin(h.ctx, h.tx, h.account, "jury_duty/a", 0); !errors.Is(err, ErrUnknown) {
			t.Fatalf("Begin = %v, want ErrUnknown", err)
		}
	})
}

/* ── finishing ─────────────────────────────────────────────────────────── */

/*
§5.1's authored hidden factor deltas, **applied on completion**.

Not as the player goes: paying per choice would make abandoning part way a way
of taking the credit and leaving the consequences, and it would turn a
six-minute endeavour into a series of small transactions.

They arrive summed per factor and in the simulation's own order, so two players
who made the same choices in a different sequence are paid identically — the
last place chance could have crept into this package (§7).
*/
func TestTheAuthoredDeltasAreAppliedOnCompletionAndNotBefore(t *testing.T) {
	with(t, func(h harness) {
		if _, err := h.m.Begin(h.ctx, h.tx, h.account, trial, 0); err != nil {
			t.Fatalf("Begin: %v", err)
		}
		if _, err := h.m.Choose(h.ctx, h.tx, h.account, trial, "stoop", "listen"); err != nil {
			t.Fatalf("Choose: %v", err)
		}
		if h.paid.calls != 0 {
			t.Fatalf("%d factor awards part way through, want none", h.paid.calls)
		}

		run, err := h.m.Choose(h.ctx, h.tx, h.account, trial, "kitchen", "side-with-her")
		if err != nil {
			t.Fatalf("Choose: %v", err)
		}
		if !run.Done {
			t.Fatal("the run is not done after an option with nothing after it")
		}
		if h.paid.calls != 1 {
			t.Fatalf("%d factor awards on completion, want exactly one", h.paid.calls)
		}
		want := []simulation.Grant{
			{Factor: simulation.Stability, Credit: 2},
			{Factor: simulation.Kindness, Credit: 3},
		}
		if len(h.paid.grants) != len(want) {
			t.Fatalf("grants = %+v, want %+v", h.paid.grants, want)
		}
		for i, g := range want {
			if h.paid.grants[i] != g {
				t.Errorf("grant %d = %+v, want %+v", i, h.paid.grants[i], g)
			}
		}
	})
}

/*
§5.1's outcome buckets: the trail scores, and the score picks a band with
authored reaction text. The number is never the player's business — it is here
so the band can be chosen and so a finished run reads back honestly.
*/
func TestTheOutcomeIsTheBandTheTrailScoredInto(t *testing.T) {
	for _, tc := range []struct {
		name   string
		scene  string
		option string
		want   string
		score  int
	}{
		{"walking on", "stoop", "walk-on", "dismissed", -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			with(t, func(h harness) {
				if _, err := h.m.Begin(h.ctx, h.tx, h.account, trial, 0); err != nil {
					t.Fatalf("Begin: %v", err)
				}
				run, err := h.m.Choose(h.ctx, h.tx, h.account, trial, tc.scene, tc.option)
				if err != nil {
					t.Fatalf("Choose: %v", err)
				}
				if !run.Done || run.Outcome.Key != tc.want || run.Score != tc.score {
					t.Fatalf("run = done %v, outcome %q, score %d; want %q at %d",
						run.Done, run.Outcome.Key, run.Score, tc.want, tc.score)
				}
				if run.Outcome.Reaction == "" {
					t.Error("the outcome carries no reaction, which is what the player is shown")
				}
				if h.paid.calls != 0 {
					t.Errorf("%d awards for a trail whose deltas are all zero, want none", h.paid.calls)
				}
			})
		})
	}

	// The other way through the same content lands somewhere else, which is
	// §3.10's "there is no correct one; each leads somewhere different".
	with(t, func(h harness) {
		if _, err := h.m.Begin(h.ctx, h.tx, h.account, trial, 0); err != nil {
			t.Fatalf("Begin: %v", err)
		}
		if _, err := h.m.Choose(h.ctx, h.tx, h.account, trial, "stoop", "listen"); err != nil {
			t.Fatalf("Choose: %v", err)
		}
		run, err := h.m.Choose(h.ctx, h.tx, h.account, trial, "kitchen", "side-with-him")
		if err != nil {
			t.Fatalf("Choose: %v", err)
		}
		if run.Outcome.Key != "heard" || run.Score != 2 {
			t.Errorf("outcome = %q at %d, want heard at 2", run.Outcome.Key, run.Score)
		}
	})
}

/*
§4's line between the two systems, from this side: an event marked `awaits` is a
doorway, and the run settles it when it knows how it went — in the same
transaction as finishing, so an event that was resolved and a run that is not
finished is not a state this game has.
*/
func TestTheEventThatOpenedItIsSettledWhenTheRunFinishes(t *testing.T) {
	with(t, func(h harness) {
		if _, err := h.m.Begin(h.ctx, h.tx, h.account, trial, 41); err != nil {
			t.Fatalf("Begin: %v", err)
		}
		if h.settled.calls != 0 {
			t.Fatal("the event was settled before there was an outcome")
		}

		h.clk.Advance(4 * time.Minute)
		if _, err := h.m.Choose(h.ctx, h.tx, h.account, trial, "stoop", "walk-on"); err != nil {
			t.Fatalf("Choose: %v", err)
		}
		if h.settled.calls != 1 || h.settled.id != 41 {
			t.Fatalf("settled = %d calls for event %d, want one for 41", h.settled.calls, h.settled.id)
		}
		if !h.settled.at.Equal(h.clk.Now()) {
			t.Errorf("settled at %v, want the moment the run finished", h.settled.at)
		}
	})
}

// A challenge entered from the world settles nothing, because nothing opened it.
func TestARunNobodyOpenedSettlesNoEvent(t *testing.T) {
	with(t, func(h harness) {
		if _, err := h.m.Begin(h.ctx, h.tx, h.account, trial, 0); err != nil {
			t.Fatalf("Begin: %v", err)
		}
		if _, err := h.m.Choose(h.ctx, h.tx, h.account, trial, "stoop", "walk-on"); err != nil {
			t.Fatalf("Choose: %v", err)
		}
		if h.settled.calls != 0 {
			t.Errorf("%d events settled by a run nothing opened", h.settled.calls)
		}
	})
}

/*
§5.3's whole argument for three columns rather than one flat string: "has this
player done a resolve-conflict-shaped thing?" is a real query about the family
rather than an unindexable prefix match over a code.
*/
func TestTheFamilyIsAskableSeparatelyFromTheInstance(t *testing.T) {
	withCatalogue(t, []Definition{authored(t, "a"), authored(t, "b")}, func(h harness) {
		shaped, err := h.m.Shaped(h.ctx, h.tx, h.account, "resolve_conflict")
		if err != nil || shaped {
			t.Fatalf("Shaped before anything = %v, %v; want false", shaped, err)
		}

		if _, err := h.m.Begin(h.ctx, h.tx, h.account, "resolve_conflict/b", 0); err != nil {
			t.Fatalf("Begin: %v", err)
		}
		if shaped, err = h.m.Shaped(h.ctx, h.tx, h.account, "resolve_conflict"); err != nil || shaped {
			t.Fatalf("Shaped part way through = %v, %v; want false", shaped, err)
		}

		if _, err := h.m.Choose(h.ctx, h.tx, h.account, "resolve_conflict/b", "stoop", "walk-on"); err != nil {
			t.Fatalf("Choose: %v", err)
		}
		if shaped, err = h.m.Shaped(h.ctx, h.tx, h.account, "resolve_conflict"); err != nil || !shaped {
			t.Fatalf("Shaped after finishing a variant = %v, %v; want true", shaped, err)
		}
		// The instance is still its own question: the other variant is untouched.
		played, err := h.m.Played(h.ctx, h.tx, h.account, "resolve_conflict/a")
		if err != nil || played {
			t.Fatalf("Played of the other variant = %v, %v; want false", played, err)
		}
	})
}

// finish plays the shortest complete run there is.
func finish(t *testing.T, h harness) {
	t.Helper()
	if _, err := h.m.Begin(h.ctx, h.tx, h.account, trial, 0); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if _, err := h.m.Choose(h.ctx, h.tx, h.account, trial, "stoop", "walk-on"); err != nil {
		t.Fatalf("Choose: %v", err)
	}
}
