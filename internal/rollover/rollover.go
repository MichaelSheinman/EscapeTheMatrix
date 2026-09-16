/*
Package rollover advances the world one day at a time, once per player, at that
player's own local midnight (§3.3). One game day is one real calendar day, and
there is no global tick: nothing in the design has sub-day granularity.

The schedule is a chain rather than a list the world walks — each rollover
schedules its own successor only if the player has been seen recently, so a
dormant account costs nothing. Their next sign-in puts them back on the chain and
catches them up, because Wake runs the rollover before re-arming the schedule.

Everything here is somebody else's rule, run in the right order on the right
date: the body's rules through internal/health, the day being lived from
internal/routine, the timezone from internal/character. This package owns one
table — rollover, the ledger of which day each player has been advanced through —
and reads no other module's (§3.1).
*/
package rollover

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/character"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/health"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/routine"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
)

// Dormant is how long a player may go unplayed before the world stops
// scheduling work for them. Nothing is lost by falling off it: the next sign-in
// catches them up.
const Dormant = 14 * 24 * time.Hour

// Window is the furthest back a returning player is caught up from. Anything
// older is counted as skipped and the ledger is stamped forward.
const Window = 30

// Settle is how long after local midnight a rollover is scheduled for. Not zero:
// the worker reads the player's day off the clock when it runs, and a job picked
// up a moment early would compute yesterday's date and do nothing.
const Settle = 5 * time.Minute

/*
Queue is where the next rollover is put. An interface rather than the queue
itself, so this package does not import riverqueue/river (§3.4) and a test can
assert what would have been scheduled without running a worker.

Schedule takes the caller's transaction: a job enqueued by a rollover that then
failed to commit never existed, so a ledger that moved forward without arming its
successor is unreachable rather than a state to recover from.
*/
type Queue interface {
	Schedule(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date, at time.Time) error
}

// Nowhere is a queue that drops what it is given — the binding a Rollovers is
// constructed with, so a server that has not started its workers still rolls
// players over on the paths a request drives. It changes when the next rollover
// happens, never whether one is correct.
type Nowhere struct{}

func (Nowhere) Schedule(context.Context, pgx.Tx, string, simulation.Date, time.Time) error {
	return nil
}

// Activity is what this package is allowed to know about how recently an
// account was played: one timestamp and the ability to touch it, and no way to
// reach anything else internal/account owns (§3.1).
type Activity interface {
	Seen(ctx context.Context, tx pgx.Tx, accountID string) error
	Active(ctx context.Context, tx pgx.Tx, accountID string, within time.Duration) (bool, error)
}

// World is the rest of §3.3's rollover, as interfaces so that the module owning
// each piece can be swapped without editing this package.
type World interface {
	// Replenish tops the event backlog up for the day now beginning (§4).
	Replenish(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date) error
	// Decay applies Karma decay for the environment the player is in, for one
	// day (§3.15). Per day rather than per run, because decay is a curve against
	// elapsed days: a player caught up from a week away loses a week of it.
	Decay(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date) error
	// Recompute recomputes environment ceilings and factor caps (§3.5, §3.15),
	// once per run because a ceiling is a statement about now.
	Recompute(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date) error
	// Escalate puts what the playbook could not handle in front of the player
	// (§4). Once per run rather than per day, and capped by its implementation.
	// It is the one piece of the day that also runs when no day is due — see
	// [Rollovers.Advance].
	Escalate(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date) error
	// Delegated runs a day of a job the player does not sit inside (§3.14). Per
	// day, since an active role generates situations while the player is
	// elsewhere. It runs after the day has been lived, because a situation is
	// something the job produced.
	Delegated(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date) error
	// Among runs a day of living beside the people the player lives beside
	// (§3.15's daily conflict). Per day, and last of the day's work.
	Among(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date) error
}

/*
Employer is the job half of a day (§3.14): the shift takes hours out of the day
and the tier pays for it. One call, because they are one fact about that day, and
it happens once per day with nobody asking rather than on a command — §3.4 makes
income structural.

What comes back is the day as it was actually lived: the [simulation.Day] the
routines produced with the shift already taken out, so internal/health is handed
one day rather than a day and a correction.

It is asked for every day the rollover walks. Paying is idempotent in the date on
the far side of this interface, where its ledger lives.
*/
type Employer interface {
	Shift(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date, d simulation.Day) (simulation.Day, error)
}

// Unemployed is an Employer for a world with no jobs in it: the day is the
// day the routines produced, and nobody is paid.
type Unemployed struct{}

func (Unemployed) Shift(_ context.Context, _ pgx.Tx, _ string, _ simulation.Date, d simulation.Day) (simulation.Day, error) {
	return d, nil
}

