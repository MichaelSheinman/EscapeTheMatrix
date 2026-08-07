package factor

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
The leaderboards (§3.5).

**The board is distributed:** one overall board for the composite, plus a
separate board for each individual factor. That is the design's own wording, and
the reason it is in this file rather than in a package of its own is the veil.
Ranking the three hidden factors means reading them, and the rule this package
exists to keep is that they do not leave it — so the sort happens on the inside,
and what comes out is a *position*.

# Why a ranking discloses nothing

A hidden board's rows carry a rank, a name, and no number at all. There is no
column in the schema a hidden reading is written to (see 00011_boards.sql), no
field on [Row] it could occupy, and no board key on the wire that names the
factor being ranked — the three sealed boards are "slot-3", "slot-4" and
"slot-5", which is the same positional handle a locked [Slot] has. A player can
therefore see that they are 14th at *something*, and work at finding out what,
which is exactly §3.5's hook stated as a mechanic instead of as a mystery.

# Why it is a snapshot and not a query

§3.4 lists "leaderboard refresh" among V1's job types, and a rank is why: it is a
global fact about every player at once. Composing it per request would mean
sorting the whole population on a page load and *still* handing back a snapshot,
merely an expensive one taken at a worse moment. So [Factors.Rebuild] replaces
the whole table under an advisory lock on a schedule, and a read is an index scan
over something already in order.

The honest cost of that is staleness, which is why [Index] carries the moment it
was built: a number minutes old should say so.
*/

// ErrNoBoard is a board key nobody publishes.
var ErrNoBoard = errors.New("factor: no such board")

// Overall is the composite's own board — §3.5's "the overall board gets an
// evocative name". It is the one board that is not a factor.
const Overall = "karma"

// PageSize is how many rungs a page of a board carries.
//
// A screenful, and a bound on the query rather than a preference: the read is
// paginated because a board is unbounded, and letting the caller choose the page
// size would let it choose "all of them".
const PageSize = 25

// lock is the advisory lock a rebuild takes, so that two instances refreshing at
// once serialise instead of deadlocking on each other's DELETE.
//
// An arbitrary constant, and it only has to be unique within this application.
// Transaction-scoped, so it is released by the commit or the rollback and there
// is no path that leaks it.
const lock = 0x_B0A2D_1

/*
Player is who a rung on the board belongs to.

Identity comes from the account layer (technical.md's module map calls it
"leaderboard identity"), and this is the whole of what a board is allowed to know
about a person: what they are called and what they look like. No email, no
provider, no session — a board is a public surface, and the narrow type is what
makes "could a leaderboard leak an address" a question with a structural answer.
*/
type Player struct {
	ID     string
	Name   string
	Avatar string
}

/*
Roster is the population a board ranks, and who they are on it.

An interface for the same reason [Panel] and [Purse] are: this package composes
progression and has no business reading the account table (§3.1). internal/api
binds it, in one small adapter, next to the one that answers [Environment].
*/
type Roster interface {
	// Everyone is every account that may appear on a board.
	Everyone(ctx context.Context, tx pgx.Tx) ([]string, error)
	// Identify names the given accounts. Order is not required to match, and an
	// account that has since been deleted may simply be absent.
	Identify(ctx context.Context, tx pgx.Tx, ids []string) ([]Player, error)
}

// Alone is a Roster for a world with one player in it and no account table to
// ask: nobody is ranked, and a rebuild writes nothing. Never bound in the
// server.
type Alone struct{}

func (Alone) Everyone(context.Context, pgx.Tx) ([]string, error) { return nil, nil }

func (Alone) Identify(context.Context, pgx.Tx, []string) ([]Player, error) { return nil, nil }

/*
Board is one leaderboard, as the player is allowed to see it.

Same veil as [Slot], applied to the same five factors plus the composite: a
sealed board has a key that names a position rather than a factor, no name, and a
note that deliberately does not explain what it measures.

Reads says how to render a row's value — "karma", "age" or "money" — so the
client formats without knowing a rule. Empty for a sealed board, which has no
value to format.
*/
type Board struct {
	Key   string
	Name  string
	Note  string
	Reads string
	Shown bool
}

/*
Boards is every board there is, in §3.5's order: the composite, then one per
factor in the order the design tabulates them.

Authored vocabulary, sent to the client rather than compiled into it, for the
same reason the city's places are (§17 leaves the factor list open past V1).
*/
func Boards() []Board {
	out := make([]Board, 0, len(simulation.Factors)+1)
	out = append(out, Board{
		Key:   Overall,
		Name:  "Karma",
		Note:  "everything at once, composed fresh — which is why it can fall",
		Reads: "karma",
		Shown: true,
	})
	for i, f := range simulation.Factors {
		out = append(out, board(f, i))
	}
	return out
}

