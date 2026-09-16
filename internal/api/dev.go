package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/challenge"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/event"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
The developer's toolbar, challenge half.

Four commands so somebody authoring a challenge can put themselves in it. They
are not registered at all when the process is in production — see
[Server.registerCommands] — rather than guarded inside the handler or gated on a
role. A command that checks whether it is allowed to run is a command that runs.

devconflict.go is the other half, under the same rule: authoring a conversation
needs a thread assembled and torn down, so it has its own four commands.

Everything here breaks a rule the rest of the codebase is arranged around.
dev.reset deletes a run, against §5.1's once-ever, and erases §3.9's record of
what the world offered; dev.enable puts a specific event in front of a specific
player, against §4's release being a server decision.

Those are rules about the game, and authoring is not the game: a six-minute
challenge takes forty passes to write and every one starts from nothing. What
matters is that the breaking is four named commands in one file absent from a
production binary, rather than flags the modules carry forever.

It cannot invent content, edit a definition, or move a factor. Everything it
touches is a decision the world would otherwise have made — which challenge is
open to you, whether the fire is on your landing — and never what those things
then do. A challenge played through the toolbar pays what it would have paid, and
a reset does not un-pay it (see [challenge.Challenges.Forget]).
*/

// devChallengeView is one authored challenge as somebody building it needs to
// see it. A player is never sent the catalogue; this sends the whole of it
// including what has been played and what is gated.
type devChallengeView struct {
	Challenge string `json:"challenge"`
	Name      string `json:"name"`
	Mechanic  string `json:"mechanic"`
	// Gate is `open` or `flag`. A flagged challenge is one this toolbar can
	// actually turn on; an open one reaches everybody already.
	Gate string `json:"gate"`
	// Opens is the event that is this challenge's doorway, or empty for one that
	// stands somewhere in the world.
	Opens string `json:"opens"`
	// Places is where it stands of its own accord, by key. Empty for a challenge
	// that only ever arrives through its event.
	Places []string `json:"places"`

	// On is whether the gate is open for this player. Always true for `open`.
	On bool `json:"on"`
	// Played and Running are the once-ever rule, seen from the outside: a
	// challenge that has been finished cannot be entered again without a reset.
	Played  bool `json:"played"`
	Running bool `json:"running"`
}

// devResult is what every one of these commands answers with: the whole
// catalogue restated, plus the two player surfaces the toolbar just moved. The
// whole of it every time.
type devResult struct {
	Challenges []devChallengeView `json:"challenges"`
	// Challenge and Events are the player's own surfaces, so the client can drop
	// them straight into the store and the fire appears without a reload.
	Challenge challengeView `json:"challenge"`
	Events    eventsView    `json:"events"`
	// Notice is one plain sentence about what actually happened, for the
	// developer reading it. It is not player-facing copy and does not have to be
	// in the world's voice.
	Notice string `json:"notice"`
}

// devChallengeInput names one challenge. Shared by the three commands that take
// one, because they take exactly the same argument.
type devChallengeInput struct {
	Challenge string `json:"challenge"`
}

