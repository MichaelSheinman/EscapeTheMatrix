package event

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/clock"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store/storetest"
)

/*
The backlog, against a real Postgres (§14).

Two of this ticket's rules are held by the schema rather than by Go — one offer
on screen at a time, and one unfinished instance of an event per player — so a
mock would prove nothing about either. The rest is time: patience, the offer
window, the rest after a refusal and the cooldown before something comes round
again are all statements about a clock, and every one of them is driven below by
moving one rather than by sleeping.

The catalogue these tests run against is the one that ships. That is deliberate:
a test that authored its own events would pass over a content tree that could
not release anything.
*/

// world is a fresh database, a clock a test moves by hand, one player, and a
// backlog bound to a purse the test controls.
//
// The transaction is rolled back by a cleanup rather than by a closure, for the
// reason internal/routine's tests are written that way: a t.Fatalf inside a
// closure ends the goroutine outright and the rollback never runs.
func world(t *testing.T, f func(ctx context.Context, tx pgx.Tx, e *Events, clk *clock.Stepping, who string, held *wallet)) {
	t.Helper()
	worldWith(t, Always{}, f)
}

func worldWith(t *testing.T, p Policy, f func(ctx context.Context, tx pgx.Tx, e *Events, clk *clock.Stepping, who string, held *wallet)) {
	t.Helper()
	worldKnowing(t, p, Here{}, f)
}

// worldKnowing is the same, with the bearings the release policy is handed under
// the test's control too — for the tests that are about what reaches [Attention]
// rather than about what is done with it.
func worldKnowing(t *testing.T, p Policy, b Bearings, f func(ctx context.Context, tx pgx.Tx, e *Events, clk *clock.Stepping, who string, held *wallet)) {
	t.Helper()
	st := storetest.New(t)
	ctx := t.Context()

	tx, err := st.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO account (provider, subject, email, display_name)
		VALUES ('google', 'a', 'a@example.test', 'Player a')
		RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("create account: %v", err)
	}

	clk := &clock.Stepping{At: time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)}
	held := &wallet{money: 100_000}
	f(ctx, tx, New(clk, held, b, p), clk, id, held)
}

// wallet is a Purse a test can empty.
type wallet struct{ money simulation.Money }

func (w *wallet) Balance(context.Context, pgx.Tx, string) (simulation.Money, error) {
	return w.money, nil
}

// today is a date the rollover would pass in. Nothing in this module reads it —
// the timestamps come off the clock — and it is here so the calls read as the
// rollover makes them.
var today = simulation.Date{Year: 2026, Month: 5, Day: 1}

// standingAt walks the player somewhere and returns whatever the world had for
// them there.
func standingAt(t *testing.T, ctx context.Context, e *Events, tx pgx.Tx, who, place string) *Offer {
	t.Helper()
	offer, err := e.Arrived(ctx, tx, who, place)
	if err != nil {
		t.Fatalf("arrive at %s: %v", place, err)
	}
	return offer
}

// backlogState reads one row's state, for the assertions that are about the
// lifecycle rather than about what the player sees.
func backlogState(t *testing.T, ctx context.Context, tx pgx.Tx, id int64) (state string, declines int, eligible time.Time) {
	t.Helper()
	if err := tx.QueryRow(ctx,
		`SELECT state, declines, eligible_at FROM event_backlog WHERE id = $1`, id,
	).Scan(&state, &declines, &eligible); err != nil {
		t.Fatalf("read the backlog row: %v", err)
	}
	return state, declines, eligible
}

func answers(t *testing.T, ctx context.Context, tx pgx.Tx, who string) []string {
	t.Helper()
	rows, err := tx.Query(ctx,
		`SELECT answer FROM event_record WHERE account_id = $1 ORDER BY id`, who)
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var answer string
		if err := rows.Scan(&answer); err != nil {
			t.Fatalf("read the record: %v", err)
		}
		out = append(out, answer)
	}
	return out
}

/* ── generation ───────────────────────────────────────────────────────── */

