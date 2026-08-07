package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/clock"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/playbook"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
)

/*
Delegated work through the transport that ships it: a real router, a real
Postgres, and the same commands the browser sends (§14).

What is worth proving here is what neither internal/situation's tests nor
internal/playbook's can — that the whole of §3.14's loop is reachable by a
client. A day passes with nobody asking; the role produces something; the
player's own rules settle it or it is standing in front of them on the next page
load; answering it is one command; and the log reads back all four of what
happened, which rule fired, what the character did, and what came of it.
*/

// situationSurfaceView is the delegated-work surface as it comes off the wire,
// which is the only shape a client ever sees it in.
type situationSurfaceView struct {
	Waiting *struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		Line        string `json:"line"`
		Role        string `json:"role"`
		Environment string `json:"environment"`
		Why         string `json:"why"`
		On          string `json:"on"`
		At          string `json:"at"`
		Answers     []struct {
			Key  string `json:"key"`
			Does string `json:"does"`
		} `json:"answers"`
	} `json:"waiting"`
}

// waitingFor reads the delegated-work surface out of the world snapshot.
func waitingFor(t *testing.T, s *Server, as *http.Cookie) situationSurfaceView {
	t.Helper()
	var out struct {
		Situations situationSurfaceView `json:"situations"`
	}
	rec := read(t, s, as, worldPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d, body = %s", rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("snapshot is not a world: %v", err)
	}
	return out.Situations
}

// running puts a player in the operations chair.
//
// Directly in the table rather than through the interview, because how somebody
// gets hired is internal/career's ticket and career_test.go's test — what this
// file is about starts on the first morning after.
func running(t *testing.T, st *store.Store, accountID string) {
	t.Helper()
	if _, err := st.Pool().Exec(t.Context(), `
		UPDATE career SET tier = $2, reached = $2, since = now(), paid_through = NULL
		 WHERE account_id = $1`, accountID, int(simulation.Delegated),
	); err != nil {
		t.Fatalf("put them in the chair: %v", err)
	}
}

// logLine is one moment as §3.14's review surface reads it back: what happened,
// which answers were weighed, what the character did, and what came of it.
type logLine struct {
	ID         int64  `json:"id"`
	Situation  string `json:"situation"`
	Name       string `json:"name"`
	Line       string `json:"line"`
	Came       string `json:"came"`
	Verdict    string `json:"verdict"`
	Outcome    string `json:"outcome"`
	Note       string `json:"note"`
	Considered []struct {
		N     int    `json:"n"`
		State string `json:"state"`
		Says  string `json:"says"`
		Did   string `json:"did"`
	} `json:"considered"`
}

