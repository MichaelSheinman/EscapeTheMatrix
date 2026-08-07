package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

/*
Daily conflicts through the transport that ships them: a real router, a real
Postgres, and the same commands the browser sends (§14).

What is worth proving here is what internal/conflict's own tests cannot — that
the whole of §3.15's loop is reachable by a client. Days pass with nobody
asking; the people the player lives among produce friction; what the rules do
not cover is standing in front of them on the next page load; settling it is one
command; and a fortnight of it reads back as a pattern rather than as a list.
*/

// conflictSurfaceView is the daily-conflict surface as it comes off the wire,
// which is the only shape a client ever sees it in.
type conflictSurfaceView struct {
	Waiting *struct {
		ID          int64    `json:"id"`
		Name        string   `json:"name"`
		Line        string   `json:"line"`
		Who         string   `json:"who"`
		Does        string   `json:"does"`
		Where       string   `json:"where"`
		Environment string   `json:"environment"`
		Why         string   `json:"why"`
		Standing    []string `json:"standing"`
		On          string   `json:"on"`
		At          string   `json:"at"`
		Answers     []struct {
			Key  string `json:"key"`
			Does string `json:"does"`
		} `json:"answers"`
	} `json:"waiting"`
	Among *struct {
		Environment string   `json:"environment"`
		Name        string   `json:"name"`
		Note        string   `json:"note"`
		Holds       []string `json:"holds"`
		People      []struct {
			Key      string   `json:"key"`
			Name     string   `json:"name"`
			Note     string   `json:"note"`
			Does     string   `json:"does"`
			Where    string   `json:"where"`
			Standing []string `json:"standing"`
			Met      int      `json:"met"`
			Good     int      `json:"good"`
			Poor     int      `json:"poor"`
		} `json:"people"`
		Questions []struct {
			Question string `json:"question"`
			Ask      string `json:"ask"`
			Met      int    `json:"met"`
			Covered  int    `json:"covered"`
			Good     int    `json:"good"`
			Poor     int    `json:"poor"`
		} `json:"questions"`
		Since string `json:"since"`
	} `json:"among"`
}

// livingFor reads the daily-conflict surface out of the world snapshot.
func livingFor(t *testing.T, s *Server, as *http.Cookie) conflictSurfaceView {
	t.Helper()
	var out struct {
		Conflicts conflictSurfaceView `json:"conflicts"`
	}
	rec := read(t, s, as, worldPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d, body = %s", rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("snapshot is not a world: %v", err)
	}
	return out.Conflicts
}

