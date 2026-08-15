package command

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/wire"
)

// maxKeyLen bounds what a client may send as an idempotency key. It is a
// primary-key column, and an unbounded one is a way to fill a table with a
// single request.
const maxKeyLen = 200

// Request is one command arriving from a client, with the transport stripped
// off. The HTTP layer builds it; nothing below this line knows about headers.
type Request struct {
	// Name addresses the handler — the {name} in POST /api/commands/{name}.
	Name string
	// Key is the client's Idempotency-Key. Required: a command without one
	// cannot be made to apply once, so it is refused rather than run.
	Key string
	// Actor is who is issuing it (see Actor).
	Actor Actor
	// Body is the raw JSON payload, possibly empty.
	Body []byte
}

// Result is the answer, ready to be written. Body is always valid JSON.
type Result struct {
	Status int
	Body   []byte
	// Replayed reports that this answer was recorded by an earlier delivery of
	// the same command rather than produced now. Surfaced as a response header
	// so a duplicate is visible in the network panel (§8: debuggability).
	Replayed bool
}

// Executor runs registered commands against Postgres.
//
// One instance is built at boot and shared by every request. It holds a
// registry, a store, and a logger — no per-player state of any kind, which is
// what makes the servers interchangeable (§3.2).
type Executor struct {
	registry *Registry
	store    *store.Store
	log      *slog.Logger
}

// NewExecutor binds a registry to the database its handlers will run against.
func NewExecutor(r *Registry, st *store.Store, log *slog.Logger) *Executor {
	return &Executor{registry: r, store: st, log: log}
}

// Execute runs one command to completion and always produces a Result — a
// failure is a status code, never a returned error, because every failure has
// to reach the client as the protocol rather than as a panic in the handler.
//
// The sequence inside the single transaction is what makes a duplicate safe:
//
//  1. claim the idempotency key;
//  2. if the claim was taken, the command already ran — return what it returned;
//  3. otherwise run the handler and record its answer.
//
// Because the claim and the effect are the same transaction, there is no window
// in which one exists without the other. A second delivery arriving while the
// first is still running blocks on the claim rather than racing it.
func (e *Executor) Execute(ctx context.Context, req Request) Result {
	started := time.Now()

	handler, ok := e.registry.lookup(req.Name)
	if !ok {
		return e.answer(req, started, NotFound("no command named %q", req.Name), Result{})
	}
	if err := validateKey(req.Key); err != nil {
		return e.answer(req, started, err, Result{})
	}

	hash := sha256.Sum256(req.Body)
	var res Result

	err := e.store.InTx(ctx, func(tx pgx.Tx) error {
		prior, claimed, err := claim(ctx, tx, req, hash[:])
		if err != nil {
			return err
		}

		if !claimed {
			// The same key with a different payload is a client bug — two
			// different commands sharing a key — and answering it with the
			// first one's result would apply neither.
			if !bytes.Equal(prior.hash, hash[:]) {
				return Invalid("idempotency key %q was already used for a different request", req.Key)
			}
			if prior.response == nil {
				// Unreachable while every path records before committing;
				// worth naming rather than returning a null body.
				return fmt.Errorf("idempotency record for key %q has no result", req.Key)
			}
			res = Result{Status: prior.status, Body: prior.response, Replayed: true}
			return nil
		}

		value, err := handler(ctx, tx, req.Actor, req.Body)
		if err != nil {
			return err
		}
		// wire, not encoding/json: a command's result reaches the client the
		// same way a REST body does, so it gets the same guarantee that no
		// collection in it is null.
		body, err := wire.Marshal(value)
		if err != nil {
			return fmt.Errorf("encode result of %q: %w", req.Name, err)
		}

		res = Result{Status: http.StatusOK, Body: body}
		return record(ctx, tx, req, res.Status, body)
	})
	if err != nil {
		return e.answer(req, started, err, Result{})
	}

	return e.answer(req, started, nil, res)
}

// answer logs the outcome and turns an error into a body. It is the single exit
// from Execute so that every command produces exactly one log line.
func (e *Executor) answer(req Request, started time.Time, err error, res Result) Result {
	if err != nil {
		status, message := statusOf(err)
		res = Result{Status: status, Body: errorBody(message)}

		if status >= http.StatusInternalServerError {
			// The client is told nothing useful about a 500, so the log is the
			// only place the real cause exists.
			e.log.Error("command failed",
				"command", req.Name, "actor", req.Actor.ID, "key", req.Key, "error", err)
		}
	}

	e.log.Debug("command",
		"command", req.Name,
		"actor", req.Actor.ID,
		"status", res.Status,
		"replayed", res.Replayed,
		"took", time.Since(started),
	)
	return res
}

func errorBody(message string) []byte {
	body, err := json.Marshal(map[string]string{"error": message})
	if err != nil {
		// message is a plain string, so this cannot fail; if it somehow does,
		// hand-written JSON still keeps the response decodable.
		return []byte(`{"error":"` + unexpected + `"}`)
	}
	return body
}

// priorResult is a recorded answer to a command that already ran.
type priorResult struct {
	hash     []byte
	status   int
	response []byte
}

// claim takes the idempotency key for this command, reporting whether it was
// this transaction that took it.
//
// When it was not, the answer already recorded under that key comes back
// instead. The ON CONFLICT arm is the concurrency story: a second transaction
// inserting the same key waits on the first's uncommitted row, and then — in
// READ COMMITTED, where each statement takes a fresh snapshot — the SELECT that
// follows sees the result the first one committed.
func claim(ctx context.Context, tx pgx.Tx, req Request, hash []byte) (priorResult, bool, error) {
	var taken bool
	err := tx.QueryRow(ctx, `
		INSERT INTO command_idempotency (actor, command, key, request_hash)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT DO NOTHING
		RETURNING true
	`, req.Actor.ID, req.Name, req.Key, hash).Scan(&taken)

	switch {
	case err == nil:
		return priorResult{}, true, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return priorResult{}, false, fmt.Errorf("claim idempotency key: %w", err)
	}

	var prior priorResult
	var status *int16
	if err := tx.QueryRow(ctx, `
		SELECT request_hash, status, response
		FROM command_idempotency
		WHERE actor = $1 AND command = $2 AND key = $3
	`, req.Actor.ID, req.Name, req.Key).Scan(&prior.hash, &status, &prior.response); err != nil {
		return priorResult{}, false, fmt.Errorf("read idempotency record: %w", err)
	}
	if status != nil {
		prior.status = int(*status)
	}
	return prior, false, nil
}

// record stores the answer against the claim, in the same transaction that
// produced it.
func record(ctx context.Context, tx pgx.Tx, req Request, status int, body []byte) error {
	if _, err := tx.Exec(ctx, `
		UPDATE command_idempotency
		SET status = $4, response = $5
		WHERE actor = $1 AND command = $2 AND key = $3
	`, req.Actor.ID, req.Name, req.Key, int16(status), body); err != nil {
		return fmt.Errorf("record idempotency result: %w", err)
	}
	return nil
}

// validateKey rejects what cannot serve as an idempotency key. A missing key is
// the important case: without one a retry is indistinguishable from a second
// deliberate action, so the command is refused rather than run once too often.
func validateKey(key string) error {
	if key == "" {
		return Malformed("this command requires an Idempotency-Key header")
	}
	if len(key) > maxKeyLen {
		return Malformed("the Idempotency-Key header must be at most %d characters", maxKeyLen)
	}
	for _, c := range key {
		if c < '!' || c > '~' {
			return Malformed("the Idempotency-Key header must be printable ASCII without spaces")
		}
	}
	return nil
}
