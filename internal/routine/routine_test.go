package routine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store/storetest"
)

/*
The module, against a real Postgres (§14).

What is worth proving here is not that a row round-trips. It is the two rules
the ticket is actually about: the configuration is one unit — stated whole,
written whole, and never patched a field at a time — and a routine nobody has
earned cannot be configured, which is the gate #32 replaces.
*/

// with runs f inside one transaction on a fresh database, which is the shape
// every command has (§3.2) and therefore the shape every test should have.
func with(t *testing.T, f func(ctx context.Context, tx pgx.Tx, m *Routines, account string)) {
	t.Helper()
	gated(t, Open{}, f)
}

// gated is with, for a test that cares which routines are unlocked.
//
// The transaction is rolled back by a cleanup rather than by a closure around
// f, because a t.Fatalf inside f ends the goroutine outright — a closure would
// never return, and the connection would be held until the pool gave up.
// Nothing here needs to commit: what is being tested is what the module does,
// not that Postgres remembers it afterwards.
func gated(t *testing.T, gate Gate, f func(ctx context.Context, tx pgx.Tx, m *Routines, account string)) {
	t.Helper()
	st := storetest.New(t)
	ctx := t.Context()

	tx, err := st.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	f(ctx, tx, New(gate), newAccount(t, ctx, tx))
}

// newAccount makes somebody for the routines to belong to. The routine table
// references account, so there has to be one.
func newAccount(t *testing.T, ctx context.Context, tx pgx.Tx) string {
	t.Helper()
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO account (provider, subject, email, display_name)
		VALUES ('google', 'routine-test', 'routine@example.com', 'Routine')
		RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("create account: %v", err)
	}
	return id
}

// shut is a gate with nothing open — what the world looks like before a player
// has been through anything (§3.6).
type shut struct{ open map[string]bool }

func (s shut) Unlocked(context.Context, pgx.Tx, string) (map[string]bool, error) {
	return s.open, nil
}

func TestAPlayerWhoHasSetNothingStillHasRoutines(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Routines, account string) {
		c, err := m.Of(ctx, tx, account)
		if err != nil {
			t.Fatalf("read: %v", err)
		}

		if c.Set {
			t.Error("a player who has never configured anything reads as configured")
		}
		if len(c.Choices) != len(Kinds) {
			t.Errorf("read %d answers, want one per routine (%d)", len(c.Choices), len(Kinds))
		}
		for _, k := range Kinds {
			if c.Choices[k.Key] != k.Default() {
				t.Errorf("%s defaulted to %q, want %q", k.Key, c.Choices[k.Key], k.Default())
			}
		}

		// And the rules those defaults add up to are an unarranged life rather
		// than an empty struct: the drift is a routine too.
		if got := c.Routine(); got.Sleep != 5*time.Hour+30*time.Minute || got.Diet != simulation.Poor {
			t.Errorf("the default routine is %+v, want the life that happens on its own", got)
		}
	})
}

func TestTheConfigurationIsWrittenAndReadWhole(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Routines, account string) {
		first, err := m.Set(ctx, tx, account, Choices{
			"sleep": "protected", "workout": "split", "diet": "cooked", "organization": "listed",
		})
		if err != nil {
			t.Fatalf("set: %v", err)
		}
		if !first.Set {
			t.Error("configuring the routines did not record that they had been configured")
		}

		read, err := m.Of(ctx, tx, account)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		for key, want := range first.Choices {
			if read.Choices[key] != want {
				t.Errorf("%s read back as %q, want %q", key, read.Choices[key], want)
			}
		}

		// The load-bearing half: a second statement replaces the whole
		// document. Sleep is left out, so sleep goes back to what an
		// unconfigured life does — it is not quietly kept from last time.
		second, err := m.Set(ctx, tx, account, Choices{"workout": "daily"})
		if err != nil {
			t.Fatalf("set again: %v", err)
		}
		if got := second.Choices["sleep"]; got != Sleep.Default() {
			t.Errorf("sleep = %q after a configuration that did not mention it, want %q — the unit is the whole set",
				got, Sleep.Default())
		}
		if got := second.Choices["workout"]; got != "daily" {
			t.Errorf("workout = %q, want %q", got, "daily")
		}
	})
}

func TestAnAnswerOutsideTheVocabularyIsRefused(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Routines, account string) {
		if _, err := m.Set(ctx, tx, account, Choices{"sleep": "eight-ish"}); !errors.Is(err, ErrNoSuchOption) {
			t.Errorf("an invented answer returned %v, want ErrNoSuchOption", err)
		}
		if _, err := m.Set(ctx, tx, account, Choices{"posture": "upright"}); !errors.Is(err, ErrNoSuchRoutine) {
			t.Errorf("an invented routine returned %v, want ErrNoSuchRoutine", err)
		}

		// And neither wrote anything: a refused command is a command that
		// never happened.
		c, err := m.Of(ctx, tx, account)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if c.Set {
			t.Error("a refused configuration still recorded that the routines had been set")
		}
	})
}

