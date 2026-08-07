/*
Package event owns §4's backlog: the things that could happen to a player, and
everything that may become of one.

technical.md §4 is the whole specification and it is unusually prescriptive.
**The world maintains a backlog** — events are generated ahead of demand and sit
in a queue, so there is always something to do. **Release is a server decision,
not a clock event.** **Events chain**: responding to one immediately queues its
successor. And the lifecycle is written out:

	generated → eligible → offered → (declined | accepted) → resolved

# The two rules that shape the schema

**Declining does not consume** (§3.9). Walking past an argument returns it to
the backlog rather than burning it, "otherwise players learn that avoiding
content destroys it". So there is no code path in this package that deletes a
backlog row, and refusing one puts it back — with a count incremented, because
a refusal is worth remembering even though it costs nothing.

**Revalidation at release.** A backlogged event is re-checked against current
state before it is surfaced, and silently dropped or requeued if its
preconditions no longer hold. That is why eligibility is computed at the moment
of release and stored nowhere: a stored eligibility is a stale one, and the
whole point is that a fortnight may pass between an event being generated and
anybody standing where it happens.

# What this package refuses to know

Four things, each behind an interface (§3.1):

  - **What the player is holding** — [Purse], because a precondition on money is
    the one precondition §4's authored events need and reading somebody else's
    table is not how this codebase gets it.
  - **Who is coming back to this** — [Bearings]: how long they were away, and
    which factors they are behind on. §4 decides release from both and neither
    is in this schema.
  - **When to surface something** — [Policy]. It runs *after* placement and
    revalidation, so a policy cannot place an event anywhere; the worst it can
    say is "not now".
  - **Whether a moment had to escalate** — internal/playbook's, entirely. What
    reaches this package is [Events.Raise], which is a decision already made.
  - **What an accepted event does** — nothing here. An event that opens a
    challenge (#23), a job situation (#28) or a conflict (#30) is marked
    `awaits` in content, and the module that runs the thing calls [Events.Resolve]
    when it knows how it went. §4 is explicit that the event queue and the
    challenge catalogue are distinct systems rather than one abstraction, and
    this is where that line is.

It owns `event_backlog` and `event_record`, takes the caller's transaction and
never opens one (§3.2), and reads no other module's tables.
*/
package event

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
The knobs. Constants rather than configuration, for the reason every other
module's are: a server running a different backlog depth would be a different
game, and the boards those populations share would not mean the same thing.
*/
const (
	// Depth is how many unfinished events the world keeps ready for one player.
	//
	// §4's "generated ahead of demand… so there is always something to do", as
	// a number. Deep enough that a walk across the city has candidates for most
	// of the spots it passes, shallow enough that the backlog is not a stockpile
	// of things the player will meet in three weeks' time and that revalidation
	// will mostly throw away.
	Depth = 6

	// Window is how long an offer stands before it lapses.
	//
	// Short, because an offer is a moment rather than an errand: the person at
	// the door is at the door now. Lapsing costs nothing — the event goes back
	// to the backlog like every other way of not answering it (§3.9).
	Window = 45 * time.Minute

	// Rest is how long an event waits after the player did not take it.
	//
	// Declining does not consume, but it does not mean nothing either: the same
	// encounter offered again on the next step of the same journey would be a
	// refusal the world ignored, which reads worse than losing the event
	// altogether.
	Rest = 6 * time.Hour

	// Requeue is how long an event waits after failing revalidation.
	//
	// §4's "silently dropped or requeued". A precondition about money can
	// become true again in an afternoon, so the event is requeued rather than
	// dropped — and it waits, so a player standing at the barrier with an empty
	// pocket is not re-checked on every step they take past it.
	Requeue = 12 * time.Hour

	// LogPage is how many records one page of the history carries.
	LogPage = 20
)

var (
	// ErrNoOffer is an answer to something that is not on screen: the wrong id,
	// somebody else's, one already answered, or one whose window has passed.
	// Deliberately one error — telling them apart would tell a client probing
	// ids something about offers that were never theirs.
	ErrNoOffer = errors.New("event: there is nothing open by that name")

	// ErrNotOpen is settling something that was not accepted, or was settled
	// already. It is a programming error in the module that called it rather
	// than anything a player did, which is why it is separate.
	ErrNotOpen = errors.New("event: that is not an accepted event awaiting an outcome")

	// ErrNotRaised is [Events.Raise] called with something that is not an
	// authored escalation. Also a programming error: raising an ordinary event
	// would put it past the backlog's depth, past the daily cap and past the
	// cooling, which is exactly the exemption escalation exists to be and
	// exactly what it must not become a way of buying.
	ErrNotRaised = errors.New("event: that is not an event the world raises")
)

/*
Purse is what this package may know about the player's money.

The narrowest interface that answers the one question §4's preconditions ask.
internal/api binds it to the same adapter internal/factor's Purse gets, which
ensures the character row exists first — a player whose first wake failed must
not take the release path down with them.
*/
type Purse interface {
	Balance(ctx context.Context, tx pgx.Tx, accountID string) (simulation.Money, error)
}

// Broke is a Purse with nothing in it. For tests about placement rather than
// about preconditions; never bound in the server.
type Broke struct{}

func (Broke) Balance(context.Context, pgx.Tx, string) (simulation.Money, error) { return 0, nil }

