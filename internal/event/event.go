// [Claude fable5] Package event owns §4's backlog. The lifecycle is
// generated → eligible → offered → (declined | accepted) → resolved. Nothing
// here deletes a backlog row, and eligibility is computed at release and stored
// nowhere. It owns event_backlog and event_record and takes the caller's
// transaction.
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

// [Claude fable5] The knobs. Constants rather than configuration.
const (
	// [Claude fable5] Depth is how many unfinished events the world keeps ready
	// for one player.
	Depth = 6

	// [Claude fable5] Window is how long an offer stands before it lapses,
	// unless the event says otherwise (see [Event.Stands]).
	Window = 45 * time.Minute

	// [Claude fable5] Rest is how long an event waits after the player did not
	// take it.
	Rest = 6 * time.Hour

	// [Claude fable5] Requeue is how long an event waits after failing
	// revalidation.
	Requeue = 12 * time.Hour

	// [Claude fable5] LogPage is how many records one page of the history
	// carries.
	LogPage = 20
)

var (
	// [Claude fable5] ErrNoOffer is an answer to something that is not on
	// screen: the wrong id, somebody else's, one already answered, or one whose
	// window has passed. One error, so a client probing ids learns nothing.
	ErrNoOffer = errors.New("event: there is nothing open by that name")

	// [Claude fable5] ErrNotOpen is settling something that was not accepted,
	// or was settled already.
	ErrNotOpen = errors.New("event: that is not an accepted event awaiting an outcome")

	// [Claude fable5] ErrNotRaised is [Events.Raise] called with something that
	// is not an authored escalation.
	ErrNotRaised = errors.New("event: that is not an event the world raises")
)

// [Claude fable5] Purse is what this package may know about the player's money
// — the one question §4's preconditions ask. internal/api binds it to the same
// adapter internal/factor's Purse gets, which ensures the character row exists
// first.
type Purse interface {
	Balance(ctx context.Context, tx pgx.Tx, accountID string) (simulation.Money, error)
}

// [Claude fable5] Offer is an event shown to one player at one place.
type Offer struct {
	ID    int64
	Event Event
	// [Claude fable5] Place and Zone are internal/city keys.
	Place string
	Zone  string

	OfferedAt time.Time
	ExpiresAt time.Time
	// [Claude fable5] Because is why an escalation was raised. Empty for ordinary
	// events.
	Because string
	// [Claude fable5] Title replaces [Event.Name] when set.
	Title string
	// [Claude fable5] Fire is the playbook record an escalation is about. Zero for
	// ordinary events.
	Fire int64
}

// [Claude fable5] Standing is the offer on the player's screen.
type Standing struct {
	// [Claude fable5] Offer is nil when nothing is on screen.
	Offer *Offer
}

// [Claude fable5] Answer is what became of one offer.
type Answer string

const (
	// [Claude fable5] Accepted is the player accepting the offer.
	Accepted Answer = "accepted"
	// [Claude fable5] Declined is the player refusing the offer. The event
	// returns to the backlog.
	Declined Answer = "declined"
	// [Claude fable5] Left is the player walking to another place before
	// answering. The event returns to the backlog.
	Left Answer = "left"
	// [Claude fable5] Lapsed is the offer expiring unanswered.
	Lapsed Answer = "lapsed"
	// [Claude fable5] Dropped is an event removed from the backlog because it
	// waited past its patience or is no longer in content.
	Dropped Answer = "dropped"
)

// [Claude fable5] Answered is what one answer did. The successor is queued
// inside the same transaction as the answer.
type Answered struct {
	Event  Event
	Place  string
	Zone   string
	Answer Answer
	// [Claude fable5] Next is the event queued after an accepted one, or empty.
	Next string
	// [Claude fable5] Open is whether an accepted event is still waiting on the
	// challenge, situation or conflict it opened.
	Open     bool
	Standing Standing
}

// [Claude fable5] Record is one thing that happened, as the history keeps it.
type Record struct {
	ID   int64
	Key  string
	Name string
	At   time.Time
	// [Claude fable5] Place and Zone are internal/city keys.
	Place string
	Zone  string

	Answer Answer
	// [Claude fable5] Resolved is when an accepted event finished. Zero while it
	// runs.
	Resolved time.Time
	// [Claude fable5] Because is copied from [Offer.Because].
	Because string
	// [Claude fable5] Fire is copied from [Offer.Fire].
	Fire int64
}

// [Claude fable5] Log is one page of the history, newest first.
type Log struct {
	Records []Record
	Page    int
	Pages   int
	Total   int
}

