package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/account"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/factor"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
)

/*
The leaderboards on the wire (§3.5): one overall board plus one per factor,
including the hidden ones, because a ranking discloses nothing about the formula.
A sealed board's key names a position rather than a factor, its rows carry a null
value, and no struct below has a field a hidden reading could occupy.

GETs rather than commands, because they write nothing and a read has nothing to
make idempotent (§8). A leaderboard is the one pull-shaped surface in this client
(§10.2): a read-only paginated list, fetched on demand.

The push half is here too — a rank moves because somebody else played, which is
exactly what a player cannot learn from their own command's response (§8).
*/

// boardsPath is the index: every board, and where the reader stands on each.
const boardsPath = "/api/boards"

// boardPath is one board, one page.
const boardPath = "/api/boards/{key}"

// boardView is one board's identity. A sealed board carries a positional key, no
// name, and a note that deliberately does not explain what it measures.
type boardView struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Note  string `json:"note"`
	Reads string `json:"reads"`
	Shown bool   `json:"shown"`
}

// placingView is where one player stands on one board.
type placingView struct {
	Board boardView `json:"board"`
	// Rank is 0 for somebody the last rebuild did not place.
	Rank int `json:"rank"`
	Of   int `json:"of"`
	// Null on a sealed board, and on a board this player is not ranked on.
	Value *float64 `json:"value"`
}

