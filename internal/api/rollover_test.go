package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/rollover"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
)

/*
The catch-up, through the route a returning player actually takes.

technical.md §3.3: a dormant player gets no scheduled work at all, and **their
next login triggers a catch-up rollover.** For a game whose sessions last a
month, "login" in the useful sense is opening the app — so this is a test of the
world snapshot, which is the first thing every page load fetches.
*/

// Opening the app applies the days the player was away for, before the world
// they are handed is composed. The body they see is up to date, not the one
// they left.
func TestOpeningTheAppCatchesTheDaysUp(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)

	// One snapshot to create the ledger — a brand-new player owes nothing, so
	// nothing has been lived yet.
	if got := history(t, s, as); len(got) != 0 {
		t.Fatalf("a new player already has %d days of history", len(got))
	}

	// Away for four days.
	fallBehind(t, st, 4)

	if got := len(history(t, s, as)); got != 4 {
		t.Fatalf("came back to %d days of history, want 4", got)
	}
}

// The catch-up is bounded. A player back after a year does not get a year of
// mornings that happened to nobody (§3.3).
func TestALongAbsenceIsCaughtUpFromTheWindow(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)
	history(t, s, as)

	fallBehind(t, st, 365)
	history(t, s, as)

	// The panel only carries a few weeks of history, so the assertion is on
	// the ledger: the window's worth of days lived, the rest counted as
	// skipped, and the date stamped all the way forward — which is what makes
	// the player caught up rather than stuck replaying a year a night at a time.
	lived, over := ran(t, st)
	if lived != rollover.Window {
		t.Errorf("lived %d days, want the window's %d", lived, rollover.Window)
	}
	if want := 365 - rollover.Window; over != want {
		t.Errorf("skipped %d days, want %d", over, want)
	}
}

// Loading the page twice does not live the days twice.
func TestOpeningTheAppTwiceLivesNothingTwice(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)
	history(t, s, as)

	fallBehind(t, st, 3)

	first := history(t, s, as)
	second := history(t, s, as)
	if len(first) != 3 || len(second) != 3 {
		t.Errorf("history went %d then %d, want 3 both times", len(first), len(second))
	}
}

/* ── helpers ──────────────────────────────────────────────────────────── */

// history is the biological-age series the snapshot reports, which is one
// reading per day the body has lived.
func history(t *testing.T, s *Server, as *http.Cookie) []float64 {
	t.Helper()
	rec := read(t, s, as, worldPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var out struct {
		Health struct {
			Metrics []struct {
				Key     string    `json:"key"`
				History []float64 `json:"history"`
			} `json:"metrics"`
		} `json:"health"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not a world snapshot: %v", err)
	}
	for _, m := range out.Health.Metrics {
		// A baseline metric, so it is visible without any instrument bought.
		if m.Key == "sleep" {
			return m.History
		}
	}
	t.Fatal("the snapshot has no sleep metric")
	return nil
}

// fallBehind moves the rollover ledger back, which is what being away looks
// like from the database's side.
func fallBehind(t *testing.T, st *store.Store, days int) {
	t.Helper()
	tag, err := st.Pool().Exec(t.Context(),
		`UPDATE rollover SET last_day = last_day - make_interval(days => $1)`, days)
	if err != nil {
		t.Fatalf("move the ledger back: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("moved %d ledgers back, want 1 — the snapshot did not create one",
			tag.RowsAffected())
	}
}

// ran is what the last rollover applied and what it declined to replay.
func ran(t *testing.T, st *store.Store) (lived, over int) {
	t.Helper()
	if err := st.Pool().QueryRow(t.Context(),
		`SELECT days, skipped FROM rollover`).Scan(&lived, &over); err != nil {
		t.Fatalf("read the ledger: %v", err)
	}
	return lived, over
}
