package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/event"
)

/*
The event backlog, at the edge it is allowed to reach (§4).

The mechanic is proved in internal/event against a clock a test can move. What
is worth proving here is what only the transport can show: that walking is what
releases something, that answering it is a command with no event key in it, that
the backlog itself never crosses the wire, and that a client is told about a
release over the socket rather than by asking.
*/

// eventsOf reads the event surface out of a world snapshot.
func eventsOf(t *testing.T, s *Server, as *http.Cookie) eventsView {
	t.Helper()
	rec := read(t, s, as, worldPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var out struct {
		Events eventsView `json:"events"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not a world snapshot: %v", err)
	}
	return out.Events
}

// walkTo moves the character on foot, which is the encounter surface (§3.12).
func walkTo(t *testing.T, s *Server, as *http.Cookie, key, place string) {
	t.Helper()
	rec := post(t, s, as, "character.move", key, `{"to":"`+place+`","by":"foot"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("walking to %s: status = %d, body = %s", place, rec.Code, rec.Body)
	}
}

// offered walks home, which is where the shipped catalogue has something, and
// returns whatever the world put in front of the player.
func offered(t *testing.T, s *Server, as *http.Cookie) *eventView {
	t.Helper()
	// The snapshot first: it is what fills an empty backlog, and it is what a
	// real client does before it moves anywhere.
	eventsOf(t, s, as)
	walkTo(t, s, as, "walk-1", "east-side-apartment")
	return eventsOf(t, s, as).Offer
}

// The world puts something in front of the player when they walk somewhere it
// has something for them — and it is placed where they are actually standing.
func TestWalkingIsWhatReleasesAnEvent(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	if before := eventsOf(t, s, as).Offer; before != nil {
		t.Fatalf("something was on screen before the player moved: %s", before.Name)
	}

	offer := offered(t, s, as)
	if offer == nil {
		t.Fatal("walking home put nothing in front of the player")
	}
	if offer.Place != "east-side-apartment" {
		t.Errorf("it was placed at %s, and the player is at home", offer.Place)
	}
	if offer.Zone == "" {
		t.Error("it was placed nowhere in particular")
	}
	if offer.Line == "" || offer.Accept == "" || offer.Decline == "" {
		t.Errorf("the offer cannot be read or answered: %+v", offer)
	}
	if offer.Until == "" {
		t.Error("the offer never says when it lapses")
	}
}

/*
A release is announced over the socket, to every tab.

No Origin even though the player's own walk caused it: the release was the
world's decision rather than the command's, and §8's rule 4 is about echoes of
your own action. An event finding you is not one.
*/
func TestAReleaseIsAnnouncedToEveryTab(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)

	if offered(t, s, as) == nil {
		t.Fatal("nothing was released")
	}

	who := accountOf(t, st)
	var origins []any
	rows, err := st.Pool().Query(t.Context(),
		`SELECT origin FROM realtime_outbox WHERE account_id = $1 AND type = 'event.offered'`, who)
	if err != nil {
		t.Fatalf("read the outbox: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var origin *string
		if err := rows.Scan(&origin); err != nil {
			t.Fatalf("read the outbox: %v", err)
		}
		if origin != nil {
			t.Errorf("a release was addressed away from the tab that caused it (%q)", *origin)
		}
		origins = append(origins, origin)
	}
	if len(origins) != 1 {
		t.Errorf("%d releases were announced, want 1", len(origins))
	}
}

// Answering is a command that names the offer and nothing else. There is no
// event key on the wire, because a client that named one would be choosing.
func TestAnsweringNamesTheOfferAndNothingElse(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	offer := offered(t, s, as)
	if offer == nil {
		t.Fatal("nothing was released")
	}

	rec := post(t, s, as, "event.accept", "answer-1", `{"event":`+itoa(offer.ID)+`}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("accepting: status = %d, body = %s", rec.Code, rec.Body)
	}

	var out eventResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not an answer: %v", err)
	}
	if out.Events.Offer != nil {
		t.Error("something is still on screen after it was answered")
	}
	if !out.Open && out.After == "" {
		t.Error("an event that finished on the spot said nothing about how it went")
	}

	// Again is a 409: the request is fine and the world has moved.
	if rec := post(t, s, as, "event.accept", "answer-2", `{"event":`+itoa(offer.ID)+`}`); rec.Code != http.StatusConflict {
		t.Errorf("answering twice: status = %d, want 409", rec.Code)
	}
	// Zero is wrong forever, which is the one refusal that is a 422.
	if rec := post(t, s, as, "event.accept", "answer-3", `{"event":0}`); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("answering nothing: status = %d, want 422", rec.Code)
	}
}

// Declining takes it off screen and does not consume it: the row is still in
// the backlog, waiting, with the refusal counted (§3.9, §4).
func TestDecliningTakesItOffScreenWithoutConsumingIt(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)

	offer := offered(t, s, as)
	if offer == nil {
		t.Fatal("nothing was released")
	}

	rec := post(t, s, as, "event.decline", "no-1", `{"event":`+itoa(offer.ID)+`}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("declining: status = %d, body = %s", rec.Code, rec.Body)
	}
	if eventsOf(t, s, as).Offer != nil {
		t.Error("something is still on screen after it was declined")
	}

	var (
		state    string
		declines int
	)
	if err := st.Pool().QueryRow(t.Context(),
		`SELECT state, declines FROM event_backlog WHERE id = $1`, offer.ID,
	).Scan(&state, &declines); err != nil {
		t.Fatalf("read the backlog: %v", err)
	}
	if state != "waiting" {
		t.Errorf("a declined event is %q, want waiting — declining must not consume", state)
	}
	if declines != 1 {
		t.Errorf("the refusal was counted %d times", declines)
	}
}

/*
The backlog does not cross the wire.

A client that could read what was queued could walk towards it, which would turn
§4's sandbox into a quest list. The snapshot carries the one thing standing in
front of the player and no key that names anything else — asserted by reading
the whole body as text and looking for the events the world is holding back.
*/
func TestTheWorldSnapshotCarriesNoBacklog(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	// Fill the backlog: the snapshot itself does it when there is nothing.
	eventsOf(t, s, as)

	rec := read(t, s, as, worldPath)
	body := rec.Body.String()

	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &snapshot); err != nil {
		t.Fatalf("response is not a world snapshot: %v", err)
	}
	if _, leaked := snapshot["backlog"]; leaked {
		t.Error("the world snapshot carries the backlog")
	}

	// Nothing is on screen at the start place, so no event's words should be
	// anywhere in the body at all.
	for _, e := range event.All() {
		if strings.Contains(body, e.Line) {
			t.Errorf("the snapshot carries the words of %s, which is not in front of the player", e.Key)
		}
	}
}

// The history is its own paginated GET, like the boards and the playbook's log:
// it writes nothing and it is unbounded (§10.2).
func TestTheEventRecordIsItsOwnPage(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	offer := offered(t, s, as)
	if offer == nil {
		t.Fatal("nothing was released")
	}
	if rec := post(t, s, as, "event.decline", "no-1", `{"event":`+itoa(offer.ID)+`}`); rec.Code != http.StatusOK {
		t.Fatalf("declining: status = %d, body = %s", rec.Code, rec.Body)
	}

	rec := read(t, s, as, eventLogPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var log eventLogView
	if err := json.Unmarshal(rec.Body.Bytes(), &log); err != nil {
		t.Fatalf("response is not a record: %v", err)
	}
	if log.Total != 1 || len(log.Records) != 1 {
		t.Fatalf("the record holds %d rows, want 1", log.Total)
	}
	if got := log.Records[0].Answer; got != string(event.Declined) {
		t.Errorf("the record says %q, want %q", got, event.Declined)
	}
	if log.Records[0].Name != offer.Name {
		t.Errorf("the record names %q, want %q", log.Records[0].Name, offer.Name)
	}
	if log.Size != event.LogPage {
		t.Errorf("a page holds %d, want %d", log.Size, event.LogPage)
	}

	// A page that is not a number is 422; one past the end is clamped.
	if rec := read(t, s, as, eventLogPath+"?page=soon"); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("a page that is not a number: status = %d, want 422", rec.Code)
	}
	if rec := read(t, s, as, eventLogPath+"?page=99"); rec.Code != http.StatusOK {
		t.Errorf("a page past the end: status = %d, want 200", rec.Code)
	}
}