// [Claude fable5] Events is the module. It holds no per-player state.
type Events struct {
	clk    simulation.Clock
	purse  Purse
	policy Policy
}

// [Claude fable5] New binds the module to the clock it reads and the two things
// it cannot reach otherwise.
func New(clk simulation.Clock, purse Purse, policy Policy) *Events {
	return &Events{clk: clk, purse: purse, policy: policy}
}

// [Claude fable5] Of reads the surface without changing it: what a command's
// response restates once it has done its work. It releases nothing and lapses
// nothing.
func (e *Events) Of(ctx context.Context, tx pgx.Tx, accountID string) (Standing, error) {
	return e.standing(ctx, tx, accountID, e.clk.Now())
}

func (e *Events) standing(ctx context.Context, tx pgx.Tx, accountID string, now time.Time) (Standing, error) {
	var (
		out     Offer
		key     string
		place   string
		zone    string
		because *string
		title   *string
		fire    *int64
	)
	err := tx.QueryRow(ctx, `
		SELECT id, event, place, zone, offered_at, expires_at, because, title, fire
		  FROM event_backlog
		 WHERE account_id = $1 AND state = 'offered' AND expires_at > $2`,
		accountID, now,
	).Scan(&out.ID, &key, &place, &zone, &out.OfferedAt, &out.ExpiresAt, &because, &title, &fire)
	if because != nil {
		out.Because = *because
	}
	if title != nil {
		out.Title = *title
	}
	if fire != nil {
		out.Fire = *fire
	}
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Standing{}, nil
	case err != nil:
		return Standing{}, fmt.Errorf("event: read the offer: %w", err)
	}

	authored, ok := Known(key)
	if !ok {
		// [Claude fable5] Authored out of the game while it was on screen. The
		// next release drops the row.
		return Standing{}, nil
	}
	out.Event, out.Place, out.Zone = authored, place, zone
	return Standing{Offer: &out}, nil
}

// [Claude fable5] Replenish tops the backlog up. Called by the rollover once
// per run, and as a floor by the release path when it finds a backlog that is
// empty. Selection is deterministic: what the player has seen least, then what
// the author weighted heaviest, then the catalogue's own order.
func (e *Events) Replenish(ctx context.Context, tx pgx.Tx, accountID string, _ simulation.Date) error {
	return e.fill(ctx, tx, accountID, e.clk.Now())
}

// [Claude fable5] floor is the one generation the release path does for itself,
// and only when the backlog is empty.
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

	// [Claude fable5] What is already in the backlog, so the same thing is not
	// queued twice. The partial unique index is the backstop.
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

	// [Claude fable5] When each event was last finished, which is what a
	// cooldown is measured from. An event with no cooldown authored is finished
	// forever.
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

	// [Claude fable5] All() is already heaviest-first and then by key, so this
	// is a stable sort on one key: how little the player has seen it.
	candidates := make([]Event, 0, len(events))
	for _, candidate := range All() {
		switch {
		case candidate.Chained:
			// [Claude fable5] Only ever put there by its parent.
			continue
		case candidate.Raised:
			// [Claude fable5] Only ever put there by [Events.Raise].
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
		queued, err := e.queue(ctx, tx, accountID, candidate.Key, now, now, Why{})
		if err != nil {
			return err
		}
		if queued {
			room--
		}
	}
	return nil
}

