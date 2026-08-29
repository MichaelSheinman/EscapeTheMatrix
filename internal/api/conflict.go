package api

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/account"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/conflict"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/playbook"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
Living among people on the wire (§3.15), and the seams internal/conflict cannot
reach itself.

Nothing here creates a conflict. The only thing that generates friction is a day
of the rollover passing; what is on this surface is the one conflict the player's
rules could not settle, and how the last fortnight has gone.

Nothing here is advice. The moment says what happened and the answers say what
your character would do — no field says which is right or what any of them is
worth (§3.2).

Nothing here is a register. The population is described by what its people do and
what they hold in common, in the content's own words, and internal/environment's
boot check guarantees none of those words names a stage or a framework.

And the counts are counts. met, covered, good and poor are how a fortnight went,
per person and per question. Nothing composes them into a verdict, because the
diagnosis is the player's (§3.6).
*/

/*
neighbourView is one person you live among, and how it has been going.

does is the load-bearing field. A population's values have to be legible purely
through behaviour and never labelled (§3.15), so a person crosses the wire as a
name, a note and a sentence about what they can be relied on to do — never as a
disposition or anything that could be rendered as a badge.

lives and known are facts rather than a description of anybody: an address and a
connection, which is what a contacts card holds.

standing is what is between the two of you, in the game's own words rather than
as four integers. A client handed the numbers would have to be told what they
mean, which puts a game rule in the browser (§10.2).
*/
type neighbourView struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Note string `json:"note"`
	// Lives and Known are the contacts card: where somebody lives, and how the
	// character knows them at all. Facts, both of them — see
	// content/environments/SPEC.md.
	Lives string `json:"lives"`
	Known string `json:"known"`
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

// recurringView is how one of the playbook's questions has been going among
// these people — §3.6's pattern, per question. covered is the number the design
// turns on: good rules buy time, and the holes are what demand attention
// (§3.15).
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
	// Holds is what these people have in common, written as behaviour — the one
	// place the player is told what their rules are up against, showable only
	// because nothing in it names what it is an example of (§3.2).
	Holds     []string        `json:"holds"`
	People    []neighbourView `json:"people"`
	Questions []recurringView `json:"questions"`
	// Since is the first day the counts include.
	Since string `json:"since"`
}

