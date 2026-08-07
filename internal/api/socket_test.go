package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
)

// The socket seen through the transport it ships behind: a real HTTP server, a
// real WebSocket, a real Postgres carrying the fan-out, and a real session
// cookie on the handshake.
//
// What is worth proving here is not that a connected client receives a message
// — that is the easy half — but that a *disconnected* one does. §8 names
// reconnect-resume as the most commonly forgotten piece of this design, so the
// test that kills a connection mid-flight is the point of this file.

// patience bounds every read. Generous, because it covers a commit, a Postgres
// notification, and a socket write; still finite, because a message that never
// arrives has to fail rather than hang.
const patience = 10 * time.Second

// online is a signed-in player, a server on a real port, and the database
// underneath, which the tests read and write directly.
type online struct {
	server  *Server
	http    *httptest.Server
	store   *store.Store
	session *http.Cookie
}

func newOnline(t *testing.T) *online {
	t.Helper()
	s, st, as := signedIn(t)

	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)

	return &online{server: s, http: srv, store: st, session: as}
}

// instance is a second server over the same database: another process in front
// of one Postgres, which is the deployment §3.2 describes and the one a rolling
// deploy produces. It has its own hub, its own listener, and no knowledge of
// the first one's sockets.
//
// The session cookie carries over untouched, because a session is a row rather
// than anything held in the process that issued it.
func (o *online) instance(t *testing.T) *online {
	t.Helper()

	s := build(t, o.store, "http://localhost:8080")
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)

	return &online{server: s, http: srv, store: o.store, session: o.session}
}

// tab opens a socket as one of the player's tabs. since is the sequence number
// it has already processed; a negative one means "I have just loaded, send me
// what happens from now on".
func (o *online) tab(t *testing.T, session string, since int64) *websocket.Conn {
	t.Helper()

	url := "ws" + strings.TrimPrefix(o.http.URL, "http") + socketPath + "?session=" + session
	if since >= 0 {
		url += "&since=" + strconv.FormatInt(since, 10)
	}

	ctx, cancel := context.WithTimeout(t.Context(), patience)
	defer cancel()

	conn, res, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader: http.Header{"Cookie": {o.session.Name + "=" + o.session.Value}},
	})
	if err != nil {
		status := 0
		if res != nil {
			status = res.StatusCode
		}
		t.Fatalf("dial the socket (%d): %v", status, err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	return conn
}

// probe sends the walking-skeleton command as a named tab, which is what
// decides whose socket treats the message as its own echo.
func (o *online) probe(t *testing.T, session, label string) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/commands/probe",
		strings.NewReader(fmt.Sprintf(`{"label":%q}`, label)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "probe-"+label)
	req.Header.Set(sessionHeader, session)
	req.AddCookie(o.session)

	rec := httptest.NewRecorder()
	o.server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("probe %q = %d (%s), want 200", label, rec.Code, rec.Body)
	}
}

// next reads one envelope, failing rather than blocking forever.
func next(t *testing.T, conn *websocket.Conn) realtime.Envelope {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), patience)
	defer cancel()

	_, body, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read from the socket: %v", err)
	}
	var env realtime.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("the socket sent something that is not an envelope: %s", body)
	}
	return env
}

// hello reads the welcome every connection opens with, which is also where the
// server says what it did with the resume point it was given.
func hello(t *testing.T, conn *websocket.Conn) realtime.Welcome {
	t.Helper()

	env := next(t, conn)
	if env.Type != "welcome" || env.Seq != 0 {
		t.Fatalf("first message = %s/%d, want the welcome at seq 0", env.Type, env.Seq)
	}
	var w realtime.Welcome
	if err := json.Unmarshal(env.Payload, &w); err != nil {
		t.Fatalf("decode the welcome: %v", err)
	}
	return w
}

// label pulls the probe label back out of a message, which is what identifies
// which probe a given envelope is about.
func label(t *testing.T, env realtime.Envelope) string {
	t.Helper()

	if env.Type != "probe.recorded" {
		t.Fatalf("message type = %q, want probe.recorded", env.Type)
	}
	var p struct {
		Label string `json:"label"`
	}
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		t.Fatalf("decode the message: %v", err)
	}
	return p.Label
}