// [Claude fable5] queue puts one event into the backlog, and reports whether it
// went in. ON CONFLICT DO NOTHING against the partial unique index, so an event
// already waiting is not an error. An escalation that collides brings its
// reason and pointer forward onto the waiting row (see [Events.Raise]).
func (e *Events) queue(
	ctx context.Context, tx pgx.Tx, accountID, key string, at, eligible time.Time, why Why,
) (bool, error) {
	var because *string
	if why.Because != "" {
		because = &why.Because
	}
	var title *string
	if why.Title != "" {
		title = &why.Title
	}
	var fire *int64
	if why.Fire != 0 {
		fire = &why.Fire
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO event_backlog (account_id, event, state, generated_at, eligible_at, because, title, fire)
		VALUES ($1, $2, 'waiting', $3, $4, $5, $6, $7)
		ON CONFLICT DO NOTHING`, accountID, key, at, eligible, because, title, fire)
	if err != nil {
		return false, fmt.Errorf("event: generate %s: %w", key, err)
	}
	if tag.RowsAffected() == 1 {
		return true, nil
	}

	// [Claude fable5] It was already there. Only a raise has anything to bring
	// forward, and only onto a row still waiting.
	if because != nil || title != nil || fire != nil {
		if _, err := tx.Exec(ctx, `
			UPDATE event_backlog SET because = $3, title = $4, fire = $5
			 WHERE account_id = $1 AND event = $2 AND state = 'waiting'`,
			accountID, key, because, title, fire); err != nil {
			return false, fmt.Errorf("event: raise %s again: %w", key, err)
		}
	}
	return false, nil
}

// [Claude fable5] Raise puts an escalation into the backlog: something another
// module decided has to reach this player. It is exempt from [Depth], the daily
// cap and cooling, and not exempt from the placement rules, the offer window,
// or one thing at a time. It reports whether it went in; false means one is
// already waiting, and a waiting row takes the newer reason and pointer.
func (e *Events) Raise(
	ctx context.Context, tx pgx.Tx, accountID, key string, why Why,
) (bool, error) {
	authored, known := Known(key)
	if !known || !authored.Raised {
		return false, fmt.Errorf("%w: %q", ErrNotRaised, key)
	}
	now := e.clk.Now()
	return e.queue(ctx, tx, accountID, key, now, now, why)
}

// [Claude fable5] Why is what an escalation is about, as whoever raised it
// hands it over. The zero value is every ordinary event.
type Why struct {
	// [Claude fable5] Because is the moment and the rule in one sentence — see
	// [Offer.Because].
	Because string
	// [Claude fable5] Title is what to call this one — see [Offer.Title]. Empty
	// falls back to the authored event's own name.
	Title string
	// [Claude fable5] Fire is the playbook record it was about — see
	// [Offer.Fire].
	Fire int64
}

// [Claude fable5] Ready brings one waiting event's cooling forward, so it can
// be offered now. For authoring; nothing in the game calls it. It moves one
// row's eligibility and nothing else — the record of the lapse stays, which is
// the difference between this and [Events.Forget].
func (e *Events) Ready(ctx context.Context, tx pgx.Tx, accountID, key string) (bool, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE event_backlog SET eligible_at = $3
		 WHERE account_id = $1 AND event = $2 AND state = 'waiting'`,
		accountID, key, e.clk.Now())
	if err != nil {
		return false, fmt.Errorf("event: bring %s forward: %w", key, err)
	}
	return tag.RowsAffected() > 0, nil
}

// [Claude fable5] Forget removes every trace of one event for one player: what
// is queued, what is on screen, and what has already happened. It reports how
// many rows went, across both tables. For authoring; internal/api registers its
// caller only outside production.
func (e *Events) Forget(ctx context.Context, tx pgx.Tx, accountID, key string) (int64, error) {
	backlog, err := tx.Exec(ctx,
		`DELETE FROM event_backlog WHERE account_id = $1 AND event = $2`, accountID, key)
	if err != nil {
		return 0, fmt.Errorf("event: forget what is queued of %s: %w", key, err)
	}
	record, err := tx.Exec(ctx,
		`DELETE FROM event_record WHERE account_id = $1 AND event = $2`, accountID, key)
	if err != nil {
		return 0, fmt.Errorf("event: forget what happened with %s: %w", key, err)
	}
	return backlog.RowsAffected() + record.RowsAffected(), nil
}

// [Claude fable5] seen is how many times each event has been put to this
// player, ever.
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

// [Claude fable5] Arrived is the walk's half of §3.12: the player is standing
// somewhere new. An offer whose window has passed lapses and one pinned to a
// place the player has left is walked past, both back to the backlog; an empty
// backlog is filled; then the first waiting event this place allows, whose
// preconditions still hold, is offered if the release policy says now. The
// offer is nil most of the time.
func (e *Events) Arrived(ctx context.Context, tx pgx.Tx, accountID, place string) (*Offer, error) {
	return e.release(ctx, tx, accountID, place, func(Surface) bool { return true })
}

// [Claude fable5] Looked is the queue surface's half: the player opened the
// game. Only events authored surface: queue are released here. Placement still
// applies.
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

	// [Claude fable5] One thing on screen at a time, decided here rather than by
	// the policy. The unique index is the third line of the same defence.
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
		// [Claude fable5] Left exactly as it was, not requeued: the world is
		// saying this is not the moment, not that the event is wrong for here.
		return nil, nil
	}

	return e.offer(ctx, tx, accountID, candidate, authored, place, zone, now)
}

