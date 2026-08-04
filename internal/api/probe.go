package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

// probeResult is what the browser renders. Every field came out of Postgres —
// that is the entire point of the walking skeleton.
type probeResult struct {
	Probes   int64     `json:"probes"`
	NotedAt  time.Time `json:"notedAt"`
	Database string    `json:"database"`
	Schema   int64     `json:"schema"`
}

// handleProbe writes a row, then reads the world back, inside one transaction.
//
// It is the end-to-end path every later ticket builds on: a click in the
// browser, a command over REST, a single transaction against Postgres, and a
// value on screen that could not have been faked client-side.
func (s *Server) handleProbe(w http.ResponseWriter, r *http.Request) {
	out := probeResult{Schema: s.schemaVersion}

	err := s.store.InTx(r.Context(), func(tx pgx.Tx) error {
		if err := tx.QueryRow(r.Context(),
			`INSERT INTO skeleton_probe DEFAULT VALUES RETURNING noted_at`,
		).Scan(&out.NotedAt); err != nil {
			return err
		}
		if err := tx.QueryRow(r.Context(),
			`SELECT count(*) FROM skeleton_probe`,
		).Scan(&out.Probes); err != nil {
			return err
		}
		return tx.QueryRow(r.Context(),
			`SELECT current_setting('server_version')`,
		).Scan(&out.Database)
	})
	if err != nil {
		s.log.Error("probe command failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "the probe could not reach the database",
		})
		return
	}

	writeJSON(w, http.StatusOK, out)
}

// writeJSON is the one place a response body is encoded, so the content type
// and the error path are decided once.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	// The status line is already written, so an encoding failure can only be
	// logged by the caller's recovery — never turned into a different status.
	_ = json.NewEncoder(w).Encode(body)
}
