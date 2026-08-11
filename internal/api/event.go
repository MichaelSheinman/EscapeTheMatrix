package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/account"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/character"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/event"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/factor"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
The event backlog on the wire (§4).

Two properties of this file are the ticket rather than the plumbing.

**Nothing here creates an offer.** There is no `event.ask` command and no way for
a client to request one. Release is a server decision (§4), and the only two
things that ever surface an event are a walk arriving somewhere and a world
snapshot being read — both of them things the player did for another reason.

**Nothing the player is not standing in front of crosses the wire.** The backlog
is not sent, its depth is not sent, and neither is the catalogue. A client that
knew what was queued could route towards it, which would turn §4's sandbox into
a quest list — and the only thing on this surface is the one moment that is
actually in front of them.
*/

// eventView is one event, put to one player, in one spot.
//
// `line`, `accept` and `decline` are authored content sent as text rather than
// as keys the client looks up: the catalogue is content in the repo (§5.2) and
// a client holding its own copy would show yesterday's writing after a deploy —
// the same argument the city is sent under.
type eventView struct {
	// ID is what an answer names. Meaningless to anybody else: every read of an
	// offer is keyed by (id, account).
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// Kind is the family (§4's decline signal is counted by it). Sent because a
	// screen groups by it, and because it is the honest label for what sort of
	// thing this is before the player reads a word.
	Kind string `json:"kind"`
	Line string `json:"line"`
	// Accept and Decline are the two answers, in the content's own words.
	Accept  string `json:"accept"`
	Decline string `json:"decline"`
	// Because is why this reached you, for an escalation: which moment it was,
	// who it was with, and which of your own rules fired in it. Empty for
	// every ordinary event. Composed when the escalation was raised
	// (internal/api/rollover.go) rather than resolved from content here — it
	// is a statement about a moment that has already happened.
	Because string `json:"because"`

	// Place and Zone are where in the world it is, by key. The zone is what a
	// painting draws the marker in (§10.3) — a client whose painting has no
	// such trigger simply does not pin it, which is the same tolerance the
	// unpainted places already have.
	Place string `json:"place"`
	Zone  string `json:"zone"`
	// Glyph is what the map draws where this is, by name, and is empty for the
	// ordinary event a client pins however it pins events.
	//
	// A name and never a picture. §10.3 keeps the drawing on the client side, so
	// what crosses here is which of the shapes it already knows to use — a
	// server that sent a path would be art direction shipped through an API.
	Glyph string `json:"glyph,omitempty"`
	// Until is when it lapses, so a screen can say so rather than having the
	// buttons quietly stop working.
	Until string `json:"until"`
}

/*
eventsView is the whole event surface for one player.

One nullable field. §4's backlog depth is deliberately not here: a number saying
how many things are queued would turn "there is always something to do" into a
counter to grind down, which is the exact reading of the sandbox this design
argues against.
*/
type eventsView struct {
	Offer *eventView `json:"offer"`
}

func events(s event.Standing) eventsView {
	if s.Offer == nil {
		return eventsView{}
	}
	return eventsView{Offer: &eventView{
		ID:      s.Offer.ID,
		Name:    s.Offer.Event.Name,
		Kind:    string(s.Offer.Event.Kind),
		Line:    s.Offer.Event.Line,
		Accept:  s.Offer.Event.Accept,
		Decline: s.Offer.Event.Decline,
		Because: s.Offer.Because,
		Place:   s.Offer.Place,
		Zone:    s.Offer.Zone,
		Glyph:   s.Offer.Event.Glyph,
		Until:   stamp(s.Offer.ExpiresAt),
	}}
}

/*
What a player's tabs are told when the world puts something in front of them.

Published only when something is actually released, which is at most a few times
a day (event.Attentive) — not on every step of every journey. A message per walk
would be the polling §8 exists instead of.

No Origin, even though the player's own walk is usually what triggered it: the
release was the world's decision rather than the command's, and the acting tab
has to hear about it like every other tab. §8's rule 4 is about echoes of your
own action, and an event finding you is not one.
*/
type eventOffered struct {
	Events eventsView `json:"events"`
}

var eventOfferedMessage = realtime.Define[eventOffered]("event.offered")

