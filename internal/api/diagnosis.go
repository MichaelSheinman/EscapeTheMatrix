package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/diagnosis"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/environment"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/health"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/playbook"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/routine"
)

/*
The diagnosis on the wire (§3.6), and the four seams the module cannot reach for
itself.

This file is where the game's primary consciousness-raising mechanism is joined
to the rest of it, and three of the joins are the ticket rather than the
plumbing.

**No verdict leaves, because none exists.** There is nothing in this file that
scores an argument, and nothing in internal/diagnosis that could be asked to.
The one refusal that touches a player's reasoning is `ErrDoesNotSpeak` and it is
mapped to a 422 — the same status a playbook term outside the vocabulary gets —
because it is a statement about what was pointed at rather than about what is
true. There is deliberately no status code in this game that means *wrong*.

**The evidence is composed from the modules that own it.** Two ledgers are bound
below, and each one turns its own rows into readings. This package carries them;
it does not know what a short night is and could not compute one.

**Which routines are unlocked is a read, not a write.** [gate] answers
internal/routine's `Gate` straight out of the conclusions table, so a player who
named a cause has the routine the next time anything asks, with nothing to
back-fill and nothing to keep in step. The line it replaces in api.New was left
by #9 for exactly this.
*/

/* ── the ledgers a diagnosis is argued from ────────────────────────────── */

/*
bodyLedger answers diagnosis.Ledger out of the health panel (§3.16).

It reports no failures, and that is the interesting half. A body is evidence, not
a moment: nothing that happens to a resting heart rate is an event a person could
sit down and account for, and staging a scene about one would be the game telling
the player that something is wrong — which is the diagnosis they are supposed to
be making. So the body says what it observes and never what went badly, and every
occasion in this game comes from something that actually happened to somebody.
*/
type bodyLedger struct{ bodies *health.Health }

func (bodyLedger) Failed(context.Context, pgx.Tx, string) ([]diagnosis.Failure, error) {
	return nil, nil
}

func (l bodyLedger) Observed(
	ctx context.Context, tx pgx.Tx, accountID string,
) ([]diagnosis.Observation, error) {
	// Ensure rather than Of. This runs on a command path as well as inside the
	// world snapshot, and the snapshot is the thing that happens to have created
	// the panel already — a player whose first act after signing in is to say
	// what they think happened would otherwise read a body that does not exist
	// yet. Doing nothing is the overwhelmingly common case and costs one insert
	// that conflicts, which is the argument health.create makes for itself.
	panel, err := l.bodies.Ensure(ctx, tx, accountID)
	if err != nil {
		return nil, err
	}
	out := []diagnosis.Observation{}
	for _, o := range panel.Observed() {
		out = append(out, diagnosis.Observation{Key: o.Key, Says: o.Says, From: o.From})
	}
	return out, nil
}

/*
logLedger answers diagnosis.Ledger out of the playbook's records (§3.14).

Both halves, and the failure half is the one that decides when a diagnosis is
available at all: §4's three escalation conditions, already put to the player by
the world, read back as the moments there is something to account for.

`Says` is composed here rather than in either module, because it is a sentence
about a *situation* that internal/playbook only holds the key of. The situation
catalogue belongs to internal/situation and internal/conflict, and rather than
teach this file to read either, the moment is described by what the playbook made
of it — which is also the half a diagnosis is about.
*/
type logLedger struct {
	playbooks *playbook.Playbooks
	clk       interface{ Now() time.Time }
}

func (l logLedger) Failed(
	ctx context.Context, tx pgx.Tx, accountID string,
) ([]diagnosis.Failure, error) {
	raised, err := l.playbooks.Failed(ctx, tx, accountID, l.clk.Now())
	if err != nil {
		return nil, err
	}
	out := make([]diagnosis.Failure, 0, len(raised))
	for _, e := range raised {
		out = append(out, diagnosis.Failure{
			Key:  fmt.Sprintf("playbook/fire/%d", e.Fire),
			At:   e.At,
			Says: went[e.Cause],
		})
	}
	return out, nil
}

