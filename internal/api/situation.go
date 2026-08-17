package api

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/career"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/playbook"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/situation"
)

/*
Delegated work on the wire (§3.14), and the seams internal/situation cannot reach
itself.

Nothing here creates a situation. §3.14's loop begins "it happens without you",
so the only thing that generates work is a day of the rollover passing, and the
only thing on this surface is the one moment the player's rules failed to handle.

Nothing here is advice. The moment says what happened and the answers say what
your character would do — no field says which is right or how any of them scored
(§3.2).

The escalation is the moment itself. Two of §4's three conditions are met here:
nothing covered it, or two answers disagreed. Unlike the generic escalation in
rollover.go, this puts the situation in front of the player rather than an event
about it. The third condition, a rule that fired and went badly, has already been
carried out and reaches them the ordinary way.
*/

/*
answerView is one thing the character could do, in the content's own words.

key is the posture the answer amounts to — internal/playbook's closed set — and
the one place a stance crosses the wire. Not the leak the playbook screen avoids:
there a stance would let a client work out what the matcher will do with a rule
it can read (§10.2), while here the player is choosing an answer directly.

What it is worth is not here. The authored deltas (§3.5) are paid by
internal/situation and never read back out of it.
*/
type answerView struct {
	Key string `json:"key"`
	// Does is what your character would do, in the second person.
	Does string `json:"does"`
}

// situationView is one moment standing in front of the player, carrying
// everything to re-establish it: a live decision that cannot be found again
// after closing the tab was never live (§5.1).
type situationView struct {
	// ID is what an answer names. Meaningless to anybody else: every read of a
	// waiting moment is keyed by (id, account).
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Line string `json:"line"`
	// Role is the job that produced it, in the words somebody in it would use.
	// §3.14's point is that the role ran while the player was elsewhere, and a
	// moment with no job attached would read as something that happened to them
	// rather than as work.
	Role string `json:"role"`
	// Environment is where their rules were read from, by key (§3.15).
	Environment string `json:"environment"`
	// Why is the reason it reached them, in the log's own words: nothing
	// covered it, or these two answers disagreed. Never a suggestion.
	Why string `json:"why"`
	// On is the player's own day the role produced it on, and At is when it was
	// written down.
	On string `json:"on"`
	At string `json:"at"`

	Answers []answerView `json:"answers"`
}

// situations is the whole delegated-work surface for one player: the one moment
// waiting, or nothing.
type situationsView struct {
	Waiting *situationView `json:"waiting"`
}

func situationOf(w situation.Waiting) situationView {
	out := situationView{
		ID:          w.ID,
		Name:        w.Situation.Name,
		Line:        w.Situation.Line,
		Environment: w.Environment,
		Why:         w.Note,
		On:          w.On.At().Format("2006-01-02"),
		At:          stamp(w.At),
		Answers:     []answerView{},
	}
	if role, known := career.ByKey(w.Role); known {
		out.Role = role.Name
	}
	if out.Why == "" {
		// Only [playbook.Backfired] has no sentence, and it cannot wait — but a
		// moment with nothing said about why it is here would read as a bug.
		out.Why = "your playbook did not settle this one."
	}
	for _, a := range w.Situation.Answers() {
		out.Answers = append(out.Answers, answerView{Key: string(a.Stance), Does: a.Did})
	}
	return out
}

// situationSurface reads what is waiting for this player. A plain read: unlike
// the event backlog and the endorsement offer, looking cannot cause one — only a
// day of the rollover passing produces a situation.
func (s *Server) situationSurface(ctx context.Context, tx pgx.Tx, accountID string) (situationsView, error) {
	waiting, standing, err := s.situations.Waiting(ctx, tx, accountID)
	if err != nil {
		return situationsView{}, err
	}
	if !standing {
		return situationsView{}, nil
	}
	view := situationOf(waiting)
	return situationsView{Waiting: &view}, nil
}

/* ── answering one by hand ────────────────────────────────────────────── */

// situationAnswered is what a player's other tabs are told when a moment is
// settled. came is the content's own words — the same sentence the log reads
// back, because there is one account of how it went.
type situationAnswered struct {
	Situations situationsView `json:"situations"`
	Name       string         `json:"name"`
	Did        string         `json:"did"`
	Came       string         `json:"came"`
}

var situationAnsweredMessage = realtime.Define[situationAnswered]("situation.answered")