// logOf reads a page of the playbook's log.
func logOf(t *testing.T, s *Server, as *http.Cookie) (out struct {
	Fires []logLine `json:"fires"`
	Total int       `json:"total"`
}) {
	t.Helper()
	rec := read(t, s, as, logPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("log status = %d, body = %s", rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("the log is not a log: %v", err)
	}
	return out
}

/*
§3.14's loop, end to end, over the wire.

A player with an empty playbook holds a role that runs itself. A day passes with
nobody asking for anything. What the role produced is standing in front of them
the next time they look, with the answers the content wrote for it — which is
the ticket's fourth line, and it arrives on the same world snapshot as
everything else rather than through a channel of its own.
*/
func TestAJobRunsItselfAndBringsWhatNothingCoveredToThePlayer(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)
	clk := stopped(t, s, time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC))

	account := accountOf(t, st)
	// The first snapshot creates the world and the ledger. Everything after it
	// is a day this test decided should happen.
	read(t, s, as, worldPath)
	running(t, st, account)
	if standing := waitingFor(t, s, as); standing.Waiting != nil {
		t.Fatalf("something was waiting before a day had passed: %+v", standing.Waiting)
	}

	// Three days pass. Nobody sends a command.
	upTo(t, st, account, "2026-08-05")
	clk.At = clk.At.AddDate(0, 0, 3)

	standing := waitingFor(t, s, as).Waiting
	if standing == nil {
		t.Fatal("three days in an operations job produced nothing")
	}
	if standing.Name == "" || standing.Line == "" {
		t.Errorf("a moment with nothing said about it: %+v", standing)
	}
	if standing.Role == "" {
		t.Error("the moment does not say which job produced it")
	}
	if len(standing.Answers) < 2 {
		t.Errorf("%d things the player could do about it", len(standing.Answers))
	}
	if standing.Why == "" {
		t.Error("nothing says why it reached them")
	}
	// §3.2: the surface says what happened and never what to do about it.
	for _, word := range []string{"recommend", "suggest", "you should", "best", "correct"} {
		if strings.Contains(strings.ToLower(standing.Line+standing.Why), word) {
			t.Errorf("the moment says %q; the game states the price and leaves the verdict", word)
		}
	}

	// And it is the same moment on the next page load, because a live decision
	// that cannot be found again was never live.
	again := waitingFor(t, s, as).Waiting
	if again == nil || again.ID != standing.ID {
		t.Fatalf("it was gone by the next look: %+v", again)
	}

	// Answering it is one command.
	chosen := standing.Answers[0]
	rec := post(t, s, as, "situation.answer", "a1",
		`{"situation":`+itoa(standing.ID)+`,"answer":"`+chosen.Key+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("answer = %d, body = %s", rec.Code, rec.Body)
	}
	var answered struct {
		Situations situationSurfaceView `json:"situations"`
		Came       string               `json:"came"`
		Did        string               `json:"did"`
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
	if answered.Situations.Waiting != nil {
		t.Errorf("something is still waiting after it was answered: %+v", answered.Situations.Waiting)
	}

	// Twice is refused, and so is a posture the moment has no answer for.
	if rec := post(t, s, as, "situation.answer", "a2",
		`{"situation":`+itoa(standing.ID)+`,"answer":"`+chosen.Key+`"}`); rec.Code != http.StatusConflict {
		t.Errorf("answering it twice = %d, want 409", rec.Code)
	}
	if rec := post(t, s, as, "situation.answer", "a3",
		`{"situation":`+itoa(standing.ID)+`,"answer":""}`); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("an answer with nothing in it = %d, want 422", rec.Code)
	}

	// §3.14's step 2: what happened, which rule was weighed, what the character
	// did, and what came of it — all four on one page.
	read := logOf(t, s, as)
	if len(read.Fires) == 0 {
		t.Fatal("nothing is in the log")
	}
	line := read.Fires[0]
	if line.Name == "" || line.Line == "" {
		t.Errorf("the log line does not say what happened: %+v", line)
	}
	if line.Came == "" {
		t.Error("the log line does not say what came of it")
	}
	if line.Verdict != "silent" || line.Note == "" {
		t.Errorf("verdict %q, note %q; the player had no rule for this", line.Verdict, line.Note)
	}
	if len(line.Considered) == 0 {
		t.Error("the log does not say which question was put")
	}
}

/*
The ticket's second and fifth lines over the wire: **rules auto-resolve
situations they cover**, and **revising a rule visibly changes subsequent
outcomes.**

One player, one job, and one word of their playbook changed halfway through. The
same moment comes round again on the other side of the edit and comes to
something different — which is only demonstrable because nothing else moved: the
matcher is deterministic (§7) and so is which moment a day produces.
*/
func TestRevisingTheRulesChangesWhatTheJobDoes(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)
	clk := stopped(t, s, time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC))
	account := accountOf(t, st)

	read(t, s, as, worldPath)
	running(t, st, account)

	// Every question the tier-three pool can put, answered the same way, so the
	// role never stalls on something nobody covered. All four amount to going
	// along with it, which is the playbook a player writes in week one.
	covering(t, s, as, "c", "accept")

	// The roster gap is §3.14's own example, and the answer in force is its
	// over-general one: accept it, otherwise accept it.
	before := untilItComesUp(t, s, as, clk, "the-roster-gap")

	// One word.
	compose(t, s, as, "r", "overtime", `{"term":"refuse"}`, `{"term":"accept"}`)
	after := untilItComesUp(t, s, as, clk, "the-roster-gap")

	if before.Verdict != "answered" || after.Verdict != "answered" {
		t.Fatalf("the rules settled %q and then %q", before.Verdict, after.Verdict)
	}
	if before.Came == "" || after.Came == "" {
		t.Fatal("a settled moment came to nothing")
	}
	if before.Came == after.Came {
		t.Errorf("the same moment came to the same thing after the rule changed: %.60s…", before.Came)
	}
	t.Logf("accept/accept → %.60s…", before.Came)
	t.Logf("refuse/accept → %.60s…", after.Came)
}