// The socket's reason to exist: a thing the player did somewhere else arrives
// without being asked for — and does not arrive back at the tab that did it,
// which already has the command's response (§8, rules 3 and 4).
func TestASecondTabIsToldWhatTheFirstOneDid(t *testing.T) {
	t.Parallel()
	o := newOnline(t)

	first := o.tab(t, "tab-1", -1)
	second := o.tab(t, "tab-2", -1)
	hello(t, first)
	hello(t, second)

	o.probe(t, "tab-1", "from-the-first-tab")

	// The other tab hears about it.
	if got := label(t, next(t, second)); got != "from-the-first-tab" {
		t.Errorf("the second tab was told about %q, want the first tab's probe", got)
	}

	// The tab that acted does not. Proved by giving it something it *should*
	// hear: if its own echo had been sent, this read would return that instead.
	o.probe(t, "tab-2", "from-the-second-tab")
	got := next(t, first)
	if l := label(t, got); l != "from-the-second-tab" {
		t.Errorf("the acting tab was sent %q — it should skip its own echo", l)
	}
	if got.Seq != 2 {
		t.Errorf("seq = %d, want 2: the skipped message still takes a number", got.Seq)
	}
}

// The acceptance criterion, and the piece §8 says is most commonly forgotten:
// kill the connection while messages are being published, come back, and lose
// nothing.
//
// The messages here are published *while there is no socket at all*, which is
// the strong version of the case — a laptop asleep through an event release.
// They are not held in a process that could be redeployed out from under them;
// they are rows, and the reconnect names the sequence number to continue from.
func TestKillingTheConnectionMidFlightLosesNothing(t *testing.T) {
	t.Parallel()
	o := newOnline(t)

	conn := o.tab(t, "tab-1", -1)
	hello(t, conn)

	o.probe(t, "elsewhere", "before-the-drop")
	if got := label(t, next(t, conn)); got != "before-the-drop" {
		t.Fatalf("received %q before the drop, want before-the-drop", got)
	}
	processed := int64(1)

	// The socket dies without a close frame — the laptop lid, not a sign-out.
	conn.CloseNow()

	// Everything below happens with nobody listening.
	missed := []string{"during-1", "during-2", "during-3", "during-4"}
	for _, l := range missed {
		o.probe(t, "elsewhere", l)
	}

	// The same tab comes back, saying where it got to.
	back := o.tab(t, "tab-1", processed)
	w := hello(t, back)
	if w.Gap {
		t.Fatalf("the server reported a gap resuming from %d, with everything still retained", processed)
	}
	if w.Head != int64(1+len(missed)) {
		t.Errorf("head = %d, want %d", w.Head, 1+len(missed))
	}

	for i, want := range missed {
		env := next(t, back)
		if got := label(t, env); got != want {
			t.Fatalf("replayed message %d is %q, want %q", i+1, got, want)
		}
		if env.Seq != processed+int64(i)+1 {
			t.Errorf("replayed %q at seq %d, want %d", want, env.Seq, processed+int64(i)+1)
		}
	}

	// And the stream carries on from there rather than starting again.
	o.probe(t, "elsewhere", "after-the-resume")
	if env := next(t, back); label(t, env) != "after-the-resume" || env.Seq != 6 {
		t.Errorf("after the replay: %s at seq %d, want after-the-resume at 6", label(t, env), env.Seq)
	}
}

// Fan-out crosses instances (§8, rule 5). A player whose socket is held by one
// server hears about a command another server handled, because the message went
// into Postgres and the notification came back out of it — no second datastore,
// and no requirement that a player's socket and their commands land on the same
// process.
func TestAMessagePublishedOnOneInstanceReachesASocketOnAnother(t *testing.T) {
	t.Parallel()
	first := newOnline(t)
	second := first.instance(t)

	conn := second.tab(t, "tab-1", -1)
	hello(t, conn)

	first.probe(t, "elsewhere", "handled-by-the-other-instance")

	if got := label(t, next(t, conn)); got != "handled-by-the-other-instance" {
		t.Errorf("received %q, want the probe the other instance handled", got)
	}
}

// Sequence numbers are per player. A global one would make every resume a scan
// for relevance, and would leak how busy the rest of the game is.
func TestSequenceNumbersAreThePlayersOwn(t *testing.T) {
	t.Parallel()
	o := newOnline(t)

	// A second player, on the same server and the same database.
	other := &online{server: o.server, http: o.http, store: o.store,
		session: signIn(t, o.server, "xyz")}

	mine := o.tab(t, "tab-1", -1)
	theirs := other.tab(t, "tab-1", -1)
	hello(t, mine)
	hello(t, theirs)

	other.probe(t, "elsewhere", "theirs")
	o.probe(t, "elsewhere", "mine")

	if env := next(t, theirs); env.Seq != 1 {
		t.Errorf("the other player's first message is seq %d, want 1", env.Seq)
	}
	if env := next(t, mine); env.Seq != 1 {
		t.Errorf("this player's first message is seq %d, want 1 — sequences are per player", env.Seq)
	}
}