// §4: events are generated ahead of demand and sit in a backlog. The rollover
// is what fills it, and it fills it to a depth rather than by one.
func TestTheBacklogIsFilledAheadOfDemand(t *testing.T) {
	world(t, func(ctx context.Context, tx pgx.Tx, e *Events, _ *clock.Stepping, who string, _ *wallet) {
		if err := e.Replenish(ctx, tx, who, today); err != nil {
			t.Fatalf("replenish: %v", err)
		}

		var waiting int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM event_backlog WHERE account_id = $1 AND state = 'waiting'`, who,
		).Scan(&waiting); err != nil {
			t.Fatalf("count the backlog: %v", err)
		}
		if waiting != Depth {
			t.Errorf("the backlog holds %d, want %d", waiting, Depth)
		}

		// Again, and it does not grow: the depth is a target rather than a
		// per-day allowance. A rollover catching up a fortnight would otherwise
		// queue a fortnight of things nobody has been anywhere near.
		if err := e.Replenish(ctx, tx, who, today); err != nil {
			t.Fatalf("replenish again: %v", err)
		}
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM event_backlog WHERE account_id = $1`, who).Scan(&waiting); err != nil {
			t.Fatalf("count the backlog: %v", err)
		}
		if waiting != Depth {
			t.Errorf("two replenishes hold %d, want %d", waiting, Depth)
		}
	})
}

