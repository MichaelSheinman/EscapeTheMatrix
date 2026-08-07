package factor

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store/storetest"
)

/*
The module, against a real Postgres (§14).

The arithmetic is proved in internal/simulation, where it runs in milliseconds
and where the weights are tuned. What is worth proving here is the part that only
exists once there is a database and a boundary: that Karma is composed from rows
rather than stored in one, that the hidden factors are tracked and never leave,
that credit only arrives from a committed choice, and that an environment wears
out and a retried rollover cannot wear it out twice.
*/

// world is the four things the module reads, standing in for four modules.
//
// They are interfaces in the package for exactly this reason (§3.1): the body,
// the balance, the environment and the population are one struct each in a test,
// and internal/health, internal/character, internal/city and internal/account
// never come into it.
//
// `bodies` and `purses` are the per-account overrides the board tests need — a
// leaderboard is only interesting when the players on it differ. Everybody else
// reads the same default, which is what the single-player tests want.
type world struct {
	body  simulation.Body
	held  simulation.Money
	where string

	bodies   map[string]simulation.Body
	purses   map[string]simulation.Money
	everyone []string
	names    map[string]string
}

func (w *world) Body(_ context.Context, _ pgx.Tx, accountID string) (simulation.Body, error) {
	if body, ok := w.bodies[accountID]; ok {
		return body, nil
	}
	return w.body, nil
}

func (w *world) Balance(_ context.Context, _ pgx.Tx, accountID string) (simulation.Money, error) {
	if held, ok := w.purses[accountID]; ok {
		return held, nil
	}
	return w.held, nil
}

func (w *world) Where(context.Context, pgx.Tx, string) (string, error) {
	return w.where, nil
}

func (w *world) Everyone(context.Context, pgx.Tx) ([]string, error) {
	return append([]string(nil), w.everyone...), nil
}

func (w *world) Identify(_ context.Context, _ pgx.Tx, ids []string) ([]Player, error) {
	out := make([]Player, 0, len(ids))
	for _, id := range ids {
		out = append(out, Player{ID: id, Name: w.names[id], Avatar: "https://example.test/" + id})
	}
	return out, nil
}

// with runs f inside one transaction on a fresh database, which is the shape
// every command has (§3.2) and therefore the shape every test should have.
//
// The transaction is rolled back by a cleanup rather than by a closure around f:
// a t.Fatalf inside f ends the goroutine outright, so a closure would never
// return and the connection would be held until the pool gave up.
func with(t *testing.T, f func(ctx context.Context, tx pgx.Tx, m *Factors, w *world, account string)) {
	t.Helper()

	st := storetest.New(t)
	ctx := t.Context()

	tx, err := st.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	w := &world{
		body:   simulation.Starting(1.78, 78),
		held:   simulation.Dollars(1_200),
		where:  "east-side",
		bodies: map[string]simulation.Body{},
		purses: map[string]simulation.Money{},
		names:  map[string]string{},
	}
	account := newAccount(t, ctx, tx, "factor-test")
	w.everyone = []string{account}
	w.names[account] = "Factor"

	f(ctx, tx, New(w, w, w, w), w, account)
}

