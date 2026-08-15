package api

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/conflict"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/playbook"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
Living among people on the wire (§3.15), and the seam internal/conflict cannot
reach itself.

Four properties of this file are the ticket rather than the plumbing.

**Nothing here creates a conflict.** There is no command that produces one and
there could not be: friction is what living somewhere *is*, so the only thing
that ever generates it is a day of the rollover passing. What is on this surface
is the one conflict the player's own rules could not settle, and the account of
how the last fortnight has gone.

**Nothing here is advice.** The moment says what happened and the answers say
what your character would do; no field says which is right, which one your rules
would have picked, or what any of them is worth. §3.2's "nothing is labelled" is
the whole reason this loop teaches anything.

**Nothing here is a register.** The population is described by what its people
do and what they hold in common, in the content's own words, and
internal/environment's boot check is what guarantees none of those words names a
stage or a framework. A client showing "these people are competitive" would be
the game handing over the answer; a client showing "asks what you do before your
name, and adjusts the length of the conversation to the answer" is the game
showing its working.

**The counts are counts.** `met`, `covered`, `good` and `poor` are how a
fortnight went, per person and per question, and they are the pattern §3.15 says
transfer failure only becomes legible as. Nothing composes them into a verdict,
because a verdict is the diagnosis and the diagnosis is the player's (§3.6).
*/

/*
neighbourView is one person you live among, and how it has been going.

`does` is the load-bearing field. §3.15: "characters must be consistent within an
environment; the population's values have to be legible purely through
behaviour, never labelled." So a person crosses the wire as a name, a note and a
sentence about what they can be relied on to do — and never as a disposition, an
alignment, or anything that could be rendered as a badge.

`standing` is what is between the two of you, in the game's own words rather
than as four integers. A client handed the numbers would have to be told what
they mean, which is the same thing as shipping the mechanic to the browser
(§10.2).
*/
type neighbourView struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Note string `json:"note"`
	Does string `json:"does"`
	// Where is the city place they turn up in, by key, so a client can say where
	// to find them without holding its own map.
	Where string `json:"where"`
	// Standing is what is outstanding between you, and is empty when nothing is.
	Standing []string `json:"standing"`
	// Met, Good and Poor are conflicts inside the window. Mixed is deliberately
	// not sent: three numbers is a shape and four is a scorecard.
	Met  int `json:"met"`
	Good int `json:"good"`
	Poor int `json:"poor"`
}

/*
recurringView is how one of the playbook's questions has been going among these
people — §3.6's pattern, per question.

`covered` is the number the whole design turns on. §3.15: "good rules buy time,
and the holes in your playbook are exactly what demands your attention" — so the
one thing a player has to be able to read off this surface is how many of the
week's conflicts their own rules handled and how many came to them.
*/
type recurringView struct {
	Question string `json:"question"`
	// Ask is the question in the player's own words, so this table reads as
	// sentences rather than as keys.
	Ask     string `json:"ask"`
	Met     int    `json:"met"`
	Covered int    `json:"covered"`
	Good    int    `json:"good"`
	Poor    int    `json:"poor"`
}

// amongView is who the player lives among, whole.
type amongView struct {
	Environment string `json:"environment"`
	Name        string `json:"name"`
	Note        string `json:"note"`
	// Holds is what these people have in common, written as behaviour. The one
	// place the player is told what their rules are up against — and it can be
	// shown at all only because nothing in it names what any of it is an example
	// of (§3.2).
	Holds     []string        `json:"holds"`
	People    []neighbourView `json:"people"`
	Questions []recurringView `json:"questions"`
	// Since is the first day the counts include.
	Since string `json:"since"`
}

/*
waitingConflictView is one conflict standing in front of the player.

Everything to re-establish it, because a live decision that cannot be found
again after closing the tab was never live — the same property §5.1 makes
mandatory for a challenge run.

`standing` is here as well as on the person because it is the reason this is
happening. §3.6 requires a player to be able to work out why the world did
something, and a conflict that followed from a debt is only legible as following
from one if the debt is on the same screen.
*/
type waitingConflictView struct {
	// ID is what an answer names. Meaningless to anybody else: every read of a
	// waiting conflict is keyed by (id, account).
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Line string `json:"line"`
	// Who and Does are the person, and what they can be relied on to do. The
	// second is sent because it is the only thing that makes the population's
	// values legible, and because it is what a player is supposed to be reading
	// when they decide (§3.15).
	Who  string `json:"who"`
	Does string `json:"does"`
	// Where they turn up, by city key.
	Where string `json:"where"`
	// Environment is where their rules were read from, by key (§3.15).
	Environment string `json:"environment"`
	// Why is the reason it reached them, in the log's own words: nothing covered
	// it, or these two answers disagreed. Never a suggestion.
	Why string `json:"why"`
	// Standing is what is outstanding between the two of you.
	Standing []string `json:"standing"`
	// On is the player's own day it arose on, and At is when it was written.
	On string `json:"on"`
	At string `json:"at"`

	Answers []answerView `json:"answers"`
}