// A successor is never generated spontaneously: it arrives because its parent
// was answered, and it arrives then whatever else is in the backlog.
func TestASuccessorIsOnlyEverQueuedByItsParent(t *testing.T) {
	world(t, func(ctx context.Context, tx pgx.Tx, e *Events, _ *clock.Stepping, who string, _ *wallet) {
		// Fill it several times over, so that "it just did not fit" is not an
		// available explanation.
		for range 5 {
			if err := e.Replenish(ctx, tx, who, today); err != nil {
				t.Fatalf("replenish: %v", err)
			}
			if _, err := tx.Exec(ctx,
				`UPDATE event_backlog SET state = 'done', settled_at = now() WHERE account_id = $1`, who,
			); err != nil {
				t.Fatalf("clear the backlog: %v", err)
			}
		}

		var chained int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM event_backlog WHERE account_id = $1 AND event = 'the-parcel-was-not-yours'`,
			who).Scan(&chained); err != nil {
			t.Fatalf("look for the successor: %v", err)
		}
		if chained != 0 {
			t.Errorf("a chained event was generated %d times on its own", chained)
		}
	})
}

/* ── placement ────────────────────────────────────────────────────────── */

/*
The headline of the acceptance criteria: an event is never placed somewhere its
placement rules disallow.

Asserted by walking the whole city with a full backlog and holding every offer
against the catalogue's own rules. It is not a spot check — every place, every
release, every time.
*/
func TestAnEventIsNeverPlacedWhereItsRulesDisallow(t *testing.T) {
	world(t, func(ctx context.Context, tx pgx.Tx, e *Events, clk *clock.Stepping, who string, _ *wallet) {
		if err := e.Replenish(ctx, tx, who, today); err != nil {
			t.Fatalf("replenish: %v", err)
		}

		placed := 0
		for _, place := range everywhere() {
			for range 3 {
				offer := standingAt(t, ctx, e, tx, who, place)
				if offer == nil {
					clk.At = clk.At.Add(2 * time.Hour)
					continue
				}
				placed++
				if !offer.Event.Where.Allows(offer.Place, offer.Zone) {
					t.Errorf("%s was placed at %s/%s, which it disallows",
						offer.Event.Key, offer.Place, offer.Zone)
				}
				if offer.Place != place {
					t.Errorf("%s was placed at %s while the player stood at %s",
						offer.Event.Key, offer.Place, place)
				}
				// Answered, so the next walk has a clear screen.
				if _, err := e.Accept(ctx, tx, who, offer.ID); err != nil {
					t.Fatalf("accept: %v", err)
				}
				clk.At = clk.At.Add(2 * time.Hour)
			}
		}
		if placed == 0 {
			t.Fatal("nothing was ever placed anywhere — the test proves nothing")
		}
	})
}

// The zone an event is placed in is one the *place* has, not merely one the
// event named. That is the §10.3 correspondence: a painting draws the trigger,
// and the world may only pin something to a spot that exists.
func TestAnEventIsPinnedToASpotThePlaceActuallyHas(t *testing.T) {
	world(t, func(ctx context.Context, tx pgx.Tx, e *Events, clk *clock.Stepping, who string, _ *wallet) {
		if err := e.Replenish(ctx, tx, who, today); err != nil {
			t.Fatalf("replenish: %v", err)
		}

		seen := 0
		for _, place := range everywhere() {
			offer := standingAt(t, ctx, e, tx, who, place)
			if offer == nil {
				clk.At = clk.At.Add(2 * time.Hour)
				continue
			}
			seen++
			if !hasZone(offer.Place, offer.Zone) {
				t.Errorf("%s was pinned to %q, which %s does not have",
					offer.Event.Key, offer.Zone, offer.Place)
			}
			if _, err := e.Accept(ctx, tx, who, offer.ID); err != nil {
				t.Fatalf("accept: %v", err)
			}
			clk.At = clk.At.Add(2 * time.Hour)
		}
		if seen == 0 {
			t.Fatal("nothing was placed at all")
		}
	})
}

/* ── the lifecycle ────────────────────────────────────────────────────── */

/*
The rule the ticket is named for: declining must not consume.

The row goes back to 'waiting', the count of refusals goes up, and — crucially —
the same event can still happen later. A test that only asserted the state would
pass over an implementation that requeued it with a patience of zero.
*/
func TestDecliningReturnsTheEventToTheBacklog(t *testing.T) {
	world(t, func(ctx context.Context, tx pgx.Tx, e *Events, clk *clock.Stepping, who string, _ *wallet) {
		if err := e.Replenish(ctx, tx, who, today); err != nil {
			t.Fatalf("replenish: %v", err)
		}

		offer := firstOffer(t, ctx, e, tx, who, clk)
		answered, err := e.Decline(ctx, tx, who, offer.ID)
		if err != nil {
			t.Fatalf("decline: %v", err)
		}
		if answered.Standing.Offer != nil {
			t.Error("something is still on screen after declining it")
		}

		state, declines, eligible := backlogState(t, ctx, tx, offer.ID)
		if state != "waiting" {
			t.Errorf("a declined event is %q, want waiting", state)
		}
		if declines != 1 {
			t.Errorf("the refusal was counted %d times", declines)
		}
		if !eligible.After(clk.At) {
			t.Error("a declined event is eligible again immediately")
		}

		// And it really can come round again: past the rest, back to the same
		// spot, the same event is offered.
		clk.At = clk.At.Add(Rest + time.Hour)
		again := standingAt(t, ctx, e, tx, who, offer.Place)
		if again == nil {
			t.Fatal("nothing came back after the rest")
		}
		if again.Event.Key != offer.Event.Key {
			t.Errorf("what came back was %s, want %s", again.Event.Key, offer.Event.Key)
		}
		if again.ID != offer.ID {
			t.Errorf("it came back as a new row (%d, was %d) — that is consumption with extra steps",
				again.ID, offer.ID)
		}
	})
}

// Walking past is the other way of not answering, and §4 names it explicitly:
// "walking past an argument returns it to the backlog rather than burning it".
func TestWalkingPastReturnsTheEventToTheBacklog(t *testing.T) {
	world(t, func(ctx context.Context, tx pgx.Tx, e *Events, clk *clock.Stepping, who string, _ *wallet) {
		if err := e.Replenish(ctx, tx, who, today); err != nil {
			t.Fatalf("replenish: %v", err)
		}

		offer := firstOffer(t, ctx, e, tx, who, clk)

		// Somewhere else. The offer was pinned to a spot, and the player is no
		// longer standing at it.
		elsewhere := "downtown"
		if offer.Place == elsewhere {
			elsewhere = "riverfront"
		}
		if _, err := e.Arrived(ctx, tx, who, elsewhere); err != nil {
			t.Fatalf("walk away: %v", err)
		}

		state, declines, _ := backlogState(t, ctx, tx, offer.ID)
		if state != "waiting" {
			t.Errorf("an event walked past is %q, want waiting", state)
		}
		if declines != 1 {
			t.Errorf("walking past was counted %d times", declines)
		}

		// Recorded as `left` rather than as `declined`: one of them is a
		// decision and the other is not, and #22 must be able to tell them
		// apart.
		if got := answers(t, ctx, tx, who); len(got) == 0 || got[len(got)-1] != string(Left) {
			t.Errorf("the record says %v, want the last one to be %q", got, Left)
		}
	})
}

// An offer nobody answered inside its window lapses — and lapsing, like every
// other way of not answering, puts it back.
func TestAnOfferNobodyAnsweredLapsesBackIntoTheBacklog(t *testing.T) {
	world(t, func(ctx context.Context, tx pgx.Tx, e *Events, clk *clock.Stepping, who string, _ *wallet) {
		if err := e.Replenish(ctx, tx, who, today); err != nil {
			t.Fatalf("replenish: %v", err)
		}

		offer := firstOffer(t, ctx, e, tx, who, clk)

		clk.At = clk.At.Add(Window + time.Minute)
		standing, err := e.Of(ctx, tx, who)
		if err != nil {
			t.Fatalf("read the standing: %v", err)
		}
		if standing.Offer != nil {
			t.Error("an offer past its window is still on screen")
		}

		// The lapse itself is written by the next release, not by the read.
		if _, err := e.Arrived(ctx, tx, who, offer.Place); err != nil {
			t.Fatalf("arrive: %v", err)
		}
		if state, _, _ := backlogState(t, ctx, tx, offer.ID); state == "done" {
			t.Error("a lapsed offer was consumed")
		}
		if _, err := e.Accept(ctx, tx, who, offer.ID); !errors.Is(err, ErrNoOffer) {
			t.Errorf("a lapsed offer was answerable: %v", err)
		}
	})
}

// Accepting finishes an event that has nothing else to it, and queues its
// successor — §4's "responding to one immediately queues its successor".
func TestRespondingQueuesTheSuccessor(t *testing.T) {
	world(t, func(ctx context.Context, tx pgx.Tx, e *Events, clk *clock.Stepping, who string, _ *wallet) {
		// The parcel chain starts at home, so put the player there with an
		// empty backlog but that one event in it.
		if _, err := tx.Exec(ctx, `
			INSERT INTO event_backlog (account_id, event, state, generated_at, eligible_at)
			VALUES ($1, 'a-knock-at-the-door', 'waiting', $2, $2)`, who, clk.At); err != nil {
			t.Fatalf("queue the parent: %v", err)
		}

		offer := standingAt(t, ctx, e, tx, who, "east-side-apartment")
		if offer == nil || offer.Event.Key != "a-knock-at-the-door" {
			t.Fatalf("the parent was not offered: %+v", offer)
		}

		answered, err := e.Accept(ctx, tx, who, offer.ID)
		if err != nil {
			t.Fatalf("accept: %v", err)
		}
		if answered.Next != offer.Event.Then {
			t.Errorf("accepting queued %q, want %q", answered.Next, offer.Event.Then)
		}
		if answered.Open {
			t.Error("an event that finishes on the spot is still open")
		}

		var state string
		if err := tx.QueryRow(ctx,
			`SELECT state FROM event_backlog WHERE account_id = $1 AND event = $2`,
			who, offer.Event.Then).Scan(&state); err != nil {
			t.Fatalf("the successor is not in the backlog: %v", err)
		}
		if state != "waiting" {
			t.Errorf("the successor is %q, want waiting", state)
		}
	})
}

// An event marked `awaits` in content is a doorway: accepting it does not
// finish it, and the module that ran whatever it opened settles it later.
func TestAnEventThatOpensSomethingElseWaitsToBeResolved(t *testing.T) {
	world(t, func(ctx context.Context, tx pgx.Tx, e *Events, clk *clock.Stepping, who string, _ *wallet) {
		if _, err := tx.Exec(ctx, `
			INSERT INTO event_backlog (account_id, event, state, generated_at, eligible_at)
			VALUES ($1, 'a-shift-going-spare', 'waiting', $2, $2)`, who, clk.At); err != nil {
			t.Fatalf("queue it: %v", err)
		}

		offer := standingAt(t, ctx, e, tx, who, "the-strip-loading-bay")
		if offer == nil {
			t.Fatal("nothing was offered at the loading bay")
		}

		answered, err := e.Accept(ctx, tx, who, offer.ID)
		if err != nil {
			t.Fatalf("accept: %v", err)
		}
		if !answered.Open {
			t.Fatal("an event that opens something else finished on the spot")
		}

		var resolved *time.Time
		if err := tx.QueryRow(ctx,
			`SELECT resolved_at FROM event_record WHERE backlog_id = $1`, offer.ID).Scan(&resolved); err != nil {
			t.Fatalf("read the record: %v", err)
		}
		if resolved != nil {
			t.Error("the record is settled before anything settled it")
		}

		at := clk.At.Add(8 * time.Hour)
		if err := e.Resolve(ctx, tx, who, offer.ID, at); err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if state, _, _ := backlogState(t, ctx, tx, offer.ID); state != "done" {
			t.Errorf("a resolved event is %q, want done", state)
		}
		if err := tx.QueryRow(ctx,
			`SELECT resolved_at FROM event_record WHERE backlog_id = $1`, offer.ID).Scan(&resolved); err != nil {
			t.Fatalf("read the record: %v", err)
		}
		if resolved == nil || !resolved.Equal(at) {
			t.Errorf("the record settled at %v, want %v", resolved, at)
		}

		// Twice is a bug in the caller, not a second outcome.
		if err := e.Resolve(ctx, tx, who, offer.ID, at); !errors.Is(err, ErrNotOpen) {
			t.Errorf("resolving twice: %v, want ErrNotOpen", err)
		}
	})
}

/* ── revalidation ─────────────────────────────────────────────────────── */

// §4: a backlogged event is re-checked against current state at release, and
// requeued if its preconditions no longer hold. The fare inspector needs the
// fare in your pocket.
func TestAnEventWhosePreconditionsFailIsRequeuedRatherThanOffered(t *testing.T) {
	world(t, func(ctx context.Context, tx pgx.Tx, e *Events, clk *clock.Stepping, who string, held *wallet) {
		var id int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO event_backlog (account_id, event, state, generated_at, eligible_at)
			VALUES ($1, 'the-fare-inspector', 'waiting', $2, $2) RETURNING id`,
			who, clk.At).Scan(&id); err != nil {
			t.Fatalf("queue it: %v", err)
		}

		held.money = 0
		if offer := standingAt(t, ctx, e, tx, who, "downtown"); offer != nil {
			t.Fatalf("%s was offered to somebody who cannot pay", offer.Event.Key)
		}

		state, _, eligible := backlogState(t, ctx, tx, id)
		if state != "waiting" {
			t.Errorf("a requeued event is %q, want waiting", state)
		}
		if want := clk.At.Add(Requeue); !eligible.Equal(want) {
			t.Errorf("it waits until %v, want %v", eligible, want)
		}

		// It was requeued rather than dropped, so it happens once the world
		// changes back.
		held.money = 10_000
		clk.At = clk.At.Add(Requeue + time.Minute)
		if offer := standingAt(t, ctx, e, tx, who, "downtown"); offer == nil {
			t.Error("the event never came back once it could happen again")
		}
	})
}

