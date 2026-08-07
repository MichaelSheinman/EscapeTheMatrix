package rollover

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/account"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/character"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/clock"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/health"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/routine"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store/storetest"
)

/*
The rollover, against a real Postgres (§14), with a clock the test moves by hand
(§3.1).

Two things are worth proving here and the rest is bookkeeping. The first is that
the day boundary is the *player's* — technical.md §14 names it outright: "what
happens at the day boundary in Auckland" is a test that gets written repeatedly,
and Next below is the pure function that makes it writable at all. The second is
that a player who has been away is caught up a day at a time, in order, bounded,
and exactly once however many things ask for it.
*/

/* ── the boundary ─────────────────────────────────────────────────────── */

// The scheduling arithmetic, with no database anywhere near it.
//
// Every case is a real timezone with a real reason to be here: whole-hour
// offsets either side of UTC, the half-hour offset most of a billion people
// live on, the three-quarter-hour one nobody remembers exists, and both
// directions of a daylight-saving transition — the twenty-three-hour day and
// the twenty-five-hour one.
func TestTheNextRolloverIsTheNextLocalMidnight(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		zone string
		// now and want are wall-clock times in `zone`.
		now  string
		want string
	}{
		{
			name: "the small hours are still today",
			zone: "UTC",
			now:  "2026-08-06 00:30:00",
			want: "2026-08-07 00:05:00",
		},
		{
			name: "a minute to midnight is a rollover five minutes away",
			zone: "UTC",
			now:  "2026-08-06 23:59:00",
			want: "2026-08-07 00:05:00",
		},
		{
			// The reason the whole design is per-player. It is already
			// tomorrow in Auckland while Los Angeles is at lunch, and the two
			// players roll over twenty-one hours apart.
			name: "Auckland",
			zone: "Pacific/Auckland",
			now:  "2026-08-06 14:00:00",
			want: "2026-08-07 00:05:00",
		},
		{
			name: "Los Angeles",
			zone: "America/Los_Angeles",
			now:  "2026-08-06 14:00:00",
			want: "2026-08-07 00:05:00",
		},
		{
			name: "a half-hour offset",
			zone: "Asia/Kolkata",
			now:  "2026-08-06 22:10:00",
			want: "2026-08-07 00:05:00",
		},
		{
			name: "a three-quarter-hour offset",
			zone: "Pacific/Chatham",
			now:  "2026-08-06 09:00:00",
			want: "2026-08-07 00:05:00",
		},
		{
			// Spring forward: the day is twenty-three hours long. Adding a
			// calendar day to the date gets this right; adding 24h to the
			// instant would land an hour past midnight.
			name: "the day the clocks go forward",
			zone: "America/Los_Angeles",
			now:  "2026-03-07 20:00:00",
			want: "2026-03-08 00:05:00",
		},
		{
			// Fall back: twenty-five hours, and the same argument the other
			// way round.
			name: "the day the clocks go back",
			zone: "America/Los_Angeles",
			now:  "2026-11-01 20:00:00",
			want: "2026-11-02 00:05:00",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			zone := mustLoad(t, c.zone)
			now := mustParse(t, c.now, zone)

			got := Next(zone, now)
			if want := mustParse(t, c.want, zone); !got.Equal(want) {
				t.Errorf("Next(%s, %s) = %s, want %s", c.zone, c.now, got, want)
			}
			if !got.After(now) {
				t.Errorf("Next(%s, %s) = %s, which is not in the future", c.zone, c.now, got)
			}
			// Whatever the offset, the answer is Settle past a local midnight.
			if h, m, s := got.In(zone).Clock(); h != 0 || time.Duration(m)*time.Minute != Settle || s != 0 {
				t.Errorf("Next landed at %02d:%02d:%02d local, want midnight plus %s", h, m, s, Settle)
			}
		})
	}
}

// A player whose timezone the game does not know yet is on UTC — a real
// timezone, rather than whatever the process happens to be running in.
func TestAPlayerWithNoTimezoneRollsOverOnUTC(t *testing.T) {
	t.Parallel()
	now := mustParse(t, "2026-08-06 14:00:00", time.UTC)
	if got, want := Next(nil, now), Next(time.UTC, now); !got.Equal(want) {
		t.Errorf("Next(nil) = %s, want %s", got, want)
	}
}

