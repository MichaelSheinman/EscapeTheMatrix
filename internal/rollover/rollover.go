/*
Package rollover advances the world one day at a time, once per player, at that
player's own local midnight.

technical.md §3.3 is the whole specification and it is short: one game day is
one real calendar day, so the world advances on a **per-player scheduled
rollover at that player's local day boundary**, executed by the job queue. The
work is to replenish the event backlog (§4), apply Karma decay for the current
environment (§3.15), roll up health and biomarker progression (§3.16), and
recompute environment ceilings and factor caps. **Rejected: a continuous global
tick** — nothing in the design has sub-day granularity, so a loop that wakes for
everybody would be pure cost.

Two things follow from that, and they are the shape of this package.

# Nothing is scheduled for a player who is not playing

A dormant account must cost nothing. So the schedule is not a list the world
walks: it is a chain, one job at a time, and **each rollover schedules its own
successor only if the player has been seen recently.** A player who stops
playing stops being scheduled within a day, and their next sign-in is what puts
them back on the chain — which is also the catch-up, because Wake runs the
rollover before it re-arms the schedule.

# The rollover is a composition, not a mechanic

Everything here is somebody else's rule, run in the right order on the right
date. The body's rules are internal/simulation's and are applied through
internal/health; the day being lived comes from internal/routine; the timezone
that decides which day it is comes from internal/character. This package owns
one table — `rollover`, the ledger of which day each player has been advanced
through — and reads no other module's (§3.1).

The three halves of §3.3's work that have no module yet are named as interfaces
below with a stub that does nothing, in the same way internal/routine names its
Gate. That is deliberate: the ticket that builds each one replaces a binding
rather than editing this file, and until then the thing that is missing is
visible in the code rather than only in a backlog.
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

/*
Dormant is how long a player may go unplayed before the world stops scheduling
work for them.

§3.3 asks only that dormant accounts cost nothing; the number is a tuning
decision. A fortnight is long enough that an ordinary holiday does not drop
somebody off the chain, and short enough that a year-old account is not still
being woken every night for a body nobody is going to look at.

Nothing is lost by falling off it. The days keep counting on the calendar and
the next sign-in catches them up, because the catch-up reads a date rather than
a queue.
*/
const Dormant = 14 * 24 * time.Hour

/*
Window is the furthest back a returning player is caught up from.

A player who has been away for a season is not owed a season of simulated
mornings — those days happened to nobody, and writing them would be inventing
evidence for a diagnosis (§3.6) to later argue from. Thirty days is what the
walk will do; anything older is counted as skipped and the ledger is stamped
forward, so a returning player is caught up rather than stuck replaying a year
one night at a time.

It is also the bound that keeps a catch-up a request rather than a batch job:
thirty days is thirty iterations of pure arithmetic and thirty small inserts,
inside one transaction, which is a page load.
*/
const Window = 30

/*
Settle is how long after local midnight a rollover is scheduled for.

Not zero, and not for tidiness. The worker reads the player's day off the clock
when it runs, and a job scheduled for exactly midnight that is picked up a
moment early would compute yesterday's date and do nothing — leaving the player
a day behind until the next one. A few minutes is far more than any plausible
skew between this process, the database, and the queue's poll interval.
*/
const Settle = 5 * time.Minute

/*
Queue is where the next rollover is put.

An interface rather than the queue itself so that this package does not import
riverqueue/river (§3.4), and so that a test can assert what *would* have been
scheduled without running a worker. internal/jobs is the one implementation
that survives into production.

Schedule takes the caller's transaction, which is the property the whole design
leans on: a job enqueued by a rollover that then failed to commit never existed,
and a ledger that moved forward without arming its successor is unreachable
rather than a state to recover from.
*/
type Queue interface {
	Schedule(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date, at time.Time) error
}

// Nowhere is a queue that drops what it is given.
//
// The binding a Rollovers is constructed with, so that a server which has not
// started its workers yet — or a test which has none — still rolls players over
// correctly on the paths a request drives. It changes when the next rollover
// happens, never whether one is correct.
type Nowhere struct{}

func (Nowhere) Schedule(context.Context, pgx.Tx, string, simulation.Date, time.Time) error {
	return nil
}

/*
Activity is what this package is allowed to know about how recently an account
was played.

§3.1's rule about cross-module access, kept narrow: the gate needs one timestamp
and the ability to touch it, and gets no way to read an email address, a session
or anything else internal/account owns.
*/
type Activity interface {
	Seen(ctx context.Context, tx pgx.Tx, accountID string) error
	Active(ctx context.Context, tx pgx.Tx, accountID string, within time.Duration) (bool, error)
}

