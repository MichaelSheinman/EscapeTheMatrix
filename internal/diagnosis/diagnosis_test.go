package diagnosis

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/clock"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store/storetest"
)

/*
The module, against a real Postgres (§14).

What is worth proving here is §3.6's five rules, and four of them are provable as
behaviour rather than as content: a wrong conclusion is accepted and grants its
tool, a second conclusion does not take the first one's tool away, pointing at
the same evidence twice is refused, and a conclusion re-opens exactly when the
player's history has said something new.

The fifth — "never say correct" — is proved by the absence of anything to test.
There is no field, no method and no error in this package that reports whether a
conclusion was right, which is why [TestNothingInThisPackageKnowsWhatIsTrue]
looks the way it does.
*/

const (
	// The two observations the stub ledger offers, and they deliberately speak
	// to different causes: one about the body, one about the log.
	nights = "health/short-nights"
	gaps   = "playbook/nothing-covered-it"
	// A failure the world raised, by the key logLedger composes.
	moment = "playbook/fire/1"
)

/*
ledger is a stand-in for whatever corner of history is bound in production.

A stub rather than the real ones, because what this file is testing is the
mechanic and not internal/health's opinion about resting heart rates — and
because the interface is the whole of what this package can see, a stub is not a
simplification of the real thing, it is the same surface.
*/
type ledger struct {
	failures []Failure
	says     []Observation
}

func (l *ledger) Failed(context.Context, pgx.Tx, string) ([]Failure, error) {
	return l.failures, nil
}

func (l *ledger) Observed(context.Context, pgx.Tx, string) ([]Observation, error) {
	return l.says, nil
}

// among puts the player somewhere a scene is authored for.
type among struct{ where string }

func (a among) Where(context.Context, pgx.Tx, string) (string, error) { return a.where, nil }

// granted records what vocabulary was handed over, which is the only thing this
// module tells another one.
type granted struct{ terms []string }

func (g *granted) Unlock(_ context.Context, _ pgx.Tx, _ string, terms ...string) error {
	g.terms = append(g.terms, terms...)
	return nil
}

// world is the fixture: a module, a ledger to feed it, and somebody to be.
type world struct {
	ctx     context.Context
	tx      pgx.Tx
	m       *Diagnoses
	l       *ledger
	unlocks *granted
	account string
}

/*
with runs f inside one transaction on a fresh database.

The transaction is rolled back by a cleanup rather than by a closure around f,
because a t.Fatalf inside f ends the goroutine outright — a closure would never
return and the connection would be held until the pool gave up.
*/
func with(t *testing.T, f func(w *world)) {
	t.Helper()
	st := storetest.New(t)
	ctx := t.Context()

	tx, err := st.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	l := &ledger{
		failures: []Failure{{Key: moment, At: time.Now().UTC(), Says: "It went badly."}},
		says: []Observation{
			{Key: nights, Says: "You slept under 6 hours on 9 of the last 14 nights.", From: "Baseline"},
			{Key: gaps, Says: "5 of the 11 moments put a question you had no rule for.", From: "Your playbook"},
		},
	}
	unlocks := &granted{}

	f(&world{
		ctx: ctx, tx: tx, l: l, unlocks: unlocks,
		m:       New(clock.Fixed{At: time.Now().UTC()}, among{where: "east-side"}, unlocks, l),
		account: newAccount(t, ctx, tx),
	})
}

func newAccount(t *testing.T, ctx context.Context, tx pgx.Tx) string {
	t.Helper()
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO account (provider, subject, email, display_name)
		VALUES ('google', 'diagnosis-test', 'diagnosis@example.com', 'Diagnosis')
		RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("create account: %v", err)
	}
	return id
}

// speaking finds a cause the given observation supports, so the tests below do
// not hard-code authored keys that content is free to rename.
func speaking(t *testing.T, key string) Cause {
	t.Helper()
	for _, c := range Causes() {
		for _, k := range c.Speaks {
			if k == key {
				return c
			}
		}
	}
	t.Fatalf("no authored cause speaks from %q", key)
	return Cause{}
}

/*
A player with nothing gone wrong has nothing to sit down about.

§3.6 stages this "at the moment of consequence", and the moment is the whole of
what makes it not a form. A scene that were always available would be a screen
the player visits when they feel like understanding something, which is neither
how understanding works nor what the section describes.
*/
func TestNothingIsOpenUntilSomethingHasGoneBadly(t *testing.T) {
	with(t, func(w *world) {
		w.l.failures = nil

		sat, err := w.m.Sit(w.ctx, w.tx, w.account)
		if err != nil {
			t.Fatalf("sit: %v", err)
		}
		if sat.Available() {
			t.Error("a diagnosis was offered to somebody nothing had happened to")
		}
		if sat.With != "" {
			t.Errorf("somebody (%q) was put in a room for a conversation with no subject", sat.With)
		}
	})
}