func (l logLedger) Observed(
	ctx context.Context, tx pgx.Tx, accountID string,
) ([]diagnosis.Observation, error) {
	trends, err := l.playbooks.Evidence(ctx, tx, accountID, l.clk.Now().Add(-playbook.Since))
	if err != nil {
		return nil, err
	}
	out := []diagnosis.Observation{}
	for _, o := range playbook.Observed(trends) {
		out = append(out, diagnosis.Observation{Key: o.Key, Says: o.Says, From: o.From})
	}
	return out, nil
}

/*
went is what each of §4's three escalation conditions was, in the player's words.

The mapping lives here for the same reason `escalations` does, three files over:
internal/playbook knows that a rule went badly and internal/diagnosis knows that
something did, and neither of them should learn the other's vocabulary. This is
the only place the two are named together.

None of the three says why. Saying why is the whole of what the player is about
to do.
*/
var went = map[playbook.Cause]string{
	playbook.Unhandled: "Something came to you that none of your rules covered, and you answered it on the spot.",
	playbook.Contested: "Two of your own answers reached the same moment and disagreed, and it came to you.",
	playbook.Backfired: "You knew what to do, you did it, and it came back worse than doing nothing would have.",
}

/*
observable is every observation key the two ledgers above can emit.

The list is a fact about which ledgers are bound in api.New rather than about
either module, which is why it is composed here and why binding a third ledger
means adding one line to it. A cause pointing at a key nothing in here produces
fails the boot.
*/
func observable() []string {
	return append(health.Observable(), playbook.Observable()...)
}

/*
gate answers routine.Gate out of what the player has concluded (§3.6).

The whole of struggle-gated unlocking, on one method. #9 wrote the locked path
and bound it to routine.Open — "every routine, from the start" — with a comment
saying the diagnosis that would replace it was #32. This is that replacement, and
it is one line in api.New, which is what the interface was for.
*/
type gate struct{ diagnoses *diagnosis.Diagnoses }

func (g gate) Unlocked(ctx context.Context, tx pgx.Tx, accountID string) (map[string]bool, error) {
	return g.diagnoses.Named(ctx, tx, accountID)
}

/* ── what a diagnosis looks like from outside ──────────────────────────── */

// observationView is one thing the player's own history says.
type observationView struct {
	Key string `json:"key"`
	// Says is a reading with the player's own numbers in it, produced by the
	// module that owns the rows. Nothing authored is ever offered here — see
	// the package comment on internal/diagnosis, rule 1.
	Says string `json:"says"`
	From string `json:"from"`
}

// causeView is one account the player could commit to.
//
// It carries no score, no ordering and no hint. The client renders these in
// authored order and has no field it could sort them by if somebody wanted it
// to.
type causeView struct {
	Key     string `json:"key"`
	Says    string `json:"says"`
	Because string `json:"because"`
	// Speaks is which of the player's own observations bear on this account,
	// by key. Sent so the client can show the connection the player is making
	// rather than making them hold it in their head — and it is derived from
	// their evidence, so it is a fact about their data and not a hint about
	// which one is right.
	Speaks []string `json:"speaks"`
	// Unlocks is what naming it hands over, in the player's words. Stated up
	// front, because §3.6 gates the tool behind the insight and hiding which
	// tool would make the choice a lottery rather than a decision.
	Unlocks string `json:"unlocks"`
}

// sceneView is the person and what they say.
type sceneView struct {
	Who    string `json:"who"`
	Note   string `json:"note"`
	Opens  string `json:"opens"`
	Asks   string `json:"asks"`
	Weighs string `json:"weighs"`
	Closes string `json:"closes"`
}

// diagnosisView is the whole surface: what is open, who is here, what the
// player has to go on, and what they have concluded before.
type diagnosisView struct {
	// Open is whether there is anything to account for. False is the ordinary
	// state and the client says nothing about it.
	Open bool `json:"open"`
	// Occasion is the failure's key, sent back with the command so a player
	// answering a scene the world has moved past gets a 409 rather than a
	// conclusion filed against the wrong moment.
	Occasion string            `json:"occasion"`
	Went     string            `json:"went"`
	Scene    sceneView         `json:"scene"`
	Evidence []observationView `json:"evidence"`
	Causes   []causeView       `json:"causes"`
	// Again and Said are §3.6's fifth rule: this is a revisit, and here is what
	// was concluded last time.
	Again bool   `json:"again"`
	Said  string `json:"said"`
	// Named is what the player has concluded, newest first — the record a
	// revisit is argued against, and the only place a past conclusion is
	// visible. Nothing on it says how any of them turned out.
	Named []namedView `json:"named"`
}

