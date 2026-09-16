package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
)

// socketPath is the one WebSocket. It sits under /api/ like everything else the
// server owns, behind the same auth middleware as the command route.
const socketPath = "/api/socket"

// sessionHeader carries the tab's session id on a command, so that the message
// the command publishes knows which socket not to echo to (§8, rule 4). Absent
// is allowed — a `curl` has no tab, and a command sent without one simply
// reaches every socket the player has open, including their own.
const sessionHeader = "Etm-Session"

// maxSessionIDLen bounds the client-chosen session id. It is stored on every
// message the tab causes, so an unbounded one is a way to make rows large.
const maxSessionIDLen = 64

// handleSocket opens the push channel. A GET rather than a command, and resume
// is a query parameter rather than a first message, so a reconnecting client is
// caught up by the time the handshake finishes.
func (s *Server) handleSocket(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorFrom(r.Context())
	if !ok {
		// Unreachable: registered behind authenticated(), as the command route
		// is. Named rather than assumed, because the failure it would otherwise
		// be is a socket serving somebody else's messages.
		s.log.Error("socket route reached without an actor", "path", r.URL.Path)
		writeJSON(w, http.StatusInternalServerError, errorBody("the connection could not be opened"))
		return
	}

	q := r.URL.Query()

	session := q.Get("session")
	if !validSessionID(session) {
		writeJSON(w, http.StatusBadRequest, errorBody("session must be 1–64 printable characters without spaces"))
		return
	}

	// Absent means "I have just loaded my state over REST, send me what happens
	// from now on". Present means "I have state as of this sequence number,
	// send me everything after it" — which is the whole of reconnect-resume.
	var since int64
	resume := q.Has("since")
	if resume {
		n, err := strconv.ParseInt(q.Get("since"), 10, 64)
		if err != nil || n < 0 {
			writeJSON(w, http.StatusBadRequest, errorBody("since must be a sequence number"))
			return
		}
		since = n
	}

	s.realtime.Serve(w, r, realtime.Client{
		Account: actor.ID,
		Session: session,
		Since:   since,
		Resume:  resume,
	}, realtime.Options{Origins: s.socketOrigins(), Here: s.hereNow})
}

/*
hereNow records that a socket's player is at the keyboard (§3.15).

The transport's presence hook, bound here because the hub holds no rules and
internal/account owns what an activity stamp means. It runs on connect and on
every ping, which is the only signal there is for somebody who is reading a
thread and making no requests at all — and reading a thread is exactly when
being seen as present matters most, because that is where the conflict would
otherwise arrive.

Its own transaction. This is not part of any command and there is nothing for it
to be consistent with; a failure is the caller's to log and swallow, which is
what [realtime.Hub] does with it.
*/
func (s *Server) hereNow(ctx context.Context, accountID string) error {
	return s.store.InTx(ctx, func(tx pgx.Tx) error {
		return s.accounts.Here(ctx, tx, accountID)
	})
}

// socketOrigins says which origins may open a socket. Same-origin is always
// allowed and is all production needs. Development is the exception: `make dev`
// serves the client from Vite on another port and proxies /api here.
func (s *Server) socketOrigins() []string {
	if s.cfg.IsProduction() {
		return nil
	}
	return []string{"localhost:*", "127.0.0.1:*"}
}

// validSessionID accepts what may identify a tab: short, printable, no spaces —
// the same rule as an idempotency key. The value is the client's own choice and
// scopes echo suppression within one player's own sockets.
func validSessionID(id string) bool {
	if id == "" || len(id) > maxSessionIDLen {
		return false
	}
	for _, c := range id {
		if c < '!' || c > '~' {
			return false
		}
	}
	return true
}