/*
§3.15's loop, end to end, over the wire.

A player with an empty playbook lives among six people. Days pass with nobody
asking for anything. What the people around them produced is standing in front
of them the next time they look, with the answers the content wrote for it, and
with what is between the two of them said in the game's own words — because a
conflict that follows from a debt is only legible as following from one if the
debt is on the same screen (§3.6).
*/
func TestLivingSomewhereProducesFrictionAndBringsWhatNothingCoveredToThePlayer(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)
	clk := stopped(t, s, time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC))

	account := accountOf(t, st)
	// The first snapshot creates the world and the ledger. Everything after it
	// is a day this test decided should happen.
	first := livingFor(t, s, as)
	if first.Waiting != nil {
		t.Fatalf("something was waiting before a day had passed: %+v", first.Waiting)
	}
	// The population is there from the first look, because who you are among is
	// authored content rather than something that has to be earned (§3.15).
	if first.Among == nil || len(first.Among.People) == 0 {
		t.Fatalf("the player is among nobody: %+v", first.Among)
	}
	if len(first.Among.Holds) == 0 {
		t.Error("nothing says what these people have in common")
	}
	for _, p := range first.Among.People {
		if p.Does == "" {
			t.Errorf("%q is legible from nothing they do", p.Key)
		}
	}

	// Three days pass. Nobody sends a command.
	upTo(t, st, account, "2026-08-05")
	clk.At = clk.At.AddDate(0, 0, 3)

	standing := livingFor(t, s, as).Waiting
	if standing == nil {
		t.Fatal("three days among six people produced nothing")
	}
	if standing.Name == "" || standing.Line == "" {
		t.Errorf("a conflict with nothing said about it: %+v", standing)
	}
	if standing.Who == "" || standing.Does == "" {
		t.Errorf("a conflict with nobody in it: %+v", standing)
	}
	if !strings.Contains(standing.Line, standing.Who) {
		t.Errorf("the moment does not name the person it is with: %q", standing.Line)
	}
	if standing.Why == "" {
		t.Error("nothing says why it reached them")
	}
	if len(standing.Answers) < 2 {
		t.Errorf("%d things the player could do about it", len(standing.Answers))
	}
	// §3.2: the surface says what happened and never what to do about it, and
	// §3.15: nothing anywhere names what kind of place this is.
	for _, word := range []string{"recommend", "suggest", "you should", "the best thing"} {
		if strings.Contains(strings.ToLower(standing.Line+standing.Why), word) {
			t.Errorf("the conflict says %q; the game states what happened and leaves the verdict", word)
		}
	}
	// And nothing anywhere names what kind of place this is — including the
	// sentence about the person, which is the one piece of writing whose whole
	// job is to make the population legible without labelling it.
	for _, word := range []string{"orange", "green", "stage", "register"} {
		if strings.Contains(strings.ToLower(standing.Line+standing.Why+standing.Does), word) {
			t.Errorf("the conflict says %q, which labels the framework (§3.2)", word)
		}
	}

	// And it is the same conflict on the next page load, because a live decision
	// that cannot be found again was never live.
	again := livingFor(t, s, as).Waiting
	if again == nil || again.ID != standing.ID {
		t.Fatalf("it was gone by the next look: %+v", again)
	}

	// Settling it is one command.
	chosen := standing.Answers[0]
	rec := post(t, s, as, "conflict.answer", "c1",
		`{"conflict":`+itoa(standing.ID)+`,"answer":"`+chosen.Key+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("answer = %d, body = %s", rec.Code, rec.Body)
	}
	var answered struct {
		Conflicts conflictSurfaceView `json:"conflicts"`
		Came      string              `json:"came"`
		Did       string              `json:"did"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &answered); err != nil {
		t.Fatalf("the answer is not a result: %v", err)
	}
	if answered.Came == "" {
		t.Error("nothing came of it")
	}
	if answered.Did != chosen.Does {
		t.Errorf("the character %q, and the answer offered %q", answered.Did, chosen.Does)
	}
	if answered.Conflicts.Waiting != nil {
		t.Errorf("something is still waiting after it was settled: %+v", answered.Conflicts.Waiting)
	}
	// The ledger moved, which is what makes tomorrow follow from today.
	met := 0
	for _, p := range answered.Conflicts.Among.People {
		met += p.Met
	}
	if met == 0 {
		t.Error("a settled conflict counted against nobody")
	}

	// Twice is refused, and so is a posture nobody wrote an answer for.
	if rec := post(t, s, as, "conflict.answer", "c2",
		`{"conflict":`+itoa(standing.ID)+`,"answer":"`+chosen.Key+`"}`); rec.Code != http.StatusConflict {
		t.Errorf("settling it twice = %d, want 409", rec.Code)
	}
	if rec := post(t, s, as, "conflict.answer", "c3",
		`{"conflict":`+itoa(standing.ID)+`,"answer":""}`); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("an answer with nothing in it = %d, want 422", rec.Code)
	}

	// §3.14's step 2, for the other half of what writes records: the log reads
	// back what happened, who it was with, and what came of it.
	read := logOf(t, s, as)
	var line logLine
	for _, f := range read.Fires {
		if f.Name == standing.Name {
			line = f
			break
		}
	}
	if line.Name == "" || line.Line == "" || line.Came == "" {
		t.Fatalf("the conflict is not in the log: %+v", read.Fires)
	}
}

/*
The pattern, which is the whole reason any of it is counted (§3.6, §3.15).

"One bad outcome is noise, the same rule failing daily for a week is a pattern —
and a pattern is diagnosable." What a client is handed is the counts, per person
and per question, with the number that matters most being how many of them the
player's own rules covered.
*/
func TestAFortnightOfLivingSomewhereReadsBackAsCounts(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)
	clk := stopped(t, s, time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC))
	account := accountOf(t, st)
	read(t, s, as, worldPath)

	// A fortnight, one conflict of which is settled by hand each time the player
	// looks — because nothing new arrives while something is waiting, which is
	// itself the shape of a playbook with holes in it.
	for day := 0; day < 6; day++ {
		upTo(t, st, account, "2026-08-05")
		clk.At = clk.At.AddDate(0, 0, 1)
		standing := livingFor(t, s, as).Waiting
		if standing == nil {
			continue
		}
		post(t, s, as, "conflict.answer", "settle-"+itoa(int64(day)),
			`{"conflict":`+itoa(standing.ID)+`,"answer":"`+standing.Answers[0].Key+`"}`)
	}

	among := livingFor(t, s, as).Among
	if among == nil {
		t.Fatal("the player is among nobody")
	}
	met, poorOrGood := 0, 0
	for _, p := range among.People {
		met += p.Met
		poorOrGood += p.Good + p.Poor
	}
	if met == 0 {
		t.Fatal("a fortnight of living somewhere counted against nobody")
	}
	if len(among.Questions) == 0 {
		t.Fatal("a fortnight of conflicts adds up to no pattern at all")
	}
	for _, q := range among.Questions {
		if q.Ask == "" {
			t.Errorf("%q reads back as a key rather than as a question", q.Question)
		}
		// A player with no rules covers nothing, which is the number §3.15 says
		// the holes in a playbook are made of.
		if q.Covered != 0 {
			t.Errorf("%d of %q were covered by a player who has written no rules", q.Covered, q.Question)
		}
	}
	if among.Since == "" {
		t.Error("nothing says what window the counts cover")
	}
}
