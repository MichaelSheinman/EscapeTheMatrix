package api

import (
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"time"
)

// The Bemuse music library, relayed. The dance studio's game is pointed at
// this route (?server=/bemuse-music/server) rather than at the library's own
// host, because that host denies its song images and the game's loading
// screen treats a failed image as a failed song. The index is rewritten so
// every song file downloads through here, and a denied image is answered
// with a plain dark tile.
const musicUpstream = "https://music4.bemuse.ninja"

const musicPrefix = "/bemuse-music"

// A 1x1 dark PNG, served in place of an image the upstream denies.
var musicTile, _ = base64.StdEncoding.DecodeString(
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGMQkNAAAACMAFFd57umAAAAAElFTkSuQmCC")

// Song packages run to a few megabytes each; the timeout covers a slow pull
// of the largest one, not a single round trip.
var musicClient = &http.Client{Timeout: 5 * time.Minute}

func (s *Server) bemuseMusic(w http.ResponseWriter, r *http.Request) {
	// The escaped form is what the upstream expects: song folders carry
	// brackets in their names, and the decoded path would send them bare.
	upstreamPath := strings.TrimPrefix(r.URL.EscapedPath(), musicPrefix)

	resp, err := musicClient.Get(musicUpstream + upstreamPath)
	if err != nil {
		http.Error(w, "music library unreachable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden && strings.HasSuffix(upstreamPath, ".png") {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(musicTile)
		return
	}

	// The index names each song's location on the upstream host; pointed back
	// at this route instead, so the game never talks to the upstream itself.
	if strings.HasSuffix(upstreamPath, "/index.json") {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, "music library unreachable", http.StatusBadGateway)
			return
		}
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		rewritten := strings.ReplaceAll(string(body),
			musicUpstream, scheme+"://"+r.Host+musicPrefix)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write([]byte(rewritten))
		return
	}

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
