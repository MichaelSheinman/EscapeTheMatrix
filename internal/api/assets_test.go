package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func testAssets() http.Handler {
	return Assets(fstest.MapFS{
		"index.html":              {Data: []byte("<!doctype html>shell")},
		"assets/index-abc123.js":  {Data: []byte("console.log(1)")},
		"assets/index-abc123.css": {Data: []byte(".a{}")},
		"locations/street/street.tmj": {
			Data: []byte(`{"type":"map"}`),
		},
	})
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestServesAppShellAtRoot(t *testing.T) {
	rec := get(t, testAssets(), "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "<!doctype html>shell" {
		t.Errorf("body = %q, want the app shell", got)
	}
}

// index.html names the fingerprinted bundles, so a cached copy after a deploy
// points at files that no longer exist.
func TestAppShellIsNeverCached(t *testing.T) {
	rec := get(t, testAssets(), "/")

	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}
}

func TestFingerprintedAssetsAreCachedForever(t *testing.T) {
	rec := get(t, testAssets(), "/assets/index-abc123.js")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got == "" || got == "no-cache" {
		t.Errorf("Cache-Control = %q, want an immutable long max-age", got)
	}
}

// A location is authored in Tiled and exported as JSON the renderer loads
// directly (§10.3). Nothing in Go's MIME table knows .tmj, so without the
// registration in assets.go the map is served as sniffed text.
func TestTiledMapsAreServedAsJSON(t *testing.T) {
	rec := get(t, testAssets(), "/locations/street/street.tmj")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestUnknownPathServesShellForClientRouting(t *testing.T) {
	rec := get(t, testAssets(), "/playbook/rules")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "<!doctype html>shell" {
		t.Error("deep link did not fall back to the app shell")
	}
}

// Without the guard the SPA fallback answers a mistyped command with 200 and a
// page of HTML, which the client then tries to decode as JSON.
func TestUnknownAPIPathIs404JSON(t *testing.T) {
	rec := get(t, testAssets(), "/api/commands/nope")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}
}
