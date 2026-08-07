package career

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/clock"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store/storetest"
)

/*
The module, against a real Postgres (§14).

The arithmetic is proved in internal/simulation, where the ladder is tuned and
where a fortnight of nights runs in microseconds. What is worth proving here is
everything that only exists once there is a database and two boundaries: that a
wage is paid once per day whatever the rollover does, that leaving a job does not
take back an interview, that a rung the player cannot reach is refused, and that
the day handed back to the body is the day the shift left behind.
*/

// world stands in for the two modules this package is allowed to reach.
//
// They are interfaces for exactly this reason (§3.1): the balance and the
// capability question are one field each in a test, and internal/character and
// internal/factor never come into it.
type world struct {
	// paid is every wage this player has been handed, in order. A slice rather
	// than a total, because "paid twice for one day" and "paid double once" are
	// different bugs and a total cannot tell them apart.
	paid []simulation.Money
	// able is whether the player reaches whatever they are measured against,
	// and asked is every requirement they were measured against.
	able  bool
	asked []simulation.Requirement
}

func (w *world) Wage(_ context.Context, _ pgx.Tx, _ string, amount simulation.Money) error {
	w.paid = append(w.paid, amount)
	return nil
}

func (w *world) Ready(_ context.Context, _ pgx.Tx, _ string, need simulation.Requirement) (bool, error) {
	w.asked = append(w.asked, need)
	return w.able, nil
}

func (w *world) total() simulation.Money {
	var out simulation.Money
	for _, p := range w.paid {
		out += p
	}
	return out
}

// today is the day every test in this file happens on, in the player's own
// timezone — which for a test is UTC, because nothing here is about a boundary.
var today = simulation.Date{Year: 2026, Month: time.August, Day: 6}

/*
with runs f inside one transaction on a fresh database, which is the shape every
command has (§3.2) and therefore the shape every test should have.

The transaction is rolled back by a cleanup rather than by a closure around f: a
t.Fatalf inside f ends the goroutine outright, so a closure would never return
and the connection would be held until the pool gave up.
*/
func with(t *testing.T, f func(ctx context.Context, tx pgx.Tx, m *Careers, w *world, account string)) {
	t.Helper()

	st := storetest.New(t)
	ctx := t.Context()

	tx, err := st.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	w := &world{able: true}
	clk := &clock.Stepping{At: today.At()}
	f(ctx, tx, New(clk, w, w), w, newAccount(t, ctx, tx))
}

func newAccount(t *testing.T, ctx context.Context, tx pgx.Tx) string {
	t.Helper()
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO account (provider, subject, email, display_name)
		VALUES ('google', 'career-test', 'career-test@example.com', 'Player')
		RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("create account: %v", err)
	}
	return id
}

// dock is where the bottom rung is taken on.
const dock = "the-strip-loading-bay"

// aDay is a day of ordinary routines, so that a test can see what a shift left
// of one.
func aDay() simulation.Day {
	return simulation.Day{
		Sleep:    8*time.Hour + 30*time.Minute,
		Training: simulation.Hard,
		Diet:     simulation.Clean,
		Order:    simulation.Loose,
	}
}

// Everybody starts holding nothing, which is where §3.14's ladder starts.
func TestAPlayerBeginsHoldingNothing(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Careers, _ *world, account string) {
		held, err := m.Ensure(ctx, tx, account)
		if err != nil {
			t.Fatalf("ensure: %v", err)
		}
		if held.Working() {
			t.Errorf("a new player holds %d", held.Tier)
		}
		if held.Pays() != 0 {
			t.Errorf("holding nothing pays %d", held.Pays())
		}
		if !held.Post().Zero() {
			t.Error("holding nothing asks something of the body")
		}
		if _, _, ok := held.Next(); !ok {
			t.Error("there is no step up from holding nothing")
		}
	})
}

