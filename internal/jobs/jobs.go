/*
Package jobs is the durable background queue, and the workers that drain it.

technical.md §3.4: **riverqueue/river — a durable Postgres-backed queue, workers
running in the same binary.** Durable across restarts, retried automatically,
and inspectable with a `SELECT` rather than a dashboard. Zero added
infrastructure, which is what keeps the one-binary, one-datastore property (§1).
A `time.Ticker` was rejected for the reasons this package exists: it is lost on
restart, it never retries, and it is invisible — and the daily rollover matters.

# What is in here and what is not

Only the plumbing. The queue holds no rules: a rollover job is an account id, a
date, and a call into internal/rollover, which is where the day actually
happens. That split is why this package can be swapped for another queue without
a game change, and why the rollover is testable without a worker anywhere near
it.

# riverqueue/river's schema

riverqueue/river owns its own tables and its own migration ledger, and it moves
on its own release schedule rather than the game's. Migrate below runs its
migrator, next to — not inside — the goose run that owns `migrations/`. Copying
riverqueue/river's SQL into that directory would pin the game's migration
history to a library version and break the first time the two disagreed about
what version the database is at.
*/
package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertype"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
)

/*
Runner is what a rollover job runs.

An interface, and a one-method one, so that this package imports no game rules
at all — internal/rollover implements it, and the arrow points that way and not
back. The practical payoff is that `internal/store/storetest` can migrate the
queue's schema alongside the game's without dragging the whole simulation into
every module's test binary, which is otherwise an import cycle.
*/
type Runner interface {
	Due(ctx context.Context, accountID string) error
}

// name is the queue rollovers run on.
//
// Its own rather than the default, so that the moderation actions and
// leaderboard refresh §3.4 also names cannot starve it — and so that "how many
// rollovers are in flight" is one WHERE clause.
const name = "rollover"

// boards is the queue the leaderboard rebuild runs on.
//
// Separate from the rollovers for the reason above, and it is the case that
// argues for it: one rebuild is a sort of the whole population and holds a lock
// while it runs, so a slow one must not be able to sit in front of somebody's
// midnight.
const boards = "boards"

// workers is how many rollovers this process runs at once.
//
// Each one is a handful of small statements in a single transaction, so the
// limit is the database's connection pool rather than CPU. Deliberately modest:
// the load is one job per player per day and it arrives spread across every
// timezone on earth, which is the least bursty arrival pattern a queue can have.
const workers = 10

/*
attempts is how many times a failed rollover is retried before it is discarded.

riverqueue/river's default is twenty-five, which with its backoff spans days —
far too long for work that is only meaningful on the day it is for. Eight
attempts is a few hours, which covers a database restart or a deploy and gives
up before it starts re-living a day nobody is waiting for. The next day's
rollover catches the player up anyway, because catch-up reads a date rather
than a queue.
*/
const attempts = 8

/*
Rollover is the job: which player, and the local day it is for.

The date is carried rather than derived so that the queue's own uniqueness does
the work of "one rollover per player per day" — two inserts for the same account
and day collapse into one, whoever made them — and so that a pending row says
what it is for when somebody looks at the table. The worker still reads the day
off the clock when it runs; this is a label and a key, never an instruction.
*/
type Rollover struct {
	Account string `json:"account"`
	On      string `json:"on"`
}

func (Rollover) Kind() string { return "rollover" }

// InsertOpts is inherited by every rollover inserted, so the uniqueness rule
// cannot be forgotten at a call site.
func (Rollover) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       name,
		MaxAttempts: attempts,
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
	}
}

// rolloverWorker runs one. It holds the queue rather than the rollover module
// so that the module can reach back through it to schedule its successor.
type rolloverWorker struct {
	river.WorkerDefaults[Rollover]
	q *Queue
}

func (w *rolloverWorker) Work(ctx context.Context, job *river.Job[Rollover]) error {
	// Everything about the day — which days are owed, whether the player has
	// gone dormant, when the next one is due — is decided in there. This
	// function is the part that had to be woken up.
	if err := w.q.rollovers.Due(ctx, job.Args.Account); err != nil {
		return fmt.Errorf("jobs: rollover for %s: %w", job.Args.Account, err)
	}
	return nil
}