/*
Bearings is what this package may know about the player that is not in its own
tables and is not money: how long they were away, and what they are behind on.

The two facts §4 names that the backlog cannot answer for itself. They are one
interface rather than two because they are one question — "who is this, coming
back to this" — and because a module that grew a fifth constructor argument for
every sentence in a design document would end up with the design document as its
constructor.

Neither method returns a number the player could be shown. [Bearings.Lagging]
returns factors and no scores, which is internal/factor's boundary honoured on
this side of it: a release heuristic that could read a Karma would be a release
heuristic somebody eventually rendered.
*/
type Bearings interface {
	// Away is how long the player had been away when this sitting began. Zero
	// for somebody who has not been anywhere.
	Away(ctx context.Context, tx pgx.Tx, accountID string) (time.Duration, error)
	// Lagging is which of the player's factors sit below their own average,
	// worst first.
	Lagging(ctx context.Context, tx pgx.Tx, accountID string) ([]simulation.Factor, error)
}

// Here is a Bearings for a player who has been here all along and is behind on
// nothing. For tests about placement rather than about the policy; never bound
// in the server.
type Here struct{}

func (Here) Away(context.Context, pgx.Tx, string) (time.Duration, error) { return 0, nil }

func (Here) Lagging(context.Context, pgx.Tx, string) ([]simulation.Factor, error) {
	return nil, nil
}

/*
Offer is one event, put to one player, in one spot.

The id is what an answer names. It is this player's offer or it is not an offer
at all — every read of one is keyed by (id, account) — so handing a bigint to a
browser discloses nothing.
*/
type Offer struct {
	ID    int64
	Event Event
	// Place and Zone are where it was put, out of internal/city. The zone is
	// what a painting draws the marker in (§10.3), and it is chosen from the
	// event's placement rules intersected with what that place actually has.
	Place string
	Zone  string

	OfferedAt time.Time
	ExpiresAt time.Time
}

// Standing is the whole of the event surface for one player.
//
// One field, deliberately. The backlog's depth is the world's business and not
// the player's: a number saying how many things are queued up would turn "there
// is always something to do" into a counter to grind down, which is the exact
// reading of §4 the sandbox is meant to avoid.
type Standing struct {
	// Offer is nil most of the time, which is the design rather than a gap.
	Offer *Offer
}

// Answer is what became of one offer.
type Answer string

const (
	// Accepted is the player taking it up.
	Accepted Answer = "accepted"
	// Declined is the player pressing the other button. The event goes back to
	// the backlog (§3.9).
	Declined Answer = "declined"
	// Left is the player walking away without answering. Also back to the
	// backlog — and kept apart from Declined because one of them is a decision
	// and the other is not, and a release policy that could not tell them apart
	// would read inattention as refusal.
	Left Answer = "left"
	// Lapsed is the window passing with nobody looking.
	Lapsed Answer = "lapsed"
	// Dropped is the world giving up on an event at the moment it would have
	// been surfaced: out of patience, or authored out of the game since.
	Dropped Answer = "dropped"
)

/*
Answered is what one answer did.

`Next` is §4's chain, stated rather than implied: the successor is queued inside
the same transaction as the answer, so an event that was responded to and a
backlog that does not know it is not a state this game has.
*/
type Answered struct {
	Event  Event
	Place  string
	Zone   string
	Answer Answer
	// Next is the successor queued by this answer, or empty. Only an accepted
	// event chains — a declined one is still in the backlog, and queueing what
	// comes after something that has not happened yet would be a chain that
	// overtook itself.
	Next string
	// Open is whether the event is still running: accepted, and waiting on the
	// module that owns whatever it opened (§4's `resolved`).
	Open     bool
	Standing Standing
}

// Record is one thing that happened, as the history keeps it.
type Record struct {
	ID   int64
	Key  string
	Name string
	Kind Kind
	At   time.Time
	// Place and Zone as they were, by key. A place withdrawn from the city
	// leaves its name in the record rather than an empty line.
	Place string
	Zone  string

	Answer Answer
	// Resolved is when an accepted event finished, and zero while it is still
	// running.
	Resolved time.Time
}

// Log is one page of the history, newest first.
type Log struct {
	Records []Record
	Page    int
	Pages   int
	Total   int
}

/*
Tally is how one family of events has been going: §4's signal, counted.

Offered and the three answers rather than a ratio, because "walked away from
four of four" and "walked away from four of forty" are different sentences and
only one of them means the player is routing around something.
*/
type Tally struct {
	Kind     Kind
	Offered  int
	Accepted int
	Declined int
	// Passed is left plus lapsed: not answered, rather than answered no.
	Passed int
}

// Events is the module. It holds no per-player state.
type Events struct {
	clk      simulation.Clock
	purse    Purse
	bearings Bearings
	policy   Policy
}

// New binds the module to the clock it reads and the three things it cannot
// reach otherwise.
func New(clk simulation.Clock, purse Purse, bearings Bearings, policy Policy) *Events {
	return &Events{clk: clk, purse: purse, bearings: bearings, policy: policy}
}

/* ── reading ──────────────────────────────────────────────────────────── */

// Of reads the surface without changing it: what a command's response restates
// once it has done its work. It never releases anything and never lapses
// anything, so calling it is free of consequence.
func (e *Events) Of(ctx context.Context, tx pgx.Tx, accountID string) (Standing, error) {
	return e.standing(ctx, tx, accountID, e.clk.Now())
}

