/*
Package career owns the job a player holds and the ladder above it (§3.14).

HighLevelOverview.md §3.14 is the specification:

	**Jobs sit in tiers.** Low tiers are genuinely passive — you hold the job,
	it pays, and it costs you health if your routines are bad. Higher tiers pay
	far more and demand real involvement. Each step up requires capability plus
	an interview (§3.4).

and §3.4 is why the money arrives here rather than on a button: **income is
primarily structural.** It comes from job tier, performance and assets rather
than from repeating actions. The paid-action cap in internal/character is the
same sentence from the other end, and this package is the half that pays.

# The split against internal/simulation

The same one internal/routine keeps, and for the same reason. The pure core
holds the mechanics — what a rung pays, what its shift does to a body, and what
capability it is hired against, all tunable by a unit test (§17's "job tier
thresholds"). This package holds the *content*: which roles exist, what they are
called, what they ask of somebody in words, and where in the city they are
taken. A tuning pass changes constants in the core; an authored pass changes
sentences here; neither is a migration.

# Three seams, and none of them is a table

  - **[Payroll]** is where a day's wage goes. internal/character owns money and
    is the only thing that may add to it — and the method it exposes for this
    deliberately does not touch the paid-action count, because a wage is not an
    action (§3.4).
  - **[Capability]** is whether a player reaches a rung's requirement. The
    question is answered inside internal/factor's veil and comes back as a
    boolean: three of the five factors are ones the player is never shown
    (§3.5), so a shortfall crossing this boundary would eventually be rendered.
  - **The interview is a challenge**, and this package has never heard of
    internal/challenge. What it holds is a code and the outcome keys that are an
    offer of the job; the composition root tells it when a run finished and
    what it landed in. §4's line between two systems, kept.

It owns `career` (§3.1), takes the caller's transaction and never opens one
(§3.2), and reads no other module's tables.
*/
package career