// A connection that is not resuming has just loaded its state over REST, so
// replaying the outbox onto it would apply the same things twice.
func TestAFreshConnectionIsNotSentTheBacklog(t *testing.T) {
	t.Parallel()
	o := newOnline(t)

	o.probe(t, "elsewhere", "long-before-this-tab-existed")

	conn := o.tab(t, "tab-1", -1)
	if w := hello(t, conn); w.Head != 1 || w.Gap {
		t.Fatalf("welcome = %+v, want head 1 and no gap", w)
	}

	// The first thing it hears is the first thing that happens.
	o.probe(t, "elsewhere", "after-connecting")
	if got := label(t, next(t, conn)); got != "after-connecting" {
		t.Errorf("the first message was %q — a fresh connection was replayed the backlog", got)
	}
}

// The outbox is short-lived on purpose, so resuming from further back than it
// still holds has to be answered honestly. Replaying the surviving tail would
// apply patches on top of a state that is missing the ones already swept; the
// client is told to rehydrate instead, and put at the head of the stream.
func TestResumingFromASweptPointIsReportedAsAGap(t *testing.T) {
	t.Parallel()
	o := newOnline(t)

	o.probe(t, "elsewhere", "swept-1")
	o.probe(t, "elsewhere", "swept-2")

	// What the sweep does when these rows age past realtime.Retention.
	if _, err := o.store.Pool().Exec(t.Context(), `DELETE FROM realtime_outbox`); err != nil {
		t.Fatalf("sweep the outbox: %v", err)
	}

	conn := o.tab(t, "tab-1", 0)
	w := hello(t, conn)
	if !w.Gap {
		t.Error("resuming from 0 with an empty outbox reported no gap")
	}
	if w.Head != 2 {
		t.Errorf("head = %d, want 2 — the sequence survives the sweep", w.Head)
	}

	// And it is at the head, not the beginning: what comes next is what
	// happens next.
	o.probe(t, "elsewhere", "after-the-gap")
	if env := next(t, conn); label(t, env) != "after-the-gap" || env.Seq != 3 {
		t.Errorf("after a gap: %s at seq %d, want after-the-gap at 3", label(t, env), env.Seq)
	}
}

// The socket is behind the same door as the command route (§8): no session, no
// connection. There is no anonymous stream to fall through to.
func TestTheSocketIsRefusedWithoutASession(t *testing.T) {
	t.Parallel()
	o := newOnline(t)

	ctx, cancel := context.WithTimeout(t.Context(), patience)
	defer cancel()

	url := "ws" + strings.TrimPrefix(o.http.URL, "http") + socketPath + "?session=tab-1"
	conn, res, err := websocket.Dial(ctx, url, nil)
	if err == nil {
		conn.CloseNow()
		t.Fatal("a socket opened with no session cookie")
	}
	if res == nil || res.StatusCode != http.StatusUnauthorized {
		t.Errorf("handshake = %v, want 401", res)
	}
}

// The session id is what echo suppression is keyed on, so a connection without
// a usable one is refused rather than served a stream that cannot tell its own
// actions from anyone else's.
func TestTheSocketRequiresAUsableSessionID(t *testing.T) {
	t.Parallel()
	o := newOnline(t)

	for _, id := range []string{"", "with a space", strings.Repeat("x", maxSessionIDLen+1)} {
		ctx, cancel := context.WithTimeout(t.Context(), patience)
		url := "ws" + strings.TrimPrefix(o.http.URL, "http") + socketPath + "?session=" + id
		conn, res, err := websocket.Dial(ctx, url, &websocket.DialOptions{
			HTTPHeader: http.Header{"Cookie": {o.session.Name + "=" + o.session.Value}},
		})
		cancel()

		if err == nil {
			conn.CloseNow()
			t.Errorf("session=%q opened a socket", id)
			continue
		}
		if res == nil || res.StatusCode != http.StatusBadRequest {
			t.Errorf("session=%q: handshake = %v, want 400", id, res)
		}
	}
}

// A command may carry a session id, and if it does it has to be one that could
// name a socket — it is stored on every message the tab causes.
func TestACommandWithAnUnusableSessionIDIsRefused(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	req := httptest.NewRequest(http.MethodPost, "/api/commands/probe", strings.NewReader(`{}`))
	req.Header.Set("Idempotency-Key", "key-1")
	req.Header.Set(sessionHeader, "not a session id")
	req.AddCookie(as)

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d (%s), want 400", rec.Code, rec.Body)
	}
}
