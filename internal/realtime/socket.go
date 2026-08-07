package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

// The socket's timings.
//
// A push-only connection is indistinguishable from a wedged one until something
// is sent, and half of these connections will sit behind a proxy with an idle
// timeout shorter than the quiet stretches this game has. The ping is what
// keeps the connection alive through those and what turns "the client's network
// vanished" into an error here rather than a socket that is written to forever.
const (
	pingEvery   = 30 * time.Second
	pingTimeout = 10 * time.Second
	writeWait   = 10 * time.Second
)

// readLimit bounds what a client may send. It is small because the client sends
// nothing: resume is a query parameter on the handshake, so there is no
// client→server message to be large.
const readLimit = 1 << 10

// Client is who is on the other end of one socket.
type Client struct {
	// Account is the signed-in player, resolved from the session cookie by the
	// same middleware that guards the command route. Messages are addressed to
	// a player, never to a connection.
	Account string

	// Session is the tab. Two tabs of one player are two sessions (§8, rule 4),
	// and a message caused by a command from this session is not sent back to
	// it — the acting client applied the command's HTTP response directly
	// (rule 3), so an echo would apply it twice.
	//
	// It is chosen by the client rather than assigned here, because it has to
	// survive a reconnect: a server-assigned id would be new after every drop,
	// and the messages a tab caused before the drop would come back to it as
	// somebody else's.
	Session string

	// Since is the last sequence number the client processed, meaningful only
	// when Resume is set. A client that is not resuming has just loaded its
	// state over REST and wants the stream from now on; one that is resuming
	// has state as of Since and wants everything after it.
	Since  int64
	Resume bool
}

// Options are the accept-time decisions the transport layer makes.
type Options struct {
	// Origins are the extra origin patterns a handshake may come from, on top
	// of same-origin, which is always allowed. Empty in production: the browser
	// sends Origin on a WebSocket handshake, the cookie rides it like any other
	// request, and refusing a cross-origin one is what stops another site from
	// opening a socket as the signed-in player.
	Origins []string
}

// Serve upgrades the request and pushes to it until one end goes away.
//
// Everything after the upgrade is written to the socket rather than to the
// response, so failures here are logged and never returned: by then there is no
// status code left to send.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request, c Client, opts Options) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: opts.Origins})
	if err != nil {
		// Accept has already answered the request; a rejected handshake is
		// ordinary (a stale tab, a cross-origin attempt) and not an error.
		h.log.Debug("realtime handshake refused", "error", err)
		return
	}
	defer conn.CloseNow()

	conn.SetReadLimit(readLimit)
	// The client is not expected to say anything, but the read side still has
	// to run: it is what answers pings, notices a close frame, and gives us a
	// context that is cancelled the moment the connection dies.
	ctx := conn.CloseRead(r.Context())

	// Before the first read of the outbox, so that a message published in
	// between rings a doorbell this socket is already holding.
	sub := h.subscribe(c.Account)
	defer h.unsubscribe(sub)

	if err := h.push(ctx, conn, sub, c); err != nil {
		if isGone(err) {
			h.log.Debug("realtime socket closed", "account", c.Account, "session", c.Session)
			return
		}
		h.log.Error("realtime socket", "account", c.Account, "session", c.Session, "error", err)
		// Told rather than dropped, so a client can tell "the server hung up"
		// from "the network vanished" and back off accordingly.
		_ = conn.Close(websocket.StatusInternalError, "the connection could not be served")
		return
	}
	_ = conn.Close(websocket.StatusGoingAway, "the server is shutting down")
}

// push writes the welcome, drains whatever the client has not seen, and then
// waits — for a doorbell, for the ping timer, or for the end.
func (h *Hub) push(ctx context.Context, conn *websocket.Conn, sub *subscriber, c Client) error {
	cursor, hello, err := h.resume(ctx, c)
	if err != nil {
		return err
	}
	env, err := welcome.envelope(0, hello)
	if err != nil {
		return err
	}
	if err := write(ctx, conn, env); err != nil {
		return err
	}

	ping := time.NewTicker(pingEvery)
	defer ping.Stop()

	for {
		cursor, err = h.drain(ctx, conn, cursor, c)
		if err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-h.stopped:
			return nil
		case <-sub.wake:
		case <-ping.C:
			pctx, cancel := context.WithTimeout(ctx, pingTimeout)
			err := conn.Ping(pctx)
			cancel()
			if err != nil {
				return err
			}
		}
	}
}

// drain sends everything past the cursor, a batch at a time, and returns where
// it got to.
//
// The cursor advances over a message this socket skips as well as over one it
// sends. Skipping is this connection ignoring its own echo, not the message
// being undelivered — every other tab of the player still receives it — so
// re-reading it on the next wake would be pure repetition.
func (h *Hub) drain(ctx context.Context, conn *websocket.Conn, cursor int64, c Client) (int64, error) {
	for {
		msgs, err := h.after(ctx, c.Account, cursor)
		if err != nil {
			return cursor, err
		}
		for _, m := range msgs {
			cursor = m.seq
			if m.origin != "" && m.origin == c.Session {
				continue
			}
			env := Envelope{Type: m.kind, Seq: m.seq, Payload: m.payload}
			if err := write(ctx, conn, env); err != nil {
				return cursor, err
			}
		}
		if len(msgs) < batch {
			return cursor, nil
		}
	}
}

// write puts one envelope on the wire under a deadline, so that a client that
// has stopped reading blocks this connection rather than the process.
func write(ctx context.Context, conn *websocket.Conn, env Envelope) error {
	body, err := json.Marshal(env)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, writeWait)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, body)
}

// isGone reports the ordinary end of a socket: the tab closed, the laptop slept,
// the request context was cancelled. None of it is worth an error log — it is
// what every connection eventually does.
func isGone(err error) bool {
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, net.ErrClosed) ||
		websocket.CloseStatus(err) != -1
}