// board is one factor's board, veiled.
func board(f simulation.Factor, at int) Board {
	if !f.Shown() {
		return Board{
			// A position, not a factor. The client keys a sealed board by this
			// exactly as it keys a sealed slot by its index — and the two lists
			// are in the same order, so the fourth board and the third slot are
			// the same unnamed thing.
			Key:  fmt.Sprintf("slot-%d", at+1),
			Note: "a real ranking of something nobody has told you the name of",
		}
	}
	switch f {
	case simulation.Health:
		return Board{
			Key:   f.Key(),
			Name:  "Biological age",
			Note:  "youngest first — the one number here worth driving downward",
			Reads: "age",
			Shown: true,
		}
	default:
		return Board{
			Key:   f.Key(),
			Name:  "Money",
			Note:  "what is being held, not what has been earned",
			Reads: "money",
			Shown: true,
		}
	}
}

// find returns the board a wire key names.
func find(key string) (Board, bool) {
	for _, b := range Boards() {
		if b.Key == key {
			return b, true
		}
	}
	return Board{}, false
}

// Row is one rung: who, where, and — on a board that shows one — what they read.
type Row struct {
	Rank   int
	Player Player
	// Value is nil on a sealed board. Not zero: a zero is a reading, and there
	// is no reading anywhere for a hidden factor to have leaked out of.
	Value *float64
	// You marks the reader's own rung, so a page that happens to contain them
	// can say so without the client comparing account ids it does not have.
	You bool
}

// Placing is where one player stands on one board.
type Placing struct {
	Board Board
	// Rank is 0 for a player who is not on the board — nobody has been ranked
	// yet, or they joined after the last rebuild.
	Rank int
	// Of is how many players are ranked on it. A rank without a field size is
	// half a sentence.
	Of    int
	Value *float64
}

/*
Index is every board at once, with the reader's own standing on each.

What the boards screen opens with, and what the socket's rank messages patch. It
is deliberately not part of the world snapshot: a leaderboard is a read-only
paginated list, which technical.md §10.2 names as the one genuinely pull-shaped
thing in this client.
*/
type Index struct {
	// RefreshedAt is when the job last rebuilt the table. Zero before it ever
	// has, which is a state the client is told about rather than one it guesses.
	RefreshedAt time.Time
	Players     int
	Boards      []Placing
}

// Page is one page of one board.
type Page struct {
	Board Board
	Rows  []Row
	// Page is the page actually served, which may not be the page asked for —
	// see Factors.Page.
	Page  int
	Pages int
	Total int
	// You is the reader's own rung, wherever on the board it is. A player is
	// entitled to know where they stand without paging to find out.
	You         *Placing
	RefreshedAt time.Time
}

/*
Standings is the index of boards, with this player's rank on each.

Every board appears, including the ones nobody is ranked on yet, because the
board *existing* is part of what §3.5 shows the player — the same argument as a
locked slot being drawn at all.
*/
func (f *Factors) Standings(ctx context.Context, tx pgx.Tx, accountID string) (Index, error) {
	out := Index{Boards: make([]Placing, 0, len(Boards()))}

	at, players, err := f.refreshed(ctx, tx)
	if err != nil {
		return Index{}, err
	}
	out.RefreshedAt, out.Players = at, players

	sizes, err := f.sizes(ctx, tx)
	if err != nil {
		return Index{}, err
	}

	mine := map[string]Placing{}
	rows, err := tx.Query(ctx,
		`SELECT board, rank, value FROM factor_board WHERE account_id = $1`, accountID)
	if err != nil {
		return Index{}, fmt.Errorf("factor: read standings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			key   string
			rank  int
			value *float64
		)
		if err := rows.Scan(&key, &rank, &value); err != nil {
			return Index{}, fmt.Errorf("factor: read standings: %w", err)
		}
		mine[key] = Placing{Rank: rank, Value: value}
	}
	if err := rows.Err(); err != nil {
		return Index{}, fmt.Errorf("factor: read standings: %w", err)
	}

	for _, b := range Boards() {
		p := mine[b.Key]
		p.Board, p.Of = b, sizes[b.Key]
		out.Boards = append(out.Boards, p)
	}
	return out, nil
}

