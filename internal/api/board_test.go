package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/factor"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
The leaderboards, at the edge they are allowed to reach (§3.5).

The ranking itself is proved in internal/factor. What is worth proving here is
the pair of things only the transport can show: that a board read is
authenticated, paginated and answers with somebody else's *name* rather than
their account, and that nothing about §3.5's three hidden factors survives the
trip — not a key, not a value, not the word.
*/

// boards reads the index as the player holding `as`.
func boards(t *testing.T, s *Server, as *http.Cookie) indexView {
	t.Helper()
	rec := read(t, s, as, boardsPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var out indexView
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not a board index: %v", err)
	}
	return out
}

// boardPageAt reads one page of one board.
func boardPageAt(t *testing.T, s *Server, as *http.Cookie, key string, page int) pageView {
	t.Helper()
	at := boardsPath + "/" + key
	if page > 0 {
		at += "?page=" + strconv.Itoa(page)
	}
	rec := read(t, s, as, at)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var out pageView
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not a board page: %v", err)
	}
	return out
}

/*
The index carries every board §3.5 asks for, and the three sealed ones arrive
carrying nothing but a position.
*/
func TestTheBoardIndexCarriesEveryBoardAndNamesNoHiddenFactor(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	if err := s.RefreshBoards(t.Context()); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	index := boards(t, s, as)
	if len(index.Boards) != len(simulation.Factors)+1 {
		t.Fatalf("the index has %d boards, want one per factor plus the composite", len(index.Boards))
	}
	if index.RefreshedAt == "" || index.Players != 1 {
		t.Fatalf("the index reports %+v, want a snapshot over the one player there is", index)
	}

	sealed := 0
	for _, p := range index.Boards {
		if p.Rank != 1 || p.Of != 1 {
			t.Errorf("the only player is %d of %d on %q", p.Rank, p.Of, p.Board.Key)
		}
		if p.Board.Shown {
			continue
		}
		sealed++
		if p.Board.Name != "" || p.Board.Reads != "" || p.Value != nil {
			t.Errorf("a sealed board reached the wire carrying something: %+v", p)
		}
	}
	if sealed != 3 {
		t.Fatalf("%d sealed boards, want §3.5's three", sealed)
	}
}

/*
The strongest assertion available, the same one the world snapshot gets: the
whole payload read as text.

§3.5's hidden boards rank without disclosing how the number is produced, and a
board named `kindness` would disclose exactly that. This catches a careless field
on a view, a debug dump, or a board key that leaked the pure core's spelling.
*/
func TestNoHiddenFactorIsNamedAnywhereInAnyBoard(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	if err := s.RefreshBoards(t.Context()); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	paths := []string{boardsPath}
	for _, b := range factor.Boards() {
		paths = append(paths, boardsPath+"/"+b.Key)
	}

	for _, path := range paths {
		rec := read(t, s, as, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, body = %s", path, rec.Code, rec.Body)
		}
		body := rec.Body.String()
		for _, f := range simulation.Factors {
			if f.Shown() {
				continue
			}
			if contains(body, f.Key()) {
				t.Errorf("%q appears in %s — a ranking is meant to disclose nothing (§3.5)", f.Key(), path)
			}
		}
	}
}

// A board is per-player enough to sit behind the same guard everything else
// does (§9): there is no anonymous mode, and "where do I stand" has no answer
// for nobody.
func TestTheBoardsAreBehindTheDoor(t *testing.T) {
	t.Parallel()
	s, _ := newServer(t)

	for _, path := range []string{boardsPath, boardsPath + "/karma"} {
		if rec := read(t, s, nil, path); rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s with no session = %d, want 401", path, rec.Code)
		}
	}
}

/*
A page of a board carries other players by name, and never by account.

Identity comes from the account layer (§3.5's leaderboard identity) and stops
there: a display name and a picture. An email would be a privacy incident and an
account id would be a handle #17's endorsement should not be able to inherit
from a list anybody can page through.
*/
func TestABoardPageNamesPlayersAndDiscloseNothingElseAboutThem(t *testing.T) {
	t.Parallel()
	s, _ := newServer(t)

	mine := signIn(t, s, "abc")
	signIn(t, s, "def")
	signIn(t, s, "ghi")

	if err := s.RefreshBoards(t.Context()); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	page := boardPageAt(t, s, mine, factor.Overall, 1)
	if page.Total != 3 || len(page.Rows) != 3 {
		t.Fatalf("the composite board has %d of %d rows", len(page.Rows), page.Total)
	}
	if page.Size != factor.PageSize || page.Pages != 1 || page.Page != 1 {
		t.Fatalf("pagination reads %+v", page)
	}

	mineFound := 0
	for i, row := range page.Rows {
		if row.Rank != i+1 {
			t.Errorf("row %d is ranked %d", i, row.Rank)
		}
		if row.Name == "" {
			t.Errorf("rank %d has nobody on it", row.Rank)
		}
		if row.You {
			mineFound++
		}
	}
	if mineFound != 1 {
		t.Errorf("%d rows are marked as the reader, want exactly one", mineFound)
	}

	// The reader's own rung comes back whether or not it is on this page.
	if page.You == nil || page.You.Rank < 1 {
		t.Fatalf("the reader's own rung is %+v", page.You)
	}

	// And nothing about anybody's account made the trip.
	body := read(t, s, mine, boardsPath+"/"+factor.Overall).Body.String()
	for _, leak := range []string{"@example.test", "provider", "account"} {
		if contains(body, leak) {
			t.Errorf("%q appears on a public board", leak)
		}
	}
}

