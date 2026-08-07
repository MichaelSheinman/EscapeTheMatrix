package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/playbook"
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