// The other half of revalidation: an event that has waited longer than its
// patience is dropped, silently, and the row is finished rather than left to be
// reconsidered forever.
func TestAnEventOutOfPatienceIsDropped(t *testing.T) {
	world(t, func(ctx context.Context, tx pgx.Tx, e *Events, clk *clock.Stepping, who string, _ *wallet) {
		var id int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO event_backlog (account_id, event, state, generated_at, eligible_at)
			VALUES ($1, 'somebody-asking-for-directions', 'waiting', $2, $2) RETURNING id`,
			who, clk.At).Scan(&id); err != nil {
			t.Fatalf("queue it: %v", err)
		}

		authored, _ := Known("somebody-asking-for-directions")
		clk.At = clk.At.Add(authored.Patience + time.Hour)

		if offer := standingAt(t, ctx, e, tx, who, "east-side"); offer != nil && offer.ID == id {
			t.Error("an event past its patience was offered anyway")
		}
		if state, _, _ := backlogState(t, ctx, tx, id); state != "done" {
			t.Errorf("an event out of patience is %q, want done", state)
		}

		// Recorded as dropped rather than as something the player did.
		var answer string
		if err := tx.QueryRow(ctx,
			`SELECT answer FROM event_record WHERE backlog_id = $1`, id).Scan(&answer); err != nil {
			t.Fatalf("read the record: %v", err)
		}
		if answer != string(Dropped) {
			t.Errorf("it was recorded as %q, want %q", answer, Dropped)
		}
	})
}

// An event authored out of the game leaves rows behind it, and they are dropped
// rather than crashing anything.
func TestAnEventNoLongerInTheCatalogueIsDropped(t *testing.T) {
	world(t, func(ctx context.Context, tx pgx.Tx, e *Events, clk *clock.Stepping, who string, _ *wallet) {
		var id int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO event_backlog (account_id, event, state, generated_at, eligible_at)
			VALUES ($1, 'a-thing-that-was-cut', 'waiting', $2, $2) RETURNING id`,
			who, clk.At).Scan(&id); err != nil {
			t.Fatalf("queue it: %v", err)
		}

		if _, err := e.Arrived(ctx, tx, who, "east-side"); err != nil {
			t.Fatalf("arrive: %v", err)
		}
		if state, _, _ := backlogState(t, ctx, tx, id); state != "done" {
			t.Errorf("a withdrawn event is %q, want done", state)
		}
	})
}