// Unbuilt is the World nobody has written yet.
type Unbuilt struct{}

func (Unbuilt) Replenish(context.Context, pgx.Tx, string, simulation.Date) error { return nil }
func (Unbuilt) Decay(context.Context, pgx.Tx, string, simulation.Date) error     { return nil }
func (Unbuilt) Recompute(context.Context, pgx.Tx, string, simulation.Date) error { return nil }
func (Unbuilt) Escalate(context.Context, pgx.Tx, string, simulation.Date) error  { return nil }
func (Unbuilt) Delegated(context.Context, pgx.Tx, string, simulation.Date) error { return nil }
func (Unbuilt) Among(context.Context, pgx.Tx, string, simulation.Date) error     { return nil }

// Announce is how a rollover tells a player's open tabs that their world moved
// — §8's push half. Message shapes live in internal/api next to the views they
// carry, so this package says that something happened and never what it looks
// like.
type Announce interface {
	Rolled(ctx context.Context, tx pgx.Tx, accountID string, r Report) error
}

// Silence is an Announce with nobody listening.
type Silence struct{}

func (Silence) Rolled(context.Context, pgx.Tx, string, Report) error { return nil }

// Report is what one rollover did. Days and Skipped are a pair: a player away
// for a season has a large Skipped and a Days of [Window], and reporting only
// the first would make a catch-up look like a full replay.
type Report struct {
	// Through is the local day the player is now advanced through. Zero when
	// nothing was due.
	Through simulation.Date
	// From is the first day actually lived, and Days how many were.
	From simulation.Date
	Days int
	// Skipped is how many days fell outside the catch-up window.
	Skipped int
	// Effects are the pure core's, from the last day applied — the last rather
	// than all of them, because a threshold crossed three weeks ago is not news.
	Effects simulation.Effects
	// Next is when the following rollover is due, and zero when none was
	// scheduled because the player has gone dormant.
	Next time.Time
}

// Ran reports whether this rollover advanced the world at all.
func (r Report) Ran() bool { return r.Days > 0 || r.Skipped > 0 }

// Rollovers is the module. It holds no per-player state: the transaction comes
// from the caller on every path but one, the clock is injected (§3.1), and
// everything below is fixed before the server accepts a request.
type Rollovers struct {
	clk   simulation.Clock
	store *store.Store
	log   *slog.Logger

	accounts   Activity
	characters *character.Characters
	health     *health.Health
	routines   *routine.Routines
	employer   Employer
	world      World
	announce   Announce

	queue Queue
}

// New binds the rollover to the modules whose day it is running. The queue is
// not a parameter — its worker runs this, so one of the two has to exist first
// and it is tied on afterwards with [Rollovers.Through]. Until then a Rollovers
// simply schedules nothing.
func New(
	clk simulation.Clock,
	st *store.Store,
	log *slog.Logger,
	accounts Activity,
	characters *character.Characters,
	h *health.Health,
	routines *routine.Routines,
	employer Employer,
	world World,
	announce Announce,
) *Rollovers {
	return &Rollovers{
		clk: clk, store: st, log: log,
		accounts: accounts, characters: characters, health: h, routines: routines,
		employer: employer, world: world, announce: announce,
		queue: Nowhere{},
	}
}

// Through ties the rollover to the queue that carries it. Called once at boot,
// before anything can reach the server, so internal/api's "fixed at boot and
// read-only while the process serves" property stays true.
func (r *Rollovers) Through(q Queue) { r.queue = q }

// Wake catches a returning player up and re-arms their schedule (§3.3) — one
// act, not two. It runs inside the request's transaction, so a page that failed
// to load did not half-advance anybody.
func (r *Rollovers) Wake(ctx context.Context, tx pgx.Tx, accountID string) (Report, error) {
	if err := r.accounts.Seen(ctx, tx, accountID); err != nil {
		return Report{}, err
	}
	return r.Advance(ctx, tx, accountID)
}

// Waking is Wake in a transaction of its own, for a handler about to read the
// world. Run separately, a failed rollover is a player one day behind rather
// than a blank screen.
func (r *Rollovers) Waking(ctx context.Context, accountID string) (Report, error) {
	return r.own(ctx, func(tx pgx.Tx) (Report, error) { return r.Wake(ctx, tx, accountID) })
}

// Due runs the rollover for one player in its own transaction — the entry point
// the queue's worker calls. It reports only an error; a caller that wants the
// report calls Advance in its own transaction.
func (r *Rollovers) Due(ctx context.Context, accountID string) error {
	_, err := r.own(ctx, func(tx pgx.Tx) (Report, error) { return r.Advance(ctx, tx, accountID) })
	return err
}

