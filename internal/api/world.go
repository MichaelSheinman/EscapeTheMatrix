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
		// §3.14's ladder: the job held, the step up, and the five rungs whole
		// — because "the ladder itself teaches the trade" is only true of a
		// ladder somebody can see more than one rung of. The requirement chips
		// arrive already veiled (§3.5), so nothing here names a hidden factor.
		Career careerView `json:"career"`
		// The rules the player has set for their own judgement (§3.14): the
		// vocabulary they may compose from — already veiled to what they have
		// unlocked — and one context-tagged rule set per environment (§3.15).
		//
		// In the snapshot rather than fetched, because it is the player's own
		// world and because the screen that edits it has to open on what is
		// already in force rather than on a spinner.
		Playbook playbookView `json:"playbook"`
		// Progression (§3.5): the composite, and the board it is made of —
		// with the three factors the player is never shown arriving as slots
		// carrying nothing. Composed on read from the body, the balance and
		// three stored totals, and stored nowhere itself (§3.3).
		Karma karmaView `json:"karma"`
		// Peer endorsement (§3.5): whether the game has somebody for this
		// player to like right now, and who has said they like them. It is in
		// the snapshot rather than fetched like the leaderboards because it is
		// the *player's* world — one offer and a short list of names — whereas
		// a board is a page of other people (§10.2).
		//
		// Reading it is also the only thing that ever draws an offer, which is
		// why this key is not merely a projection of stored rows.
		Endorsement endorsementView `json:"endorsement"`
		// §4's backlog, as far as the player is allowed to see it: the one
		// thing standing in front of them, or nothing. The queue behind it is
		// not sent and its depth is not sent — a client that could read what
		// was coming could walk towards it, and the sandbox depends on not
		// being a quest list.
		//
		// Like the endorsement above, reading this key is one of the two things
		// that can *cause* an offer to exist (the other is walking somewhere),
		// which is why it is not merely a projection of stored rows.
		Events eventsView `json:"events"`
		// §5's challenges, as far as this player is concerned: the run they are
		// part way through, or a challenge standing where they are — never
		// both, and never a catalogue. §5.1 makes save-and-resume mandatory, so
		// this key is what puts somebody who closed the tab in the middle of a
		// conversation back into it rather than at a list.
		//
		// A read rather than a release, unlike the two above it: an entry point
		// stands somewhere until it is used, so looking cannot cause one.
		Challenge challengeView `json:"challenge"`
		// §3.14's delegated half of a job: the one moment an active role
		// produced that the player's own rules could not settle, waiting for
		// them to settle it by hand. Never more than one — a role that is
		// waiting produces nothing new, which is what makes "good rules buy
		// time" a mechanic rather than a slogan.
		//
		// A read rather than a release: a situation is produced by a day of the
		// rollover passing and by nothing else, so looking cannot cause one.
		Situations situationsView `json:"situations"`
		// §3.15's engine: the people the player lives among, what a fortnight
		// of living beside them has come to, and the one piece of friction
		// their own rules could not settle. The population is authored content
		// and the counts are composed on read, so this key is a projection of
		// two tables and a directory rather than anything stored.
		//
		// A read rather than a release, like the situations above it: a
		// conflict is produced by a day of the rollover passing and by nothing
		// else, so looking cannot cause one.
		Conflicts conflictsView `json:"conflicts"`
		// §3.15's relocation: where the player lives, everywhere else there is
		// to live, and — once the decay has said it first — what somebody here
		// has heard about one of them. What is deliberately not in it is
		// anything about the people anywhere else: arriving is the moment the
		// old answers fail, not the moment the new ones are handed over.
		Living livingView `json:"living"`
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

		// Before the forecast, because a job is part of the arc: the fortnight
		// ahead of somebody on nights is not the fortnight their routines
		// describe (§3.14).
		among, err := s.among(r.Context(), tx, actor.ID)
		if err != nil {
			return err
		}
		if out.Career, err = s.careerSurface(r.Context(), tx, actor.ID, where(c), among); err != nil {
			return err
		}
		held, err := s.careers.Of(r.Context(), tx, actor.ID)
		if err != nil {
			return err
		}
		out.Forecast = forecast(p, set, held.Post(), today)

		book, err := s.playbooks.Of(r.Context(), tx, actor.ID)
		if err != nil {
			return err
		}
		out.Playbook = playbookOf(book)

		standing, err := s.standing(r.Context(), tx, actor.ID, today)
		if err != nil {
			return err
		}
		out.Karma = karma(standing)

		// Composed from the two reads above rather than from a table: where the
		// player lives is the character's, and whether anybody has mentioned
		// anywhere else is a point on the standing's decay curve (§3.15).
		out.Living = living(c, standing, s.clk)

		// The other read that can hand the player something new: a queued event
		// (§4) surfaced where they are standing. It is after the character
		// above because it is placed at the place that read established.
		out.Events, err = s.lookedForEvents(r.Context(), tx, actor.ID, where(c))
		if err != nil {
			return err
		}

		// §5's surface, standing where the character above is. After the events
		// because a run that has just been opened by an accepted event is the
		// run this has to find, and before the endorsement for no reason but
		// that the order of two reads that cannot affect each other has to be
		// some order.
		out.Challenge, err = s.challengeSurface(r.Context(), tx, actor.ID, where(c))
		if err != nil {
			return err
		}

		// §3.14's delegated work. After the events for no reason but that the
		// two cannot affect each other; the rollover above is what produced
		// anything this finds.
		out.Situations, err = s.situationSurface(r.Context(), tx, actor.ID)
		if err != nil {
			return err
		}

		// And §3.15's other half of the same day. After the situations for no
		// reason but that the two cannot affect each other; the rollover above
		// is what produced anything either of them finds.
		out.Conflicts, err = s.conflictSurface(r.Context(), tx, actor.ID, today)
		if err != nil {
			return err
		}

		// The one read in this snapshot that can hand the player something new:
		// if the cadence has come round, this is where the game names somebody
		// for them (§3.5).
		out.Endorsement, err = s.looked(r.Context(), tx, actor.ID)
		if err != nil {
			return err
		}

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
