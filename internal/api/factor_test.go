package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
Progression, at the edge it is allowed to reach (§3.5).

The composition is proved in internal/simulation and the ownership in
internal/factor. What is worth proving here is the one thing neither of those
can: that what actually goes over the wire to a browser contains the composite,
the two revealed readings, and **nothing whatsoever** about the other three.
*/

// snapshotKarma reads the progression board out of a world snapshot.
func snapshotKarma(t *testing.T, s *Server, as *http.Cookie) karmaView {
	t.Helper()

	rec := read(t, s, as, worldPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var out struct {
		Karma karmaView `json:"karma"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not a world snapshot: %v", err)
	}
	return out.Karma
}

// The board a client is hydrated with: five slots, two of which say anything.
func TestTheWorldSnapshotCarriesTheCompositeAndNoHiddenFactor(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	board := snapshotKarma(t, s, as)

	if board.Karma <= 0 {
		t.Fatalf("a new player's composite is %v", board.Karma)
	}
	if len(board.Slots) != len(simulation.Factors) {
		t.Fatalf("the board has %d slots, want one per factor", len(board.Slots))
	}

	shown := 0
	for _, slot := range board.Slots {
		if !slot.Shown {
			if slot.Key != "" || slot.Name != "" || slot.Reads != "" || slot.Value != nil {
				t.Fatalf("a hidden factor reached the wire carrying something: %+v", slot)
			}
			continue
		}
		shown++
		if slot.Value == nil {
			t.Fatalf("the %s slot is shown with no reading", slot.Key)
		}
	}
	if shown != 2 {
		t.Fatalf("%d slots are shown, want the two §3.5 reveals", shown)
	}

	// A new player is standing where the city starts them, in a place nobody
	// has worn out yet (§3.15).
	if board.Where == "" || board.Familiarity != 1 {
		t.Fatalf("board = %+v, want a named environment worth all of what it offers", board)
	}
}

/*
The strongest assertion available: the whole payload, read as text.

Not the struct — the bytes. A field added carelessly to a view, a debug dump, a
map keyed by factor, anything at all that let one of §3.5's hidden three across
this boundary would be caught here rather than by somebody reading a JSON tab.
*/
func TestNoHiddenFactorIsNamedAnywhereInTheWorldSnapshot(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	rec := read(t, s, as, worldPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()

	for _, f := range simulation.Factors {
		if f.Shown() {
			continue
		}
		if contains(body, f.Key()) {
			t.Errorf("%q appears in the world snapshot — the hidden factors are the hook (§3.5)", f.Key())
		}
	}
}

// contains is strings.Contains, spelled out so the assertion above reads as one
// idea rather than as an import.
func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

/*
The composite follows the world, over the routes a player actually drives.

Karma is a reading of a body and a balance among other things (§3.5), so a fare
paid and a day lived both move it — and it is recomposed on every read rather
than incremented anywhere, which is why the snapshot is the thing asserted.
*/
func TestTravellingAndLivingMoveTheComposite(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	before := snapshotKarma(t, s, as)

	post(t, s, as, "character.move", "fare-1", `{"to":"downtown","by":"transit"}`)
	poorer := snapshotKarma(t, s, as)
	if poorer.Karma >= before.Karma {
		t.Fatalf("a fare paid did not move the composite: %v then %v", before.Karma, poorer.Karma)
	}

	// A wrecked day is a worse body, and a worse body is a lower composite.
	post(t, s, as, "health.live", "day-1", `{"sleep":"short","training":"hard","diet":"poor"}`)
	after := snapshotKarma(t, s, as)
	if after.Karma >= poorer.Karma {
		t.Fatalf("five hours' sleep, hard training and bad food read %v, no lower than %v", after.Karma, poorer.Karma)
	}
}

/*
There is no command that awards a factor, and §7 is why.

Hidden factors are scored from committed choices, not from anything the player
types — so the absence of a route is the mechanism, and it is asserted rather
than left to be noticed. Credit arrives through internal/factor's Go surface,
from the modules that own the choices.
*/
func TestThereIsNoCommandThatAwardsAFactor(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	for _, name := range []string{"factor.record", "factor.grant", "factor.set", "karma.add"} {
		rec := post(t, s, as, name, "key-"+name, `{"factor":"kindness","credit":1000}`)
		if rec.Code != http.StatusNotFound {
			t.Errorf("POST %s = %d, want 404 — a factor a player can award themselves measures nothing", name, rec.Code)
		}
	}
}

/*
The restatement reaches the tab that caused it.

Karma is a reading of everything at once, so no single command's response carries
it — which means the tab that acted has to be told the same way the others are.
Every `factor.stood` row is therefore written with no origin, and a socket only
skips rows carrying its *own* session id (§8, rules 3 and 4).
*/
func TestTheCompositeIsRestatedToEveryTabIncludingTheOneThatActed(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)

	// A journey, sent from a named tab. Its own response says nothing about
	// Karma.
	sent := httptest.NewRequest(http.MethodPost, "/api/commands/character.move",
		strings.NewReader(`{"to":"the-strip-bar","by":"foot"}`))
	sent.Header.Set("Content-Type", "application/json")
	sent.Header.Set("Idempotency-Key", "journey-2")
	sent.Header.Set(sessionHeader, "a-tab")
	sent.AddCookie(as)

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, sent)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	rows, err := st.Pool().Query(t.Context(),
		`SELECT type, origin FROM realtime_outbox ORDER BY seq`)
	if err != nil {
		t.Fatalf("read the outbox: %v", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var (
			kind   string
			origin *string
		)
		if err := rows.Scan(&kind, &origin); err != nil {
			t.Fatalf("read the outbox: %v", err)
		}
		if kind != "factor.stood" {
			continue
		}
		found = true
		if origin != nil {
			t.Errorf("factor.stood was published with origin %q — the acting tab would skip it", *origin)
		}
	}
	if !found {
		t.Error("a command that moved money published no factor.stood")
	}
}

/*
The rollover ages the place the player is standing in (§3.3, §3.15).

This is the piece #15 fills into rollover.World, and it is per *day* rather than
per run: a player caught up from four days away has four days of tenure, not one.
Everything else on that interface is still a stub, and the stubs are the tickets
that have not landed.
*/
func TestTheRolloverAgesTheEnvironment(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)

	// The first snapshot creates the ledger a catch-up is measured against.
	if got := snapshotKarma(t, s, as).Days; got != 0 {
		t.Fatalf("a new player has already stood here for %d days", got)
	}

	fallBehind(t, st, 4)

	if got := snapshotKarma(t, s, as).Days; got != 4 {
		t.Fatalf("four days away counted as %d", got)
	}
	// Opening the app again is not another day. The ledger is what says how
	// many are owed, and it has none left.
	if got := snapshotKarma(t, s, as).Days; got != 4 {
		t.Fatalf("a second page load aged the place to %d days", got)
	}
}

/*
§3.15's inversion, at the edge: the environment is who you live among, and
walking about does not change it.

The decay and the ceilings are keyed by population, and the population follows
the player's *home* rather than their feet. If it followed their feet, an
afternoon downtown would put a player among nobody, reset nothing they had worn
out and un-cap everything they had reached the top of — and the pressure §3.15
builds the whole progression on would be avoidable by pacing.

It is also the reason the rule set in force arrives from the server: which people
you are among is not a thing the client can derive from a place key, and a client
that tried would disagree with the matcher the moment somebody went to work.
*/
func TestThePopulationFollowsWhereYouLiveAndNotWhereYouWalk(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	home := snapshotKarma(t, s, as)
	if home.Where == "" || home.Among == "" {
		t.Fatalf("a new player is among %q (%q)", home.Among, home.Where)
	}
	// One term per environment, and it is the map's: the key is a district the
	// client's own place list can find, so the rule-set chip and the map are
	// never speaking two languages about one place.
	if place, ok := city.Lookup(home.Where); !ok || place.Layer != city.District {
		t.Fatalf("%q is not a district; an environment goes by the map's name for it", home.Where)
	}

	for _, district := range []string{"downtown", "riverfront", "the-strip"} {
		walkTo(t, s, as, "walk-"+district, district)
		got := snapshotKarma(t, s, as)
		if got.Where != home.Where || got.Among != home.Among {
			t.Fatalf("standing in %s put the player among %q (%q)", district, got.Among, got.Where)
		}
	}

	// And the rule set in force is that population's, wherever the player ended
	// up standing.
	view := snapshotPlaybook(t, s, as)
	if set := setFor(t, view, home.Where); set.Name != home.Among {
		t.Fatalf("the set for %q is called %q, and the player is among %q", home.Where, set.Name, home.Among)
	}
}
