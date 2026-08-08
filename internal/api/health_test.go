package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/health"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
)

/*
The health panel through the transport, over a real Postgres.

The load-bearing assertion in this file is the last one: an uninstrumented
player's body moves *identically* to an instrumented one's. §3.16 makes that the
whole design — buying a watch changes your resolution on your own body, not your
body — and it is the property that would rot silently, because a bug that made
instrumentation matter would look like a working game for a long time.
*/

// panelOf reads the panel out of a response body, whatever else it carries.
type panelBody struct {
	BiologicalAge float64 `json:"biologicalAge"`
	LivedToday    bool    `json:"livedToday"`
	Tiers         []struct {
		Key     string `json:"key"`
		Owned   bool   `json:"owned"`
		Price   int64  `json:"price"`
		ForSale bool   `json:"forSale"`
	} `json:"tiers"`
	Metrics []struct {
		Key     string    `json:"key"`
		Name    string    `json:"name"`
		Means   string    `json:"means"`
		Tier    string    `json:"tier"`
		Visible bool      `json:"visible"`
		Value   *float64  `json:"value"`
		History []float64 `json:"history"`
	} `json:"metrics"`
}

func (p panelBody) metric(t *testing.T, key string) struct {
	Key     string    `json:"key"`
	Name    string    `json:"name"`
	Means   string    `json:"means"`
	Tier    string    `json:"tier"`
	Visible bool      `json:"visible"`
	Value   *float64  `json:"value"`
	History []float64 `json:"history"`
} {
	t.Helper()
	for _, m := range p.Metrics {
		if m.Key == key {
			return m
		}
	}
	t.Fatalf("no metric %q in the panel", key)
	panic("unreachable")
}

// healthOf reads the panel out of the world snapshot.
func healthOf(t *testing.T, s *Server, as *http.Cookie) panelBody {
	t.Helper()
	rec := read(t, s, as, worldPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d, body = %s", rec.Code, rec.Body)
	}
	var out struct {
		Health panelBody `json:"health"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("snapshot is not a world: %v", err)
	}
	return out.Health
}

// liveADay sends one day of living and fails if the server refused it.
func liveADay(t *testing.T, s *Server, as *http.Cookie, key, body string) panelBody {
	t.Helper()
	rec := post(t, s, as, "health.live", key, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("living a day = %d, body = %s", rec.Code, rec.Body)
	}
	var out struct {
		Panel   panelBody `json:"panel"`
		Crossed []struct {
			Metric string `json:"metric"`
			Worse  bool   `json:"worse"`
		} `json:"crossed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not a lived day: %v", err)
	}
	return out.Panel
}

// bodyOf reads a player's stored panel straight out of the table, which is what
// a comparison between two players has to be made against: not what either of
// them was shown.
func bodyOf(t *testing.T, st *store.Store, accountID string) simulation.Body {
	t.Helper()
	var b simulation.Body
	if err := st.Pool().QueryRow(t.Context(), `
		SELECT sleep_debt, fitness, height_m, last_sleep, resting_hr, weight_kg,
		       hrv, recovery, load, glucose, crp, ldl, cortisol
		  FROM health_panel WHERE account_id = $1`, accountID,
	).Scan(&b.SleepDebt, &b.Fitness, &b.HeightM, &b.LastSleep, &b.RestingHR, &b.WeightKg,
		&b.HRV, &b.Recovery, &b.Load, &b.Glucose, &b.CRP, &b.LDL, &b.Cortisol); err != nil {
		t.Fatalf("read stored panel: %v", err)
	}
	return b
}

func accountFor(t *testing.T, st *store.Store, subject string) string {
	t.Helper()
	var id string
	if err := st.Pool().QueryRow(t.Context(),
		`SELECT id FROM account WHERE subject = $1`, subject).Scan(&id); err != nil {
		t.Fatalf("read account %q: %v", subject, err)
	}
	return id
}

