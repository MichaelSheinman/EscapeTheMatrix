/*
Package career owns the job a player holds and the work they have done before it.

§3.4 is why the money arrives here rather than on a button: **income is
primarily structural.** It comes from the job held, performance and assets rather
than from repeating actions. The paid-action cap in internal/character is the
same sentence from the other end, and this package is the half that pays.

# There is no ladder (2026-08-15)

§3.14 used to say "jobs sit in tiers", and until this decision a rung number in
the pure core decided what a job paid, how long its shift was and what capability
it was hired against. It is gone. §3.15 — "progression is lateral, not vertical;
the player does not climb, they widen" — is the design's spine, and a single
integer ordering every job in the world was the one mechanic arguing with it.

What replaced it is **[Role.Experience]**: a posting names the work somebody has
to have done to be taken on for it, as routes ([Experience]), any one of which
qualifies. Forty-five days on a dispatch desk, or sixty on a bank counter, or
thirty at a clinic desk together with ninety at a laundromat. Career paths are
now a thing an author draws between two jobs rather than a thing that falls out
of arithmetic, and two jobs paying the same can lead nowhere near each other.

Tenure is cumulative and never lost: [Careers.Leave] costs the wage and nothing
else. A stint table records every period of employment, and days worked are days
worked whether or not they were consecutive.

# The split against internal/simulation

The same one internal/routine keeps. The pure core holds the shapes —
[simulation.Post], what a shift does to a body, and [simulation.Requirement], a
floor on the factor readings. This package holds the *content*: which jobs exist,
what they pay, what they ask, what they are called, and where in the city they
are taken. An authored pass changes files in content/jobs; none of it is a
migration.

# Three seams, and none of them is a table

  - **[Payroll]** is where a day's wage goes. internal/character owns money and
    is the only thing that may add to it — and the method it exposes for this
    deliberately does not touch the paid-action count, because a wage is not an
    action (§3.4).
  - **[Capability]** is whether a player reaches a job's requirement. The
    question is answered inside internal/factor's veil and comes back as a
    boolean: three of the five factors are ones the player is never shown
    (§3.5), so a shortfall crossing this boundary would eventually be rendered.
  - **The interview is a challenge**, and this package has never heard of
    internal/challenge. What it holds is a code and the outcome keys that are an
    offer of the job; the composition root tells it when a run finished and
    what it landed in. §4's line between two systems, kept.

It owns `career` and `career_stint` (§3.1), takes the caller's transaction and
never opens one (§3.2), and reads no other module's tables.
*/
package career

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

// The refusals this module has. Go errors rather than status codes:
// internal/api maps them at the edge (§8), so a module never learns HTTP.
var (
	// ErrNothingHiring is a place where nobody is taking anybody on.
	ErrNothingHiring = errors.New("career: nobody is hiring here")
	// ErrHoldingOne is taking a job while already holding one. The request is
	// fine and the world is in the way, which is a 409 at the edge.
	ErrHoldingOne = errors.New("career: you are already holding a job")
	// ErrNotHoldingOne is leaving a job nobody has.
	ErrNotHoldingOne = errors.New("career: you are not holding a job")
	// ErrNotHiredYet is a role that only takes somebody who has been through its
	// interview, asked for by somebody who has not.
	ErrNotHiredYet = errors.New("career: that is not a job you can walk into")
	// ErrNotCapable is a job whose requirement this player does not reach.
	ErrNotCapable = errors.New("career: they will not take you for that yet")
	// ErrNotExperienced is a job whose authored experience this player has not
	// done — no route through [Role.Experience] is complete. Separate from
	// ErrNotCapable because they are different sentences to a player: one is
	// what you are, the other is what you have done.
	ErrNotExperienced = errors.New("career: you have not done the work that job asks for")
	// ErrNoSuchRole is a job named off the board that the catalogue has never
	// heard of. Wrong forever, unlike the refusals above — the request is
	// malformed, not the world in the way.
	ErrNoSuchRole = errors.New("career: there is no such job")
)

/*
Spot is where a role is taken on: places, and the zones inside them.

The same arrangement §4 requires of an event's placement and internal/challenge
of a challenge's entry point — "placement conditions and world trigger zones are
the same objects" (§10.3). Written out of internal/city's closed vocabulary and
resolved once at init, so a role that hires nowhere is a panic at boot rather
than a door nobody ever finds.
*/
type Spot struct {
	places map[string]string
	order  []string
}

// At is the zone this role is taken on in at a place, and whether it is taken
// on there at all.
func (s Spot) At(place string) (string, bool) {
	zone, ok := s.places[place]
	return zone, ok
}

