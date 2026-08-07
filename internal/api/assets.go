package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"sync"
)

// Tiled writes its JSON map export as .tmj, an extension nothing in Go's table
// knows (§10.3). Registered once so a painted location is served as the JSON it
// is rather than sniffed as text — the client parses it either way, but a
// content type nobody stated is a thing a proxy or a cache gets to guess at.
//
// The plates beside those maps are .webp (§10.4: backgrounds ship compressed),
// which Go's built-in table does know, so there is nothing to register for them.
func init() { _ = mime.AddExtensionType(".tmj", "application/json") }

// Assets serves the frontend that was embedded into the binary (§1: one binary
// serves API, WebSocket, and frontend). Nothing is read from disk at runtime,
// so the binary is the whole deployable.
func Assets(assets fs.FS) http.Handler {
	files := http.FileServer(http.FS(assets))
	tags := &etags{assets: assets, known: map[string]string{}}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// An unmatched /api/ path is a bug in the client, not a route into the
		// app shell. Without this guard the SPA fallback below would answer a
		// mistyped command with a 200 and a page of HTML.
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such endpoint"})
			return
		}

		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || name == "." {
			serveIndex(w, r, assets)
			return
		}

		if _, err := fs.Stat(assets, name); err != nil {
			// Deep links belong to the client router, so an unknown path is
			// the app shell rather than a 404.
			serveIndex(w, r, assets)
			return
		}

		// Vite fingerprints everything under assets/, so those URLs can never
		// change meaning and are safe to cache forever.
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			files.ServeHTTP(w, r)
			return
		}

		// Everything else is served under a name that outlives its contents —
		// `plate.webp` is `plate.webp` in every build — so it cannot be cached
		// by URL. §10.4 makes the plates the heaviest thing this server hands
		// out, and re-sending one on every navigation is exactly the cost the
		// asset pipeline exists to avoid, so they are cached against their
		// *contents* instead: an ETag over the bytes, and `no-cache` meaning
		// "ask me, every time" rather than "do not keep this".
		//
		// http.ServeContent reads the ETag back out of the header we just set
		// and answers a matching If-None-Match with a 304 and no body.
		if tag := tags.of(name); tag != "" {
			w.Header().Set("ETag", tag)
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

// etags remembers a content hash per path.
//
// The embedded filesystem cannot change while the process is running — it is
// literally part of the binary (§1) — so a hash computed once is correct until
// the process is replaced, which is also exactly when the deploy that should
// invalidate it happens. Hashing a megapixel plate costs a few milliseconds,
// once, on the first request for it.
type etags struct {
	assets fs.FS
	mu     sync.RWMutex
	known  map[string]string
}

// of returns the strong ETag for a path, or "" if it could not be read.
func (e *etags) of(name string) string {
	e.mu.RLock()
	tag, ok := e.known[name]
	e.mu.RUnlock()
	if ok {
		return tag
	}

	body, err := fs.ReadFile(e.assets, name)
	if err != nil {
		// Not worth remembering a failure: the caller falls through to the file
		// server, which will produce the real error for this request.
		return ""
	}
	sum := sha256.Sum256(body)
	tag = fmt.Sprintf("%q", hex.EncodeToString(sum[:16]))

	e.mu.Lock()
	e.known[name] = tag
	e.mu.Unlock()
	return tag
}

// serveIndex writes the app shell.
//
// index.html is explicitly never cached: it names the fingerprinted bundles,
// and a cached copy after a deploy points at files that no longer exist.
func serveIndex(w http.ResponseWriter, r *http.Request, assets fs.FS) {
	f, err := assets.Open("index.html")
	if err != nil {
		http.Error(w, "app shell missing", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.Copy(w, f)
}
