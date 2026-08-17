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

No factor is named and no floor is a number. A job's requirement carries a floor
on all five factors including the three the player is never shown (§3.5), so the
chips below come out of internal/factor with the hidden ones already stripped:
the two revealed ones carry a name and whether they are cleared, the rest are
blank slots. A chip
reading "stability: 12 / 15" would answer the question the player is playing to
work out.
*/

// payroll answers career.Payroll: where a day's wage goes. An adapter rather
// than the character module, because internal/character has two ways to add
// money and only Wage is right for this — Work is a paid action and spends the
// day's allowance (§3.4).
type payroll struct {
	characters interface {
		Wage(ctx context.Context, tx pgx.Tx, accountID string, amount simulation.Money) error
	}
}

func (p payroll) Wage(ctx context.Context, tx pgx.Tx, accountID string, amount simulation.Money) error {
	return p.characters.Wage(ctx, tx, accountID, amount)
}

// capability answers career.Ready: whether a player reaches a job's
// requirement. One boolean, and the adapter exists to keep it one — the
// comparison happens inside internal/factor, because three of the five factors
// are never named (§3.5).
type capability struct {
	factors *factor.Factors
}

func (c capability) Ready(
	ctx context.Context, tx pgx.Tx, accountID string, need simulation.Requirement,
) (bool, error) {
	return c.factors.Ready(ctx, tx, accountID, need)
}

// tolls answers career.Credits: what a day of a job did to the hidden factors.
// The Recorded internal/factor hands back is dropped, which is the adapter's
// whole job — every other caller of Record is a moment the player lived through
// and is owed an account of, and a day of work is not one.
type tolls struct {
	factors *factor.Factors
}

func (t tolls) Record(
	ctx context.Context, tx pgx.Tx, accountID string, grants ...simulation.Grant,
) error {
	_, err := t.factors.Record(ctx, tx, accountID, grants...)
	return err
}

// employed answers rollover.Employer: the job half of a day. A named type rather
// than the module, because [rollover.Employer] is one method and handing over
// the whole module would hand over hiring.
type employed struct {
	careers *career.Careers
}

func (e employed) Shift(
	ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date, d simulation.Day,
) (simulation.Day, error) {
	return e.careers.Shift(ctx, tx, accountID, on, d)
}

// Compile-time proof that the two seams above are the interfaces they are for.
// This is the line that fails first if either interface grows.
var (
	_ career.Payroll    = payroll{}
	_ career.Capability = capability{}
	_ rollover.Employer = employed{}
)

/* ── what a job looks like from outside ───────────────────────────────── */

// wantView is one thing a job asks for, as the player is allowed to read it.
// state is met, unmet or hidden — @etm/ui's Posting vocabulary. The third is not
// a masked value: no name and no verdict, because a blanked chip that flipped to
// "met" would name what it was measuring at the moment it stopped mattering.
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

// roleView is one job, in the words a player reads. Pay is cents a day and stays
// cents to the screen. Hours and nights are here because they are the honest
// half of what a job costs: a posting showing its salary and not its shift would
// be making half the argument.
type roleView struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Posting is the job as advertised, in the sections a real posting has.
	// Every posting carries the same ones, so the layout says nothing about
	// which line matters (§3.2).
	Posting postingView `json:"posting"`
	// Pay is what one day of holding it is worth, in cents.
	Pay    int64 `json:"pay"`
	Hours  int   `json:"hours"`
	Nights bool  `json:"nights"`
	// Shift is the working day as a posting states it, "21:00–07:00": the
	// authored start plus the job's length, composed here so no client adds
	// hours to a clock.
	Shift string `json:"shift"`
}

// postingView is [career.Posting] on the wire, one field per section.
type postingView struct {
	Intro          string   `json:"intro"`
	Duties         []string `json:"duties"`
	Qualifications []string `json:"qualifications"`
}

// role is a job in the words a player reads. It deliberately omits
// career.Role.Toll: what a job quietly does to somebody is what the posting is
// asking them to work out, and a field on the wire answers that in the network
// tab.
func role(r career.Role) roleView {
	return roleView{
		Key: r.Key, Name: r.Name,
		Posting: postingView{
			Intro:          r.Posting.Intro,
			Duties:         r.Posting.Duties,
			Qualifications: r.Posting.Qualifications,
		},
		Pay:    r.Pay.Cents(),
		Hours:  int(r.Post.Hours.Hours()),
		Nights: r.Post.Nights,
		Shift:  wallClock(r.ShiftStart) + "–" + wallClock(r.ShiftStart+int(r.Post.Hours.Minutes())),
	}
}

// wallClock renders minutes from midnight as a wall clock, wrapping past it
// the way a nights posting does.
func wallClock(minutes int) string {
	minutes %= 24 * 60
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}