/*
World is the rest of §3.3's rollover: the three pieces of work whose modules do
not exist yet.

Named here, with a stub that does nothing, rather than left out — because a
rollover that silently does three quarters of its job is indistinguishable from
one that does all of it, and the difference is a ticket. #21 brought the event
backlog, #15 Karma and the hidden factors, #29 the populations, their ceilings
and their decay. Each of them replaced this binding in internal/api; none of
them edited this package, which is the whole argument for naming the seam.
*/
type World interface {
	// Replenish tops the event backlog up for the day now beginning (§4).
	Replenish(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date) error
	// Decay applies Karma decay for the environment the player is in, for one
	// day (§3.15). Per day rather than per run, because decay is a curve
	// against elapsed days and a player caught up from a week away has to lose
	// a week of it rather than a night.
	Decay(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date) error
	// Recompute recomputes environment ceilings and factor caps (§3.5, §3.15).
	// Once per run: a ceiling is a statement about now, and computing it for
	// each of thirty caught-up days would be thirty answers thrown away.
	//
	// #29 landed and the honest implementation of this is **nothing**, which is
	// worth reading rather than deleting. A ceiling turned out to be a pure
	// function of authored content and one tenure row, so it is composed on
	// read like everything else derived (§3.3) and there is no stored copy for
	// a rollover to bring up to date. The seam stays because the day a ceiling
	// does need materialising, this is where it goes — and because a method
	// that deliberately does nothing is a decision, while a missing one is an
	// omission.
	Recompute(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date) error
	// Escalate puts what the playbook could not handle in front of the player
	// (§4's escalation rule).
	//
	// Once per run rather than per day, and capped by whoever implements it,
	// for the same reason the event backlog is: a player who was away a
	// fortnight while their rules went wrong every morning is owed a
	// conversation, not fourteen of them at once.
	//
	// It is the one piece of the day that also runs when no day is due — see
	// [Rollovers.Advance] for why.
	Escalate(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date) error
	// Delegated runs a day of a job the player does not sit inside (§3.14).
	//
	// Per day, and for the same reason Decay is: an active role generates
	// situations while the player is elsewhere, and being elsewhere for a week
	// is a week of them rather than one. What comes back from a day nobody was
	// there for is exactly what makes the playbook worth composing.
	//
	// It runs after the day has been lived rather than before, because a
	// situation is something the *job* produced and the job is the thing the
	// body has just spent ten hours on.
	Delegated(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date) error
	// Among runs a day of living beside the people the player lives beside
	// (§3.15's daily conflict).
	//
	// Per day, and for the same reason Decay and Delegated are: a population
	// produces friction whether or not anybody is looking at it, and being away
	// for a week is a week of it. §3.15 makes this the engine of the whole loop
	// — "living among a population means resolving conflicts with them every
	// day" — so it is a piece of the day rather than something a screen causes.
	//
	// Last of the day's work, after the job. What a role produced is about the
	// hours the body has just spent; what the people around you produced is
	// about the rest of them.
	Among(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date) error
}

/*
Employer is the job half of a day (§3.14).

Two things happen to a day because a player holds a job, and they are one call
because they are one fact about that day: the shift takes hours out of it, and
the tier pays for it. §3.4 makes the second structural — income comes from job
tier rather than from repeating actions — which is precisely why it belongs
here, in the thing that happens once per day with nobody asking, rather than on
a command a player could press.

An interface for the same reason [World] is: this package composes a day and
owns none of its rules. internal/career implements it, and what comes back is
the day as it was actually lived — the same [simulation.Day] the routines
produced, with the shift already taken out of it, so internal/health is handed
one day rather than a day and a correction.

It is asked for every day the rollover walks, including days the body has
already had. Paying is idempotent in the date on the far side of this
interface, which is where the ledger for it lives.
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

/*
Announce is how a rollover tells a player's open tabs that their world moved.

The rollover is the one thing in the game that happens with nobody asking, which
makes it exactly what §8's push half exists for — and exactly why this is an
interface. Message shapes live in internal/api next to the views they carry, so
this package says *that* something happened and never what it looks like.
*/
type Announce interface {
	Rolled(ctx context.Context, tx pgx.Tx, accountID string, r Report) error
}

// Silence is an Announce with nobody listening.
type Silence struct{}

func (Silence) Rolled(context.Context, pgx.Tx, string, Report) error { return nil }

/*
Report is what one rollover did.

Days and Skipped are the honest pair: a player away for a season has a large
Skipped and a Days of Window, and reporting only the first would make a catch-up
look like a full replay.
*/
type Report struct {
	// Through is the local day the player is now advanced through. Zero when
	// nothing was due.
	Through simulation.Date
	// From is the first day actually lived, and Days how many were.
	From simulation.Date
	Days int
	// Skipped is how many days fell outside the catch-up window.
	Skipped int
	// Effects are the pure core's, from the last day applied. The last one
	// rather than all of them because a threshold crossed three weeks ago is
	// not news, and a client told about thirty days of crossings at once would
	// have to decide which to believe.
	Effects simulation.Effects
	// Next is when the following rollover is due, and zero when none was
	// scheduled because the player has gone dormant.
	Next time.Time
}