/*
§3.14's bottom rung: it takes anybody, and it takes them where it is.

Placement is the server's, exactly as an event's and a challenge's are — which is
what stops the ladder becoming a job board somebody browses from their kitchen.
*/
func TestTheBottomRungIsWalkedIntoWhereItIs(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Careers, _ *world, account string) {
		if _, err := m.Take(ctx, tx, account, "east-side", today); !errors.Is(err, ErrNothingHiring) {
			t.Fatalf("taking a job in the east side = %v, want %v", err, ErrNothingHiring)
		}

		held, err := m.Take(ctx, tx, account, dock, today)
		if err != nil {
			t.Fatalf("take the shift: %v", err)
		}
		if held.Tier != 1 || held.Reached != 1 {
			t.Fatalf("holding tier %d, reached %d", held.Tier, held.Reached)
		}
		if held.Since.IsZero() {
			t.Error("the job has no start date")
		}
		// The ledger is stamped to the day before, so the first day paid for is
		// the first day held — and a catch-up cannot pay for employment that
		// never happened.
		if want := today.Plus(-1); held.PaidThrough != want {
			t.Errorf("paid through %v, want %v", held.PaidThrough, want)
		}

		if _, err := m.Take(ctx, tx, account, dock, today); !errors.Is(err, ErrHoldingOne) {
			t.Errorf("taking a second job = %v, want %v", err, ErrHoldingOne)
		}
	})
}

/*
§3.4: "income is **structural** — it comes from job tier … not from repeating
actions."

So a day of holding a job pays the rung's day rate, once, and a day nobody held
a job pays nothing. The pay is read off the pure core rather than written here,
because a tuning pass must move both.
*/
func TestADayHeldPaysTheRungsDayRate(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Careers, w *world, account string) {
		// Before anybody is hired: no wage, and the day is the player's own.
		day, err := m.Shift(ctx, tx, account, today, aDay())
		if err != nil {
			t.Fatalf("shift while unemployed: %v", err)
		}
		if day != aDay() {
			t.Errorf("an unemployed day became %+v", day)
		}
		if len(w.paid) != 0 {
			t.Errorf("somebody with no job was paid %v", w.paid)
		}

		if _, err := m.Take(ctx, tx, account, dock, today); err != nil {
			t.Fatalf("take the shift: %v", err)
		}

		rung, _ := simulation.Rungs().At(1)
		if _, err := m.Shift(ctx, tx, account, today, aDay()); err != nil {
			t.Fatalf("shift: %v", err)
		}
		if len(w.paid) != 1 || w.paid[0] != rung.Pay {
			t.Fatalf("paid %v, want one day at %d", w.paid, rung.Pay)
		}

		// A week of it is a week's wages and no more.
		for on := today.Next(); on.Until(today.Plus(7)) >= 0; on = on.Next() {
			if _, err := m.Shift(ctx, tx, account, on, aDay()); err != nil {
				t.Fatalf("shift %v: %v", on, err)
			}
		}
		if want := simulation.Money(8) * rung.Pay; w.total() != want {
			t.Errorf("eight days paid %d, want %d", w.total(), want)
		}
	})
}

/*
The rollover walks a calendar and may walk the same day twice — a retried job, a
second tab hydrating, a scheduled run arriving mid-catch-up. None of them may pay
twice.

This is the same property internal/health's ledger and internal/factor's tenure
both have, and it is the reason the column exists at all.
*/
func TestADayIsOnlyEverPaidForOnce(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Careers, w *world, account string) {
		if _, err := m.Take(ctx, tx, account, dock, today); err != nil {
			t.Fatalf("take the shift: %v", err)
		}

		for range 5 {
			if _, err := m.Shift(ctx, tx, account, today, aDay()); err != nil {
				t.Fatalf("shift: %v", err)
			}
		}
		if len(w.paid) != 1 {
			t.Errorf("the same day paid %d times", len(w.paid))
		}

		// And a day *before* the ledger — a catch-up walking backwards, which
		// the rollover does not do but which the ledger must survive anyway.
		if _, err := m.Shift(ctx, tx, account, today.Plus(-3), aDay()); err != nil {
			t.Fatalf("shift on an old day: %v", err)
		}
		if len(w.paid) != 1 {
			t.Errorf("a day already behind the ledger paid again: %v", w.paid)
		}
	})
}