/* ── the day itself ───────────────────────────────────────────────────── */

// A day still happening cannot be rolled up, and a player who has never been
// away owes nothing.
func TestTodayIsNotRolledUpWhileItIsStillHappening(t *testing.T) {
	t.Parallel()
	with(t, "UTC", func(w *world) {
		r := w.advance(t)
		if r.Days != 0 || r.Skipped != 0 {
			t.Errorf("a brand-new player was caught up on %d days (%d skipped)", r.Days, r.Skipped)
		}
		if w.lived(t) != 0 {
			t.Errorf("the body lived %d days it should not have", w.lived(t))
		}
	})
}

// The whole of §3.3's catch-up: every missed day, in order, one per calendar
// date, none of them today.
func TestAReturningPlayerIsCaughtUpADayAtATime(t *testing.T) {
	t.Parallel()
	with(t, "Pacific/Auckland", func(w *world) {
		w.behind(t, 5)

		r := w.advance(t)
		if r.Days != 5 {
			t.Fatalf("caught up %d days, want 5", r.Days)
		}
		if r.Skipped != 0 {
			t.Errorf("skipped %d days inside the window", r.Skipped)
		}

		yesterday := w.today(t).Plus(-1)
		if r.Through != yesterday {
			t.Errorf("rolled through %v, want %v", r.Through, yesterday)
		}
		if got := w.ledger(t); got != yesterday {
			t.Errorf("the ledger says %v, want %v", got, yesterday)
		}

		// One sample per day, dated across the days it describes rather than
		// arriving as five readings taken in the same second.
		days := w.samples(t)
		if len(days) != 5 {
			t.Fatalf("wrote %d samples, want 5", len(days))
		}
		for i, on := range days {
			if want := yesterday.Plus(-4 + i); on != want {
				t.Errorf("sample %d is for %v, want %v", i, on, want)
			}
		}
	})
}

// A long absence is caught up from a bounded window, not replayed in full.
func TestALongAbsenceIsCaughtUpFromTheWindowRatherThanReplayed(t *testing.T) {
	t.Parallel()
	with(t, "UTC", func(w *world) {
		w.behind(t, 200)

		r := w.advance(t)
		if r.Days != Window {
			t.Errorf("lived %d days, want the window's %d", r.Days, Window)
		}
		if want := 200 - Window; r.Skipped != want {
			t.Errorf("skipped %d days, want %d", r.Skipped, want)
		}
		// The ledger is still stamped all the way forward, so the player is
		// caught up rather than stuck replaying a year one night at a time.
		if got, want := w.ledger(t), w.today(t).Plus(-1); got != want {
			t.Errorf("the ledger says %v, want %v", got, want)
		}
		if got, want := len(w.samples(t)), Window; got != want {
			t.Errorf("wrote %d samples, want %d", got, want)
		}
	})
}

// Running it twice is running it once. This is what makes a retried job, a
// second tab hydrating, and a scheduled rollover landing on top of a catch-up
// all safe.
func TestARolloverIsSafeToRunTwice(t *testing.T) {
	t.Parallel()
	with(t, "America/Los_Angeles", func(w *world) {
		w.behind(t, 3)

		if got := w.advance(t).Days; got != 3 {
			t.Fatalf("first run caught up %d days, want 3", got)
		}
		again := w.advance(t)
		if again.Days != 0 || again.Skipped != 0 {
			t.Errorf("the second run applied %d days (%d skipped), want none", again.Days, again.Skipped)
		}
		if got := len(w.samples(t)); got != 3 {
			t.Errorf("there are %d samples after two runs, want 3", got)
		}
	})
}