// What a player's *other* tabs are told when this one answered.
//
// `after` is what accepting came to, in the content's own words, and is empty
// for a decline and for an event that opened something that finishes later.
type eventAnswered struct {
	Events eventsView `json:"events"`
	// Answer is "accepted" or "declined".
	Answer string `json:"answer"`
	Name   string `json:"name"`
	After  string `json:"after"`
}

var eventAnsweredMessage = realtime.Define[eventAnswered]("event.answered")

// answerEventInput is the whole of what either command takes: which offer.
//
// There is no event key here, and there could not be: a client that named an
// event would be a client choosing which one it wanted, and release is the
// server's decision (§4).
type answerEventInput struct {
	Event int64 `json:"event"`
}

type eventResult struct {
	Events eventsView `json:"events"`
	// After is what accepting came to. Empty for a decline, and empty for an
	// event whose outcome is somebody else's to decide (#23, #28, #30).
	After string `json:"after"`
	// Open is whether the event is still running rather than finished.
	Open bool `json:"open"`
	// Challenge is the run this acceptance opened, if the event was a doorway
	// into one (§4, §5). Null otherwise, which is nearly always: most events
	// finish on the spot and the rest wait on modules that are not this one.
	//
	// It is here so that saying yes and being in the thing you said yes to are
	// one round trip. The alternative is a state in which a door has been opened
	// and not gone through, which is a state somebody would have to draw.
	Challenge *challengeView `json:"challenge"`
}

/*
Taking up what the world put in front of you.

Refusals are 409 rather than 422, which is the honest reading of §8: the request
is well formed and it is the world that is in the way. An offer that lapsed while
the screen was open and one another tab already answered are both things that
were true a moment ago.
*/
func (s *Server) acceptEvent(ctx context.Context, tx pgx.Tx, actor command.Actor, in answerEventInput) (eventResult, error) {
	if in.Event <= 0 {
		// The one refusal that is permanent: there is no such thing as event
		// zero, however long the client waits.
		return eventResult{}, command.Invalid("an answer names the event it answers")
	}

	answered, err := s.events.Accept(ctx, tx, actor.ID, in.Event)
	switch {
	case errors.Is(err, event.ErrNoOffer):
		return eventResult{}, command.Conflict("that is no longer in front of you")
	case err != nil:
		return eventResult{}, err
	}

	out := eventResult{Events: events(answered.Standing), After: answered.Event.After, Open: answered.Open}
	if answered.Open {
		// It opened something this module does not run (#23, #28, #30), so
		// there is nothing to say it came to yet — and saying anything would be
		// writing the outcome in advance.
		out.After = ""

		// §4's doorway, walked through in the same transaction as the
		// acceptance: if a challenge is authored behind this event, its run
		// begins now and the response carries the first scene of it.
		if out.Challenge, err = s.opened(ctx, tx, actor, answered, in.Event); err != nil {
			return eventResult{}, err
		}
	}
	return out, s.answered(ctx, tx, actor, answered, out)
}

/*
Letting it go.

The one thing this must not do is consume it (§3.9, §4). internal/event puts the
row back in the backlog; what this file adds is that the *player* is not told it
is gone, because it is not — the response simply carries a surface with nothing
on it, exactly as walking away does.
*/
func (s *Server) declineEvent(ctx context.Context, tx pgx.Tx, actor command.Actor, in answerEventInput) (eventResult, error) {
	if in.Event <= 0 {
		return eventResult{}, command.Invalid("an answer names the event it answers")
	}

	answered, err := s.events.Decline(ctx, tx, actor.ID, in.Event)
	switch {
	case errors.Is(err, event.ErrNoOffer):
		return eventResult{}, command.Conflict("that is no longer in front of you")
	case err != nil:
		return eventResult{}, err
	}

	out := eventResult{Events: events(answered.Standing)}
	return out, s.answered(ctx, tx, actor, answered, out)
}

// answered tells this player's other tabs what was decided.
func (s *Server) answered(ctx context.Context, tx pgx.Tx, actor command.Actor, a event.Answered, out eventResult) error {
	return eventAnsweredMessage.Publish(ctx, tx,
		realtime.To{Account: actor.ID, Origin: actor.Session},
		eventAnswered{
			Events: out.Events,
			Answer: string(a.Answer),
			Name:   a.Event.Name,
			After:  out.After,
		},
	)
}