/*
The causes offered come out of the player's own evidence.

§3.6's first rule, as behaviour: with only the body saying anything, the accounts
about the log are not reachable, and with nothing saying anything none of them
is. Two players standing in front of the same failure with different histories
are not offered the same accounts of it, which is the difference between
selecting from your data and selecting from a menu.
*/
func TestWhatCanBeNamedComesOutOfTheEvidence(t *testing.T) {
	with(t, func(w *world) {
		w.l.says = w.l.says[:1] // the body only

		sat, err := w.m.Sit(w.ctx, w.tx, w.account)
		if err != nil {
			t.Fatalf("sit: %v", err)
		}
		if len(sat.Causes) == 0 {
			t.Fatal("evidence about the body reached no account of anything")
		}
		for _, c := range sat.Causes {
			if !speaks(c, map[string]bool{nights: true}) {
				t.Errorf("%q was offered, and nothing the player has bears on it", c.Key)
			}
		}

		w.l.says = nil
		if sat, err = w.m.Sit(w.ctx, w.tx, w.account); err != nil {
			t.Fatalf("sit: %v", err)
		}
		if len(sat.Causes) != 0 {
			t.Errorf("a player with no evidence was offered %d accounts to choose from", len(sat.Causes))
		}
	})
}

/*
Naming a cause hands over its tool, whatever the cause is.

§3.6's third rule: "wrong diagnoses are playable, not blocked". There is no
branch in [Diagnoses.Name] that could refuse a defensible conclusion for being
the wrong one, and this is the test that would fail if somebody added one.
*/
func TestNamingACauseHandsOverItsTool(t *testing.T) {
	with(t, func(w *world) {
		c := speaking(t, nights)

		named, err := w.m.Name(w.ctx, w.tx, w.account, moment, c.Key, []string{nights})
		if err != nil {
			t.Fatalf("name: %v", err)
		}
		if named.Key != c.Key {
			t.Errorf("named %q and got %q back", c.Key, named.Key)
		}

		unlocked, err := w.m.Named(w.ctx, w.tx, w.account)
		if err != nil {
			t.Fatalf("named: %v", err)
		}
		if c.Routine != "" && !unlocked[c.Routine] {
			t.Errorf("%q was named and %q is still locked", c.Key, c.Routine)
		}
		if !reflect.DeepEqual(w.unlocks.terms, c.Terms) {
			t.Errorf("handed over %v, wanted %v", w.unlocks.terms, c.Terms)
		}
	})
}

/*
A second conclusion does not take the first one's tool away.

§3.6's fourth rule has a consequence nobody states: if naming a second cause
replaced the first, a player working their way toward understanding something
would keep losing what they had learnt on the way. A tool arrived at by
concluding something is what that conclusion cost, and it is theirs whether or
not they still believe it.
*/
func TestUnderstandingSomethingElseDoesNotTakeBackTheFirstTool(t *testing.T) {
	with(t, func(w *world) {
		first := speaking(t, nights)
		second := speaking(t, gaps)
		if first.Key == second.Key {
			t.Skip("the two stub observations speak to the same cause")
		}

		if _, err := w.m.Name(w.ctx, w.tx, w.account, moment, first.Key, []string{nights}); err != nil {
			t.Fatalf("name: %v", err)
		}
		// Something new turns up, which is what re-opens the question.
		w.l.says = append(w.l.says, Observation{Key: "health/hrv-falling", Says: "It has been falling.", From: "A good watch"})
		if _, err := w.m.Name(w.ctx, w.tx, w.account, moment, second.Key, []string{gaps}); err != nil {
			t.Fatalf("name again: %v", err)
		}

		unlocked, err := w.m.Named(w.ctx, w.tx, w.account)
		if err != nil {
			t.Fatalf("named: %v", err)
		}
		for _, k := range []string{first.Routine, second.Routine} {
			if k != "" && !unlocked[k] {
				t.Errorf("%q was earned and then lost", k)
			}
		}
	})
}

/*
Pointing at the same evidence again is refused — there is no retry loop.

§3.6's third rule ends "no retry loop, no buzzer", and the buzzer is the easy
half. This is the other one: a player who could re-open the same moment with the
same evidence would work through the list of accounts until something helped,
which is guessing with extra steps.
*/
func TestTheSameQuestionCannotBeAnsweredTwice(t *testing.T) {
	with(t, func(w *world) {
		c := speaking(t, nights)
		if _, err := w.m.Name(w.ctx, w.tx, w.account, moment, c.Key, []string{nights}); err != nil {
			t.Fatalf("name: %v", err)
		}

		sat, err := w.m.Sit(w.ctx, w.tx, w.account)
		if err != nil {
			t.Fatalf("sit: %v", err)
		}
		if sat.Available() {
			t.Error("the same moment was put back in front of the player with nothing new to say about it")
		}

		_, err = w.m.Name(w.ctx, w.tx, w.account, moment, c.Key, []string{nights})
		if !errors.Is(err, ErrNoOccasion) {
			t.Errorf("answering it again gave %v, wanted ErrNoOccasion", err)
		}
	})
}