// openingView is a job the player does not hold, and what stands between them
// and it. Two ways in, at most one true at once: Walk is a job taken on where
// they are standing, Interview is the authored challenge they have to get
// through (§3.4). A job with neither is on the board and out of reach, which is
// more honest than leaving it off.
type openingView struct {
	Role roleView `json:"role"`
	// Wants is the requirement, with the hidden factors stripped.
	Wants []wantView `json:"wants"`
	// Ready is whether this player reaches all of it, including the parts wants
	// refuses to describe — the one thing the hidden floors affect here.
	Ready bool `json:"ready"`
	// Walk is where it is taken on, if the player is standing there.
	Walk *whereView `json:"walk"`
	// Take is whether the board takes them into it from here: a walk-in role, or
	// one whose interview they have already passed. Separate from ready — this
	// is whether the door exists, that is whether they fit through it.
	Take bool `json:"take"`
	// Interview is the challenge that hires into it, by code, and only when the
	// player may sit it. Sat from the board, wherever they are standing.
	Interview string `json:"interview"`

	// Experience is every route into this job, nearest first, and empty for a
	// posting that asks for none. All of them, because any one qualifies and a
	// posting showing only the closest would hide the choice it exists to
	// present. The order is the server's (§7).
	//
	// Not §3.2's labelling: a real posting states the experience it wants, and
	// this is that sentence with the player's own days subtracted.
	Experience []routeView `json:"experience"`
	// Worked is whether any route is complete. Separate from experience, which
	// is still sent once it is: "45 of 45 days" is worth reading.
	Worked bool `json:"worked"`
}

// tenureView is one job for a number of days: an entry of a route, or what is
// left of one. The name travels with the key so the client does not need a
// second copy of the catalogue to write the sentence.
type tenureView struct {
	Role string `json:"role"`
	Name string `json:"name"`
	Days int    `json:"days"`
}

// routeView is one way into a job: the whole of what it asks, and what of it
// this player still owes.
type routeView struct {
	// Needs is every job in the route, in key order, with the days it wants.
	Needs []tenureView `json:"needs"`
	// Short is the same list with the days already worked taken off, and only
	// the entries still owed. Empty for a route this player has finished.
	Short []tenureView `json:"short"`
}

// tenures names each entry of a route so a client can write the sentence.
func tenures(list []career.Tenure) []tenureView {
	out := make([]tenureView, 0, len(list))
	for _, t := range list {
		view := tenureView{Role: t.Role, Days: t.Days, Name: t.Role}
		if r, known := career.ByKey(t.Role); known {
			view.Name = r.Name
		}
		out = append(out, view)
	}
	return out
}

// heldKey is the role a player is in, by key, and empty for nobody.
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

// careerView is the whole job surface for one player: the job they hold, the
// board, and every job this population has. The last is the piece worth arguing
// for — work only teaches a trade if somebody can see where it goes, so every
// posting is sent with its pay and hours, and most are things this player cannot
// have yet.
type careerView struct {
	// Held is the job, or null.
	Held *roleView `json:"held"`
	// Since is when it was taken, RFC 3339, and empty for nobody.
	Since string `json:"since"`
	// Paid is what a day of it is worth, in cents. Zero for nobody.
	Paid int64 `json:"paid"`
	// Here is a job being taken on where the player is standing.
	Here *openingView `json:"here"`
	// Board is every role somebody could be taken into, as postings: read the
	// job, read the shift, act on it from wherever you are standing. A posting
	// nobody can answer is not a posting.
	Board []openingView `json:"board"`
	// Work is every job this population has, cheapest first, including the ones
	// nothing hires into yet: a board somebody can only see the open half of
	// teaches nothing about where the open half leads.
	Work []roleView `json:"work"`
}

/*
postings is every job one population is offered, cheapest first.

Local rather than global since 2026-08-09. §3.15 makes the work part of what an
environment *is*, so the postings a player can see are the ones belonging to the
people they live among — and moving house changes which trade is on offer, not
just who is on the landing.

Composed per response rather than held: it is four short strings a job, and the
alternative is a cache that goes stale on a deploy.
*/
func postings(env string) []roleView {
	jobs := career.RolesIn(env)
	out := make([]roleView, 0, len(jobs))
	for _, r := range jobs {
		out = append(out, role(r))
	}
	return out
}

