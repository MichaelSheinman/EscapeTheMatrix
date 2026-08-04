package api

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
)

// maxLabelLen bounds the probe's one input. It is short on purpose: the point
// of the field is to have something a client can get wrong.
const maxLabelLen = 40

// probeInput is what the browser sends. An absent label is allowed.
type probeInput struct {
	Label string `json:"label"`
}

// probeResult is what the browser renders. Every field came out of Postgres —
// that is the entire point of the walking skeleton.
type probeResult struct {
	Probes   int64     `json:"probes"`
	NotedAt  time.Time `json:"notedAt"`
	Label    string    `json:"label"`
	Database string    `json:"database"`
	Schema   int64     `json:"schema"`
}

// registerCommands binds every command this build serves.
//
// It is the whole registry: one call per command, at boot, read-only
// afterwards. Domain modules (§3.1) add their own here as they land, which is
// why it takes the registry rather than building one.
func (s *Server) registerCommands(reg *command.Registry) {
	command.Register(reg, "probe", s.probe)
}

// probe writes a row, then reads the world back.
//
// It is the end-to-end path every later ticket builds on: a click in the
// browser, a command over REST, a single transaction against Postgres, and a
// value on screen that could not have been faked client-side. The command layer
// supplies the rest of the shape — the transaction is opened for it, a retry
// that duplicates it applies once, and both ways of refusing it are legible
// from the status code alone:
//
//   - a label longer than allowed is 422, and stays 422 however often it is
//     sent;
//   - a label some earlier probe already took is 409, because the request is
//     fine and it is the world that is in the way.
func (s *Server) probe(ctx context.Context, tx pgx.Tx, _ command.Actor, in probeInput) (probeResult, error) {
	out := probeResult{Schema: s.schemaVersion}

	label := strings.TrimSpace(in.Label)
	if len([]rune(label)) > maxLabelLen {
		return out, command.Invalid("a label may be at most %d characters", maxLabelLen)
	}
	out.Label = label

	// nil rather than "" so that the partial unique index ignores unlabelled
	// probes, which is what keeps the bare button clickable forever.
	var stored *string
	if label != "" {
		stored = &label

		var taken bool
		if err := tx.QueryRow(ctx,
			`SELECT exists(SELECT 1 FROM skeleton_probe WHERE label = $1)`, label,
		).Scan(&taken); err != nil {
			return out, err
		}
		if taken {
			return out, command.Conflict("the label %q has already been probed", label)
		}
	}

	if err := tx.QueryRow(ctx,
		`INSERT INTO skeleton_probe (label) VALUES ($1) RETURNING noted_at`, stored,
	).Scan(&out.NotedAt); err != nil {
		return out, err
	}
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM skeleton_probe`,
	).Scan(&out.Probes); err != nil {
		return out, err
	}
	if err := tx.QueryRow(ctx,
		`SELECT current_setting('server_version')`,
	).Scan(&out.Database); err != nil {
		return out, err
	}
	return out, nil
}