// Baseline is free and there from the start; everything else is named, priced,
// and empty.
func TestTheBaselinePanelIsVisibleWithoutBuyingAnything(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	p := healthOf(t, s, as)

	if p.BiologicalAge < 18 || p.BiologicalAge > 95 {
		t.Errorf("biological age = %.1f, want a plausible number at every tier", p.BiologicalAge)
	}
	for _, key := range []string{"sleep", "resting-hr", "weight"} {
		m := p.metric(t, key)
		if !m.Visible || m.Value == nil {
			t.Errorf("%s is not visible without instrumentation, and it should be", key)
		}
	}
	for _, key := range []string{"hrv", "recovery", "glucose", "crp"} {
		m := p.metric(t, key)
		if m.Visible || m.Value != nil {
			t.Errorf("%s is visible without the instrument that reads it", key)
		}
		// The metric is still described. §3.16 withholds the reading, not the
		// idea: a player has to be able to know the measurement exists, and
		// what would let them take it.
		if m.Name == "" || m.Means == "" || m.Tier == "" {
			t.Errorf("%s is hidden *and* nameless — nothing tells the player what to buy", key)
		}
	}
}

// The headline test, and the reason this package is shaped the way it is.
func TestAnUninstrumentedPlayerSuffersTheSameEffects(t *testing.T) {
	t.Parallel()
	s, st := newServer(t)
	clk := stopped(t, s, time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC))

	watched := signIn(t, s, "watched")
	blind := signIn(t, s, "blind")

	// One of them can see. Both start with the same money, so the only
	// difference between the two accounts is the instrument.
	if rec := post(t, s, watched, "health.buy", "w-buy", `{"tier":"wearable"}`); rec.Code != http.StatusOK {
		t.Fatalf("buying a watch = %d, body = %s", rec.Code, rec.Body)
	}

	// Fourteen identical days of living badly.
	bad := `{"sleep":"short","training":"hard","diet":"poor"}`
	for day := range 14 {
		liveADay(t, s, watched, newKey(day)+"-w", bad)
		liveADay(t, s, blind, newKey(day)+"-b", bad)
		clk.Advance(24 * time.Hour)
	}

	seen := bodyOf(t, st, accountFor(t, st, "watched"))
	unseen := bodyOf(t, st, accountFor(t, st, "blind"))

	if seen != unseen {
		t.Errorf("two identical fortnights produced two different bodies:\n watch: %+v\n none:  %+v", seen, unseen)
	}

	// And both are genuinely worse off — a test where nothing happened would
	// pass the comparison above and prove nothing.
	if simulation.BiologicalAge(unseen) <= simulation.BiologicalAge(simulation.Starting(1.78, 78)) {
		t.Error("a fortnight of five-hour nights did not age the uninstrumented player at all")
	}

	// What the *blind* player can see is nothing about it, which is the game.
	p := healthOf(t, s, blind)
	if m := p.metric(t, "hrv"); m.Value != nil {
		t.Error("the uninstrumented player can see their HRV")
	}
	if p.BiologicalAge <= 26 {
		t.Errorf("biological age = %.1f — the one number they *can* see should have moved", p.BiologicalAge)
	}
}