// conflictsView is the whole daily-conflict surface for one player: the one
// thing waiting, and the people it came from.
type conflictsView struct {
	Waiting *waitingConflictView `json:"waiting"`
	Among   *amongView           `json:"among"`
}

func conflictOf(w conflict.Waiting) waitingConflictView {
	out := waitingConflictView{
		ID:          w.ID,
		Name:        w.Pathway.Name,
		Line:        w.Line,
		Who:         w.Who.Name,
		Does:        w.Who.Does,
		Where:       w.Who.Found,
		Environment: w.Environment,
		Why:         w.Note,
		Standing:    w.Standing,
		On:          w.On.At().Format("2006-01-02"),
		At:          stamp(w.At),
		Answers:     []answerView{},
	}
	if out.Standing == nil {
		out.Standing = []string{}
	}
	if out.Why == "" {
		// Only [playbook.Backfired] has no sentence, and it cannot wait — but a
		// conflict with nothing said about why it is here would read as a bug.
		out.Why = "your playbook did not settle this one."
	}
	for _, a := range w.Answers() {
		out.Answers = append(out.Answers, answerView{Key: string(a.Stance), Does: a.Did})
	}
	return out
}

func amongOf(l conflict.Living) *amongView {
	if !l.Among() {
		return nil
	}
	out := &amongView{
		Environment: l.Environment, Name: l.Name, Note: l.Note,
		Holds:     l.Holds,
		People:    make([]neighbourView, 0, len(l.People)),
		Questions: make([]recurringView, 0, len(l.Questions)),
		Since:     l.Since.At().Format("2006-01-02"),
	}
	if out.Holds == nil {
		out.Holds = []string{}
	}
	for _, n := range l.People {
		reads := n.Standing.Reads()
		if reads == nil {
			reads = []string{}
		}
		out.People = append(out.People, neighbourView{
			Key: n.Person.Key, Name: n.Person.Name, Note: n.Person.Note,
			Does: n.Person.Does, Where: n.Person.Found, Standing: reads,
			Met: n.Met, Good: n.Good, Poor: n.Poor,
		})
	}
	for _, r := range l.Questions {
		row := recurringView{
			Question: r.Question, Met: r.Met, Covered: r.Covered,
			Good: r.Good, Poor: r.Poor,
		}
		if q, asked := playbook.Vocab().Question(r.Question); asked {
			row.Ask = q.About
		}
		out.Questions = append(out.Questions, row)
	}
	return out
}

/*
conflictSurface reads what living somewhere has come to for this player.

A plain read: unlike the event backlog and the endorsement offer beside it in
the world snapshot, looking cannot cause one. A conflict is produced by a day of
the rollover passing and by nothing else, which is what "living among a
population" means when it is a shape rather than a sentence.
*/
func (s *Server) conflictSurface(
	ctx context.Context, tx pgx.Tx, accountID string, today simulation.Date,
) (conflictsView, error) {
	out := conflictsView{}

	living, err := s.conflicts.Living(ctx, tx, accountID, today)
	if err != nil {
		return conflictsView{}, err
	}
	out.Among = amongOf(living)

	waiting, standing, err := s.conflicts.Waiting(ctx, tx, accountID)
	if err != nil {
		return conflictsView{}, err
	}
	if standing {
		view := conflictOf(waiting)
		out.Waiting = &view
	}
	return out, nil
}

// surface is the same read, for a caller that has the character to hand rather
// than the day.
func (s *Server) conflictsFor(ctx context.Context, tx pgx.Tx, accountID string) (conflictsView, error) {
	who, err := s.characters.Ensure(ctx, tx, accountID)
	if err != nil {
		return conflictsView{}, err
	}
	return s.conflictSurface(ctx, tx, accountID, simulation.Today(s.clk, who.Zone))
}

/* ── settling one by hand ─────────────────────────────────────────────── */

// What a player's *other* tabs are told when a conflict is settled.
//
// `came` is what the answer came to, in the content's own words — the same
// sentence the log will read back, because there is only one account of how it
// went and both surfaces report it rather than composing one.
type conflictAnswered struct {
	Conflicts conflictsView `json:"conflicts"`
	Name      string        `json:"name"`
	Who       string        `json:"who"`
	Did       string        `json:"did"`
	Came      string        `json:"came"`
}