// A board nobody publishes is a 404, and a page that is not a number is a 422 —
// §8's status codes as protocol, on a read.
func TestAnUnknownBoardIsNotFoundAndANonsensePageIsRefused(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	if rec := read(t, s, as, boardsPath+"/wisdom"); rec.Code != http.StatusNotFound {
		t.Errorf("GET an unpublished board = %d, want 404", rec.Code)
	}
	// Including one named after a hidden factor: the wire has no such board.
	if rec := read(t, s, as, boardsPath+"/kindness"); rec.Code != http.StatusNotFound {
		t.Errorf("GET the kindness board = %d, want 404", rec.Code)
	}
	if rec := read(t, s, as, boardsPath+"/karma?page=soon"); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("GET page=soon = %d, want 422", rec.Code)
	}
}

/*
A rank that moved reaches the player over the socket (§8).

The rebuild is the clearest case in the game for the push channel after the
rollover: a rank moves because *somebody else* played, so there is no response of
the player's own that could ever carry it. Published with no origin, for the same
reason `factor.stood` is — no tab caused this.
*/
func TestARankThatMovedIsPublishedToThePlayerWithNoOrigin(t *testing.T) {
	t.Parallel()
	s, st, _ := signedIn(t)

	// The first rebuild places them, which is a movement from nowhere.
	if err := s.RefreshBoards(t.Context()); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	var (
		kinds   []string
		origins []*string
	)
	rows, err := st.Pool().Query(t.Context(),
		`SELECT type, origin FROM realtime_outbox ORDER BY seq`)
	if err != nil {
		t.Fatalf("read the outbox: %v", err)
	}
	for rows.Next() {
		var (
			kind   string
			origin *string
		)
		if err := rows.Scan(&kind, &origin); err != nil {
			t.Fatalf("read the outbox: %v", err)
		}
		kinds = append(kinds, kind)
		origins = append(origins, origin)
	}
	rows.Close()

	moved := 0
	for i, kind := range kinds {
		if kind != "board.moved" {
			continue
		}
		moved++
		if origins[i] != nil {
			t.Errorf("board.moved was published with origin %q", *origins[i])
		}
	}
	if moved == 0 {
		t.Fatal("a rebuild that placed a player told them nothing")
	}

	// A rebuild that changes nothing says nothing, which is the half that keeps
	// this from being a five-minute broadcast to everybody in the game.
	before := len(kinds)
	if err := s.RefreshBoards(t.Context()); err != nil {
		t.Fatalf("second rebuild: %v", err)
	}
	var after int
	if err := st.Pool().QueryRow(t.Context(),
		`SELECT count(*) FROM realtime_outbox`).Scan(&after); err != nil {
		t.Fatalf("count the outbox: %v", err)
	}
	if after != before {
		t.Errorf("an unchanged rebuild published %d messages", after-before)
	}

	// The message decodes into the shape the generated client type describes.
	var payload boardMoved
	if err := st.Pool().QueryRow(t.Context(),
		`SELECT payload FROM realtime_outbox WHERE type = 'board.moved' ORDER BY seq LIMIT 1`,
	).Scan(&payload); err != nil {
		t.Fatalf("read the payload: %v", err)
	}
	if len(payload.Moved) == 0 || payload.RefreshedAt == "" {
		t.Fatalf("the message says %+v", payload)
	}
	for _, m := range payload.Moved {
		if m.To < 1 || m.Of < 1 {
			t.Errorf("a movement reads %+v", m)
		}
		if !m.Board.Shown && (m.Board.Name != "" || m.Board.Reads != "") {
			t.Errorf("a sealed board was named in a movement: %+v", m.Board)
		}
	}
}

// The boards are not in the world snapshot, deliberately: §10.2 names them the
// one pull-shaped surface in this client, and putting a paginated list of other
// people into the boot hydration would make every page load pay for it.
func TestTheWorldSnapshotDoesNotCarryTheBoards(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	rec := read(t, s, as, worldPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not a world snapshot: %v", err)
	}
	if _, there := out["boards"]; there {
		t.Error("the world snapshot carries the boards — they are a separate, paginated read")
	}
}