// own runs one rollover in a transaction this package opened.
func (r *Rollovers) own(ctx context.Context, fn func(pgx.Tx) (Report, error)) (Report, error) {
	var out Report
	err := r.store.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = fn(tx)
		return err
	})
	if err != nil {
		return Report{}, err
	}
	return out, nil
}

/*
Advance is the rollover itself: every day this player is behind, in order, and
then the schedule for the next one.

Days are lived oldest first, because a body has inertia and applying Tuesday to a
body that has already had Wednesday is a different body. Ceilings are recomputed
once at the end, because a ceiling is a statement about now.

It is safe to run twice: the ledger row is taken under a lock and the walk starts
from the day after it, so a retried job, a second tab hydrating and the scheduled
rollover all reach the same state.
*/
func (r *Rollovers) Advance(ctx context.Context, tx pgx.Tx, accountID string) (Report, error) {
	// The day boundary is the player's own (§3.3), and the character is what
	// knows where they are.
	who, err := r.characters.Ensure(ctx, tx, accountID)
	if err != nil {
		return Report{}, err
	}
	now := r.clk.Now()
	today := simulation.On(now, who.Zone)

	// Through yesterday, never today: a day that is still happening cannot be
	// rolled up, and the player may still live it by hand.
	target := today.Plus(-1)

	last, err := r.ledger(ctx, tx, accountID, target)
	if err != nil {
		return Report{}, err
	}

	// Before the "is a day due" question: escalation is not a piece of the day
	// (§4), and gating it on a date boundary would leave a conflict that
	// happened this morning unmentioned until tomorrow's rollover. After the
	// ledger row is taken, so two tabs serialise here rather than racing.
	if err := r.world.Escalate(ctx, tx, accountID, target); err != nil {
		return Report{}, fmt.Errorf("rollover: escalate: %w", err)
	}

	out := Report{Through: last}
	behind := last.Until(target)
	if behind <= 0 {
		// Nothing is due, but the schedule is still re-armed: a player back
		// after an afternoon needs a chain as much as anybody.
		out.Next, err = r.schedule(ctx, tx, accountID, who.Zone, now)
		return out, err
	}

	from := last.Next()
	if behind > Window {
		out.Skipped = behind - Window
		from = target.Plus(-(Window - 1))
	}

	// The body keeps its own ledger and is allowed to be ahead of this one,
	// because a player may narrate a day by hand (§3.4). Starting the walk after
	// whichever is further along keeps a day from being offered to a body that
	// has already had it.
	body, err := r.health.Ensure(ctx, tx, accountID)
	if err != nil {
		return Report{}, err
	}
	if !body.LastDay.Before(from) {
		from = body.LastDay.Next()
	}
	out.From = from

	for on := from; !target.Before(on); on = on.Next() {
		fx, lived, err := r.day(ctx, tx, accountID, on)
		if err != nil {
			return Report{}, err
		}
		if lived {
			out.Days++
			out.Effects = fx
		}
		// The job's own day, after the body's, and asked for even on a day the
		// body has already had: a role that ran while the player narrated their
		// morning by hand still ran.
		if err := r.world.Delegated(ctx, tx, accountID, on); err != nil {
			return Report{}, fmt.Errorf("rollover: delegated %s: %w", on.At().Format(time.DateOnly), err)
		}
		// And the day's living among people (§3.15), after the job: what a role
		// produced is about the hours the body just spent, and this is about the
		// rest of them.
		if err := r.world.Among(ctx, tx, accountID, on); err != nil {
			return Report{}, fmt.Errorf("rollover: among %s: %w", on.At().Format(time.DateOnly), err)
		}
		if err := r.world.Decay(ctx, tx, accountID, on); err != nil {
			return Report{}, fmt.Errorf("rollover: decay %s: %w", on.At().Format(time.DateOnly), err)
		}
	}

	if err := r.world.Replenish(ctx, tx, accountID, target); err != nil {
		return Report{}, fmt.Errorf("rollover: replenish: %w", err)
	}
	if err := r.world.Recompute(ctx, tx, accountID, target); err != nil {
		return Report{}, fmt.Errorf("rollover: recompute: %w", err)
	}

	if err := r.record(ctx, tx, accountID, target, out); err != nil {
		return Report{}, err
	}
	out.Through = target

	if out.Next, err = r.schedule(ctx, tx, accountID, who.Zone, now); err != nil {
		return Report{}, err
	}

	if err := r.announce.Rolled(ctx, tx, accountID, out); err != nil {
		return Report{}, err
	}

	r.log.Debug("rolled over",
		"account", accountID, "through", target.At().Format(time.DateOnly),
		"days", out.Days, "skipped", out.Skipped)
	return out, nil
}

