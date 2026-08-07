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

# River's schema

River owns its own tables and its own migration ledger, and it moves on its own
release schedule rather than the game's. Migrate below runs its migrator, next
to — not inside — the goose run that owns `migrations/`. Copying River's SQL
into that directory would pin the game's migration history to a library version
and break the first time the two disagreed about what version the database is
at.
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

// workers is how many rollovers this process runs at once.
//
// Each one is a handful of small statements in a single transaction, so the
// limit is the database's connection pool rather than CPU. Deliberately modest:
// the load is one job per player per day and it arrives spread across every
// timezone on earth, which is the least bursty arrival pattern a queue can have.
const workers = 10

/*
attempts is how many times a failed rollover is retried before it is discarded.

River's default is twenty-five, which with its backoff spans days — far too long
for work that is only meaningful on the day it is for. Eight attempts is a few
hours, which covers a database restart or a deploy and gives up before it starts
re-living a day nobody is waiting for. The next day's rollover catches the
player up anyway, because catch-up reads a date rather than a queue.
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
Queue is the running queue: a River client, its workers, and the game modules
they call into.

One per process. It is created stopped, so a binary can be built and its routes
wired before anything starts draining — which is what lets internal/api hold a
Queue that is not running yet in a test.
*/
type Queue struct {
	client    *river.Client[pgx.Tx]
	rollovers Runner
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
func New(st *store.Store, log *slog.Logger, rollovers Runner) (*Queue, error) {
	q := &Queue{rollovers: rollovers, log: log}

	w := river.NewWorkers()
	river.AddWorker(w, &rolloverWorker{q: q})

	client, err := river.NewClient(riverpgxv5.New(st.Pool()), &river.Config{
		Logger:  quiet(log, slog.LevelInfo),
		Workers: w,
		Queues:  map[string]river.QueueConfig{name: {MaxWorkers: workers}},
	})
	if err != nil {
		return nil, fmt.Errorf("jobs: build queue: %w", err)
	}
	q.client = client
	return q, nil
}

// Start begins draining. Idempotent, like River's own.
func (q *Queue) Start(ctx context.Context) error {
	if err := q.client.Start(ctx); err != nil {
		return fmt.Errorf("jobs: start queue: %w", err)
	}
	q.log.Info("job queue running", "queue", name, "workers", workers)
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

Transactional insert is the point. River will not see the row until the
transaction commits, so a rollover that failed halfway leaves no job behind, and
a ledger that moved forward without arming its successor is unreachable rather
than a state to recover from.

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
learning River's table layout. It is the same query a person would type:

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
be read while playing. River at debug narrates its election, its notifier, its
poll loop and its six maintenance services several times a second, which buries
every line worth reading in a log nobody can then use. Everything at or above
the floor still comes through untouched — a client starting, a job failing — so
nothing is hidden, only the heartbeat.
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
Migrate brings River's own tables up to date.

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
