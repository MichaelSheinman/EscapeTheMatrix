/*
Package challenge owns §5: the machinery every challenge runs on. A challenge is
a big endeavour of roughly six minutes rather than a small interaction, and it is
the game's main content and largest production cost (§3.10).

Three responsibilities:

  - The catalogue (catalogue.go). Definitions are repo files, loaded and
    validated at boot, and the identity of one is the path it lives at (§5.3).
  - The mechanic registry (mechanic.go). Mechanic types are code and challenges
    are data (§5.2): a type validates its own config document, and the framework
    validates the plan that comes out of it.
  - Runs (this file). One player's passage through one challenge — begun,
    checkpointed at each scene boundary, resumed, and finished once ever.

Two rules shape a run. Once ever per player (§5.1): challenge_run has a unique
constraint on (player, challenge) and no code path here starts a second one,
because a replay would tell the player none of it was load-bearing.

And save-and-resume is mandatory. State is checkpointed at scene boundaries
rather than on a timer, because the only place a run can be picked up is
somewhere the content decided was a place to stand. The checkpoint moves when an
option is committed, in the same transaction, and nowhere else.

Two things this package refuses to know, each behind an interface (§3.1): where a
factor goes ([Awards], which applies §5.1's deltas on completion without ever
learning what they came to), and what opened it ([Doorway] — an event marked
awaits settles through the one method internal/event exposes for it, which is the
line §4 draws between the two systems).

It owns challenge and challenge_run, takes the caller's transaction and never
opens one (§3.2), and reads no other module's tables.
*/
package challenge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

var (
	// ErrUnknown is a code that names no authored challenge. A challenge
	// withdrawn from content is the ordinary case: the catalog row survives so
	// old runs keep their foreign key, and nobody can start a new one.
	ErrUnknown = errors.New("challenge: there is no such challenge")

	// ErrDone is §5.1's "once ever, per player", refused.
	ErrDone = errors.New("challenge: that has already been played")

	// ErrNotRunning is choosing in a challenge that was never begun, or that
	// has finished.
	ErrNotRunning = errors.New("challenge: that challenge is not in progress")

	// ErrMoved is a choice made at a scene the run has already left — the second
	// tab, or the resent request, of somebody who answered once. Its own error
	// so the caller can re-state where they actually are rather than applying
	// the choice twice.
	ErrMoved = errors.New("challenge: the run has moved on from there")

	// ErrNoOption is an option that is not one of the ones on offer.
	ErrNoOption = errors.New("challenge: that is not one of the choices")
)

// Awards is where a completed run's authored deltas go. It returns an error
// rather than the standing internal/factor hands back: a module that could read
// what a challenge did to a hidden factor is one somebody eventually renders,
// and scoring is never surfaced as a grade (§3.10).
type Awards interface {
	Award(ctx context.Context, tx pgx.Tx, accountID string, grants ...simulation.Grant) error
}

// Bare is an Awards that pays nothing. For tests about the machinery rather
// than about progression; never bound in the server.
type Bare struct{}

func (Bare) Award(context.Context, pgx.Tx, string, ...simulation.Grant) error { return nil }

/*
Purse is where money a run put in the player's pocket goes. It hands an amount
over and cannot read a balance, so nothing in an authored challenge can branch on
how much the player has — which would make a moral choice a budgeting one and
make the same content read differently to a rich player and a poor one.

A second interface rather than a second method on [Awards], because a factor
delta is internal/factor's hidden credit and this is internal/character's balance
moving.
*/
type Purse interface {
	Keep(ctx context.Context, tx pgx.Tx, accountID string, amount simulation.Money) error
}

// Empty is a Purse that carries nothing. For tests about the machinery rather
// than about the balance; never bound in the server.
type Empty struct{}

func (Empty) Keep(context.Context, pgx.Tx, string, simulation.Money) error { return nil }

// Doorway settles the event that opened a run, if one did — §4's seam from this
// side. internal/event marks such an event awaits and leaves its record pending,
// and this closes it in the same transaction as the run finishing.
type Doorway interface {
	Resolve(ctx context.Context, tx pgx.Tx, accountID string, id int64, at time.Time) error
}

// Unopened is a Doorway that settles nothing. For tests about runs that were
// entered from the world rather than through an event.
type Unopened struct{}

func (Unopened) Resolve(context.Context, pgx.Tx, string, int64, time.Time) error { return nil }