// namedView is one past conclusion.
type namedView struct {
	Says string `json:"says"`
	At   string `json:"at"`
	// Unlocks is what it handed over, restated, because a player looking back
	// at a conclusion they now doubt should be able to see what it bought them
	// — including when the answer is a routine they have never once used.
	Unlocks string `json:"unlocks"`
}

/*
diagnosisOf composes the view.

The causes' `Speaks` is narrowed to the evidence this player actually has, which
is the one piece of shaping this function does and it is worth naming: a cause
that lists three observations and matches one must not tell the player about the
other two. That would be the game describing evidence they do not have, which is
both a hint and a description of somebody else's life.
*/
func diagnosisOf(s diagnosis.Sitting, named []diagnosis.Conclusion) diagnosisView {
	out := diagnosisView{
		Open:     s.Available(),
		Occasion: s.Open.Key,
		Went:     s.Open.Says,
		Again:    s.Again,
		Said:     s.Said,
		Evidence: []observationView{},
		Causes:   []causeView{},
		Named:    []namedView{},
	}

	have := make(map[string]bool, len(s.Evidence))
	for _, o := range s.Evidence {
		have[o.Key] = true
		out.Evidence = append(out.Evidence, observationView{Key: o.Key, Says: o.Says, From: o.From})
	}
	for _, c := range s.Causes {
		speaks := []string{}
		for _, k := range c.Speaks {
			if have[k] {
				speaks = append(speaks, k)
			}
		}
		out.Causes = append(out.Causes, causeView{
			Key: c.Key, Says: c.Says, Because: c.Because,
			Speaks: speaks, Unlocks: hands(c),
		})
	}
	for _, c := range named {
		view := namedView{Says: c.Cause, At: c.At.Format(time.DateOnly)}
		if authored, ok := diagnosis.Known(c.Cause); ok {
			view.Says, view.Unlocks = authored.Says, hands(authored)
		}
		out.Named = append(out.Named, view)
	}

	if s.Available() {
		out.Scene = sceneView{
			Who: s.With, Opens: s.Scene.Opens, Asks: s.Scene.Asks,
			Weighs: s.Scene.Weighs, Closes: s.Scene.Closes,
		}
		// The person's own name and what they are like, from the population
		// they belong to. Read here rather than carried on the scene because
		// internal/environment is authored content with no state behind it, and
		// a second copy of a name is a second thing to keep correct.
		if env, ok := environment.Lookup(s.Scene.Population); ok {
			if who, ok := env.Person(s.With); ok {
				out.Scene.Who, out.Scene.Note = who.Name, who.Does
			}
		}
	}
	return out
}

/*
hands says what naming a cause gives you, in the player's words.

A sentence rather than a list of keys, because a term ID is a fact about the
vocabulary's storage and "the words for saying that a moment is not the moment
you wrote the rule for" is a fact about what the player can now think. The count
is deliberately vague about *which* words: §3.14 is explicit that a locked term
is absent rather than hidden, so naming them before they are earned would be the
one thing that file refuses.
*/
func hands(c diagnosis.Cause) string {
	var parts []string
	if c.Routine != "" {
		for _, k := range routine.Kinds {
			if k.Key == c.Routine {
				parts = append(parts, "the arranging of "+k.Name)
				break
			}
		}
	}
	switch n := len(c.Terms); {
	case n == 1:
		parts = append(parts, "a word for your playbook")
	case n > 1:
		parts = append(parts, fmt.Sprintf("%d words for your playbook", n))
	}
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	default:
		return parts[0] + ", and " + parts[1]
	}
}

/* ── the read ──────────────────────────────────────────────────────────── */

// sitting reads the whole surface in one transaction.
func (s *Server) sitting(ctx context.Context, tx pgx.Tx, accountID string) (diagnosisView, error) {
	sat, err := s.diagnoses.Sit(ctx, tx, accountID)
	if err != nil {
		return diagnosisView{}, err
	}
	named, err := s.diagnoses.Concluded(ctx, tx, accountID)
	if err != nil {
		return diagnosisView{}, err
	}
	return diagnosisOf(sat, named), nil
}