// [Claude fable5] candidate is §4's revalidation: the first waiting event this
// place allows and current state still permits. A row is dropped (authored out
// of the game, or waiting longer than its patience), requeued for [Requeue]
// (its preconditions no longer hold), or offered. A row this place does not
// allow is left alone.
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

	// [Claude fable5] Read once, and only if something actually asks: most
	// events have no preconditions at all.
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
		// [Claude fable5] An escalation first, then the heaviest. The queue is
		// already in eligibility order, so the first of two equal weights is
		// the one that has waited longest.
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

// [Claude fable5] drop finishes an event the world has given up on, and records
// that it did. Recorded rather than merely dropped, because §3.6's diagnosis
// argues from the history.
func (e *Events) drop(
	ctx context.Context, tx pgx.Tx, accountID string, id int64, key string,
	authored Event, known bool, place string, now time.Time,
) error {
	name := key
	if known {
		name = authored.Name
	}
	if _, err := tx.Exec(ctx, `
		UPDATE event_backlog SET state = 'done', settled_at = $2,
		       offered_at = NULL, expires_at = NULL
		 WHERE id = $1 AND state = 'waiting'`, id, now); err != nil {
		return fmt.Errorf("event: drop %s: %w", key, err)
	}
	return e.record(ctx, tx, accountID, id, key, name, place, zoneOr(place), Dropped, now, time.Time{})
}

// [Claude fable5] zoneOr is the spot a record uses when there was not one — a
// dropped event was never placed. The place's own first zone rather than an
// empty string.
func zoneOr(place string) string {
	zones := city.ZonesIn(place)
	if len(zones) == 0 {
		return ""
	}
	return zones[len(zones)-1]
}

// [Claude fable5] wait moves a row's eligibility forward without touching
// anything else.
func (e *Events) wait(ctx context.Context, tx pgx.Tx, id int64, until time.Time) error {
	if _, err := tx.Exec(ctx,
		`UPDATE event_backlog SET eligible_at = $2 WHERE id = $1 AND state = 'waiting'`,
		id, until); err != nil {
		return fmt.Errorf("event: requeue: %w", err)
	}
	return nil
}

// [Claude fable5] offer puts the event on screen. The write is wrapped in a
// savepoint because the partial unique index — one offer per player — is a
// constraint a second tab can hit, and a violation would otherwise abort the
// caller's whole transaction. Losing that race is not an error.
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
		id, accountID, now, now.Add(authored.Stands()), place, zone,
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

// [Claude fable5] uniqueViolation is Postgres's SQLSTATE for a unique index
// refusing a row.
const uniqueViolation = "23505"

// [Claude fable5] lapse closes an offer nobody answered inside its window, and
// puts it back.
func (e *Events) lapse(ctx context.Context, tx pgx.Tx, accountID string, now time.Time) error {
	return e.unoffer(ctx, tx, accountID, now, Lapsed, `
		SELECT id, event, place, zone FROM event_backlog
		 WHERE account_id = $1 AND state = 'offered' AND expires_at <= $2`, accountID, now)
}

// [Claude fable5] walkedPast returns an offer the player has walked away from
// to the backlog rather than burning it, which is why the row is updated rather
// than deleted here and everywhere else in this file.
func (e *Events) walkedPast(ctx context.Context, tx pgx.Tx, accountID, place string, now time.Time) error {
	return e.unoffer(ctx, tx, accountID, now, Left, `
		SELECT id, event, place, zone FROM event_backlog
		 WHERE account_id = $1 AND state = 'offered' AND place <> $2`, accountID, place)
}

// [Claude fable5] unoffer takes whatever the query names off screen, records
// why, and returns each of them to the backlog after a rest.
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
		name := row.key
		if authored, ok := Known(row.key); ok {
			name = authored.Name
		}
		if err := e.record(ctx, tx, accountID, row.id, row.key, name,
			row.place, row.zone, answer, now, time.Time{}); err != nil {
			return err
		}
	}
	return nil
}

