// An external test package, deliberately. `internal/store/storetest` migrates
// this package's schema alongside the game's — because a test database that is
// not the production database in every respect lets a missing table through —
// and a test compiled *into* this package could therefore not import it.
package jobs_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/jobs"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store/storetest"
)

/*
The queue, against a real Postgres (§14).

§3.4 chose a durable Postgres-backed queue over a `time.Ticker` for four
properties, and these are tests of those four rather than of River: a job
survives the process that made it, it is worked, it is retried when it fails,
and it is visible in a `SELECT`. The fifth — that an enqueue rolls back with the
transaction that made it — is the one that makes "the ledger moved forward
without arming its successor" an unreachable state rather than a bug to recover
from, so it is here too.
*/

// A job outlives the process that scheduled it. This is the whole argument for
// a queue over a goroutine: a deploy in the middle of the night does not cost
// anybody their rollover.
func TestAScheduledRolloverSurvivesTheProcessThatMadeIt(t *testing.T) {
	t.Parallel()
	st := storetest.New(t)
	ctx := t.Context()
	account := newAccount(t, ctx, st)

	first, _ := newQueue(t, st)
	schedule(t, ctx, st, first, account, time.Now().Add(time.Hour))
	if err := first.Stop(ctx); err != nil {
		t.Fatalf("stop: %v", err)
	}

	// A different client, over the same database — which is what the next
	// process to start is.
	second, _ := newQueue(t, st)
	pending, err := second.Scheduled(ctx, st)
	if err != nil {
		t.Fatalf("read scheduled: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("a new process sees %d scheduled rollovers, want 1", len(pending))
	}
	if pending[0].Account != account {
		t.Errorf("the job is for %s, want %s", pending[0].Account, account)
	}
	// It says what it is for without anybody having to decode anything, which
	// is what "inspectable with a SELECT rather than a dashboard" means.
	if pending[0].On == "" {
		t.Error("the job does not say which day it is for")
	}
}

// A job that is due is worked, by a worker in this same binary (§3.4).
func TestADueRolloverIsWorked(t *testing.T) {
	t.Parallel()
	st := storetest.New(t)
	ctx := t.Context()
	account := newAccount(t, ctx, st)

	q, runner := newQueue(t, st)
	if err := q.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = q.Stop(context.Background()) })

	schedule(t, ctx, st, q, account, time.Now())

	if got := runner.waitFor(1); got != 1 {
		t.Fatalf("the rollover ran %d times, want 1", got)
	}
	if got := runner.accounts(); got[0] != account {
		t.Errorf("it ran for %s, want %s", got[0], account)
	}
}

