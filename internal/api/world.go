package api

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

// worldPath is the read side of the protocol: REST hydrates the client's world
// model at boot and the socket patches it from there (§10.2). Rehydrating is
// also how §8's gap handling recovers when a resume point is older than the
// outbox still holds.
const worldPath = "/api/world"

// zoneHeader carries the IANA timezone the browser reports, on the snapshot
// request. It rides the read rather than being a command because it is not a
// decision the player makes — it is a fact about their device, learned the
// same way the character itself is ensured: as part of arriving.
const zoneHeader = "Etm-Timezone"

// worldSnapshot answers "what is true right now". A plain GET rather than a
// command, because it writes nothing and has nothing to make idempotent (§8),
// behind the same auth middleware because what it answers is per-player (§9).
//
// The body is shaped like the client's world model rather than like the rows
// underneath, and every field is one the socket's patches later move: one shape
// per piece of state, hydrated here and patched by a message, so the store never
// needs a reducer per transport.
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
		// §3.14's jobs: the one held and everything on the board. The
		// requirement chips arrive with the hidden factors already stripped
		// (§3.5), so nothing here names one.
		Career careerView `json:"career"`
		// The rules the player has set for their own judgement (§3.14): the
		// vocabulary they may compose from, already cut down to what they have
		// unlocked, and one rule set per environment (§3.15). In the snapshot
		// rather than fetched, so the screen that edits it opens on what is in
		// force rather than on a spinner.
		Playbook playbookView `json:"playbook"`
		// Progression (§3.5): the composite, and the board it is made of —
		// with the three factors the player is never shown arriving as slots
		// carrying nothing. Composed on read from the body, the balance and
		// three stored totals, and stored nowhere itself (§3.3).
		Karma karmaView `json:"karma"`
		// Peer endorsement (§3.5): whether the game has somebody for this player
		// to like right now, and who has said they like them. In the snapshot
		// rather than fetched, because one offer and a short list of names is
		// the player's own world while a board is a page of other people
		// (§10.2). Reading it is also the only thing that draws an offer.
		Endorsement endorsementView `json:"endorsement"`
		// §4's backlog, as far as the player may see it: the one thing standing
		// in front of them, or nothing. Neither the queue nor its depth is sent
		// — a client that could read what was coming could walk towards it.
		// Reading this key is one of the two things that can cause an offer to
		// exist; the other is walking somewhere.
		Events eventsView `json:"events"`
		// §5's challenges: the run this player is part way through, or a
		// challenge standing where they are — never both, and never a
		// catalogue. §5.1 makes save-and-resume mandatory, so this is what puts
		// somebody who closed the tab back into the conversation. A read rather
		// than a release: an entry point stands somewhere until it is used.
		Challenge challengeView `json:"challenge"`
		// §3.14's delegated half of a job: the one moment an active role
		// produced that the player's rules could not settle. Never more than
		// one, because a role that is waiting produces nothing new. A read
		// rather than a release — only a rollover day produces a situation.
		Situations situationsView `json:"situations"`
		// §3.15's engine: the people the player lives among, what a fortnight
		// beside them has come to, and the one piece of friction their rules
		// could not settle. A projection of two tables and a directory — the
		// population is authored content and the counts are composed on read.
		Conflicts conflictsView `json:"conflicts"`
		// §3.15's relocation: where the player lives, everywhere else there is
		// to live, and — once the decay has said it first — what somebody here
		// has heard about one of them. Nothing about the people anywhere else:
		// arriving is the moment the old answers fail, not the moment the new
		// ones are handed over.
		Living livingView `json:"living"`
		// The city: every place, and every way between them (§3.12). Sent
		// rather than compiled into the client, which would show yesterday's
		// city after a deploy. The ways travel with the places because a portal
		// drawn in a painting is only the placement of a way (§10.3).
		Places []placeView `json:"places"`

		Probes int64 `json:"probes"`
		// The two the probe command also reports. Here as well, so a client does
		// not have to keep a copy of a command's response to go on showing
		// them.
		Database string `json:"database"`
		Schema   int64  `json:"schema"`
	}

	// Per-player and momentary: a shared cache would hand one player's world to
	// the next, and a private one would serve a snapshot the socket has since
	// patched past.
	w.Header().Set("Cache-Control", "no-store")

	actor, ok := actorFrom(r.Context())
	if !ok {
		// Unreachable behind the middleware, and better said out loud than
		// answered with somebody else's world.
		writeJSON(w, http.StatusInternalServerError, errorBody("the world could not be read"))
		return
	}

	// Fixed at boot: migrations run before the server starts.
	out := body{Schema: s.schemaVersion, Places: places()}

	// The player's timezone, as their browser reports it (§3.3). Sign-in is a
	// redirect and cannot carry it, so this header is where the real zone first
	// appears — recorded before the catch-up below, so a returning player's days
	// are counted at their own midnight. An unknown zone name is ignored rather
	// than 4xx'd: the stored zone keeps working.
	if zone := r.Header.Get(zoneHeader); zone != "" && len(zone) <= 64 {
		if _, err := time.LoadLocation(zone); err == nil {
			if err := s.store.InTx(r.Context(), func(tx pgx.Tx) error {
				return s.characters.SetZone(r.Context(), tx, actor.ID, zone)
			}); err != nil {
				s.log.Error("record timezone", "account", actor.ID, "error", err)
			}
		}
	}

	// §3.3's catch-up, before the read, so the snapshot below describes a body
	// that is up to date. It is also what re-arms the schedule: a dormant
	// account holds nothing in the queue, and this is where that ends.
	s.wake(r.Context(), actor.ID)

	// One transaction for the whole snapshot, so the client cannot be handed a
	// character from one instant and a count from another — and so the character
	// module has somewhere to create a row for an account that has never had
	// one.
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

		// Before the forecast: the fortnight ahead of somebody on nights is not
		// the fortnight their routines describe (§3.14).
		among, err := s.among(r.Context(), tx, actor.ID)
		if err != nil {
			return err
		}
		if out.Career, err = s.careerSurface(r.Context(), tx, actor.ID, where(c), among, today); err != nil {
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
		// anywhere else is a point on the decay curve (§3.15).
		out.Living = living(c, standing, s.clk)

		// The other read that can hand the player something new: a queued event
		// (§4) surfaced where they are standing, which is why it comes after the
		// character read that established the place.
		out.Events, err = s.lookedForEvents(r.Context(), tx, actor.ID, where(c))
		if err != nil {
			return err
		}

		// §5's surface, standing where the character above is. After the events,
		// because a run just opened by an accepted event is the run this has to
		// find.
		out.Challenge, err = s.challengeSurface(r.Context(), tx, actor.ID, where(c), today)
		if err != nil {
			return err
		}

		// §3.14's delegated work. The rollover above is what produced anything
		// this finds.
		out.Situations, err = s.situationSurface(r.Context(), tx, actor.ID)
		if err != nil {
			return err
		}

		// And §3.15's other half of the same day, from the same rollover.
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