/* ── the command ───────────────────────────────────────────────────────── */

/*
namedResult is what comes back, and what the player's other tabs are told.

Three surfaces rather than one, because naming a cause is the only moment in the
game where a tool appears. The diagnosis has moved (the occasion is accounted
for), the routines have moved (one of them may now be configurable), and the
playbook has moved (words that were absent are now in the vocabulary). A response
carrying only the first would leave a client showing a locked routine one beat
after the sentence that unlocked it.
*/
type namedResult struct {
	// Said is the account the player committed to, in their own words. It is
	// what the scene shows back to them and it is not a confirmation of
	// anything — see the `closes` line in either scene file.
	Said      string        `json:"said"`
	Diagnosis diagnosisView `json:"diagnosis"`
	Routines  routinesView  `json:"routines"`
	Playbook  playbookView  `json:"playbook"`
}

var diagnosisNamedMessage = realtime.Define[namedResult]("diagnosis.named")

// nameInput is the command's input: which moment, which account of it, and what
// the player pointed at.
type nameInput struct {
	Occasion string   `json:"occasion"`
	Cause    string   `json:"cause"`
	Pointed  []string `json:"pointed"`
}

/*
Naming a cause.

Every refusal below is about the request or about the world, and the list is
worth reading for what is missing from it: there is no path through this handler
that tells a player their reasoning was poor. The closest is the 422 for a cause
that nothing they pointed at bears on, which refuses a malformed argument rather
than a false one — and a client cannot reach it in the ordinary way, because the
scene only offers causes the player's own evidence already speaks to.

There is one command and no second. Nothing here opens a diagnosis, because what
opens one is something having gone badly, and a button for that would be the
game asking the player to schedule their own moment of consequence.
*/
func (s *Server) nameCause(
	ctx context.Context, tx pgx.Tx, actor command.Actor, in nameInput,
) (namedResult, error) {
	if in.Occasion == "" {
		return namedResult{}, command.Invalid("a conclusion names the moment it is about")
	}
	if in.Cause == "" {
		return namedResult{}, command.Invalid("a conclusion names what you think happened")
	}

	named, err := s.diagnoses.Name(ctx, tx, actor.ID, in.Occasion, in.Cause, in.Pointed)
	switch {
	case errors.Is(err, diagnosis.ErrNoOccasion):
		// The world moved rather than the request being wrong: the moment has
		// been accounted for, or something newer is in front of them now.
		return namedResult{}, command.Conflict("that is not what is in front of you any more")
	case errors.Is(err, diagnosis.ErrNoSuchCause):
		return namedResult{}, command.Invalid("that is not an account of anything")
	case errors.Is(err, diagnosis.ErrNotYours):
		return namedResult{}, command.Invalid("that is not something your own history says")
	case errors.Is(err, diagnosis.ErrDoesNotSpeak):
		// Not a judgement. The player has not pointed at anything that bears on
		// the account they are giving, which is a claim about the argument's
		// shape and the only refusal in this game that touches one.
		return namedResult{}, command.Invalid("nothing you pointed at bears on that")
	case errors.Is(err, diagnosis.ErrNowhereToSit):
		return namedResult{}, command.Conflict("there is nobody here to say it to")
	case err != nil:
		return namedResult{}, err
	}

	out := namedResult{Said: named.Says}
	if out.Diagnosis, err = s.sitting(ctx, tx, actor.ID); err != nil {
		return namedResult{}, err
	}

	// Both tools, re-read in the same transaction. The routines come back
	// through the gate this naming just moved, which is the whole of §3.6
	// arriving in one response.
	set, err := s.routines.Of(ctx, tx, actor.ID)
	if err != nil {
		return namedResult{}, err
	}
	out.Routines = routines(set)

	book, err := s.playbooks.Of(ctx, tx, actor.ID)
	if err != nil {
		return namedResult{}, err
	}
	out.Playbook = playbookOf(book)

	if err := diagnosisNamedMessage.Publish(ctx, tx,
		realtime.To{Account: actor.ID, Origin: actor.Session}, out,
	); err != nil {
		return namedResult{}, err
	}
	return out, nil
}
