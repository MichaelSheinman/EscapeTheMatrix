/*
Package career owns the job a player holds and the work they have done before it.
Income is structural (§3.4) — it comes from the job held rather than from
repeating actions, and internal/character's paid-action cap is the same rule from
the other end.

There is no ladder. Progression is lateral (§3.15), so what qualifies somebody
for a posting is [Role.Experience]: routes of previous work, any one of which
qualifies — forty-five days on a dispatch desk, or sixty on a bank counter, or
thirty at a clinic desk together with ninety at a laundromat. Career paths are
drawn by an author between two jobs rather than falling out of arithmetic, and
two jobs paying the same can lead nowhere near each other.

Tenure is cumulative and never lost: [Careers.Leave] costs the wage and nothing
else, and career_stint records every period of employment.

The split against internal/simulation is internal/routine's. The pure core holds
the shapes — [simulation.Post] and [simulation.Requirement] — and this package
holds the content: which jobs exist, what they pay, what they ask, and where in
the city they are taken. An authored pass changes files in content/jobs.

Three seams, none of them a table. [Payroll] is where a day's wage goes, and the
method it exposes does not touch the paid-action count because a wage is not an
action. [Capability] answers whether a player reaches a requirement as a boolean, inside
internal/factor, since three of the five factors are never shown (§3.5). And the
interview is a challenge this package has never heard of: it holds a code and the
outcome keys that are an offer, and the composition root tells it how a run
finished.

It owns career and career_stint (§3.1), takes the caller's transaction and never
opens one (§3.2), and reads no other module's tables.
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

// Spot is where a role is taken on: places, and the zones inside them — the
// same objects an event's placement and a challenge's entry point use (§10.3).
// Written out of internal/city's closed vocabulary and resolved once at init, so
// a role that hires nowhere is a panic at boot rather than a door nobody finds.
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

// at resolves an authored placement against internal/city. It panics rather
// than returning an error, as every other content loader here does: a role
// placed nowhere is a job the player can never take, and boot is the only
// acceptable time to find that out.
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
// Pay and hours live on the role instead, and the client renders them beside
// these words.
type Posting struct {
	Intro          string
	Duties         []string
	Qualifications []string
}

/*
Role is one job, as the player meets it: what it is called, what the posting
says, where it is taken on, and what a day of it quietly does to them.

The words are the design — nothing is labelled (§3.2). A posting is a realistic
document (content/jobs/SPEC.md): intro, duties and qualifications, the way an
employer writes one. Every posting has the same three sections, so the structure
carries no signal about which line matters.

What a posting is for is [Role.Toll]. Some jobs pay well because of what they
are, and the only way a player is told so is that the work, described plainly,
has something in it a careful reader would rather not do. Nothing marks those
lines. The number moves months later, and working out which line did it is the
game.
*/
type Role struct {
	// Environment is which population's working life this is — the directory
	// the file was authored in (§3.15). Not a place: an east-sider takes the
	// night shift on the Strip, and the work they can see is their own.
	Environment string

	Key  string
	Name string
	// Posting is the job as advertised: what the work is and what it asks, in
	// the sections a real posting has, every line reading like every other.
	Posting Posting
	// ShiftStart is when the working day begins, in minutes from midnight.
	// The end is [Role.Post]'s hours later — authored start, derived finish, so
	// a posting's times can never disagree with what the rollover charges.
	ShiftStart int

	// Pay is what one day of holding this job is worth, in cents. Structural
	// income (§3.4): it arrives because the job is held and the day happened,
	// and no path in this codebase makes it arrive faster because somebody
	// clicked more. It sits on the posting rather than on a rung, which is what
	// makes [Role.Toll] a choice — two jobs asking the same hours may pay
	// differently.
	Pay simulation.Money

	// Post is what a day of this job asks of the body: how long, whether it is
	// against the body clock, and whether it leaves the player eating what they
	// meant to eat.
	Post simulation.Post

	// Needs is the capability floor this job is hired against (§3.4's "each
	// step up requires capability"). The zero requirement asks for nothing,
	// which is what an entry-level posting is.
	Needs simulation.Requirement

	// Experience is the work somebody has to have done to be taken on here. Any
	// one [Route] qualifies, and a route's entries all have to be met. Empty is
	// an entry-level posting, and every environment has to offer at least one or
	// it is an environment nobody can start working in.
	Experience Experience

	// Delegated is whether this job runs while the player is elsewhere and hands
	// them situations to answer through the playbook (§3.14). Authored per job
	// rather than implied by seniority: a walk-in clinic desk can plausibly
	// generate the moments and a regional director's can plausibly not.
	// internal/situation checks at boot that every delegated role has work.
	Delegated bool

	// Toll is what one day of holding this role does to the hidden factors, and
	// is never sent to a client: a posting whose cost arrives as a number in a
	// JSON payload is solved once, with the network tab open, for everybody.
	//
	// Small on purpose against [simulation.CreditHalf] of 240. A week of the
	// wrong job should be nothing anybody could point at, and a season of it
	// should be why the board has quietly moved.
	Toll []simulation.Grant

	// Hired is where somebody is taken on. A role with nowhere and no interview
	// is a role nobody can take — visible on the board, out of reach.
	Hired Spot

	// Interview is the authored challenge that hires into this role — getting
	// hired is a challenge rather than a dice roll (§3.4). Empty for a role that
	// hires off the street. A code rather than a definition: this package has
	// never heard of internal/challenge, and the composition root joins the two.
	Interview string
	// Offered is which outcomes of that interview are an offer of the job.
	// Outcome keys rather than a score, since a score is not readable outside
	// internal/challenge (§3.10), and which bands hire is an authored decision
	// about a role.
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

// Route is one way to qualify for a job by experience: every entry in it, met —
// the all_of block of a posting. A map rather than a list, so a role cannot
// appear twice with two different answers, and [Route.Needs] walks it in key
// order so two processes compose the same sentence (§7).
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
Experience is every route into a job, any one of which qualifies — the any_of
block of a posting. Empty is an entry-level posting, which every environment has
to have at least one of (checked at boot).

Depth stops here on purpose: a route holds jobs and an experience holds routes,
and neither may hold the other. That keeps this a shape a loader checks with a
switch rather than a boolean grammar nested four deep inside a job advert.
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
Ordered is every route into a job, nearest first — all of them rather than only
the closest, because the alternatives are the mechanic. A posting offering three
ways in and showing one has hidden the choice it was written to present.

Fewest days left, ties broken by the route's first job in key order so two
processes compose the same list (§7). A presentation order and nothing more:
every route in it qualifies, and [Experience.Met] does not care which.
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

// Hiring is the role somebody is taken on for at a place, if any. The
// catalogue's order decides it, so two roles hiring in one spot resolve by the
// order an author wrote rather than by chance (§7).
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

// Payroll is what this package is allowed to do to a player's money: add a
// wage, and nothing else. Deliberately not character.Wallet — the module that
// pays has no business reading a balance. The method it calls does not touch the
// paid-action count, so §3.4's structural income is a boundary rather than a
// promise.
type Payroll interface {
	Wage(ctx context.Context, tx pgx.Tx, accountID string, amount simulation.Money) error
}

// Capability is whether a player reaches a job's requirement (§3.4). A
// [simulation.Requirement] crosses in and a boolean crosses out: three of the
// five factors are never named to the player (§3.5), so a caller handed the
// shortfall would eventually render it. internal/factor answers it without
// letting the numbers out.
type Capability interface {
	Ready(ctx context.Context, tx pgx.Tx, accountID string, need simulation.Requirement) (bool, error)
}

// Credits is what a day of work is allowed to do to the hidden factors: grants
// in, nothing out. This package may say a night on the corner shop till moved
// something and may not be told what the standing now reads (§3.5). It is the
// only port [Role.Toll] arrives through, and there is no path from here to a
// total.
type Credits interface {
	Record(ctx context.Context, tx pgx.Tx, accountID string, grants ...simulation.Grant) error
}

// Held is the job a player has, named by its catalogue key. Everything derived
// is derived rather than stored (§3.3): what it pays and what it asks of a body
// are read off the authored role.
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
// Everybody starts unemployed.
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

// read loads the row, optionally taking the lock a read-modify-write needs. FOR
// UPDATE is what serialises two commands arriving together — a player pressing
// "take it" twice, or a rollover arriving while a catch-up is running.
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
		// Not an error: a missing row is an account nobody has ensured yet, and
		// it holds no job. Callers about to write go through Ensure.
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
[TakeRole] is the board's, and they refuse for the same reasons: nobody is hiring
here, you are already holding this one, or they will not take you (never
interviewed, experience not done, or capability not reached).

today is the player's own local day, which the caller has because it needs the
character's timezone (§3.3). The wage ledger is stamped to the day before it, so
the first day paid for is the first day held and a fortnight of catch-up cannot
pay for employment that never happened.
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

// TakeRole takes a job off the board, by role key — hiring happens on the job
// surface, and the map spot is a second way in rather than the gate. The
// refusals are [Careers.Take]'s with one more in front: an unknown key is
// [ErrNoSuchRole], and a role with no workplace and no interview is
// [ErrNothingHiring].
func (m *Careers) TakeRole(
	ctx context.Context, tx pgx.Tx, accountID, key string, today simulation.Date,
) (Held, error) {
	role, known := ByKey(key)
	if !known {
		return Held{}, ErrNoSuchRole
	}
	// A role with no way in at all. Hires() alone would say the senior jobs can
	// be walked into, since what those lack is the interview; what makes a role
	// takeable is having somewhere or something that takes you.
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
	// Somebody already working may take a different job: nobody resigns into
	// unemployment in order to be allowed to accept an offer. What is refused is
	// taking the job you already hold, which would only reset the arrangement.
	if held.Working() && held.Key == role.Key {
		return Held{}, ErrHoldingOne
	}
	// A role that hires through an interview may still be walked into by
	// somebody who has held it before — a challenge is once ever (§5.1), so
	// asking them to prove it twice asks for something the game cannot give.
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

// Worked is how many days this player has held each job, on their own calendar
// — the one query career_stint exists for. An open stint counts up to today
// exclusive of both ends, so somebody hired this morning has done nothing yet,
// matching what [Careers.hire] stamps on paid_through. Cumulative: two stints of
// twenty days are forty days, and nothing here can take a day back.
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
			// A stint opened by a clock that has since moved backwards, such as
			// a timezone change. Nothing downstream expects a negative.
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
// open.
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

The wage ledger is only stamped when somebody had none, or the day of a move
between jobs is paid twice.

The stint is the half that outlives the job. Whatever they were in is closed as
of today and the new one opened, in that order, because the unique index — one
open stint per account — is what keeps that true.
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
Leave gives the job up. A job is a standing arrangement and a player has to be
able to end one, which is why this exists where a challenge run's abandon command
does not.

Leaving costs the wage and nothing else. The stint closes and every day in it
stays on the record, so somebody who worked the dispatch desk for forty days and
walked out has forty days of dispatch experience for the rest of the game — and
can walk back in, since the interview was passed and a challenge is once ever
(§5.1).
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

[Role.Experience] is the one gate it asks about. A route is entirely legible — it
names jobs the player knows, in days they can count, and the posting states how
many of them they have — so a closed door here is one the screen has already
explained. Factor floors are deliberately not asked: three of the five are never
shown (§3.5), so a door that will not open because of one has no explanation
possible.

It gates the interview rather than the hire. Somebody who sat an interview and
passed it is hired unconditionally ([Careers.Interviewed] asks none of this): the
honest place to say "not yet" is before six minutes of committed judgement, not
after.
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
Interviewed is an interview that has finished, and what it came to. Called by the
composition root when a run completes, because this package has never heard of
internal/challenge (§4). What crosses is a code and the key of the outcome band —
never a score, which nothing outside internal/challenge may read (§3.10).

An outcome that is not an offer is the ordinary case rather than a failure to
report: the player is already being told in the reaction the content authored.
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
		// Already in it. Idempotent because a challenge run can be reported
		// finished more than once.
		return held, false, nil
	}

	after, err := m.hire(ctx, tx, accountID, held, role, today)
	return after, true, err
}

/*
Shift is one day of holding a job: the day the routine produced as it was
actually lived, and the wage that day was worth. One call rather than two,
because a day at work is one thing. The wage goes through [Payroll], which cannot
touch the paid-action cap; [simulation.Working] takes the shift out of the hours
the routine was spending.

It is idempotent in the date, because the rollover may walk the same day twice.
The ledger column makes the second walk pay nothing, and it is read under a lock
so two walks serialise rather than race.

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
		// about. Inside the ledger guard with the wage: a rollover that walked
		// the same date twice must not charge for a morning twice either.
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
