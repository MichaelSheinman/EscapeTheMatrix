package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
Routines through the transport, over a real Postgres.

The assertions worth having here are the ones that only exist once the pieces
are joined up: the configuration crosses the wire as one unit, the day the
player lives is the one their rules produced, and the forecast is instrumented
exactly like the panel it is a projection of — a player with no watch is not
told what their HRV will be in a fortnight either.
*/

type routinesBody struct {
	Set      bool `json:"set"`
	Routines []struct {
		Key      string `json:"key"`
		Name     string `json:"name"`
		Asks     string `json:"asks"`
		Learn    string `json:"learn"`
		Unlocked bool   `json:"unlocked"`
		Chosen   string `json:"chosen"`
		Options  []struct {
			Key    string `json:"key"`
			Says   string `json:"says"`
			Costs  string `json:"costs"`
			Chosen bool   `json:"chosen"`
		} `json:"options"`
	} `json:"routines"`
}

func (r routinesBody) of(t *testing.T, key string) string {
	t.Helper()
	for _, one := range r.Routines {
		if one.Key == key {
			return one.Chosen
		}
	}
	t.Fatalf("no routine %q", key)
	return ""
}

type forecastBody struct {
	Days          int       `json:"days"`
	BiologicalAge []float64 `json:"biologicalAge"`
	Metrics       []struct {
		Key    string    `json:"key"`
		Values []float64 `json:"values"`
	} `json:"metrics"`
}

func (f forecastBody) values(t *testing.T, key string) []float64 {
	t.Helper()
	for _, m := range f.Metrics {
		if m.Key == key {
			return m.Values
		}
	}
	t.Fatalf("no metric %q in the forecast", key)
	return nil
}

