/*
Package challenge owns §5: the machinery every challenge runs on.

technical.md §5 is the specification and §3.10 is what it is for — "a challenge
is a big endeavour, not a small interaction… roughly six minutes… this is the
game's main content and its largest production cost". Building one is an epic.
Building the thing they all stand on is this package, and §3.10 says outright
that it "should exist first".

# What it is

Three responsibilities, and the boundaries between them are the design.

  - **The catalogue** (catalogue.go). Definitions are repo files, loaded and
    validated at boot, and the identity of one is the path it lives at (§5.3).
  - **The mechanic registry** (mechanic.go). §5.2: mechanic types are code,
    challenges are data. A type validates its own config document; the framework
    validates the plan that comes out of it.
  - **Runs** (this file). One player's passage through one challenge — begun,
    checkpointed at each scene boundary, resumed, and finished once ever.

# The two rules that shape the run

**Once ever, per player** (§5.1). `challenge_run` carries a unique constraint on
(player, challenge), and there is no code path here that starts a second one.
A challenge is a committed judgement with authored consequences; a replay would
be the player discovering that none of it was load-bearing.

**Save and resume is mandatory.** §5.1: "even at 6 minutes, players abandon
partway. In-flight run state is checkpointed at scene boundaries." *At scene
boundaries* rather than on a timer is the whole of it — a timer would put a
returning player back into the middle of a sentence, and the only place a run
can coherently be picked up is somewhere the content decided was a place to
stand. So the checkpoint moves exactly when an option is committed, in the same
transaction, and nowhere else.

# What this package refuses to know

Two things, each behind an interface (§3.1):

  - **Where a factor goes** — [Awards]. §5.1's authored deltas are applied on
    completion, and this package hands them over without ever learning what they
    came to. It cannot read a score, which is §3.5's hidden factors honoured on
    this side of the boundary.
  - **What opened it** — [Doorway]. §4 keeps the event queue and the challenge
    catalog as "distinct systems, not one abstraction", and this is that line: an
    event marked `awaits` is a doorway, and when the run finishes it settles the
    event through the one method internal/event exposes for it.

It owns `challenge` and `challenge_run`, takes the caller's transaction and
never opens one (§3.2), and reads no other module's tables.
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

	// ErrMoved is a choice made at a scene the run has already left.
	//
	// Its own error because it is not a mistake: it is the second tab, or the
	// resent request, of somebody who answered once. The caller can say "you
	// have already moved on" and re-state where they actually are, which is a
	// far better answer than applying the choice twice.
	ErrMoved = errors.New("challenge: the run has moved on from there")

	// ErrNoOption is an option that is not one of the ones on offer.
	ErrNoOption = errors.New("challenge: that is not one of the choices")
)

/*
Awards is where a completed run's authored deltas go.

One method, and it returns an error rather than the standing internal/factor
actually hands back — the same one-way shape peer endorsement's grants take, and
for the same reason. A module that could read what a challenge did to a hidden
factor is a module somebody eventually renders, and §3.10 is explicit that
scoring is "never surfaced as a grade, never confirmed as right".
*/
type Awards interface {
	Award(ctx context.Context, tx pgx.Tx, accountID string, grants ...simulation.Grant) error
}

// Bare is an Awards that pays nothing. For tests about the machinery rather
// than about progression; never bound in the server.
type Bare struct{}

func (Bare) Award(context.Context, pgx.Tx, string, ...simulation.Grant) error { return nil }

/*
Purse is where money a run put in the player's pocket goes.

The same one-way shape as [Awards], and for a reason worth stating separately
because money is the visible half of §3.5. This hands an amount over and cannot
read a balance — so nothing in an authored challenge can branch on how much the
player has, which would make a moral choice a budgeting one and would make the
same content read differently to a rich player and a poor one.

It is a second interface rather than a second method on [Awards] because they
are answered by different modules for different reasons: a factor delta is
internal/factor's hidden credit, and this is internal/character's balance moving
because an object changed hands. Folding them together would be one adapter that
had to hold both, which is the composition root's job and not this package's.
*/
type Purse interface {
	Keep(ctx context.Context, tx pgx.Tx, accountID string, amount simulation.Money) error
}

// Empty is a Purse that carries nothing. For tests about the machinery rather
// than about the balance; never bound in the server.
type Empty struct{}