func (e *Events) standing(ctx context.Context, tx pgx.Tx, accountID string, now time.Time) (Standing, error) {
	var (
		out   Offer
		key   string
		place string
		zone  string
	)
	err := tx.QueryRow(ctx, `
		SELECT id, event, place, zone, offered_at, expires_at
		  FROM event_backlog
		 WHERE account_id = $1 AND state = 'offered' AND expires_at > $2`,
		accountID, now,
	).Scan(&out.ID, &key, &place, &zone, &out.OfferedAt, &out.ExpiresAt)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Standing{}, nil
	case err != nil:
		return Standing{}, fmt.Errorf("event: read the offer: %w", err)
	}

	authored, ok := Known(key)
	if !ok {
		// Authored out of the game while it was on screen. There is nothing to
		// show and nothing to answer; the next release drops the row.
		return Standing{}, nil
	}
	out.Event, out.Place, out.Zone = authored, place, zone
	return Standing{Offer: &out}, nil
}

/* ── generating ───────────────────────────────────────────────────────── */

/*
Replenish tops the backlog up: §4's "generated ahead of demand".

Called by the rollover once per run (§3.3) and, as a floor, by the release path
when it finds a player with nothing waiting at all. The second case is not a
short cut round the first — a backlog that is empty at the moment somebody walks
into a doorway is a world with nothing in it, and there is no reading of "there
is always something to do" that waits until tomorrow to fix that.

Selection is deterministic (§7): what the player has seen least, then what the
author weighted heaviest, then the catalogue's own order. No randomness anywhere
in this package, and nothing here reads a factor — "what would serve the player
next" is the release policy's question (#22) and it is asked at release, over
events that are already in the backlog.
*/
func (e *Events) Replenish(ctx context.Context, tx pgx.Tx, accountID string, _ simulation.Date) error {
	return e.fill(ctx, tx, accountID, e.clk.Now())
}

/*
floor is the one generation the release path does for itself.

Only when the backlog is *empty*, and that bound is the design rather than an
optimisation. Topping up to [Depth] on every arrival would make the rollover's
replenish decorative and would mean a player who walks a lot has a permanently
full backlog while one who does not has a permanently stale one. Filling an
empty one is different in kind: a world with nothing in it is not a world, and
there is no reading of "there is always something to do" that waits until
tomorrow to fix that.
*/
func (e *Events) floor(ctx context.Context, tx pgx.Tx, accountID string, now time.Time) error {
	var unfinished int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM event_backlog
		 WHERE account_id = $1 AND state <> 'done'`, accountID).Scan(&unfinished); err != nil {
		return fmt.Errorf("event: measure the backlog: %w", err)
	}
	if unfinished > 0 {
		return nil
	}
	return e.fill(ctx, tx, accountID, now)
}

func (e *Events) fill(ctx context.Context, tx pgx.Tx, accountID string, now time.Time) error {
	var waiting int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM event_backlog
		 WHERE account_id = $1 AND state <> 'done'`, accountID).Scan(&waiting); err != nil {
		return fmt.Errorf("event: measure the backlog: %w", err)
	}
	room := Depth - waiting
	if room <= 0 {
		return nil
	}

	// What is already in the backlog, so the same thing is not queued twice.
	// The partial unique index would refuse it anyway; asking first is what
	// keeps that index a backstop rather than the mechanism.
	held := map[string]bool{}
	rows, err := tx.Query(ctx, `
		SELECT event FROM event_backlog WHERE account_id = $1 AND state <> 'done'`, accountID)
	if err != nil {
		return fmt.Errorf("event: read the backlog: %w", err)
	}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return fmt.Errorf("event: read the backlog: %w", err)
		}
		held[key] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("event: read the backlog: %w", err)
	}

	// When each event was last finished, which is what a cooldown is measured
	// from. An event with no cooldown authored is finished forever.
	done := map[string]time.Time{}
	rows, err = tx.Query(ctx, `
		SELECT event, max(settled_at) FROM event_backlog
		 WHERE account_id = $1 AND state = 'done' GROUP BY event`, accountID)
	if err != nil {
		return fmt.Errorf("event: read what is finished: %w", err)
	}
	for rows.Next() {
		var (
			key string
			at  time.Time
		)
		if err := rows.Scan(&key, &at); err != nil {
			rows.Close()
			return fmt.Errorf("event: read what is finished: %w", err)
		}
		done[key] = at
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("event: read what is finished: %w", err)
	}

	seen, err := e.seen(ctx, tx, accountID)
	if err != nil {
		return err
	}

	// All() is already heaviest-first and then by key, so this is a stable sort
	// on one key: how little the player has seen it.
	candidates := make([]Event, 0, len(events))
	for _, candidate := range All() {
		switch {
		case candidate.Chained:
			// Only ever put there by its parent (§4's chain). Generating one
			// spontaneously would be the second half of a conversation
			// arriving first.
			continue
		case candidate.Raised:
			// Only ever put there by [Events.Raise] (§4's escalation). The
			// world generating one would be the game inventing a conflict the
			// playbook never had an answer to, which is the whole of what
			// escalating means, invented.
			continue
		case held[candidate.Key]:
			continue
		}
		if at, finished := done[candidate.Key]; finished {
			if candidate.Again == 0 || now.Before(at.Add(candidate.Again)) {
				continue
			}
		}
		candidates = append(candidates, candidate)
	}
	stableBy(candidates, func(a, b Event) bool { return seen[a.Key] < seen[b.Key] })

	for _, candidate := range candidates {
		if room == 0 {
			break
		}
		queued, err := e.queue(ctx, tx, accountID, candidate.Key, now, now)
		if err != nil {
			return err
		}
		if queued {
			room--
		}
	}
	return nil
}