// Ran reports whether this rollover advanced the world at all.
func (r Report) Ran() bool { return r.Days > 0 || r.Skipped > 0 }

/*
Rollovers is the module.

It holds no per-player state. The transaction comes from the caller on every
path but one, the clock is injected (§3.1), and everything below is fixed before
the server accepts a request.
*/
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

// New binds the rollover to the modules whose day it is running.
//
// The queue is not a parameter. It cannot be: the queue's worker runs *this*,
// so one of the two has to exist first, and it is tied on afterwards with
// Through. Until then a Rollovers is complete and correct — it simply schedules
// nothing, which is the right behaviour for a process with no workers in it.
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

// Through ties the rollover to the queue that carries it.
//
// Called once, at boot, before anything can reach the server — so the "fixed at
// boot and read-only while the process serves" property internal/api depends on
// stays true even though this one field is written after construction.
func (r *Rollovers) Through(q Queue) { r.queue = q }

/*
Wake catches a returning player up and re-arms their schedule.

This is §3.3's "their next login triggers a catch-up rollover", and it is one
function rather than two because the two are the same act: a player who is back
is a player whose missed days are owed and whose chain has to start again. It
runs inside the request's transaction, so a page that failed to load did not
half-advance anybody.
*/
func (r *Rollovers) Wake(ctx context.Context, tx pgx.Tx, accountID string) (Report, error) {
	if err := r.accounts.Seen(ctx, tx, accountID); err != nil {
		return Report{}, err
	}
	return r.Advance(ctx, tx, accountID)
}

/*
Waking is Wake in a transaction of its own.

What a handler that is about to read the world calls, and it is separate rather
than convenient: a failed statement poisons the transaction it happened in, so a
catch-up that went wrong inside the snapshot's transaction would take the
snapshot down with it. Run first and on its own, a rollover that failed is a
player who is a day behind and will be caught up by the next one, rather than a
blank screen.
*/
func (r *Rollovers) Waking(ctx context.Context, accountID string) (Report, error) {
	return r.own(ctx, func(tx pgx.Tx) (Report, error) { return r.Wake(ctx, tx, accountID) })
}

/*
Due runs the rollover for one player, in its own transaction.

The entry point the queue's worker calls. A job is not a request and has no
caller to borrow a transaction from, so this is the one place in the package
that opens one — and it is still exactly one, which is what makes a job that
crashed halfway leave nothing behind and simply be retried.
*/
// It reports only an error because that is all a queue can do with an answer.
// A caller that wants to know what the rollover did calls Advance inside its
// own transaction, which is what every test here does.
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

The order matters and it is §3.3's. Days are lived oldest first, because a body
has inertia and applying Tuesday to a body that has already had Wednesday is a
different body. Ceilings are recomputed once, at the end, because a ceiling is a
statement about now.

It is safe to run twice. The ledger row is taken under a lock and the walk
starts from the day after it, so a retried job, a second tab hydrating, and the
scheduled rollover arriving while a catch-up is still running all reach the same
state — the second one finds nothing to do.
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

	// Through yesterday, never today. A day that is still happening cannot be
	// rolled up, and leaving today alone is what keeps the player's own day
	// theirs — they may still live it by hand, and if they do not, tomorrow's
	// rollover takes it.
	target := today.Plus(-1)

	last, err := r.ledger(ctx, tx, accountID, target)
	if err != nil {
		return Report{}, err
	}

	// Before the "is a day due" question, and deliberately. Escalation is not a
	// piece of the day (§4): a moment the playbook could not handle is owed to
	// the player at the next moment they are actually present, and gating it on
	// a date boundary would mean a conflict that happened this morning waiting
	// until tomorrow's rollover to be mentioned.
	//
	// After the ledger row is taken, though, so two tabs arriving together
	// serialise here rather than racing to raise the same thing.
	if err := r.world.Escalate(ctx, tx, accountID, target); err != nil {
		return Report{}, fmt.Errorf("rollover: escalate: %w", err)
	}

	out := Report{Through: last}
	behind := last.Until(target)
	if behind <= 0 {
		// Nothing is due. The schedule is still re-armed: this is the path a
		// returning player takes when they were only away for an afternoon, and
		// they need a chain as much as anybody.
		out.Next, err = r.schedule(ctx, tx, accountID, who.Zone, now)
		return out, err
	}

	from := last.Next()
	if behind > Window {
		out.Skipped = behind - Window
		from = target.Plus(-(Window - 1))
	}

	// The body keeps its own ledger, and it is allowed to be ahead of this one:
	// a player may narrate a day by hand (§3.4's interim command), and the two
	// records then disagree by a day. Starting the walk after whichever is
	// further along is what keeps a day from being offered to a body that has
	// already had it — the body would refuse it anyway, and refusing thirty of
	// them in a row is thirty statements to reach the same place.
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
		// The job's own day, after the body's. It is asked for even on a day the
		// body turns out to have had already — the two ledgers are allowed to
		// disagree by a day (see the comment in Advance) and a role that ran
		// while the player narrated their morning by hand still ran.
		if err := r.world.Delegated(ctx, tx, accountID, on); err != nil {
			return Report{}, fmt.Errorf("rollover: delegated %s: %w", on.At().Format(time.DateOnly), err)
		}
		// And the day's living among people (§3.15). After the job for the same
		// reason the job is after the body: what a role produced is about the
		// hours the body has just spent, and what the people around you
		// produced is about the rest of them.
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