/* ── release ──────────────────────────────────────────────────────────── */

// One thing on screen at a time, whatever the policy says. The policy here says
// yes to everything, and the module still refuses to stack two offers.
func TestOnlyOneThingIsEverOnScreen(t *testing.T) {
	world(t, func(ctx context.Context, tx pgx.Tx, e *Events, _ *clock.Stepping, who string, _ *wallet) {
		if err := e.Replenish(ctx, tx, who, today); err != nil {
			t.Fatalf("replenish: %v", err)
		}

		for range 6 {
			if _, err := e.Arrived(ctx, tx, who, "east-side"); err != nil {
				t.Fatalf("arrive: %v", err)
			}
		}

		var open int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM event_backlog WHERE account_id = $1 AND state = 'offered'`, who,
		).Scan(&open); err != nil {
			t.Fatalf("count what is open: %v", err)
		}
		if open > 1 {
			t.Errorf("%d things are on screen at once", open)
		}
	})
}

// The policy is asked, and a policy that says no means nothing is released —
// and, just as importantly, means the backlog is left exactly as it was.
func TestAPolicyThatSaysNoLeavesTheBacklogAlone(t *testing.T) {
	worldWith(t, Held{}, func(ctx context.Context, tx pgx.Tx, e *Events, clk *clock.Stepping, who string, _ *wallet) {
		if err := e.Replenish(ctx, tx, who, today); err != nil {
			t.Fatalf("replenish: %v", err)
		}

		var before time.Time
		if err := tx.QueryRow(ctx, `
			SELECT min(eligible_at) FROM event_backlog WHERE account_id = $1`, who).Scan(&before); err != nil {
			t.Fatalf("read the backlog: %v", err)
		}

		for _, place := range everywhere() {
			if offer := standingAt(t, ctx, e, tx, who, place); offer != nil {
				t.Fatalf("%s was released by a policy that releases nothing", offer.Event.Key)
			}
		}

		var after time.Time
		if err := tx.QueryRow(ctx, `
			SELECT min(eligible_at) FROM event_backlog WHERE account_id = $1`, who).Scan(&after); err != nil {
			t.Fatalf("read the backlog: %v", err)
		}
		if !after.Equal(before) {
			t.Error("a policy saying 'not now' moved the backlog's eligibility")
		}
	})
}

// The queue surface is released when the player looks; the world surface is
// not. §4's two delivery surfaces, kept apart.
func TestOnlyQueuedEventsAreSurfacedByLooking(t *testing.T) {
	world(t, func(ctx context.Context, tx pgx.Tx, e *Events, clk *clock.Stepping, who string, _ *wallet) {
		// A world event and a queue event, both placeable at home.
		for _, key := range []string{"the-lights-left-on"} {
			if _, err := tx.Exec(ctx, `
				INSERT INTO event_backlog (account_id, event, state, generated_at, eligible_at)
				VALUES ($1, $2, 'waiting', $3, $3)`, who, key, clk.At); err != nil {
				t.Fatalf("queue %s: %v", key, err)
			}
		}

		standing, released, err := e.Looked(ctx, tx, who, "east-side-apartment")
		if err != nil {
			t.Fatalf("look: %v", err)
		}
		if !released || standing.Offer == nil {
			t.Fatal("a queued event was not surfaced by looking")
		}
		if standing.Offer.Event.Surface != InQueue {
			t.Errorf("looking surfaced a %s event", standing.Offer.Event.Surface)
		}

		// Answer it, then check that looking does not surface a world event.
		if _, err := e.Accept(ctx, tx, who, standing.Offer.ID); err != nil {
			t.Fatalf("accept: %v", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO event_backlog (account_id, event, state, generated_at, eligible_at)
			VALUES ($1, 'the-till-is-short', 'waiting', $2, $2)`, who, clk.At); err != nil {
			t.Fatalf("queue it: %v", err)
		}
		_, released, err = e.Looked(ctx, tx, who, "east-side-corner-shop")
		if err != nil {
			t.Fatalf("look: %v", err)
		}
		if released {
			t.Error("looking surfaced an event that is found by walking")
		}
	})
}