// queue puts one event into the backlog, and reports whether it went in.
//
// ON CONFLICT DO NOTHING against the partial unique index, so an event already
// waiting — put there by a chain, or by another tab a moment ago — is not an
// error. It is the answer.
func (e *Events) queue(ctx context.Context, tx pgx.Tx, accountID, key string, at, eligible time.Time) (bool, error) {
	tag, err := tx.Exec(ctx, `
		INSERT INTO event_backlog (account_id, event, state, generated_at, eligible_at)
		VALUES ($1, $2, 'waiting', $3, $4)
		ON CONFLICT DO NOTHING`, accountID, key, at, eligible)
	if err != nil {
		return false, fmt.Errorf("event: generate %s: %w", key, err)
	}
	return tag.RowsAffected() == 1, nil
}

/*
Raise puts an escalation into the backlog: something another module decided has
to reach this player.

technical.md §4's escalation rule — "escalate on three conditions, not one: no
rule matches, two rules conflict, or a rule fired and the outcome was poor.
Escalating only on 'no match' lets one catch-all rule silence the game forever."
Deciding *whether* is internal/playbook's, and it is deliberately not this
package's business; what happens next is: the moment becomes something waiting
in the world, and it is surfaced by the same release path, the same placement
rules and the same revalidation as everything else.

Three exemptions, and each of them is the point rather than a shortcut.

  - **Past the depth.** [Depth] caps what the world *generates*; an escalation
    is not generated, and a full backlog is not a reason for a conflict the
    player's rules could not handle to go unmentioned.
  - **Past the daily cap and the cooling** — see [Attentive]. A cap that could
    swallow an escalation would re-introduce the catch-all rule problem one
    layer down: write one rule that matches everything, never escalate on "no
    match", and let the release policy quietly eat the other two conditions.
  - **Not past the placement rules, the offer window, or one thing at a time.**
    It is still an authored event that happens somewhere, and §4's revalidation
    still applies to it.

It reports whether it went in. False is the honest answer to raising something
already waiting: the same conflict, raised twice, is one conflict.
*/
func (e *Events) Raise(ctx context.Context, tx pgx.Tx, accountID, key string) (bool, error) {
	authored, known := Known(key)
	if !known || !authored.Raised {
		return false, fmt.Errorf("%w: %q", ErrNotRaised, key)
	}
	now := e.clk.Now()
	return e.queue(ctx, tx, accountID, key, now, now)
}

// seen is how many times each event has been put to this player, ever.
func (e *Events) seen(ctx context.Context, tx pgx.Tx, accountID string) (map[string]int, error) {
	out := map[string]int{}
	rows, err := tx.Query(ctx, `
		SELECT event, count(*) FROM event_record WHERE account_id = $1 GROUP BY event`, accountID)
	if err != nil {
		return nil, fmt.Errorf("event: count what has been seen: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			key string
			n   int
		)
		if err := rows.Scan(&key, &n); err != nil {
			return nil, fmt.Errorf("event: count what has been seen: %w", err)
		}
		out[key] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("event: count what has been seen: %w", err)
	}
	return out, nil
}

/* ── releasing ────────────────────────────────────────────────────────── */

/*
Arrived is the walk's half of §3.12: the player is standing somewhere new.

Three things happen, in this order, and each of them is a rule from §4:

 1. An offer whose window has passed lapses, and an offer pinned to a place the
    player has left is walked past. **Both go back to the backlog** — that is
    §3.9's "declining does not consume", applied to the two ways of not
    answering that involve no button at all.
 2. If the backlog is empty, it is filled — see [Events.floor]. A world with
    nothing in it is not a world.
 3. The first waiting event this place allows, whose preconditions still hold,
    is offered — if the release policy says now (#22).

The offer it returns is nil most of the time, which is the design: §4's sandbox
"depends on not everything arriving as a push", and most steps of most journeys
are somebody walking down a street.
*/
func (e *Events) Arrived(ctx context.Context, tx pgx.Tx, accountID, place string) (*Offer, error) {
	return e.release(ctx, tx, accountID, place, func(Surface) bool { return true })
}

/*
Looked is the queue surface's half: the player opened the game (§4's pushed
delivery).

Only events authored `surface: queue` are released here. That is §4's table
taken literally — the world/sandbox surface is "encounters found by traversal",
and an encounter that found *you* while you were reading your own biomarkers
would be the sandbox arriving as a push.

Placement still applies. A queue event is released at the place the player is
standing in, against the same authored rules, so being pushed about is not an
exemption from being placed sensibly.
*/
func (e *Events) Looked(ctx context.Context, tx pgx.Tx, accountID, place string) (Standing, bool, error) {
	offer, err := e.release(ctx, tx, accountID, place, func(s Surface) bool { return s == InQueue })
	if err != nil {
		return Standing{}, false, err
	}
	out, err := e.standing(ctx, tx, accountID, e.clk.Now())
	return out, offer != nil, err
}

func (e *Events) release(ctx context.Context, tx pgx.Tx, accountID, place string, allow func(Surface) bool) (*Offer, error) {
	now := e.clk.Now()

	if err := e.lapse(ctx, tx, accountID, now); err != nil {
		return nil, err
	}
	if err := e.walkedPast(ctx, tx, accountID, place, now); err != nil {
		return nil, err
	}

	// One thing on screen at a time, decided here rather than by the policy.
	// A policy is allowed to be wrong about *when*; it is not allowed to be
	// wrong about this, and the unique index is the third line of the same
	// defence.
	open, err := e.standing(ctx, tx, accountID, now)
	if err != nil {
		return nil, err
	}
	if open.Offer != nil {
		return nil, nil
	}

	if err := e.floor(ctx, tx, accountID, now); err != nil {
		return nil, err
	}

	candidate, authored, zone, err := e.candidate(ctx, tx, accountID, place, now, allow)
	if err != nil || candidate == 0 {
		return nil, err
	}

	attention, err := e.attention(ctx, tx, accountID, authored, now)
	if err != nil {
		return nil, err
	}
	if !e.policy.Release(attention) {
		// Left exactly as it was. Not requeued: the world is not saying this
		// event is wrong for here, only that it is not the moment — and moving
		// its eligibility would be the policy quietly editing the backlog.
		return nil, nil
	}

	return e.offer(ctx, tx, accountID, candidate, authored, place, zone, now)
}