func (Empty) Keep(context.Context, pgx.Tx, string, simulation.Money) error { return nil }

/*
Doorway settles the event that opened a run, if one did.

§4's seam, from this side: "an event *can* offer a challenge… they are distinct
systems, not one abstraction". internal/event marks such an event `awaits` and
leaves its record pending; this is the call that closes it, made in the same
transaction as the run finishing so the pair commit together.
*/
type Doorway interface {
	Resolve(ctx context.Context, tx pgx.Tx, accountID string, id int64, at time.Time) error
}

// Unopened is a Doorway that settles nothing. For tests about runs that were
// entered from the world rather than through an event.
type Unopened struct{}

func (Unopened) Resolve(context.Context, pgx.Tx, string, int64, time.Time) error { return nil }

/*
Step is one committed choice, as the trail keeps it.

The scene as well as the option, because an option key is only unique within its
scene — and because the trail is meant to read back as a passage through the
content rather than as a list of button presses.
*/
type Step struct {
	Scene  string `json:"scene"`
	Option string `json:"option"`
}

/*
Run is one player's passage through one challenge.

It is what a resume lands on, so everything a screen needs to re-establish the
moment is here: which challenge, which scene, and what has been committed. The
score is not a grade and is deliberately not something the player is told; it is
here because the module that finishes a run has to bucket it, and because a
finished run's own history has to be readable afterwards.
*/
type Run struct {
	Challenge Definition
	// Scene is the checkpoint: where the run is standing. For a finished run it
	// is the scene it ended on.
	Scene Scene
	// Trail is everything committed, oldest first.
	Trail []Step

	Done bool
	// Outcome is the bucket the trail scored into, and is only meaningful once
	// the run is done. Its reaction text is what the player is actually shown
	// (§5.1) — never the number underneath it.
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
Publish upserts every validated definition into the catalog table (§5.2).

The table exists so foreign keys work and for no other reason, so this writes
identity and provenance and nothing else — no name, no scene, no option, no
delta. §5.2: "the database stores *runs*, never definitions."

Anything in the catalog that is no longer in the repo is **retired rather than
deleted**. A withdrawn challenge has runs behind it, and a player's history must
not be removed by an author tidying up; retiring also makes "this used to exist"
answerable, which deleting does not.

Called at boot, and again after a hot reload in development — which is why it is
idempotent and why it re-publishes rather than diffing.
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
Begin starts a challenge, or hands back the one already in progress.

The two are one method deliberately. §5.1's save-and-resume means "start" is not
a thing a player asks for twice — they open the challenge, and either they have
never been in it or they are part way through it, and in both cases what they
want is the scene they are standing at. Making resume a separate call would
create a state where a client could ask for the wrong one.

`opened` is the event backlog row this came through, or zero for a challenge
entered from the world. It is recorded on the run so the event can be settled
when the run finishes, and it is ignored on a resume — the doorway a player came
through the first time is the one they came through.

Finishing is once ever (§5.1), so a completed challenge is [ErrDone] rather than
a fresh run.
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

/*
Of is the run this player has in progress, if any.

What a client asks on load, so a player who closed the tab in the middle of a
conversation is put back where they were rather than at a list. At most one:
somebody who abandoned two challenges part way is somebody the world has lost
track of, so the most recently moved is the answer and the others wait.
*/
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
		// Authored out of the game while somebody was part way through it.
		// There is nothing to show and nothing to choose; the run stays in the
		// table because it is history.
		return Run{}, false, nil
	}
	return c.held(ctx, tx, accountID, authored, false)
}

// Played reports whether this player has finished a challenge, by code.
//
// §5.3's real query, asked of one instance. What it is deliberately not is a
// list of everything they have done: that is history, and history is read a page
// at a time rather than carried around.
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
Choose commits one option, and moves the checkpoint.

This is §5.1's save-and-resume, and every line of it is that sentence:

  - The scene is named by the caller, so a stale tab answering a question the
    run has already left gets [ErrMoved] instead of a second choice applied.
  - The trail and the new scene are written in **one** statement,
    guarded on the run still being at the scene it was answering — so two tabs
    pressing the same button produce one step.
  - The write happens *because* the run left a scene, and at no other time.
    There is no timer here and there must never be one.