// rowView is one rung.
type rowView struct {
	Rank   int    `json:"rank"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
	// Null on a sealed board. There is nothing else on this struct a hidden
	// factor's reading could arrive in.
	Value *float64 `json:"value"`
	You   bool     `json:"you"`
}

// indexView is the boards screen's first read. refreshedAt is empty until the
// job has ever run, and is sent because a leaderboard is a snapshot and a
// snapshot should say when it was taken.
type indexView struct {
	RefreshedAt string        `json:"refreshedAt"`
	Players     int           `json:"players"`
	Boards      []placingView `json:"boards"`
}

// pageView is one page of one board.
type pageView struct {
	Board boardView `json:"board"`
	Rows  []rowView `json:"rows"`
	// Page is the page actually served, which may not be the one asked for: a
	// page past the end of a board that shrank is answered with the last one.
	Page  int `json:"page"`
	Pages int `json:"pages"`
	Total int `json:"total"`
	Size  int `json:"size"`
	// You is the reader's own rung wherever it is, so they never have to page
	// to find themselves. Null when the rebuild has not placed them.
	You         *placingView `json:"you"`
	RefreshedAt string       `json:"refreshedAt"`
}

func boardOf(b factor.Board) boardView {
	return boardView{Key: b.Key, Name: b.Name, Note: b.Note, Reads: b.Reads, Shown: b.Shown}
}

func placing(p factor.Placing) placingView {
	return placingView{Board: boardOf(p.Board), Rank: p.Rank, Of: p.Of, Value: p.Value}
}

// stamp renders the moment a snapshot was taken, or the empty string before
// there has ever been one. Empty rather than a zero time, so "never" does not
// render as the first instant of the year one.
func stamp(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.UTC().Format(time.RFC3339)
}

// boardIndex answers "what boards are there, and where do I stand on each".
// Every board appears, including ones nobody is ranked on yet: a board existing
// is part of what §3.5 shows the player.
func (s *Server) boardIndex(w http.ResponseWriter, r *http.Request) {
	// The rebuild decides how stale this is, and a cache in front of it could
	// only make it staler without saying so.
	w.Header().Set("Cache-Control", "no-store")

	actor, ok := actorFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusInternalServerError, errorBody("the boards could not be read"))
		return
	}

	var index factor.Index
	if err := s.store.InTx(r.Context(), func(tx pgx.Tx) error {
		var err error
		index, err = s.factors.Standings(r.Context(), tx, actor.ID)
		return err
	}); err != nil {
		s.log.Error("read board index", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorBody("the boards could not be read"))
		return
	}

	out := indexView{
		RefreshedAt: stamp(index.RefreshedAt),
		Players:     index.Players,
		Boards:      make([]placingView, 0, len(index.Boards)),
	}
	for _, p := range index.Boards {
		out.Boards = append(out.Boards, placing(p))
	}
	writeJSON(w, http.StatusOK, out)
}

// boardPage answers one page of one board. The page size is the server's, so no
// query parameter asks for the whole population. A page past the end is clamped
// rather than refused, and the answer says which page it got.
func (s *Server) boardPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	actor, ok := actorFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusInternalServerError, errorBody("the board could not be read"))
		return
	}

	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			// 422 for a value that is not a number at all (§8: validation), and
			// not for a number that is merely out of range — that one is clamped.
			writeJSON(w, http.StatusUnprocessableEntity, errorBody("page must be a number"))
			return
		}
		page = n
	}

	var read factor.Page
	err := s.store.InTx(r.Context(), func(tx pgx.Tx) error {
		var err error
		read, err = s.factors.Page(r.Context(), tx, actor.ID, r.PathValue("key"), page)
		return err
	})
	switch {
	case errors.Is(err, factor.ErrNoBoard):
		writeJSON(w, http.StatusNotFound, errorBody("there is no board by that name"))
		return
	case err != nil:
		s.log.Error("read board", "board", r.PathValue("key"), "error", err)
		writeJSON(w, http.StatusInternalServerError, errorBody("the board could not be read"))
		return
	}

	out := pageView{
		Board:       boardOf(read.Board),
		Rows:        make([]rowView, 0, len(read.Rows)),
		Page:        read.Page,
		Pages:       read.Pages,
		Total:       read.Total,
		Size:        factor.PageSize,
		RefreshedAt: stamp(read.RefreshedAt),
	}
	for _, row := range read.Rows {
		out.Rows = append(out.Rows, rowView{
			Rank: row.Rank,
			// No account id: nothing on this screen addresses another player,
			// and endorsement needs a handle the server can check rather than
			// one this list handed out.
			Name:   row.Player.Name,
			Avatar: row.Player.Avatar,
			Value:  row.Value,
			You:    row.You,
		})
	}
	if read.You != nil {
		you := placing(*read.You)
		out.You = &you
	}
	writeJSON(w, http.StatusOK, out)
}

// boardsMoved is what a player's tabs are told when a rebuild moved them: only
// the boards they moved on, and only if they moved on one.
type boardMoved struct {
	RefreshedAt string         `json:"refreshedAt"`
	Players     int            `json:"players"`
	Moved       []movementView `json:"moved"`
}

type movementView struct {
	Board boardView `json:"board"`
	// From is 0 when they were not on the board before: a first placing is a
	// movement.
	From int `json:"from"`
	To   int `json:"to"`
	Of   int `json:"of"`
}

var boardMovedMessage = realtime.Define[boardMoved]("board.moved")

/*
RefreshBoards rebuilds every board and tells whoever moved — the entry point the
background job calls (§3.4), and the only thing that writes the board tables.

One transaction for the sort, the replacement and the messages, so a rank a
player was told about is a rank in the table and a rebuild that failed halfway
has announced nothing.

It is not on the request path: recomposing per page load would sort the whole
population to answer one question.
*/
func (s *Server) RefreshBoards(ctx context.Context) error {
	var done factor.Rebuilt
	err := s.store.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		done, err = s.factors.Rebuild(ctx, tx, s.clk.Now())
		if err != nil {
			return err
		}

		at := stamp(done.At)
		for _, move := range done.Moves {
			out := boardMoved{
				RefreshedAt: at,
				Players:     done.Players,
				Moved:       make([]movementView, 0, len(move.Boards)),
			}
			for _, m := range move.Boards {
				out.Moved = append(out.Moved, movementView{
					Board: boardOf(m.Board), From: m.From, To: m.To, Of: m.Of,
				})
			}
			// No Origin: nobody's command caused this. A rank moves because
			// somebody else played, which is the clearest case in the game for
			// a message no response could have carried.
			if err := boardMovedMessage.Publish(ctx, tx,
				realtime.To{Account: move.Account}, out); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.log.Info("boards rebuilt", "players", done.Players, "moved", len(done.Moves))
	return nil
}

/*
roster answers factor.Roster: who is playing, and who they are on a board.

The adapter §3.1 asks for, in the layer that is allowed to compose two modules.
internal/factor cannot read the account table and internal/account has never
heard of a factor; this is the one place that knows both exist, and it is three
lines because the narrow type on either side is doing the work.

Note what it drops. account.Profile is already name-and-picture rather than an
Account, so there is no email on this path to forget to strip.
*/
type roster struct{ accounts *account.Accounts }

func (r roster) Everyone(ctx context.Context, tx pgx.Tx) ([]string, error) {
	return r.accounts.Everyone(ctx, tx)
}

func (r roster) Identify(ctx context.Context, tx pgx.Tx, ids []string) ([]factor.Player, error) {
	profiles, err := r.accounts.Profiles(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]factor.Player, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, factor.Player{ID: p.ID, Name: p.Name, Avatar: p.AvatarURL})
	}
	return out, nil
}