/*
Boards is what the leaderboard refresh runs.

The second one-method interface in this file, for the same reason as [Runner]:
the queue holds no rules. Rebuilding a board is a global sort over every player
and it lives in the module that owns progression; this package knows only that
something has to be woken up on a schedule.
*/
type Boards interface {
	RefreshBoards(ctx context.Context) error
}

/*
Lapsing hands a walked-away-from conversation back to the player's own rules.

The third one-method interface here, and the same rule applies: the queue holds
no rules. Who is present, what their playbook would have said, and whether a
conflict may be settled on their behalf are all internal/conflict's, and this
package knows only that something has to be woken up on a schedule.
*/
type Lapsing interface {
	LapseDeferred(ctx context.Context) error
}

/*
Cadence is how often the leaderboards are rebuilt.

§3.5's boards are a snapshot by design (§3.4 lists "leaderboard refresh" as a job
type), so the only question is how stale a rank may be. Five minutes is short
enough that a player who has just done something sees it move while they are
still playing, and long enough that a population of any size is sorted a handful
of times an hour rather than continuously.

Nothing depends on the interval being exact. A rebuild replaces the whole table
from what is true at the moment it runs, so a missed one costs a few minutes of
freshness and nothing else — which is why this is a plain interval rather than a
cron expression pinned to a wall clock.
*/
const Cadence = 5 * time.Minute

/*
Leaving is how often the game checks whether anybody has walked away from a
conversation that was theirs to answer (§3.15).

A conflict goes to the phone when the player is at the keyboard and to their
rules when they are not, and "did not answer" is not a third outcome — it is
being away, arrived at slowly. Something has to notice the arriving. A day's
rollover cannot: it runs once per day, so a message handed over at nine would sit
unanswered until midnight rather than being taken by the rules at five past.

One minute, against `account.Present`'s two: a player who shuts the laptop has
their rules take over inside about three minutes. Short enough that coming back
to find your character dealt with it reads as "I was out", long enough that it is
sixty cheap queries an hour over a table with a partial index on exactly this.
*/
const Leaving = time.Minute

/*
Refresh is the leaderboard rebuild job (§3.4).

It carries nothing. A rollover is *for* a player and a day, and says so in its
args; a rebuild is for everybody and for now, so an empty struct is the honest
shape — and the uniqueness rule below then means two of them can never queue up
behind each other.
*/
type Refresh struct{}

func (Refresh) Kind() string { return "board.refresh" }

/*
InsertOpts keeps at most one rebuild in flight at a time.

By state rather than by args: there are no args, and what must not happen is a
backlog of rebuilds accumulating behind one slow run — each of which would throw
away the work of the one before it. `running` is in the list deliberately, so a
rebuild that is still going when the next interval elapses is not joined by a
second one waiting to redo it the moment it finishes.

riverqueue/river requires available, pending, running and scheduled to be
present whenever custom states are given; retryable is the one this adds, so
that a rebuild backing off after a failure still counts as in flight.
*/
func (Refresh) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       boards,
		MaxAttempts: attempts,
		UniqueOpts: river.UniqueOpts{
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable,
				rivertype.JobStatePending,
				rivertype.JobStateRetryable,
				rivertype.JobStateRunning,
				rivertype.JobStateScheduled,
			},
		},
	}
}

/*
Lapse is the sweep that hands deferred conflicts back to the playbook (§3.15).

It carries nothing, for the same reason [Refresh] does not: it is for everybody
and for now. The uniqueness rule below then means two of them can never queue up
behind each other, which matters more here than for a rebuild — two sweeps
racing over the same waiting conflict would both read it as lapsable.
*/
type Lapse struct{}

func (Lapse) Kind() string { return "conflict.lapse" }

// InsertOpts keeps at most one sweep in flight, by state. The same shape
// [Refresh] uses and for the same reason: a backlog of sweeps behind one slow
// run is a backlog of work already done.
func (Lapse) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       boards,
		MaxAttempts: attempts,
		UniqueOpts: river.UniqueOpts{
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable,
				rivertype.JobStatePending,
				rivertype.JobStateRetryable,
				rivertype.JobStateRunning,
				rivertype.JobStateScheduled,
			},
		},
	}
}

// lapseWorker runs one sweep.
type lapseWorker struct {
	river.WorkerDefaults[Lapse]
	q *Queue
}

func (w *lapseWorker) Work(ctx context.Context, _ *river.Job[Lapse]) error {
	if err := w.q.leaving.LapseDeferred(ctx); err != nil {
		return fmt.Errorf("jobs: lapse deferred conflicts: %w", err)
	}
	return nil
}

