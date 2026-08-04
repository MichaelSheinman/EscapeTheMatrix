package command_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store/storetest"
)

// The harness: a real Postgres, a registry of commands that do just enough to
// have observable state, and a way to send one command and look at both halves
// of what happened — the response, and what is now in the database.
//
// technical.md §14 puts commands on exactly this shape of test. The behaviours
// worth proving here are transactional (a rollback taking its idempotency claim
// with it; a duplicate blocking on a claim rather than racing it), and a mock
// would be asserting its own imagination.

type bumpInput struct {
	Counter string `json:"counter"`
}

type bumpResult struct {
	Counter string `json:"counter"`
	Count   int    `json:"count"`
}

type harness struct {
	t    *testing.T
	exec *command.Executor
	reg  *command.Registry
	st   *store.Store
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	st := storetest.New(t)

	if _, err := st.Pool().Exec(t.Context(), `
		CREATE TABLE counter (
			name  text PRIMARY KEY,
			count int NOT NULL DEFAULT 0
		)
	`); err != nil {
		t.Fatalf("create counter table: %v", err)
	}

	reg := command.NewRegistry()

	// bump is the stand-in for every command that changes the world: it writes,
	// and the write is visible afterwards whether or not the response was.
	command.Register(reg, "bump", func(ctx context.Context, tx pgx.Tx, actor command.Actor, in bumpInput) (bumpResult, error) {
		out := bumpResult{Counter: in.Counter}
		err := tx.QueryRow(ctx, `
			INSERT INTO counter (name, count) VALUES ($1, 1)
			ON CONFLICT (name) DO UPDATE SET count = counter.count + 1
			RETURNING count
		`, in.Counter).Scan(&out.Count)
		return out, err
	})

	// Writes, then fails. The write must not survive.
	command.Register(reg, "half.done", func(ctx context.Context, tx pgx.Tx, _ command.Actor, in bumpInput) (bumpResult, error) {
		if _, err := tx.Exec(ctx, `INSERT INTO counter (name, count) VALUES ($1, 1)`, in.Counter); err != nil {
			return bumpResult{}, err
		}
		return bumpResult{}, errors.New("the disk caught fire")
	})

	command.Register(reg, "refused", func(context.Context, pgx.Tx, command.Actor, struct{}) (struct{}, error) {
		return struct{}{}, command.Conflict("you cannot afford that")
	})

	command.Register(reg, "rejected", func(context.Context, pgx.Tx, command.Actor, struct{}) (struct{}, error) {
		return struct{}{}, command.Invalid("that is not a number")
	})

	return &harness{t: t, exec: command.NewExecutor(reg, st, quietLogger()), reg: reg, st: st}
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// send delivers one command. body may be empty.
func (h *harness) send(name, key, body string) command.Result {
	h.t.Helper()
	return h.exec.Execute(h.t.Context(), command.Request{
		Name:  name,
		Key:   key,
		Actor: command.Anonymous,
		Body:  []byte(body),
	})
}

// count reads the state a command was supposed to have changed.
func (h *harness) count(name string) int {
	h.t.Helper()
	var n int
	err := h.st.Pool().QueryRow(h.t.Context(),
		`SELECT coalesce((SELECT count FROM counter WHERE name = $1), 0)`, name).Scan(&n)
	if err != nil {
		h.t.Fatalf("read counter %q: %v", name, err)
	}
	return n
}

func (h *harness) records() int {
	h.t.Helper()
	var n int
	if err := h.st.Pool().QueryRow(h.t.Context(),
		`SELECT count(*) FROM command_idempotency`).Scan(&n); err != nil {
		h.t.Fatalf("count idempotency records: %v", err)
	}
	return n
}

func requireStatus(t *testing.T, res command.Result, want int) {
	t.Helper()
	if res.Status != want {
		t.Fatalf("status = %d (%s), want %d", res.Status, res.Body, want)
	}
}

// The ticket in one test: a command delivered twice applies once and answers
// the same both times.
func TestDuplicateCommandAppliesOnce(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	first := h.send("bump", "key-1", `{"counter":"money"}`)
	second := h.send("bump", "key-1", `{"counter":"money"}`)

	requireStatus(t, first, http.StatusOK)
	requireStatus(t, second, http.StatusOK)

	if got := h.count("money"); got != 1 {
		t.Errorf("counter = %d after the same command twice, want 1", got)
	}
	if string(first.Body) != string(second.Body) {
		t.Errorf("retry answered %s, want the original %s", second.Body, first.Body)
	}
	if first.Replayed {
		t.Error("the first delivery reported itself as a replay")
	}
	if !second.Replayed {
		t.Error("the second delivery was not marked as a replay")
	}
}

// The other half of the same guarantee: idempotency must not make a player's
// second deliberate action disappear.
func TestDistinctKeysBothApply(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	requireStatus(t, h.send("bump", "key-1", `{"counter":"money"}`), http.StatusOK)
	requireStatus(t, h.send("bump", "key-2", `{"counter":"money"}`), http.StatusOK)

	if got := h.count("money"); got != 2 {
		t.Errorf("counter = %d after two distinct commands, want 2", got)
	}
}

// A duplicate that arrives while the first is still in flight — the mobile
// network case — is the one a naive check-then-act loses.
func TestConcurrentDuplicatesApplyOnce(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	var wg sync.WaitGroup
	results := make([]command.Result, 2)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = h.send("bump", "key-1", `{"counter":"money"}`)
		}()
	}
	wg.Wait()

	if got := h.count("money"); got != 1 {
		t.Errorf("counter = %d after two concurrent duplicates, want 1", got)
	}

	replays := 0
	for _, res := range results {
		requireStatus(t, res, http.StatusOK)
		if res.Replayed {
			replays++
		}
	}
	if replays != 1 {
		t.Errorf("%d of 2 concurrent deliveries were replays, want exactly 1", replays)
	}
	if string(results[0].Body) != string(results[1].Body) {
		t.Errorf("concurrent duplicates answered differently: %s vs %s", results[0].Body, results[1].Body)
	}
}