/*
A day the player already lived by hand is never lived twice.

The two ledgers are allowed to disagree. The body's records days lived; the
rollover's records days the world was advanced through, and §3.4's interim
"narrate a day" command moves only the first. What must hold is that no day is
recorded twice and the rollover completes anyway — the walk starts after
whichever ledger is further along rather than offering the body days it has
already had.
*/
func TestADayAlreadyLivedByHandIsNotLivedAgain(t *testing.T) {
	t.Parallel()
	with(t, "UTC", func(w *world) {
		w.behind(t, 3)
		yesterday := w.today(t).Plus(-1)

		// Their own last night, narrated before the rollover reached it.
		if _, _, err := w.health.Lived(w.ctx, w.tx, w.account, yesterday,
			simulation.Day{Sleep: 8 * time.Hour}); err != nil {
			t.Fatalf("live a day by hand: %v", err)
		}

		r := w.advance(t)
		if r.Days != 0 {
			t.Errorf("lived %d days on top of a body already at yesterday", r.Days)
		}
		if got, want := w.ledger(t), yesterday; got != want {
			t.Errorf("the ledger says %v, want %v", got, want)
		}

		// One record for that day, not two.
		days := w.samples(t)
		if len(days) != 1 || days[0] != yesterday {
			t.Errorf("the body has samples for %v, want exactly [%v]", days, yesterday)
		}
	})
}

// The days are applied oldest first. A body has inertia, so Tuesday applied to
// a body that has already had Wednesday is a different body.
func TestTheDaysAreAppliedInOrder(t *testing.T) {
	t.Parallel()
	with(t, "UTC", func(w *world) {
		w.behind(t, 4)
		w.advance(t)

		days := w.samples(t)
		for i := 1; i < len(days); i++ {
			if !days[i-1].Before(days[i]) {
				t.Fatalf("sample %v came before %v", days[i-1], days[i])
			}
		}
	})
}

/* ── the rest of the rollover ─────────────────────────────────────────── */

// §3.3's four pieces of work, each asked for the right number of times: decay
// per day because it is a curve against elapsed days, ceilings and the backlog
// once because they are statements about now.
func TestEveryHalfOfTheRolloverIsRun(t *testing.T) {
	t.Parallel()
	with(t, "UTC", func(w *world) {
		w.behind(t, 4)
		w.advance(t)

		if got := len(w.rest.decayed); got != 4 {
			t.Errorf("decay ran %d times, want one per day (4)", got)
		}
		// And the delegated half of a job (§3.14), for the same reason: a role
		// that runs itself runs on every day the player was not there, and a
		// week away is a week of its work rather than one morning of it.
		// And §3.15's daily conflict, which is per day for the same reason: the
		// people you live among carry on living beside you while you are away.
		if got := len(w.rest.among); got != 4 {
			t.Errorf("living among people ran %d times over 4 days", got)
		}
		if got := len(w.rest.delegated); got != 4 {
			t.Errorf("the job ran itself %d times, want one per day (4)", got)
		}
		if w.rest.replenished != 1 {
			t.Errorf("the backlog was replenished %d times, want 1", w.rest.replenished)
		}
		if w.rest.recomputed != 1 {
			t.Errorf("ceilings were recomputed %d times, want 1", w.rest.recomputed)
		}
		if w.rest.escalated != 1 {
			t.Errorf("escalations were checked %d times, want 1", w.rest.escalated)
		}
	})
}

/*
The job half of a day (§3.14), asked for every day walked.

Two claims, and both of them are about the seam rather than about jobs. Every
day the rollover lives is offered to the employer *first*, so the body is handed
the day the shift left rather than the day the routines described — and the day
that comes back is the one that is used, which is what the shortened night here
proves.

What a real job does to a night is internal/simulation's test and
internal/career's. This package composes a day and owns none of its rules.
*/
func TestEveryDayGoesThroughTheJobBeforeItReachesTheBody(t *testing.T) {
	t.Parallel()
	with(t, "UTC", func(w *world) {
		w.job.nights = 2 * time.Hour
		w.behind(t, 3)
		w.advance(t)

		if got := len(w.job.worked); got != 3 {
			t.Fatalf("the job was asked about %d days, want one per day (3)", got)
		}
		for i := 1; i < len(w.job.worked); i++ {
			if !w.job.worked[i-1].Before(w.job.worked[i]) {
				t.Errorf("the days went to the job out of order: %v", w.job.worked)
			}
		}

		// And the shortened night is the night the body had. Two hours off
		// every night for three nights is a debt no routine on its own
		// produces, which is what makes this readable at all.
		body := w.panel(t)
		if body.LastSleep != 5.5-2 {
			t.Errorf("the body slept %.2f hours, want the night the shift left", body.LastSleep)
		}
	})
}