// Places is everywhere this role is taken on, in the city's own order.
func (s Spot) Places() []string {
	out := make([]string, len(s.order))
	copy(out, s.order)
	return out
}

// Anywhere reports whether this role is taken on anywhere at all.
func (s Spot) Anywhere() bool { return len(s.order) > 0 }

/*
at resolves an authored placement against internal/city.

It panics rather than returning an error, which is the decision internal/city,
internal/event, internal/playbook and internal/challenge all take about their
own content: a role placed nowhere is a job the player can never take, and
finding that out at boot is the only acceptable time to find it out.
*/
func at(places, zones []string) Spot {
	out := Spot{places: map[string]string{}}
	allowed := map[string]bool{}
	for _, z := range zones {
		if !city.Zone(z) {
			panic("career: " + z + " is not a kind of spot")
		}
		allowed[z] = true
	}
	// The city's own order decides which zone is the more specific, exactly as
	// it does for a challenge's entry point.
	for _, p := range city.Places() {
		for _, key := range places {
			if p.Key != key {
				continue
			}
			for _, z := range city.ZonesIn(p.Key) {
				if allowed[z] {
					out.places[p.Key], out.order = z, append(out.order, p.Key)
					break
				}
			}
		}
	}
	if len(out.order) != len(places) {
		panic(fmt.Sprintf("career: %v has none of the zones %v", places, zones))
	}
	return out
}

// Posting is a job advert in the sections a real one has (content/jobs/SPEC.md).
// Pay and hours are deliberately absent from the *prose*: they are fields on the
// role, and the client renders them beside these words rather than inside them.
type Posting struct {
	Intro          string
	Duties         []string
	Qualifications []string
}

/*
Role is one job, as the player meets it: what it is called, what the posting
says, where it is taken on, and what a day of it quietly does to them.

The words are the design (§3.2: nothing is labelled), and [Role.Posting] is
where the whole of that argument now lives. A posting is a realistic document
(the 2026-08-09 decision, content/jobs/SPEC.md): an intro, duties and
qualifications, the way an actual employer writes one. Every posting has the
same three sections, so the structure carries no signal about which line
matters.

What a posting is *for* is [Role.Toll]. Some of these jobs pay well because of
what they are, and the only way a player is ever told so is that the work,
described plainly and without comment, has something in it that a careful reader
would rather not do. The toll reads as ordinary duty and qualification lines.
Nothing marks them. Nothing scores the job. The number moves months later, and
working out which line did it is the game.

The wage, the shift and the requirement are all here, on the job, since
2026-08-15. They used to be read off a rung by tier, which meant two jobs
sharing a number could not disagree about what a day of them was worth or what
it took out of a body — and that was the ladder deciding the content instead of
the other way round.
*/
type Role struct {
	// Environment is which population's working life this is — the directory
	// the file was authored in (§3.15). Not a place: an east-sider takes the
	// night shift on the Strip, and the work they can see is their own.
	Environment string

	Key  string
	Name string
	// Posting is the job as it is advertised: what the work is and what it
	// asks, in the sections a real posting has, with no line of it flagged as
	// the important one.
	Posting Posting
	// ShiftStart is when the working day begins, in minutes from midnight.
	// The end is [Role.Post]'s hours later — authored start, derived finish, so
	// a posting's times can never disagree with what the rollover charges.
	ShiftStart int

	/*
		Pay is what one day of holding this job is worth, in cents.

		Structural income (§3.4): it arrives because the job is held and the day
		happened, and the paid-action cap in internal/character has nothing to
		say about it. There is no path in this codebase by which holding a job
		produces money faster because somebody clicked more.

		A number on the posting rather than on a rung, which is what makes
		[Role.Toll] a choice rather than a trap: two jobs asking the same hours
		may pay differently, and the one that pays more is allowed to have
		something in the duties a careful reader would rather not do.
	*/
	Pay simulation.Money

	// Post is what a day of this job asks of the body: how long, whether it is
	// against the body clock, and whether it leaves the player eating what they
	// meant to eat.
	Post simulation.Post

	// Needs is the capability floor this job is hired against (§3.4's "each
	// step up requires capability"). The zero requirement asks for nothing,
	// which is what an entry-level posting is.
	Needs simulation.Requirement

	/*
		Experience is the work somebody has to have done to be taken on here.

		Any one [Route] qualifies; a route's entries all have to be met. Empty
		is an entry-level posting — a job that takes anybody who can do it —
		and every environment has to offer at least one, or it is an
		environment nobody can start working in.

		This is what a tier used to be, said properly. A rung number claimed
		that every job at a level was interchangeable as a stepping stone;
		routes let the firm principal be reachable only through partner-track
		while the clinic desk and the bank counter open different work
		entirely.
	*/
	Experience Experience

	// Delegated is whether this job runs while the player is elsewhere and
	// hands them situations to answer through the playbook (§3.14's loop).
	//
	// Authored per job rather than implied by seniority, which is the point: a
	// walk-in clinic desk can plausibly be the job that generates the moments
	// and a regional director's can plausibly not. internal/situation checks at
	// boot that every delegated role has work written for it.
	Delegated bool

	/*
		Toll is what one day of holding this role does to the hidden factors.

		Never sent to a client, and this is the only comment that needs to say
		why: a posting whose cost arrives as a number in a JSON payload is a
		posting that is solved with the network tab open, once, by one player,
		for everybody. §3.5 keeps three of the five factors unnamed; this keeps
		the eighth thing unnamed too.

		Small on purpose, against [simulation.CreditHalf] of 240. A week of the
		wrong job should be nothing anybody could point at and a season of it
		should be the reason the board has quietly moved — which is the same
		saturating curve internal/social relies on, read from the other end.
	*/
	Toll []simulation.Grant

	// Hired is where somebody is taken on. A role with nowhere and no interview
	// is a role nobody can take — visible on the board, out of reach.
	Hired Spot

	// Interview is the authored challenge that hires into this role (§3.4:
	// "getting hired spawns an interview challenge the player actually has to
	// get through, rather than a dice roll"). Empty for a role that hires off
	// the street, which is what an entry-level posting is.
	//
	// A code rather than a definition: this package has never heard of
	// internal/challenge, and the composition root is what joins the two.
	Interview string
	// Offered is which outcomes of that interview are an offer of the job.
	//
	// Outcome keys rather than a score, because a score is not a thing anybody
	// outside internal/challenge may read (§3.10's hidden scoring) — and
	// because "which bands hire" is an authored decision about a role rather
	// than a threshold about a challenge.
	Offered []string
}

