package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/playbook"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
)

/*
The playbook at the edge it is allowed to reach (§3.14).

The composition and the storage are proved in internal/playbook. What is worth
proving here is what neither of those can: that what actually goes over the wire
to a browser carries the sentence, the vocabulary this player has, and **not one
word of the vocabulary they have not** — and that there is no route by which a
client could send prose, save a draft, or name a term the server did not just
offer it.
*/

// snapshotPlaybook reads the playbook out of a world snapshot.
func snapshotPlaybook(t *testing.T, s *Server, as *http.Cookie) playbookView {
	t.Helper()

	rec := read(t, s, as, worldPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var out struct {
		Playbook playbookView `json:"playbook"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not a world snapshot: %v", err)
	}
	return out.Playbook
}

func ruleFor(t *testing.T, view playbookView, environment, question string) ruleView {
	t.Helper()
	for _, set := range view.Sets {
		if set.Environment != environment {
			continue
		}
		for _, r := range set.Rules {
			if r.Question == question {
				return r
			}
		}
	}
	t.Fatalf("no rule for %s/%s", environment, question)
	return ruleView{}
}

// What a client is hydrated with: a set per environment and a whole vocabulary,
// none of it answered yet.
func TestTheWorldSnapshotCarriesAPlaybookPerEnvironment(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	view := snapshotPlaybook(t, s, as)

	if len(view.Sets) != len(districts{}.Environments()) {
		t.Fatalf("%d rule sets, want one per environment (§3.15)", len(view.Sets))
	}
	if len(view.Questions) != len(playbook.Vocab().Questions()) {
		t.Fatalf("%d questions on the wire", len(view.Questions))
	}
	for _, q := range view.Questions {
		if q.N < 1 || q.Ask == "" || q.About == "" {
			t.Errorf("question %q arrived as %+v", q.Key, q)
		}
		if len(q.Conditions) == 0 || len(q.Actions) < 2 {
			t.Errorf("%q offers %d conditions and %d actions", q.Key, len(q.Conditions), len(q.Actions))
		}
	}
	for _, set := range view.Sets {
		if set.Name == "" {
			t.Errorf("the %q set has no name", set.Environment)
		}
		for _, r := range set.Rules {
			if r.InForce {
				t.Errorf("%s/%s is in force before anybody answered it", set.Environment, r.Question)
			}
		}
	}
}

/*
The strongest assertion available: the whole payload, read as text.

Not the struct — the bytes. A field added carelessly to a view, a shared table of
value lists, a debug dump, anything at all that let a term this player has not
earned across the boundary would be caught here rather than by somebody reading
a JSON tab. §3.14's terms are not achievements to be shown greyed out: what is
withheld is that the word exists at all.
*/
func TestNoLockedTermIsNamedAnywhereInTheWorldSnapshot(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	rec := read(t, s, as, worldPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()

	locked := 0
	for _, q := range playbook.Vocab().Questions() {
		for _, term := range append(q.Conditions(), q.Actions()...) {
			if term.Start {
				continue
			}
			locked++
			if contains(body, term.Says) {
				t.Errorf("%q says %q, which appears in the world snapshot", term.ID(), term.Says)
			}
		}
	}
	if locked == 0 {
		t.Fatal("nothing is locked, so this test proved nothing (§3.6)")
	}
}

// The command, end to end: a sentence composed is a sentence in force, with
// nothing else to press.
func TestComposingARuleIsTheChange(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	rec := post(t, s, as, "playbook.compose", "key-1", `{
		"environment": "east-side",
		"question": "help",
		"when": [{"term":"fee","value":"fair"}],
		"join": "and",
		"then": {"term":"yes"},
		"otherwise": {"term":"no"}
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var out composeResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not a compose result: %v", err)
	}
	got := ruleFor(t, out.Playbook, "east-side", "help")
	if !got.InForce || got.When[0] == nil || got.When[0].Value != "fair" {
		t.Fatalf("rule = %+v", got)
	}
	// And the next page load says the same thing, with no save in between.
	if again := ruleFor(t, snapshotPlaybook(t, s, as), "east-side", "help"); !again.InForce {
		t.Error("the rule is not in force on the next read")
	}
	// The other sets are untouched (§3.15).
	if ruleFor(t, out.Playbook, "downtown", "help").InForce {
		t.Error("answering on the east side answered the question downtown too")
	}
}

// Every refusal, and the status code that says which kind it is (§8).
func TestTheComposeCommandRefusesAnythingOutsideTheVocabulary(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	cases := []struct {
		name, body string
		want       int
	}{
		{"a term nobody authored", `{"environment":"east-side","question":"help",
			"when":[{"term":"vibes"}],"then":{"term":"yes"},"otherwise":{"term":"no"}}`,
			http.StatusUnprocessableEntity},
		{"a term belonging to another question", `{"environment":"east-side","question":"help",
			"when":[{"term":"watching"}],"then":{"term":"yes"},"otherwise":{"term":"no"}}`,
			http.StatusUnprocessableEntity},
		{"a number outside the closed list", `{"environment":"east-side","question":"help",
			"when":[{"term":"fee","value":"above $137"}],"then":{"term":"yes"},"otherwise":{"term":"no"}}`,
			http.StatusUnprocessableEntity},
		{"a third condition", `{"environment":"east-side","question":"help",
			"when":[{"term":"legal"},{"term":"legal"},{"term":"legal"}],
			"then":{"term":"yes"},"otherwise":{"term":"no"}}`,
			http.StatusUnprocessableEntity},
		{"an environment that is not one", `{"environment":"atlantis","question":"help",
			"when":[{"term":"legal"}],"then":{"term":"yes"},"otherwise":{"term":"no"}}`,
			http.StatusUnprocessableEntity},
		{"a question the world does not ask", `{"environment":"east-side","question":"love",
			"when":[{"term":"legal"}],"then":{"term":"yes"},"otherwise":{"term":"no"}}`,
			http.StatusUnprocessableEntity},
		// The one that is not the request's fault: the term exists and the
		// player has not been through what unlocks it (§3.6), so the same
		// command works later. That is a 409, not a 422.
		{"a term this player has not earned", `{"environment":"east-side","question":"help",
			"when":[{"term":"useful"}],"then":{"term":"yes"},"otherwise":{"term":"no"}}`,
			http.StatusConflict},
	}

	for i, c := range cases {
		rec := post(t, s, as, "playbook.compose", composeKey(i), c.body)
		if rec.Code != c.want {
			t.Errorf("%s: status = %d, want %d (body %s)", c.name, rec.Code, c.want, rec.Body)
		}
	}
}

// composeKey is a fresh idempotency key per case, so a refusal is not replayed.
func composeKey(i int) string { return "playbook-" + string(rune('a'+i)) }

/*
There is no save, and there is no draft.

Four plausible names, all 404. §3.14's loop runs while the world runs, so a
commit step would only be honest if the player could sit and think in a paused
world — and the blank is what replaces it. If a later ticket adds any of these,
it is a design change and this test is where the argument has to be had.
*/
func TestThereIsNoWayToSaveOrDiscardAPlaybookDraft(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	for _, name := range []string{"playbook.save", "playbook.commit", "playbook.publish", "playbook.discard"} {
		rec := post(t, s, as, name, "key-"+name, `{}`)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s answered %d; editing the sentence is the change", name, rec.Code)
		}
	}
}

/*
Nothing on this surface takes prose.

Asserted as a shape rather than as a filter: every string field of the command's
input is a key, and a value that did not come out of the vocabulary the server
just sent is refused. The nearest thing to free text a client could try is a
sentence in the term field, and it is refused for being a term nobody authored
rather than for looking like prose.
*/
func TestNoFieldOfTheComposeCommandAcceptsFreeText(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	rec := post(t, s, as, "playbook.compose", "prose-1", `{
		"environment": "east-side",
		"question": "help",
		"when": [{"term":"if they seem like they deserve it, honestly"}],
		"then": {"term":"be a good person about it"},
		"otherwise": {"term":"no"}
	}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	// And nothing was written: a rule half-composed from prose is worse than a
	// refusal, because it would look like it worked.
	if ruleFor(t, snapshotPlaybook(t, s, as), "east-side", "help").InForce {
		t.Error("a refused command left a rule in force")
	}
}

/* ── the log (§3.14's other half) ──────────────────────────────────────── */

/*
What the rules did while the player was elsewhere.

The matcher and the records are proved in internal/playbook. What is worth
proving at this edge is the same thing the vocabulary's tests prove: that the
payload carries the sentence and the attribution a player needs to argue with,
and **not one number or posture the matcher decided on** — because a client
holding either could work out what the server was going to do, and §10.2 says the
client computes no game rules.
*/

// onlyAccount is the one player signedIn made.
func onlyAccount(t *testing.T, st *store.Store) string {
	t.Helper()
	var id string
	if err := st.Pool().QueryRow(t.Context(), `SELECT id FROM account LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("read account: %v", err)
	}
	return id
}

/*
aRecordedMoment composes a rule, puts a moment to it and writes the record.

Directly through the module rather than over a command route, because there is
no command that fires a rule and there is not supposed to be: situations arise
while the player is elsewhere (§3.14), and the thing that generates them is #30.
*/
func aRecordedMoment(t *testing.T, s *Server, st *store.Store, fee int64, outcome playbook.Outcome) {
	t.Helper()
	account := onlyAccount(t, st)
	ctx := t.Context()

	// No t.Fatalf inside the closure: a Goexit skips the rollback and holds the
	// connection until the pool gives up.
	if err := s.store.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := s.playbooks.Compose(ctx, tx, account, "east-side", "help", playbook.Sentence{
			When: []playbook.Ref{{Term: "fee", Value: "fair"}},
			Join: playbook.And,
			Then: playbook.Ref{Term: "yes"},
			Else: playbook.Ref{Term: "no"},
		}); err != nil {
			return err
		}
		moment := playbook.Arises("a-favour", "help").Reading("help/condition/fee", fee)
		d, err := s.playbooks.Weigh(ctx, tx, account, "east-side", moment)
		if err != nil {
			return err
		}
		_, err = s.playbooks.Record(ctx, tx, account, s.clk.Now(), "east-side", d, outcome)
		return err
	}); err != nil {
		t.Fatalf("record a moment: %v", err)
	}
}