/*
looked is the world snapshot's half: read the surface, and surface a queued
event if the world has one for where the player is standing.

A read that may write, like the endorsement offer beside it. It is one of the
two triggers there is — this and a walk — and there is no job handing events out
to people who are not playing, which is what keeps §4's "release is a server
decision" from quietly becoming a timer.
*/
func (s *Server) lookedForEvents(ctx context.Context, tx pgx.Tx, accountID, place string) (eventsView, error) {
	standing, released, err := s.events.Looked(ctx, tx, accountID, place)
	if err != nil {
		return eventsView{}, err
	}

	out := events(standing)
	if released {
		if err := eventOfferedMessage.Publish(ctx, tx,
			realtime.To{Account: accountID}, eventOffered{Events: out}); err != nil {
			return eventsView{}, err
		}
	}
	return out, nil
}

/* ── the record ───────────────────────────────────────────────────────── */

// eventLogPath is what has happened to this player, newest first.
const eventLogPath = "/api/events/log"

/*
eventRecordView is one thing that happened, and what came of it.

Until 2026-08-08 this was a title, a place and a verb — "A knock at the door ·
east side apartment · you walked on" — which is a row about a moment rather than
the moment. A player reading their own week could not tell who knocked, what
they wanted, or how it turned out, and the writing that answers all three was
sitting in `content/events` being shown once and then thrown away.

So the record carries the writing now. `line` is the moment as it was put to
them, `did` is the choice in its own words rather than as a state, and `came` is
what accepting came to. All three are resolved from the catalogue at read time
rather than copied into the row: a record is what happened to this player, the
words are content, and content that was edited should read back edited.

They are empty for an event whose content has since been withdrawn. The title
and the place are stored on the row and survive it, so a withdrawn event still
reads back as something that happened rather than vanishing — a log that quietly
lost a moment would be a smaller lie than a wrong one but a lie all the same.
*/
type eventRecordView struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
	At   string `json:"at"`
	// Line is the moment as it was put to them, in the second person.
	Line string `json:"line"`
	// Did is the choice they made in the content's own words — "Open the door",
	// "Stand very still" — and empty for the three answers that were never a
	// choice. A player who walked past something did not pick anything, and
	// putting the accept wording on that row would invent a decision.
	Did string `json:"did"`
	// Came is what accepting came to, for an event that finished on the spot.
	// Empty for one that opened something finishing elsewhere, and for every
	// event nobody accepted.
	Came string `json:"came"`
	// Because is why an escalation reached them — see [eventView.Because]. It
	// is stored on the record rather than resolved, so a week-old escalation
	// still reads back as the moment it was actually about.
	Because string `json:"because"`
	// Place and Zone as they were, by key.
	Place string `json:"place"`
	Zone  string `json:"zone"`
	// Answer is one of accepted | declined | left | lapsed | dropped. The last
	// three are all "not answered" and are deliberately different words: one is
	// walking away, one is not looking, and one is the world giving up.
	Answer string `json:"answer"`
	// Resolved is when an accepted event finished, and empty while it is still
	// running.
	Resolved string `json:"resolved"`
}

type eventLogView struct {
	Records []eventRecordView `json:"records"`
	Page    int               `json:"page"`
	Pages   int               `json:"pages"`
	Total   int               `json:"total"`
	Size    int               `json:"size"`
}

/*
The history, a page at a time.

A GET rather than a command for the same two reasons the boards and the
playbook's log are: it writes nothing, and it is history — unbounded, so it is
fetched when somebody looks rather than carried in the world snapshot (§10.2).

What it does *not* carry is anything about what is still queued. A record is
something that happened; the backlog is what might, and a client that could read
it would be able to walk towards it.
*/
func (s *Server) eventLog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	actor, ok := actorFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusInternalServerError, errorBody("the record could not be read"))
		return
	}

	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, errorBody("page must be a number"))
			return
		}
		page = n
	}

	var read event.Log
	if err := s.store.InTx(r.Context(), func(tx pgx.Tx) error {
		var err error
		read, err = s.events.Records(r.Context(), tx, actor.ID, page)
		return err
	}); err != nil {
		s.log.Error("read event record", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorBody("the record could not be read"))
		return
	}

	out := eventLogView{
		Records: make([]eventRecordView, 0, len(read.Records)),
		Page:    read.Page,
		Pages:   read.Pages,
		Total:   read.Total,
		Size:    event.LogPage,
	}
	for _, record := range read.Records {
		row := eventRecordView{
			ID:       record.ID,
			Name:     record.Name,
			Kind:     string(record.Kind),
			At:       stamp(record.At),
			Place:    record.Place,
			Zone:     record.Zone,
			Answer:   string(record.Answer),
			Resolved: stamp(record.Resolved),
			Because:  record.Because,
		}
		if authored, written := event.Known(record.Key); written {
			row.Line = authored.Line
			switch record.Answer {
			case event.Accepted:
				row.Did, row.Came = authored.Accept, authored.After
			case event.Declined:
				// Declining is a decision and the other three are not, so it is
				// the only one of the four that has words. `after` belongs to
				// accepting and is deliberately not shown here: nothing came of
				// walking away except the event going back into the backlog.
				row.Did = authored.Decline
			}
		}
		out.Records = append(out.Records, row)
	}
	writeJSON(w, http.StatusOK, out)
}