// Hires reports whether this role can be walked into, or has to be interviewed
// for.
func (r Role) Hires() bool { return r.Interview == "" }

// offers reports whether an outcome of this role's interview is an offer.
func (r Role) offers(outcome string) bool {
	for _, key := range r.Offered {
		if key == outcome {
			return true
		}
	}
	return false
}

/*
Worked is how many days a player has held each job, by role key.

Cumulative and never lost (2026-08-15): leaving a job costs the wage and nothing
else, so two stints of twenty days read as forty here. There is deliberately no
second reading of the same history — no longest run, no "continuous since" —
because a player who quit for a fortnight to deal with something should not
discover that a number they could not see was reset.
*/
type Worked map[string]int

/*
Route is one way to qualify for a job by experience: every entry in it, met.

The `all_of` block of a posting. A route is a map rather than a list so that a
role cannot appear in one twice with two different answers, and [Route.Needs]
walks it in key order so that two processes compose the same sentence (§7).
*/
type Route map[string]int

// Met reports whether this route is complete.
func (r Route) Met(w Worked) bool {
	for role, days := range r {
		if w[role] < days {
			return false
		}
	}
	return true
}

// Short is how many more days each unmet entry of this route wants, in key
// order. Empty for a route already met.
func (r Route) Short(w Worked) []Tenure {
	out := make([]Tenure, 0, len(r))
	for _, role := range sorted(r) {
		if left := r[role] - w[role]; left > 0 {
			out = append(out, Tenure{Role: role, Days: left})
		}
	}
	return out
}

// Needs is every entry of this route, in key order.
func (r Route) Needs() []Tenure {
	out := make([]Tenure, 0, len(r))
	for _, role := range sorted(r) {
		out = append(out, Tenure{Role: role, Days: r[role]})
	}
	return out
}

// Roles is every job this route names, in key order.
func (r Route) Roles() []string { return sorted(r) }

// remaining is how many days of work this route still wants in total — the
// number [Experience.Nearest] ranks by.
func (r Route) remaining(w Worked) int {
	var left int
	for role, days := range r {
		if short := days - w[role]; short > 0 {
			left += short
		}
	}
	return left
}

// Tenure is one job for one number of days: an entry of a route, or what is
// left of one.
type Tenure struct {
	Role string
	Days int
}

/*
Experience is every route into a job, any one of which qualifies.

The `any_of` block of a posting, and the whole of what a tier used to claim.
Empty is an entry-level posting: a job that asks for no previous work, which
every environment has to have at least one of (checked at boot) or it is an
environment nobody can start working in.

Depth stops here, on purpose. A route holds jobs and an experience holds routes,
and neither may hold the other: that keeps this a shape a loader checks with a
switch rather than a boolean grammar an author can nest four deep inside a job
advert. If a posting ever needs more logic than this, that is a design
conversation rather than a file.
*/
type Experience []Route