/*
careerSurface reads the whole job surface for one player, standing where they
are.

Everything in it is derived (§3.3): the pay and the shift come off the authored
job, the requirement chips come out of internal/factor, and the days
already worked come off the stint table. Nothing is stored but the role in the
row and the stints behind it.
*/
func (s *Server) careerSurface(
	ctx context.Context, tx pgx.Tx, accountID, place, among string, today simulation.Date,
) (careerView, error) {
	held, err := s.careers.Ensure(ctx, tx, accountID)
	if err != nil {
		return careerView{}, err
	}
	// Read once for the whole surface rather than per posting: it is one
	// grouped query, and thirteen of them would be thirteen answers that could
	// disagree if a stint closed between two of them.
	worked, err := s.careers.Worked(ctx, tx, accountID, today)
	if err != nil {
		return careerView{}, err
	}

	// The work where this player lives (§3.15). A player among nobody has no
	// local ladder, which is a real state of the world and reads as an empty
	// board rather than as an error.
	local := career.RolesIn(among)

	out := careerView{
		Work: postings(among),
		Paid: held.Pays().Cents(),
		// Never null on the wire: the snapshot is assigned into the client's
		// store unconverted, and a board with nothing on it is an empty list.
		Board: make([]openingView, 0, len(local)),
	}
	if r, working := held.Role(); working {
		view := role(r)
		out.Held = &view
		out.Since = stamp(held.Since)
	}

	// What is being taken on where they are standing, which the board also
	// carries — asked separately because the map is a second way in and a
	// player standing in front of a vacancy should be told so.
	if here, _, hiring := career.Hiring(place); hiring && here.Key != heldKey(held) {
		opening, err := s.opening(ctx, tx, accountID, here, place, worked)
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
		opening, err := s.opening(ctx, tx, accountID, r, place, worked)
		if err != nil {
			return careerView{}, err
		}
		// Holding a job is no longer part of this answer (2026-08-08). Nobody
		// resigns into unemployment in order to be allowed to accept an offer,
		// and a board that greyed out while somebody was employed was charging
		// a day's wage for the privilege of switching.
		//
		// A job that interviews can still be walked into by somebody who has
		// held it before — a stint on the record rather than a high-water rung
		// number, which is the same rule said about this job instead of about
		// everything that used to share its level.
		opening.Take = r.Hires() || worked[r.Key] > 0 || heldKey(held) == r.Key
		out.Board = append(out.Board, opening)
	}
	return out, nil
}

// opening composes one job the player does not hold: its requirement with the
// hidden factors stripped, the experience it asks for against what they have done, and the ways in that
// are actually open to them.
func (s *Server) opening(
	ctx context.Context, tx pgx.Tx, accountID string, r career.Role, place string,
	worked career.Worked,
) (openingView, error) {
	out := openingView{
		Role:   role(r),
		Worked: r.Experience.Met(worked),
		// Never null on the wire, for the reason the board above says: the world
		// snapshot is assigned into the client's store unconverted, so a nil
		// slice arrives as `null` and the first `.length` on it takes the whole
		// React tree down. A job that asks for no experience has an empty list
		// of routes, which is what it means.
		Experience: []routeView{},
	}

	asked, err := s.factors.Wanting(ctx, tx, accountID, r.Needs)
	if err != nil {
		return openingView{}, err
	}
	out.Wants = wants(asked)
	if out.Ready, err = s.factors.Ready(ctx, tx, accountID, r.Needs); err != nil {
		return openingView{}, err
	}

	for _, route := range r.Experience.Ordered(worked) {
		out.Experience = append(out.Experience, routeView{
			Needs: tenures(route.Needs()),
			Short: tenures(route.Short(worked)),
		})
	}

	if zone, there := r.Hired.At(place); there {
		out.Walk = &whereView{Place: place, Zone: zone}
	}
	// Only when they may actually sit it. A code the player cannot use is a
	// door drawn on a wall, and §4's rule about availability is that an entry
	// point that cannot be opened should not be shown. What closes it is the
	// experience the posting asks for, which the posting also shows — so the
	// button that is not there has a sentence three lines above it saying why.
	if r.Interview != "" && career.Sittable(worked, r.Interview) {
		out.Interview = r.Interview
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

	_, err = s.careers.Leave(ctx, tx, actor.ID, simulation.Today(s.clk, who.Zone))
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
	who, err := s.characters.Of(ctx, tx, actor.ID)
	if err != nil {
		return careerResult{}, err
	}
	view, err := s.careerSurface(ctx, tx, actor.ID, place, among, simulation.Today(s.clk, who.Zone))
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
	// showing the reaction rather than the board.
	among, err := s.among(ctx, tx, actor.ID)
	if err != nil {
		return err
	}
	view, err := s.careerSurface(ctx, tx, actor.ID, where(who), among, simulation.Today(s.clk, who.Zone))
	if err != nil {
		return err
	}
	return careerChangedMessage.Publish(ctx, tx,
		realtime.To{Account: actor.ID},
		careerChanged{Career: view, Character: s.view(who)},
	)
}