// Step is one committed choice, as the trail keeps it: the scene as well as the
// option, because an option key is only unique within its scene and because the
// trail has to read back as a passage through the content.
type Step struct {
	Scene  string `json:"scene"`
	Option string `json:"option"`
}

// Run is one player's passage through one challenge, and what a resume lands on:
// which challenge, which scene, and what has been committed. The score is here
// because the module that finishes a run has to bucket it and because a finished
// run's history has to be readable afterwards. The player is never told it.
type Run struct {
	Challenge Definition
	// Scene is the checkpoint: where the run is standing. For a finished run it
	// is the scene it ended on.
	Scene Scene
	// Trail is everything committed, oldest first.
	Trail []Step

	Done bool
	// Outcome is the bucket the trail scored into, meaningful once the run is
	// done. Its reaction text is what the player is shown (§5.1).
	Outcome Bucket
	// Score is what the trail added up to, composed on read from the trail and
	// the plan and stored nowhere (§3.3). Never crosses the wire.
	Score int

	Began    time.Time
	Moved    time.Time
	Finished time.Time
	// Opened is the event backlog row this run came through, or zero.
	Opened int64
}

// Challenges is the module. It holds no per-player state.
type Challenges struct {
	clk     simulation.Clock
	awards  Awards
	purse   Purse
	doorway Doorway
}

// New binds the module to the clock it reads and the three things it cannot
// reach otherwise.
func New(clk simulation.Clock, awards Awards, purse Purse, doorway Doorway) *Challenges {
	return &Challenges{clk: clk, awards: awards, purse: purse, doorway: doorway}
}

/* ── publishing the catalogue ──────────────────────────────────────────── */