// Asks reports whether this job wants any previous work at all.
func (e Experience) Asks() bool { return len(e) > 0 }

// Met reports whether any route through this experience is complete.
func (e Experience) Met(w Worked) bool {
	if !e.Asks() {
		return true
	}
	for _, r := range e {
		if r.Met(w) {
			return true
		}
	}
	return false
}

/*
Ordered is every route into a job, nearest first.

All of them rather than only the closest, because the alternatives *are* the
mechanic: a posting offering three ways in and showing one is a posting that has
hidden the choice it was written to present. The player is meant to be able to
read "or forty-five days at the laundromat" and decide that is the shorter road
from where they actually are.

Nearest first is the ranking that makes that readable — fewest days left, ties
broken by the route's first job in key order so that two processes compose the
same list (§7). It is a presentation order and nothing more: every route in it
qualifies, and [Experience.Met] does not care which.
*/
func (e Experience) Ordered(w Worked) []Route {
	out := make([]Route, len(e))
	copy(out, e)
	sort.SliceStable(out, func(i, j int) bool {
		left, right := out[i].remaining(w), out[j].remaining(w)
		if left != right {
			return left < right
		}
		return first(out[i]) < first(out[j])
	})
	return out
}

// Roles is every job named anywhere in this experience, in key order and
// without repeats — what the boot checks resolve and what the acyclicity walk
// follows.
func (e Experience) Roles() []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range e {
		for _, role := range r.Roles() {
			if !seen[role] {
				seen[role], out = true, append(out, role)
			}
		}
	}
	sort.Strings(out)
	return out
}