import (
	"context"
	"errors"
	"fmt"
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
	// interview, asked for by somebody who has not (§3.14).
	ErrNotHiredYet = errors.New("career: that is not a job you can walk into")
	// ErrNotCapable is a rung whose requirement this player does not reach.
	ErrNotCapable = errors.New("career: they will not take you for that yet")
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

/*
Role is one job, as the player meets it: what it is called, what it is, what it
costs them, and where it is taken on.

The words are the design (§3.2: nothing is labelled). `Costs` is the honest half
— what the job asks of somebody — because the game states the price and leaves
the verdict, exactly as a routine option does.

What is not here is a number. The pay, the hours and the requirement are the
pure core's, read off [simulation.Rungs] by tier, so the sentence a player reads
and the arithmetic the rollover does cannot come apart.
*/
type Role struct {
	Tier simulation.Tier
	Key  string
	Name string
	// Does is the job, in the words somebody in it would use.
	Does string
	// Costs is what holding it asks of them.
	Costs string

	// Hired is where somebody is taken on. A role with nowhere is a role
	// nobody in V1 can take — see the tiers §3.14 calls active, which are #28's.
	Hired Spot

	// Interview is the authored challenge that hires into this role (§3.4:
	// "getting hired spawns an interview challenge the player actually has to
	// get through, rather than a dice roll"). Empty for a role that hires off
	// the street, which is what the bottom of a ladder is.
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
The roles §3.14 tabulates, in the order they are climbed.

Five, because the design's table has five, and the top two are named the way
§3.14 names them — "politician, founder" is a life rather than a post, and the
role below it is the last one that still has an office in it.

**Only the two passive rungs can be taken in V1**, and the three above them
carry no `Hired` and no `Interview` on purpose. §3.14 splits the ladder in half:
low tiers "pay without being played", which is this ticket, and higher ones
"generate interactions while the player is elsewhere", which is the playbook
delegation in #28. A rung that could be taken before the thing it does exists
would be a job that pays a salary for nothing, so they stand on the ladder,
visible and out of reach, and #28 gives them a way in.
*/
var (
	Nights = Role{
		Tier: 1,
		Key:  "warehouse-nights",
		Name: "Warehouse nights",
		Does: "Ten hours on the loading bay from ten at night, moving what the day shift left. " +
			"Cash on Friday. Nobody asks you anything and nobody expects anything.",
		Costs: "The night, every night. It is the easiest job in this city to get and the " +
			"hardest one to still be doing in a year.",
		// The loading bay and not the warehouse beside it. One place hires one
		// role — see TestNoPlaceHiresTwoRoles — and the geography says the same
		// thing the rule does: the night shift is on the bay, and the desk that
		// hires for the job above it is inside.
		Hired: at([]string{"the-strip-loading-bay"}, []string{"dock"}),
	}

	Dispatch = Role{
		Tier: 2,
		Key:  "dispatch",
		Name: "The dispatch desk",
		Does: "Nine hours in the daylight with a phone, a board, and forty drivers who all " +
			"think theirs is the one that matters.",
		Costs: "Somebody else's emergency, all day. You will be asked to decide things " +
			"nobody told you about, and told afterwards what you should have done.",
		Hired:     at([]string{"the-strip-warehouse"}, []string{"desk"}),
		Interview: "interview/a",
		Offered:   []string{"offered", "start-monday"},
	}

	Operations = Role{
		Tier: 3,
		Key:  "operations",
		Name: "Operations manager",
		Does: "The floor, the roster and the budget, and the two of the three that are " +
			"always wrong at once.",
		Costs: "It follows you home. The decisions are yours whether or not you were there " +
			"when they had to be made.",
	}

	Regional = Role{
		Tier: 4,
		Key:  "regional",
		Name: "Regional director",
		Does: "Four sites, none of which you can be at, and a number at the end of the " +
			"quarter that is the only thing anybody will remember.",
		Costs: "Consequences that take months to arrive, from choices you will not remember making.",
	}

	Principal = Role{
		Tier:  5,
		Key:   "principal",
		Name:  "Your own name on the door",
		Does:  "Whatever you decide it is, which turns out to be the hard part.",
		Costs: "Everything else. There is no version of this that is nine hours.",
	}
)

// Roles is every role, lowest rung first.
var Roles = []Role{Nights, Dispatch, Operations, Regional, Principal}

// Of finds the role at a tier. The false is [simulation.Unemployed], which is
// the absence of a role rather than one.
func Of(tier simulation.Tier) (Role, bool) {
	for _, r := range Roles {
		if r.Tier == tier {
			return r, true
		}
	}
	return Role{}, false
}

// Hiring is the role somebody is taken on for at a place, if any.
//
// The catalogue's order decides it, which is the same rule internal/api's
// challenge surface uses for two entry points in one spot: deterministically,
// and by the order an author wrote rather than by chance (§7).
func Hiring(place string) (Role, string, bool) {
	for _, r := range Roles {
		if zone, there := r.Hired.At(place); there {
			return r, zone, true
		}
	}
	return Role{}, "", false
}

// Interviewing is the role an authored challenge is the interview for.
func Interviewing(code string) (Role, bool) {
	if code == "" {
		return Role{}, false
	}
	for _, r := range Roles {
		if r.Interview == code {
			return r, true
		}
	}
	return Role{}, false
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
Capability is whether a player reaches a rung's requirement (§3.14's "each step
up requires capability").

A [simulation.Requirement] crosses in and a boolean crosses out, and nothing
else may: three of the five factors are never named to the player (§3.5), and a
caller handed the shortfall would eventually render it. internal/factor answers
this inside its own veil, which is the only place it can be answered.
*/
type Capability interface {
	Ready(ctx context.Context, tx pgx.Tx, accountID string, need simulation.Requirement) (bool, error)
}

/*
Held is the job a player has, and the ladder as it stands for them.

Everything derived is derived here rather than stored (§3.3): the role, the
rung, what it pays, and what the step up would be are all read off the pure core
and the two numbers in the row.
*/
type Held struct {
	AccountID string
	// Tier is the rung held, and [simulation.Unemployed] for nobody.
	Tier simulation.Tier
	// Reached is the highest rung ever held — what an interview already passed
	// bought, and what leaving a job does not take back.
	Reached simulation.Tier
	Since   time.Time
	// PaidThrough is the last of the player's own days a wage has been paid
	// for. The zero date is somebody who has never held a job.
	PaidThrough simulation.Date
}

// Working reports whether this player holds a job at all.
func (h Held) Working() bool { return h.Tier.Held() }

// Role is the role held, and false for nobody.
func (h Held) Role() (Role, bool) { return Of(h.Tier) }

// Rung is the rung held, and false for nobody.
func (h Held) Rung() (simulation.Rung, bool) { return simulation.Rungs().At(h.Tier) }

// Post is what the job asks of the body, and the zero post for nobody.
func (h Held) Post() simulation.Post { return simulation.Rungs().Post(h.Tier) }

// Pays is what one day of this job is worth.
func (h Held) Pays() simulation.Money {
	if r, ok := h.Rung(); ok {
		return r.Pay
	}
	return 0
}

// Next is the rung above the one held, and false at the top of the ladder.
func (h Held) Next() (simulation.Rung, Role, bool) {
	rung, ok := simulation.Rungs().Above(h.Tier)
	if !ok {
		return simulation.Rung{}, Role{}, false
	}
	role, ok := Of(rung.Tier)
	return rung, role, ok
}

// Careers is the module. It holds no per-player state: the transaction comes
// from the caller, and the clock and the two seams are injected (§3.1).
type Careers struct {
	clk     simulation.Clock
	payroll Payroll
	can     Capability
}

func New(clk simulation.Clock, payroll Payroll, can Capability) *Careers {
	return &Careers{clk: clk, payroll: payroll, can: can}
}

// Ensure creates the career row if the account has none, and returns it.
//
// Unemployed, which is where the ladder starts. Onboarding (#33) may eventually
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
	q := `SELECT tier, reached, since, paid_through FROM career WHERE account_id = $1`
	if lock {
		q += ` FOR UPDATE`
	}

	var (
		tier, reached int
		since         *time.Time
		paid          *time.Time
	)
	err := tx.QueryRow(ctx, q, accountID).Scan(&tier, &reached, &since, &paid)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Not an error: an account nobody has ensured yet holds no job, which is
		// exactly what a missing row means. Callers that are about to write go
		// through Ensure, which creates it.
		return Held{AccountID: accountID}, nil
	case err != nil:
		return Held{}, fmt.Errorf("career: read: %w", err)
	}

	out := Held{
		AccountID: accountID,
		Tier:      simulation.Tier(tier),
		Reached:   simulation.Tier(reached),
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
Take walks somebody into a job.

Three refusals, and each of them is the world rather than the request:

  - **Nobody is hiring here.** Placement is the server's, exactly as an event's
    and a challenge's are. A client that could take a job from anywhere would
    have turned §3.12's walking-is-the-encounter-surface into a menu.
  - **You are already holding one.** Two jobs is two shifts in one day, which
    the pure core has no way to express and no business expressing.
  - **They will not take you.** Either the role is one that has to be
    interviewed for and this player never has been (§3.14), or the rung's
    capability requirement is not reached.

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

	held, err := m.Ensure(ctx, tx, accountID)
	if err != nil {
		return Held{}, err
	}
	if held.Working() {
		return Held{}, ErrHoldingOne
	}
	// A role that hires through an interview may still be walked into by
	// somebody who has already been through it — a challenge is once ever
	// (§5.1), so asking them to prove it twice is asking for something the game
	// structurally cannot give.
	if !role.Hires() && held.Reached < role.Tier {
		return Held{}, ErrNotHiredYet
	}

	rung, known := simulation.Rungs().At(role.Tier)
	if !known {
		return Held{}, fmt.Errorf("career: %s is tier %d, which is not on the ladder", role.Key, role.Tier)
	}
	ready, err := m.can.Ready(ctx, tx, accountID, rung.Needs)
	if err != nil {
		return Held{}, err
	}
	if !ready {
		return Held{}, ErrNotCapable
	}

	return m.hire(ctx, tx, accountID, held, role.Tier, today)
}

/*
hire writes the rung, the ledger, and the day it started.

The wage ledger is only stamped when somebody had none — a promotion off a job
they were already holding must not reset it, or the day of the promotion is paid
twice.
*/
func (m *Careers) hire(
	ctx context.Context, tx pgx.Tx, accountID string, held Held,
	tier simulation.Tier, today simulation.Date,
) (Held, error) {
	reached := held.Reached
	if tier > reached {
		reached = tier
	}

	from := today.Plus(-1)
	if _, err := tx.Exec(ctx, `
		UPDATE career
		   SET tier         = $2,
		       reached      = $3,
		       since        = $4,
		       paid_through = COALESCE(paid_through, make_date($5, $6, $7)),
		       updated_at   = now()
		 WHERE account_id = $1`,
		accountID, int(tier), int(reached), m.clk.Now(),
		from.Year, int(from.Month), from.Day,
	); err != nil {
		return Held{}, fmt.Errorf("career: hire: %w", err)
	}
	return m.read(ctx, tx, accountID, false)
}

/*
Leave gives the job up.

It exists, and the challenge framework's deliberate lack of an abandon command
is not the precedent. A run is a six-minute committed judgement that means
nothing if it can be walked out of; a job is a standing arrangement whose entire
point is that it costs something, and an arrangement you cannot end is not an
arrangement — it is a trap the player made once and can never unmake. §3.14's
ladder only teaches its trade if walking back down it is a real option.

What is not given up is [Held.Reached]. The interview was passed; a player who
leaves the dispatch desk can walk back into it, because a challenge is once ever
(§5.1) and there is nothing left for them to prove.
*/
func (m *Careers) Leave(ctx context.Context, tx pgx.Tx, accountID string) (Held, error) {
	held, err := m.Ensure(ctx, tx, accountID)
	if err != nil {
		return Held{}, err
	}
	if !held.Working() {
		return Held{}, ErrNotHoldingOne
	}
	if _, err := tx.Exec(ctx, `
		UPDATE career SET tier = 0, since = NULL, updated_at = now() WHERE account_id = $1`,
		accountID,
	); err != nil {
		return Held{}, fmt.Errorf("career: leave: %w", err)
	}
	return m.read(ctx, tx, accountID, false)
}

/*
Sitting reports whether this player may go into an interview.

The other half of §4's availability rule, applied to a doorway this package owns
the conditions of rather than internal/challenge: an interview standing in the
warehouse is not a thing anybody may walk into, it is a thing somebody who
already does the job below it and reaches the rung's requirement may walk into.

A challenge that is not an interview is always sittable as far as this package
is concerned, which is what lets the caller ask about every doorway rather than
having to know which ones are jobs.
*/
func (m *Careers) Sitting(ctx context.Context, tx pgx.Tx, accountID, code string) (bool, error) {
	role, interview := Interviewing(code)
	if !interview {
		return true, nil
	}

	held, err := m.Of(ctx, tx, accountID)
	if err != nil {
		return false, err
	}
	// One rung at a time. The step up is from where they actually stand, which
	// is what makes the bottom of the ladder somewhere they have to leave
	// rather than somewhere they can skip.
	if held.Tier != role.Tier-1 {
		return false, nil
	}
	rung, known := simulation.Rungs().At(role.Tier)
	if !known {
		return false, nil
	}
	return m.can.Ready(ctx, tx, accountID, rung.Needs)
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
	if held.Tier >= role.Tier {
		// Already there. Unreachable through the doorway, which Sitting gates
		// on standing exactly one rung below — and worth being idempotent about
		// anyway, because the thing on the other end is a challenge run.
		return held, false, nil
	}

	after, err := m.hire(ctx, tx, accountID, held, role.Tier, today)
	return after, true, err
}

/*
Shift is one day of holding a job: the day the routine produced as it was
actually lived, and the wage that day was worth.

The rollover's seam (§3.3), and it is one call rather than two because a day at
work is one thing. Both halves are §3.14's:

  - **The wage.** Structural income, paid because the tier is held and the day
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