/*
§3.14: "you hold the job, it pays, and it costs you health if your routines are
bad."

The costing itself is internal/simulation's and is tested there. What this
asserts is the wiring: the day handed back is the day the shift left, not the day
the routines described, and the two are different.
*/
func TestTheDayHandedBackIsTheDayTheShiftLeft(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Careers, _ *world, account string) {
		if _, err := m.Take(ctx, tx, account, dock, today); err != nil {
			t.Fatalf("take the shift: %v", err)
		}

		day, err := m.Shift(ctx, tx, account, today, aDay())
		if err != nil {
			t.Fatalf("shift: %v", err)
		}
		if day == aDay() {
			t.Fatal("a night on the loading bay cost nothing")
		}
		if want := simulation.Working(aDay(), simulation.Rungs().Post(1)); day != want {
			t.Errorf("lived %+v, want %+v", day, want)
		}
	})
}

/*
§3.14's step up: capability *and* an interview. Neither on its own.

The dispatch desk cannot be walked into by somebody who has never sat its
interview, however capable they are — and the interview itself is refused to
somebody standing on the wrong rung or short of the requirement.
*/
func TestTheStepUpNeedsBothCapabilityAndAnInterview(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Careers, w *world, account string) {
		// Standing at the desk with everything the rung asks for, and no
		// interview behind them.
		if _, err := m.Take(ctx, tx, account, "the-strip-warehouse", today); !errors.Is(err, ErrNotHiredYet) {
			t.Fatalf("walking into the desk job = %v, want %v", err, ErrNotHiredYet)
		}

		// Nor may they sit the interview from the ground: §3.14's ladder is
		// climbed a rung at a time.
		sitting, err := m.Sitting(ctx, tx, account, Dispatch.Interview)
		if err != nil {
			t.Fatalf("sitting: %v", err)
		}
		if sitting {
			t.Error("somebody holding no job may sit the interview for tier 2")
		}

		if _, err := m.Take(ctx, tx, account, dock, today); err != nil {
			t.Fatalf("take the shift: %v", err)
		}
		if sitting, err = m.Sitting(ctx, tx, account, Dispatch.Interview); err != nil || !sitting {
			t.Fatalf("sitting from the rung below = %v, %v", sitting, err)
		}
		// And what it was measured against is the rung's own requirement.
		rung, _ := simulation.Rungs().At(2)
		if last := w.asked[len(w.asked)-1]; last != rung.Needs {
			t.Errorf("measured against %+v, want %+v", last, rung.Needs)
		}

		// A body that has fallen below the floor is refused, which is §3.14's
		// trap: the job at the bottom takes the thing the step out of it needs.
		w.able = false
		if sitting, err = m.Sitting(ctx, tx, account, Dispatch.Interview); err != nil || sitting {
			t.Fatalf("sitting without the capability = %v, %v", sitting, err)
		}
	})
}

// A challenge that is not an interview is nothing to do with this package, and
// gating every doorway through it must not close the ordinary ones.
func TestAChallengeThatIsNotAnInterviewIsAlwaysSittable(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Careers, _ *world, account string) {
		for _, code := range []string{"resolve_conflict/a", "resolve_conflict/b", ""} {
			sitting, err := m.Sitting(ctx, tx, account, code)
			if err != nil || !sitting {
				t.Errorf("sitting %q = %v, %v", code, sitting, err)
			}
		}
	})
}