/* ── the bindings ─────────────────────────────────────────────────────── */

/*
encounters answers character.Encounters: what a walk offers the rest of the game.

§3.12's "walking is the encounter surface, not a wait" — and #12 left this as a
stub bound to character.Quiet precisely so that the ticket which built the
backlog would replace one argument rather than edit the character module. This
is that argument.

Only the *arrival* of each step is offered, and the release path is what decides
whether anything comes of it. Note what is not here: no decision about what
happens, no factor moved, no money. A walk says where somebody now is, and
internal/event says whether the world had anything there.
*/
type encounters struct {
	events *event.Events
	// announce publishes what was released. It is the Server, and it is here
	// rather than in internal/event for §3.1's reason: message shapes live
	// beside the views they carry, and the event module has no idea a socket
	// exists.
	announce func(ctx context.Context, tx pgx.Tx, accountID string, offer *event.Offer) error
}

func (e encounters) Walked(ctx context.Context, tx pgx.Tx, accountID string, step character.Step) error {
	offer, err := e.events.Arrived(ctx, tx, accountID, step.To.Key)
	if err != nil {
		return err
	}
	if offer == nil {
		return nil
	}
	return e.announce(ctx, tx, accountID, offer)
}

// offered publishes a release to every tab this player has open.
func (s *Server) offered(ctx context.Context, tx pgx.Tx, accountID string, offer *event.Offer) error {
	return eventOfferedMessage.Publish(ctx, tx,
		realtime.To{Account: accountID},
		eventOffered{Events: events(event.Standing{Offer: offer})},
	)
}

/*
backlog answers the Replenish half of rollover.World (§3.3, §4).

One method, filled in here rather than in internal/rollover, exactly as the Karma
decay beside it was. The date is the day being rolled into; the module reads its
own clock for the timestamps it writes, because a backlog is topped up now even
when the day it is being topped up for is being caught up on.
*/
func (w work) Replenish(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date) error {
	return w.events.Replenish(ctx, tx, accountID, on)
}

/*
bearings answers event.Bearings: who is coming back to this.

§4 decides release from "activity, time away, and what would serve the player
next", and neither half is in internal/event's tables. This is the seam that
hands it both without handing it either module.

Two things it does *not* do, and both are the point. It does not read a factor
score — internal/factor answers with a list of which factors are lagging and no
numbers, so §3.2's unquantified hidden three stay unquantified on the other side
of this boundary. And it does not touch `last_seen_at`: measuring an absence
would end it, which is why internal/account keeps the gap rather than deriving
it.
*/
type bearings struct {
	accounts *account.Accounts
	factors  *factor.Factors
}

func (b bearings) Away(ctx context.Context, tx pgx.Tx, accountID string) (time.Duration, error) {
	return b.accounts.Away(ctx, tx, accountID)
}

func (b bearings) Lagging(ctx context.Context, tx pgx.Tx, accountID string) ([]simulation.Factor, error) {
	return b.factors.Lagging(ctx, tx, accountID)
}

// where is the place a player is standing, for the world snapshot's release.
//
// The snapshot has already ensured the character, so this is a read of something
// it holds rather than a second query — see worldSnapshot, which passes it in.
// Named here so that "an event is placed where the player actually is" is one
// fact with one source.
func where(c character.Character) string {
	if c.At.Key == "" {
		return city.Start
	}
	return c.At.Key
}
