package api

import (
	"net/http"
	"strconv"

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

// handleSocket opens the push channel.
//
// It is a GET rather than a command because it is the transport, not an action:
// nothing here changes the world, and everything it sends was decided by
// something that already committed. Resume is a query parameter rather than a
// first message, which means a reconnecting client is caught up by the time the
// handshake finishes and there is no client→server protocol to get wrong.
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
	}, realtime.Options{Origins: s.socketOrigins()})
}

// socketOrigins says which origins may open a socket.
//
// Same-origin is always allowed and is all production needs: the browser sends
// Origin on the handshake and attaches the session cookie like any other
// request, so refusing a cross-origin one is what stops another site from
// opening a socket as the signed-in player. Development is the exception —
// `make dev` serves the client from Vite on another port and proxies /api here,
// so the handshake genuinely arrives from a different origin.
func (s *Server) socketOrigins() []string {
	if s.cfg.IsProduction() {
		return nil
	}
	return []string{"localhost:*", "127.0.0.1:*"}
}

// validSessionID accepts what may identify a tab: short, printable, no spaces —
// the same rule as an idempotency key, and for the same reasons.
//
// The value is the client's own choice and is trusted with nothing: it scopes
// echo suppression within one player's own sockets, so the worst a hostile one
// can do is stop the tab that sent it from hearing itself.
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
