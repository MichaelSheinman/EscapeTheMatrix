package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
)

// maxCommandBody caps a request payload. Commands carry a handful of fields —
// an option chosen, a rule composed — so anything larger is a mistake or an
// attack, and a bounded read is the cheapest defence against both.
const maxCommandBody = 64 << 10

// replayHeader marks a response that was recorded by an earlier delivery of the
// same command. Nothing in the client depends on it; it exists so that a
// duplicate is visible in the network panel (technical.md §8).
const replayHeader = "Idempotent-Replay"

// handleCommand is the only route commands go through: POST
// /api/commands/{name}, dispatched by name to a registered handler.
//
// It is deliberately thin. Deciding a status code, opening a transaction, and
// making a retry apply once all belong to internal/command, where they happen
// identically for every command rather than once per handler. Deciding who is
// asking belongs to the middleware in front of it, for the same reason.
func (s *Server) handleCommand(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorFrom(r.Context())
	if !ok {
		// Unreachable: this route is registered behind authenticated(), which
		// is the only way to reach it and cannot pass a request without one.
		// Named rather than assumed, because the failure it would otherwise be
		// is a command running as nobody.
		s.log.Error("command route reached without an actor", "path", r.URL.Path)
		writeJSON(w, http.StatusInternalServerError, errorBody("the command could not be completed"))
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxCommandBody))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeJSON(w, http.StatusRequestEntityTooLarge, errorBody("the request body is too large"))
			return
		}
		writeJSON(w, http.StatusBadRequest, errorBody("the request body could not be read"))
		return
	}

	res := s.commands.Execute(r.Context(), command.Request{
		Name:  r.PathValue("name"),
		Key:   r.Header.Get("Idempotency-Key"),
		Actor: command.Actor{ID: actor.ID},
		Body:  body,
	})

	if res.Replayed {
		w.Header().Set(replayHeader, "true")
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(res.Status)
	_, _ = w.Write(res.Body)
}

func errorBody(message string) map[string]string { return map[string]string{"error": message} }

// writeJSON is the one place a response body is encoded, so the content type
// and the error path are decided once.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	// The status line is already written, so an encoding failure can only be
	// logged by the caller's recovery — never turned into a different status.
	_ = json.NewEncoder(w).Encode(body)
}