// answerSituationInput is one posture, at the moment it was offered for. The
// moment is named by the client, so a stale tab answering something already
// settled is refused rather than applied to whatever is waiting now.
type answerSituationInput struct {
	Situation int64  `json:"situation"`
	Answer    string `json:"answer"`
}

type situationResult struct {
	Situations situationsView `json:"situations"`
	// Came is what it came to: the authored sentence rather than a verdict.
	// Nothing here says good or poor — the counts in the log are where a pattern
	// becomes visible (§3.2).
	Came string `json:"came"`
	Did  string `json:"did"`
}

/*
answerSituation is §3.14's step 4 by hand: the rules did not cover it, so the
player does. Not "compose a rule and let it run" — the moment already happened,
and writing the answer that covers the next one is a different command on a
different screen. That gap is the loop.

Two refusals, both 409 rather than 422: a moment another tab already settled and
one whose content went out in a deploy were both true a moment ago.
*/
func (s *Server) answerSituation(
	ctx context.Context, tx pgx.Tx, actor command.Actor, in answerSituationInput,
) (situationResult, error) {
	if in.Situation <= 0 {
		return situationResult{}, command.Invalid("an answer names the situation it answers")
	}
	stance := playbook.Stance(strings.TrimSpace(in.Answer))
	if stance == "" {
		return situationResult{}, command.Invalid("an answer names one of the things you could do")
	}

	// Read before answering, so the message below can say which moment was
	// settled. After it there is no waiting moment to read the name off.
	name := ""
	if before, standing, err := s.situations.Waiting(ctx, tx, actor.ID); err != nil {
		return situationResult{}, err
	} else if standing && before.ID == in.Situation {
		name = before.Situation.Name
	}

	answer, err := s.situations.Answer(ctx, tx, actor.ID, in.Situation, stance)
	switch {
	case errors.Is(err, situation.ErrNoSuchSituation):
		return situationResult{}, command.Conflict("that is no longer in front of you")
	case errors.Is(err, situation.ErrWithdrawn):
		return situationResult{}, command.Conflict("that is no longer something the world has in it")
	case errors.Is(err, situation.ErrNoSuchAnswer):
		return situationResult{}, command.Invalid("that is not one of the things you can do about it")
	case err != nil:
		return situationResult{}, err
	}

	// The whole surface, restated: a response carrying only what was settled
	// would leave a client holding a moment that no longer exists, and a role
	// that was waiting starts producing work again the moment it stops.
	out := situationResult{Came: answer.Came, Did: answer.Did}
	if out.Situations, err = s.situationSurface(ctx, tx, actor.ID); err != nil {
		return situationResult{}, err
	}

	// A settled moment moves hidden factors (§3.5), so Karma moved with it.
	if err := s.stood(ctx, tx, actor.ID); err != nil {
		return situationResult{}, err
	}

	return out, situationAnsweredMessage.Publish(ctx, tx,
		realtime.To{Account: actor.ID, Origin: actor.Session},
		situationAnswered{
			Situations: out.Situations,
			Name:       name,
			Did:        answer.Did,
			Came:       answer.Came,
		},
	)
}

/* ── the bindings ─────────────────────────────────────────────────────── */

// Delegated answers rollover.World's job half: one day of a role that runs
// itself. internal/rollover names the seam and internal/situation fills it.
func (w work) Delegated(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date) error {
	_, err := w.situations.Day(ctx, tx, accountID, on)
	return err
}

// holds answers situation.Holding: which job the player is in. An adapter rather
// than the career module, which also owns a wage, a ledger and an interview,
// none of which is a job situation's business.
type holds struct {
	careers *career.Careers
}

func (h holds) Holds(ctx context.Context, tx pgx.Tx, accountID string) (string, error) {
	held, err := h.careers.Of(ctx, tx, accountID)
	if err != nil {
		return "", err
	}
	return held.Key, nil
}

// Compile-time proof that the seams above are the interfaces they are for.
var (
	_ situation.Holding = holds{}
	// The playbook itself answers situation.Judgement — four of its own methods,
	// named as an interface so a job cannot reach the other eleven.
	_ situation.Judgement = (*playbook.Playbooks)(nil)
	// And the same two bindings internal/factor takes: where the player is, and
	// where a hidden delta goes.
	_ situation.Where  = population{}
	_ situation.Awards = awards{}
)
