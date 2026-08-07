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
		"locations/street/plate.webp": {
			Data: []byte("RIFF....WEBPVP8L"),
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

// §10.4: painted backgrounds are the main frontend performance constraint, and
// they ship compressed. Nothing has to be registered for .webp — Go's built-in
// table knows it — but the client refusing to draw a plate served as text is a
// silent failure worth an assertion.
func TestPlatesAreServedAsWebP(t *testing.T) {
	rec := get(t, testAssets(), "/locations/street/plate.webp")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/webp") {
		t.Errorf("Content-Type = %q, want image/webp", ct)
	}
}

// A plate is served under a name that outlives its contents, so it cannot be
// cached by URL the way a fingerprinted bundle is. It is cached against its
// contents instead — which is what stops the heaviest asset in the build from
// being re-sent on every navigation.
func TestUnfingerprintedAssetsRevalidateAgainstTheirContents(t *testing.T) {
	handler := testAssets()
	rec := get(t, handler, "/locations/street/plate.webp")

	tag := rec.Header().Get("ETag")
	if tag == "" {
		t.Fatal("no ETag on a plate, so a browser has nothing to revalidate with")
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}

	again := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/locations/street/plate.webp", nil)
	req.Header.Set("If-None-Match", tag)
	handler.ServeHTTP(again, req)

	if again.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", again.Code)
	}
	if again.Body.Len() != 0 {
		t.Errorf("a 304 carried %d bytes of body", again.Body.Len())
	}
}

// Two files with different contents must not share a tag, or a deploy would
// leave one of them stale in every browser that had seen the other.
func TestETagsFollowTheContents(t *testing.T) {
	handler := testAssets()
	plate := get(t, handler, "/locations/street/plate.webp").Header().Get("ETag")
	tmj := get(t, handler, "/locations/street/street.tmj").Header().Get("ETag")

	if plate == tmj {
		t.Errorf("two different files share the ETag %s", plate)
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