// Sharing a key between two different requests is a client bug. Answering the
// second with the first's result would apply neither.
func TestKeyReusedForADifferentRequestIsRejected(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	requireStatus(t, h.send("bump", "key-1", `{"counter":"money"}`), http.StatusOK)
	res := h.send("bump", "key-1", `{"counter":"karma"}`)

	requireStatus(t, res, http.StatusUnprocessableEntity)
	if got := h.count("karma"); got != 0 {
		t.Errorf("karma = %d, want 0 — the rejected command must not have applied", got)
	}
}

// The key is scoped by command as well, so two commands cannot collide on one
// client's key.
func TestSameKeyOnADifferentCommandIsItsOwn(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	requireStatus(t, h.send("bump", "key-1", `{"counter":"money"}`), http.StatusOK)
	requireStatus(t, h.send("refused", "key-1", `{}`), http.StatusConflict)
}

// Status codes are the protocol: a client must be able to tell "you cannot do
// that yet" from "that request is wrong" without reading the message.
func TestInvalidStateAndValidationHaveDistinctStatuses(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	requireStatus(t, h.send("refused", "key-1", `{}`), http.StatusConflict)
	requireStatus(t, h.send("rejected", "key-2", `{}`), http.StatusUnprocessableEntity)
}

// A field the server does not know is client/server drift, and finding it here
// is the point of generating the client's types from these structs (§8).
func TestUnknownFieldIsAValidationFailure(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	requireStatus(t, h.send("bump", "key-1", `{"countr":"money"}`), http.StatusUnprocessableEntity)
	if got := h.count("money"); got != 0 {
		t.Errorf("counter = %d, want 0", got)
	}
}

// Everything the handler did is one transaction, including the record that it
// ran. A command that failed is a command that never happened — so its key is
// free and the retry is a real attempt rather than a replayed failure.
func TestFailedCommandLeavesNoTrace(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	res := h.send("half.done", "key-1", `{"counter":"money"}`)

	requireStatus(t, res, http.StatusInternalServerError)
	if got := h.count("money"); got != 0 {
		t.Errorf("counter = %d, want 0 — the failed command's write survived", got)
	}
	if got := h.records(); got != 0 {
		t.Errorf("%d idempotency records, want 0 — the claim survived its transaction", got)
	}

	// And the same key now belongs to nobody, so a retry runs for real.
	requireStatus(t, h.send("bump", "key-1", `{"counter":"money"}`), http.StatusOK)
	if got := h.count("money"); got != 1 {
		t.Errorf("counter = %d after retrying the freed key, want 1", got)
	}
}