func logAt(t *testing.T, s *Server, as *http.Cookie, page int) logView {
	t.Helper()
	at := logPath
	if page != 0 {
		at += "?page=" + strconv.Itoa(page)
	}
	rec := read(t, s, as, at)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var out logView
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not a log: %v", err)
	}
	return out
}

// The log carries the sentence, which condition matched, and what was done —
// §3.14's "the player can see exactly which condition matched and which action
// fired", as fields rather than as a claim.
func TestTheLogSaysWhichRuleFiredAndWhatItDid(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)

	aRecordedMoment(t, s, st, 500, playbook.Poor)

	out := logAt(t, s, as, 0)
	if out.Total != 1 || len(out.Fires) != 1 || out.Size != playbook.LogPage {
		t.Fatalf("log = %+v", out)
	}
	f := out.Fires[0]
	if f.Situation != "a-favour" || f.Environment != "east-side" ||
		f.Verdict != "answered" || f.Outcome != "poor" || f.Note != "" {
		t.Fatalf("record = %+v", f)
	}
	if len(f.Considered) != 1 {
		t.Fatalf("%d questions weighed", len(f.Considered))
	}
	c := f.Considered[0]
	if c.N != 1 || c.State != "fired" || c.Held != "unmet" {
		t.Errorf("considered = %+v", c)
	}
	if len(c.Met) != 1 || c.Met[0] != "unmet" {
		t.Errorf("met = %v; the log knows which condition matched", c.Met)
	}
	if c.Says != "when they are paying me above $60 then say yes otherwise say no" || c.Did != "say no" {
		t.Errorf("says %q, did %q", c.Says, c.Did)
	}

	// And the week, counted — §3.6's pattern beside the moments that make it up.
	if len(out.Trends) != 1 || out.Trends[0].Question != "help" ||
		out.Trends[0].Fired != 1 || out.Trends[0].Poor != 1 {
		t.Errorf("trends = %+v", out.Trends)
	}
	if out.Since == "" {
		t.Error("the window is not stated; a screen would have to guess what a trend covers")
	}
}