/*
§3.14's "good rules buy time, and gaps in the playbook are what demand
attention", proved where a player would feel it.

A role that is waiting on somebody produces nothing new, so a fortnight away
with an empty playbook is one decision rather than fourteen — which is also §4's
"a long absence does not dump the backlog", applied to the one surface that
could most easily have broken it.
*/
func TestALongAbsenceIsOneDecisionRatherThanAFortnightOfThem(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)
	clk := stopped(t, s, time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC))
	account := accountOf(t, st)

	read(t, s, as, worldPath)
	running(t, st, account)

	upTo(t, st, account, "2026-07-24")
	clk.At = clk.At.AddDate(0, 0, 14)
	read(t, s, as, worldPath)

	var waiting, total int
	if err := st.Pool().QueryRow(t.Context(),
		`SELECT count(*) FILTER (WHERE waiting), count(*) FROM job_situation WHERE account_id = $1`,
		account).Scan(&waiting, &total); err != nil {
		t.Fatalf("count: %v", err)
	}
	if waiting != 1 || total != 1 {
		t.Errorf("a fortnight away left %d moments, %d of them waiting; want one of each", total, waiting)
	}
	if standing := waitingFor(t, s, as).Waiting; standing == nil {
		t.Fatal("nothing is in front of the player after a fortnight")
	}
}

/*
The first active rung is reachable, which is what makes any of the above
something a player can get to.

#27 left tiers three to five on the ladder with no way into any of them, because
a rung that could be taken before the thing it does exists would be a salary
paid for nothing. The thing it does is the rest of this file, so the door is
open: the same shape as the rung below it — an interview standing somewhere,
hidden from anybody it is not for.
*/
func TestTheOperationsChairIsReachable(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)
	stopped(t, s, time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC))
	account := accountOf(t, st)

	// Standing at the office with no job at all, there is no door: an interview
	// is sat by somebody exactly one rung below it (§3.14).
	toTheOffice(t, s, as)
	if door := doorway(t, s, as); door != "" {
		t.Fatalf("the office is interviewing somebody off the street: %q", door)
	}

	// On the desk, capable, it is there.
	running(t, st, account)
	if _, err := st.Pool().Exec(t.Context(),
		`UPDATE career SET tier = 2, reached = 2 WHERE account_id = $1`, account); err != nil {
		t.Fatalf("put them on the desk: %v", err)
	}
	capableFor(t, st, account, simulation.Delegated)

	if door := doorway(t, s, as); door != "interview/b" {
		t.Fatalf("standing at the office on the rung below, the door is %q", door)
	}
	if rec := post(t, s, as, "challenge.begin", "b1",
		`{"challenge":"interview/b"}`); rec.Code != http.StatusOK {
		t.Fatalf("begin = %d, body = %s", rec.Code, rec.Body)
	}
	best(t, s, as, "interview/b")

	held := job(t, s, as)
	if held.Held == nil || held.Held.Tier != int(simulation.Delegated) {
		t.Fatalf("after the interview, holding %+v", held.Held)
	}

	// And the job it hired them into is one that runs itself.
	if !simulation.Delegated.Held() || simulation.Delegated.Passive() {
		t.Error("the rung it opens is not a delegated one")
	}
}

/* ── helpers ──────────────────────────────────────────────────────────── */

// toTheOffice walks from the east side to the desk on the fourth floor: one
// street west into downtown, and one door.
func toTheOffice(t *testing.T, s *Server, as *http.Cookie) {
	t.Helper()
	for i, to := range []string{"downtown", "downtown-office"} {
		walkTo(t, s, as, "office-"+itoa(int64(i)), to)
	}
}

/*
compose writes one rule, in the environment the player is standing in.

The commands rather than the module, because what this file is about is the loop
being reachable by a client — and the client has exactly these two verbs.
*/
func compose(t *testing.T, s *Server, as *http.Cookie, key, question, then, otherwise string) {
	t.Helper()
	body := `{"environment":"east-side","question":"` + question + `",` +
		`"when":[` + opening(t, question) + `],"join":"and",` +
		`"then":` + then + `,"otherwise":` + otherwise + `}`
	if rec := post(t, s, as, "playbook.compose", key+"-"+question, body); rec.Code != http.StatusOK {
		t.Fatalf("compose %s = %d, body = %s", question, rec.Code, rec.Body)
	}
}

