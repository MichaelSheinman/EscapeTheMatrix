// Package realtime is the server→client half of the protocol.
//
// technical.md §8 splits the two directions on purpose: commands go over REST,
// and everything the server decides on its own — event releases, results of
// things that happened while the player was elsewhere, leaderboard movement,
// chat — comes down one WebSocket. This package owns that socket and the four
// protocol rules that make it survivable:
//
//   - **A typed envelope.** Every message is `{type, seq, payload}` with a Go
//     struct behind the payload and a TypeScript type generated from it
//     (typescript.go). Two hand-maintained lists drift, and you find out in
//     production.
//   - **Resume, not reconnect.** The client tracks the last seq it processed
//     and sends it when it comes back; the server replays anything newer from a
//     short-lived per-player outbox. Without this a reconnecting player
//     silently misses an event release and the world appears to skip.
//   - **Keyed by session.** A message records the tab whose command caused it,
//     and that tab's socket skips it — the acting client already applied the
//     command's HTTP response, so an echo would apply it twice. Every other tab
//     of the same player receives it.
//   - **Fan-out through Postgres.** A message published on one instance reaches
//     a socket on another through `LISTEN`/`NOTIFY`, which keeps the dependency
//     list at one datastore (§1).
//
// The outbox is the single path. A notification carries no payload — it is a
// doorbell — so a live delivery and a resume are the same query against the
// same rows, and a message that was written cannot fail to be delivered by a
// connection that outlives it.
package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/wire"
)

// Retention is how long a message stays in the outbox.
//
// Long enough to cover a drop — a laptop lid, a deploy, a tunnel — and no
// longer. The outbox is not a history: a client resuming from further back than
// this is told there is a gap and rehydrates over REST, which is exactly what
// opening the page does anyway.
const Retention = time.Hour

// Envelope is every server→client message, on the wire.
//
// Seq is the player's own sequence number, gapless and increasing, and is what
// a client sends back to resume. Seq 0 is reserved for connection control —
// the welcome below — which is not part of the ordered stream and never
// advances a client's cursor.
type Envelope struct {
	Type    string          `json:"type"`
	Seq     int64           `json:"seq"`
	Payload json.RawMessage `json:"payload"`
}

// Kind is one message type: a name on the wire and the Go type of its payload.
//
// Values are created by Define at package initialisation and are the only way
// to publish, so a message on the socket always has a registered type and a
// generated TypeScript counterpart. There is no way to send an ad-hoc map.
type Kind[P any] struct{ name string }

// Name is the string that appears in the envelope's type field.
func (k Kind[P]) Name() string { return k.name }

// definitions is the registry the TypeScript generator reads. It is written
// only by Define, during package initialisation, and read-only afterwards.
var definitions = map[string]reflect.Type{}

// Define registers a message type and returns the handle used to publish it.
//
// Call it in a package-level var, in the module that owns the thing being
// described (§3.1) — the message and the table it reports on belong together.
// A duplicate name panics: it happens at boot, where a crash is the cheapest
// possible feedback, and two payload shapes under one name is a client that
// decodes one of them wrongly.
func Define[P any](name string) Kind[P] {
	if !validName(name) {
		panic(fmt.Sprintf("realtime: %q is not a usable message type", name))
	}
	if _, dup := definitions[name]; dup {
		panic(fmt.Sprintf("realtime: message type %q defined twice", name))
	}

	t := reflect.TypeFor[P]()
	if t.Kind() != reflect.Struct {
		// The payload is an object on the wire, so that a message can gain a
		// field without every client's decoding changing shape.
		panic(fmt.Sprintf("realtime: payload of %q must be a struct, got %s", name, t.Kind()))
	}

	definitions[name] = t
	return Kind[P]{name: name}
}

// definedNames lists every registered type, sorted. Sorted rather than in
// definition order because that order depends on which packages the importing
// binary happens to pull in, and generated output has to be stable.
func definedNames() []string {
	names := make([]string, 0, len(definitions))
	for name := range definitions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// validName accepts the same lowercase dotted names commands use, for the same
// reason: a message type is a log field, a metric label, and a discriminant in
// generated TypeScript.
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
	return !atSegmentStart
}

// To is the audience for one message: which player, and which of their tabs
// caused it.
type To struct {
	// Account is the player the message is for. Messages are addressed to a
	// player, never to a connection — the point of the outbox is that it
	// outlives the connection.
	Account string
	// Origin is the session id of the tab whose command caused this, taken
	// from the command's actor. Empty when the world acted on its own, in
	// which case every tab receives it.
	Origin string
}

// Publish appends a message to a player's outbox and rings the doorbell.
//
// It takes the caller's transaction rather than opening its own, which is the
// whole of its correctness: the message and the effect it describes commit
// together or not at all. A command that rolled back has told nobody anything,
// and a command that committed cannot fail to have said so. `pg_notify` is
// transactional for the same reason — the notification is queued by Postgres
// and delivered on commit, so no listener can be woken to read a row that is
// not there yet.
func (k Kind[P]) Publish(ctx context.Context, tx pgx.Tx, to To, payload P) error {
	if to.Account == "" {
		return fmt.Errorf("realtime: %s published to no account", k.name)
	}

	// wire, not encoding/json: the outbox row is what the client eventually
	// reads, so the no-null-collections guarantee has to be applied before it
	// is stored, not on the way out of the socket.
	body, err := wire.Marshal(payload)
	if err != nil {
		return fmt.Errorf("realtime: encode %s: %w", k.name, err)
	}

	var origin *string
	if to.Origin != "" {
		origin = &to.Origin
	}

	var seq int64
	if err := tx.QueryRow(ctx, `
		WITH claimed AS (
			INSERT INTO realtime_stream (account_id, last_seq)
			VALUES ($1, 1)
			ON CONFLICT (account_id) DO UPDATE
			SET last_seq = realtime_stream.last_seq + 1
			RETURNING last_seq
		)
		INSERT INTO realtime_outbox (account_id, seq, type, payload, origin)
		SELECT $1, last_seq, $2, $3, $4 FROM claimed
		RETURNING seq
	`, to.Account, k.name, body, origin).Scan(&seq); err != nil {
		return fmt.Errorf("realtime: publish %s: %w", k.name, err)
	}

	// The payload deliberately stays out of the notification: NOTIFY has an
	// 8000-byte ceiling, and a message that travels by two routes is a message
	// with two ways of arriving wrong. This says only "there is something new
	// for this player".
	if _, err := tx.Exec(ctx, `SELECT pg_notify($1, $2)`, channel, to.Account); err != nil {
		return fmt.Errorf("realtime: notify: %w", err)
	}
	return nil
}

// Welcome is the first message on every connection, and the only one carrying
// seq 0.
//
// It exists so the client never has to guess what it just resumed into. Head is
// the player's current last sequence number, which a freshly-connected client
// adopts as its cursor; Gap reports that the messages it asked for are older
// than the outbox still holds, and that it must rehydrate over REST rather than
// trust the patches that follow.
type Welcome struct {
	Session string `json:"session"`
	Since   int64  `json:"since"`
	Head    int64  `json:"head"`
	Gap     bool   `json:"gap"`
}

var welcome = Define[Welcome]("welcome")

// envelope encodes a payload under this kind, for a message written straight to
// a socket rather than through the outbox. Only the welcome qualifies.
func (k Kind[P]) envelope(seq int64, payload P) (Envelope, error) {
	body, err := wire.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("realtime: encode %s: %w", k.name, err)
	}
	return Envelope{Type: k.name, Seq: seq, Payload: body}, nil
}