An option with nothing after it finishes the run — see [Challenges.finish],
which is where §5.1's authored deltas are actually paid.
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
	// Guarded on the scene as well as the state: the read above took the row
	// for update, and this restates the guard so the statement is correct on
	// its own terms rather than only in the company of the read.
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

The order is not arbitrary. The row is settled first, guarded on it still being
in progress, so a second finish is impossible before anything irreversible
happens — and only then are the factors moved. A run that paid twice because two
requests raced would be the one bug in this package nobody could unpick from the
outside.

**The deltas are paid here and only here** (§5.1: "each option carries authored
hidden factor deltas"), summed over the whole trail rather than applied as the
player goes. Paying per choice would make abandoning part way a way of taking
the credit and leaving the consequences, and it would make the six-minute
endeavour a series of small transactions — which is the reading of §3.10 this
whole package is arranged against.
*/
func (c *Challenges) finish(
	ctx context.Context, tx pgx.Tx, accountID string, authored Definition,
	held Run, trail []Step, now time.Time,
) (Run, error) {
	score := tally(authored.Plan, trail)
	bucket, ok := authored.Plan.Outcome(score)
	if !ok {
		// Unreachable for a validated plan — [Plan.buckets] refuses one whose
		// lowest outcome is above the worst trail — and worth saying out loud
		// rather than finishing a challenge with nothing to say.
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

	// Whatever the player walked out with, in the same transaction and by the
	// same argument as the deltas: summed over the whole trail and paid once, at
	// the end, so abandoning part way through is not a way of keeping what you
	// picked up without deciding what to do with it.
	if kept := takings(authored.Plan, trail); kept > 0 {
		if err := c.purse.Keep(ctx, tx, accountID, kept); err != nil {
			return Run{}, err
		}
	}

	// §4's other half of the seam: the event that opened this is settled now
	// that there is an outcome, in the same transaction.
	if held.Opened > 0 {
		if err := c.doorway.Resolve(ctx, tx, accountID, held.Opened, now); err != nil {
			return Run{}, err
		}
	}

	out, _, err := c.held(ctx, tx, accountID, authored, false)
	return out, err
}

/*
tally is what a trail scores.

Derived on read rather than kept in a column, which is §3.3's rule taken
literally: "everything derived is computed on read, never stored as a stale
number. Only the inputs are persisted." The inputs are the trail and the plan,
and both are to hand. What *is* stored is the bucket the score fell in, because
that is not a derivation — it is what the player was told, and an author
retuning a threshold next month must not change it retroactively.

A step naming a scene or an option the content no longer has counts for nothing
rather than failing. That is the same tolerance the record everywhere else in
this codebase has for withdrawn content: history is what happened, and it does
not stop being readable because somebody edited a file.
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

/*
deltas is what a whole trail moves, one grant per factor.

Summed per factor and emitted in the simulation's own order rather than in the
order the player happened to meet them, so two players who made the same choices
in a different sequence are paid identically. Nothing in this package is decided
by chance (§7), and this is the last place it could have crept in.

A factor whose deltas cancel out is dropped: handing internal/factor a grant of
zero would wear the environment down (see factor.Record) for a choice that
amounted to nothing.
*/
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

/*
takings is what a whole trail put in the player's pocket.

Summed the same way the deltas are, and tolerant of withdrawn content for the
same reason: a step naming a scene an author has since deleted counts for
nothing rather than failing, because history is what happened and it does not
stop being readable because somebody edited a file.

The one asymmetry with [deltas] is that there is nothing to order. Money is one
quantity rather than five, so two players who made the same choices in a
different sequence are paid identically without anything having to arrange for
it.
*/
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

// held reads one player's run of one challenge.
//
// `lock` takes the row for update, which every path that is about to write one
// does: a challenge is answered from a screen somebody may have open twice, and
// the read-then-write in [Challenges.Choose] has to be one decision.
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
	// Derived on read from the trail, never stored (§3.3): a column for it
	// would be a second copy of a sum whose inputs are in the row beside it.
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
	// author has since deleted reads as an empty scene with no options — which
	// a caller can see and refuse, instead of a run that offers choices that do
	// not exist.
	if s, ok := authored.Plan.Scene(scene); ok {
		out.Scene = s
	} else {
		out.Scene = Scene{Key: scene}
	}
	return out, true, nil
}
