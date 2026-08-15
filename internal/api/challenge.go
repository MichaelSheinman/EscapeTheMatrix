package api

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/career"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/challenge"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/event"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/factor"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
Challenges on the wire (§5), and the two seams the module cannot reach itself.

#23 built the framework and deliberately left this file without a route: "the
commands, the socket messages and the screen arrive with the first challenge
there is to play (#24), because a route that can only ever answer 'there is no
such challenge' is not a surface, it is a placeholder." #24 is the argument on
the stoop, so the surface is here now.

Three properties of it are the ticket rather than the plumbing.

**No number leaves.** Not an option's weight, not a running total, not the key
of the band a finished run landed in — the whole account of how it went is the
reaction text authored for that band (§5.1), and §3.10's hidden scoring is
"never surfaced as a grade, never confirmed as right". internal/challenge does
not export a way to read a score into a view, and this file does not invent one.

**The words come from the mechanic, and this package never learns what they
mean.** A `challenge.Told` is text, the keys a choice is made by, and how far
through the run is. §5.2 gives each mechanic type "a Go handler plus a matching
client scene module"; this is the carriage between them.

**Beginning is not a thing a player asks for twice.** There is one command to
enter a challenge, and it either starts the run or hands back the one already in
progress — §5.1's save-and-resume means a client cannot be in a position to know
which of those it wants.
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

/*
pocket answers challenge.Purse: where money a run put in the player's hands goes.

Two things about it are decisions rather than plumbing.

**It is character.Wage rather than character.Work.** internal/character has two
ways to add money and §3.4 draws the line between them: `Work` is a paid *action*
and spends the day's allowance, `Wage` is structural and does not. What a
challenge hands over is neither a shift nor a click — it is a consequence of a
committed judgement that happens once ever — so counting it against the daily cap
would mean a player who carried a stranger's cash out of a fire could not also do
their job that day. The adapter is what makes the wrong one unreachable from
here, exactly as `payroll` does for internal/career.

**It is write-only, like `awards` beside it.** internal/challenge hands an amount
over and can never read a balance back, so nothing in an authored challenge can
branch on how rich the player is. A challenge that could would read differently
to a player with money than to one without, which is the same content telling two
people two different stories about what they are willing to do.
*/
type pocket struct {
	characters interface {
		Wage(ctx context.Context, tx pgx.Tx, accountID string, amount simulation.Money) error
	}
}

func (p pocket) Keep(ctx context.Context, tx pgx.Tx, accountID string, amount simulation.Money) error {
	return p.characters.Wage(ctx, tx, accountID, amount)
}

/* ── what a challenge looks like from outside ─────────────────────────── */

// doorwayView is a challenge standing where the player is (§4's availability
// half), and nothing about what it is.
//
// A name and a place. Not the scenes, not the options, not how long it is: a
// client that held the content could show yesterday's writing after a deploy,
// and one that could read the graph before entering could see where each
// reading leads — which would make the committed judgement a preview.
type doorwayView struct {
	// Challenge is the code, and what the begin command names.
	Challenge string `json:"challenge"`
	Name      string `json:"name"`
	// Place and Zone are where its entry point stands, by key. The zone is what
	// a painting draws the marker in (§10.3), exactly as an event's is.
	Place string `json:"place"`
	Zone  string `json:"zone"`
	// Door is what this says to somebody standing at it, and Enter is the
	// label on the button that begins it. Authored per challenge: every word a
	// player reads inside a challenge is content, and a client that supplied
	// these said the same sentence at a burning flat and a bank counter.
	Door  string `json:"door"`
	Enter string `json:"enter"`
}

// heldView is one thing in the player's hands: what it is, and what it is
// called. The key is what a client draws the picture from — art is the
// client's (§10.3), so the server names the object and never describes it.
type heldView struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// beatView is one line of a scene: somebody, and what they said. `who` is empty
// for the narration itself.
type beatView struct {
	Who  string `json:"who"`
	Line string `json:"line"`
}

// choiceView is one authored position, in the words the player reads.
//
// A key and a sentence. What it is worth is not here and cannot be: §5.1's
// authored deltas are paid by internal/challenge on completion and are never
// read back out of it, which is what keeps "each option carries authored hidden
// factor deltas" from being visible in a network panel.
type choiceView struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

/*
runView is one player's passage through one challenge, as a screen needs it.

Everything to re-establish the moment, because §5.1's save-and-resume means this
is what a returning player lands on rather than a list they choose from again.
The scene key crosses because the choose command names it: a stale tab answering
a question the run has already left is refused by name rather than applied
twice.
*/
type runView struct {
	Challenge string `json:"challenge"`
	Name      string `json:"name"`
	// Scene is the checkpoint's key — production vocabulary, and the one piece
	// of it the player's own client has to hold in order to answer.
	Scene string `json:"scene"`
	// Where the run is standing, in the content's own words.
	Where string `json:"where"`
	// Taken is how the last thing they committed to was received, which is not
	// the same as what they meant by it (§3.10).
	Taken   string       `json:"taken"`
	Beats   []beatView   `json:"beats"`
	Ask     string       `json:"ask"`
	Choices []choiceView `json:"choices"`

	// Karma is how much the composite moved when this run finished. Negative
	// when it fell; absent while the run is going or when it moved nothing.
	//
	// A number, because Karma is a number the player is already shown (§3.5
	// reveals the composite; it is the factors underneath that stay hidden). It
	// says how far it moved and nothing about why — not which factor, not what
	// they should have done, and never whether it was the right call.
	Karma float64 `json:"karma,omitempty"`
	// Earned is money the run put in their pocket, in cents. Shown because they
	// can already see their balance, so hiding it would only be a discrepancy.
	Earned int64 `json:"earned,omitempty"`

	// Stage and Stages are how far through it is: a length, not a mark.
	Stage  int `json:"stage"`
	Stages int `json:"stages"`

	// Mechanic is the type running this, so the client can pick the scene
	// module built for it (§5.2: "a Go handler plus a matching client scene
	// module"). It is the one piece of the definition a screen needs and the
	// only reason it crosses.
	Mechanic string `json:"mechanic"`
	// Canvas is the painted location this run stands in, or empty. A client
	// with no such painting simply keeps showing the world the player is
	// actually in.
	Canvas string `json:"canvas,omitempty"`
	// Held is what the player is carrying and Holds is how much they can carry.
	// Both empty for a mechanic with no notion of carrying anything.
	Held  []heldView `json:"held,omitempty"`
	Holds int        `json:"holds,omitempty"`

	Done bool `json:"done"`
	// Closing is the reaction authored for the outcome the trail landed in
	// (§5.1), and the only account of how it went the player ever gets. Empty
	// until it is finished.
	Closing string `json:"closing"`

	Began string `json:"began"`
	Moved string `json:"moved"`
}

/*
challengeView is the whole challenge surface for one player.

Two fields, and at most one of them is ever set. A player in the middle of a
six-minute endeavour is not also being shown a door into another one — §5.1's
resume is the thing that has to be unmissable, and an entry point beside it
would be an invitation to abandon a run by starting a second.

What is deliberately absent is a catalogue. Nothing here says what else exists,
what has been played, or what is coming: §4 keeps the world a sandbox rather
than a quest list, and a challenge nobody is standing in front of is not this
player's business.
*/
type challengeView struct {
	Here *doorwayView `json:"here"`
	Run  *runView     `json:"run"`
}

// told composes the run a mechanic narrated. The false from challenge.Say is a
// challenge withdrawn from content, or one whose mechanic has no words: both
// leave a run that exists and cannot be shown, which is reported as no run
// rather than as an empty screen.
func told(run challenge.Run) *runView {
	said, speaks := challenge.Say(run)
	if !speaks {
		return nil
	}

	out := &runView{
		Challenge: string(run.Challenge.Code),
		Name:      run.Challenge.Name,
		Scene:     run.Scene.Key,
		Where:     said.Where,
		Taken:     said.Taken,
		Ask:       said.Ask,
		Stage:     said.Stage,
		Stages:    said.Stages,
		Mechanic:  string(run.Challenge.Mechanic),
		Canvas:    run.Challenge.Canvas,
		Holds:     said.Holds,
		Done:      run.Done,
		Closing:   said.Closing,
		Began:     stamp(run.Began),
		Moved:     stamp(run.Moved),
	}
	for _, b := range said.Beats {
		out.Beats = append(out.Beats, beatView{Who: b.Who, Line: b.Line})
	}
	for _, c := range said.Choices {
		out.Choices = append(out.Choices, choiceView{Key: c.Key, Text: c.Text})
	}
	for _, h := range said.Held {
		out.Held = append(out.Held, heldView{Key: h.Key, Name: h.Name})
	}
	return out
}

/*
challenges reads the whole surface for one player, standing where they are.

The run comes first and wins. Everything else is §4's availability half: the
authored entry points that stand somewhere of their own accord, filtered to this
place and to what this player has not already been through — because a challenge
is once ever (§5.1) and a door that cannot be opened is worse than no door.

At most one is offered. Two challenges standing in the same spot is a content
question rather than a screen one, and the catalogue's order decides it the same
way it decides everything else — deterministically (§7).
*/
func (s *Server) challengeSurface(
	ctx context.Context, tx pgx.Tx, accountID, place string, today simulation.Date,
) (challengeView, error) {
	run, running, err := s.challenges.Of(ctx, tx, accountID)
	if err != nil {
		return challengeView{}, err
	}
	if running {
		if shown := told(run); shown != nil {
			return challengeView{Run: shown}, nil
		}
	}

	for _, d := range challenge.All() {
		zone, stands := d.Where.At(place)
		if !stands {
			continue
		}
		played, err := s.challenges.Played(ctx, tx, accountID, d.Code)
		if err != nil {
			return challengeView{}, err
		}
		if played {
			continue
		}
		// The gate: some challenges are things the world decides happen to you
		// rather than fixtures you can walk up to. An ungated one answers true
		// without touching the database, so this costs nothing for the four
		// challenges that predate it.
		open, err := s.challenges.Ungated(ctx, tx, accountID, d)
		if err != nil {
			return challengeView{}, err
		}
		if !open {
			continue
		}
		// The other condition on a doorway. An interview is a challenge like
		// any other and the framework does not know it is one — what decides
		// whether somebody may sit it is the experience its job asks for, which
		// is internal/career's question. Every challenge that is not an
		// interview passes this.
		sitting, err := s.careers.Sitting(ctx, tx, accountID, string(d.Code), today)
		if err != nil {
			return challengeView{}, err
		}
		if !sitting {
			continue
		}
		return challengeView{Here: &doorwayView{
			Challenge: string(d.Code), Name: d.Name, Place: place, Zone: zone,
			Door: d.Door, Enter: d.Enter,
		}}, nil
	}
	return challengeView{}, nil
}

/* ── the two commands ─────────────────────────────────────────────────── */

// challengeResult is what both commands answer with: the surface, restated.
//
// The whole surface rather than the piece that moved, for the reason every
// other command in this package restates its own slice — a response carrying
// only the new scene would leave the client able to hold a run and a doorway
// that disagree about what it is looking at.
type challengeResult struct {
	Challenge challengeView `json:"challenge"`
}

// beginChallengeInput names which challenge. There is no scene here: a run
// begins at the opening the content decided on, and a client that could name
// one could start in the middle of somebody else's story.
type beginChallengeInput struct {
	Challenge string `json:"challenge"`
}

/*
Walking up to a challenge that is standing there (§4's availability half).

Two refusals, and both are 409 rather than 422: the request is well formed and
it is the world that is in the way.

  - **It does not stand here.** Placement is the server's, exactly as an event's
    is. A client that could begin a challenge from anywhere would have turned
    §3.12's walking-is-the-encounter-surface into a menu.
  - **You are in the middle of something.** internal/challenge would happily
    hold two runs; the surface would then only be able to show one of them, and
    the other would be a six-minute endeavour the player abandoned without ever
    deciding to. One at a time is the honest reading of §5.1.

Beginning something already begun is not a refusal at all — see
[challenge.Challenges.Begin], where start and resume are one call.
*/
func (s *Server) beginChallenge(ctx context.Context, tx pgx.Tx, actor command.Actor, in beginChallengeInput) (challengeResult, error) {
	code := challenge.Code(strings.TrimSpace(in.Challenge))
	if code == "" {
		return challengeResult{}, command.Invalid("a challenge has to be named")
	}
	authored, known := challenge.Known(code)
	if !known {
		return challengeResult{}, command.Conflict("that is not something the world has in it")
	}

	// The gate, refused rather than merely hidden. A surface that does not offer
	// something and a command that will not do it are different guarantees, and
	// only the second one is a guarantee.
	open, err := s.challenges.Ungated(ctx, tx, actor.ID, authored)
	if err != nil {
		return challengeResult{}, err
	}
	if !open {
		return challengeResult{}, command.Conflict("that is not something happening to you")
	}

	c, err := s.characters.Ensure(ctx, tx, actor.ID)
	if err != nil {
		return challengeResult{}, err
	}
	// Standing at the entry point is the rule for a challenge — except an
	// interview, which is sat from the job board wherever the player is
	// (2026-08-08: even today, interviews happen online). The gate that
	// matters for those is internal/career's, below, and it is checked either
	// way in.
	_, interview := career.Interviewing(string(code))
	if !interview {
		if _, stands := authored.Where.At(where(c)); !stands {
			return challengeResult{}, command.Conflict("that is not in front of you")
		}
	}

	held, running, err := s.challenges.Of(ctx, tx, actor.ID)
	if err != nil {
		return challengeResult{}, err
	}
	if running && held.Challenge.Code != code {
		return challengeResult{}, command.Conflict("you are in the middle of something else")
	}

	// The third refusal, and the only one that is not about the framework:
	// whether internal/career will let this player sit this interview at all.
	// Checked here as well as on the surface because a hidden door and a
	// refused command are different guarantees, and only the second one is a
	// guarantee: the board hides an interview whose experience is not done, and
	// this refuses the command that names it anyway.
	sitting, err := s.careers.Sitting(ctx, tx, actor.ID, string(code), simulation.Today(s.clk, c.Zone))
	if err != nil {
		return challengeResult{}, err
	}
	if !sitting {
		return challengeResult{}, command.Conflict("they are not seeing anybody like you for that")
	}

	run, err := s.challenges.Begin(ctx, tx, actor.ID, code, 0)
	// An interview can be sat again (2026-08-10). §5.1's once-ever is right for
	// a challenge the world hands you — it happened, and that it happened is
	// the point — but an interview is a door to a job, and one bad answer
	// closing that door forever is the harshest lock in the game sitting behind
	// the least explanation: the button stays on the board and simply stops
	// working. So the spent run is forgotten and a fresh one begins.
	//
	// ErrDone is the trigger rather than a lookup because it is exactly the
	// question being asked, and because it cannot fire on a run in progress —
	// somebody part way through an interview resumes it (see
	// [challenge.Challenges.Begin]) rather than losing their place to a restart.
	//
	// What this does not do is un-pay the first sitting: the factors it granted
	// stay granted, which makes an interview repeatable *income* and is a known
	// cost of getting the path in front of players for V1. Balance is a later
	// version's problem, and the note is here so it is found rather than
	// rediscovered.
	if interview && errors.Is(err, challenge.ErrDone) {
		if _, forgotten := s.challenges.Forget(ctx, tx, actor.ID, code); forgotten != nil {
			return challengeResult{}, forgotten
		}
		run, err = s.challenges.Begin(ctx, tx, actor.ID, code, 0)
	}
	switch {
	case errors.Is(err, challenge.ErrDone):
		return challengeResult{}, command.Conflict("you have already been through that once")
	case err != nil:
		return challengeResult{}, err
	}

	out := challengeResult{Challenge: challengeView{Run: told(run)}}
	return out, s.moved(ctx, tx, actor, out.Challenge)
}

// chooseInChallengeInput is one committed option, at the scene it was offered
// at. The scene is named by the client so that a tab answering a question the
// run has already left is refused rather than applied twice (§5.1).
type chooseInChallengeInput struct {
	Challenge string `json:"challenge"`
	Scene     string `json:"scene"`
	Option    string `json:"option"`
}

/*
Committing to one of the authored positions.

The checkpoint moves here and nowhere else (§5.1), and so does everything that
comes of it: an option with nothing after it finishes the run, pays the authored
deltas, buckets the trail and settles the event that opened it — all inside the
one transaction this handler was given.

`ErrMoved` answers with the run as it actually stands rather than only with a
refusal, because the tab that sent it is showing a scene that no longer exists
and the useful reply is the one that puts it right.
*/
func (s *Server) chooseInChallenge(ctx context.Context, tx pgx.Tx, actor command.Actor, in chooseInChallengeInput) (challengeResult, error) {
	code := challenge.Code(strings.TrimSpace(in.Challenge))
	scene, option := strings.TrimSpace(in.Scene), strings.TrimSpace(in.Option)
	if code == "" || scene == "" || option == "" {
		return challengeResult{}, command.Invalid("a choice names the challenge, the scene and the option")
	}

	// What they were on before, so the last choice can say which way it went.
	// Read here rather than inside internal/challenge, which is deliberately
	// unable to see what its own deltas came to (see the `awards` adapter).
	before, err := s.factors.Of(ctx, tx, actor.ID)
	if err != nil {
		return challengeResult{}, err
	}
	wasHolding, err := s.characters.Ensure(ctx, tx, actor.ID)
	if err != nil {
		return challengeResult{}, err
	}

	run, chooseErr := s.challenges.Choose(ctx, tx, actor.ID, code, scene, option)
	err = chooseErr
	switch {
	case errors.Is(err, challenge.ErrUnknown):
		return challengeResult{}, command.Conflict("that is not something the world has in it")
	case errors.Is(err, challenge.ErrNotRunning):
		return challengeResult{}, command.Conflict("you are not in the middle of that")
	case errors.Is(err, challenge.ErrMoved):
		return challengeResult{}, command.Conflict("you have already answered that")
	case errors.Is(err, challenge.ErrNoOption):
		return challengeResult{}, command.Invalid("that is not one of the things you can say")
	case err != nil:
		return challengeResult{}, err
	}

	// An interview that has just been passed is a job offered, in the same
	// transaction as the option that ended the run (§3.14). internal/challenge
	// has never heard of a job and internal/career has never heard of a
	// challenge; this is the whole of what joins them, and it is here because
	// the composition root is where two systems are allowed to be named
	// together.
	if run.Done {
		if err := s.hired(ctx, tx, actor, string(code), run.Outcome.Key); err != nil {
			return challengeResult{}, err
		}
	}

	out := challengeResult{Challenge: challengeView{Run: told(run)}}

	// Which way it went, said once, at the end.
	//
	// Only a direction, and only on the last choice: the player is told that
	// something moved, never what or how much (§3.10). Money is the exception
	// and has to be — it is on their own balance either way, so saying nothing
	// would be a number changing behind their back.
	if run.Done && out.Challenge.Run != nil {
		after, err := s.factors.Of(ctx, tx, actor.ID)
		if err != nil {
			return challengeResult{}, err
		}
		out.Challenge.Run.Karma = after.Karma - before.Karma
		holding, err := s.characters.Ensure(ctx, tx, actor.ID)
		if err != nil {
			return challengeResult{}, err
		}
		if gained := holding.Money - wasHolding.Money; gained > 0 {
			out.Challenge.Run.Earned = gained.Cents()
		}

		// The run said which way it went; nothing has yet said where the player
		// now stands. The outcome's deltas landed on the hidden factors and the
		// money on the balance, so the composite has moved (§3.5) — and this is
		// the message every other command that moves it already sends. Without
		// it the rail keeps yesterday's number under an outcome card that just
		// told the player it went up, until they reload.
		if err := s.stood(ctx, tx, actor.ID); err != nil {
			return challengeResult{}, err
		}
	}

	return out, s.moved(ctx, tx, actor, out.Challenge)
}

// abandonChallengeInput names the run being walked out of.
type abandonChallengeInput struct {
	Challenge string `json:"challenge"`
}

/*
Walking out of a challenge, which puts it back to never having happened.

The run is deleted rather than marked, so the player may meet the challenge
again — "abandon" here means *it did not happen*, not *you had your go and
wasted it*. Once-ever counts finished runs, and there is now no run.

Nothing it did is undone, because it did nothing: the authored deltas and any
money are paid on completion and only on completion (see
[challenge.Challenges.Choose]), so a run walked out of half way through has
moved nothing in the world and there is nothing to take back.
*/
func (s *Server) abandonChallenge(ctx context.Context, tx pgx.Tx, actor command.Actor, in abandonChallengeInput) (challengeResult, error) {
	code := challenge.Code(strings.TrimSpace(in.Challenge))
	if code == "" {
		return challengeResult{}, command.Invalid("a challenge has to be named")
	}

	held, running, err := s.challenges.Of(ctx, tx, actor.ID)
	if err != nil {
		return challengeResult{}, err
	}
	if !running || held.Challenge.Code != code {
		return challengeResult{}, command.Conflict("you are not in the middle of that")
	}
	if _, err := s.challenges.Forget(ctx, tx, actor.ID, code); err != nil {
		return challengeResult{}, err
	}

	// The doorway has to close with it. §4 leaves an `awaits` event pending
	// until the module behind it settles the record — normally that is the run
	// finishing, and a run that no longer exists will never do it. Without this
	// the backlog keeps a row nobody can answer, and the world quietly stops
	// being able to offer that event ever again.
	//
	// It is resolved rather than put back: the event *did* happen — it was put
	// in front of the player and they said yes to it — and it is the challenge
	// behind it that did not. Which is the whole distinction this command is
	// for.
	if held.Opened > 0 {
		if err := s.events.Resolve(ctx, tx, actor.ID, held.Opened, s.clk.Now()); err != nil {
			return challengeResult{}, err
		}
	}

	c, err := s.characters.Ensure(ctx, tx, actor.ID)
	if err != nil {
		return challengeResult{}, err
	}
	out, err := s.challengeSurface(ctx, tx, actor.ID, where(c), simulation.Today(s.clk, c.Zone))
	if err != nil {
		return challengeResult{}, err
	}
	return challengeResult{Challenge: out}, s.moved(ctx, tx, actor, out)
}

/*
What a player's *other* tabs are told when a run moves.

One message for beginning, choosing and finishing, because they are one event
seen three times: the run is now standing somewhere else. The tab that acted is
skipped — it has the command's response, which is §8's rule 3 — and the payload
is the same view, so the client needs one reducer rather than three.
*/
type challengeMoved struct {
	Challenge challengeView `json:"challenge"`
}

var challengeMovedMessage = realtime.Define[challengeMoved]("challenge.moved")

func (s *Server) moved(ctx context.Context, tx pgx.Tx, actor command.Actor, view challengeView) error {
	return challengeMovedMessage.Publish(ctx, tx,
		realtime.To{Account: actor.ID, Origin: actor.Session},
		challengeMoved{Challenge: view},
	)
}

/*
opened is §4's doorway, walked through: an accepted event that offers a
challenge begins its run in the same transaction as the acceptance.

Accepting *is* beginning. The alternative — an event that leaves a challenge
"available" and a second command to enter it — would put a step between the
player saying yes and the thing they said yes to, and would need a surface
describing a door that had been opened and not gone through.

The one case worth naming is a player who has already been through this
challenge some other way. The event cannot know that (§4 keeps the two systems
distinct and internal/event has never heard of a challenge), so it is answered
here: the event is settled on the spot rather than left pending forever on a run
that will never exist.
*/
func (s *Server) opened(ctx context.Context, tx pgx.Tx, actor command.Actor, a event.Answered, offer int64) (*challengeView, error) {
	behind, offers := challenge.Opened(a.Event.Key)
	if !offers {
		return nil, nil
	}

	// The gate applies to the doorway too, and it has to: §4 keeps the event
	// backlog and the challenge catalogue as distinct systems, so an event has
	// no way of knowing that the challenge behind it is one the world has not
	// decided to give this player yet. Settled on the spot rather than left
	// pending, for the same reason a challenge already played is — the event
	// would otherwise wait forever on a run nobody will ever begin.
	open, err := s.challenges.Ungated(ctx, tx, actor.ID, behind)
	if err != nil {
		return nil, err
	}
	if !open {
		return nil, s.events.Resolve(ctx, tx, actor.ID, offer, s.clk.Now())
	}

	run, err := s.challenges.Begin(ctx, tx, actor.ID, behind.Code, offer)
	switch {
	case errors.Is(err, challenge.ErrDone):
		return nil, s.events.Resolve(ctx, tx, actor.ID, offer, s.clk.Now())
	case err != nil:
		return nil, err
	}

	out := challengeView{Run: told(run)}
	if err := s.moved(ctx, tx, actor, out); err != nil {
		return nil, err
	}
	return &out, nil
}