var conflictAnsweredMessage = realtime.Define[conflictAnswered]("conflict.answered")

// answerConflictInput is one posture, at the conflict it was offered for.
//
// The conflict is named by the client so that a stale tab answering something
// the world has already settled is refused rather than applied to whatever is
// waiting now.
type answerConflictInput struct {
	Conflict int64  `json:"conflict"`
	Answer   string `json:"answer"`
}

type conflictResult struct {
	Conflicts conflictsView `json:"conflicts"`
	// Came is what it came to, in their words: their reply back, and the only
	// account of how it went the player gets. The authored sentence rather than
	// a verdict — nothing here says
	// good or poor, because §3.2 does not label a choice and the counts on the
	// surface above are where a pattern is supposed to become visible instead.
	Came string `json:"came"`
	Did  string `json:"did"`
}

/*
Settling one yourself.

§3.15's step 3, by hand: the rules did not cover it, so the player does. What
this deliberately is not is "compose a rule and let it run" — the conflict
already happened, and writing the answer that covers the *next* one is a
different command on a different screen. That gap is the loop.

Two refusals, and both are 409 rather than 422: the request is well formed and it
is the world that is in the way. A conflict another tab already settled and one
whose content went out in a deploy are both things that were true a moment ago.
*/
func (s *Server) answerConflict(
	ctx context.Context, tx pgx.Tx, actor command.Actor, in answerConflictInput,
) (conflictResult, error) {
	if in.Conflict <= 0 {
		return conflictResult{}, command.Invalid("an answer names the conflict it answers")
	}
	stance := playbook.Stance(strings.TrimSpace(in.Answer))
	if stance == "" {
		return conflictResult{}, command.Invalid("an answer names one of the things you could do")
	}

	// Read before answering, so the message below can say what was settled and
	// with whom. After it there is nothing waiting to read the name off, and a
	// tab told only "your character did this" would have to guess what about.
	var name, who string
	if before, standing, err := s.conflicts.Waiting(ctx, tx, actor.ID); err != nil {
		return conflictResult{}, err
	} else if standing && before.ID == in.Conflict {
		name, who = before.Pathway.Name, before.Who.Name
	}

	answer, err := s.conflicts.Answer(ctx, tx, actor.ID, in.Conflict, stance)
	switch {
	case errors.Is(err, conflict.ErrNoSuchConflict):
		return conflictResult{}, command.Conflict("that is no longer in front of you")
	case errors.Is(err, conflict.ErrWithdrawn):
		return conflictResult{}, command.Conflict("that is no longer something the world has in it")
	case errors.Is(err, conflict.ErrNoSuchAnswer):
		return conflictResult{}, command.Invalid("that is not one of the things you can do about it")
	case err != nil:
		return conflictResult{}, err
	}

	// The whole surface, restated. A response carrying only what was settled
	// would leave a client holding a conflict that no longer exists — and the
	// standing behind it has moved, which is the half of this mechanic that
	// decides what tomorrow is about.
	out := conflictResult{Came: answer.Came, Did: answer.Did}
	if out.Conflicts, err = s.conflictsFor(ctx, tx, actor.ID); err != nil {
		return conflictResult{}, err
	}

	// A settled conflict moves hidden factors (§3.5), so Karma moved with it.
	if err := s.stood(ctx, tx, actor.ID); err != nil {
		return conflictResult{}, err
	}

	return out, conflictAnsweredMessage.Publish(ctx, tx,
		realtime.To{Account: actor.ID, Origin: actor.Session},
		conflictAnswered{
			Conflicts: out.Conflicts,
			Name:      name,
			Who:       who,
			Did:       answer.Did,
			Came:      answer.Came,
		},
	)
}

/* ── the bindings ─────────────────────────────────────────────────────── */

/*
Among answers rollover.World's §3.15 half: one day of living beside people.

The composition root's line, exactly like `delegated` and `employed` beside it —
internal/rollover names the seam, internal/conflict fills it, and neither
package edits the other.
*/
func (w work) Among(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date) error {
	_, err := w.conflicts.Day(ctx, tx, accountID, on)
	return err
}

// Compile-time proof that the seams this module takes are the interfaces they
// are for.
var (
	// The playbook itself answers conflict.Judgement — four of its own methods,
	// named as an interface so that a day among people cannot reach the other
	// eleven. It is the module rather than an adapter because every one of the
	// four is already exactly the shape the seam asks for.
	_ conflict.Judgement = (*playbook.Playbooks)(nil)
	// And the same two bindings internal/factor and internal/situation take:
	// which population the player lives among, and where a hidden delta goes.
	_ conflict.Where  = population{}
	_ conflict.Awards = awards{}
)