// worldRoutines reads the routines and the forecast out of the world snapshot.
func worldRoutines(t *testing.T, s *Server, as *http.Cookie) (routinesBody, forecastBody) {
	t.Helper()
	rec := read(t, s, as, worldPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d, body = %s", rec.Code, rec.Body)
	}
	var out struct {
		Routines routinesBody `json:"routines"`
		Forecast forecastBody `json:"forecast"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("snapshot is not a world: %v", err)
	}
	return out.Routines, out.Forecast
}

// setRoutines states the whole configuration and fails if the server refused it.
func setRoutines(t *testing.T, s *Server, as *http.Cookie, key, choices string) (routinesBody, forecastBody) {
	t.Helper()
	rec := post(t, s, as, "routine.set", key, `{"choices":`+choices+`}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("setting the routines = %d, body = %s", rec.Code, rec.Body)
	}
	var out struct {
		Routines routinesBody `json:"routines"`
		Forecast forecastBody `json:"forecast"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("the answer is not a set of routines: %v", err)
	}
	return out.Routines, out.Forecast
}

// liveByRoutine lives one day by the rules and fails if the server refused it.
func liveByRoutine(t *testing.T, s *Server, as *http.Cookie, key string) panelBody {
	t.Helper()
	rec := post(t, s, as, "routine.live", key, `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("living by the routine = %d, body = %s", rec.Code, rec.Body)
	}
	var out struct {
		Panel panelBody `json:"panel"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("the answer is not a panel: %v", err)
	}
	return out.Panel
}

// The screen a player who has never touched routines is shown. Everything is
// answerable and nothing has been answered, which is a state with something to
// say rather than an empty screen.
func TestTheWorldCarriesEveryRoutineAndWhatItAsks(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	set, ahead := worldRoutines(t, s, as)

	if set.Set {
		t.Error("a new player reads as having configured their routines")
	}
	if len(set.Routines) != 4 {
		t.Fatalf("%d routines, want the four §3.4 names", len(set.Routines))
	}

	want := map[string]bool{"sleep": true, "workout": true, "diet": true, "organization": true}
	for _, one := range set.Routines {
		if !want[one.Key] {
			t.Errorf("unexpected routine %q", one.Key)
		}
		delete(want, one.Key)

		// The unlock gate is stubbed open until diagnosis lands (#32), and the
		// sentence that says what would unlock it is sent either way.
		if !one.Unlocked {
			t.Errorf("%s is locked, but nothing unlocks anything yet", one.Key)
		}
		if one.Asks == "" || one.Learn == "" || one.Name == "" {
			t.Errorf("%s arrived without the words it is read with: %+v", one.Key, one)
		}
		if len(one.Options) < 2 {
			t.Errorf("%s offers %d answers", one.Key, len(one.Options))
		}

		var chosen int
		for _, o := range one.Options {
			if o.Says == "" || o.Costs == "" {
				t.Errorf("%s/%s has no sentence or no price", one.Key, o.Key)
			}
			if o.Chosen {
				chosen++
				if o.Key != one.Chosen {
					t.Errorf("%s marks %q chosen but says %q is", one.Key, o.Key, one.Chosen)
				}
			}
		}
		if chosen != 1 {
			t.Errorf("%s has %d answers marked chosen, want exactly one", one.Key, chosen)
		}
	}
	if len(want) > 0 {
		t.Errorf("missing routines: %v", want)
	}

	// And the forecast comes with it, because a routine with nothing said about
	// what it would do is a form rather than a decision.
	if ahead.Days != forecastDays || len(ahead.BiologicalAge) != forecastDays {
		t.Errorf("forecast is %d days and %d readings, want %d of each",
			ahead.Days, len(ahead.BiologicalAge), forecastDays)
	}
}

/*
The configuration is one unit: stated whole, written whole.

The second statement below leaves sleep out, and sleep goes back to what an
unarranged life does. That is the behaviour §3.4's "configured as a unit" asks
for — there is no command that changes one routine, so there is no way for a
client to assemble a configuration the player never looked at.
*/
func TestTheRoutinesAreStatedAsOneUnit(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	set, _ := setRoutines(t, s, as, "set-1",
		`{"sleep":"protected","workout":"split","diet":"cooked","organization":"listed"}`)
	if !set.Set {
		t.Error("stating the routines did not record that they had been stated")
	}
	if got := set.of(t, "sleep"); got != "protected" {
		t.Errorf("sleep = %q, want protected", got)
	}

	// It survives the round trip, because it was written rather than echoed.
	stored, _ := worldRoutines(t, s, as)
	if got := stored.of(t, "workout"); got != "split" {
		t.Errorf("workout read back as %q, want split", got)
	}
	if !stored.Set {
		t.Error("the world snapshot does not know the routines were set")
	}

	again, _ := setRoutines(t, s, as, "set-2", `{"workout":"daily"}`)
	if got := again.of(t, "sleep"); got != "whenever" {
		t.Errorf("sleep = %q after a statement that did not mention it, want the default — the unit is the whole set", got)
	}
	if got := again.of(t, "workout"); got != "daily" {
		t.Errorf("workout = %q, want daily", got)
	}
}

// An answer outside the vocabulary is wrong forever, however often it is sent
// (§8). Nothing on this surface takes a free field.
func TestAnAnswerOutsideTheVocabularyIsRefused(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	if rec := post(t, s, as, "routine.set", "odd-1", `{"choices":{"sleep":"eight-ish"}}`); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("an invented answer = %d, want 422; body = %s", rec.Code, rec.Body)
	}
	if rec := post(t, s, as, "routine.set", "odd-2", `{"choices":{"posture":"upright"}}`); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("an invented routine = %d, want 422; body = %s", rec.Code, rec.Body)
	}

	set, _ := worldRoutines(t, s, as)
	if set.Set {
		t.Error("a refused statement still recorded that the routines were set")
	}
}

/*
The day the player lives is the one their rules produced.

This is what the routines are *for*: the three choices `health.live` asks for are
produced rather than answered, which is §3.4's arc toward setting it once and no
longer being asked. The rules are the pure core's and the assertion is that
nothing between here and there changed them.
*/
func TestLivingByTheRoutineLivesTheDayTheRulesProduce(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)
	// A Monday, which the split routine trains hard on.
	stopped(t, s, time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC))

	setRoutines(t, s, as, "set",
		`{"sleep":"protected","workout":"split","diet":"cooked","organization":"regimented"}`)

	panel := liveByRoutine(t, s, as, "day-1")
	if !panel.LivedToday {
		t.Error("the panel does not say today has been lived")
	}

	// What the rules say the day was, worked out independently of the server.
	rules := simulation.Routine{
		Sleep: 8*time.Hour + 30*time.Minute,
		Training: [7]simulation.Training{
			time.Monday: simulation.Hard, time.Tuesday: simulation.Light,
			time.Wednesday: simulation.Hard, time.Thursday: simulation.Light,
			time.Friday: simulation.Hard, time.Saturday: simulation.Light,
			time.Sunday: simulation.Rest,
		},
		Diet:         simulation.Clean,
		Organization: simulation.Regimented,
	}
	want, _ := simulation.Advance(fixedAt(time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC)),
		simulation.Starting(1.78, 78), rules.On(simulation.Date{Year: 2026, Month: time.August, Day: 3}))

	if got := bodyOf(t, st, accountOf(t, st)); got != want {
		t.Errorf("the body after one day by the routine is\n %+v\nthe rules say\n %+v", got, want)
	}

	// And still one game day per real day, whichever command asked for it.
	if rec := post(t, s, as, "routine.live", "day-1-again", `{}`); rec.Code != http.StatusConflict {
		t.Errorf("living the same day twice = %d, want 409; body = %s", rec.Code, rec.Body)
	}
}

/*
The acceptance criterion, end to end: a routine moves the panel over subsequent
days, and a better one moves it further.

Two players, the only difference between them being the rules they set. Nothing
is narrated by hand — every day of both lives came out of a configuration.

The horizon is deliberately longer than a fortnight. Two weeks into a routine
that trains, the panel reads *worse* than it did on day one: the load is up and
the fitness it buys has not arrived. That is real, it is the thing the forecast
exists to let a player see past, and a test that asserted improvement after
fourteen days would be asserting the game away.
*/
func TestARoutineMovesThePanelOverSubsequentDays(t *testing.T) {
	t.Parallel()
	s, _ := newServer(t)
	clk := stopped(t, s, time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC))

	kept := signIn(t, s, "kept")
	adrift := signIn(t, s, "adrift")

	setRoutines(t, s, kept, "k-set",
		`{"sleep":"protected","workout":"split","diet":"cooked","organization":"listed"}`)
	setRoutines(t, s, adrift, "a-set",
		`{"sleep":"whenever","workout":"whenever","diet":"whatever","organization":"loose"}`)

	before := worldPanel(t, s, kept).BiologicalAge

	for day := range 60 {
		liveByRoutine(t, s, kept, newKey(day)+"-k")
		liveByRoutine(t, s, adrift, newKey(day)+"-a")
		clk.Advance(24 * time.Hour)

		// The fortnight where it looks like a mistake, said out loud so that a
		// change in tuning that removes it is a decision rather than a drift.
		if day == 13 && worldPanel(t, s, kept).BiologicalAge <= before {
			t.Log("two weeks in, the routine already reads younger — the early load cost has been tuned away")
		}
	}

	good := worldPanel(t, s, kept)
	bad := worldPanel(t, s, adrift)

	if good.BiologicalAge >= bad.BiologicalAge {
		t.Errorf("after two months: kept routine reads %.2f, no routine reads %.2f",
			good.BiologicalAge, bad.BiologicalAge)
	}
	if good.BiologicalAge >= before {
		t.Errorf("two months of the routine did not move the number at all: %.2f → %.2f",
			before, good.BiologicalAge)
	}

	// The panel's own history is what the player watches it move in. It is
	// bounded to what a trend needs rather than to everything ever recorded.
	if m := good.metric(t, "sleep"); len(m.History) == 0 {
		t.Error("no sleep history after two months of living by a routine")
	}
}

/*
A projection is a reading of a body, so it obeys the same rule the panel does
(§3.16): instrumentation changes what the player can see, and never what is
happening to them.
*/
func TestTheForecastIsInstrumentedLikeThePanel(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)
	stopped(t, s, time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC))

	_, blind := setRoutines(t, s, as, "set",
		`{"sleep":"protected","workout":"split","diet":"cooked","organization":"listed"}`)

	if got := blind.values(t, "hrv"); len(got) != 0 {
		t.Errorf("the uninstrumented player is shown %d days of projected HRV", len(got))
	}
	if got := blind.values(t, "sleep"); len(got) != forecastDays {
		t.Errorf("%d days of projected sleep, want %d — the baseline needs no instrument", len(got), forecastDays)
	}
	// The one number visible at every tier, ahead as well as now (§3.5).
	if len(blind.BiologicalAge) != forecastDays {
		t.Errorf("%d days of projected biological age, want %d", len(blind.BiologicalAge), forecastDays)
	}

	post(t, s, as, "character.work", "pay", `{}`)
	if rec := post(t, s, as, "health.buy", "buy", `{"tier":"wearable"}`); rec.Code != http.StatusOK {
		t.Fatalf("buying a watch = %d, body = %s", rec.Code, rec.Body)
	}

	_, seen := worldRoutines(t, s, as)
	if got := seen.values(t, "hrv"); len(got) != forecastDays {
		t.Errorf("%d days of projected HRV after buying the watch, want %d", len(got), forecastDays)
	}
	if !equal(seen.BiologicalAge, blind.BiologicalAge) {
		t.Error("buying an instrument changed the projected biological age — a watch buys resolution, not a different future")
	}
}

// The forecast and the days actually lived have to agree, or the routine screen
// is telling a story the body does not follow.
func TestTheForecastIsWhatLivingItProduces(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)
	clk := stopped(t, s, time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC))

	post(t, s, as, "character.work", "pay-1", `{}`)
	post(t, s, as, "health.buy", "buy", `{"tier":"wearable"}`)

	_, ahead := setRoutines(t, s, as, "set",
		`{"sleep":"protected","workout":"split","diet":"cooked","organization":"listed"}`)
	predicted := ahead.values(t, "hrv")

	// The forecast starts from today, because today has not been lived yet.
	for day := range forecastDays {
		liveByRoutine(t, s, as, newKey(day))
		clk.Advance(24 * time.Hour)
	}

	lived := worldPanel(t, s, as).metric(t, "hrv").History
	if len(lived) < forecastDays {
		t.Fatalf("%d days of history, want at least %d", len(lived), forecastDays)
	}
	lived = lived[len(lived)-forecastDays:]

	if !equal(lived, predicted) {
		t.Errorf("a fortnight was projected as %v and lived as %v", predicted, lived)
	}
}

// worldPanel reads the health panel out of the world snapshot. Named apart from
// healthOf so this file reads without a hop into health_test.go.
func worldPanel(t *testing.T, s *Server, as *http.Cookie) panelBody {
	t.Helper()
	return healthOf(t, s, as)
}

// fixedAt is a clock stopped at an instant, for working out what the rules say
// without asking the server.
type fixedAt time.Time

func (f fixedAt) Now() time.Time { return time.Time(f) }

func equal(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