func newAccount(t *testing.T, ctx context.Context, tx pgx.Tx, subject string) string {
	t.Helper()
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO account (provider, subject, email, display_name)
		VALUES ('google', $1, $1 || '@example.com', 'Player ' || $1)
		RETURNING id`, subject).Scan(&id); err != nil {
		t.Fatalf("create account: %v", err)
	}
	return id
}

/*
§3.3: everything derived is computed on read, and only the inputs are persisted.

The strongest form of that is a schema with nowhere to put the answer, which is
what this asserts — no column anywhere holds a Karma, so there is no stale one to
find.
*/
func TestNothingInTheDatabaseHoldsAKarma(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Factors, w *world, account string) {
		var columns []string
		rows, err := tx.Query(ctx, `
			SELECT table_name || '.' || column_name
			  FROM information_schema.columns
			 WHERE table_schema = 'public'
			   AND (column_name ILIKE '%karma%' OR column_name ILIKE '%score%')`)
		if err != nil {
			t.Fatalf("read the schema: %v", err)
		}
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				t.Fatalf("read the schema: %v", err)
			}
			columns = append(columns, name)
		}
		rows.Close()

		if len(columns) > 0 {
			t.Fatalf("the schema has somewhere to store a composite: %v", columns)
		}

		// And it composes anyway, from a body and a balance nobody wrote here.
		standing, err := m.Of(ctx, tx, account)
		if err != nil {
			t.Fatalf("compose: %v", err)
		}
		if standing.Karma <= 0 {
			t.Fatalf("a new player composed to %v", standing.Karma)
		}
	})
}

// The composite answers to the world it is a reading of, with nothing in this
// module's own tables having changed.
func TestKarmaFollowsTheBodyAndTheBalance(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Factors, w *world, account string) {
		before, err := m.Of(ctx, tx, account)
		if err != nil {
			t.Fatalf("compose: %v", err)
		}

		w.held = simulation.Dollars(90_000)
		richer, err := m.Of(ctx, tx, account)
		if err != nil {
			t.Fatalf("compose: %v", err)
		}
		if richer.Karma <= before.Karma {
			t.Fatalf("earning $88,800 did not move the composite: %v then %v", before.Karma, richer.Karma)
		}

		// And it falls again, which is the property that stops it being a level
		// bar under another name (§3.5).
		w.held = simulation.Dollars(20)
		w.body.Fitness = 21
		w.body.SleepDebt = 26
		spent, err := m.Of(ctx, tx, account)
		if err != nil {
			t.Fatalf("compose: %v", err)
		}
		if spent.Karma >= before.Karma {
			t.Fatalf("a wrecked body and an empty account read %v, no lower than the %v it started at", spent.Karma, before.Karma)
		}
	})
}

/*
The boundary, and the point of the package.

Three factors are real, stored, and moving the composite — and there is nothing
in what leaves this module that names them, values them, or distinguishes one
from another.
*/
func TestTheHiddenFactorsAreTrackedAndNeverSurfaced(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Factors, w *world, account string) {
		before, err := m.Of(ctx, tx, account)
		if err != nil {
			t.Fatalf("compose: %v", err)
		}

		done, err := m.Record(ctx, tx, account,
			simulation.Grant{Factor: simulation.Kindness, Credit: 300},
			simulation.Grant{Factor: simulation.Intelligence, Credit: 150},
		)
		if err != nil {
			t.Fatalf("record: %v", err)
		}

		// Tracked: the composite moved, and nothing about the body or the
		// balance did.
		if done.Standing.Karma <= before.Karma {
			t.Fatalf("credit in two hidden factors did not move the composite: %v then %v", before.Karma, done.Standing.Karma)
		}

		// Never surfaced: five slots, two of which say anything at all.
		if len(done.Standing.Slots) != len(simulation.Factors) {
			t.Fatalf("the board has %d slots, want one per factor", len(done.Standing.Slots))
		}
		shown := 0
		for _, slot := range done.Standing.Slots {
			if slot.Shown {
				shown++
				continue
			}
			if slot.Key != "" || slot.Name != "" || slot.Value != nil || slot.Reads != "" {
				t.Fatalf("a hidden factor left the module carrying something: %+v", slot)
			}
		}
		if shown != 2 {
			t.Fatalf("%d slots are shown, want the two §3.5 reveals", shown)
		}

		// The two that are shown are the readings the design surfaces them as,
		// and not their 0–100 scores — a score would state half the formula.
		health := done.Standing.Slots[0]
		money := done.Standing.Slots[1]
		if health.Key != "health" || health.Reads != "age" {
			t.Fatalf("the first slot is %+v, want the biological age", health)
		}
		if *health.Value != simulation.BiologicalAge(w.body) {
			t.Fatalf("health reads %v rather than the biological age %v", *health.Value, simulation.BiologicalAge(w.body))
		}
		if money.Key != "money" || *money.Value != float64(w.held.Cents()) {
			t.Fatalf("the money slot is %+v, want the balance held", money)
		}
	})
}

// §3.5 and §7: the two revealed factors are read from the world and are nobody's
// to award. A caller trying to grant one is refused rather than quietly landing
// in a total that no longer means what it says.
func TestHealthAndMoneyCannotBeGranted(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Factors, w *world, account string) {
		for _, f := range []simulation.Factor{simulation.Health, simulation.Wealth} {
			_, err := m.Record(ctx, tx, account, simulation.Grant{Factor: f, Credit: 100})
			if !errors.Is(err, ErrNotGranted) {
				t.Fatalf("granting %s returned %v, want ErrNotGranted", f, err)
			}
		}
	})
}

// Credit is a running total, not a snapshot: two committed choices add up, and
// a choice that costs takes some back without digging a hole (§5.1).
func TestCreditAccumulatesAndCanBeSpent(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Factors, w *world, account string) {
		for range 3 {
			if _, err := m.Record(ctx, tx, account,
				simulation.Grant{Factor: simulation.Stability, Credit: 40}); err != nil {
				t.Fatalf("record: %v", err)
			}
		}
		grown, err := m.Of(ctx, tx, account)
		if err != nil {
			t.Fatalf("compose: %v", err)
		}

		if _, err := m.Record(ctx, tx, account,
			simulation.Grant{Factor: simulation.Stability, Credit: -10_000}); err != nil {
			t.Fatalf("record: %v", err)
		}
		emptied, err := m.Of(ctx, tx, account)
		if err != nil {
			t.Fatalf("compose: %v", err)
		}

		if emptied.Karma >= grown.Karma {
			t.Fatalf("a costly choice did not lower the composite: %v then %v", grown.Karma, emptied.Karma)
		}

		// And the total is floored rather than negative, so one bad afternoon is
		// not unrecoverable.
		var credit float64
		if err := tx.QueryRow(ctx,
			`SELECT credit FROM factor_credit WHERE account_id = $1 AND factor = 'stability'`,
			account).Scan(&credit); err != nil {
			t.Fatalf("read credit: %v", err)
		}
		if credit != 0 {
			t.Fatalf("stability was driven to %v", credit)
		}
	})
}

/*
§3.15: the same effort pays less the more thoroughly a place has been worked, and
relocating restores the rate.

This is the decay that makes travel the progression vehicle rather than a side
activity, and it is per environment — a player who leaves and returns finds the
place exactly as worn out as they left it.
*/
func TestAnEnvironmentWearsOutAndAnotherOneDoesNot(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Factors, w *world, account string) {
		first, err := m.Record(ctx, tx, account, simulation.Grant{Factor: simulation.Kindness, Credit: 200})
		if err != nil {
			t.Fatalf("record: %v", err)
		}
		if first.Paid != first.Offered {
			t.Fatalf("the first choice in a new place paid %v of %v offered", first.Paid, first.Offered)
		}

		// Keep working the same place.
		for range 4 {
			if _, err := m.Record(ctx, tx, account,
				simulation.Grant{Factor: simulation.Kindness, Credit: 200}); err != nil {
				t.Fatalf("record: %v", err)
			}
		}
		worn, err := m.Record(ctx, tx, account, simulation.Grant{Factor: simulation.Kindness, Credit: 200})
		if err != nil {
			t.Fatalf("record: %v", err)
		}
		if worn.Paid >= first.Paid/2 {
			t.Fatalf("after five more of the same, the place still paid %v of the %v it paid on day one", worn.Paid, first.Paid)
		}
		if worn.Paid <= 0 {
			t.Fatalf("the place paid %v — §3.15: returns decay but never go negative", worn.Paid)
		}

		// Somewhere else is worth all of what it offers.
		w.where = "riverfront"
		fresh, err := m.Record(ctx, tx, account, simulation.Grant{Factor: simulation.Kindness, Credit: 200})
		if err != nil {
			t.Fatalf("record: %v", err)
		}
		if fresh.Paid != fresh.Offered {
			t.Fatalf("a place nobody has worked paid %v of %v", fresh.Paid, fresh.Offered)
		}

		// And going back does not refresh the first one.
		w.where = "east-side"
		returned, err := m.Of(ctx, tx, account)
		if err != nil {
			t.Fatalf("compose: %v", err)
		}
		if returned.Familiarity >= 0.5 {
			t.Fatalf("a lap of the city restored the East Side to %v", returned.Familiarity)
		}
	})
}

// A cost is not effort, so it does not wear a place out — otherwise a player
// could exhaust an environment by having a bad time in it.
func TestOnlyRewardsWearAPlaceOut(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Factors, w *world, account string) {
		for range 20 {
			if _, err := m.Record(ctx, tx, account,
				simulation.Grant{Factor: simulation.Kindness, Credit: -50}); err != nil {
				t.Fatalf("record: %v", err)
			}
		}
		standing, err := m.Of(ctx, tx, account)
		if err != nil {
			t.Fatalf("compose: %v", err)
		}
		if standing.Familiarity != 1 {
			t.Fatalf("twenty costly choices wore the place down to %v", standing.Familiarity)
		}
	})
}

/*
The rollover's half: a day of standing somewhere counts, and counts once.

§3.3 runs Day per missed day rather than per run, so a player caught up from a
week away loses a week of it — and the guard is strictly forward, so a retried
job, a second tab hydrating, and the scheduled rollover arriving mid-catch-up all
reach the same state.
*/
func TestADayIsCountedOnceAgainstThePlaceItWasSpent(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Factors, w *world, account string) {
		day := simulation.Date{Year: 2026, Month: 8, Day: 6}

		if err := m.Day(ctx, tx, account, day); err != nil {
			t.Fatalf("age: %v", err)
		}
		if err := m.Day(ctx, tx, account, day); err != nil {
			t.Fatalf("age again: %v", err)
		}
		// And a day before the last one applied, which is what a job retried
		// out of order looks like.
		if err := m.Day(ctx, tx, account, day.Plus(-1)); err != nil {
			t.Fatalf("age backwards: %v", err)
		}

		standing, err := m.Of(ctx, tx, account)
		if err != nil {
			t.Fatalf("compose: %v", err)
		}
		if standing.Days != 1 {
			t.Fatalf("one day, applied three times, counted as %d", standing.Days)
		}

		last := day.Plus(6)
		for on := day.Next(); !last.Before(on); on = on.Next() {
			if err := m.Day(ctx, tx, account, on); err != nil {
				t.Fatalf("age %v: %v", on, err)
			}
		}
		caught, err := m.Of(ctx, tx, account)
		if err != nil {
			t.Fatalf("compose: %v", err)
		}
		if caught.Days != 7 {
			t.Fatalf("a week of catch-up counted as %d days", caught.Days)
		}

		// Elapsed days alone barely move the rate — §3.15 wants decay to track
		// mastery, not the calendar.
		if caught.Familiarity < 0.9 {
			t.Fatalf("a week of doing nothing left familiarity at %v", caught.Familiarity)
		}
	})
}
