package command

import (
	"errors"
	"fmt"
	"net/http"
)

// Fault is a command failure the client is allowed to be told about.
//
// technical.md §8 makes status codes the protocol, so the constructors below
// are the only way a handler chooses one. Anything else a handler returns — a
// pgx error, a nil map, a bug — is a 500 whose text the client never sees,
// because that text is where connection strings and SQL end up.
type Fault struct {
	Status  int
	Message string
}

func (f *Fault) Error() string { return f.Message }

// Invalid is 422: the request itself was wrong. A label too long, a field
// missing, a number out of range. Sending it again unchanged fails again.
func Invalid(format string, args ...any) error {
	return &Fault{Status: http.StatusUnprocessableEntity, Message: fmt.Sprintf(format, args...)}
}

// Conflict is 409: the request was well-formed but the world is not in a state
// that allows it. Not enough money, the challenge already finished, the day
// already rolled over. Sending it again after the world moves may succeed.
//
// The distinction from Invalid is the whole point — a client can retry a 409
// later and must never retry a 422.
func Conflict(format string, args ...any) error {
	return &Fault{Status: http.StatusConflict, Message: fmt.Sprintf(format, args...)}
}

// NotFound is 404: the thing addressed does not exist. Used by the executor for
// an unknown command name, and by handlers for an unknown id.
func NotFound(format string, args ...any) error {
	return &Fault{Status: http.StatusNotFound, Message: fmt.Sprintf(format, args...)}
}

// Malformed is 400: the envelope is wrong rather than the payload — a missing
// idempotency key, a body too large. It is distinct from Invalid so that "the
// client is not speaking the protocol" and "the player asked for something
// impossible" never arrive as the same code.
func Malformed(format string, args ...any) error {
	return &Fault{Status: http.StatusBadRequest, Message: fmt.Sprintf(format, args...)}
}

// unexpected is what a client is told about an error nobody classified.
const unexpected = "the command could not be completed"

// statusOf maps a handler error onto the wire. The second return is the message
// that is safe to send; the original error stays in the log.
func statusOf(err error) (int, string) {
	var f *Fault
	if errors.As(err, &f) {
		return f.Status, f.Message
	}
	return http.StatusInternalServerError, unexpected
}
