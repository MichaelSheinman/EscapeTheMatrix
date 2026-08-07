package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/character"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/clock"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/health"
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
	s.characters = character.New(clk)
	s.health = health.New(clk, s.characters)
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
		Key string `json:"key"`
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
	if len(world.Places) != len(character.Districts) {
		t.Errorf("%d districts offered, want %d", len(world.Places), len(character.Districts))
	}
	if world.Character.EarningsLeft != simulation.PaidActionsPerDay {
		t.Errorf("earnings left = %d on a fresh character, want %d",
			world.Character.EarningsLeft, simulation.PaidActionsPerDay)
	}
}

func TestWalkingChangesWhereThePlayerIs(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	rec := post(t, s, as, "character.move", "key-1", `{"district":"riverfront"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	if got := snapshot(t, s, as).Character.At.Key; got != "riverfront" {
		t.Errorf("standing in %q after walking to the riverfront", got)
	}

	// Walking somewhere that is not in the city is wrong forever, however often
	// it is sent — a 422 rather than a 409 (§8).
	rec = post(t, s, as, "character.move", "key-2", `{"district":"atlantis"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("walking to a place that does not exist = %d, want 422", rec.Code)
	}

	// And walking to where you already are is not an error: a client that lost
	// the response to the first attempt has to be able to repeat it.
	rec = post(t, s, as, "character.move", "key-3", `{"district":"riverfront"}`)
	if rec.Code != http.StatusOK {
		t.Errorf("walking to where you already stand = %d, want 200", rec.Code)
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
