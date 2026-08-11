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
// Pay and hours are deliberately absent: they are the pure core's by tier, and
// the client renders them beside these words.
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

What is not here is a wage. The pay, the hours and the requirement are the pure
core's, read off [simulation.Rungs] by tier, so the sentence a player reads and
the arithmetic the rollover does cannot come apart.
*/
type Role struct {
	// Environment is which population's working life this is — the directory
	// the file was authored in (§3.15). Not a place: an east-sider takes the
	// night shift on the Strip, and the ladder they can see is their own.
	Environment string

	Tier simulation.Tier
	Key  string
	Name string
	// Posting is the job as it is advertised: what the work is and what it
	// asks, in the sections a real posting has, with no line of it flagged as
	// the important one.
	Posting Posting
	// ShiftStart is when the working day begins, in minutes from midnight.
	// The end is the rung's hours later — authored start, derived finish, so a
	// posting's times can never disagree with what the rollover charges.
	ShiftStart int

	/*
		Premium is what this role pays over its rung, in cents a day.

		The rung is the ladder's arithmetic — §3.14's "more money costs more of
		your attention" — and it is a fact about a tier. This is a fact about a
		job: two roles on the same rung ask for the same nine or ten hours, and
		one of them hands over more than the other. Zero for most of them.

		It is the other half of [Role.Toll] and the reason that mechanic is a
		choice rather than a trap. A worse job that paid the same would simply
		be the wrong answer, and a game whose wrong answers are free is a game
		with nothing in it to weigh.
	*/
	Premium simulation.Money

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

// Of finds the first-listed role at a tier. The false is
// [simulation.Unemployed], which is the absence of a role rather than one.
//
// Since the job board (2026-08-08) a rung can carry more than one role, and a
// caller that knows which one it means goes through [ByKey]; this is for the
// ones that only hold a tier — a career row from before the role column, or a
// situation naming the rung it arose on.
func Of(tier simulation.Tier) (Role, bool) {
	var found Role
	var ok bool
	every(func(r Role) bool {
		if r.Tier == tier {
			found, ok = r, true
			return false
		}
		return true
	})
	return found, ok
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
Held is the job a player has, and the ladder as it stands for them.

Everything derived is derived here rather than stored (§3.3): the role, the
rung, what it pays, and what the step up would be are all read off the pure core
and the two numbers in the row.
*/
type Held struct {
	AccountID string
	// Tier is the rung held, and [simulation.Unemployed] for nobody.
	Tier simulation.Tier
	// Key is which role the rung is held in, since a rung can carry more than
	// one (the warehouse and the corner shop are both tier 1). Empty for
	// nobody, and for a row written before the role column existed — which
	// [Held.Role] resolves to the first role of the tier, exactly what those
	// rows meant when they were written.
	Key string
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
func (h Held) Role() (Role, bool) {
	if h.Key != "" {
		if r, ok := ByKey(h.Key); ok {
			return r, true
		}
	}
	return Of(h.Tier)
}

// Rung is the rung held, and false for nobody.
func (h Held) Rung() (simulation.Rung, bool) { return simulation.Rungs().At(h.Tier) }

// Post is what the job asks of the body, and the zero post for nobody.
func (h Held) Post() simulation.Post { return simulation.Rungs().Post(h.Tier) }

// Pays is what one day of this job is worth: the rung's wage, and whatever this
// particular role hands over on top of it (see [Role.Premium]).
func (h Held) Pays() simulation.Money {
	r, ok := h.Rung()
	if !ok {
		return 0
	}
	if role, ok := h.Role(); ok {
		return r.Pay + role.Premium
	}
	return r.Pay
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
	credits Credits
}

func New(clk simulation.Clock, payroll Payroll, can Capability, credits Credits) *Careers {
	return &Careers{clk: clk, payroll: payroll, can: can, credits: credits}
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
	q := `SELECT tier, reached, role, since, paid_through FROM career WHERE account_id = $1`
	if lock {
		q += ` FOR UPDATE`
	}

	var (
		tier, reached int
		key           *string
		since         *time.Time
		paid          *time.Time
	)
	err := tx.QueryRow(ctx, q, accountID).Scan(&tier, &reached, &key, &since, &paid)
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
	return m.take(ctx, tx, accountID, role, today)
}

/*
TakeRole takes a job off the board, by role key (2026-08-08: hiring happens on
the job surface, not by standing somewhere — even today interviews happen
online. The map spot stays as a second way in, not the gate).

The refusals are [Take]'s, with one more in front: a key the catalogue has
never heard of is [ErrNoSuchRole], and a role nothing hires into in V1 — the
rungs that carry no workplace and no interview — is [ErrNothingHiring], because
"nobody is taking anybody on" is as true of a posting as of a place.
*/
func (m *Careers) TakeRole(
	ctx context.Context, tx pgx.Tx, accountID, key string, today simulation.Date,
) (Held, error) {
	role, known := ByKey(key)
	if !known {
		return Held{}, ErrNoSuchRole
	}
	// A role with no way in at all. Hires() alone would say the top of the
	// ladder can be walked into, since what those rungs lack is precisely an
	// interview; what makes a role takeable is having somewhere or something
	// that takes you.
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

	return m.hire(ctx, tx, accountID, held, role, today)
}

/*
hire writes the rung, the ledger, and the day it started.

The wage ledger is only stamped when somebody had none — a promotion off a job
they were already holding must not reset it, or the day of the promotion is paid
twice.
*/
func (m *Careers) hire(
	ctx context.Context, tx pgx.Tx, accountID string, held Held,
	role Role, today simulation.Date,
) (Held, error) {
	reached := held.Reached
	if role.Tier > reached {
		reached = role.Tier
	}

	from := today.Plus(-1)
	if _, err := tx.Exec(ctx, `
		UPDATE career
		   SET tier         = $2,
		       reached      = $3,
		       role         = $4,
		       since        = $5,
		       paid_through = COALESCE(paid_through, make_date($6, $7, $8)),
		       updated_at   = now()
		 WHERE account_id = $1`,
		accountID, int(role.Tier), int(reached), role.Key, m.clk.Now(),
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
		UPDATE career SET tier = 0, role = NULL, since = NULL, updated_at = now() WHERE account_id = $1`,
		accountID,
	); err != nil {
		return Held{}, fmt.Errorf("career: leave: %w", err)
	}
	return m.read(ctx, tx, accountID, false)
}

/*
Sitting reports whether this player may go into an interview.

The interview is the gate, and the only gate (2026-08-10). This used to hold
§3.14's ladder shut twice over — the rung's factor floors, then one-rung-at-a-time
— and both checks made a working pathway invisible: a player who did not qualify
saw no button and read the feature as absent. Until locking earns its place
(V3/V4, when there is enough game for a gate to be worth hitting), every
interview is sittable by anybody and what decides the hire is the interview
itself. [Interviewed] still refuses to demote: a passing outcome below the tier
already held writes nothing.
*/
func (m *Careers) Sitting(ctx context.Context, tx pgx.Tx, accountID, code string) (bool, error) {
	return true, nil
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

	after, err := m.hire(ctx, tx, accountID, held, role, today)
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
