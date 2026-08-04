package api

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// Assets serves the frontend that was embedded into the binary (§1: one binary
// serves API, WebSocket, and frontend). Nothing is read from disk at runtime,
// so the binary is the whole deployable.
func Assets(assets fs.FS) http.Handler {
	files := http.FileServer(http.FS(assets))

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
		}
		files.ServeHTTP(w, r)
	})
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