// day lives one day by the player's routines, reporting whether it counted.
//
// A day the body has already had is not an error. The player may have lived it
// by hand before the rollover reached it, and the two ledgers are allowed to
// disagree by a day for exactly that reason — the body refuses the duplicate,
// the world moves on, and the arithmetic is the same either way because the
// pure core is deterministic in the date (§3.1).
func (r *Rollovers) day(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date) (simulation.Effects, bool, error) {
	day, err := r.routines.Day(ctx, tx, accountID, on)
	if err != nil {
		return nil, false, err
	}

	// The job, before the body (§3.14). What the routines describe is what the
	// player intended; what a day at work leaves of it is what actually
	// happened, and the body is only ever handed the second one.
	//
	// It is also where the day's structural income is paid, which is why this
	// is asked even for a day the body turns out to have already had: the two
	// ledgers are allowed to disagree by a day (see the comment in Advance),
	// and a wage that went unpaid because the body was ahead would be a day's
	// work done for nothing.
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

/*
schedule arms the next rollover, if the player is still playing.

This is §3.3's activity gate and the whole of it: a player who has not been seen
inside Dormant is not scheduled, so a dormant account holds no rows in the queue
and costs nothing. The chain restarts from Wake the moment they come back.

It returns the instant scheduled, and the zero time when nothing was.
*/
func (r *Rollovers) schedule(ctx context.Context, tx pgx.Tx, accountID string, zone *time.Location, now time.Time) (time.Time, error) {
	active, err := r.accounts.Active(ctx, tx, accountID, Dormant)
	if err != nil {
		return time.Time{}, err
	}
	if !active {
		return time.Time{}, nil
	}

	at := Next(zone, now)
	// The day the job is *for*: the one that will have just ended when it runs.
	// It is in the args rather than derived at run time so that the queue's own
	// uniqueness can do the work of "one rollover per player per day", and so
	// that a pending job says what it is for when somebody looks at the table.
	on := simulation.On(at, zone).Plus(-1)

	if err := r.queue.Schedule(ctx, tx, accountID, on, at); err != nil {
		return time.Time{}, fmt.Errorf("rollover: schedule: %w", err)
	}
	return at, nil
}

/*
Next is the instant a player's next rollover is due: the first midnight in their
own timezone that is still ahead of now, plus Settle.

Pure, exported, and the reason "what happens at the day boundary in Auckland" is
a unit test rather than a playthrough (§3.1). A nil zone is UTC, which is the
sane reading of a player the game does not know the location of yet — a real
timezone rather than whatever the process happens to be running in.

Daylight saving is handled by construction rather than by arithmetic: the next
boundary is built as a wall-clock midnight *in that zone* on the next calendar
day, so the twenty-three-hour day and the twenty-five-hour one both land on
their own midnight instead of on the same number of hours later.
*/
func Next(zone *time.Location, now time.Time) time.Time {
	if zone == nil {
		zone = time.UTC
	}
	local := now.In(zone)
	y, m, d := local.Date()
	// A day that begins later than 00:00 — Lord Howe once did, and some zones
	// still do on a transition — is what time.Date normalises for. Adding the
	// day to the *date* rather than adding 24 hours to the instant is what
	// makes that true.
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
// under a lock if there is none.
//
// A new row starts at `start` — the day before the player's current one — so a
// brand-new account has nothing to catch up on. Starting it at zero would hand
// every first sign-in a backfill of days that happened to nobody.
//
// The lock is what makes two rollovers arriving together correct rather than
// lucky: the second waits for the first to commit and then reads what it wrote,
// so a scheduled job and a catch-up cannot both walk the same days.
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