// A refusal is not an outcome worth recording either: the world may move, and
// the same command may then be allowed.
func TestRefusedCommandIsNotRecorded(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	requireStatus(t, h.send("refused", "key-1", `{}`), http.StatusConflict)
	if got := h.records(); got != 0 {
		t.Errorf("%d idempotency records after a 409, want 0", got)
	}
}

// A 500 must never carry the underlying error to the client — that text is
// where SQL and connection strings live.
func TestInternalErrorsAreNotLeaked(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	res := h.send("half.done", "key-1", `{"counter":"money"}`)

	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(res.Body, &body); err != nil {
		t.Fatalf("error body is not JSON: %v", err)
	}
	if body.Error == "" {
		t.Fatal("error body carries no message")
	}
	if body.Error == "the disk caught fire" {
		t.Errorf("error body leaks the internal failure: %q", body.Error)
	}
}

func TestUnknownCommandIsNotFound(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	requireStatus(t, h.send("no.such.command", "key-1", `{}`), http.StatusNotFound)
}

// Without a key a retry is indistinguishable from a second deliberate action,
// so the command is refused rather than run once too often.
func TestMissingIdempotencyKeyIsRefused(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	requireStatus(t, h.send("bump", "", `{"counter":"money"}`), http.StatusBadRequest)
	if got := h.count("money"); got != 0 {
		t.Errorf("counter = %d, want 0 — a keyless command ran", got)
	}
}

func TestUnusableIdempotencyKeyIsRefused(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	for _, key := range []string{"has space", "line\nbreak", string(make([]byte, 400))} {
		requireStatus(t, h.send("bump", key, `{"counter":"money"}`), http.StatusBadRequest)
	}
}

// Servers hold no per-player state (§3.2), so a retry that lands on a different
// instance than the delivery it duplicates must still replay. Two executors
// over one database is that situation, minus the network.
func TestReplayCrossesServerInstances(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	other := command.NewExecutor(h.reg, h.st, quietLogger())

	first := h.send("bump", "key-1", `{"counter":"money"}`)
	second := other.Execute(t.Context(), command.Request{
		Name: "bump", Key: "key-1", Actor: command.Anonymous,
		Body: []byte(`{"counter":"money"}`),
	})

	requireStatus(t, second, http.StatusOK)
	if !second.Replayed {
		t.Error("the second instance did not recognise the duplicate")
	}
	if string(second.Body) != string(first.Body) {
		t.Errorf("the second instance answered %s, want %s", second.Body, first.Body)
	}
	if got := h.count("money"); got != 1 {
		t.Errorf("counter = %d, want 1", got)
	}
}

// Different actors sharing a key is not a duplicate: the record holds a
// response body, and handing one player another's would be a data leak.
func TestKeysAreScopedToTheActor(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	requireStatus(t, h.send("bump", "key-1", `{"counter":"money"}`), http.StatusOK)

	res := h.exec.Execute(t.Context(), command.Request{
		Name: "bump", Key: "key-1", Actor: command.Actor{ID: "someone-else"},
		Body: []byte(`{"counter":"money"}`),
	})

	requireStatus(t, res, http.StatusOK)
	if res.Replayed {
		t.Error("one actor's command replayed another's result")
	}
	if got := h.count("money"); got != 2 {
		t.Errorf("counter = %d, want 2 — the second actor's command did not apply", got)
	}
}

// An empty body is an empty object, so a command that takes no arguments needs
// no payload.
func TestEmptyBodyIsAnEmptyObject(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	requireStatus(t, h.send("bump", "key-1", ``), http.StatusOK)
	if got := h.count(""); got != 1 {
		t.Errorf("counter = %d, want 1", got)
	}
}