/*
candidate is §4's revalidation: the first waiting event this place allows and
current state still permits.

The three outcomes for a row it looks at are the three §4 names.

  - **Dropped** — the event has been authored out of the game, or it has been
    waiting longer than its patience. It is finished, recorded as dropped, and
    the player is never told: "silently dropped" is the specification's own
    word.
  - **Requeued** — its preconditions no longer hold. It waits [Requeue] and is
    reconsidered, because a bound on money can be false this afternoon and true
    tomorrow.
  - **Offered** — everything holds. The caller asks the policy and then places
    it.

A row this place simply does not allow is none of those: it is left alone, not
requeued, because there is nothing wrong with it and nothing has changed.
*/
func (e *Events) candidate(
	ctx context.Context, tx pgx.Tx, accountID, place string, now time.Time, allow func(Surface) bool,
) (int64, Event, string, error) {
	type waiting struct {
		id        int64
		key       string
		generated time.Time
	}

	rows, err := tx.Query(ctx, `
		SELECT id, event, generated_at
		  FROM event_backlog
		 WHERE account_id = $1 AND state = 'waiting' AND eligible_at <= $2
		 ORDER BY eligible_at, id`, accountID, now)
	if err != nil {
		return 0, Event{}, "", fmt.Errorf("event: read the backlog: %w", err)
	}
	var queue []waiting
	for rows.Next() {
		var row waiting
		if err := rows.Scan(&row.id, &row.key, &row.generated); err != nil {
			rows.Close()
			return 0, Event{}, "", fmt.Errorf("event: read the backlog: %w", err)
		}
		queue = append(queue, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, Event{}, "", fmt.Errorf("event: read the backlog: %w", err)
	}

	// Read once, and only if something actually asks. Most events have no
	// preconditions at all, and a query per step of every journey to answer a
	// question nobody asked is the kind of cost that gets a boundary blamed.
	var (
		circumstances = Circumstances{Place: place}
		asked         bool
	)

	best := struct {
		id   int64
		zone string
		of   Event
	}{}
	for _, row := range queue {
		authored, known := Known(row.key)
		if !known || !now.Before(row.generated.Add(authored.Patience)) {
			if err := e.drop(ctx, tx, accountID, row.id, row.key, authored, known, place, now); err != nil {
				return 0, Event{}, "", err
			}
			continue
		}
		if !allow(authored.Surface) {
			continue
		}
		zone, placeable := authored.Where.Where(place)
		if !placeable {
			continue
		}
		if authored.Needs.Any() {
			if !asked {
				money, err := e.purse.Balance(ctx, tx, accountID)
				if err != nil {
					return 0, Event{}, "", err
				}
				circumstances.Money, asked = money, true
			}
			if !authored.Needs.Met(circumstances) {
				if err := e.wait(ctx, tx, row.id, now.Add(Requeue)); err != nil {
					return 0, Event{}, "", err
				}
				continue
			}
		}
		// An escalation first, then the heaviest — and the queue is already in
		// eligibility order, so the first of two equal weights is the one that
		// has waited longest.
		//
		// Raised beats weight rather than being given a large weight, because
		// weight is an authoring judgement between things the world thought of
		// and this is not one of those: a moment the player's own rules could
		// not handle outranks anything the generator produced, whatever an
		// author wrote in a file.
		better := authored.Weight > best.of.Weight
		if authored.Raised != best.of.Raised {
			better = authored.Raised
		}
		if best.id == 0 || better {
			best.id, best.zone, best.of = row.id, zone, authored
		}
	}
	return best.id, best.of, best.zone, nil
}

// drop finishes an event the world has given up on, and records that it did.
//
// It is recorded rather than merely deleted-in-effect because §3.2's history is
// what §3.6's diagnosis argues from, and "the world had four things for you and
// you were never anywhere near any of them" is exactly the sort of pattern that
// surface exists to be able to show.
func (e *Events) drop(
	ctx context.Context, tx pgx.Tx, accountID string, id int64, key string,
	authored Event, known bool, place string, now time.Time,
) error {
	name, kind := key, Opportunity
	if known {
		name, kind = authored.Name, authored.Kind
	}
	if _, err := tx.Exec(ctx, `
		UPDATE event_backlog SET state = 'done', settled_at = $2,
		       offered_at = NULL, expires_at = NULL
		 WHERE id = $1 AND state = 'waiting'`, id, now); err != nil {
		return fmt.Errorf("event: drop %s: %w", key, err)
	}
	return e.record(ctx, tx, accountID, id, key, name, kind, place, zoneOr(place), Dropped, now, time.Time{})
}

// zoneOr is the spot a record uses when there was not one — a dropped event was
// never placed. The place's own first zone rather than an empty string, so the
// column stays honest about being a spot in the world.
func zoneOr(place string) string {
	zones := city.ZonesIn(place)
	if len(zones) == 0 {
		return ""
	}
	return zones[len(zones)-1]
}

// wait moves a row's eligibility forward without touching anything else.
func (e *Events) wait(ctx context.Context, tx pgx.Tx, id int64, until time.Time) error {
	if _, err := tx.Exec(ctx,
		`UPDATE event_backlog SET eligible_at = $2 WHERE id = $1 AND state = 'waiting'`,
		id, until); err != nil {
		return fmt.Errorf("event: requeue: %w", err)
	}
	return nil
}

/*
offer puts the event on screen.

The write is wrapped in a savepoint because the partial unique index — one offer
per player — is a real constraint that a second tab can hit, and a violation
would otherwise abort the caller's whole transaction. Losing that race is not an
error: it means somebody already has something on screen, which is the state
this was checking for a moment ago.
*/
func (e *Events) offer(
	ctx context.Context, tx pgx.Tx, accountID string, id int64,
	authored Event, place, zone string, now time.Time,
) (*Offer, error) {
	inner, err := tx.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("event: offer: %w", err)
	}

	var out Offer
	err = inner.QueryRow(ctx, `
		UPDATE event_backlog
		   SET state = 'offered', offered_at = $3, expires_at = $4,
		       place = $5, zone = $6, offers = offers + 1
		 WHERE id = $1 AND account_id = $2 AND state = 'waiting'
		RETURNING id, offered_at, expires_at`,
		id, accountID, now, now.Add(Window), place, zone,
	).Scan(&out.ID, &out.OfferedAt, &out.ExpiresAt)

	var violation *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		_ = inner.Rollback(ctx)
		return nil, nil
	case errors.As(err, &violation) && violation.Code == uniqueViolation:
		_ = inner.Rollback(ctx)
		return nil, nil
	case err != nil:
		_ = inner.Rollback(ctx)
		return nil, fmt.Errorf("event: offer %s: %w", authored.Key, err)
	}
	if err := inner.Commit(ctx); err != nil {
		return nil, fmt.Errorf("event: offer %s: %w", authored.Key, err)
	}

	out.Event, out.Place, out.Zone = authored, place, zone
	return &out, nil
}