/*
devSurface composes the answer: the catalogue, and where the player now stands.

Read after every change rather than predicted from it, so the toolbar shows what
the server thinks rather than what the command believed it was about to do.
*/
func (s *Server) devSurface(ctx context.Context, tx pgx.Tx, accountID, notice string) (devResult, error) {
	c, err := s.characters.Ensure(ctx, tx, accountID)
	if err != nil {
		return devResult{}, err
	}
	place := where(c)

	out := devResult{Notice: notice}
	for _, d := range challenge.All() {
		view := devChallengeView{
			Challenge: string(d.Code),
			Name:      d.Name,
			Mechanic:  string(d.Mechanic),
			Gate:      string(d.Gate),
			Opens:     d.Opens,
			Places:    d.Where.Places(),
		}
		if view.On, err = s.challenges.Ungated(ctx, tx, accountID, d); err != nil {
			return devResult{}, err
		}
		if view.Played, err = s.challenges.Played(ctx, tx, accountID, d.Code); err != nil {
			return devResult{}, err
		}
		out.Challenges = append(out.Challenges, view)
	}

	held, running, err := s.challenges.Of(ctx, tx, accountID)
	if err != nil {
		return devResult{}, err
	}
	if running {
		for i := range out.Challenges {
			if out.Challenges[i].Challenge == string(held.Challenge.Code) {
				out.Challenges[i].Running = true
			}
		}
	}

	if out.Challenge, err = s.challengeSurface(
		ctx, tx, accountID, place, simulation.Today(s.clk, c.Zone),
	); err != nil {
		return devResult{}, err
	}
	// Read through Looked rather than a bare read, because that is the only
	// exported way to see the surface — and it is the honest one: it lapses an
	// offer whose window has passed, which is exactly what a toolbar wants to
	// show after somebody has been sitting on a five-minute fire.
	standing, _, err := s.events.Looked(ctx, tx, accountID, place)
	if err != nil {
		return devResult{}, err
	}
	if out.Events, err = s.eventSurface(ctx, tx, accountID, standing); err != nil {
		return devResult{}, err
	}
	return out, nil
}

// Listing everything there is to play, and what state this player is in for it.
func (s *Server) devChallenges(ctx context.Context, tx pgx.Tx, actor command.Actor, _ struct{}) (devResult, error) {
	return s.devSurface(ctx, tx, actor.ID, "")
}

/*
Turning a challenge on, and putting it in front of the player now.

Two steps, because a gated challenge and its doorway are two different things
(§4 keeps them distinct systems). Opening the gate says *this may happen to you*;
it does not make it happen. So:

 1. The gate opens, recorded as `dev` — which is the provenance a release policy
    will write its own name into later.
 2. If the challenge has an event behind it, that event is raised and the world
    is asked to release it where the player is standing. Raising is §4's
    escalation path and is exempt from the daily cap and the cooling, which is
    exactly the exemption a toolbar needs and exactly why it is the honest route
    rather than a bypass written for this file.

A challenge that stands in the world instead has nothing to raise: opening the
gate is the whole of it, and it appears when the player is standing in the right
place. The notice says which of the two happened, because otherwise the toolbar
looks broken for half the catalogue.
*/
func (s *Server) devEnable(ctx context.Context, tx pgx.Tx, actor command.Actor, in devChallengeInput) (devResult, error) {
	code, authored, err := s.devNamed(in)
	if err != nil {
		return devResult{}, err
	}

	/*
		A challenge already played is put back first.

		"Enable" means *make this playable now*, and for anything already
		finished that has to include forgetting the finished run — otherwise the
		toolbar cheerfully raises the doorway, the fire appears on the landing,
		and accepting it hits §5.1's once-ever, settles the event without a word
		and leaves the player standing in the street. Which is exactly what it
		did, and it looked like the click was broken rather than like the rule
		was working.

		The event's own record goes with it, because a doorway that has already
		been answered cannot be offered again — see [Events.Forget]. This is the
		one command in the toolbar that quietly does another one's work, and it
		earns it: nobody presses "enable" wanting to be told to press "reset"
		first.
	*/
	played, err := s.challenges.Played(ctx, tx, actor.ID, code)
	if err != nil {
		return devResult{}, err
	}
	if played {
		if _, err := s.challenges.Forget(ctx, tx, actor.ID, code); err != nil {
			return devResult{}, err
		}
		if authored.Opens != "" {
			if _, err := s.events.Forget(ctx, tx, actor.ID, authored.Opens); err != nil {
				return devResult{}, err
			}
		}
	}

	if authored.Gated() {
		if err := s.challenges.Enable(ctx, tx, actor.ID, code, "dev"); err != nil {
			return devResult{}, err
		}
	}

	notice := fmt.Sprintf("%s is open", code)
	if played {
		notice = fmt.Sprintf("%s was played and has been put back", code)
	}
	switch {
	case authored.Opens == "":
		if places := authored.Where.Places(); len(places) > 0 {
			notice += ". It stands in " + strings.Join(places, ", ") + " — walk there"
		}
	default:
		c, err := s.characters.Ensure(ctx, tx, actor.ID)
		if err != nil {
			return devResult{}, err
		}
		if _, err := s.events.Raise(ctx, tx, actor.ID, authored.Opens, event.Why{}); err != nil {
			return devResult{}, err
		}
		// Raising puts it in the backlog; it does not make the backlog willing
		// to hand it over. A previous offer that lapsed set this event's
		// cooling six hours out (event.Rest).
		if _, err := s.events.Ready(ctx, tx, actor.ID, authored.Opens); err != nil {
			return devResult{}, err
		}
		offer, err := s.events.Arrived(ctx, tx, actor.ID, where(c))
		if err != nil {
			return devResult{}, err
		}
		switch {
		case offer != nil:
			notice = fmt.Sprintf("%s is in front of you at %s — it lapses in %s",
				authored.Opens, where(c), offer.ExpiresAt.Sub(s.clk.Now()).Round(1e9))
		default:
			// Several things can leave an event queued and unoffered — one
			// already on screen, a placement that does not reach here, a row
			// still pending on something else. Say what is true and leave the
			// reason to the backlog.
			notice = authored.Opens + " is queued and did not surface here — check the backlog"
		}
	}
	return s.devSurface(ctx, tx, actor.ID, notice)
}

