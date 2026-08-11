package api

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/career"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/factor"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/rollover"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
Jobs on the wire (§3.14), and the three seams internal/career cannot reach
itself.

Two commands and one screen's worth of view. What is not here is anything that
produces money on demand: §3.4's income is structural, so the only thing that
ever pays a wage is the day ending, and the only thing on this surface is which
job is held.

**No factor is named and no floor is a number.** A rung's requirement carries a
floor on all five factors including the three the player is never shown (§3.5),
so the chips below come out of internal/factor already veiled — the two revealed
ones with a name and whether they are cleared, and the rest as blanked slots
that say something else is being asked and nothing about what. Working out what
is the hook (§3.5), and a chip that read "stability: 12 / 15" would answer it.
*/

/*
payroll answers career.Payroll: where a day's wage goes.

An adapter rather than the character module itself, and the reason is the one
§3.4 draws: internal/character has two ways to add money and only one of them is
right for this. `Work` is a paid *action* and spends the day's allowance; `Wage`
is structural and does not. Handing internal/career the whole module would hand
it both.
*/
type payroll struct {
	characters interface {
		Wage(ctx context.Context, tx pgx.Tx, accountID string, amount simulation.Money) error
	}
}

func (p payroll) Wage(ctx context.Context, tx pgx.Tx, accountID string, amount simulation.Money) error {
	return p.characters.Wage(ctx, tx, accountID, amount)
}

/*
capability answers career.Ready: whether a player reaches a rung's requirement.

One boolean, and the adapter exists to make sure it stays one. §3.14's "each
step up requires capability" is asked of factors three of which are never named
(§3.5), so the comparison happens inside internal/factor and what comes out is a
yes or a no — the same one-way shape peer endorsement's `grants` and the
challenge framework's `awards` take.
*/
type capability struct {
	factors *factor.Factors
}

func (c capability) Ready(
	ctx context.Context, tx pgx.Tx, accountID string, need simulation.Requirement,
) (bool, error) {
	return c.factors.Ready(ctx, tx, accountID, need)
}

/*
tolls answers career.Credits: what a day of a job did to the hidden factors.

The Recorded that internal/factor hands back is dropped here, and that is the
adapter's whole job. Every other caller of Record is an authored moment the
player just lived through and is owed an account of; a day of work is not one.
Nobody is told that the till moved anything, so nothing that could tell them
is allowed back across.
*/
type tolls struct {
	factors *factor.Factors
}

func (t tolls) Record(
	ctx context.Context, tx pgx.Tx, accountID string, grants ...simulation.Grant,
) error {
	_, err := t.factors.Record(ctx, tx, accountID, grants...)
	return err
}

/*
employed answers rollover.Employer: the job half of a day.

The composition root's job, exactly like `work` in rollover.go — internal/
rollover names the seam and internal/career fills it, and neither package edits
the other. It is a named type rather than the module because [rollover.Employer]
is one method and handing over the whole module would hand over hiring.
*/
type employed struct {
	careers *career.Careers
}

func (e employed) Shift(
	ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date, d simulation.Day,
) (simulation.Day, error) {
	return e.careers.Shift(ctx, tx, accountID, on, d)
}

// Compile-time proof that the two seams above are the two interfaces they are
// for. Both are bound in New, so this is belt and braces — and it is the line
// that fails first if either interface grows.
var (
	_ career.Payroll    = payroll{}
	_ career.Capability = capability{}
	_ rollover.Employer = employed{}
)

/* ── what a job looks like from outside ───────────────────────────────── */

// wantView is one thing a rung asks for, as the player is allowed to read it.
//
// `state` is `met`, `unmet` or `hidden`, which is exactly @etm/ui's `Posting`
// vocabulary. The third is not a masked value: it carries no name and no
// verdict, because a blanked chip that flipped to "met" would say what it was
// measuring at the moment it stopped mattering.
type wantView struct {
	Label string `json:"label"`
	State string `json:"state"`
}