// uniqueViolation is Postgres's SQLSTATE for a unique index refusing a row.
const uniqueViolation = "23505"

// lapse closes an offer nobody answered inside its window, and puts it back.
func (e *Events) lapse(ctx context.Context, tx pgx.Tx, accountID string, now time.Time) error {
	return e.unoffer(ctx, tx, accountID, now, Lapsed, `
		SELECT id, event, place, zone FROM event_backlog
		 WHERE account_id = $1 AND state = 'offered' AND expires_at <= $2`, accountID, now)
}

// walkedPast returns an offer the player has walked away from.
//
// §4's own example: "walking past an argument returns it to the backlog rather
// than burning it — otherwise players learn that avoiding content destroys it."
// This is that sentence, and it is why the backlog row is updated rather than
// deleted here and everywhere else in this file.
func (e *Events) walkedPast(ctx context.Context, tx pgx.Tx, accountID, place string, now time.Time) error {
	return e.unoffer(ctx, tx, accountID, now, Left, `
		SELECT id, event, place, zone FROM event_backlog
		 WHERE account_id = $1 AND state = 'offered' AND place <> $2`, accountID, place)
}

// unoffer takes whatever the query names off screen, records why, and returns
// each of them to the backlog after a rest.
func (e *Events) unoffer(
	ctx context.Context, tx pgx.Tx, accountID string, now time.Time, answer Answer,
	sql string, args ...any,
) error {
	type open struct {
		id          int64
		key         string
		place, zone string
	}

	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("event: read what is open: %w", err)
	}
	var found []open
	for rows.Next() {
		var row open
		if err := rows.Scan(&row.id, &row.key, &row.place, &row.zone); err != nil {
			rows.Close()
			return fmt.Errorf("event: read what is open: %w", err)
		}
		found = append(found, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("event: read what is open: %w", err)
	}

	for _, row := range found {
		if _, err := tx.Exec(ctx, `
			UPDATE event_backlog
			   SET state = 'waiting', offered_at = NULL, expires_at = NULL,
			       eligible_at = $2, declines = declines + 1
			 WHERE id = $1`, row.id, now.Add(Rest)); err != nil {
			return fmt.Errorf("event: return %s to the backlog: %w", row.key, err)
		}
		name, kind := row.key, Opportunity
		if authored, ok := Known(row.key); ok {
			name, kind = authored.Name, authored.Kind
		}
		if err := e.record(ctx, tx, accountID, row.id, row.key, name, kind,
			row.place, row.zone, answer, now, time.Time{}); err != nil {
			return err
		}
	}
	return nil
}

/* ── answering ────────────────────────────────────────────────────────── */

/*
Accept takes the event up.

Two things happen that are not obvious from the name.

**The successor is queued here** — §4: "responding to one immediately queues its
successor." Inside the same transaction, so the pair commit together.

**Whether it is finished depends on the content.** An event authored `awaits`
opened something this package does not run: a challenge (#23), a job situation
(#28), a conflict (#30). It stays accepted, its record stays pending, and the
module that ran the thing calls [Events.Resolve]. Everything else finishes on
the spot, because it already has — the player read what it came to.
*/
func (e *Events) Accept(ctx context.Context, tx pgx.Tx, accountID string, id int64) (Answered, error) {
	now := e.clk.Now()

	key, place, zone, err := e.claim(ctx, tx, accountID, id, now, `
		UPDATE event_backlog
		   SET state = 'accepted', offered_at = NULL, expires_at = NULL
		 WHERE id = $1 AND account_id = $2 AND state = 'offered' AND expires_at > $3
		RETURNING event, place, zone`)
	if err != nil {
		return Answered{}, err
	}

	authored, known := Known(key)
	out := Answered{Event: authored, Place: place, Zone: zone, Answer: Accepted, Open: known && authored.Awaits}

	resolved := time.Time{}
	if !out.Open {
		// Finished the moment it was accepted. An event withdrawn from content
		// while it stood is also finished, for the same reason it cannot be
		// shown: there is nothing left of it to run.
		if _, err := tx.Exec(ctx,
			`UPDATE event_backlog SET state = 'done', settled_at = $2 WHERE id = $1`, id, now,
		); err != nil {
			return Answered{}, fmt.Errorf("event: finish %s: %w", key, err)
		}
		resolved = now
	}

	name, kind := key, Opportunity
	if known {
		name, kind = authored.Name, authored.Kind
	}
	if err := e.record(ctx, tx, accountID, id, key, name, kind, place, zone, Accepted, now, resolved); err != nil {
		return Answered{}, err
	}

	if known && authored.Then != "" {
		queued, err := e.queue(ctx, tx, accountID, authored.Then, now, now)
		if err != nil {
			return Answered{}, err
		}
		if queued {
			out.Next = authored.Then
		}
	}

	out.Standing, err = e.standing(ctx, tx, accountID, now)
	return out, err
}