// opening is a condition of a question that a player starts with and that takes
// no number, so a sentence can be composed on day one.
func opening(t *testing.T, question string) string {
	t.Helper()
	q, ok := playbook.Vocab().Question(question)
	if !ok {
		t.Fatalf("%q is not a question", question)
	}
	for _, term := range q.Conditions() {
		if term.Start && !term.Takes() {
			return `{"term":"` + term.Key + `"}`
		}
	}
	t.Fatalf("%q has no plain opening condition", question)
	return ""
}

/*
covering answers every question the tier-three pool can put, all one way.

A role only produces new work while nothing is waiting on the player (§3.14's
"good rules buy time"), so a test about what the *rules* do has to start from a
playbook with no holes in it — otherwise the first uncovered moment stops the
job and the rest of the fortnight is empty.
*/
func covering(t *testing.T, s *Server, as *http.Cookie, key, overtime string) {
	t.Helper()
	compose(t, s, as, key, "help", `{"term":"yes"}`, `{"term":"yes"}`)
	compose(t, s, as, key, "voice", `{"term":"finish"}`, `{"term":"finish"}`)
	compose(t, s, as, key, "money",
		`{"term":"give","value":"token"}`, `{"term":"give","value":"token"}`)
	compose(t, s, as, key, "overtime", `{"term":"`+overtime+`"}`, `{"term":"accept"}`)
}

/*
untilItComesUp lives one day at a time until the named moment is in the log.

Which moment a day produces is decided rather than rolled (§7) and this test has
no way to compute it from out here, so it walks — a day at a time, so each pass
is exactly one day of the rollover, and with a bound so a change that stopped
the job producing anything fails rather than hangs.
*/
func untilItComesUp(t *testing.T, s *Server, as *http.Cookie, clk *clock.Stepping, key string) logLine {
	t.Helper()
	seen := map[int64]bool{}
	for _, line := range logOf(t, s, as).Fires {
		seen[line.ID] = true
	}
	for range 40 {
		clk.At = clk.At.AddDate(0, 0, 1)
		read(t, s, as, worldPath)
		for _, line := range logOf(t, s, as).Fires {
			if !seen[line.ID] && line.Situation == key {
				return line
			}
			seen[line.ID] = true
		}
	}
	t.Fatalf("%q did not come up in forty days", key)
	return logLine{}
}

// capableFor grants enough hidden credit to clear a rung's requirement.
//
// The same inversion career_test.go's `capable` does, with the tier as an
// argument: the credit a reading corresponds to, read off the pure core's own
// curve, so a tuning pass moves the test with the numbers.
func capableFor(t *testing.T, st *store.Store, accountID string, tier simulation.Tier) {
	t.Helper()
	rung, ok := simulation.Rungs().At(tier)
	if !ok {
		t.Fatalf("tier %d is not on the ladder", tier)
	}
	for _, which := range rung.Needs.Wants() {
		if which.Shown() {
			continue
		}
		floor := rung.Needs.Of(which)
		credit := simulation.CreditHalf * floor / (100 - floor)
		if _, err := st.Pool().Exec(t.Context(), `
			INSERT INTO factor_credit (account_id, factor, credit)
			VALUES ($1, $2, $3)
			ON CONFLICT (account_id, factor) DO UPDATE SET credit = EXCLUDED.credit`,
			accountID, which.Key(), credit*1.4,
		); err != nil {
			t.Fatalf("grant %s: %v", which, err)
		}
	}

	// The money factor is the one shown floor a fresh character does not clear,
	// and tier three is the first rung that asks about it (§3.5's curve is
	// steep at the bottom and $1,200 is nowhere on it). It is not granted —
	// it is held — so the test puts it in the character's pocket.
	if rung.Needs.Asks(simulation.Wealth) {
		if _, err := st.Pool().Exec(t.Context(),
			`UPDATE character SET money_cents = $2 WHERE account_id = $1`,
			accountID, simulation.Dollars(60_000).Cents(),
		); err != nil {
			t.Fatalf("hand them the money: %v", err)
		}
	}
}