func wants(asked []factor.Want) []wantView {
	out := make([]wantView, 0, len(asked))
	for _, w := range asked {
		if !w.Shown {
			out = append(out, wantView{State: "hidden"})
			continue
		}
		state := "unmet"
		if w.Met {
			state = "met"
		}
		out = append(out, wantView{Label: w.Name, State: state})
	}
	return out
}

/*
roleView is one rung of §3.14's ladder, in the words a player reads.

Pay is cents a day and stays cents to the screen, like every other amount in
this package. Hours and nights are here because they are the honest half of what
a job costs — the ladder is an argument about attention, and a rung that showed
its salary and not its shift would be making half of it.
*/
type roleView struct {
	Tier int    `json:"tier"`
	Key  string `json:"key"`
	Name string `json:"name"`
	// Posting is the job as it is advertised, in the sections a real posting
	// has. Nothing is labelled good or bad (§3.2): every posting carries the
	// same sections, so the layout says nothing about which line matters.
	Posting postingView `json:"posting"`
	// Pay is what one day of holding it is worth, in cents: the rung's wage and
	// the role's premium over it, added here so no client ever has to know there
	// were two numbers.
	Pay    int64 `json:"pay"`
	Hours  int   `json:"hours"`
	Nights bool  `json:"nights"`
	// Shift is the working day as a posting states it, "21:00–07:00": the
	// file's start, the rung's length, composed here so no client ever adds
	// hours to a clock.
	Shift string `json:"shift"`
}

// postingView is [career.Posting] on the wire, one field per section.
type postingView struct {
	Intro          string   `json:"intro"`
	Duties         []string `json:"duties"`
	Qualifications []string `json:"qualifications"`
}

// role is a rung in the words a player reads. What it deliberately does not
// carry is career.Role.Toll: what a job quietly does to somebody is the thing
// the posting is asking them to work out, and a field on the wire is that
// question answered in the network tab.
func role(r career.Role) roleView {
	out := roleView{
		Tier: int(r.Tier), Key: r.Key, Name: r.Name,
		Posting: postingView{
			Intro:          r.Posting.Intro,
			Duties:         r.Posting.Duties,
			Qualifications: r.Posting.Qualifications,
		},
	}
	if rung, known := simulation.Rungs().At(r.Tier); known {
		out.Pay = (rung.Pay + r.Premium).Cents()
		out.Hours = int(rung.Post.Hours.Hours())
		out.Nights = rung.Post.Nights
		out.Shift = wallClock(r.ShiftStart) + "–" + wallClock(r.ShiftStart+int(rung.Post.Hours.Minutes()))
	}
	return out
}

// wallClock renders minutes from midnight as a wall clock, wrapping past it
// the way a nights posting does.
func wallClock(minutes int) string {
	minutes %= 24 * 60
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}

/*
openingView is a rung the player does not hold, and what stands between them and
it.

Two ways in, and at most one of them is ever true at once. `Walk` is a job that
is taken on where the player is standing right now; `Interview` is the authored
challenge they have to get through (§3.4: "getting hired spawns an interview
challenge the player actually has to get through, rather than a dice roll"). A
rung with neither is one of §3.14's active tiers, which #28 brings — it is on the
ladder and out of reach, and saying so is more honest than leaving it off.
*/
type openingView struct {
	Role roleView `json:"role"`
	// Wants is the requirement, veiled.
	Wants []wantView `json:"wants"`
	// Ready is whether this player reaches all of it — including the parts
	// `wants` refuses to describe. It is the one thing the hidden floors are
	// ever allowed to affect on this surface.
	Ready bool `json:"ready"`
	// Walk is where it is taken on, if the player is standing there.
	Walk *whereView `json:"walk"`
	// Take is whether the board takes them into it from here — a walk-in role,
	// or one whose interview this player has already passed (2026-08-08: jobs
	// are applied for on the job surface; standing at the workplace is a second
	// way in, not the gate). Capability is `ready`, deliberately separate: this
	// is whether the door exists, that is whether they fit through it.
	Take bool `json:"take"`
	// Interview is the challenge that hires into it, by code, and only when the
	// player may actually sit it. Sat from the board, wherever they are
	// standing — even today, interviews happen online.
	Interview string `json:"interview"`
}