// first is a route's lowest role key, which is what breaks a tie between two
// routes the same distance away.
func first(r Route) string {
	keys := sorted(r)
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

// sorted is a route's role keys in key order.
func sorted(r Route) []string {
	out := make([]string, 0, len(r))
	for role := range r {
		out = append(out, role)
	}
	sort.Strings(out)
	return out
}

// ByKey finds a role by its catalogue key.
func ByKey(key string) (Role, bool) {
	var found Role
	var ok bool
	every(func(r Role) bool {
		if r.Key == key {
			found, ok = r, true
			return false
		}
		return true
	})
	return found, ok
}

// Hiring is the role somebody is taken on for at a place, if any.
//
// The catalogue's order decides it, which is the same rule internal/api's
// challenge surface uses for two entry points in one spot: deterministically,
// and by the order an author wrote rather than by chance (§7).
func Hiring(place string) (Role, string, bool) {
	var found Role
	var at string
	var ok bool
	every(func(r Role) bool {
		if zone, there := r.Hired.At(place); there {
			found, at, ok = r, zone, true
			return false
		}
		return true
	})
	return found, at, ok
}

// Interviewing is the role an authored challenge is the interview for.
func Interviewing(code string) (Role, bool) {
	if code == "" {
		return Role{}, false
	}
	var found Role
	var ok bool
	every(func(r Role) bool {
		if r.Interview == code {
			found, ok = r, true
			return false
		}
		return true
	})
	return found, ok
}

/*
Payroll is what this package is allowed to do to a player's money.

One method, and it is deliberately not [character.Wallet]: a wage is added and
never spent, and the module that pays one has no business reading a balance.

The method it calls does **not** touch the paid-action count, and that is the
whole of §3.4's "income is structural, not per-action" held as a boundary rather
than as a promise. There is no way to reach the capped path from here.
*/
type Payroll interface {
	Wage(ctx context.Context, tx pgx.Tx, accountID string, amount simulation.Money) error
}

/*
Capability is whether a player reaches a job's requirement (§3.4's "each step up
requires capability").

A [simulation.Requirement] crosses in and a boolean crosses out, and nothing
else may: three of the five factors are never named to the player (§3.5), and a
caller handed the shortfall would eventually render it. internal/factor answers
this inside its own veil, which is the only place it can be answered.
*/
type Capability interface {
	Ready(ctx context.Context, tx pgx.Tx, accountID string, need simulation.Requirement) (bool, error)
}

/*
Credits is what a day of work is allowed to do to the hidden factors.

Grants in, nothing out. The asymmetry is the same one [Capability] has and it is
there for the same reason: this package may say that a night on the corner shop
till moved something, and may not be told what the player's standing now reads,
because a module that could read it could eventually render it (§3.5).

It is the port [Role.Toll] arrives through, and the only one. There is no path
from here to a total.
*/
type Credits interface {
	Record(ctx context.Context, tx pgx.Tx, accountID string, grants ...simulation.Grant) error
}

/*
Held is the job a player has.

One job, named by its key, which since 2026-08-15 is the only name a job has —
the rung it used to sit on is gone, and with it the possibility of a row whose
two columns disagree about what somebody does for a living.

Everything derived is derived here rather than stored (§3.3): what it pays and
what it asks of a body are read off the authored role.
*/
type Held struct {
	AccountID string
	// Key is the job held, by catalogue key, and empty for nobody.
	Key   string
	Since time.Time
	// PaidThrough is the last of the player's own days a wage has been paid
	// for. The zero date is somebody who has never held a job.
	PaidThrough simulation.Date
}

// Working reports whether this player holds a job at all.
func (h Held) Working() bool { return h.Key != "" }

// Role is the role held, and false for nobody.
func (h Held) Role() (Role, bool) {
	if h.Key == "" {
		return Role{}, false
	}
	return ByKey(h.Key)
}

// Post is what the job asks of the body, and the zero post for nobody.
func (h Held) Post() simulation.Post {
	if r, ok := h.Role(); ok {
		return r.Post
	}
	return simulation.Post{}
}

// Pays is what one day of this job is worth, and nothing for nobody.
func (h Held) Pays() simulation.Money {
	if r, ok := h.Role(); ok {
		return r.Pay
	}
	return 0
}

// Careers is the module. It holds no per-player state: the transaction comes
// from the caller, and the clock and the two seams are injected (§3.1).
type Careers struct {
	clk     simulation.Clock
	payroll Payroll
	can     Capability
	credits Credits
}

func New(clk simulation.Clock, payroll Payroll, can Capability, credits Credits) *Careers {
	return &Careers{clk: clk, payroll: payroll, can: can, credits: credits}
}

// Ensure creates the career row if the account has none, and returns it.
//
// Unemployed, which is where everybody starts. Onboarding (#33) may eventually
// put somebody in a job on day one; until then everybody begins holding
// nothing, and that fact lives in one place.
func (m *Careers) Ensure(ctx context.Context, tx pgx.Tx, accountID string) (Held, error) {
	if _, err := tx.Exec(ctx,
		`INSERT INTO career (account_id) VALUES ($1) ON CONFLICT (account_id) DO NOTHING`,
		accountID,
	); err != nil {
		return Held{}, fmt.Errorf("career: ensure: %w", err)
	}
	return m.read(ctx, tx, accountID, false)
}

// Of reads the career.
func (m *Careers) Of(ctx context.Context, tx pgx.Tx, accountID string) (Held, error) {
	return m.read(ctx, tx, accountID, false)
}

// read loads the row, optionally taking the lock a read-modify-write needs.
//
// FOR UPDATE is what makes two commands arriving together correct rather than
// lucky: a player pressing "take it" twice, or a scheduled rollover arriving
// while a catch-up is running, both serialise here.
func (m *Careers) read(ctx context.Context, tx pgx.Tx, accountID string, lock bool) (Held, error) {
	q := `SELECT role, since, paid_through FROM career WHERE account_id = $1`
	if lock {
		q += ` FOR UPDATE`
	}

	var (
		key   *string
		since *time.Time
		paid  *time.Time
	)
	err := tx.QueryRow(ctx, q, accountID).Scan(&key, &since, &paid)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Not an error: an account nobody has ensured yet holds no job, which is
		// exactly what a missing row means. Callers that are about to write go
		// through Ensure, which creates it.
		return Held{AccountID: accountID}, nil
	case err != nil:
		return Held{}, fmt.Errorf("career: read: %w", err)
	}

	out := Held{AccountID: accountID}
	if key != nil {
		out.Key = *key
	}
	if since != nil {
		out.Since = *since
	}
	if paid != nil {
		// A DATE carries no timezone and needs none: it is already the player's
		// own calendar day, written by whoever last paid them.
		out.PaidThrough = simulation.On(*paid, time.UTC)
	}
	return out, nil
}

/*
Take walks somebody into a job where they are standing — the map's way in.
[TakeRole] is the board's, and they refuse for the same reasons.

Three refusals, and each of them is the world rather than the request:

  - **Nobody is hiring here.** Placement is the server's, exactly as an event's
    and a challenge's are — this door is about where the player actually is.
  - **You are already holding one.** Two jobs is two shifts in one day, which
    the pure core has no way to express and no business expressing.
  - **They will not take you.** Either the role is one that has to be
    interviewed for and this player has never held, the experience it asks for
    has not been done, or its capability requirement is not reached.

`today` is the player's own local day, which the caller has because it needs the
character's timezone to know it (§3.3). The wage ledger is stamped to the day
*before* it, so the first day paid for is the first day held — and a fortnight
of catch-up cannot pay for a fortnight of employment that never happened.
*/
func (m *Careers) Take(
	ctx context.Context, tx pgx.Tx, accountID, place string, today simulation.Date,
) (Held, error) {
	role, _, hiring := Hiring(place)
	if !hiring {
		return Held{}, ErrNothingHiring
	}
	return m.take(ctx, tx, accountID, role, today)
}