// [Claude fable5] Accept takes the event up. The successor is queued here,
// inside the same transaction. An event authored awaits opened something this
// package does not run, so it stays accepted with its record pending until that
// module calls [Events.Resolve]; everything else finishes on the spot.
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
		// [Claude fable5] Finished the moment it was accepted. An event
		// withdrawn from content while it stood is also finished.
		if _, err := tx.Exec(ctx,
			`UPDATE event_backlog SET state = 'done', settled_at = $2 WHERE id = $1`, id, now,
		); err != nil {
			return Answered{}, fmt.Errorf("event: finish %s: %w", key, err)
		}
		resolved = now
	}

	name := key
	if known {
		name = authored.Name
	}
	if err := e.record(ctx, tx, accountID, id, key, name, place, zone, Accepted, now, resolved); err != nil {
		return Answered{}, err
	}

	if known && authored.Then != "" {
		queued, err := e.queue(ctx, tx, accountID, authored.Then, now, now, Why{})
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

// [Claude fable5] Decline puts it back: declining does not consume. The row
// returns to waiting after a [Rest] and the count of how often it has been
// turned down goes up. The refusal is recorded as its own answer rather than as
// an absence, because it is a signal.
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
	name := key
	if known {
		name = authored.Name
	}
	if err := e.record(ctx, tx, accountID, id, key, name, place, zone, Declined, now, time.Time{}); err != nil {
		return Answered{}, err
	}

	out := Answered{Event: authored, Place: place, Zone: zone, Answer: Declined}
	out.Standing, err = e.standing(ctx, tx, accountID, now)
	return out, err
}

// [Claude fable5] claim takes an offer off screen with one statement, so two
// tabs answering the same thing produce one answer and one refusal.
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

// [Claude fable5] Resolve settles an accepted event whose outcome was decided
// elsewhere. An event marked awaits in content is a doorway, and whichever
// module walked through it calls this when it knows how it went. It takes the
// instant rather than reading the clock, because the rollover replays days.
// Settling twice is [ErrNotOpen].
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

// [Claude fable5] record writes one line of §3.2's event history.
func (e *Events) record(
	ctx context.Context, tx pgx.Tx, accountID string, backlog int64,
	key, name, place, zone string, answer Answer, at, resolved time.Time,
) error {
	var settled *time.Time
	if !resolved.IsZero() {
		settled = &resolved
	}
	// [Claude fable5] because and title are taken off the backlog row rather
	// than passed in. The composed title wins over the authored name when there
	// is one.
	if _, err := tx.Exec(ctx, `
		INSERT INTO event_record
		       (account_id, backlog_id, event, name, at, place, zone, answer, resolved_at, because, fire)
		SELECT $1, $2, $3, COALESCE(NULLIF(b.title, ''), $4), $5, $6, $7, $8, $9, b.because, b.fire
		  FROM event_backlog b WHERE b.id = $2`,
		accountID, backlog, key, name, at, place, zone, string(answer), settled,
	); err != nil {
		return fmt.Errorf("event: record %s: %w", key, err)
	}
	return nil
}

// [Claude fable5] Records is one page of what has happened to this player,
// newest first. A page rather than part of the world snapshot: history is
// unbounded and a world model is not.
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
		// [Claude fable5] Clamped rather than refused: the client asked a
		// reasonable question about a world that moved.
		out.Page = out.Pages
	}

	rows, err := tx.Query(ctx, `
		SELECT id, event, name, at, place, zone, answer, resolved_at, because, fire
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
			answer   string
			resolved *time.Time
			because  *string
			fire     *int64
		)
		if err := rows.Scan(&row.ID, &row.Key, &row.Name, &row.At,
			&row.Place, &row.Zone, &answer, &resolved, &because, &fire); err != nil {
			return Log{}, fmt.Errorf("event: read the record: %w", err)
		}
		row.Answer = Answer(answer)
		if because != nil {
			row.Because = *because
		}
		if fire != nil {
			row.Fire = *fire
		}
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

// [Claude fable5] attention composes what the policy is allowed to decide from.
// [Policy] is pure, so everything it decides from is gathered before it is
// asked.
func (e *Events) attention(
	ctx context.Context, tx pgx.Tx, accountID string, authored Event, now time.Time,
) (Attention, error) {
	out := Attention{
		Now:    now,
		Since:  Never,
		Raised: authored.Raised,
	}

	var last *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE at > $2), max(at)
		  FROM event_record
		 WHERE account_id = $1 AND answer <> 'dropped'`,
		accountID, now.Add(-Around),
	).Scan(&out.Released, &last); err != nil {
		return Attention{}, fmt.Errorf("event: read what has been released: %w", err)
	}
	if last != nil {
		out.Since = now.Sub(*last)
	}
	return out, nil
}

// [Claude fable5] stableBy sorts in place, keeping the order of equal elements,
// so "heaviest, then least seen, then authored order" is one order rather than
// three.
func stableBy(all []Event, less func(a, b Event) bool) {
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && less(all[j], all[j-1]); j-- {
			all[j], all[j-1] = all[j-1], all[j]
		}
	}
}
