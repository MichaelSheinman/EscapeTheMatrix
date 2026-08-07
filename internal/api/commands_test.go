package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/config"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store/storetest"
)

// The command layer seen through the transport it actually ships behind:
// httptest against the real router, over a real Postgres (technical.md §14),
// asserting the response and the state it left.

// schemaVersion is what the migrations in this tree add up to. Asserted on
// rather than read back, so adding one without noticing fails a test —
// TestTheSchemaVersionIsWhatTheMigrationsAddUpTo is where that happens.
const schemaVersion = 13

// newServer builds the router over its own database, with a stand-in for Google
// wired in so a test can sign a player in without leaving the process.
func newServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	st := storetest.New(t)
	return build(t, st, "http://localhost:8080"), st
}

// serverOn is newServer with the public origin as a parameter, which is what
// decides whether cookies are marked Secure.
func serverOn(t *testing.T, publicURL string) *Server {
	t.Helper()
	return build(t, storetest.New(t), publicURL)
}

func build(t *testing.T, st *store.Store, publicURL string) *Server {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	// A real hub over the test's own database. Every test gets one because
	// commands publish through it: a probe that could not write to the outbox
	// is a probe that fails, so leaving it out would test a server that does
	// not exist.
	hub := realtime.NewHub(st, log)
	go hub.Run(t.Context())

	s := New(config.Config{
		Env:       config.Development,
		PublicURL: publicURL,
		Google:    config.OAuthClient{ID: "client-1", Secret: "secret-1"},
	}, st, schemaVersion, log, testAssets(), hub)

	provider := fakeGoogle(t)
	s.google.AuthEndpoint = provider + "/authorize"
	s.google.TokenEndpoint = provider + "/token"
	s.google.UserinfoEndpoint = provider + "/userinfo"
	return s
}

// signedIn is the ordinary starting point for a command test: a server, its
// database, and a player who is already through the door.
func signedIn(t *testing.T) (*Server, *store.Store, *http.Cookie) {
	t.Helper()
	s, st := newServer(t)
	return s, st, signIn(t, s, "abc")
}

// post sends a command as the player holding `as`. An empty key omits the
// header entirely; a nil cookie sends no session at all.
func post(t *testing.T, s *Server, as *http.Cookie, name, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/commands/"+name, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	if as != nil {
		req.AddCookie(as)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func probeCount(t *testing.T, st *store.Store) int {
	t.Helper()
	var n int
	if err := st.Pool().QueryRow(t.Context(), `SELECT count(*) FROM skeleton_probe`).Scan(&n); err != nil {
		t.Fatalf("count probes: %v", err)
	}
	return n
}

func TestProbeCommandWritesAndReadsBack(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)

	rec := post(t, s, as, "probe", "key-1", `{"label":"first"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var out probeResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not a probe result: %v", err)
	}
	if out.Probes != 1 || out.Label != "first" || out.Database == "" || out.Schema != schemaVersion {
		t.Errorf("result = %+v, want one probe labelled \"first\" from a named database at schema %d", out, schemaVersion)
	}
	if got := probeCount(t, st); got != 1 {
		t.Errorf("%d rows in skeleton_probe, want 1", got)
	}
}

// The ticket, over HTTP: a retried command applies once and answers identically.
func TestRetriedCommandAppliesOnce(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)

	first := post(t, s, as, "probe", "key-1", `{"label":"first"}`)
	second := post(t, s, as, "probe", "key-1", `{"label":"first"}`)

	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("statuses = %d, %d; want 200, 200", first.Code, second.Code)
	}
	if first.Body.String() != second.Body.String() {
		t.Errorf("retry answered %s, want the original %s", second.Body, first.Body)
	}
	if got := probeCount(t, st); got != 1 {
		t.Errorf("%d rows in skeleton_probe after the same command twice, want 1", got)
	}
	if first.Header().Get(replayHeader) != "" {
		t.Error("the first delivery was marked as a replay")
	}
	if second.Header().Get(replayHeader) != "true" {
		t.Error("the replay is invisible in the response headers")
	}
}

// Status codes are the protocol: every one of these refusals is told apart by
// code alone, with no need to read the message.
func TestRefusalsAreDistinguishedByStatus(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	post(t, s, as, "probe", "key-1", `{"label":"taken"}`)

	cases := []struct {
		name, key, body string
		want            int
	}{
		{"probe", "", `{}`, http.StatusBadRequest},                                                        // no idempotency key
		{"probe", "key-2", `{"label":"` + strings.Repeat("x", 41) + `"}`, http.StatusUnprocessableEntity}, // the request is wrong
		{"probe", "key-3", `{"label":"taken"}`, http.StatusConflict},                                      // the world is in the way
		{"probe", "key-4", `{"lable":"typo"}`, http.StatusUnprocessableEntity},                            // client/server drift
		{"probe", "key-5", `not json`, http.StatusUnprocessableEntity},                                    // not a payload at all
		{"no.such.command", "key-6", `{}`, http.StatusNotFound},                                           // nothing answers to that
	}
	for _, c := range cases {
		if rec := post(t, s, as, c.name, c.key, c.body); rec.Code != c.want {
			t.Errorf("%s %q → %d, want %d (%s)", c.name, c.body, rec.Code, c.want, rec.Body)
		}
	}
}

func TestOversizedBodyIsRefused(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)

	rec := post(t, s, as, "probe", "key-1", `{"label":"`+strings.Repeat("x", maxCommandBody)+`"}`)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", rec.Code)
	}
	if got := probeCount(t, st); got != 0 {
		t.Errorf("%d rows in skeleton_probe, want 0", got)
	}
}

// Every command answers in JSON, including the ones that failed before any
// handler ran — the client decodes one shape or none.
func TestEveryAnswerIsJSON(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	for _, rec := range []*httptest.ResponseRecorder{
		post(t, s, as, "probe", "key-1", `{}`),
		post(t, s, as, "probe", "", `{}`),
		post(t, s, as, "nope", "key-2", `{}`),
		post(t, s, nil, "probe", "key-3", `{}`),
	} {
		if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
			t.Errorf("Content-Type = %q for a %d, want JSON", ct, rec.Code)
		}
		if !json.Valid(rec.Body.Bytes()) {
			t.Errorf("body of a %d is not JSON: %s", rec.Code, rec.Body)
		}
	}
}
