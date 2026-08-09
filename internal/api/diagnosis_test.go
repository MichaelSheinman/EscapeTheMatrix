package api

import (
	"net/http"
	"testing"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/diagnosis"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/routine"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
)

/*
The diagnosis at the edge (§3.6).

What is worth proving at this boundary is not that the module works — its own
tests do that against a real database — but the two things only this layer can
get wrong: that no verdict crosses the wire, and that the gate the composition
root binds is actually the one the routines are read through.
*/

/*
workedOut records a conclusion straight into the table, without the scene.

A test about *routines* should not have to play out a diagnosis to reach one,
and going through the scene would make every routine test depend on this
player's history producing the right evidence on the right day. So the row goes
in directly, which is honest about what it is: the state of having concluded
something, arrived at by fiat because the conclusion is not what is under test.

The tests that are about the mechanic itself are below, and none of them uses
this.
*/
func workedOut(t *testing.T, st *store.Store, causes ...string) {
	t.Helper()
	for _, key := range causes {
		if _, ok := diagnosis.Known(key); !ok {
			t.Fatalf("no cause called %q is authored", key)
		}
		if _, err := st.Pool().Exec(t.Context(), `
			INSERT INTO diagnosis (account_id, occasion, cause, at, pointed, had)
			SELECT id, $1, $2, now(), ARRAY['test'], ARRAY['test'] FROM account`,
			"test/"+key, key,
		); err != nil {
			t.Fatalf("record %q: %v", key, err)
		}
	}
}

/*
everything names whatever it takes to unlock all four routines.

For every account in the test's database rather than one, because two of the
tests that need it run two players side by side — which is itself the point of
those tests, since a body cannot be un-lived and comparing one routine against
another takes two of them.
*/
func everything(t *testing.T, st *store.Store) {
	t.Helper()
	want := map[string]string{}
	for _, c := range diagnosis.Causes() {
		if c.Routine != "" {
			want[c.Routine] = c.Key
		}
	}
	for _, k := range routine.Kinds {
		key, ok := want[k.Key]
		if !ok {
			t.Fatalf("no authored cause unlocks %q", k.Key)
		}
		workedOut(t, st, key)
	}
}

/*
A new player has no routines and a reason for each of them.

The state §3.6 describes and the one the game shipped without: struggle before
mastery, so nothing is configurable until something has been worked out. What is
sent for a locked routine is the sentence saying what is still missing — a
routine you cannot configure is a visible absence with a reason attached.
*/
func TestANewPlayerHasEarnedNoRoutines(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	set, _ := worldRoutines(t, s, as)

	for _, one := range set.Routines {
		if one.Unlocked {
			t.Errorf("%s is configurable by somebody who has not been through anything", one.Key)
		}
		if one.Learn == "" {
			t.Errorf("%s is locked and does not say what is still missing", one.Key)
		}
	}
}

// And naming a cause is what opens the one it unlocks, and only that one.
func TestNamingACauseOpensTheRoutineItUnlocks(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)

	var named diagnosis.Cause
	for _, c := range diagnosis.Causes() {
		if c.Routine != "" {
			named = c
			break
		}
	}
	workedOut(t, st, named.Key)

	set, _ := worldRoutines(t, s, as)
	for _, one := range set.Routines {
		want := one.Key == named.Routine
		if one.Unlocked != want {
			t.Errorf("%s unlocked = %v, want %v after naming %q", one.Key, one.Unlocked, want, named.Key)
		}
	}
}

/*
Nothing about a diagnosis on the wire says whether anything is right.

The one thing only this layer can get wrong. internal/diagnosis has no
representation of a correct answer, but a view is a new set of fields and the
temptation — a `score`, a `best`, an ordering that is really a ranking — arrives
here rather than there.
*/
func TestNoVerdictCrossesTheWire(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	rec := read(t, s, as, worldPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d, body = %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	for _, word := range []string{
		`"correct"`, `"score"`, `"grade"`, `"right"`, `"wrong"`, `"best"`, `"rank"`,
	} {
		if contains(body, word) {
			t.Errorf("the world snapshot carries %s, and this game has no such fact", word)
		}
	}
}

// A conclusion about a moment that is not in front of the player is refused,
// and it is a 409 rather than a 422: the request is fine and the world moved.
func TestNamingSomethingThatIsNotInFrontOfYouIsRefused(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	rec := post(t, s, as, "diagnosis.name", newKey(1),
		`{"occasion":"playbook/fire/1","cause":"short-nights","pointed":["health/short-nights"]}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("naming a moment nothing raised = %d, want 409: %s", rec.Code, rec.Body.String())
	}
}

// A conclusion with no moment named at all is wrong forever, however often it
// is sent.
func TestAConclusionAboutNothingIsRefused(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	rec := post(t, s, as, "diagnosis.name", newKey(1), `{"cause":"short-nights"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("a conclusion about no moment = %d, want 422: %s", rec.Code, rec.Body.String())
	}
}