// Shutting the gate again. The run, if there is one, is left alone: closing a
// door on somebody standing in the room is not what this is for.
func (s *Server) devDisable(ctx context.Context, tx pgx.Tx, actor command.Actor, in devChallengeInput) (devResult, error) {
	code, _, err := s.devNamed(in)
	if err != nil {
		return devResult{}, err
	}
	if err := s.challenges.Disable(ctx, tx, actor.ID, code); err != nil {
		return devResult{}, err
	}
	return s.devSurface(ctx, tx, actor.ID, fmt.Sprintf("%s is shut", code))
}

/*
Putting one challenge back to never having happened.

Three deletions, and the third is the one that is easy to forget: the run, the
gate, and **the event that opened it**. Without the last, a second `dev.enable`
finds a backlog row that has already been answered and a record saying this
player has seen it, and the fire never comes back — which reads as the toolbar
being broken rather than as the world being consistent.

What it does not undo is anything the run paid. The factors stay granted and the
money stays in the balance, because those went to other modules through
interfaces internal/challenge deliberately cannot read back. Resetting replays
the *content*; it does not rewind the world, and a version that tried would need
the challenge framework to know what everything downstream did with what it was
given.
*/
func (s *Server) devReset(ctx context.Context, tx pgx.Tx, actor command.Actor, in devChallengeInput) (devResult, error) {
	code, authored, err := s.devNamed(in)
	if err != nil {
		return devResult{}, err
	}

	forgotten, err := s.challenges.Forget(ctx, tx, actor.ID, code)
	if err != nil {
		return devResult{}, err
	}
	if err := s.challenges.Disable(ctx, tx, actor.ID, code); err != nil {
		return devResult{}, err
	}
	if authored.Opens != "" {
		if _, err := s.events.Forget(ctx, tx, actor.ID, authored.Opens); err != nil {
			return devResult{}, err
		}
	}

	notice := fmt.Sprintf("%s is back to never having happened", code)
	if !forgotten {
		notice = fmt.Sprintf("%s had no run to forget; the gate is shut and its doorway is clear", code)
	}
	return s.devSurface(ctx, tx, actor.ID, notice)
}

// devNamed resolves the one argument these commands share.
func (s *Server) devNamed(in devChallengeInput) (challenge.Code, challenge.Definition, error) {
	code := challenge.Code(strings.TrimSpace(in.Challenge))
	if code == "" {
		return "", challenge.Definition{}, command.Invalid("a challenge has to be named")
	}
	authored, known := challenge.Known(code)
	if !known {
		return "", challenge.Definition{}, command.Conflict("that is not something the world has in it")
	}
	return code, authored, nil
}