// heldKey is the role a player is in, by key, and empty for nobody. The one
// posting that is never on the board and never a vacancy they are standing in.
func heldKey(held career.Held) string {
	if role, working := held.Role(); working {
		return role.Key
	}
	return ""
}

// whereView is a spot in the world: a place, and the zone inside it a painting
// would draw the marker in (§10.3). The same shape a challenge's doorway has.
type whereView struct {
	Place string `json:"place"`
	Zone  string `json:"zone"`
}

/*
careerView is the whole job surface for one player.

The rung they hold, the rung above it, the board, and the ladder as a whole.
The last is the piece worth arguing for: §3.14 says "the ladder itself teaches
the trade", and a ladder somebody can only see one rung of teaches nothing. So
every rung is sent, with its pay and its hours, and most of them are things
this player cannot have yet.
*/
type careerView struct {
	// Held is the job, or null.
	Held *roleView `json:"held"`
	// Since is when it was taken, RFC 3339, and empty for nobody.
	Since string `json:"since"`
	// Paid is what a day of it is worth, in cents. Zero for nobody.
	Paid int64 `json:"paid"`
	// Next is the step up, or null at the top of the ladder.
	Next *openingView `json:"next"`
	// Here is a job being taken on where the player is standing, which is not
	// always the step up: somebody who left the dispatch desk and walked back
	// to the warehouse is looking at a rung they have already reached.
	Here *openingView `json:"here"`
	// Board is every role somebody could ever be taken into, as postings
	// (2026-08-08): read the job, read the shift, and act on it from wherever
	// you are standing. The rungs with no way in yet are the ladder's to show,
	// not the board's — a posting nobody can answer is not a posting.
	Board []openingView `json:"board"`
	// Ladder is §3.14's table, lowest rung first.
	Ladder []roleView `json:"ladder"`
}

/*
ladder is the rungs of one population's ladder, in the order they are climbed.

Local rather than global since 2026-08-09. §3.15 makes the work part of what an
environment *is*, so the ladder a player can see is the one belonging to the
people they live among — and moving house changes which trade the ladder is
teaching them, not just who is on the landing.

Composed per response rather than held: it is four short strings a rung, and the
alternative is a cache that goes stale on a deploy.
*/
func ladder(env string) []roleView {
	rungs := career.RolesIn(env)
	out := make([]roleView, 0, len(rungs))
	for _, r := range rungs {
		out = append(out, role(r))
	}
	return out
}

/*
careerSurface reads the whole job surface for one player, standing where they
are.

Everything in it is derived (§3.3): the role and the pay come off the pure core
by tier, the requirement chips come out of internal/factor's veil, and whether
the interview may be sat is a question internal/career answers about the rung
below it. Nothing is stored but the two numbers in the row.
*/
func (s *Server) careerSurface(
	ctx context.Context, tx pgx.Tx, accountID, place, among string,
) (careerView, error) {
	held, err := s.careers.Ensure(ctx, tx, accountID)
	if err != nil {
		return careerView{}, err
	}

	// The work where this player lives (§3.15). A player among nobody has no
	// local ladder, which is a real state of the world and reads as an empty
	// board rather than as an error.
	local := career.RolesIn(among)

	out := careerView{
		Ladder: ladder(among),
		Paid:   held.Pays().Cents(),
		// Never null on the wire: the snapshot is assigned into the client's
		// store unconverted, and a board with nothing on it is an empty list.
		Board: make([]openingView, 0, len(local)),
	}
	if r, working := held.Role(); working {
		view := role(r)
		out.Held = &view
		out.Since = stamp(held.Since)
	}

	if _, above, ok := held.Next(); ok {
		opening, err := s.opening(ctx, tx, accountID, above, place)
		if err != nil {
			return careerView{}, err
		}
		out.Next = &opening
	}

	// What is being taken on where they are standing. Usually the same rung as
	// the step up and deliberately asked separately: a player who left a job
	// can walk back into one they have already reached, which is not a step up
	// at all.
	if here, _, hiring := career.Hiring(place); hiring && here.Key != heldKey(held) {
		opening, err := s.opening(ctx, tx, accountID, here, place)
		if err != nil {
			return careerView{}, err
		}
		out.Here = &opening
	}

	// The board: every role with a way in, as a posting, whether or not the
	// player is anywhere near it. The one they hold is not on it — a job is
	// not a posting to the person doing it.
	for _, r := range local {
		if !r.Hired.Anywhere() && r.Interview == "" {
			continue
		}
		if r.Key == heldKey(held) {
			continue
		}
		opening, err := s.opening(ctx, tx, accountID, r, place)
		if err != nil {
			return careerView{}, err
		}
		// Holding a job is no longer part of this answer (2026-08-08). Nobody
		// resigns into unemployment in order to be allowed to accept an offer,
		// and a board that greyed out while somebody was employed was charging
		// a day's wage for the privilege of switching.
		opening.Take = r.Hires() || held.Reached >= r.Tier
		out.Board = append(out.Board, opening)
	}
	return out, nil
}