// Nothing due means nothing done. A player who was away for an afternoon does
// not get their backlog topped up four times a day.
//
// Escalation is the exception, and the exception is technical.md §4's: a moment
// the playbook could not handle is owed to the player at the next moment they
// are present, and a conflict that happened this morning waiting for tomorrow's
// date boundary would be the game being quiet about the one thing it must not
// be quiet about.
func TestNothingIsRunWhenNothingIsDueExceptEscalation(t *testing.T) {
	t.Parallel()
	with(t, "UTC", func(w *world) {
		w.advance(t)
		if len(w.rest.decayed) != 0 || w.rest.replenished != 0 || w.rest.recomputed != 0 {
			t.Error("the world was advanced for a player who owed no days")
		}
		if len(w.rest.among) != 0 {
			t.Errorf("a day among people ran with no day due")
		}
		if len(w.rest.delegated) != 0 {
			t.Error("a job produced work on a day that has not happened")
		}
		if w.rest.escalated != 1 {
			t.Errorf("escalations were checked %d times for a player who is here, want 1", w.rest.escalated)
		}
	})
}

/* ── the schedule ─────────────────────────────────────────────────────── */

// The chain: every rollover arms the next one, at that player's own midnight,
// for the day that will have just ended when it fires.
func TestARolloverArmsTheNextOne(t *testing.T) {
	t.Parallel()
	with(t, "Pacific/Auckland", func(w *world) {
		r := w.advance(t)

		if len(w.queue.jobs) != 1 {
			t.Fatalf("scheduled %d rollovers, want 1", len(w.queue.jobs))
		}
		job := w.queue.jobs[0]
		if job.account != w.account {
			t.Errorf("scheduled for %s, want %s", job.account, w.account)
		}
		if want := Next(w.zone, w.clk.Now()); !job.at.Equal(want) {
			t.Errorf("scheduled for %s, want %s", job.at, want)
		}
		if !r.Next.Equal(job.at) {
			t.Errorf("the report says %s, the queue says %s", r.Next, job.at)
		}
		// The job is labelled with the day it will roll up, which is the day
		// that is happening now.
		if want := w.today(t); job.on != want {
			t.Errorf("the job is for %v, want %v", job.on, want)
		}
	})
}

// §3.3's gate: a dormant account costs nothing, so nothing is scheduled for it.
func TestADormantPlayerIsNotScheduled(t *testing.T) {
	t.Parallel()
	with(t, "UTC", func(w *world) {
		w.unseenFor(t, Dormant+24*time.Hour)

		r := w.advance(t)
		if len(w.queue.jobs) != 0 {
			t.Errorf("scheduled %d rollovers for a dormant player, want none", len(w.queue.jobs))
		}
		if !r.Next.IsZero() {
			t.Errorf("the report claims a next rollover at %s", r.Next)
		}
	})
}

// ...and coming back is what puts them on the chain again, along with the days
// they missed. This is the whole of "their next login triggers a catch-up".
func TestComingBackCatchesThePlayerUpAndSchedulesThemAgain(t *testing.T) {
	t.Parallel()
	with(t, "UTC", func(w *world) {
		w.unseenFor(t, Dormant+24*time.Hour)
		w.behind(t, 6)

		r, err := w.rollovers.Wake(w.ctx, w.tx, w.account)
		if err != nil {
			t.Fatalf("wake: %v", err)
		}
		if r.Days != 6 {
			t.Errorf("caught up %d days, want 6", r.Days)
		}
		if len(w.queue.jobs) != 1 {
			t.Fatalf("scheduled %d rollovers, want 1", len(w.queue.jobs))
		}
		if r.Next.IsZero() {
			t.Error("the report claims no next rollover for a player who just arrived")
		}
	})
}

// A player who is up to date is still armed. Otherwise the chain ends the first
// day somebody logs in twice.
func TestAPlayerWithNothingDueIsStillScheduled(t *testing.T) {
	t.Parallel()
	with(t, "UTC", func(w *world) {
		w.advance(t)
		w.advance(t)
		if len(w.queue.jobs) != 2 {
			t.Errorf("scheduled %d rollovers over two runs, want 2", len(w.queue.jobs))
		}
	})
}

