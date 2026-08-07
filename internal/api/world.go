package api

import (
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

// worldPath is the read side of the protocol.
//
// technical.md §10.2: REST hydrates the client's world model at boot, and the
// socket patches it from there. Both halves are needed for either to be worth
// anything — a store that only ever learns from patches starts empty and stays
// wrong until something happens, and the gap handling in §8 (a resume point
// older than the outbox still holds) is answered by rehydrating, which requires
// something to rehydrate from.
const worldPath = "/api/world"

// worldSnapshot answers "what is true right now".
//
// It is a plain GET rather than a command because it writes nothing. Commands
// are POSTs that change the world and carry an idempotency key for the retry
// that duplicates them (§8); a read has nothing to make idempotent and no state
// to conflict with, so putting it behind that route would only mean inventing a
// key for it. It sits behind the same auth middleware, because what it answers
// is per-player and there is no anonymous mode to answer for (§9).
//
// The body is shaped like the client's world model rather than like the rows
// underneath it, and every field it carries is one the socket's patches later
// move. That is the property worth protecting: one shape per piece of state,
// hydrated by this route and patched by a message, so the store never needs a
// reducer per transport. Later domain modules (§3.1) add their own key here.
func (s *Server) worldSnapshot(w http.ResponseWriter, r *http.Request) {
	type body struct {
		// The player, as internal/character holds them.
		Character characterView `json:"character"`
		// The body, as internal/health holds it — the whole panel, with the
		// metrics this player has no instrument for sent as themselves and
		// without a reading (§3.16).
		Health panelView `json:"health"`
		// The rules the player has set for their body (§3.4), and what those
		// rules would do to it over the next fortnight. The forecast is
		// composed on read from the panel above and never stored (§3.3).
		Routines routinesView `json:"routines"`
		Forecast forecastView `json:"forecast"`
		// Progression (§3.5): the composite, and the board it is made of —
		// with the three factors the player is never shown arriving as slots
		// carrying nothing. Composed on read from the body, the balance and
		// three stored totals, and stored nowhere itself (§3.3).
		Karma karmaView `json:"karma"`
		// The city: every place, and every way between them (§3.12). Authored
		// vocabulary sent rather than compiled into the client — the city and
		// its district list are still open (§17), and a client holding its own
		// copy shows yesterday's city after a deploy. The ways travel with the
		// places because a portal drawn in a painting is only the *placement*
		// of a way (§10.3), and most of the city is not painted yet (#35).
		Places []placeView `json:"places"`

		Probes int64 `json:"probes"`
		// The two the probe command also reports. They belong here because a
		// client that can only learn them by acting has to hold a copy of a
		// command's response to keep showing them, and that copy is a second
		// home for state the store already exists to be.
		Database string `json:"database"`
		Schema   int64  `json:"schema"`
	}

	// Per-player and momentary. A shared cache holding it would hand one
	// player's world to the next, and even a private cache would serve a
	// snapshot the socket has since patched past.
	w.Header().Set("Cache-Control", "no-store")

	actor, ok := actorFrom(r.Context())
	if !ok {
		// Unreachable behind the middleware, and worth saying out loud rather
		// than answering with somebody else's world.
		writeJSON(w, http.StatusInternalServerError, errorBody("the world could not be read"))
		return
	}

	// Fixed at boot: migrations run before the server starts and never again in
	// the life of the process.
	out := body{Schema: s.schemaVersion, Places: places()}

	// §3.3's catch-up. A page load is the most reliable signal there is that a
	// player is here, and a player who has been away owes days — so the world
	// is advanced *before* it is read, and the snapshot below describes a body
	// that is up to date rather than one from whenever they last looked.
	//
	// It is also what re-arms their schedule: a dormant account holds nothing
	// in the queue, and this is the moment it stops being dormant.
	s.wake(r.Context(), actor.ID)

	// One transaction for the whole snapshot, so the client cannot be handed a
	// character from one instant and a count from another. It also gives the
	// character module somewhere to create a row for an account that has never
	// had one — a read that may write, which is why it is a transaction rather
	// than a bare query.
	err := s.store.InTx(r.Context(), func(tx pgx.Tx) error {
		c, err := s.characters.Ensure(r.Context(), tx, actor.ID)
		if err != nil {
			return err
		}
		out.Character = s.view(c)

		p, err := s.health.Ensure(r.Context(), tx, actor.ID)
		if err != nil {
			return err
		}
		today := simulation.Today(s.clk, c.Zone)
		out.Health = panel(p, today)

		set, err := s.routines.Of(r.Context(), tx, actor.ID)
		if err != nil {
			return err
		}
		out.Routines = routines(set)
		out.Forecast = forecast(p, set, today)

		standing, err := s.factors.Of(r.Context(), tx, actor.ID)
		if err != nil {
			return err
		}
		out.Karma = karma(standing)

		return tx.QueryRow(r.Context(),
			`SELECT count(*), current_setting('server_version') FROM skeleton_probe`,
		).Scan(&out.Probes, &out.Database)
	})
	if err != nil {
		s.log.Error("read world snapshot", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorBody("the world could not be read"))
		return
	}

	writeJSON(w, http.StatusOK, out)
}

// places is the city, in the order it is authored.
func places() []placeView {
	all := city.Places()
	out := make([]placeView, 0, len(all))
	for _, p := range all {
		out = append(out, place(p))
	}
	return out
}