// day lives one day by the player's routines, reporting whether it counted. A
// day the body has already had is not an error.
func (r *Rollovers) day(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date) (simulation.Effects, bool, error) {
	day, err := r.routines.Day(ctx, tx, accountID, on)
	if err != nil {
		return nil, false, err
	}

	// The job, before the body (§3.14). What the routines describe is what the
	// player intended; what a day at work leaves of it is what happened, and the
	// body is only ever handed the second. It is also where the day's structural
	// income is paid, so it is asked even for a day the body has already had.
	day, err = r.employer.Shift(ctx, tx, accountID, on, day)
	if err != nil {
		return nil, false, fmt.Errorf("rollover: shift %s: %w", on.At().Format(time.DateOnly), err)
	}

	_, fx, err := r.health.Lived(ctx, tx, accountID, on, day)
	switch {
	case errors.Is(err, health.ErrAlreadyLived):
		return nil, false, nil
	case err != nil:
		return nil, false, err
	}
	return fx, true, nil
}

// schedule arms the next rollover if the player is still playing, and returns
// the instant scheduled or the zero time. A player unseen for [Dormant] holds no
// rows in the queue; the chain restarts from Wake when they come back.
func (r *Rollovers) schedule(ctx context.Context, tx pgx.Tx, accountID string, zone *time.Location, now time.Time) (time.Time, error) {
	active, err := r.accounts.Active(ctx, tx, accountID, Dormant)
	if err != nil {
		return time.Time{}, err
	}
	if !active {
		return time.Time{}, nil
	}

	at := Next(zone, now)
	// The day the job is for: the one that will have just ended when it runs.
	// In the args rather than derived at run time, so the queue's own uniqueness
	// enforces one rollover per player per day and a pending job says what it is
	// for.
	on := simulation.On(at, zone).Plus(-1)

	if err := r.queue.Schedule(ctx, tx, accountID, on, at); err != nil {
		return time.Time{}, fmt.Errorf("rollover: schedule: %w", err)
	}
	return at, nil
}

// Next is the instant a player's next rollover is due: the first midnight in
// their own timezone still ahead of now, plus [Settle]. A nil zone is UTC. The
// next boundary is a wall-clock midnight in that zone on the next calendar day.
func Next(zone *time.Location, now time.Time) time.Time {
	if zone == nil {
		zone = time.UTC
	}
	local := now.In(zone)
	y, m, d := local.Date()
	// Adding the day to the date rather than 24 hours to the instant is what
	// handles a zone whose day begins later than 00:00 on a transition.
	next := time.Date(y, m, d, 0, 0, 0, 0, zone).AddDate(0, 0, 1).Add(Settle)
	for !next.After(now) {
		// Unreachable for any real zone, and cheap insurance against one whose
		// transition swallows the midnight we just built.
		next = next.AddDate(0, 0, 1)
	}
	return next
}

/* ── the ledger ───────────────────────────────────────────────────────── */

// ledger reads the last day this player was advanced through, creating the row
// under a lock if there is none. A new row starts at start — the day before the
// player's current one — so a brand-new account has nothing to catch up on.
// The lock makes two rollovers arriving together correct: the second waits for
// the first to commit and reads what it wrote.
func (r *Rollovers) ledger(ctx context.Context, tx pgx.Tx, accountID string, start simulation.Date) (simulation.Date, error) {
	if _, err := tx.Exec(ctx, `
		INSERT INTO rollover (account_id, last_day)
		VALUES ($1, make_date($2, $3, $4))
		ON CONFLICT (account_id) DO NOTHING`,
		accountID, start.Year, int(start.Month), start.Day,
	); err != nil {
		return simulation.Date{}, fmt.Errorf("rollover: create ledger: %w", err)
	}

	var last time.Time
	if err := tx.QueryRow(ctx,
		`SELECT last_day FROM rollover WHERE account_id = $1 FOR UPDATE`, accountID,
	).Scan(&last); err != nil {
		return simulation.Date{}, fmt.Errorf("rollover: read ledger: %w", err)
	}
	// A DATE carries no timezone and needs none: it is already the player's own
	// calendar day, written by whoever last rolled them over.
	return simulation.On(last, time.UTC), nil
}

// record stamps the ledger forward.
func (r *Rollovers) record(ctx context.Context, tx pgx.Tx, accountID string, through simulation.Date, out Report) error {
	if _, err := tx.Exec(ctx, `
		UPDATE rollover
		   SET last_day = make_date($2, $3, $4),
		       ran_at   = now(),
		       days     = $5,
		       skipped  = $6,
		       runs     = runs + 1
		 WHERE account_id = $1`,
		accountID, through.Year, int(through.Month), through.Day, out.Days, out.Skipped,
	); err != nil {
		return fmt.Errorf("rollover: write ledger: %w", err)
	}
	return nil
}