/*
TakeRole takes a job off the board, by role key (2026-08-08: hiring happens on
the job surface, not by standing somewhere — even today interviews happen
online. The map spot stays as a second way in, not the gate).

The refusals are [Take]'s, with one more in front: a key the catalogue has
never heard of is [ErrNoSuchRole], and a role nothing hires into in V1 — the
jobs that carry no workplace and no interview — is [ErrNothingHiring], because
"nobody is taking anybody on" is as true of a posting as of a place.
*/
func (m *Careers) TakeRole(
	ctx context.Context, tx pgx.Tx, accountID, key string, today simulation.Date,
) (Held, error) {
	role, known := ByKey(key)
	if !known {
		return Held{}, ErrNoSuchRole
	}
	// A role with no way in at all. Hires() alone would say the senior jobs can
	// be walked into, since what those lack is precisely an interview; what
	// makes a role takeable is having somewhere or something that takes you.
	if !role.Hired.Anywhere() && role.Interview == "" {
		return Held{}, ErrNothingHiring
	}
	return m.take(ctx, tx, accountID, role, today)
}

// take is both ways in, once the role is settled: the same three refusals,
// whichever door was used.
func (m *Careers) take(
	ctx context.Context, tx pgx.Tx, accountID string, role Role, today simulation.Date,
) (Held, error) {
	held, err := m.Ensure(ctx, tx, accountID)
	if err != nil {
		return Held{}, err
	}
	// Somebody already working may take a different job (2026-08-08). Quitting
	// first was never a rule about the world — nobody resigns into unemployment
	// in order to be allowed to accept an offer — and as a rule about the screen
	// it was worse than useless: it made the honest move (leave, then apply, and
	// hope the second half works) cost a day of wages that the dishonest one
	// did not. What is refused is taking the job you are standing in, which is
	// not a switch and would only reset the arrangement.
	if held.Working() && held.Key == role.Key {
		return Held{}, ErrHoldingOne
	}
	// A role that hires through an interview may still be walked into by
	// somebody who has already done it — a challenge is once ever (§5.1), so
	// asking them to prove it twice is asking for something the game
	// structurally cannot give. "Has already done it" is now a stint rather
	// than a high-water rung number, which is the same rule said about the
	// actual job instead of about everything at its level.
	if !role.Hires() {
		before, err := m.everHeld(ctx, tx, accountID, role.Key)
		if err != nil {
			return Held{}, err
		}
		if !before {
			return Held{}, ErrNotHiredYet
		}
	}

	if role.Experience.Asks() {
		worked, err := m.Worked(ctx, tx, accountID, today)
		if err != nil {
			return Held{}, err
		}
		if !role.Experience.Met(worked) {
			return Held{}, ErrNotExperienced
		}
	}

	ready, err := m.can.Ready(ctx, tx, accountID, role.Needs)
	if err != nil {
		return Held{}, err
	}
	if !ready {
		return Held{}, ErrNotCapable
	}

	return m.hire(ctx, tx, accountID, held, role, today)
}

/*
Worked is how many days this player has held each job, on their own calendar.

The one query `career_stint` exists for. An open stint counts up to `today`
inclusive of neither end — somebody hired this morning has done nothing yet,
which is the same arithmetic the wage ledger does when [Careers.hire] stamps
`paid_through` to yesterday.

Cumulative, and that is the whole rule: two stints of twenty days are forty days
here. Leaving a job costs the wage and nothing else (2026-08-15), so there is no
branch in this query that could take a day back.
*/
func (m *Careers) Worked(
	ctx context.Context, tx pgx.Tx, accountID string, today simulation.Date,
) (Worked, error) {
	rows, err := tx.Query(ctx, `
		SELECT role, SUM(COALESCE(ended_on, make_date($2, $3, $4)) - started_on)::int
		  FROM career_stint
		 WHERE account_id = $1
		 GROUP BY role`,
		accountID, today.Year, int(today.Month), today.Day,
	)
	if err != nil {
		return nil, fmt.Errorf("career: worked: %w", err)
	}
	defer rows.Close()

	out := Worked{}
	for rows.Next() {
		var role string
		var days int
		if err := rows.Scan(&role, &days); err != nil {
			return nil, fmt.Errorf("career: worked: %w", err)
		}
		if days < 0 {
			// A stint opened by a clock that has since moved backwards — a
			// timezone change, or the backfill's cast. Zero rather than a
			// negative, which nothing downstream is written to expect.
			days = 0
		}
		out[role] = days
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("career: worked: %w", err)
	}
	return out, nil
}