// opening composes one rung the player does not hold: its requirement, veiled,
// and the ways in that are actually open to them.
func (s *Server) opening(
	ctx context.Context, tx pgx.Tx, accountID string, r career.Role, place string,
) (openingView, error) {
	out := openingView{Role: role(r)}

	rung, known := simulation.Rungs().At(r.Tier)
	if !known {
		return out, nil
	}

	asked, err := s.factors.Wanting(ctx, tx, accountID, rung.Needs)
	if err != nil {
		return openingView{}, err
	}
	out.Wants = wants(asked)
	if out.Ready, err = s.factors.Ready(ctx, tx, accountID, rung.Needs); err != nil {
		return openingView{}, err
	}

	if zone, there := r.Hired.At(place); there {
		out.Walk = &whereView{Place: place, Zone: zone}
	}
	if r.Interview != "" {
		// Only when they may actually sit it. A code the player cannot use is a
		// door drawn on a wall, and §4's rule about availability is that an
		// entry point that cannot be opened should not be shown.
		sitting, err := s.careers.Sitting(ctx, tx, accountID, r.Interview)
		if err != nil {
			return openingView{}, err
		}
		if sitting {
			out.Interview = r.Interview
		}
	}
	return out, nil
}

/* ── the two commands ─────────────────────────────────────────────────── */

// careerResult is what both commands answer with: the surface, restated, and
// the character — because taking a job changes what a day is worth and the
// screen showing one has to say so without a second fetch.
type careerResult struct {
	Career    careerView    `json:"career"`
	Character characterView `json:"character"`
}

// What a player's other tabs are told when the job changes. One message for
// taking, leaving and being hired out of an interview, because they are one
// event seen three times: the rung moved.
type careerChanged struct {
	Career    careerView    `json:"career"`
	Character characterView `json:"character"`
}

var careerChangedMessage = realtime.Define[careerChanged]("career.changed")

// takeJobInput names a role off the board, or nothing at all. Empty is the
// map's way in: take whatever is being taken on where you are standing.
type takeJobInput struct {
	Role string `json:"role"`
}

/*
Taking a job — off the board by name, or where you are standing with no input.

The board is the door since 2026-08-08: jobs are read and answered on the job
surface, the way people actually get hired, and the named role is a posting
rather than a place. The placeless path only ever hires into roles the board
shows a way into — internal/career refuses the rest — so naming a rung with no
workplace and no interview buys nothing.

Every refusal but one is a 409 rather than a 422: the request is well formed,
and it is the world that is in the way. The exception is a role key the
catalogue has never heard of, which is wrong forever and gets the 422.
*/
func (s *Server) takeJob(ctx context.Context, tx pgx.Tx, actor command.Actor, in takeJobInput) (careerResult, error) {
	who, err := s.characters.Ensure(ctx, tx, actor.ID)
	if err != nil {
		return careerResult{}, err
	}

	today := simulation.Today(s.clk, who.Zone)
	if role := strings.TrimSpace(in.Role); role != "" {
		_, err = s.careers.TakeRole(ctx, tx, actor.ID, role, today)
	} else {
		_, err = s.careers.Take(ctx, tx, actor.ID, where(who), today)
	}
	switch {
	case errors.Is(err, career.ErrNoSuchRole):
		return careerResult{}, command.Invalid("there is no job called %q on the board", in.Role)
	case errors.Is(err, career.ErrNothingHiring):
		return careerResult{}, command.Conflict("nobody is taking anybody on here")
	case errors.Is(err, career.ErrHoldingOne):
		return careerResult{}, command.Conflict("you are already holding a job")
	case errors.Is(err, career.ErrNotHiredYet):
		return careerResult{}, command.Conflict("that one starts with an interview")
	case errors.Is(err, career.ErrNotCapable):
		return careerResult{}, command.Conflict("they will not take you for that yet")
	case err != nil:
		return careerResult{}, err
	}
	return s.careerMoved(ctx, tx, actor, where(who))
}

