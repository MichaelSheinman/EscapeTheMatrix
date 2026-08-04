// Package command is the single path every player action takes.
//
// technical.md §8: `POST /api/commands/{name}`, one ordinary handler each,
// carrying an `Idempotency-Key`. This package owns what that buys —
//
//   - a command applies **once**, no matter how many times the network or a
//     page reload delivers it, and a retry gets the original answer back;
//   - a command runs inside **one** Postgres transaction (§3.2), so its effect
//     and the record that it happened commit together or not at all;
//   - **status codes are the protocol** — 409 means the world was not in a
//     state that allows this, 422 means the request itself was wrong, and the
//     client can tell them apart without reading prose.
//
// Handlers live in the module that owns the tables they touch (§3.1) and are
// registered here at boot. They receive a transaction and return a value; they
// never see an http.ResponseWriter, never choose a status code except through
// the error constructors below, and never open their own transaction.
//
// The package holds no per-player state. Everything a command needs comes from
// its arguments or from Postgres, which is what keeps servers stateless and
// rolling deploys lossless (§3.2).
package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Actor is who issued the command: the id of the signed-in account, resolved
// from the session cookie by the middleware in front of the command route (§9).
//
// It scopes idempotency records: those hold a response body, so replaying one
// player's answer to another player's key would be a data leak by construction.
//
// There is no anonymous actor. §9 makes an account the price of entry, so a
// command with no player behind it is refused at the door rather than given a
// name to run under.
type Actor struct {
	ID string
}

// Handler is one command.
//
// It runs inside a transaction that the executor opened and will commit; the
// value it returns is what the client sees, encoded as JSON. Returning an error
// rolls the transaction back — including the idempotency claim, so a command
// that failed is a command that never happened and may be retried with the same
// key.
type Handler[In, Out any] func(ctx context.Context, tx pgx.Tx, actor Actor, in In) (Out, error)

// Registry maps command names to handlers. It is built at boot and read-only
// afterwards: no command appears or disappears while the process is serving.
type Registry struct {
	handlers map[string]invoker
}

// invoker is a Handler with its input type erased, so that handlers of
// different shapes can share one map.
type invoker func(ctx context.Context, tx pgx.Tx, actor Actor, body []byte) (any, error)

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{handlers: make(map[string]invoker)}
}

// Register adds a command under name.
//
// It is a package function rather than a method because Go methods cannot
// introduce type parameters, and the input type is what makes decoding and its
// 422 free for every handler.
//
// Both failure modes panic: a malformed name and a duplicate registration are
// programming errors that happen at boot, where a crash is the cheapest
// possible feedback.
func Register[In, Out any](r *Registry, name string, h Handler[In, Out]) {
	if !validName(name) {
		panic(fmt.Sprintf("command: %q is not a usable command name", name))
	}
	if _, dup := r.handlers[name]; dup {
		panic(fmt.Sprintf("command: %q registered twice", name))
	}

	r.handlers[name] = func(ctx context.Context, tx pgx.Tx, actor Actor, body []byte) (any, error) {
		in, err := decode[In](body)
		if err != nil {
			return nil, err
		}
		return h(ctx, tx, actor, in)
	}
}

// Names lists the registered commands, unordered. Diagnostics only.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		names = append(names, name)
	}
	return names
}

func (r *Registry) lookup(name string) (invoker, bool) {
	h, ok := r.handlers[name]
	return h, ok
}

// decode turns a request body into a handler's input.
//
// An absent body is an empty object, so a command that takes no arguments can
// be sent with no payload. Unknown fields are rejected rather than ignored:
// the client's types are generated from these structs (§8), so a field the
// server does not know is drift, and drift found at the request is drift not
// found in production.
func decode[In any](body []byte) (In, error) {
	var in In
	if len(bytes.TrimSpace(body)) == 0 {
		body = []byte("{}")
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return in, Invalid("the request body could not be read: %v", err)
	}
	// A second value in the body means the client sent something other than one
	// JSON object, and only the first would have been applied.
	if dec.More() {
		return in, Invalid("the request body must be a single JSON object")
	}
	return in, nil
}

// validName accepts lowercase dotted names — `probe`, `challenge.advance`.
//
// The route is `POST /api/commands/{name}`, so the name is a URL segment and a
// log field and a metric label; constraining it at registration keeps all three
// readable and keeps a stray `/` from silently never matching.
func validName(name string) bool {
	if name == "" {
		return false
	}
	atSegmentStart := true
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z':
		case (c >= '0' && c <= '9') && !atSegmentStart:
		case c == '.' && !atSegmentStart:
			atSegmentStart = true
			continue
		default:
			return false
		}
		atSegmentStart = false
	}
	// A trailing dot leaves the last segment empty.
	return !atSegmentStart
}