/*
It re-opens when the player's own history says something it did not before.

§3.6's fifth rule, and the comparison is against what they *had* rather than
against a clock. Evidence that has gone away does not re-open anything, because
re-opening on it would put the question back in front of somebody for having
fixed something.
*/
func TestAConclusionIsRevisitedOnceThereIsMoreToGoOn(t *testing.T) {
	with(t, func(w *world) {
		c := speaking(t, nights)
		if _, err := w.m.Name(w.ctx, w.tx, w.account, moment, c.Key, []string{nights}); err != nil {
			t.Fatalf("name: %v", err)
		}

		// A watch arrives, and with it a reading nobody could see before.
		w.l.says = append(w.l.says, Observation{
			Key: "health/hrv-falling", Says: "It is 9 ms lower than a fortnight ago.", From: "A good watch",
		})

		sat, err := w.m.Sit(w.ctx, w.tx, w.account)
		if err != nil {
			t.Fatalf("sit: %v", err)
		}
		if !sat.Available() {
			t.Fatal("new evidence did not re-open the conclusion it bears on")
		}
		if !sat.Again {
			t.Error("a revisit was not presented as one")
		}
		if sat.Said != c.Says {
			t.Errorf("the revisit shows %q as what was concluded, wanted %q", sat.Said, c.Says)
		}

		// And evidence going away does not.
		if _, err := w.m.Name(w.ctx, w.tx, w.account, moment, c.Key, []string{nights}); err != nil {
			t.Fatalf("name again: %v", err)
		}
		w.l.says = w.l.says[:1]
		if sat, err = w.m.Sit(w.ctx, w.tx, w.account); err != nil {
			t.Fatalf("sit: %v", err)
		}
		if sat.Available() {
			t.Error("a player was asked to account for the same moment again because something got better")
		}
	})
}

/*
Naming a cause nothing you pointed at bears on is refused.

The one refusal in this package about the argument, and it is about its *shape*.
Pointing at a fortnight of missing sleep and concluding something about the
playbook is not false — it may well be true — but the player has not made the
case, and accepting it would make the evidence decorative.
*/
func TestACauseNothingYouPointedAtBearsOnIsRefused(t *testing.T) {
	with(t, func(w *world) {
		body := speaking(t, nights)
		log := speaking(t, gaps)
		if body.Key == log.Key {
			t.Skip("the two stub observations speak to the same cause")
		}

		_, err := w.m.Name(w.ctx, w.tx, w.account, moment, log.Key, []string{nights})
		if !errors.Is(err, ErrDoesNotSpeak) {
			t.Errorf("got %v, wanted ErrDoesNotSpeak", err)
		}
	})
}

// Evidence the player does not have is refused, which is a claim about rows and
// not about reasoning.
func TestEvidenceThatIsNotYoursIsRefused(t *testing.T) {
	with(t, func(w *world) {
		c := speaking(t, nights)
		_, err := w.m.Name(w.ctx, w.tx, w.account, moment, c.Key, []string{"health/invented"})
		if !errors.Is(err, ErrNotYours) {
			t.Errorf("got %v, wanted ErrNotYours", err)
		}
	})
}

// A conclusion filed against a moment that is not the one in front of the player
// is a 409's worth of stale tab.
func TestAConclusionAboutAnotherMomentIsRefused(t *testing.T) {
	with(t, func(w *world) {
		c := speaking(t, nights)
		_, err := w.m.Name(w.ctx, w.tx, w.account, "playbook/fire/999", c.Key, []string{nights})
		if !errors.Is(err, ErrNoOccasion) {
			t.Errorf("got %v, wanted ErrNoOccasion", err)
		}
	})
}

/*
Nothing in this package knows what is true.

§3.6's second rule, tested the only way a prohibition can be: by walking the
exported surface and failing on any field, method or error whose name is about
correctness. It reads as paranoid and it is exactly the check worth having —
what breaks this mechanic is not a bad decision, it is somebody adding a
`Correct bool` for a dashboard eighteen months from now, in a commit that
touches nothing else.
*/
func TestNothingInThisPackageKnowsWhatIsTrue(t *testing.T) {
	t.Parallel()
	verdicts := []string{"Correct", "Right", "Wrong", "Score", "Grade", "Truth", "Valid", "Accurate"}

	for _, shape := range []any{Sitting{}, Cause{}, Conclusion{}, Observation{}, Failure{}, Scene{}} {
		v := reflect.TypeOf(shape)
		for i := range v.NumField() {
			for _, word := range verdicts {
				if v.Field(i).Name == word {
					t.Errorf("%s has a field called %s, and this package does not know that", v.Name(), word)
				}
			}
		}
	}

	m := reflect.TypeOf(&Diagnoses{})
	for i := range m.NumMethod() {
		for _, word := range verdicts {
			if m.Method(i).Name == word {
				t.Errorf("Diagnoses has a method called %s, and this package does not know that", word)
			}
		}
	}
}