/*
Giving the job up.

It exists, and internal/career's own comment says why: a run is a committed
judgement that means nothing if it can be walked out of, and a job is a standing
arrangement whose whole point is that it costs something. An arrangement that
cannot be ended is not one.
*/
func (s *Server) leaveJob(ctx context.Context, tx pgx.Tx, actor command.Actor, _ struct{}) (careerResult, error) {
	who, err := s.characters.Ensure(ctx, tx, actor.ID)
	if err != nil {
		return careerResult{}, err
	}

	_, err = s.careers.Leave(ctx, tx, actor.ID)
	switch {
	case errors.Is(err, career.ErrNotHoldingOne):
		return careerResult{}, command.Conflict("you are not holding a job")
	case err != nil:
		return careerResult{}, err
	}
	return s.careerMoved(ctx, tx, actor, where(who))
}

// careerMoved restates the surface, publishes it to the player's other tabs,
// and hands it back to the one that acted (§8, rules 3–4).
func (s *Server) careerMoved(
	ctx context.Context, tx pgx.Tx, actor command.Actor, place string,
) (careerResult, error) {
	among, err := s.among(ctx, tx, actor.ID)
	if err != nil {
		return careerResult{}, err
	}
	view, err := s.careerSurface(ctx, tx, actor.ID, place, among)
	if err != nil {
		return careerResult{}, err
	}
	who, err := s.characters.Of(ctx, tx, actor.ID)
	if err != nil {
		return careerResult{}, err
	}

	out := careerResult{Career: view, Character: s.view(who)}
	if err := careerChangedMessage.Publish(ctx, tx,
		realtime.To{Account: actor.ID, Origin: actor.Session},
		careerChanged(out),
	); err != nil {
		return careerResult{}, err
	}
	return out, nil
}

/*
hired is an interview that finished, applied.

Called from the challenge surface when a run completes, and it is here rather
than inside internal/challenge for the reason §4 gives about the event queue:
the two are "distinct systems, not one abstraction". internal/challenge finishes
runs and has never heard of a job; internal/career hires and has never heard of
a challenge; this line is the whole of what joins them, in the same transaction
as the choice that ended the run.

What crosses is the code and the key of the outcome band. Not the score — that
is not a thing anything outside internal/challenge may read (§3.10) — and not
the reaction, which is the player's business and not a job's.
*/
func (s *Server) hired(
	ctx context.Context, tx pgx.Tx, actor command.Actor, code, outcome string,
) error {
	if _, interview := career.Interviewing(code); !interview {
		return nil
	}
	who, err := s.characters.Ensure(ctx, tx, actor.ID)
	if err != nil {
		return err
	}

	_, took, err := s.careers.Interviewed(
		ctx, tx, actor.ID, code, outcome, simulation.Today(s.clk, who.Zone))
	if err != nil || !took {
		return err
	}
	// The rung moved with nobody asking for it in so many words, so every tab
	// is told — including the one that committed the last option, which is
	// showing the reaction rather than the ladder.
	among, err := s.among(ctx, tx, actor.ID)
	if err != nil {
		return err
	}
	view, err := s.careerSurface(ctx, tx, actor.ID, where(who), among)
	if err != nil {
		return err
	}
	return careerChangedMessage.Publish(ctx, tx,
		realtime.To{Account: actor.ID},
		careerChanged{Career: view, Character: s.view(who)},
	)
}