// waitingConflictView is one conflict standing in front of the player, carrying
// everything to re-establish it: a live decision that cannot be found again
// after closing the tab was never live (§5.1). standing is here as well as on
// the person because it is the reason this is happening — a conflict that
// followed from a debt is only legible as following from one if the debt is on
// the same screen (§3.6).
type waitingConflictView struct {
	// ID is what an answer names. Meaningless to anybody else: every read of a
	// waiting conflict is keyed by (id, account).
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Line string `json:"line"`
	// Who is the person it is with.
	Who string `json:"who"`
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
	switch {
	case w.Deferred:
		// Waiting because the player is here, which is not a failure of
		// anything. Saying "your playbook did not settle this" over an ordinary
		// conversation would be the game blaming their rules for the fact that
		// they were at the keyboard.
		out.Why = "you are here, so this one is yours to answer."
	case out.Why == "":
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
			Lives: n.Person.Lives, Known: n.Person.Known,
			Where: n.Person.Found, Standing: reads,
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

// conflictSurface reads what living somewhere has come to for this player. A
// plain read: unlike the event backlog and the endorsement offer, looking cannot
// cause one — only a day of the rollover passing produces a conflict.
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

// conflictAnswered is what a player's other tabs are told when a conflict is
// settled. came is what the answer came to in the content's own words — the same
// sentence the log reads back, because there is one account of how it went and
// both surfaces report it rather than composing one.
type conflictAnswered struct {
	Conflicts conflictsView `json:"conflicts"`
	Name      string        `json:"name"`
	Who       string        `json:"who"`
	Did       string        `json:"did"`
	Came      string        `json:"came"`
}

var conflictAnsweredMessage = realtime.Define[conflictAnswered]("conflict.answered")

// answerConflictInput is one posture, at the conflict it was offered for. The
// conflict is named by the client, so a stale tab answering something already
// settled is refused rather than applied to whatever is waiting now.
type answerConflictInput struct {
	Conflict int64  `json:"conflict"`
	Answer   string `json:"answer"`
}

type conflictResult struct {
	Conflicts conflictsView `json:"conflicts"`
	// Came is what it came to, in their words: the authored sentence rather than
	// a verdict. Nothing here says good or poor — the counts on the surface
	// above are where a pattern becomes visible (§3.2).
	Came string `json:"came"`
	Did  string `json:"did"`
}

/*
answerConflict is §3.15's step 3 by hand: the rules did not cover it, so the
player does. Not "compose a rule and let it run" — the conflict already happened,
and writing the answer that covers the next one is a different command on a
different screen. That gap is the loop.

Two refusals, both 409 rather than 422: a conflict another tab already settled
and one whose content went out in a deploy were both true a moment ago.
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
	// with whom. After it there is nothing waiting to read the name off.
	var name, who string
	if before, standing, err := s.conflicts.Waiting(ctx, tx, actor.ID); err != nil {
		return conflictResult{}, err
	} else if standing && before.ID == in.Conflict {
		name, who = before.Pathway.Name, before.Who.Name
	}

	// The day is carried in because answering continues it: the walk stopped at
	// this conflict and resumes now that it is settled (see conflict.Answer).
	c, err := s.characters.Ensure(ctx, tx, actor.ID)
	if err != nil {
		return conflictResult{}, err
	}
	answer, err := s.conflicts.Answer(
		ctx, tx, actor.ID, in.Conflict, stance, simulation.Today(s.clk, c.Zone))
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

	// The whole surface, restated: a response carrying only what was settled
	// would leave a client holding a conflict that no longer exists, and the
	// standing behind it decides what tomorrow is about.
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
presence answers conflict.Presence: is the player at the keyboard right now. What
present means is internal/account's — [account.Present] is the window and
Accounts.Here keeps the column current from the socket.

The window is generous on purpose. Reading somebody as present who has just left
costs one conflict a couple of minutes before their rules take it through
[conflict.Conflicts.Lapse]; reading somebody as absent who is sitting there means
the game answered a message in front of them.
*/
/*
LapseDeferred answers jobs.Lapsing: hand back whatever the players who have
walked away were holding (§3.15).

One transaction per player rather than one big one. Presence is per-account and
is asked inside the same transaction the lapse happens in, so a player who signs
back in mid-sweep keeps their message.

An error on one player is logged and the sweep goes on, rather than one withdrawn
pathway stopping everybody else's rules from covering them.
*/
func (s *Server) LapseDeferred(ctx context.Context) error {
	var holding []string
	if err := s.store.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		holding, err = s.conflicts.Deferring(ctx, tx)
		return err
	}); err != nil {
		return err
	}

	for _, accountID := range holding {
		if err := s.store.InTx(ctx, func(tx pgx.Tx) error {
			if err := s.conflicts.Lapse(ctx, tx, accountID); err != nil {
				return err
			}
			// And the day goes on, as it does when the player answers one
			// themselves: the walk stopped at the conflict that was handed over,
			// and now that their rules have taken it the rest of the population
			// gets its turn.
			c, err := s.characters.Ensure(ctx, tx, accountID)
			if err != nil {
				return err
			}
			_, err = s.conflicts.Day(ctx, tx, accountID, simulation.Today(s.clk, c.Zone))
			return err
		}); err != nil {
			s.log.Error("lapse deferred conflicts", "account", accountID, "error", err)
		}
	}
	return nil
}

type presence struct{ accounts *account.Accounts }

func (p presence) Present(ctx context.Context, tx pgx.Tx, accountID string) (bool, error) {
	return p.accounts.Active(ctx, tx, accountID, account.Present)
}

// Among answers rollover.World's §3.15 half: one day of living beside people.
// internal/rollover names the seam and internal/conflict fills it.
func (w work) Among(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date) error {
	_, err := w.conflicts.Day(ctx, tx, accountID, on)
	return err
}

// Compile-time proof that the seams this module takes are the interfaces they
// are for.
var (
	// The playbook itself answers conflict.Judgement — four of its own methods,
	// named as an interface so a day among people cannot reach the other eleven.
	_ conflict.Judgement = (*playbook.Playbooks)(nil)
	// And the same two bindings internal/factor and internal/situation take:
	// which population the player lives among, and where a hidden delta goes.
	_ conflict.Where  = population{}
	_ conflict.Awards = awards{}
)