/*
Nothing on this wire says what the matcher compared or what it thought an answer
amounted to.

The same veil the vocabulary has, on the other surface: a threshold's amount and
an action's stance are the two things a client could compute the next decision
from, and neither has a field to arrive in. Where a posture is the *point* — two
answers that disagreed — the server says so in a sentence.
*/
func TestTheLogCarriesNoAmountAndNoPosture(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)

	aRecordedMoment(t, s, st, 9000, playbook.Good)

	rec := read(t, s, as, logPath)
	body := rec.Body.String()
	for _, secret := range []string{
		// The amount behind "above $60", and every stance key there is.
		"6000", "yield", "resist", "bargain", "withdraw",
	} {
		if strings.Contains(body, secret) {
			t.Errorf("the log names %q:\n%s", secret, body)
		}
	}
}

// Behind the same door as everything else that is per-player.
func TestTheLogIsBehindTheDoor(t *testing.T) {
	t.Parallel()
	s, _ := newServer(t)

	if rec := read(t, s, nil, logPath); rec.Code != http.StatusUnauthorized {
		t.Errorf("GET %s with no session = %d, want 401", logPath, rec.Code)
	}
}

// A page that is not a number is refused; one past the end is answered with the
// last, the way a board page is.
func TestANonsenseLogPageIsRefusedAndAPagePastTheEndIsClamped(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)

	aRecordedMoment(t, s, st, 9000, playbook.Pending)

	if rec := read(t, s, as, logPath+"?page=soon"); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("page=soon = %d, want 422", rec.Code)
	}
	if out := logAt(t, s, as, 99); out.Page != 1 || len(out.Fires) != 1 {
		t.Errorf("page 99 answered as page %d with %d records", out.Page, len(out.Fires))
	}
}

/*
The log is not in the world snapshot, and a test says so.

§10.2 hydrates the client's world model once and keeps it current over the
socket, which works because a world model is bounded. History is not: a year of
moments in the snapshot would put the last of them out of reach forever. So this
is the second pull-shaped surface after the leaderboards, for the same reason.
*/
func TestTheWorldSnapshotDoesNotCarryTheLog(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)

	aRecordedMoment(t, s, st, 9000, playbook.Good)

	rec := read(t, s, as, worldPath)
	var snapshot map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("response is not a world snapshot: %v", err)
	}
	for _, key := range []string{"log", "fires", "history"} {
		if _, ok := snapshot[key]; ok {
			t.Errorf("the world snapshot carries %q; history is fetched, not hydrated", key)
		}
	}
	if strings.Contains(rec.Body.String(), "a-favour") {
		t.Error("the world snapshot carries a recorded moment")
	}
}
