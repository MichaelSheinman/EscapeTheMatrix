package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

/*
A challenge, end to end over the transport it actually ships behind (§5).

The framework is proved in internal/challenge and the content is proved beside
it. What only this level can show is the thing #24 is actually about: that there
is a challenge a player can walk up to, begin, leave, come back to, finish, and
never play again — and that nothing about how it was scored comes back with it.

And, since #25, that the same is true of a challenge nobody wrote any Go for.
*/

// challengeOf reads the challenge surface out of a world snapshot.
func challengeOf(t *testing.T, s *Server, as *http.Cookie) challengeView {
	t.Helper()
	rec := read(t, s, as, worldPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var out struct {
		Challenge challengeView `json:"challenge"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not a world snapshot: %v", err)
	}
	return out.Challenge
}

// begun starts the challenge standing where the player is, and hands back the
// run it opened.
func begun(t *testing.T, s *Server, as *http.Cookie) *runView {
	t.Helper()

	here := challengeOf(t, s, as).Here
	if here == nil {
		t.Fatal("nothing is standing where the player is")
	}
	rec := post(t, s, as, "challenge.begin", "begin-1", `{"challenge":"`+here.Challenge+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("beginning: status = %d, body = %s", rec.Code, rec.Body)
	}
	return runOf(t, rec.Body.String())
}

// runOf decodes a challenge command's own response.
func runOf(t *testing.T, body string) *runView {
	t.Helper()
	var out challengeResult
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("response is not a challenge: %v", err)
	}
	return out.Challenge.Run
}

// choose commits one option and returns where the run is afterwards.
func choose(t *testing.T, s *Server, as *http.Cookie, key string, run *runView, option string) *runView {
	t.Helper()
	body := `{"challenge":"` + run.Challenge + `","scene":"` + run.Scene + `","option":"` + option + `"}`
	rec := post(t, s, as, "challenge.choose", key, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("choosing %s: status = %d, body = %s", option, rec.Code, rec.Body)
	}
	return runOf(t, rec.Body.String())
}

// through plays a whole passage, committing to the option at `pick` each time
// (clamped to what is on offer), and hands back the finished run.
func through(t *testing.T, s *Server, as *http.Cookie, run *runView, pick int) *runView {
	t.Helper()
	for turn := 0; !run.Done; turn++ {
		if turn > 12 {
			t.Fatal("the run does not end")
		}
		if len(run.Choices) == 0 {
			t.Fatalf("%s offers nothing and has not finished", run.Scene)
		}
		at := pick
		if at >= len(run.Choices) {
			at = len(run.Choices) - 1
		}
		run = choose(t, s, as, "step-"+run.Scene, run, run.Choices[at].Key)
	}
	return run
}

/*
§4's availability half: the world exposes an entry point where the content says
it stands, and a player standing there is told so.

Nothing else about it crosses — no scenes, no options, no idea how long it is.
*/
func TestAChallengeStandsWhereItIsAuthored(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	here := challengeOf(t, s, as).Here
	if here == nil {
		t.Fatal("the player is standing where a challenge is authored and is not told")
	}
	if here.Name == "" || here.Challenge == "" {
		t.Fatalf("the doorway cannot be named or entered: %+v", here)
	}
	if here.Zone == "" {
		t.Error("it stands nowhere in particular, so no painting can draw it")
	}

	// Somewhere else, it is not in front of them — and cannot be begun.
	walkTo(t, s, as, "walk-downtown", "downtown")
	if surface := challengeOf(t, s, as); surface.Here != nil {
		t.Errorf("it followed the player to %s", surface.Here.Place)
	}
	rec := post(t, s, as, "challenge.begin", "begin-elsewhere", `{"challenge":"`+here.Challenge+`"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("beginning it from another district: status = %d, want 409", rec.Code)
	}
}

// Beginning puts the player at the opening scene, with the words the mechanic
// wrote and the authored positions it offers.
func TestBeginningStandsThePlayerInTheOpeningScene(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	run := begun(t, s, as)
	switch {
	case run == nil:
		t.Fatal("beginning produced no run")
	case run.Scene == "":
		t.Fatal("the run is standing nowhere")
	case run.Where == "" || run.Ask == "":
		t.Fatalf("the scene has no room and no question: %+v", run)
	case len(run.Beats) == 0:
		t.Fatal("nobody says anything")
	case len(run.Choices) < 2:
		t.Fatalf("%d ways out of the opening scene, so nothing is chosen", len(run.Choices))
	case run.Taken != "":
		t.Fatal("something was received before anything was committed to")
	case run.Stage != 1 || run.Stages < 3:
		t.Fatalf("it opens at stage %d of %d", run.Stage, run.Stages)
	case run.Done:
		t.Fatal("it is over before it began")
	}
	for _, c := range run.Choices {
		if c.Key == "" || c.Text == "" {
			t.Fatalf("an unreadable choice: %+v", c)
		}
	}

	// Beginning twice is resuming, not starting again: §5.1 makes them one
	// call, because a client cannot know which of the two it wants.
	second := post(t, s, as, "challenge.begin", "begin-2", `{"challenge":"`+run.Challenge+`"}`)
	if second.Code != http.StatusOK {
		t.Fatalf("beginning again: status = %d, body = %s", second.Code, second.Body)
	}
	again := runOf(t, second.Body.String())
	if again.Scene != run.Scene || again.Began != run.Began {
		t.Fatalf("a second begin started a second run: %s at %s", again.Scene, again.Began)
	}
}

/*
§5.1: "save and resume is mandatory — even at 6 minutes, players abandon
partway."

Which is what a world snapshot is for. The player who closed the tab in the
middle of a conversation is put back into it rather than at the door.
*/
func TestAbandoningPartwayResumesWhereItWasLeft(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	run := begun(t, s, as)
	run = choose(t, s, as, "one", run, run.Choices[0].Key)
	moved := choose(t, s, as, "two", run, run.Choices[1].Key)

	// A page load, which is every kind of coming back there is.
	surface := challengeOf(t, s, as)
	if surface.Run == nil {
		t.Fatal("the run was not there when the player came back")
	}
	if surface.Run.Scene != moved.Scene {
		t.Fatalf("they came back to %q, having left at %q", surface.Run.Scene, moved.Scene)
	}
	if surface.Run.Taken == "" {
		t.Error("the run resumed without saying how the last thing said was taken")
	}
	if surface.Here != nil {
		t.Error("a door was offered to somebody already in the middle of one")
	}

	// And the tab that answered a scene the run has since left is told so,
	// rather than having its answer applied twice.
	stale := post(t, s, as, "challenge.choose", "stale",
		`{"challenge":"`+run.Challenge+`","scene":"`+run.Scene+`","option":"`+run.Choices[0].Key+`"}`)
	if stale.Code != http.StatusConflict {
		t.Fatalf("a stale answer: status = %d, want 409", stale.Code)
	}
}

/*
§5.1's "once ever, per player", from the outside.

And the other half of it: a finished challenge stops standing there. A door that
cannot be opened is worse than no door.
*/
func TestAChallengeIsPlayedOnceEver(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	done := through(t, s, as, begun(t, s, as), 0)
	if !done.Done {
		t.Fatal("the passage never finished")
	}

	surface := challengeOf(t, s, as)
	if surface.Run != nil {
		t.Errorf("a finished run is still the thing on screen: %s", surface.Run.Scene)
	}
	if surface.Here != nil {
		t.Errorf("it is still standing there after being played: %s", surface.Here.Name)
	}

	rec := post(t, s, as, "challenge.begin", "begin-again", `{"challenge":"`+done.Challenge+`"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("beginning it a second time: status = %d, want 409", rec.Code)
	}
}

/*
§3.10: hidden scoring, "never surfaced as a grade, never confirmed as right."

What a finished run comes back with is the reaction authored for the band its
trail landed in, and *only* that. The rest of this test is the guard: no weight,
no total, no bucket key, no delta — not in the response and not in the snapshot
that follows it.
*/
func TestFinishingGivesAReactionAndNeverAGrade(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	rec := post(t, s, as, "challenge.begin", "begin-1",
		`{"challenge":"`+challengeOf(t, s, as).Here.Challenge+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("beginning: status = %d, body = %s", rec.Code, rec.Body)
	}

	done := through(t, s, as, runOf(t, rec.Body.String()), 0)
	switch {
	case done.Closing == "":
		t.Fatal("finishing said nothing at all")
	case done.Taken == "":
		t.Fatal("the last thing the player said was not received by anybody")
	case len(done.Choices) != 0 || done.Ask != "":
		t.Fatal("a finished run is still asking something")
	}

	// A finished run is not what the world is currently doing, so it is not in
	// the snapshot: the ending is the command's own answer, read once.
	if after := challengeOf(t, s, as); after.Run != nil || after.Here != nil {
		t.Error("the finished challenge is still on the world's surface")
	}

	// The words a grade would be made of, in everything the player is sent.
	body := strings.ToLower(rec.Body.String() + done.Closing)
	for _, word := range []string{"score", "weight", "delta", "credit", "grade", "bucket", "outcome"} {
		if strings.Contains(body, word) {
			t.Errorf("%q crossed the wire, which is the hidden scoring shown", word)
		}
	}
}

// An option that is not on offer is the request being wrong rather than the
// world being in the way, so it is 422 and stays 422.
func TestSomethingThatIsNotOneOfTheChoicesIsRefused(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	run := begun(t, s, as)
	rec := post(t, s, as, "challenge.choose", "nonsense",
		`{"challenge":"`+run.Challenge+`","scene":"`+run.Scene+`","option":"say-nothing-at-all"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}

	empty := post(t, s, as, "challenge.begin", "empty", `{"challenge":""}`)
	if empty.Code != http.StatusUnprocessableEntity {
		t.Fatalf("beginning nothing: status = %d, want 422", empty.Code)
	}
	unknown := post(t, s, as, "challenge.begin", "unknown", `{"challenge":"nowhere/x"}`)
	if unknown.Code != http.StatusConflict {
		t.Fatalf("beginning something nobody authored: status = %d, want 409", unknown.Code)
	}
}

/*
§8's rules 3 and 4: the acting tab has the response, and every other tab is told
over the socket.

One message for beginning, choosing and finishing, because all three are the
same event — the run is standing somewhere else now.
*/
func TestOtherTabsAreToldTheRunMoved(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)

	run := begun(t, s, as)
	choose(t, s, as, "one", run, run.Choices[0].Key)

	who := accountOf(t, st)
	var sent int
	if err := st.Pool().QueryRow(t.Context(),
		`SELECT count(*) FROM realtime_outbox WHERE account_id = $1 AND type = 'challenge.moved'`,
		who).Scan(&sent); err != nil {
		t.Fatalf("read the outbox: %v", err)
	}
	if sent != 2 {
		t.Fatalf("%d messages for a begin and a choice, want 2", sent)
	}
}

/*
§4's doorway: an event authored `awaits` opens a challenge, and accepting it is
entering the challenge rather than being told one is now available.

The event stays pending until the run finishes, which is internal/challenge's
Resolve seam — the pair settle in the same transaction as the last choice.
*/
func TestAcceptingTheEventBehindItOpensTheRun(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)

	// The world decides when to put something in front of somebody (§4), and
	// its policy will not release two things inside half an hour — so a test
	// that walked about hoping for this one would be a coin toss. The backlog
	// is therefore arranged rather than waited on: everything else is taken out
	// of it, and the ordinary path does the rest. What is being tested is what
	// happens when the player accepts, not which day the world picks.
	challengeOf(t, s, as) // a page load, which is what fills the backlog
	who := accountOf(t, st)
	if _, err := st.Pool().Exec(t.Context(),
		`DELETE FROM event_backlog WHERE account_id = $1 AND event <> 'two-people-arguing'`,
		who); err != nil {
		t.Fatalf("clear the backlog: %v", err)
	}
	if _, err := st.Pool().Exec(t.Context(), `
		INSERT INTO event_backlog (account_id, event, state, generated_at, eligible_at)
		VALUES ($1, 'two-people-arguing', 'waiting', now(), now())
		ON CONFLICT DO NOTHING`, who); err != nil {
		t.Fatalf("queue the event: %v", err)
	}

	// Out and back. It is a `world` event, so only arriving somewhere releases
	// it (§4) — and it is placed on the step of the player's own building.
	walkTo(t, s, as, "walk-out", "east-side-apartment")
	walkTo(t, s, as, "walk-back", "east-side")

	found := eventsOf(t, s, as).Offer
	if found == nil || found.Name == "" {
		t.Fatal("walking up to the step put nothing in front of the player")
	}

	rec := post(t, s, as, "event.accept", "accept-1", `{"event":`+itoa(found.ID)+`}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("accepting: status = %d, body = %s", rec.Code, rec.Body)
	}
	var answered eventResult
	if err := json.Unmarshal(rec.Body.Bytes(), &answered); err != nil {
		t.Fatalf("response is not an answer: %v", err)
	}
	if answered.Challenge == nil || answered.Challenge.Run == nil {
		t.Fatal("accepting the doorway did not put the player in the challenge")
	}
	if !answered.Open {
		t.Fatal("the event finished on the spot, and the challenge behind it has not")
	}

	// The event is still open until the run ends, and settled by its ending.
	done := through(t, s, as, answered.Challenge.Run, 1)
	if !done.Done {
		t.Fatal("the passage never finished")
	}
	var resolved int
	if err := st.Pool().QueryRow(t.Context(),
		`SELECT count(*) FROM event_record
		  WHERE account_id = $1 AND event = 'two-people-arguing' AND resolved_at IS NOT NULL`,
		who).Scan(&resolved); err != nil {
		t.Fatalf("read the record: %v", err)
	}
	if resolved != 1 {
		t.Fatalf("%d settled records for the event the run came through, want 1", resolved)
	}
}

/*
The challenge that is only content, played over the wire (#25).

§5.2 draws a line — **mechanic types are code, challenges are data** — and the
laundromat is the challenge on the far side of it: two YAML files, no Go written
for it, a mechanic type that had never heard of it. This is the only test that
can say the line holds all the way out to a player, because everything between
here and the content is code that was written for a different challenge.

It is also §4's availability half on its own, which the argument on the stoop
cannot show: nothing offers this one. It stands in the laundromat and waits, and
the way in is to walk through the door with your washing.
*/
func TestAChallengeThatIsOnlyContentIsPlayable(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	const laundromat = "east-side-laundromat"
	walkTo(t, s, as, "walk-in", laundromat)

	here := challengeOf(t, s, as).Here
	if here == nil {
		t.Fatal("nothing is standing in the laundromat")
	}
	if here.Challenge != "resolve_conflict/b" || here.Place != laundromat {
		t.Fatalf("the doorway is %+v", here)
	}

	rec := post(t, s, as, "challenge.begin", "begin-b", `{"challenge":"`+here.Challenge+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("beginning: status = %d, body = %s", rec.Code, rec.Body)
	}
	run := runOf(t, rec.Body.String())
	if run == nil || run.Where == "" || run.Ask == "" || len(run.Beats) == 0 || len(run.Choices) < 2 {
		t.Fatalf("the opening scene is not a scene: %+v", run)
	}
	if run.Stages < 5 {
		t.Errorf("it is %d scenes long, which is not six minutes of anything", run.Stages)
	}

	// A whole passage, and the branching is real: the words change as it goes.
	said := map[string]bool{run.Where: true}
	for turn := 0; !run.Done; turn++ {
		if turn > 12 {
			t.Fatal("the run does not end")
		}
		run = choose(t, s, as, "b-"+run.Scene, run, run.Choices[len(run.Choices)-1].Key)
		said[run.Where] = true
		if !run.Done && run.Taken == "" {
			t.Errorf("%s: nobody reacted to what was committed to", run.Scene)
		}
	}
	if len(said) < 5 {
		t.Errorf("the whole passage stood in %d places", len(said))
	}
	if run.Closing == "" {
		t.Fatal("finishing said nothing at all")
	}

	// And it is a different challenge from the one on the stoop, not the same
	// one wearing other words: the stoop is still there to be played.
	walkTo(t, s, as, "walk-out", "east-side")
	stoop := challengeOf(t, s, as).Here
	if stoop == nil || stoop.Challenge != "resolve_conflict/a" {
		t.Fatalf("playing one challenge of a type consumed another: %+v", stoop)
	}
}