// A world with nothing in it is not a world: a release path that finds an empty
// backlog fills it rather than waiting for tomorrow's rollover.
func TestAnEmptyBacklogIsFilledOnDemand(t *testing.T) {
	world(t, func(ctx context.Context, tx pgx.Tx, e *Events, _ *clock.Stepping, who string, _ *wallet) {
		if _, err := e.Arrived(ctx, tx, who, "east-side"); err != nil {
			t.Fatalf("arrive: %v", err)
		}

		var waiting int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM event_backlog WHERE account_id = $1`, who).Scan(&waiting); err != nil {
			t.Fatalf("count the backlog: %v", err)
		}
		if waiting == 0 {
			t.Error("walking into an empty world generated nothing")
		}
	})
}

/* ── the record ───────────────────────────────────────────────────────── */

// §4: declining is signal. It is counted per family, and a refusal is kept
// apart from an event that was never answered at all.
func TestDecliningIsRecordedAsSignal(t *testing.T) {
	world(t, func(ctx context.Context, tx pgx.Tx, e *Events, clk *clock.Stepping, who string, _ *wallet) {
		if err := e.Replenish(ctx, tx, who, today); err != nil {
			t.Fatalf("replenish: %v", err)
		}

		offer := firstOffer(t, ctx, e, tx, who, clk)
		kind := offer.Event.Kind
		if _, err := e.Decline(ctx, tx, who, offer.ID); err != nil {
			t.Fatalf("decline: %v", err)
		}

		tallies, err := e.Signal(ctx, tx, who, clk.At.Add(-Lately))
		if err != nil {
			t.Fatalf("read the signal: %v", err)
		}
		if len(tallies) != len(Kinds) {
			t.Fatalf("the signal has %d families, want %d", len(tallies), len(Kinds))
		}
		for _, tally := range tallies {
			if tally.Kind != kind {
				continue
			}
			if tally.Declined != 1 {
				t.Errorf("%s: declined %d, want 1", tally.Kind, tally.Declined)
			}
			if tally.Passed != 0 {
				t.Errorf("%s: a refusal was counted as walking away", tally.Kind)
			}
			if tally.Offered != 1 {
				t.Errorf("%s: offered %d, want 1", tally.Kind, tally.Offered)
			}
		}
	})
}

// The history reads back newest first, a page at a time, and a page past the
// end is clamped rather than refused.
func TestTheRecordIsPaged(t *testing.T) {
	world(t, func(ctx context.Context, tx pgx.Tx, e *Events, clk *clock.Stepping, who string, _ *wallet) {
		if err := e.Replenish(ctx, tx, who, today); err != nil {
			t.Fatalf("replenish: %v", err)
		}

		for range 3 {
			offer := firstOffer(t, ctx, e, tx, who, clk)
			if _, err := e.Decline(ctx, tx, who, offer.ID); err != nil {
				t.Fatalf("decline: %v", err)
			}
			clk.At = clk.At.Add(Rest + time.Hour)
		}

		log, err := e.Records(ctx, tx, who, 1)
		if err != nil {
			t.Fatalf("read the record: %v", err)
		}
		if log.Total != 3 {
			t.Errorf("the record holds %d, want 3", log.Total)
		}
		for i := 1; i < len(log.Records); i++ {
			if log.Records[i].At.After(log.Records[i-1].At) {
				t.Error("the record is not newest first")
			}
		}

		clamped, err := e.Records(ctx, tx, who, 99)
		if err != nil {
			t.Fatalf("read a page past the end: %v", err)
		}
		if clamped.Page != clamped.Pages {
			t.Errorf("page 99 answered page %d of %d", clamped.Page, clamped.Pages)
		}
	})
}

// Answering something that is not open is one refusal whatever the reason —
// the wrong id, one already answered, or one that lapsed.
func TestAnsweringSomethingThatIsNotOpenIsRefused(t *testing.T) {
	world(t, func(ctx context.Context, tx pgx.Tx, e *Events, clk *clock.Stepping, who string, _ *wallet) {
		if err := e.Replenish(ctx, tx, who, today); err != nil {
			t.Fatalf("replenish: %v", err)
		}

		if _, err := e.Accept(ctx, tx, who, 90210); !errors.Is(err, ErrNoOffer) {
			t.Errorf("accepting nothing: %v, want ErrNoOffer", err)
		}

		offer := firstOffer(t, ctx, e, tx, who, clk)
		if _, err := e.Accept(ctx, tx, who, offer.ID); err != nil {
			t.Fatalf("accept: %v", err)
		}
		if _, err := e.Accept(ctx, tx, who, offer.ID); !errors.Is(err, ErrNoOffer) {
			t.Errorf("accepting twice: %v, want ErrNoOffer", err)
		}
		if _, err := e.Decline(ctx, tx, who, offer.ID); !errors.Is(err, ErrNoOffer) {
			t.Errorf("declining what was accepted: %v, want ErrNoOffer", err)
		}
	})
}

/* ── helpers ──────────────────────────────────────────────────────────── */

// everywhere is every place a player can stand, in the city's order.
func everywhere() []string {
	all := city.Places()
	out := make([]string, 0, len(all))
	for _, p := range all {
		out = append(out, p.Key)
	}
	return out
}

// hasZone is the §10.3 question: does that place actually have such a spot.
func hasZone(place, zone string) bool { return city.Spot(place, zone) }

// firstOffer walks the city until something is put in front of the player.
func firstOffer(t *testing.T, ctx context.Context, e *Events, tx pgx.Tx, who string, clk *clock.Stepping) *Offer {
	t.Helper()
	for _, place := range everywhere() {
		if offer := standingAt(t, ctx, e, tx, who, place); offer != nil {
			return offer
		}
	}
	t.Fatal("the world had nothing anywhere")
	return nil
}