// everHeld reports whether this player has ever held a job, closed stint or
// open. What [Held.Reached] used to approximate with an integer.
func (m *Careers) everHeld(
	ctx context.Context, tx pgx.Tx, accountID, role string,
) (bool, error) {
	var ever bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM career_stint WHERE account_id = $1 AND role = $2)`,
		accountID, role,
	).Scan(&ever); err != nil {
		return false, fmt.Errorf("career: held before: %w", err)
	}
	return ever, nil
}

/*
hire writes the job, the wage ledger, the day it started, and the stint.

The wage ledger is only stamped when somebody had none — a move to another job
they were already holding must not reset it, or the day of the move is paid
twice.

The stint is the half that outlives the job. Whatever they were in is closed as
of today and the new one opened, in that order, because the unique index says
one account has one open stint and the index is what keeps that true rather than
care taken here.
*/
func (m *Careers) hire(
	ctx context.Context, tx pgx.Tx, accountID string, held Held,
	role Role, today simulation.Date,
) (Held, error) {
	if err := m.close(ctx, tx, accountID, today); err != nil {
		return Held{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO career_stint (account_id, role, started_on)
		VALUES ($1, $2, make_date($3, $4, $5))`,
		accountID, role.Key, today.Year, int(today.Month), today.Day,
	); err != nil {
		return Held{}, fmt.Errorf("career: open stint: %w", err)
	}

	from := today.Plus(-1)
	if _, err := tx.Exec(ctx, `
		UPDATE career
		   SET role         = $2,
		       since        = $3,
		       paid_through = COALESCE(paid_through, make_date($4, $5, $6)),
		       updated_at   = now()
		 WHERE account_id = $1`,
		accountID, role.Key, m.clk.Now(),
		from.Year, int(from.Month), from.Day,
	); err != nil {
		return Held{}, fmt.Errorf("career: hire: %w", err)
	}
	return m.read(ctx, tx, accountID, false)
}

// close ends whatever stint is open, as of today. A no-op for somebody holding
// nothing, which is what makes it safe to call on every hire.
func (m *Careers) close(
	ctx context.Context, tx pgx.Tx, accountID string, today simulation.Date,
) error {
	if _, err := tx.Exec(ctx, `
		UPDATE career_stint
		   SET ended_on = GREATEST(started_on, make_date($2, $3, $4))
		 WHERE account_id = $1 AND ended_on IS NULL`,
		accountID, today.Year, int(today.Month), today.Day,
	); err != nil {
		return fmt.Errorf("career: close stint: %w", err)
	}
	return nil
}