// A rollover that fails is retried rather than lost. The row stays, carrying
// the attempt count, which is the visibility a ticker has none of.
func TestAFailedRolloverIsRetriedRatherThanLost(t *testing.T) {
	t.Parallel()
	st := storetest.New(t)
	ctx := t.Context()
	account := newAccount(t, ctx, st)

	q, runner := newQueue(t, st)
	runner.fail = errors.New("the database went away")
	if err := q.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = q.Stop(context.Background()) })

	schedule(t, ctx, st, q, account, time.Now())
	runner.waitFor(1)

	// Still there, still ours, and now carrying an attempt.
	deadline := time.Now().Add(10 * time.Second)
	for {
		pending, err := q.Scheduled(ctx, st)
		if err != nil {
			t.Fatalf("read scheduled: %v", err)
		}
		if len(pending) == 1 && pending[0].Attempt >= 1 && pending[0].State == "retryable" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the failed rollover did not become retryable: %+v", pending)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// One rollover per player per day, however many things ask for it. A scheduled
// run and a catch-up racing each other collapse into one job rather than into
// two runs of the same day.
func TestTwoRolloversForTheSameDayCollapseIntoOne(t *testing.T) {
	t.Parallel()
	st := storetest.New(t)
	ctx := t.Context()
	account := newAccount(t, ctx, st)

	q, _ := newQueue(t, st)
	at := time.Now().Add(time.Hour)
	schedule(t, ctx, st, q, account, at)
	schedule(t, ctx, st, q, account, at.Add(time.Minute))

	pending, err := q.Scheduled(ctx, st)
	if err != nil {
		t.Fatalf("read scheduled: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("two inserts left %d jobs, want 1", len(pending))
	}
}

// Different days are different jobs, so tomorrow's rollover can be armed from
// inside today's without colliding with the one that is running.
func TestADifferentDayIsADifferentJob(t *testing.T) {
	t.Parallel()
	st := storetest.New(t)
	ctx := t.Context()
	account := newAccount(t, ctx, st)

	q, _ := newQueue(t, st)
	on := simulation.Date{Year: 2026, Month: time.August, Day: 6}
	if err := st.InTx(ctx, func(tx pgx.Tx) error {
		if err := q.Schedule(ctx, tx, account, on, time.Now().Add(time.Hour)); err != nil {
			return err
		}
		return q.Schedule(ctx, tx, account, on.Next(), time.Now().Add(25*time.Hour))
	}); err != nil {
		t.Fatalf("schedule: %v", err)
	}

	pending, err := q.Scheduled(ctx, st)
	if err != nil {
		t.Fatalf("read scheduled: %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("two days left %d jobs, want 2", len(pending))
	}
}

// The property the whole design leans on: a job enqueued by work that then
// rolled back never existed. There is no state in which a ledger moved forward
// without its successor, and none in which a successor exists for work that
// did not happen.
func TestAJobEnqueuedInARolledBackTransactionNeverExists(t *testing.T) {
	t.Parallel()
	st := storetest.New(t)
	ctx := t.Context()
	account := newAccount(t, ctx, st)

	q, _ := newQueue(t, st)

	stop := errors.New("the rollover failed after enqueueing")
	err := st.InTx(ctx, func(tx pgx.Tx) error {
		if err := q.Schedule(ctx, tx, account, today(), time.Now().Add(time.Hour)); err != nil {
			return err
		}
		return stop
	})
	if !errors.Is(err, stop) {
		t.Fatalf("InTx returned %v, want the failure", err)
	}

	pending, err := q.Scheduled(ctx, st)
	if err != nil {
		t.Fatalf("read scheduled: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("a rolled-back transaction left %d jobs behind", len(pending))
	}
}

/* ── the harness ──────────────────────────────────────────────────────── */

func newQueue(t *testing.T, st *store.Store) (*jobs.Queue, *runner) {
	t.Helper()
	r := &runner{}
	q, err := jobs.New(st, slog.New(slog.NewTextHandler(io.Discard, nil)), r)
	if err != nil {
		t.Fatalf("build queue: %v", err)
	}
	return q, r
}

func schedule(t *testing.T, ctx context.Context, st *store.Store, q *jobs.Queue, account string, at time.Time) {
	t.Helper()
	if err := st.InTx(ctx, func(tx pgx.Tx) error {
		return q.Schedule(ctx, tx, account, today(), at)
	}); err != nil {
		t.Fatalf("schedule: %v", err)
	}
}

func today() simulation.Date { return simulation.Date{Year: 2026, Month: time.August, Day: 6} }

// newAccount makes somebody for the jobs to be about. River does not care, but
// a test that used an id no account has would be testing a queue nobody could
// schedule from.
func newAccount(t *testing.T, ctx context.Context, st *store.Store) string {
	t.Helper()
	var id string
	if err := st.InTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO account (provider, subject, email, display_name)
			VALUES ('google', 'jobs-test', 'jobs@example.com', 'Jobs')
			RETURNING id`).Scan(&id)
	}); err != nil {
		t.Fatalf("create account: %v", err)
	}
	return id
}

// runner is a Runner that writes down who it was asked about, and can be told
// to fail.
type runner struct {
	mu   sync.Mutex
	ran  []string
	fail error
}

func (r *runner) Due(_ context.Context, accountID string) error {
	r.mu.Lock()
	r.ran = append(r.ran, accountID)
	r.mu.Unlock()
	return r.fail
}

func (r *runner) accounts() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.ran...)
}

// waitFor blocks until the runner has been called n times, or gives up.
//
// Polling rather than a channel because the thing being waited on is a
// background worker in another goroutine picking work off a table — there is no
// point in the test that is synchronous with it, and a fixed sleep would be
// either flaky or slow.
func (r *runner) waitFor(n int) int {
	deadline := time.Now().Add(15 * time.Second)
	for {
		got := len(r.accounts())
		if got >= n || time.Now().After(deadline) {
			return got
		}
		time.Sleep(20 * time.Millisecond)
	}
}