/*
Decline puts it back.

**This is the ticket's headline rule** (§3.9, §4): declining does not consume.
The row goes back to 'waiting' — after a rest, so that the same encounter is not
put to the player again two steps later — and the count of how often it has been
turned down goes up. Nothing is deleted, here or anywhere in this package.

The refusal is recorded as its own answer rather than as an absence, because §4
makes it a signal: "a player routing around every interpersonal encounter is
telling the release policy something worth acting on". A backlog that quietly
requeued would have thrown that away.
*/
func (e *Events) Decline(ctx context.Context, tx pgx.Tx, accountID string, id int64) (Answered, error) {
	now := e.clk.Now()

	key, place, zone, err := e.claim(ctx, tx, accountID, id, now, `
		UPDATE event_backlog
		   SET state = 'waiting', offered_at = NULL, expires_at = NULL,
		       eligible_at = $4, declines = declines + 1
		 WHERE id = $1 AND account_id = $2 AND state = 'offered' AND expires_at > $3
		RETURNING event, place, zone`, now.Add(Rest))
	if err != nil {
		return Answered{}, err
	}

	authored, known := Known(key)
	name, kind := key, Opportunity
	if known {
		name, kind = authored.Name, authored.Kind
	}
	if err := e.record(ctx, tx, accountID, id, key, name, kind, place, zone, Declined, now, time.Time{}); err != nil {
		return Answered{}, err
	}

	out := Answered{Event: authored, Place: place, Zone: zone, Answer: Declined}
	out.Standing, err = e.standing(ctx, tx, accountID, now)
	return out, err
}

// claim takes an offer off screen with one statement, so two tabs answering the
// same thing produce one answer and one refusal rather than two of anything.
func (e *Events) claim(
	ctx context.Context, tx pgx.Tx, accountID string, id int64, now time.Time,
	sql string, extra ...any,
) (key, place, zone string, err error) {
	args := append([]any{id, accountID, now}, extra...)
	err = tx.QueryRow(ctx, sql, args...).Scan(&key, &place, &zone)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return "", "", "", ErrNoOffer
	case err != nil:
		return "", "", "", fmt.Errorf("event: answer: %w", err)
	}
	return key, place, zone, nil
}