/* ── the harness ──────────────────────────────────────────────────────── */

// world is one player, on a real database, with a clock the test holds.
type world struct {
	ctx     context.Context
	tx      pgx.Tx
	account string
	zone    *time.Location
	clk     *clock.Stepping

	rollovers *Rollovers
	health    *health.Health
	queue     *recorder
	rest      *recording
	job       *shifts
}

// with runs f against a fresh database, inside one transaction, for a player in
// the named timezone.
//
// The transaction is rolled back by a cleanup rather than by a closure around f,
// because a t.Fatalf inside f ends the goroutine outright — a closure would
// never return and the connection would be held until the pool gave up.
func with(t *testing.T, zone string, f func(*world)) {
	t.Helper()
	st := storetest.New(t)
	ctx := t.Context()

	tx, err := st.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	loc := mustLoad(t, zone)
	// A fixed instant rather than the real one, so that every date in this file
	// is a date somebody chose. Mid-afternoon, so "today" is unambiguous in
	// every zone the tests use.
	clk := &clock.Stepping{At: mustParse(t, "2026-08-06 15:00:00", time.UTC)}

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	characters := character.New(clk, character.Quiet{})
	bodies := health.New(clk, characters)
	queue := &recorder{}
	rest := &recording{}
	job := &shifts{}

	w := &world{
		ctx: ctx, tx: tx, zone: loc, clk: clk,
		health: bodies, queue: queue, rest: rest, job: job,
		account: newAccount(t, ctx, tx),
	}
	w.rollovers = New(clk, st, quiet,
		account.New(st), characters, bodies, routine.New(routine.Open{}),
		job, rest, Silence{})
	w.rollovers.Through(queue)

	// The character has to exist before its timezone can be set, and Ensure is
	// what creates one.
	if _, err := characters.Ensure(ctx, tx, w.account); err != nil {
		t.Fatalf("create character: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE character SET timezone = $2 WHERE account_id = $1`, w.account, zone); err != nil {
		t.Fatalf("set timezone: %v", err)
	}

	f(w)
}

func (w *world) advance(t *testing.T) Report {
	t.Helper()
	r, err := w.rollovers.Advance(w.ctx, w.tx, w.account)
	if err != nil {
		t.Fatalf("advance: %v", err)
	}
	return r
}

// today is the player's own current day.
func (w *world) today(t *testing.T) simulation.Date {
	t.Helper()
	return simulation.Today(w.clk, w.zone)
}

// behind puts the player n days in arrears, by moving the ledger back.
//
// The ledger has to exist first, and creating it is the rollover's own job —
// so this runs one that has nothing to do and then rewrites what it wrote.
func (w *world) behind(t *testing.T, n int) {
	t.Helper()
	if _, err := w.rollovers.Advance(w.ctx, w.tx, w.account); err != nil {
		t.Fatalf("create ledger: %v", err)
	}
	w.queue.jobs = nil
	w.rest.reset()

	back := w.today(t).Plus(-1 - n)
	if _, err := w.tx.Exec(w.ctx,
		`UPDATE rollover SET last_day = make_date($2,$3,$4) WHERE account_id = $1`,
		w.account, back.Year, int(back.Month), back.Day); err != nil {
		t.Fatalf("move the ledger back: %v", err)
	}
}

// panel is the body as internal/health currently holds it.
func (w *world) panel(t *testing.T) simulation.Body {
	t.Helper()
	p, err := w.health.Of(w.ctx, w.tx, w.account)
	if err != nil {
		t.Fatalf("read the body: %v", err)
	}
	return p.Body
}

// unseenFor backdates the player's last sign of life.
func (w *world) unseenFor(t *testing.T, d time.Duration) {
	t.Helper()
	if _, err := w.tx.Exec(w.ctx,
		`UPDATE account SET last_seen_at = now() - make_interval(secs => $2) WHERE id = $1`,
		w.account, d.Seconds()); err != nil {
		t.Fatalf("backdate last seen: %v", err)
	}
}

// ledger is the last day the world was advanced through.
func (w *world) ledger(t *testing.T) simulation.Date {
	t.Helper()
	var on time.Time
	if err := w.tx.QueryRow(w.ctx,
		`SELECT last_day FROM rollover WHERE account_id = $1`, w.account).Scan(&on); err != nil {
		t.Fatalf("read the ledger: %v", err)
	}
	return simulation.On(on, time.UTC)
}

// samples is every day the body has a biomarker record for, oldest first.
func (w *world) samples(t *testing.T) []simulation.Date {
	t.Helper()
	rows, err := w.tx.Query(w.ctx,
		`SELECT on_day FROM health_sample WHERE account_id = $1 ORDER BY on_day`, w.account)
	if err != nil {
		t.Fatalf("read samples: %v", err)
	}
	defer rows.Close()

	var out []simulation.Date
	for rows.Next() {
		var on time.Time
		if err := rows.Scan(&on); err != nil {
			t.Fatalf("read samples: %v", err)
		}
		out = append(out, simulation.On(on, time.UTC))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read samples: %v", err)
	}
	return out
}

// lived is how many days the body has had.
func (w *world) lived(t *testing.T) int { t.Helper(); return len(w.samples(t)) }

// newAccount makes somebody for the rollover to belong to.
func newAccount(t *testing.T, ctx context.Context, tx pgx.Tx) string {
	t.Helper()
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO account (provider, subject, email, display_name)
		VALUES ('google', 'rollover-test', 'rollover@example.com', 'Rollover')
		RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("create account: %v", err)
	}
	return id
}