/*
Passing the interview is the offer, and the outcome key is what decides it.

Not a score: §3.10's scoring is "never surfaced as a grade, never confirmed as
right", and nothing outside internal/challenge may read one. Which bands hire is
an authored decision about the *role*, which is why it is a field on Role.
*/
func TestPassingTheInterviewTakesTheRung(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Careers, _ *world, account string) {
		if _, err := m.Take(ctx, tx, account, dock, today); err != nil {
			t.Fatalf("take the shift: %v", err)
		}

		// An outcome that is not an offer changes nothing, and is not an error:
		// the player is already being told how it went, in the reaction the
		// content authored for it.
		held, took, err := m.Interviewed(ctx, tx, account, Dispatch.Interview, "not-this-one", today)
		if err != nil {
			t.Fatalf("interviewed: %v", err)
		}
		if took || held.Tier != 1 {
			t.Fatalf("an outcome that is not an offer hired them: %v, tier %d", took, held.Tier)
		}

		held, took, err = m.Interviewed(ctx, tx, account, Dispatch.Interview, Dispatch.Offered[0], today)
		if err != nil {
			t.Fatalf("interviewed: %v", err)
		}
		if !took || held.Tier != 2 || held.Reached != 2 {
			t.Fatalf("hired = %v, holding tier %d, reached %d", took, held.Tier, held.Reached)
		}

		// A challenge that is not an interview cannot hire anybody, whatever it
		// landed in.
		if _, took, err = m.Interviewed(ctx, tx, account, "resolve_conflict/a", "moved", today); err != nil || took {
			t.Errorf("an ordinary challenge hired somebody: %v, %v", took, err)
		}
	})
}

/*
A promotion does not reset the wage ledger.

The day somebody is promoted is a day they were already being paid for, and
stamping the ledger back a day would pay for it twice — at the new rate.
*/
func TestAPromotionDoesNotPayTheSameDayTwice(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Careers, w *world, account string) {
		if _, err := m.Take(ctx, tx, account, dock, today); err != nil {
			t.Fatalf("take the shift: %v", err)
		}
		if _, err := m.Shift(ctx, tx, account, today, aDay()); err != nil {
			t.Fatalf("shift: %v", err)
		}

		if _, _, err := m.Interviewed(ctx, tx, account, Dispatch.Interview, Dispatch.Offered[0], today); err != nil {
			t.Fatalf("interviewed: %v", err)
		}
		if _, err := m.Shift(ctx, tx, account, today, aDay()); err != nil {
			t.Fatalf("shift after the promotion: %v", err)
		}
		if len(w.paid) != 1 {
			t.Errorf("the day of the promotion was paid %d times: %v", len(w.paid), w.paid)
		}

		// Tomorrow pays at the new rate, which is the whole reason anybody
		// climbed.
		if _, err := m.Shift(ctx, tx, account, today.Next(), aDay()); err != nil {
			t.Fatalf("shift tomorrow: %v", err)
		}
		second, _ := simulation.Rungs().At(2)
		if w.paid[len(w.paid)-1] != second.Pay {
			t.Errorf("the day after paid %d, want %d", w.paid[len(w.paid)-1], second.Pay)
		}
	})
}

