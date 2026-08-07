package api

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/challenge"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/factor"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
The challenge framework's bindings (§5).

There is deliberately no transport in this file. §5 splits into a framework and
the challenges built on it, and §3.10 is explicit that the framework "should
exist first" — so what this ticket wires is the two seams the module cannot
reach for itself and the boot-time publish that makes its foreign key real. The
commands, the socket messages and the screen arrive with the first challenge
there is to play (#24), because a route that can only ever answer "there is no
such challenge" is not a surface, it is a placeholder.
*/

// contentTree is where the authored content lives in a working copy.
//
// Only the development hot reload reads it: the shipped binary embeds content
// (see package content) and never touches the filesystem for it.
const contentTree = "content"

/*
PublishContent upserts the validated challenge catalogue into its catalog table.

§5.2: "on boot the server loads, validates against the mechanic's config schema,
and upserts into a `challenge` catalog table so foreign keys work." Loading and
validating already happened — at init, in internal/challenge, which panics if
content is malformed — so what is left is the row per definition that
`challenge_run.challenge` points at.

It is a method on the server rather than a call in cmd/server because it needs
the store and the module, and because the hot reloader calls it again on every
successful reload. One transaction: a catalog that was half-published would be a
challenge whose runs cannot be created.
*/
func (s *Server) PublishContent(ctx context.Context) error {
	if err := s.store.InTx(ctx, func(tx pgx.Tx) error {
		return s.challenges.Publish(ctx, tx)
	}); err != nil {
		return fmt.Errorf("publish the challenge catalogue: %w", err)
	}
	return nil
}

/*
WatchContent reloads authored challenge content when a file changes (§5.2).

Development only, and the guard is here rather than in internal/challenge so
that the module keeps one behaviour and this package keeps the decision about
which environment the process is in. A production binary has nothing on disk to
watch — its content is embedded — so the call would be harmless anyway; refusing
outright is what makes that a decision rather than an accident.

Every successful reload re-publishes the catalog table, because a reload that
added a challenge and did not publish it would be a definition no run could
reference.
*/
func (s *Server) WatchContent(ctx context.Context) {
	if s.cfg.IsProduction() {
		return
	}
	challenge.Watch(ctx, contentTree, s.log, s.PublishContent)
}

/*
awards answers challenge.Awards: where a completed run's authored deltas go.

§5.1's hidden factor deltas, handed to internal/factor and no further. The
adapter drops the standing internal/factor hands back, which is the whole point
of it being an adapter: a challenge that could read what it moved is a challenge
somebody eventually shows a grade for, and §3.10 says the scoring is never
surfaced and never confirmed as right.

It is the same one-way shape peer endorsement's `grants` takes, for the same
reason and by the same argument.
*/
type awards struct {
	factors *factor.Factors
}

func (a awards) Award(ctx context.Context, tx pgx.Tx, accountID string, grants ...simulation.Grant) error {
	_, err := a.factors.Record(ctx, tx, accountID, grants...)
	return err
}