/*
Leave gives the job up.

It exists, and the challenge framework's deliberate lack of an abandon command
is not the precedent. A run is a six-minute committed judgement that means
nothing if it can be walked out of; a job is a standing arrangement whose entire
point is that it costs something, and an arrangement you cannot end is not an
arrangement — it is a trap the player made once and can never unmake.

**Leaving costs the wage and nothing else** (2026-08-15). The stint closes and
every day in it stays on the record: a player who worked the dispatch desk for
forty days and walked out has forty days of dispatch experience for the rest of
the game, and can walk back in — the interview was passed, a challenge is once
ever (§5.1), and there is nothing left for them to prove. Tenure that could be
lost would be a punishment for a move the design went out of its way to make
available, and it would be a punishment nothing on the screen had warned about.
*/
func (m *Careers) Leave(
	ctx context.Context, tx pgx.Tx, accountID string, today simulation.Date,
) (Held, error) {
	held, err := m.Ensure(ctx, tx, accountID)
	if err != nil {
		return Held{}, err
	}
	if !held.Working() {
		return Held{}, ErrNotHoldingOne
	}
	if err := m.close(ctx, tx, accountID, today); err != nil {
		return Held{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE career SET role = NULL, since = NULL, updated_at = now() WHERE account_id = $1`,
		accountID,
	); err != nil {
		return Held{}, fmt.Errorf("career: leave: %w", err)
	}
	return m.read(ctx, tx, accountID, false)
}

/*
Sitting reports whether this player may go into an interview.

Two gates used to hold the ladder shut here — the rung's factor floors, then
one-rung-at-a-time — and both were withdrawn on 2026-08-10 because they made a
working pathway invisible: a player who did not qualify saw no button and read
the feature as absent. The factor floors are still withdrawn, and for that exact
reason: three of the five are ones the player is never shown (§3.5), so a door
that will not open because of one is a door with no explanation possible.

**[Role.Experience] is different, and it is the one gate this asks about**
(2026-08-15). A route is entirely legible — it names jobs the player knows, in
days they can count, and the posting states how many of them they have — so a
closed door here is one the screen has already explained. Nothing is hidden by
refusing it.

It gates the interview rather than the hire on purpose. Somebody who sat an
interview and passed it is hired, unconditionally ([Interviewed] asks none of
this): a refusal arriving after six minutes of committed judgement is the worst
possible moment to deliver one, and the honest place to say "not yet" is before
they begin.
*/
func (m *Careers) Sitting(
	ctx context.Context, tx pgx.Tx, accountID, code string, today simulation.Date,
) (bool, error) {
	role, interview := Interviewing(code)
	if !interview || !role.Experience.Asks() {
		return true, nil
	}
	worked, err := m.Worked(ctx, tx, accountID, today)
	if err != nil {
		return false, err
	}
	return role.Experience.Met(worked), nil
}

// Sittable is [Careers.Sitting] for a caller that already holds the tenure map
// — the job surface, which reads it once for the whole board rather than once a
// posting.
func Sittable(worked Worked, code string) bool {
	role, interview := Interviewing(code)
	if !interview {
		return false
	}
	return role.Experience.Met(worked)
}

/*
Interviewed is an interview that has finished, and what it came to.

Called by the composition root when a run completes, because this package has
never heard of internal/challenge and must not (§4: "distinct systems, not one
abstraction"). What crosses is a code and the key of the outcome band the run
landed in — never a score, which is not a thing anything outside
internal/challenge may read (§3.10).

An outcome that is not an offer is not a failure to report. It is the ordinary
case, it is already being told to the player in the reaction the content
authored for it, and there is nothing here to write.
*/
func (m *Careers) Interviewed(
	ctx context.Context, tx pgx.Tx, accountID, code, outcome string, today simulation.Date,
) (Held, bool, error) {
	role, interview := Interviewing(code)
	if !interview || !role.offers(outcome) {
		held, err := m.Of(ctx, tx, accountID)
		return held, false, err
	}

	held, err := m.Ensure(ctx, tx, accountID)
	if err != nil {
		return Held{}, false, err
	}
	if held.Key == role.Key {
		// Already in it. Worth being idempotent about, because the thing on the
		// other end is a challenge run and a run can be reported finished more
		// than once.
		return held, false, nil
	}

	after, err := m.hire(ctx, tx, accountID, held, role, today)
	return after, true, err
}

/*
Shift is one day of holding a job: the day the routine produced as it was
actually lived, and the wage that day was worth.

The rollover's seam (§3.3), and it is one call rather than two because a day at
work is one thing. Both halves are §3.14's:

  - **The wage.** Structural income, paid because the job is held and the day
    happened. It goes through [Payroll], which cannot touch the paid-action cap.
  - **The day.** [simulation.Working] takes the shift out of the hours the
    routine was spending, which is the whole of "it costs you health if your
    routines are bad".

It is idempotent in the date, which it has to be: the rollover walks a calendar
and may walk the same day twice (a retried job, a second tab hydrating, a
scheduled run arriving mid-catch-up). The ledger column is what makes the second
walk pay nothing, and it is read under a lock so two walks arriving together
serialise rather than race.

A player who holds no job gets their own day back, unchanged.
*/
func (m *Careers) Shift(
	ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date, d simulation.Day,
) (simulation.Day, error) {
	held, err := m.read(ctx, tx, accountID, true)
	if err != nil {
		return d, err
	}
	if !held.Working() {
		return d, nil
	}

	if held.PaidThrough.Before(on) {
		if err := m.payroll.Wage(ctx, tx, accountID, held.Pays()); err != nil {
			return d, err
		}
		// The third thing a day of work does, and the only one nobody is told
		// about. It is inside the ledger guard with the wage on purpose: the
		// toll is what the day cost and the wage is what it paid, and a
		// rollover that walked the same date twice must not charge for a
		// morning it has already charged for.
		if role, known := held.Role(); known && len(role.Toll) > 0 {
			if err := m.credits.Record(ctx, tx, accountID, role.Toll...); err != nil {
				return d, err
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE career
			   SET paid_through = make_date($2, $3, $4), updated_at = now()
			 WHERE account_id = $1`,
			accountID, on.Year, int(on.Month), on.Day,
		); err != nil {
			return d, fmt.Errorf("career: pay: %w", err)
		}
	}

	return simulation.Working(d, held.Post()), nil
}