// recorder is a Queue that writes down what it was asked to schedule.
type recorder struct{ jobs []scheduled }

type scheduled struct {
	account string
	on      simulation.Date
	at      time.Time
}

func (r *recorder) Schedule(_ context.Context, _ pgx.Tx, accountID string, on simulation.Date, at time.Time) error {
	r.jobs = append(r.jobs, scheduled{account: accountID, on: on, at: at})
	return nil
}

// recording is a World that counts what it was asked for — the stand-in for the
// three modules #21, #15 and #29 will bring.
type recording struct {
	decayed     []simulation.Date
	delegated   []simulation.Date
	among       []simulation.Date
	replenished int
	recomputed  int
	escalated   int
}

func (w *recording) reset() { *w = recording{} }

/*
shifts is an Employer that records the days it was asked about and shortens the
night by an hour, so that a test can tell a day the job touched from one it did
not.

The point of it being here at all is the same as recording's: internal/rollover
composes a day and owns none of its rules, so what this package can assert about
§3.14 is that the seam is asked *for every day walked* and that what comes back
is what the body is handed. What a real job does to a night is internal/career's
test and internal/simulation's.
*/
type shifts struct {
	worked []simulation.Date
	nights time.Duration
}

func (s *shifts) Shift(
	_ context.Context, _ pgx.Tx, _ string, on simulation.Date, d simulation.Day,
) (simulation.Day, error) {
	s.worked = append(s.worked, on)
	d.Sleep -= s.nights
	return d, nil
}

func (w *recording) Replenish(context.Context, pgx.Tx, string, simulation.Date) error {
	w.replenished++
	return nil
}

func (w *recording) Decay(_ context.Context, _ pgx.Tx, _ string, on simulation.Date) error {
	w.decayed = append(w.decayed, on)
	return nil
}

func (w *recording) Recompute(context.Context, pgx.Tx, string, simulation.Date) error {
	w.recomputed++
	return nil
}

func (w *recording) Escalate(context.Context, pgx.Tx, string, simulation.Date) error {
	w.escalated++
	return nil
}

func (w *recording) Delegated(_ context.Context, _ pgx.Tx, _ string, on simulation.Date) error {
	w.delegated = append(w.delegated, on)
	return nil
}

func (w *recording) Among(_ context.Context, _ pgx.Tx, _ string, on simulation.Date) error {
	w.among = append(w.among, on)
	return nil
}

func mustLoad(t *testing.T, name string) *time.Location {
	t.Helper()
	zone, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return zone
}

func mustParse(t *testing.T, wall string, zone *time.Location) time.Time {
	t.Helper()
	at, err := time.ParseInLocation(time.DateTime, wall, zone)
	if err != nil {
		t.Fatalf("parse %q: %v", wall, err)
	}
	return at
}