/*
Publish upserts every validated definition into the catalog table (§5.2). The
table exists so foreign keys work, so this writes identity and provenance and
nothing else. The database stores runs rather than definitions.

Anything in the catalog no longer in the repo is retired rather than deleted: a
withdrawn challenge has runs behind it, and a player's history must survive an
author tidying up.

Called at boot and again after a hot reload in development, which is why it is
idempotent and re-publishes rather than diffing.
*/
func (c *Challenges) Publish(ctx context.Context, tx pgx.Tx) error {
	defined := All()
	now := c.clk.Now()

	codes := make([]string, 0, len(defined))
	for _, d := range defined {
		codes = append(codes, string(d.Code))
		if _, err := tx.Exec(ctx, `
			INSERT INTO challenge (family, variant, mechanic, published_at)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (code) DO UPDATE
			   SET mechanic = EXCLUDED.mechanic,
			       published_at = EXCLUDED.published_at,
			       retired_at = NULL`,
			d.Family, d.Variant, string(d.Mechanic), now,
		); err != nil {
			return fmt.Errorf("challenge: publish %s: %w", d.Code, err)
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE challenge SET retired_at = $1
		 WHERE retired_at IS NULL AND code <> ALL($2::text[])`, now, codes); err != nil {
		return fmt.Errorf("challenge: retire what is no longer authored: %w", err)
	}
	return nil
}

/* ── beginning, and resuming ───────────────────────────────────────────── */

/*
Begin starts a challenge, or hands back the one already in progress. One method,
because a player opens the challenge and either has never been in it or is part
way through it, and in both cases what they want is the scene they are standing
at. A separate resume call would let a client ask for the wrong one.

opened is the event backlog row this came through, or zero for a challenge
entered from the world. It is recorded on the run so the event can be settled
when the run finishes, and it is ignored on a resume.

Finishing is once ever (§5.1), so a completed challenge is [ErrDone].
*/
func (c *Challenges) Begin(ctx context.Context, tx pgx.Tx, accountID string, code Code, opened int64) (Run, error) {
	authored, known := Known(code)
	if !known {
		return Run{}, fmt.Errorf("%w: %q", ErrUnknown, code)
	}

	held, running, err := c.held(ctx, tx, accountID, authored, true)
	switch {
	case err != nil:
		return Run{}, err
	case running && held.Done:
		return Run{}, fmt.Errorf("%w: %q", ErrDone, code)
	case running:
		return held, nil
	}

	now := c.clk.Now()
	var opener *int64
	if opened > 0 {
		opener = &opened
	}

	// ON CONFLICT DO NOTHING against the once-ever constraint, so two tabs
	// opening the same challenge in the same instant produce one run rather
	// than an error on the slower of them. Losing that race means the run
	// exists, which is what this was about.
	tag, err := tx.Exec(ctx, `
		INSERT INTO challenge_run
		       (account_id, challenge, state, scene, trail, opened_by, began_at, moved_at)
		VALUES ($1, $2, 'running', $3, '[]'::jsonb, $4, $5, $5)
		ON CONFLICT (account_id, challenge) DO NOTHING`,
		accountID, string(code), authored.Plan.Opening, opener, now)
	if err != nil {
		return Run{}, fmt.Errorf("challenge: begin %s: %w", code, err)
	}
	if tag.RowsAffected() == 0 {
		held, running, err = c.held(ctx, tx, accountID, authored, false)
		switch {
		case err != nil:
			return Run{}, err
		case !running:
			return Run{}, fmt.Errorf("challenge: begin %s: the run vanished as it was created", code)
		case held.Done:
			return Run{}, fmt.Errorf("%w: %q", ErrDone, code)
		}
		return held, nil
	}

	held, _, err = c.held(ctx, tx, accountID, authored, false)
	return held, err
}

// Of is the run this player has in progress, if any. What a client asks on load,
// so somebody who closed the tab mid-conversation is put back where they were.
// At most one: the most recently moved is the answer and any others wait.
func (c *Challenges) Of(ctx context.Context, tx pgx.Tx, accountID string) (Run, bool, error) {
	var code string
	err := tx.QueryRow(ctx, `
		SELECT challenge FROM challenge_run
		 WHERE account_id = $1 AND state = 'running'
		 ORDER BY moved_at DESC, id DESC
		 LIMIT 1`, accountID).Scan(&code)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Run{}, false, nil
	case err != nil:
		return Run{}, false, fmt.Errorf("challenge: read what is in progress: %w", err)
	}

	authored, known := Known(Code(code))
	if !known {
		// Authored out of the game while somebody was part way through it. The
		// run stays in the table because it is history.
		return Run{}, false, nil
	}
	return c.held(ctx, tx, accountID, authored, false)
}

// Played reports whether this player has finished a challenge, by code — §5.3's
// query, asked of one instance. Everything they have done is history, read a
// page at a time rather than carried around.
func (c *Challenges) Played(ctx context.Context, tx pgx.Tx, accountID string, code Code) (bool, error) {
	var done bool
	err := tx.QueryRow(ctx, `
		SELECT true FROM challenge_run
		 WHERE account_id = $1 AND challenge = $2 AND state = 'finished'`,
		accountID, string(code)).Scan(&done)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("challenge: read what has been played: %w", err)
	}
	return done, nil
}

/*
Shaped reports whether this player has finished anything in a family.

§5.3's argument for three columns rather than a flat code, made good: "has this
player done a resolve-conflict-shaped thing?" is a real query here rather than
an unindexable prefix match, which is what lets a selector avoid repeating a
*shape* rather than just an instance.
*/
func (c *Challenges) Shaped(ctx context.Context, tx pgx.Tx, accountID, family string) (bool, error) {
	var done bool
	err := tx.QueryRow(ctx, `
		SELECT true FROM challenge_run run
		  JOIN challenge ON challenge.code = run.challenge
		 WHERE run.account_id = $1 AND challenge.family = $2 AND run.state = 'finished'
		 LIMIT 1`, accountID, family).Scan(&done)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("challenge: read the family: %w", err)
	}
	return done, nil
}

/* ── the checkpoint ────────────────────────────────────────────────────── */

/*
Choose commits one option and moves the checkpoint — §5.1's save-and-resume:

  - The scene is named by the caller, so a stale tab answering a question the run
    has already left gets [ErrMoved] rather than a second choice applied.
  - The trail and the new scene are written in one statement, guarded on the run
    still being at the scene it was answering, so two tabs pressing the same
    button produce one step.
  - The write happens because the run left a scene and at no other time. There is
    no timer here and there must never be one.

An option with nothing after it finishes the run — see [Challenges.finish], where
the authored deltas are paid.
*/
func (c *Challenges) Choose(
	ctx context.Context, tx pgx.Tx, accountID string, code Code, scene, option string,
) (Run, error) {
	authored, known := Known(code)
	if !known {
		return Run{}, fmt.Errorf("%w: %q", ErrUnknown, code)
	}

	held, running, err := c.held(ctx, tx, accountID, authored, true)
	switch {
	case err != nil:
		return Run{}, err
	case !running, held.Done:
		return Run{}, fmt.Errorf("%w: %q", ErrNotRunning, code)
	case held.Scene.Key != scene:
		return Run{}, fmt.Errorf("%w: it is at %q", ErrMoved, held.Scene.Key)
	}

	chosen, ok := held.Scene.Option(option)
	if !ok {
		return Run{}, fmt.Errorf("%w: %q at %q", ErrNoOption, option, scene)
	}

	now := c.clk.Now()
	trail := append(append([]Step{}, held.Trail...), Step{Scene: scene, Option: option})
	if chosen.Ends() {
		return c.finish(ctx, tx, accountID, authored, held, trail, now)
	}

	written, err := json.Marshal(trail)
	if err != nil {
		return Run{}, fmt.Errorf("challenge: write the trail: %w", err)
	}
	// Guarded on the scene as well as the state, so the statement is correct on
	// its own rather than only alongside the read above.
	tag, err := tx.Exec(ctx, `
		UPDATE challenge_run
		   SET scene = $4, trail = $5, moved_at = $6
		 WHERE account_id = $1 AND challenge = $2 AND state = 'running' AND scene = $3`,
		accountID, string(code), scene, chosen.Then, written, now)
	if err != nil {
		return Run{}, fmt.Errorf("challenge: move %s on: %w", code, err)
	}
	if tag.RowsAffected() == 0 {
		return Run{}, fmt.Errorf("%w: %q", ErrMoved, scene)
	}

	held, _, err = c.held(ctx, tx, accountID, authored, false)
	return held, err
}

/*
finish ends a run: the bucket, the deltas, and the event that opened it.

The row is settled first, guarded on it still being in progress, so a second
finish is impossible before anything irreversible happens. Only then are the
factors moved — a run that paid twice because two requests raced could not be
unpicked from the outside.

The deltas are paid here and only here (§5.1), summed over the whole trail rather
than applied as the player goes. Paying per choice would make abandoning part way
a way of taking the credit and leaving the consequences.
*/
func (c *Challenges) finish(
	ctx context.Context, tx pgx.Tx, accountID string, authored Definition,
	held Run, trail []Step, now time.Time,
) (Run, error) {
	score := tally(authored.Plan, trail)
	bucket, ok := authored.Plan.Outcome(score)
	if !ok {
		// Unreachable for a validated plan, since [Plan.buckets] refuses one
		// whose lowest outcome is above the worst trail. Reported rather than
		// finishing a challenge with nothing to say.
		return Run{}, fmt.Errorf("challenge: %s scored %d, which is in no outcome", authored.Code, score)
	}

	written, err := json.Marshal(trail)
	if err != nil {
		return Run{}, fmt.Errorf("challenge: write the trail: %w", err)
	}
	last := trail[len(trail)-1]
	tag, err := tx.Exec(ctx, `
		UPDATE challenge_run
		   SET state = 'finished', scene = $4, trail = $5,
		       bucket = $6, moved_at = $7, finished_at = $7
		 WHERE account_id = $1 AND challenge = $2 AND state = 'running' AND scene = $3`,
		accountID, string(authored.Code), last.Scene, last.Scene, written, bucket.Key, now)
	if err != nil {
		return Run{}, fmt.Errorf("challenge: finish %s: %w", authored.Code, err)
	}
	if tag.RowsAffected() == 0 {
		return Run{}, fmt.Errorf("%w: %q", ErrMoved, last.Scene)
	}

	if grants := deltas(authored.Plan, trail); len(grants) > 0 {
		if err := c.awards.Award(ctx, tx, accountID, grants...); err != nil {
			return Run{}, err
		}
	}

	// Whatever the player walked out with, in the same transaction as the
	// deltas: summed over the whole trail and paid once at the end, so
	// abandoning part way is not a way of keeping what you picked up.
	if kept := takings(authored.Plan, trail); kept > 0 {
		if err := c.purse.Keep(ctx, tx, accountID, kept); err != nil {
			return Run{}, err
		}
	}

	// The event that opened this is settled now that there is an outcome, in the
	// same transaction (§4).
	if held.Opened > 0 {
		if err := c.doorway.Resolve(ctx, tx, accountID, held.Opened, now); err != nil {
			return Run{}, err
		}
	}

	out, _, err := c.held(ctx, tx, accountID, authored, false)
	return out, err
}

/*
tally is what a trail scores, derived on read from the trail and the plan rather
than kept in a column (§3.3). The bucket the score fell in is stored, because
that is what the player was told and an author retuning a threshold next month
must not change it retroactively.

A step naming a scene or option the content no longer has counts for nothing
rather than failing: history is what happened, and it does not stop being
readable because somebody edited a file.
*/
func tally(plan Plan, trail []Step) int {
	out := 0
	for _, step := range trail {
		scene, ok := plan.Scene(step.Scene)
		if !ok {
			continue
		}
		if option, ok := scene.Option(step.Option); ok {
			out += option.Weight
		}
	}
	return out
}

// deltas is what a whole trail moves, one grant per factor. Summed per factor
// and emitted in the simulation's own order rather than the order the player met
// them, so two players who made the same choices in a different sequence are
// paid identically (§7). A factor whose deltas cancel out is dropped: a grant of
// zero would still wear the environment down (see factor.Record).
func deltas(plan Plan, trail []Step) []simulation.Grant {
	totals := map[simulation.Factor]float64{}
	for _, step := range trail {
		scene, ok := plan.Scene(step.Scene)
		if !ok {
			continue
		}
		option, ok := scene.Option(step.Option)
		if !ok {
			continue
		}
		for _, d := range option.Deltas {
			totals[d.Factor] += d.Credit
		}
	}

	out := make([]simulation.Grant, 0, len(totals))
	for _, f := range simulation.Factors {
		if credit := totals[f]; credit != 0 {
			out = append(out, simulation.Grant{Factor: f, Credit: credit})
		}
	}
	return out
}

// takings is what a whole trail put in the player's pocket. Summed like [deltas]
// and tolerant of withdrawn content for the same reason. Money is one quantity
// rather than five, so nothing has to arrange for two players who made the same
// choices in a different sequence to be paid identically.
func takings(plan Plan, trail []Step) simulation.Money {
	var out simulation.Money
	for _, step := range trail {
		scene, ok := plan.Scene(step.Scene)
		if !ok {
			continue
		}
		if option, ok := scene.Option(step.Option); ok {
			out += option.Takings
		}
	}
	return out
}

/* ── reading a run back ────────────────────────────────────────────────── */

// held reads one player's run of one challenge. lock takes the row for update,
// which every path about to write one does: a challenge is answered from a
// screen somebody may have open twice, and the read-then-write in
// [Challenges.Choose] has to be one decision.
func (c *Challenges) held(
	ctx context.Context, tx pgx.Tx, accountID string, authored Definition, lock bool,
) (Run, bool, error) {
	sql := `
		SELECT state, scene, trail, bucket, opened_by, began_at, moved_at, finished_at
		  FROM challenge_run
		 WHERE account_id = $1 AND challenge = $2`
	if lock {
		sql += " FOR UPDATE"
	}

	var (
		state    string
		scene    string
		trail    []byte
		bucket   *string
		opened   *int64
		finished *time.Time
		out      = Run{Challenge: authored}
	)
	err := tx.QueryRow(ctx, sql, accountID, string(authored.Code)).
		Scan(&state, &scene, &trail, &bucket, &opened, &out.Began, &out.Moved, &finished)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Run{}, false, nil
	case err != nil:
		return Run{}, false, fmt.Errorf("challenge: read the run of %s: %w", authored.Code, err)
	}

	if err := json.Unmarshal(trail, &out.Trail); err != nil {
		return Run{}, false, fmt.Errorf("challenge: read the trail of %s: %w", authored.Code, err)
	}
	out.Done = state == "finished"
	// Derived on read from the trail (§3.3): a column would be a second copy of
	// a sum whose inputs are in the row beside it.
	out.Score = tally(authored.Plan, out.Trail)
	if opened != nil {
		out.Opened = *opened
	}
	if finished != nil {
		out.Finished = *finished
	}
	if bucket != nil {
		for _, b := range authored.Plan.Buckets {
			if b.Key == *bucket {
				out.Outcome = b
			}
		}
		// An outcome the content no longer has is left empty rather than
		// invented. The key is still in the row, which is where history lives.
	}

	// The scene is looked up rather than trusted, so a checkpoint at a scene an
	// author has since deleted reads as an empty scene with no options, which a
	// caller can see and refuse.
	if s, ok := authored.Plan.Scene(scene); ok {
		out.Scene = s
	} else {
		out.Scene = Scene{Key: scene}
	}
	return out, true, nil
}
