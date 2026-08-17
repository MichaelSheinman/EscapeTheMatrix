package api

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
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

// probeRecorded is the same event, told to the player's other tabs. The tab that
// sent the command is not told: it already has the response, and responses cover
// your own actions while the socket covers the world's (§8).
//
// The payload is not probeResult. What a command returns to its caller and what
// the world is told about it are two decisions, and collapsing them publishes
// every field added for one audience to the other.
type probeRecorded struct {
	Probes  int64     `json:"probes"`
	Label   string    `json:"label"`
	NotedAt time.Time `json:"notedAt"`
}

// The name is what appears on the wire and what the generated TypeScript is
// named after; this Go type's own name never leaves the package.
var probeRecordedMessage = realtime.Define[probeRecorded]("probe.recorded")

// registerCommands binds every command this build serves: one call per command,
// at boot, read-only afterwards. It takes the registry rather than building one,
// so domain modules add their own here as they land (§3.1).
func (s *Server) registerCommands(reg *command.Registry) {
	command.Register(reg, "probe", s.probe)

	// character (§3.3): where the player is, and where they live. Two commands
	// because they are two decisions — the population follows the second (§3.15),
	// and one "go here" that also moved your address would make the ceiling on a
	// worn-out place avoidable by walking.
	command.Register(reg, "character.move", s.move)
	command.Register(reg, "character.relocate", s.relocate)

	// health (§3.16): the body, and what the player can see of it.
	command.Register(reg, "health.buy", s.buy)

	// routines (§3.4): the rules the player sets for their own body, and the
	// day those rules produce.
	command.Register(reg, "routine.set", s.setRoutines)

	// the playbook (§3.14): the rules the player sets for their own judgement.
	// Neither command is a save, because editing the sentence is the change. The
	// second takes an answer back, which says something different from writing a
	// sentence with nothing in it.
	command.Register(reg, "playbook.compose", s.composeRule)
	command.Register(reg, "playbook.clear", s.clearRule)

	// the event backlog (§4): the two things a player may do about what the world
	// put in front of them. Nothing here asks for an event, because release is a
	// server decision.
	command.Register(reg, "event.accept", s.acceptEvent)
	command.Register(reg, "event.decline", s.declineEvent)

	// challenges (§5): entering one that is standing in front of you, and
	// committing to one of its authored positions. Two, and there is no third —
	command.Register(reg, "challenge.begin", s.beginChallenge)
	command.Register(reg, "challenge.choose", s.chooseInChallenge)
	// Walking out. The run is deleted rather than kept, so the challenge is
	// back to never having happened and can be met again — nothing was paid on
	// the way in, so there is nothing to take back.
	command.Register(reg, "challenge.abandon", s.abandonChallenge)

	// jobs (§3.14): taking the one being taken on where you are standing, and
	// giving up the one you hold. Neither takes an argument — placement is the
	// server's, exactly as an event's is — and there is deliberately no third
	// that asks for a wage, because §3.4's income is structural and arrives
	// because a day ended rather than because somebody pressed something.
	command.Register(reg, "job.take", s.takeJob)
	command.Register(reg, "job.leave", s.leaveJob)

	// delegated work (§3.14): the one thing a player may do about a moment
	// their own rules could not settle. There is deliberately no second —
	// nothing here produces a situation, because §3.14's loop begins "it
	// happens without you" and a command that asked for work would be a job
	// with a button on it.
	command.Register(reg, "situation.answer", s.answerSituation)

	// daily conflicts (§3.15): the one thing a player may do about friction with
	// the people they live among that their own rules could not settle. One
	// command, for the same reason delegated work has one — nothing here
	// produces a conflict, because living somewhere is what produces it and a
	// button that asked for friction would be a neighbour with a doorbell.
	command.Register(reg, "conflict.answer", s.answerConflict)

	// peer endorsement (§3.5): the two things a player may do about an offer
	// the game made them. Note there is no third — nothing here creates an
	// offer, because "I like this user" is not a thing to ask for.
	command.Register(reg, "social.endorse", s.endorse)
	command.Register(reg, "social.pass", s.passOffer)

	// The developer's toolbar (dev.go). Registered only outside production, and
	// **registered rather than guarded** — the difference is the whole of the
	// safety here. A handler that checked whether it was allowed to run would be
	// a handler that runs, one deploy-time environment variable away from a
	// player who can hand themselves a house fire and somebody else's savings.
	// Absent, there is no route to reach and nothing to get wrong.
	//
	// It is last in this function for the same reason: everything above is the
	// game, and this is the four things somebody authoring the game needs that
	// the game itself must never be able to do.
	if !s.cfg.IsProduction() {
		command.Register(reg, "dev.challenges", s.devChallenges)
		command.Register(reg, "dev.enable", s.devEnable)
		command.Register(reg, "dev.disable", s.devDisable)
		command.Register(reg, "dev.reset", s.devReset)

		// And the conversation half (devconflict.go). Separate commands rather
		// than more arguments on the four above, because a challenge is a thing
		// you enter once and a conversation is a thing that accumulates — what
		// authoring one needs is a thread built up and torn down, not a gate
		// opened.
		command.Register(reg, "dev.conversations", s.devConversations)
		command.Register(reg, "dev.conflict.raise", s.devConflictRaise)
		command.Register(reg, "dev.conflict.day", s.devConflictDay)
		command.Register(reg, "dev.conflict.forget", s.devConflictForget)
	}
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
func (s *Server) probe(ctx context.Context, tx pgx.Tx, actor command.Actor, in probeInput) (probeResult, error) {
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

	// Published inside the same transaction as the row it describes, which is
	// the property that makes the socket trustworthy: a message exists exactly
	// when the thing it reports committed. A probe that rolls back — a taken
	// label, a broken connection — announces nothing, and no retry has to
	// un-announce it.
	//
	// The audience is the player who acted, because that is the only audience
	// this table has an answer for. Who else cares about a given event is a
	// question each domain module answers for itself (§3.1); the transport does
	// not guess.
	if err := probeRecordedMessage.Publish(ctx, tx,
		realtime.To{Account: actor.ID, Origin: actor.Session},
		probeRecorded{Probes: out.Probes, Label: out.Label, NotedAt: out.NotedAt},
	); err != nil {
		return out, err
	}
	return out, nil
}
