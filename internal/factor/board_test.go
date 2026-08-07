package factor

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
The leaderboards, against a real Postgres (§14, §3.5).

Three things are worth proving here and nowhere else: that the board is
distributed the way §3.5 asks (one overall plus one per factor, hidden ones
included), that a hidden board ranks without a number ever leaving the package,
and that a rebuild reports who moved so the socket has something to carry.
*/

// crowd puts n extra players into the world and returns their ids, oldest
// first. Each one gets a body and a balance of their own, because a leaderboard
// over identical players proves nothing about ordering.
func crowd(t *testing.T, ctx context.Context, tx pgx.Tx, w *world, n int) []string {
	t.Helper()
	out := make([]string, 0, n)
	for i := range n {
		id := newAccount(t, ctx, tx, fmt.Sprintf("board-%d", i))
		w.everyone = append(w.everyone, id)
		w.names[id] = fmt.Sprintf("Player %d", i)
		out = append(out, id)
	}
	return out
}

func at() time.Time { return time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC) }

/*
§3.5: the board is distributed — one overall, plus a separate board per factor.

Including the three the player is never shown, which is the clause this whole
file exists for: a board that ranked only the two revealed factors would be a
board that quietly told the player which factors matter.
*/
func TestThereIsOneBoardForTheCompositeAndOnePerFactor(t *testing.T) {
	t.Parallel()

	all := Boards()
	if len(all) != len(simulation.Factors)+1 {
		t.Fatalf("%d boards, want one per factor plus the composite", len(all))
	}
	if all[0].Key != Overall {
		t.Fatalf("the first board is %q, want the composite", all[0].Key)
	}

	sealed := 0
	seen := map[string]bool{}
	for _, b := range all {
		if seen[b.Key] {
			t.Fatalf("two boards answer to %q", b.Key)
		}
		seen[b.Key] = true

		if b.Shown {
			if b.Name == "" {
				t.Errorf("the %q board has no name", b.Key)
			}
			continue
		}
		sealed++
		if b.Name != "" || b.Reads != "" {
			t.Errorf("a sealed board carries something: %+v", b)
		}
		// And its key names a position rather than a factor.
		for _, f := range simulation.Factors {
			if !f.Shown() && b.Key == f.Key() {
				t.Errorf("the sealed board is keyed %q — that is the factor's name", b.Key)
			}
		}
	}
	if sealed != 3 {
		t.Fatalf("%d boards are sealed, want §3.5's three hidden factors", sealed)
	}
}

