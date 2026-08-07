package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The read side, over the router that serves it: what a client's store is
// hydrated from at boot, and who is allowed to ask (technical.md §10.2).

// read sends a GET as the player holding `as`. A nil cookie sends no session.
func read(t *testing.T, s *Server, as *http.Cookie, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if as != nil {
		req.AddCookie(as)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestTheWorldSnapshotReportsWhatTheDatabaseHolds(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	// Empty is an answer, not an absence: a client that has just loaded needs
	// a number to start from even when nothing has happened yet.
	var out struct {
		Probes   int64  `json:"probes"`
		Database string `json:"database"`
		Schema   int64  `json:"schema"`
	}
	rec := read(t, s, as, worldPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not a world snapshot: %v", err)
	}
	if out.Probes != 0 {
		t.Errorf("probes = %d on an untouched database, want 0", out.Probes)
	}
	// A client that has never sent a command still knows what it is talking to,
	// which is what keeps those two out of a held copy of a probe's response.
	if out.Database == "" || out.Schema != schemaVersion {
		t.Errorf("snapshot = %+v, want a named database at schema %d", out, schemaVersion)
	}

	post(t, s, as, "probe", "key-1", `{"label":"first"}`)
	post(t, s, as, "probe", "key-2", `{"label":"second"}`)

	rec = read(t, s, as, worldPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not a world snapshot: %v", err)
	}
	if out.Probes != 2 {
		t.Errorf("probes = %d after two commands, want 2", out.Probes)
	}
}

// The snapshot is per-player state and goes through the same door as everything
// else (§8, §9) — there is no anonymous view of the world to fall back to.
func TestTheWorldSnapshotIsRefusedWithoutASession(t *testing.T) {
	t.Parallel()
	s, _ := newServer(t)

	rec := read(t, s, nil, worldPath)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}
}

// A cache holding this would hand one player's world to the next.
func TestTheWorldSnapshotIsNeverCached(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	rec := read(t, s, as, worldPath)

	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}