/*
Page reads one page of one board.

`page` is 1-based and is clamped rather than rejected: a client holding page four
of a board that has since shrunk to two asked a reasonable question about a world
that moved, and answering with the last page is more useful than a 422. The page
actually served comes back in the answer.
*/
func (f *Factors) Page(ctx context.Context, tx pgx.Tx, accountID, key string, page int) (Page, error) {
	b, ok := find(key)
	if !ok {
		return Page{}, fmt.Errorf("%w: %s", ErrNoBoard, key)
	}

	out := Page{Board: b, Rows: []Row{}, Page: 1}

	at, _, err := f.refreshed(ctx, tx)
	if err != nil {
		return Page{}, err
	}
	out.RefreshedAt = at

	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM factor_board WHERE board = $1`, key).Scan(&out.Total); err != nil {
		return Page{}, fmt.Errorf("factor: measure board: %w", err)
	}
	out.Pages = int(math.Ceil(float64(out.Total) / float64(PageSize)))

	out.Page = page
	if out.Page < 1 {
		out.Page = 1
	}
	if out.Pages > 0 && out.Page > out.Pages {
		out.Page = out.Pages
	}

	// The reader's own rung, read separately and unconditionally. It is the one
	// row a board is asked for that is not on the page being looked at.
	var (
		rank  int
		value *float64
	)
	switch err := tx.QueryRow(ctx,
		`SELECT rank, value FROM factor_board WHERE board = $1 AND account_id = $2`,
		key, accountID).Scan(&rank, &value); {
	case errors.Is(err, pgx.ErrNoRows):
		// Not ranked. Not an error: they have not been rebuilt into the board
		// yet, which is a sentence on screen rather than a failure.
	case err != nil:
		return Page{}, fmt.Errorf("factor: read own rank: %w", err)
	default:
		out.You = &Placing{Board: b, Rank: rank, Of: out.Total, Value: value}
	}

	if out.Total == 0 {
		return out, nil
	}

	from := (out.Page - 1) * PageSize
	rows, err := tx.Query(ctx, `
		SELECT rank, account_id, value
		  FROM factor_board
		 WHERE board = $1 AND rank > $2 AND rank <= $3
		 ORDER BY rank`, key, from, from+PageSize)
	if err != nil {
		return Page{}, fmt.Errorf("factor: read board: %w", err)
	}
	defer rows.Close()

	ids := make([]string, 0, PageSize)
	for rows.Next() {
		var (
			r     Row
			id    string
			value *float64
		)
		if err := rows.Scan(&r.Rank, &id, &value); err != nil {
			return Page{}, fmt.Errorf("factor: read board: %w", err)
		}
		r.Value = value
		r.Player = Player{ID: id}
		r.You = id == accountID
		ids = append(ids, id)
		out.Rows = append(out.Rows, r)
	}
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("factor: read board: %w", err)
	}

	// Identity is fetched for the page rather than joined, because the account
	// table is not this module's to select from (§3.1) — one call, one page.
	people, err := f.roster.Identify(ctx, tx, ids)
	if err != nil {
		return Page{}, err
	}
	named := make(map[string]Player, len(people))
	for _, p := range people {
		named[p.ID] = p
	}
	for i := range out.Rows {
		if p, ok := named[out.Rows[i].Player.ID]; ok {
			out.Rows[i].Player = p
		}
	}
	return out, nil
}

/*
Movement is one player's rank on one board, before and after a rebuild.

From is 0 when they were not on the board — a first placing is a movement too,
and reporting it as "from nowhere" is more honest than not reporting it.
*/
type Movement struct {
	Board Board
	From  int
	To    int
	Of    int
}

// Move is everything that changed for one player in one rebuild.
type Move struct {
	Account string
	Boards  []Movement
}

/*
Rebuilt is what one rebuild did.

Moves is what the caller pushes down the socket (§8: "leaderboard changes" is one
of the four things the push channel was specified to carry). It is deliberately
returned rather than published from here: this package has no idea a socket
exists, and the message shapes live next to the views they carry.
*/
type Rebuilt struct {
	At      time.Time
	Players int
	Moves   []Move
}

/*
Rebuild recomposes every board and replaces the table.

Run by the background job (§3.4) and by nothing else. Everything it reads is an
input that was persisted and everything it writes is an ordering, which is the
one form of derived state in this codebase that *is* stored — because a rank is
not a fact about one player, and §3.3's "computed on read" has no meaning for a
number that requires reading everybody.

The whole thing is one transaction on purpose. A board half-replaced is a board
with two number ones in it, and the unique index would refuse it — loudly, which
is the correct outcome, but the transaction means it cannot arise.
*/
func (f *Factors) Rebuild(ctx context.Context, tx pgx.Tx, at time.Time) (Rebuilt, error) {
	// Serialise with any other instance doing the same thing. Transaction-
	// scoped, so it goes away with the commit.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(lock)); err != nil {
		return Rebuilt{}, fmt.Errorf("factor: lock boards: %w", err)
	}

	was, err := f.ranks(ctx, tx)
	if err != nil {
		return Rebuilt{}, err
	}

	ids, err := f.roster.Everyone(ctx, tx)
	if err != nil {
		return Rebuilt{}, err
	}

	credit, err := f.everyCredit(ctx, tx)
	if err != nil {
		return Rebuilt{}, err
	}

	// One entry per player per board, gathered before anything is sorted.
	//
	// The readings are composed one player at a time through the same two narrow
	// interfaces a single standing is (§3.1) — the module does not gain a bulk
	// path into somebody else's tables just because it is looking at everybody.
	// That is a query per player per module, which is the honest cost of the
	// boundary and is why this runs on a schedule rather than on a request. A
	// population that outgrows it wants a windowed rebuild, not a join.
	entries := map[string][]entry{}
	for _, id := range ids {
		body, err := f.panel.Body(ctx, tx, id)
		if err != nil {
			return Rebuilt{}, err
		}
		held, err := f.purse.Balance(ctx, tx, id)
		if err != nil {
			return Rebuilt{}, err
		}

		// §3.15's ceilings, read per player, because they are a fact about the
		// population that player lives among rather than about the board. A
		// ranking taken without them would seat somebody who has exhausted
		// their environment above somebody who has not, on credit the first of
		// them can no longer demonstrate anywhere — which would make the boards
		// reward grinding one place, the exact thing the ceilings exist to stop.
		where, _, err := f.tenure(ctx, tx, id, false)
		if err != nil {
			return Rebuilt{}, err
		}
		ceilings, err := f.ceilings(ctx, tx, id, where)
		if err != nil {
			return Rebuilt{}, err
		}

		reading := simulation.Karma(simulation.Standing{
			Body:     body,
			Purse:    held,
			Credit:   credit[id],
			Ceilings: ceilings,
		})

		age := simulation.BiologicalAge(body)
		cents := float64(held.Cents())
		entries[Overall] = append(entries[Overall], entry{id, reading.Karma, &reading.Karma})

		for i, factor := range simulation.Factors {
			b := board(factor, i)
			e := entry{account: id, score: reading.Of(factor)}
			switch {
			case !b.Shown:
				// No value, and this is the whole of the veil on this path: the
				// number that decided the order is never carried alongside it.
			case factor == simulation.Health:
				// Ranked on the factor's score — younger is higher — and shown
				// as the biological age, which is the reading §3.5 surfaces.
				v := age
				e.value = &v
			default:
				v := cents
				e.value = &v
			}
			entries[b.Key] = append(entries[b.Key], e)
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM factor_board`); err != nil {
		return Rebuilt{}, fmt.Errorf("factor: clear boards: %w", err)
	}

	now := map[string]map[string]int{}
	sizes := map[string]int{}
	var rows [][]any
	for _, b := range Boards() {
		list := entries[b.Key]
		// Descending by score, and ties broken by account id so that a rank is a
		// stable position rather than something that flaps between rebuilds for
		// two players who read identically.
		sort.Slice(list, func(i, j int) bool {
			if list[i].score != list[j].score {
				return list[i].score > list[j].score
			}
			return list[i].account < list[j].account
		})

		sizes[b.Key] = len(list)
		placed := make(map[string]int, len(list))
		for i, e := range list {
			rank := i + 1
			placed[e.account] = rank
			rows = append(rows, []any{b.Key, e.account, rank, e.value})
		}
		now[b.Key] = placed
	}

	if len(rows) > 0 {
		if _, err := tx.CopyFrom(ctx,
			pgx.Identifier{"factor_board"},
			[]string{"board", "account_id", "rank", "value"},
			pgx.CopyFromRows(rows),
		); err != nil {
			return Rebuilt{}, fmt.Errorf("factor: write boards: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO factor_board_refresh (only_row, refreshed_at, players)
		VALUES (true, $1, $2)
		ON CONFLICT (only_row) DO UPDATE
		   SET refreshed_at = excluded.refreshed_at, players = excluded.players`,
		at, len(ids)); err != nil {
		return Rebuilt{}, fmt.Errorf("factor: stamp boards: %w", err)
	}

	return Rebuilt{At: at, Players: len(ids), Moves: moved(was, now, sizes)}, nil
}

// entry is one player's place on one board, before it has one.
type entry struct {
	account string
	score   float64
	value   *float64
}

/*
moved is who ended up somewhere they were not.

A player whose rank is unchanged on every board is not in the answer at all, so a
quiet rebuild pushes nothing — which matters, because the alternative is a
message to every account in the game every few minutes for news that is not news.
*/
func moved(was, now map[string]map[string]int, sizes map[string]int) []Move {
	by := map[string][]Movement{}
	for _, b := range Boards() {
		for account, to := range now[b.Key] {
			from := was[b.Key][account]
			if from == to {
				continue
			}
			by[account] = append(by[account], Movement{Board: b, From: from, To: to, Of: sizes[b.Key]})
		}
	}

	// Sorted by account so a rebuild's messages go out in a stable order, which
	// is what makes a test able to say what it published.
	accounts := make([]string, 0, len(by))
	for account := range by {
		accounts = append(accounts, account)
	}
	sort.Strings(accounts)

	out := make([]Move, 0, len(accounts))
	for _, account := range accounts {
		list := by[account]
		sort.Slice(list, func(i, j int) bool { return list[i].Board.Key < list[j].Board.Key })
		out = append(out, Move{Account: account, Boards: list})
	}
	return out
}

/* ── the database ─────────────────────────────────────────────────────── */

// refreshed is when the boards were last built, and over how many players. The
// zero time before they ever have been.
func (f *Factors) refreshed(ctx context.Context, tx pgx.Tx) (time.Time, int, error) {
	var (
		at      time.Time
		players int
	)
	switch err := tx.QueryRow(ctx,
		`SELECT refreshed_at, players FROM factor_board_refresh`).Scan(&at, &players); {
	case errors.Is(err, pgx.ErrNoRows):
		return time.Time{}, 0, nil
	case err != nil:
		return time.Time{}, 0, fmt.Errorf("factor: read board age: %w", err)
	}
	return at, players, nil
}

// sizes is how many players are ranked on each board.
func (f *Factors) sizes(ctx context.Context, tx pgx.Tx) (map[string]int, error) {
	rows, err := tx.Query(ctx, `SELECT board, count(*) FROM factor_board GROUP BY board`)
	if err != nil {
		return nil, fmt.Errorf("factor: measure boards: %w", err)
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var (
			key string
			n   int
		)
		if err := rows.Scan(&key, &n); err != nil {
			return nil, fmt.Errorf("factor: measure boards: %w", err)
		}
		out[key] = n
	}
	return out, rows.Err()
}

// ranks is the whole table as board → account → rank, for the before-and-after.
func (f *Factors) ranks(ctx context.Context, tx pgx.Tx) (map[string]map[string]int, error) {
	rows, err := tx.Query(ctx, `SELECT board, account_id, rank FROM factor_board`)
	if err != nil {
		return nil, fmt.Errorf("factor: read boards: %w", err)
	}
	defer rows.Close()

	out := map[string]map[string]int{}
	for rows.Next() {
		var (
			key     string
			account string
			rank    int
		)
		if err := rows.Scan(&key, &account, &rank); err != nil {
			return nil, fmt.Errorf("factor: read boards: %w", err)
		}
		if out[key] == nil {
			out[key] = map[string]int{}
		}
		out[key][account] = rank
	}
	return out, rows.Err()
}

// everyCredit is the hidden totals for every player, in one read.
//
// The one bulk query in the rebuild, and it is over this module's own table —
// which is exactly the line §3.1 draws. Nobody else's rows are read in bulk.
func (f *Factors) everyCredit(ctx context.Context, tx pgx.Tx) (map[string]simulation.Credit, error) {
	rows, err := tx.Query(ctx, `SELECT account_id, factor, credit FROM factor_credit`)
	if err != nil {
		return nil, fmt.Errorf("factor: read every credit: %w", err)
	}
	defer rows.Close()

	out := map[string]simulation.Credit{}
	for rows.Next() {
		var (
			account string
			key     string
			credit  float64
		)
		if err := rows.Scan(&account, &key, &credit); err != nil {
			return nil, fmt.Errorf("factor: read every credit: %w", err)
		}
		for _, known := range simulation.Factors {
			if known.Key() == key && known.Granted() {
				out[account] = out[account].Plus(known, credit)
			}
		}
	}
	return out, rows.Err()
}