/*
Resolve settles an accepted event whose outcome was decided elsewhere.

The seam §4 draws between the event queue and the challenge catalogue: "an event
*can* offer a challenge… they are distinct systems, not one abstraction". An
event marked `awaits` in content is a doorway, and whichever module walked
through it — #23's challenge run, #28's job situation, #30's conflict — calls
this when it knows how it went.

It takes the instant rather than reading the clock, for internal/playbook's
reason: the rollover replays days, and a record stamped with the moment it was
written would put a Tuesday's outcome on a Friday.

Settling twice is [ErrNotOpen] rather than a silent second write. A module that
resolved the same event two ways has a bug, and history is the wrong place to
find that out.
*/
func (e *Events) Resolve(ctx context.Context, tx pgx.Tx, accountID string, id int64, at time.Time) error {
	tag, err := tx.Exec(ctx, `
		UPDATE event_backlog SET state = 'done', settled_at = $3
		 WHERE id = $1 AND account_id = $2 AND state = 'accepted'`, id, accountID, at)
	if err != nil {
		return fmt.Errorf("event: resolve: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotOpen
	}

	if _, err := tx.Exec(ctx, `
		UPDATE event_record SET resolved_at = $2
		 WHERE backlog_id = $1 AND answer = 'accepted' AND resolved_at IS NULL`, id, at); err != nil {
		return fmt.Errorf("event: resolve: %w", err)
	}
	return nil
}

/* ── history ──────────────────────────────────────────────────────────── */

// record writes one line of §3.2's event history.
func (e *Events) record(
	ctx context.Context, tx pgx.Tx, accountID string, backlog int64,
	key, name string, kind Kind, place, zone string, answer Answer, at, resolved time.Time,
) error {
	var settled *time.Time
	if !resolved.IsZero() {
		settled = &resolved
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO event_record
		       (account_id, backlog_id, event, kind, name, at, place, zone, answer, resolved_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		accountID, backlog, key, string(kind), name, at, place, zone, string(answer), settled,
	); err != nil {
		return fmt.Errorf("event: record %s: %w", key, err)
	}
	return nil
}

/*
Records is one page of what has happened to this player, newest first.

A page rather than the world snapshot, for the reason the playbook's log and the
leaderboards are: history is unbounded and a world model is not (§10.2).
*/
func (e *Events) Records(ctx context.Context, tx pgx.Tx, accountID string, page int) (Log, error) {
	out := Log{Page: page, Records: []Record{}}
	if out.Page < 1 {
		out.Page = 1
	}

	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM event_record WHERE account_id = $1`, accountID,
	).Scan(&out.Total); err != nil {
		return Log{}, fmt.Errorf("event: count the record: %w", err)
	}

	out.Pages = (out.Total + LogPage - 1) / LogPage
	if out.Pages == 0 {
		out.Pages = 1
	}
	if out.Page > out.Pages {
		// Clamped rather than refused: the client asked a reasonable question
		// about a world that moved.
		out.Page = out.Pages
	}

	rows, err := tx.Query(ctx, `
		SELECT id, event, name, kind, at, place, zone, answer, resolved_at
		  FROM event_record
		 WHERE account_id = $1
		 ORDER BY at DESC, id DESC
		 LIMIT $2 OFFSET $3`, accountID, LogPage, (out.Page-1)*LogPage)
	if err != nil {
		return Log{}, fmt.Errorf("event: read the record: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			row      Record
			kind     string
			answer   string
			resolved *time.Time
		)
		if err := rows.Scan(&row.ID, &row.Key, &row.Name, &kind, &row.At,
			&row.Place, &row.Zone, &answer, &resolved); err != nil {
			return Log{}, fmt.Errorf("event: read the record: %w", err)
		}
		row.Kind, row.Answer = Kind(kind), Answer(answer)
		if resolved != nil {
			row.Resolved = *resolved
		}
		out.Records = append(out.Records, row)
	}
	if err := rows.Err(); err != nil {
		return Log{}, fmt.Errorf("event: read the record: %w", err)
	}
	return out, nil
}

/*
Signal is §4's "declining is signal", counted per family over a window.

Read by the release policy (#22) and by nothing else today. It is here rather
than in that ticket's module because it is a query over this package's own
tables (§3.1) — what to *do* about a player who walks past every interpersonal
encounter is a policy, and what they have walked past is a fact.
*/
func (e *Events) Signal(ctx context.Context, tx pgx.Tx, accountID string, since time.Time) ([]Tally, error) {
	tallies := map[Kind]*Tally{}
	for _, k := range Kinds {
		tallies[k] = &Tally{Kind: k}
	}

	rows, err := tx.Query(ctx, `
		SELECT kind, answer, count(*) FROM event_record
		 WHERE account_id = $1 AND at > $2
		 GROUP BY kind, answer`, accountID, since)
	if err != nil {
		return nil, fmt.Errorf("event: read the signal: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			kind   string
			answer string
			n      int
		)
		if err := rows.Scan(&kind, &answer, &n); err != nil {
			return nil, fmt.Errorf("event: read the signal: %w", err)
		}
		tally, ok := tallies[Kind(kind)]
		if !ok {
			continue
		}
		switch Answer(answer) {
		case Accepted:
			tally.Offered, tally.Accepted = tally.Offered+n, tally.Accepted+n
		case Declined:
			tally.Offered, tally.Declined = tally.Offered+n, tally.Declined+n
		case Left, Lapsed:
			tally.Offered, tally.Passed = tally.Offered+n, tally.Passed+n
		case Dropped:
			// Never put to the player at all, so it is not something they
			// answered and must not read as one.
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("event: read the signal: %w", err)
	}

	out := make([]Tally, 0, len(Kinds))
	for _, k := range Kinds {
		out = append(out, *tallies[k])
	}
	return out, nil
}

/*
attention composes what the policy is allowed to decide from.

Three reads, and all three are here rather than in the policy for the same
reason: [Policy] is pure, so everything it decides from has to be gathered
before it is asked. Two of them are this package's own tables and the third is
[Bearings] — which is why it is an interface, and why the release path is the
only thing that calls it.
*/
func (e *Events) attention(
	ctx context.Context, tx pgx.Tx, accountID string, authored Event, now time.Time,
) (Attention, error) {
	out := Attention{
		Now:       now,
		Kind:      authored.Kind,
		Since:     Never,
		SinceKind: Never,
		Raised:    authored.Raised,
	}

	var last, lastKind *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE at > $2), max(at),
		       max(at) FILTER (WHERE kind = $3)
		  FROM event_record
		 WHERE account_id = $1 AND answer <> 'dropped'`,
		accountID, now.Add(-Around), string(authored.Kind),
	).Scan(&out.Released, &last, &lastKind); err != nil {
		return Attention{}, fmt.Errorf("event: read what has been released: %w", err)
	}
	if last != nil {
		out.Since = now.Sub(*last)
	}
	if lastKind != nil {
		out.SinceKind = now.Sub(*lastKind)
	}

	tallies, err := e.Signal(ctx, tx, accountID, now.Add(-Lately))
	if err != nil {
		return Attention{}, err
	}
	for _, tally := range tallies {
		if tally.Kind == authored.Kind {
			out.Offered, out.Passed = tally.Offered, tally.Declined+tally.Passed
		}
	}

	if out.Away, err = e.bearings.Away(ctx, tx, accountID); err != nil {
		return Attention{}, err
	}
	if out.Lagging, err = e.bearings.Lagging(ctx, tx, accountID); err != nil {
		return Attention{}, err
	}
	return out, nil
}

// stableBy sorts in place, keeping the order of equal elements — which is what
// makes "heaviest, then least seen, then authored order" one order rather than
// three that argue.
func stableBy(all []Event, less func(a, b Event) bool) {
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && less(all[j], all[j-1]); j-- {
			all[j], all[j-1] = all[j-1], all[j]
		}
	}
}