/*
The rebuild ranks everybody, and a hidden board carries no number anywhere.

The assertion is against the *table*, not against a view: §3.5's promise is that
showing a ranking discloses nothing about the formula, and the strongest form of
that is a schema in which a hidden reading has nowhere to be.
*/
func TestARebuildRanksEverybodyAndStoresNoHiddenReading(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Factors, w *world, account string) {
		others := crowd(t, ctx, tx, w, 4)

		// Give the players different bodies and balances so there is an order.
		for i, id := range append([]string{account}, others...) {
			w.purses[id] = simulation.Dollars(int64(1_000 * (i + 1)))
			body := simulation.Starting(1.78, 78)
			body.Fitness = float64(40 + 8*i)
			w.bodies[id] = body
			if _, err := m.Record(ctx, tx, id,
				simulation.Grant{Factor: simulation.Kindness, Credit: float64(50 * i)}); err != nil {
				t.Fatalf("record: %v", err)
			}
		}

		done, err := m.Rebuild(ctx, tx, at())
		if err != nil {
			t.Fatalf("rebuild: %v", err)
		}
		if done.Players != 5 {
			t.Fatalf("the rebuild ranked %d players, want 5", done.Players)
		}

		// Every board is complete: one row per player, ranks 1..n and no gaps.
		for _, b := range Boards() {
			var n, top, bottom int
			if err := tx.QueryRow(ctx, `
				SELECT count(*), coalesce(min(rank), 0), coalesce(max(rank), 0)
				  FROM factor_board WHERE board = $1`, b.Key).Scan(&n, &top, &bottom); err != nil {
				t.Fatalf("measure %s: %v", b.Key, err)
			}
			if n != 5 || top != 1 || bottom != 5 {
				t.Errorf("board %s has %d rows ranked %d..%d", b.Key, n, top, bottom)
			}

			var withValue int
			if err := tx.QueryRow(ctx,
				`SELECT count(*) FROM factor_board WHERE board = $1 AND value IS NOT NULL`,
				b.Key).Scan(&withValue); err != nil {
				t.Fatalf("read %s: %v", b.Key, err)
			}
			switch {
			case b.Shown && withValue != 5:
				t.Errorf("the %s board stored %d readings for 5 players", b.Key, withValue)
			case !b.Shown && withValue != 0:
				t.Errorf("a sealed board stored %d readings — §3.5 says a ranking, not a number", withValue)
			}
		}

		// And no row anywhere in the table names a hidden factor.
		var named int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM factor_board
			 WHERE board IN ('intelligence', 'stability', 'kindness')`).Scan(&named); err != nil {
			t.Fatalf("read boards: %v", err)
		}
		if named != 0 {
			t.Errorf("%d board rows are keyed by a hidden factor's name", named)
		}
	})
}

// The richest player is top of the money board, and the fittest is top of the
// health board — which is the board actually being sorted on the factor's score
// while displaying the reading §3.5 surfaces it as.
func TestTheBoardsAreInOrderAndShowTheReadingsTheDesignSurfaces(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Factors, w *world, account string) {
		others := crowd(t, ctx, tx, w, 2)
		rich, poor := others[0], others[1]

		w.purses[rich] = simulation.Dollars(500_000)
		w.purses[account] = simulation.Dollars(2_000)
		w.purses[poor] = simulation.Dollars(10)

		young := simulation.Starting(1.78, 78)
		young.Fitness = 90
		w.bodies[poor] = young

		if _, err := m.Rebuild(ctx, tx, at()); err != nil {
			t.Fatalf("rebuild: %v", err)
		}

		money, err := m.Page(ctx, tx, account, "money", 1)
		if err != nil {
			t.Fatalf("read the money board: %v", err)
		}
		if money.Total != 3 || len(money.Rows) != 3 {
			t.Fatalf("the money board has %d rows of %d", len(money.Rows), money.Total)
		}
		if money.Rows[0].Rank != 1 || *money.Rows[0].Value != float64(simulation.Dollars(500_000).Cents()) {
			t.Fatalf("the top of the money board is %+v", money.Rows[0])
		}
		if money.Rows[0].Player.Name == "" {
			t.Error("a rung on the board has nobody on it — identity comes from the account layer")
		}
		// And the reader knows where they are without paging to find out.
		if money.You == nil || money.You.Rank != 2 {
			t.Fatalf("the reader's own rung is %+v, want second of three", money.You)
		}

		health, err := m.Page(ctx, tx, account, "health", 1)
		if err != nil {
			t.Fatalf("read the health board: %v", err)
		}
		// The fittest body is top, and what is *shown* is the biological age —
		// the one number §3.5 wants driven downward. So the top of the board
		// carries the lowest value on it, which is the whole point.
		if *health.Rows[0].Value >= *health.Rows[2].Value {
			t.Fatalf("the health board reads %v at the top and %v at the bottom",
				*health.Rows[0].Value, *health.Rows[2].Value)
		}
		if *health.Rows[0].Value != simulation.BiologicalAge(young) {
			t.Fatalf("the top of the health board reads %v, want the biological age %v",
				*health.Rows[0].Value, simulation.BiologicalAge(young))
		}
	})
}

// A sealed board ranks, and hands back nothing else. This is §3.5's claim
// stated as a test: a ranking discloses nothing about the formula.
func TestASealedBoardRanksWithoutDisclosingANumber(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Factors, w *world, account string) {
		others := crowd(t, ctx, tx, w, 2)

		// The one who was kindest is top of *a* board, and nobody is told which.
		if _, err := m.Record(ctx, tx, others[0],
			simulation.Grant{Factor: simulation.Kindness, Credit: 900}); err != nil {
			t.Fatalf("record: %v", err)
		}
		if _, err := m.Rebuild(ctx, tx, at()); err != nil {
			t.Fatalf("rebuild: %v", err)
		}

		sealed := 0
		for _, b := range Boards() {
			if b.Shown {
				continue
			}
			sealed++
			page, err := m.Page(ctx, tx, account, b.Key, 1)
			if err != nil {
				t.Fatalf("read %s: %v", b.Key, err)
			}
			if page.Total != 3 {
				t.Fatalf("the sealed board %s ranks %d players", b.Key, page.Total)
			}
			for _, row := range page.Rows {
				if row.Value != nil {
					t.Errorf("a sealed board handed out %v", *row.Value)
				}
				if row.Rank == 0 || row.Player.Name == "" {
					t.Errorf("a sealed rung is %+v — a ranking still has to be a ranking", row)
				}
			}
			if page.You == nil || page.You.Value != nil {
				t.Errorf("the reader's own sealed rung is %+v", page.You)
			}
		}
		if sealed != 3 {
			t.Fatalf("%d sealed boards were readable, want 3", sealed)
		}
	})
}

// Reads are paginated, and the page size is the server's rather than the
// caller's — a board is unbounded, so the bound has to be here.
func TestBoardReadsArePaginated(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Factors, w *world, account string) {
		crowd(t, ctx, tx, w, PageSize+4)
		if _, err := m.Rebuild(ctx, tx, at()); err != nil {
			t.Fatalf("rebuild: %v", err)
		}

		first, err := m.Page(ctx, tx, account, Overall, 1)
		if err != nil {
			t.Fatalf("read page 1: %v", err)
		}
		if len(first.Rows) != PageSize {
			t.Fatalf("page 1 has %d rows, want %d", len(first.Rows), PageSize)
		}
		if first.Total != PageSize+5 || first.Pages != 2 {
			t.Fatalf("the board is %d players over %d pages", first.Total, first.Pages)
		}

		second, err := m.Page(ctx, tx, account, Overall, 2)
		if err != nil {
			t.Fatalf("read page 2: %v", err)
		}
		if len(second.Rows) != 5 {
			t.Fatalf("page 2 has %d rows, want the remaining 5", len(second.Rows))
		}
		if second.Rows[0].Rank != PageSize+1 {
			t.Fatalf("page 2 starts at rank %d", second.Rows[0].Rank)
		}

		// A page past the end is clamped rather than refused: the client asked a
		// reasonable question about a world that moved.
		past, err := m.Page(ctx, tx, account, Overall, 99)
		if err != nil {
			t.Fatalf("read page 99: %v", err)
		}
		if past.Page != 2 || len(past.Rows) != 5 {
			t.Fatalf("page 99 answered as page %d with %d rows", past.Page, len(past.Rows))
		}

		if _, err := m.Page(ctx, tx, account, "intelligence", 1); !errors.Is(err, ErrNoBoard) {
			t.Fatalf("reading a board by a hidden factor's name returned %v, want ErrNoBoard", err)
		}
	})
}

/*
A rebuild says who moved, and says nothing about anybody who did not.

That is what the socket carries (§8 lists "leaderboard changes" among the four
things the push channel was specified for), and the quiet half matters as much:
a message to every account every few minutes saying nothing changed would be the
polling this protocol exists instead of.
*/
func TestARebuildReportsOnlyThePlayersWhoMoved(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Factors, w *world, account string) {
		others := crowd(t, ctx, tx, w, 2)
		for i, id := range append([]string{account}, others...) {
			w.purses[id] = simulation.Dollars(int64(1_000 * (i + 1)))
		}

		// The first rebuild places everybody, which is a movement from nowhere.
		first, err := m.Rebuild(ctx, tx, at())
		if err != nil {
			t.Fatalf("rebuild: %v", err)
		}
		if len(first.Moves) != 3 {
			t.Fatalf("the first rebuild moved %d players, want all three placed", len(first.Moves))
		}
		for _, move := range first.Moves {
			for _, m := range move.Boards {
				if m.From != 0 {
					t.Errorf("a first placing came from rank %d", m.From)
				}
				if m.Of != 3 {
					t.Errorf("a movement reports a field of %d", m.Of)
				}
			}
		}

		// Nothing changed, so nobody is told anything.
		quiet, err := m.Rebuild(ctx, tx, at().Add(time.Minute))
		if err != nil {
			t.Fatalf("rebuild: %v", err)
		}
		if len(quiet.Moves) != 0 {
			t.Fatalf("an unchanged rebuild announced %d moves", len(quiet.Moves))
		}

		// The player at the bottom of the money board buys their way to the top.
		w.purses[account] = simulation.Dollars(9_000_000)
		moved, err := m.Rebuild(ctx, tx, at().Add(2*time.Minute))
		if err != nil {
			t.Fatalf("rebuild: %v", err)
		}
		if len(moved.Moves) == 0 {
			t.Fatal("a player who went from last to first moved nobody")
		}

		found := false
		for _, move := range moved.Moves {
			if move.Account != account {
				continue
			}
			for _, m := range move.Boards {
				if m.Board.Key == "money" {
					found = true
					if m.To != 1 || m.From == 1 {
						t.Errorf("the money board moved them %d → %d", m.From, m.To)
					}
				}
			}
		}
		if !found {
			t.Error("the player who got rich was not told their money rank moved")
		}
	})
}

/*
The index is every board with the reader's own standing on each, and it says how
old it is.

A leaderboard is a snapshot by design (§3.4 puts the rebuild on the queue), so
the moment it was taken is part of the answer rather than something the client
has to assume.
*/
func TestTheIndexCarriesEveryBoardAndTheAgeOfTheSnapshot(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Factors, w *world, account string) {
		// Before any rebuild: every board is there, nobody is ranked, and the
		// snapshot has no age because there has not been one.
		empty, err := m.Standings(ctx, tx, account)
		if err != nil {
			t.Fatalf("read the index: %v", err)
		}
		if len(empty.Boards) != len(Boards()) {
			t.Fatalf("the index has %d boards, want %d", len(empty.Boards), len(Boards()))
		}
		if !empty.RefreshedAt.IsZero() || empty.Players != 0 {
			t.Fatalf("a world with no rebuild reports %+v", empty)
		}
		for _, p := range empty.Boards {
			if p.Rank != 0 || p.Of != 0 {
				t.Errorf("an unbuilt board places somebody: %+v", p)
			}
		}

		crowd(t, ctx, tx, w, 2)
		if _, err := m.Rebuild(ctx, tx, at()); err != nil {
			t.Fatalf("rebuild: %v", err)
		}

		built, err := m.Standings(ctx, tx, account)
		if err != nil {
			t.Fatalf("read the index: %v", err)
		}
		if !built.RefreshedAt.Equal(at()) || built.Players != 3 {
			t.Fatalf("the index reports %v over %d players", built.RefreshedAt, built.Players)
		}
		for _, p := range built.Boards {
			if p.Rank < 1 || p.Of != 3 {
				t.Errorf("board %s places the reader %d of %d", p.Board.Key, p.Rank, p.Of)
			}
			if !p.Board.Shown && p.Value != nil {
				t.Errorf("the index carries a sealed reading: %+v", p)
			}
		}
	})
}

// Ties are broken deterministically, so a rank is a position rather than
// something that flaps between rebuilds for two players who read identically.
func TestTiesDoNotFlapBetweenRebuilds(t *testing.T) {
	t.Parallel()

	with(t, func(ctx context.Context, tx pgx.Tx, m *Factors, w *world, account string) {
		crowd(t, ctx, tx, w, 5)

		if _, err := m.Rebuild(ctx, tx, at()); err != nil {
			t.Fatalf("rebuild: %v", err)
		}
		before, err := m.Page(ctx, tx, account, Overall, 1)
		if err != nil {
			t.Fatalf("read the board: %v", err)
		}

		again, err := m.Rebuild(ctx, tx, at().Add(time.Hour))
		if err != nil {
			t.Fatalf("rebuild: %v", err)
		}
		if len(again.Moves) != 0 {
			t.Fatalf("six identical players reshuffled into %d moves", len(again.Moves))
		}

		after, err := m.Page(ctx, tx, account, Overall, 1)
		if err != nil {
			t.Fatalf("read the board: %v", err)
		}
		for i := range before.Rows {
			if before.Rows[i].Player.ID != after.Rows[i].Player.ID {
				t.Fatalf("rank %d changed hands with nothing having happened", i+1)
			}
		}
	})
}