// Buying reveals the history as well as the number, because the samples were
// being written the whole time (§3.2, §3.6). This is what makes an instrument
// worth money rather than worth waiting for.
func TestBuyingAnInstrumentRevealsWhatWasAlreadyThere(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)
	clk := stopped(t, s, time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC))
	id := accountOf(t, st)

	for day := range 6 {
		liveADay(t, s, as, newKey(day), `{"sleep":"short","training":"rest","diet":"poor"}`)
		clk.Advance(24 * time.Hour)
	}

	before := bodyOf(t, st, id)
	blind := healthOf(t, s, as)
	if m := blind.metric(t, "hrv"); m.Visible || len(m.History) != 0 {
		t.Fatal("HRV was visible before the watch was bought")
	}

	rec := post(t, s, as, "health.buy", "buy-watch", `{"tier":"wearable"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("buying = %d, body = %s", rec.Code, rec.Body)
	}

	after := healthOf(t, s, as)
	hrv := after.metric(t, "hrv")
	if !hrv.Visible || hrv.Value == nil {
		t.Fatal("HRV is still not visible after buying the watch that reads it")
	}
	if len(hrv.History) != 6 {
		t.Errorf("%d days of HRV history, want the 6 that were recorded while it could not be seen",
			len(hrv.History))
	}

	// Nothing about the body changed. Buying an instrument buys resolution.
	if got := bodyOf(t, st, id); got != before {
		t.Errorf("buying a watch changed the body:\n before: %+v\n after:  %+v", before, got)
	}
	if after.BiologicalAge != blind.BiologicalAge {
		t.Errorf("biological age moved on a purchase: %.3f → %.3f", blind.BiologicalAge, after.BiologicalAge)
	}

	// And it cost what it says it costs.
	var spent int64
	for _, tier := range after.Tiers {
		if tier.Key == "wearable" {
			spent = tier.Price
			if !tier.Owned {
				t.Error("the wearable is not marked owned after being bought")
			}
		}
	}
	if spent != health.Wearable.Price.Cents() {
		t.Errorf("the panel prices the wearable at %d, the module at %d", spent, health.Wearable.Price.Cents())
	}
}

// The time series is a persisted history, not a window a client holds.
func TestEveryDayLivedIsRecordedForEveryPlayer(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)
	clk := stopped(t, s, time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC))
	id := accountOf(t, st)

	for day := range 3 {
		liveADay(t, s, as, newKey(day), `{"sleep":"full","training":"light","diet":"clean"}`)
		clk.Advance(24 * time.Hour)
	}

	var rows, withHRV int
	if err := st.Pool().QueryRow(t.Context(), `
		SELECT count(*), count(*) FILTER (WHERE hrv > 0) FROM health_sample WHERE account_id = $1`, id,
	).Scan(&rows, &withHRV); err != nil {
		t.Fatalf("read samples: %v", err)
	}
	if rows != 3 {
		t.Errorf("%d samples after three days, want 3", rows)
	}
	// Including the metrics this player cannot see. A history that only
	// recorded what was visible could never be revealed later.
	if withHRV != 3 {
		t.Errorf("%d samples carry an HRV, want 3 — the panel is simulated in full for everyone", withHRV)
	}
}

// One game day is one real calendar day (§3.1).
func TestADayCannotBeLivedTwice(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)
	clk := stopped(t, s, time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC))

	liveADay(t, s, as, "day-1", `{"sleep":"full","training":"rest","diet":"clean"}`)

	rec := post(t, s, as, "health.live", "day-1-again", `{"sleep":"short","training":"hard","diet":"poor"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("living the same day twice = %d, want 409; body = %s", rec.Code, rec.Body)
	}
	if !healthOf(t, s, as).LivedToday {
		t.Error("the panel does not say today has been lived")
	}

	// Tomorrow the identical request works, which is what 409 promised.
	clk.Advance(2 * time.Hour)
	liveADay(t, s, as, "day-2", `{"sleep":"short","training":"hard","diet":"poor"}`)
	if healthOf(t, s, as).LivedToday != true {
		t.Error("the new day was not recorded as lived")
	}
}

// A way of living that is not in the vocabulary is wrong forever (§8).
func TestALifeOutsideTheVocabularyIsRefused(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	rec := post(t, s, as, "health.live", "odd", `{"sleep":"forever","training":"hard","diet":"clean"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("an unknown way to sleep = %d, want 422", rec.Code)
	}
}

// The money goes out through character.Wallet, in the same transaction, so a
// purchase either happens completely or not at all.
func TestAnInstrumentCannotBeBoughtWithoutTheMoney(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)
	id := accountOf(t, st)

	// Empty the wallet through the interface, the way another module would.
	balance, err := walletBalance(t, st, s.characters, id)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	if err := walletSpend(t, st, s.characters, id, balance); err != nil {
		t.Fatalf("emptying the wallet: %v", err)
	}

	rec := post(t, s, as, "health.buy", "broke", `{"tier":"wearable"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("buying with no money = %d, want 409; body = %s", rec.Code, rec.Body)
	}
	if healthOf(t, s, as).metric(t, "hrv").Visible {
		t.Error("a refused purchase still revealed the metric")
	}

	// And an instrument that does not exist is a different kind of wrong.
	if rec := post(t, s, as, "health.buy", "nope", `{"tier":"x-ray"}`); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("buying an instrument that does not exist = %d, want 422", rec.Code)
	}
}

// Owning it twice is not a thing, and the second attempt says so rather than
// quietly taking the money again.
func TestAnInstrumentIsOnlyBoughtOnce(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)
	id := accountOf(t, st)

	if rec := post(t, s, as, "health.buy", "first", `{"tier":"wearable"}`); rec.Code != http.StatusOK {
		t.Fatalf("buying = %d", rec.Code)
	}
	after := money(t, st, id)

	if rec := post(t, s, as, "health.buy", "second", `{"tier":"wearable"}`); rec.Code != http.StatusConflict {
		t.Errorf("buying the same instrument twice = %d, want 409", rec.Code)
	}
	if got := money(t, st, id); got != after {
		t.Errorf("the refused second purchase moved money: %d → %d", after, got)
	}
}