// refreshWorker runs one rebuild.
type refreshWorker struct {
	river.WorkerDefaults[Refresh]
	q *Queue
}

func (w *refreshWorker) Work(ctx context.Context, _ *river.Job[Refresh]) error {
	if err := w.q.boards.RefreshBoards(ctx); err != nil {
		return fmt.Errorf("jobs: rebuild boards: %w", err)
	}
	return nil
}

/*
Queue is the running queue: a riverqueue/river client, its workers, and the
game modules they call into.

One per process. It is created stopped, so a binary can be built and its routes
wired before anything starts draining — which is what lets internal/api hold a
Queue that is not running yet in a test.
*/
type Queue struct {
	client    *river.Client[pgx.Tx]
	rollovers Runner
	boards    Boards
	leaving   Lapsing
	log       *slog.Logger
}

/*
New builds the queue over the store's pool and registers every worker.

The knot is worth naming: the worker has to reach the rollover module, and the
rollover module has to reach the queue to schedule its successor. Something has
to be second, and it is the queue — New takes the rollover, and the caller ties
the queue back on with rollover.Through. Both happen at boot, before the process
serves anything.
*/
func New(st *store.Store, log *slog.Logger, rollovers Runner, board Boards, leaving Lapsing) (*Queue, error) {
	q := &Queue{rollovers: rollovers, boards: board, leaving: leaving, log: log}

	w := river.NewWorkers()
	river.AddWorker(w, &rolloverWorker{q: q})
	river.AddWorker(w, &refreshWorker{q: q})
	river.AddWorker(w, &lapseWorker{q: q})

	client, err := river.NewClient(riverpgxv5.New(st.Pool()), &river.Config{
		Logger:  quiet(log, slog.LevelInfo),
		Workers: w,
		Queues: map[string]river.QueueConfig{
			name: {MaxWorkers: workers},
			// One at a time, and one is all there can usefully be: a rebuild
			// takes an advisory lock over the whole board, so a second worker
			// would only ever wait for the first to finish and then redo it.
			boards: {MaxWorkers: 1},
		},
		// §3.5's boards, rebuilt on a schedule rather than per request.
		// riverqueue/river runs periodic jobs on the elected leader only, so a
		// fleet of any size still produces one rebuild per interval — and
		// RunOnStart means a freshly migrated database has boards before
		// anybody can look at them rather than after the first interval
		// elapses.
		PeriodicJobs: []*river.PeriodicJob{
			river.NewPeriodicJob(
				river.PeriodicInterval(Cadence),
				func() (river.JobArgs, *river.InsertOpts) { return Refresh{}, nil },
				&river.PeriodicJobOpts{ID: "board-refresh", RunOnStart: true},
			),
			// §3.15's presence rule, other half: noticing that somebody left.
			// RunOnStart because a process that has just come up may be coming
			// up after one that went down holding somebody's conversation, and
			// the player has been away for the whole of the gap.
			river.NewPeriodicJob(
				river.PeriodicInterval(Leaving),
				func() (river.JobArgs, *river.InsertOpts) { return Lapse{}, nil },
				&river.PeriodicJobOpts{ID: "conflict-lapse", RunOnStart: true},
			),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("jobs: build queue: %w", err)
	}
	q.client = client
	return q, nil
}

// Start begins draining. Idempotent, like riverqueue/river's own.
func (q *Queue) Start(ctx context.Context) error {
	if err := q.client.Start(ctx); err != nil {
		return fmt.Errorf("jobs: start queue: %w", err)
	}
	q.log.Info("job queue running", "queue", name, "workers", workers,
		"boards", boards, "every", Cadence)
	return nil
}

// Stop finishes the jobs in flight and stops fetching.
//
// A job that does not finish inside the caller's deadline is not lost: it is
// still a row, and the next process to start picks it up. That is the whole
// argument for a durable queue over a goroutine (§3.4).
func (q *Queue) Stop(ctx context.Context) error {
	if err := q.client.Stop(ctx); err != nil {
		return fmt.Errorf("jobs: stop queue: %w", err)
	}
	return nil
}

/*
Schedule answers rollover.Queue: put this player's next rollover on the queue,
in the caller's transaction.

Transactional insert is the point. riverqueue/river will not see the row until
the transaction commits, so a rollover that failed halfway leaves no job
behind, and a ledger that moved forward without arming its successor is
unreachable rather than a state to recover from.

A duplicate is not an error. Two inserts for the same account and day — a
scheduled rollover and a catch-up racing each other — collapse into one by the
uniqueness rule on the args, which is exactly the outcome wanted and is reported
rather than raised.
*/
func (q *Queue) Schedule(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date, at time.Time) error {
	res, err := q.client.InsertTx(ctx, tx, Rollover{
		Account: accountID,
		On:      on.At().Format(time.DateOnly),
	}, &river.InsertOpts{ScheduledAt: at})
	if err != nil {
		return fmt.Errorf("jobs: schedule rollover: %w", err)
	}
	if res.UniqueSkippedAsDuplicate {
		q.log.Debug("rollover already scheduled", "account", accountID, "on", on.At().Format(time.DateOnly))
	}
	return nil
}

/*
Scheduled is what is waiting, newest deadline last.

The inspection §3.4 promised — "inspectable with a SELECT rather than a
dashboard" — as a method, so that a health endpoint or a test can ask without
learning riverqueue/river's table layout. It is the same query a person would
type:

	SELECT state, scheduled_at, attempt, args
	  FROM river_job
	 WHERE kind = 'rollover' AND state IN ('available','scheduled','retryable','running')
	 ORDER BY scheduled_at;
*/
func (q *Queue) Scheduled(ctx context.Context, st *store.Store) ([]Pending, error) {
	rows, err := st.Pool().Query(ctx, `
		SELECT args->>'account', args->>'on', state::text, scheduled_at, attempt
		  FROM river_job
		 WHERE kind = $1
		   AND state IN ('available', 'scheduled', 'retryable', 'running', 'pending')
		 ORDER BY scheduled_at`, Rollover{}.Kind())
	if err != nil {
		return nil, fmt.Errorf("jobs: read scheduled rollovers: %w", err)
	}
	defer rows.Close()

	var out []Pending
	for rows.Next() {
		var p Pending
		if err := rows.Scan(&p.Account, &p.On, &p.State, &p.At, &p.Attempt); err != nil {
			return nil, fmt.Errorf("jobs: read scheduled rollovers: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Pending is one rollover waiting to run.
type Pending struct {
	Account string
	On      string
	State   string
	At      time.Time
	Attempt int
}

/*
quiet raises a logger's floor, for the queue's own chatter.

The development logger runs at debug (§13) so that the game's own decisions can
be read while playing. riverqueue/river at debug narrates its election, its
notifier, its poll loop and its six maintenance services several times a
second, which buries every line worth reading in a log nobody can then use.
Everything at or above the floor still comes through untouched — a client
starting, a job failing — so nothing is hidden, only the heartbeat.
*/
func quiet(log *slog.Logger, at slog.Level) *slog.Logger {
	return slog.New(floor{Handler: log.Handler(), at: at})
}

// floor is a handler that declines everything below a level.
type floor struct {
	slog.Handler
	at slog.Level
}

func (f floor) Enabled(ctx context.Context, l slog.Level) bool {
	return l >= f.at && f.Handler.Enabled(ctx, l)
}

func (f floor) WithAttrs(attrs []slog.Attr) slog.Handler {
	return floor{Handler: f.Handler.WithAttrs(attrs), at: f.at}
}

func (f floor) WithGroup(name string) slog.Handler {
	return floor{Handler: f.Handler.WithGroup(name), at: f.at}
}

/*
Migrate brings riverqueue/river's own tables up to date.

Run at boot, immediately after the game's migrations and against the same
database. It is separate from them on purpose (see the package comment), and it
is here rather than in internal/store because internal/store is the package that
knows a connection string exists and nothing else — teaching it about a queue
would make every future datastore concern its business too.
*/
func Migrate(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	// Warn, because the migrator says "no migrations to apply" at info on every
	// single boot and the line below already says the useful half.
	migrator, err := rivermigrate.New(riverpgxv5.New(pool),
		&rivermigrate.Config{Logger: quiet(log, slog.LevelWarn)})
	if err != nil {
		return fmt.Errorf("jobs: build migrator: %w", err)
	}
	res, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		return fmt.Errorf("jobs: migrate queue: %w", err)
	}
	if len(res.Versions) > 0 {
		log.Info("queue schema migrated", "versions", len(res.Versions))
	}
	return nil
}
