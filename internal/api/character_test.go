package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/career"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/character"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/clock"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/health"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/rollover"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
)

/*
The character module through the transport that ships it: a real router, a real
Postgres, and the same commands the browser sends (§14).

What is worth proving here is what a unit test cannot: that the cap survives
being counted in a database, that the day boundary is the player's own, and that
the money a purchase takes cannot go negative however the requests arrive.
*/

// stopped replaces every clock the server reads, so a test can decide what day
// it is. Every one, because each module was handed a clock at construction and
// would otherwise still be reading the real one — and a test that moved only
// half of them would be testing two different days at once.
func stopped(t *testing.T, s *Server, at time.Time) *clock.Stepping {
	t.Helper()
	clk := &clock.Stepping{At: at}
	s.clk = clk
	s.characters = character.New(clk, character.Quiet{})
	s.health = health.New(clk, s.characters)
	// And the ladder, which was handed the character module as its payroll and
	// would otherwise be paying wages into an instance nothing else reads
	// (§3.14). Rebuilt rather than reached into for the same reason the two
	// above are: everything on a Server is fixed at boot, so the way to move a
	// clock is to build the module again with the new one.
	s.careers = career.New(clk, payroll{characters: s.characters}, capability{factors: s.factors})
	// And the day itself. §3.3's catch-up is what a page load drives, so a test
	// that moved the clock forward and left the rollover reading the real one
	// would be a test in which no day ever ends — which is where every wage
	// comes from (§3.4) and most of what a routine does to a body.
	s.rollovers = rollover.New(clk, s.store, s.log, s.accounts, s.characters, s.health, s.routines,
		employed{careers: s.careers},
		work{factors: s.factors, events: s.events, playbooks: s.playbooks, clk: clk}, s)
	return clk
}

func snapshot(t *testing.T, s *Server, as *http.Cookie) (out struct {
	Character struct {
		Money         int64 `json:"money"`
		EarningsLeft  int   `json:"earningsLeft"`
		EarningsTaken int   `json:"earningsTaken"`
		At            struct {
			Key  string `json:"key"`
			Name string `json:"name"`
		} `json:"at"`
		Home struct {
			Key string `json:"key"`
		} `json:"home"`
	} `json:"character"`
	Places []struct {
		Key   string `json:"key"`
		Layer string `json:"layer"`
		In    string `json:"in"`
		Stop  bool   `json:"stop"`
		Ways  []struct {
			To   string `json:"to"`
			Mode string `json:"mode"`
			Says string `json:"says"`
			Fare int64  `json:"fare"`
		} `json:"ways"`
	} `json:"places"`
}) {
	t.Helper()
	rec := read(t, s, as, worldPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d, body = %s", rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("snapshot is not a world: %v", err)
	}
	return out
}

func money(t *testing.T, st *store.Store, accountID string) int64 {
	t.Helper()
	var cents int64
	if err := st.Pool().QueryRow(t.Context(),
		`SELECT money_cents FROM character WHERE account_id = $1`, accountID,
	).Scan(&cents); err != nil {
		t.Fatalf("read money: %v", err)
	}
	return cents
}

// A player who has just signed in has somewhere to stand, somewhere to live,
// and something to spend — before they have done anything at all.
func TestASignedInPlayerHasACharacter(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	world := snapshot(t, s, as)

	if world.Character.Money <= 0 {
		t.Errorf("money = %d, want a starting balance", world.Character.Money)
	}
	if world.Character.At.Key == "" || world.Character.Home.Key == "" {
		t.Errorf("character = %+v, want a place and a home", world.Character)
	}
	if world.Character.At.Name == "" {
		t.Error("the district has no name — the client would have to invent one")
	}
	if len(world.Places) != len(city.Places()) {
		t.Errorf("%d places offered, want the whole city (%d)", len(world.Places), len(city.Places()))
	}
	if world.Character.Home.Key != city.Lives {
		t.Errorf("home = %q, want the flat they live in", world.Character.Home.Key)
	}
	// Every place carries its own ways out, because a portal drawn in a
	// painting is only where a way *is* — and most of the city is unpainted.
	for _, p := range world.Places {
		if len(p.Ways) == 0 {
			t.Errorf("%s has no ways out — it would be somewhere you cannot leave", p.Key)
		}
	}
	if world.Character.EarningsLeft != simulation.PaidActionsPerDay {
		t.Errorf("earnings left = %d on a fresh character, want %d",
			world.Character.EarningsLeft, simulation.PaidActionsPerDay)
	}
}