/*
Leaving a job stops the wages and keeps the interview.

The second half is the load-bearing one. A challenge is once ever (§5.1), so a
player who walked away from the dispatch desk has nothing left to prove — asking
them to prove it again would be asking for something the game structurally
cannot give.
*/
func TestLeavingStopsTheWagesAndKeepsWhatWasEarned(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Careers, w *world, account string) {
		if _, err := m.Leave(ctx, tx, account); !errors.Is(err, ErrNotHoldingOne) {
			t.Fatalf("leaving nothing = %v, want %v", err, ErrNotHoldingOne)
		}

		if _, err := m.Take(ctx, tx, account, dock, today); err != nil {
			t.Fatalf("take the shift: %v", err)
		}
		if _, _, err := m.Interviewed(ctx, tx, account, Dispatch.Interview, Dispatch.Offered[0], today); err != nil {
			t.Fatalf("interviewed: %v", err)
		}

		held, err := m.Leave(ctx, tx, account)
		if err != nil {
			t.Fatalf("leave: %v", err)
		}
		if held.Working() || held.Reached != 2 {
			t.Fatalf("after leaving: tier %d, reached %d", held.Tier, held.Reached)
		}
		if !held.Since.IsZero() {
			t.Error("a job nobody holds has a start date")
		}

		w.paid = nil
		if _, err := m.Shift(ctx, tx, account, today.Next(), aDay()); err != nil {
			t.Fatalf("shift after leaving: %v", err)
		}
		if len(w.paid) != 0 {
			t.Errorf("somebody who left was paid %v", w.paid)
		}

		// And they can walk back into it, because the interview is behind them.
		held, err = m.Take(ctx, tx, account, "the-strip-warehouse", today)
		if err != nil {
			t.Fatalf("walk back into the desk job: %v", err)
		}
		if held.Tier != 2 {
			t.Errorf("walked back into tier %d", held.Tier)
		}
	})
}

/*
Capability is asked about every rung, including the one anybody can have.

The bottom rung's requirement is empty, so the answer is always yes — but the
*question* is asked through the same interface, which is what keeps a tuning pass
that gives tier 1 a floor from silently doing nothing.
*/
func TestEveryHiringAsksAboutCapability(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Careers, w *world, account string) {
		w.able = false
		if _, err := m.Take(ctx, tx, account, dock, today); !errors.Is(err, ErrNotCapable) {
			t.Fatalf("take = %v, want %v", err, ErrNotCapable)
		}
		first, _ := simulation.Rungs().At(1)
		if len(w.asked) != 1 || w.asked[0] != first.Needs {
			t.Errorf("measured against %+v, want %+v", w.asked, first.Needs)
		}
	})
}

/*
§3.14 splits the ladder in half, and the top three rungs are #28's.

They are on the ladder — "the ladder itself teaches the trade" needs a ladder
somebody can see — and there is no way into them, because a rung that could be
taken before the thing it does exists would be a salary paid for nothing.
*/
func TestTheActiveTiersAreVisibleAndOutOfReach(t *testing.T) {
	t.Parallel()

	for _, r := range Roles {
		switch {
		case r.Tier.Passive():
			if !r.Hired.Anywhere() {
				t.Errorf("%s is a passive tier that is taken on nowhere", r.Key)
			}
		default:
			if r.Hired.Anywhere() {
				t.Errorf("%s is an active tier and can be walked into", r.Key)
			}
			if r.Interview != "" {
				t.Errorf("%s is an active tier with an interview behind it", r.Key)
			}
		}
		if r.Name == "" || r.Does == "" || r.Costs == "" {
			t.Errorf("%s is a role with nothing said about it", r.Key)
		}
		if _, known := simulation.Rungs().At(r.Tier); !known {
			t.Errorf("%s is tier %d, which is not on the ladder", r.Key, r.Tier)
		}
	}

	// And every rung of the ladder has a role, or the screen has a gap in it.
	for _, rung := range simulation.Rungs() {
		if _, ok := Of(rung.Tier); !ok {
			t.Errorf("tier %d is on the ladder and has no role", rung.Tier)
		}
	}
}

// One place hires one role, or "the job here" is decided by directory order.
func TestNoPlaceHiresTwoRoles(t *testing.T) {
	t.Parallel()

	taken := map[string]string{}
	for _, r := range Roles {
		for _, place := range r.Hired.Places() {
			if first, clash := taken[place]; clash {
				t.Errorf("%s and %s are both taken on at %s", first, r.Key, place)
			}
			taken[place] = r.Key
		}
	}
	if len(taken) == 0 {
		t.Fatal("no role is taken on anywhere, so nobody can ever hold a job")
	}
}