/*
Struggle before mastery (§3.6), which is the gate #32 replaces.

Nothing in the shipped build is locked — Open is bound in internal/api — so what
is proved here is that the *mechanism* works when something is: a locked routine
may be left at its default and may not be arranged, and the refusal is a state of
the world rather than a bad request.
*/
func TestALockedRoutineCannotBeConfigured(t *testing.T) {
	t.Parallel()

	// Everything but sleep, which this player has not earned yet.
	open := map[string]bool{}
	for _, k := range Kinds {
		open[k.Key] = k.Key != Sleep.Key
	}

	gated(t, shut{open}, func(ctx context.Context, tx pgx.Tx, m *Routines, account string) {
		_, err := m.Set(ctx, tx, account, Choices{"sleep": "protected", "workout": "split"})
		if !errors.Is(err, ErrLocked) {
			t.Fatalf("arranging a locked routine returned %v, want ErrLocked", err)
		}

		// The rest of the configuration is still settable, as long as the
		// locked one is left where it is.
		c, err := m.Set(ctx, tx, account, Choices{"workout": "split", "diet": "cooked"})
		if err != nil {
			t.Fatalf("set without touching the locked routine: %v", err)
		}
		if c.Unlocked[Sleep.Key] {
			t.Error("sleep reads as unlocked")
		}
		if c.Choices["workout"] != "split" {
			t.Errorf("workout = %q, want split", c.Choices["workout"])
		}
	})
}

// Every routine ships with a working vocabulary. Cheap, and it catches the
// authoring mistake that would otherwise show up as a screen with a blank on it.
func TestEveryRoutineIsAnsweredByEveryOneOfItsOptions(t *testing.T) {
	t.Parallel()

	seen := map[string]bool{}
	for _, k := range Kinds {
		if seen[k.Key] {
			t.Errorf("two routines share the key %q", k.Key)
		}
		seen[k.Key] = true

		if k.Name == "" || k.Asks == "" || k.Learn == "" {
			t.Errorf("%s is missing the words it is read with: %+v", k.Key, k)
		}
		if len(k.Options) < 2 {
			t.Errorf("%s offers %d options — a routine with one answer is not a choice", k.Key, len(k.Options))
		}

		keys := map[string]bool{}
		for _, o := range k.Options {
			if o.Says == "" || o.Costs == "" {
				t.Errorf("%s/%s has no sentence or no price", k.Key, o.Key)
			}
			if o.into == nil {
				t.Errorf("%s/%s does nothing to a routine", k.Key, o.Key)
			}
			if keys[o.Key] {
				t.Errorf("%s offers %q twice", k.Key, o.Key)
			}
			keys[o.Key] = true
		}
	}
}

// Every option is reachable through the module, and each one changes the rules
// it claims to. A configuration that produced the same body whatever was chosen
// would be a screen of decorative buttons.
func TestEveryOptionChangesTheRulesItNames(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Routines, account string) {
		for _, k := range Kinds {
			var last simulation.Routine
			for i, o := range k.Options {
				c, err := m.Set(ctx, tx, account, Choices{k.Key: o.Key})
				if err != nil {
					t.Fatalf("set %s = %s: %v", k.Key, o.Key, err)
				}
				got := c.Routine()
				if i > 0 && got == last {
					t.Errorf("%s/%s produces the same rules as the option before it", k.Key, o.Key)
				}
				last = got
			}
		}
	})
}

// The seam the rollover (#10) will stand in: routines produce the day, and the
// day is the one the pure core would produce for that date.
func TestTheDayAPlayerLivesComesFromTheirRoutines(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Routines, account string) {
		c, err := m.Set(ctx, tx, account, Choices{
			"sleep": "protected", "workout": "split", "diet": "cooked", "organization": "regimented",
		})
		if err != nil {
			t.Fatalf("set: %v", err)
		}

		// A Monday, which the split routine trains hard on.
		on := simulation.Date{Year: 2026, Month: time.August, Day: 3}
		day, err := m.Day(ctx, tx, account, on)
		if err != nil {
			t.Fatalf("day: %v", err)
		}
		if want := c.Routine().On(on); day != want {
			t.Errorf("the day lived is %+v, the rules say %+v", day, want)
		}
		if day.Training != simulation.Hard {
			t.Errorf("Monday trained %v on a routine with three hard sessions in it", day.Training)
		}
		if day.Order != simulation.Regimented {
			t.Errorf("the day carries Order %v, want the organization routine's", day.Order)
		}
	})
}