func TestWalkingChangesWhereThePlayerIs(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	rec := post(t, s, as, "character.move", "key-1", `{"to":"riverfront","by":"foot"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	trip := decodeTrip(t, rec)
	if trip.Character.At.Key != "riverfront" {
		t.Errorf("the walk reported arriving at %q", trip.Character.At.Key)
	}
	// The east side is two streets from the riverfront, and a walk is charged
	// for none of them.
	if len(trip.Steps) != 2 || trip.Fare != 0 {
		t.Errorf("the walk across town = %d steps costing %d cents, want 2 and 0", len(trip.Steps), trip.Fare)
	}
	if trip.Chances != len(trip.Steps) {
		t.Errorf("%d steps walked offered %d chances — walking is the encounter surface (§3.12)",
			len(trip.Steps), trip.Chances)
	}

	if got := snapshot(t, s, as).Character.At.Key; got != "riverfront" {
		t.Errorf("standing in %q after walking to the riverfront", got)
	}

	// Going somewhere that is not in the city is wrong forever, however often
	// it is sent — a 422 rather than a 409 (§8). So is a mode that is neither.
	rec = post(t, s, as, "character.move", "key-2", `{"to":"atlantis","by":"foot"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("walking to a place that does not exist = %d, want 422", rec.Code)
	}
	rec = post(t, s, as, "character.move", "key-3", `{"to":"downtown","by":"teleport"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("travelling by teleport = %d, want 422", rec.Code)
	}

	// And going to where you already are is not an error: a client that lost
	// the response to the first attempt has to be able to repeat it.
	rec = post(t, s, as, "character.move", "key-4", `{"to":"riverfront","by":"foot"}`)
	if rec.Code != http.StatusOK {
		t.Errorf("walking to where you already stand = %d, want 200", rec.Code)
	}
	if trip := decodeTrip(t, rec); len(trip.Steps) != 0 || trip.Chances != 0 {
		t.Errorf("standing still is %d steps and %d chances", len(trip.Steps), trip.Chances)
	}
}

// §3.12's layers, walked through: out of the flat, along the street, in through
// somebody else's door. One command for all of them, because there is one
// question being asked.
func TestWalkingReachesEveryLayerOfTheCity(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	for i, to := range []string{"east-side-apartment", "east-side", "the-strip-bar", "downtown-library"} {
		rec := post(t, s, as, "character.move", newKey(i), `{"to":"`+to+`","by":"foot"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("walking to %s = %d, body = %s", to, rec.Code, rec.Body)
		}
		trip := decodeTrip(t, rec)
		if trip.Character.At.Key != to {
			t.Fatalf("walking to %s left the player at %s", to, trip.Character.At.Key)
		}
		for _, step := range trip.Steps {
			if step.Mode != string(city.OnFoot) {
				t.Errorf("a step of a walk to %s was taken %q", to, step.Mode)
			}
			if step.Says == "" {
				t.Errorf("a step of the walk to %s has nothing to read", to)
			}
		}
	}

	if got := snapshot(t, s, as).Character.At.Key; got != "downtown-library" {
		t.Errorf("ended up in %q", got)
	}
}

// The trade (§3.12): the same journey, priced. Walking is free; the train takes
// the fare and passes through nothing.
func TestTakingTheTrainCostsTheFareAndSkipsWhatIsOnTheWay(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)
	id := accountOf(t, st)

	before := money(t, st, id)
	rec := post(t, s, as, "character.move", "ride", `{"to":"riverfront","by":"transit"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	trip := decodeTrip(t, rec)

	if trip.Fare != city.Fare.Cents() {
		t.Errorf("the fare was %d cents, want %d", trip.Fare, city.Fare.Cents())
	}
	if got := before - money(t, st, id); got != city.Fare.Cents() {
		t.Errorf("the ride took %d cents out of the balance, want %d", got, city.Fare.Cents())
	}
	if trip.Character.Money != money(t, st, id) {
		t.Errorf("the response says %d and the row says %d", trip.Character.Money, money(t, st, id))
	}
	// The whole of what the fare buys: nothing happened on the way.
	if trip.Chances != 0 {
		t.Errorf("a ride offered %d chances, want none", trip.Chances)
	}
	if len(trip.Steps) != 1 {
		t.Errorf("the ride was %d steps, want one", len(trip.Steps))
	}
}

// You cannot ride out of somebody's kitchen. The refusal is a 409, because
// walking out to the street makes the identical command work.
func TestThereIsNoTrainFromInsideABuilding(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	if rec := post(t, s, as, "character.move", "in", `{"to":"east-side-apartment","by":"foot"}`); rec.Code != http.StatusOK {
		t.Fatalf("walking into the flat = %d", rec.Code)
	}

	rec := post(t, s, as, "character.move", "ride", `{"to":"riverfront","by":"transit"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("riding out of the flat = %d, want 409; body = %s", rec.Code, rec.Body)
	}

	// Out to the street, and the same ride is there.
	if rec := post(t, s, as, "character.move", "out", `{"to":"east-side","by":"foot"}`); rec.Code != http.StatusOK {
		t.Fatalf("walking out of the flat = %d", rec.Code)
	}
	if rec := post(t, s, as, "character.move", "ride-2", `{"to":"riverfront","by":"transit"}`); rec.Code != http.StatusOK {
		t.Errorf("riding from the street = %d, want 200; body = %s", rec.Code, rec.Body)
	}
}

// A fare that cannot be paid refuses, and refuses without moving anybody: the
// journey and the money are one transaction (§3.2).
func TestAFareYouCannotAffordRefusesAndLeavesYouWhereYouWere(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)
	id := accountOf(t, st)

	// Spent down to less than a fare, through the interface another module
	// would use rather than by editing the row.
	balance, err := walletBalance(t, st, s.characters, id)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	if err := walletSpend(t, st, s.characters, id, balance-city.Fare+1); err != nil {
		t.Fatalf("spend: %v", err)
	}

	rec := post(t, s, as, "character.move", "ride", `{"to":"riverfront","by":"transit"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("a fare that cannot be paid = %d, want 409; body = %s", rec.Code, rec.Body)
	}

	world := snapshot(t, s, as)
	if world.Character.At.Key != city.Start {
		t.Errorf("the refused ride still moved the player to %q", world.Character.At.Key)
	}
	if got := money(t, st, id); got != city.Fare.Cents()-1 {
		t.Errorf("the refused ride left %d cents, want %d", got, city.Fare.Cents()-1)
	}

	// Walking is free, which is the point of the refusal being survivable.
	if rec := post(t, s, as, "character.move", "walk", `{"to":"riverfront","by":"foot"}`); rec.Code != http.StatusOK {
		t.Errorf("walking with no money = %d, want 200", rec.Code)
	}
}

// §3.1: movement costs no game time. Not one paid action, and not one day of
// the body — however far you go, and however you go.
func TestTravellingCostsNoGameTime(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)
	stopped(t, s, time.Date(2026, 8, 6, 15, 0, 0, 0, time.UTC))

	before := snapshot(t, s, as)

	for i, journey := range []string{
		`{"to":"east-side-apartment","by":"foot"}`,
		`{"to":"the-strip-bar","by":"foot"}`,
		`{"to":"the-strip","by":"foot"}`,
		`{"to":"downtown","by":"transit"}`,
		`{"to":"downtown-office","by":"foot"}`,
	} {
		if rec := post(t, s, as, "character.move", newKey(i), journey); rec.Code != http.StatusOK {
			t.Fatalf("%s = %d, body = %s", journey, rec.Code, rec.Body)
		}
	}

	after := snapshot(t, s, as)
	if after.Character.EarningsLeft != before.Character.EarningsLeft {
		t.Errorf("paid actions left went %d → %d over five journeys",
			before.Character.EarningsLeft, after.Character.EarningsLeft)
	}
	if after.Character.EarningsTaken != 0 {
		t.Errorf("%d paid actions taken by travelling", after.Character.EarningsTaken)
	}
	// A full day's work is still there afterwards, which is the same statement
	// from the other side.
	for i := range simulation.PaidActionsPerDay {
		if rec := post(t, s, as, "character.work", "work-"+strconv.Itoa(i), `{}`); rec.Code != http.StatusOK {
			t.Fatalf("paid action %d after travelling = %d", i+1, rec.Code)
		}
	}
}

// §3.4's cap, counted in Postgres rather than in a variable.
func TestThePaidActionCapHoldsAcrossCommands(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)
	stopped(t, s, time.Date(2026, 8, 6, 15, 0, 0, 0, time.UTC))

	start := money(t, st, accountOf(t, st))

	for i := range simulation.PaidActionsPerDay {
		rec := post(t, s, as, "character.work", newKey(i), `{}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("paid action %d = %d, body = %s", i+1, rec.Code, rec.Body)
		}
	}

	// The fourth is refused, and refused as a 409: the request is fine and it
	// is the day that is in the way. Tomorrow the identical request works.
	rec := post(t, s, as, "character.work", "key-over", `{}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("the fourth paid action = %d, want 409; body = %s", rec.Code, rec.Body)
	}

	earned := money(t, st, accountOf(t, st)) - start
	if earned != int64(simulation.PaidActionsPerDay)*dayRate.Cents() {
		t.Errorf("earned %d cents, want %d", earned, int64(simulation.PaidActionsPerDay)*dayRate.Cents())
	}
	if left := snapshot(t, s, as).Character.EarningsLeft; left != 0 {
		t.Errorf("earnings left = %d after spending the day's actions, want 0", left)
	}
}

// The count is stored with the day it belongs to, and how many are left is read
// from the two (§3.3). Nothing is written at midnight, so nothing can be missed.
func TestTheCapResetsAtThePlayersOwnMidnight(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)
	clk := stopped(t, s, time.Date(2026, 8, 6, 23, 30, 0, 0, time.UTC))

	for i := range simulation.PaidActionsPerDay {
		if rec := post(t, s, as, "character.work", newKey(i), `{}`); rec.Code != http.StatusOK {
			t.Fatalf("paid action %d = %d", i+1, rec.Code)
		}
	}
	if rec := post(t, s, as, "character.work", "key-late", `{}`); rec.Code != http.StatusConflict {
		t.Fatalf("the fourth action before midnight = %d, want 409", rec.Code)
	}

	// An hour later it is tomorrow, and nothing was written to make it so.
	clk.Advance(time.Hour)

	if left := snapshot(t, s, as).Character.EarningsLeft; left != simulation.PaidActionsPerDay {
		t.Errorf("earnings left after midnight = %d, want %d", left, simulation.PaidActionsPerDay)
	}
	if rec := post(t, s, as, "character.work", "key-tomorrow", `{}`); rec.Code != http.StatusOK {
		t.Errorf("the first action of the new day = %d, want 200", rec.Code)
	}
}

// The same command delivered twice applies once — the command layer's promise,
// asserted against money, where it matters most.
func TestARetriedPaidActionPaysOnce(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)
	stopped(t, s, time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC))

	before := money(t, st, accountOf(t, st))

	first := post(t, s, as, "character.work", "same-key", `{}`)
	second := post(t, s, as, "character.work", "same-key", `{}`)

	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("statuses = %d, %d; want both 200", first.Code, second.Code)
	}
	if second.Header().Get("Idempotent-Replay") != "true" {
		t.Error("the second delivery was not answered from the record")
	}
	if got := money(t, st, accountOf(t, st)) - before; got != dayRate.Cents() {
		t.Errorf("earned %d cents from one intent delivered twice, want %d", got, dayRate.Cents())
	}
}

// Cross-module access goes through an interface, and the interface is the only
// way another module can move money (§3.1). Spending more than there is refuses
// rather than going negative.
func TestSpendingThroughTheWalletCannotGoNegative(t *testing.T) {
	t.Parallel()
	s, st, _ := signedIn(t)
	id := accountOf(t, st)

	// Taken as the interface a consuming module would hold, not as the struct.
	var wallet character.Wallet = s.characters

	balance, err := walletBalance(t, st, wallet, id)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}

	if err := walletSpend(t, st, wallet, id, balance+1); err == nil {
		t.Fatal("spending more than the balance succeeded")
	}
	if after, _ := walletBalance(t, st, wallet, id); after != balance {
		t.Errorf("balance moved on a refused purchase: %d → %d", balance, after)
	}

	if err := walletSpend(t, st, wallet, id, balance); err != nil {
		t.Fatalf("spending the whole balance: %v", err)
	}
	if after, _ := walletBalance(t, st, wallet, id); after != 0 {
		t.Errorf("balance = %d after spending all of it", after)
	}
}

/* ── helpers ──────────────────────────────────────────────────────────── */

// accountOf returns the one account in a test's database.
func accountOf(t *testing.T, st *store.Store) string {
	t.Helper()
	var id string
	if err := st.Pool().QueryRow(t.Context(), `SELECT id FROM account`).Scan(&id); err != nil {
		t.Fatalf("read account: %v", err)
	}
	return id
}

// newKey is an idempotency key per intent, numbered so a loop can make several.
func newKey(n int) string { return "key-" + strconv.Itoa(n) }

// decodeTrip reads what a journey answered.
func decodeTrip(t *testing.T, rec *httptest.ResponseRecorder) (out struct {
	Character struct {
		Money int64 `json:"money"`
		At    struct {
			Key string `json:"key"`
		} `json:"at"`
	} `json:"character"`
	By    string `json:"by"`
	Steps []struct {
		To   string `json:"to"`
		Mode string `json:"mode"`
		Says string `json:"says"`
	} `json:"steps"`
	Fare    int64 `json:"fare"`
	Chances int   `json:"chances"`
}) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("the response is not a trip: %v", err)
	}
	return out
}

// The Wallet methods take the caller's transaction (§3.2), so a test that wants
// to use them supplies one the same way a command would.
func walletBalance(t *testing.T, st *store.Store, w character.Wallet, id string) (simulation.Money, error) {
	t.Helper()
	var got simulation.Money
	err := st.InTx(t.Context(), func(tx pgx.Tx) error {
		var err error
		got, err = w.Balance(t.Context(), tx, id)
		return err
	})
	return got, err
}

func walletSpend(t *testing.T, st *store.Store, w character.Wallet, id string, amount simulation.Money) error {
	t.Helper()
	return st.InTx(t.Context(), func(tx pgx.Tx) error {
		return w.Spend(t.Context(), tx, id, amount)
	})
}
