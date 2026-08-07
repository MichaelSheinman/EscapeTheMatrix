package playbook

import (
	"strings"
	"testing"
	"testing/fstest"
)

/*
The authored vocabulary, and the validation that stands between an author and a
game nobody can play.

Two kinds of test here. The first reads what actually ships, because §3.14 makes
the vocabulary the design and a content file that stopped meaning what it says
would be a silent change to the game rather than a broken build. The second
drives `parse` over deliberately wrong content, which is the half `init` cannot
be asked about — by the time a panic could be caught, it has already happened.
*/

// What ships. Anything here failing is a content change worth noticing, not a
// test to update on the way past.
func TestTheShippedVocabularyIsWhatThePlaybookPromises(t *testing.T) {
	t.Parallel()
	v := Vocab()

	questions := v.Questions()
	if len(questions) < 4 {
		t.Fatalf("%d standing questions; §3.14's playbook is a set of them", len(questions))
	}

	ids := map[string]bool{}
	for i, q := range questions {
		if q.Order != i+1 {
			t.Errorf("%q is question %d in position %d; the numbering is what a log line cites",
				q.Key, q.Order, i+1)
		}
		if len(q.Actions()) < 2 || len(q.Conditions()) == 0 {
			t.Errorf("%q offers %d conditions and %d actions", q.Key, len(q.Conditions()), len(q.Actions()))
		}
		for _, term := range append(q.Conditions(), q.Actions()...) {
			if ids[term.ID()] {
				t.Errorf("two terms are called %q", term.ID())
			}
			ids[term.ID()] = true

			holds := strings.Contains(term.Says, Placeholder)
			if holds != term.Takes() {
				t.Errorf("%q says %q and takes(%v)", term.ID(), term.Says, term.Takes())
			}
			if term.Takes() && len(term.Values.Options()) < 2 {
				t.Errorf("%q draws from a list with %d numbers in it", term.ID(), len(term.Values.Options()))
			}
			// A posture on every answer and on no fact. The matcher compares
			// answers across questions by what they amount to, so an action
			// without one is a rule that can never be found to agree or
			// disagree with anything.
			if (term.Kind == Action) != (term.Stance != "") {
				t.Errorf("%q is a %s with stance %q", term.ID(), term.Kind, term.Stance)
			}
			if term.Stance != "" && term.Stance.Reads() == "" {
				t.Errorf("%q takes the stance %q, which has no reading", term.ID(), term.Stance)
			}
		}
		// And the question can actually be decided either way from the words a
		// player arrives with.
		if p := postures(q.actions); len(p) < 2 {
			t.Errorf("%q opens on the single posture %v", q.Key, p)
		}
	}

	// The whole point of the ticket, stated as an assertion: some of the
	// vocabulary is earned. A build in which everything started unlocked would
	// pass every other test in this file and would have spent §3.6 entirely.
	started, locked := 0, 0
	for _, q := range questions {
		for _, term := range append(q.Conditions(), q.Actions()...) {
			if term.Start {
				started++
			} else {
				locked++
			}
		}
	}
	if started == 0 || locked == 0 {
		t.Fatalf("%d terms start and %d are earned; the vocabulary grows as the player earns it (§3.14)", started, locked)
	}
}

// A term id round-trips, which is what an unlock is recorded against.
func TestATermIsFoundByTheNameAnUnlockIsRecordedAgainst(t *testing.T) {
	t.Parallel()
	v := Vocab()

	for _, q := range v.Questions() {
		for _, want := range append(q.Conditions(), q.Actions()...) {
			got, ok := v.Term(want.ID())
			if !ok || got.Key != want.Key || got.Kind != want.Kind {
				t.Fatalf("Term(%q) = %+v, %v", want.ID(), got, ok)
			}
		}
	}
	if _, ok := v.Term("help/condition/nothing"); ok {
		t.Errorf("a term nobody authored was found")
	}
	if _, ok := v.Term("nonsense"); ok {
		t.Errorf("a malformed id was resolved")
	}
}

/*
The two pools of one question are separate namespaces, and the vocabulary of one
question is not the vocabulary of another.

`voice` has both a condition and an action called "leave" — "I could walk out of
the room" and "leave the situation" — which is the case that would break a
global term table, and the reason the ids are three parts long.
*/
func TestTermKeysAreScopedToOnePoolOfOneQuestion(t *testing.T) {
	t.Parallel()
	v := Vocab()

	voice, ok := v.Question("voice")
	if !ok {
		t.Fatal("there is no question called voice")
	}
	cond, hasCond := voice.Term(Condition, "leave")
	act, hasAct := voice.Term(Action, "leave")
	if !hasCond || !hasAct {
		t.Fatalf("voice/leave: condition %v, action %v", hasCond, hasAct)
	}
	if cond.Says == act.Says {
		t.Errorf("both leaves say %q", cond.Says)
	}

	// A term that exists under another question is not a term here.
	help, _ := v.Question("help")
	if _, ok := help.Term(Condition, "watching"); ok {
		t.Errorf("help can say something only voice can say")
	}
}

/* ── what a wrong content file gets ────────────────────────────────────── */

// good is a minimal, valid content tree. Each test below breaks exactly one
// thing in it, so the failure asserted is the one the case is named after.
func good() fstest.MapFS {
	return fstest.MapFS{
		"playbook/values.yaml": &fstest.MapFile{Data: []byte(`
- key: fee
  label: paid at least
  unit: cents
  compare: at-least
  options:
    - { key: low, says: above $20, amount: 2000 }
    - { key: high, says: above $60, amount: 6000 }
- key: cap
  label: the ceiling
  unit: cents
  compare: quantity
  options:
    - { key: token, says: up to $10, amount: 1000 }
    - { key: real, says: up to $40, amount: 4000 }
`)},
		"playbook/questions/help.yaml": &fstest.MapFile{Data: []byte(`
key: help
order: 1
ask: Somebody asks you for help. When do you say yes?
note: It happens and you are not there for it.
about: someone asks me for help
conditions:
  - { key: fee, says: "they are paying me {value}", values: fee, start: true }
  - { key: legal, says: the task is legal, start: true }
actions:
  - { key: "yes", says: say yes, stance: yield, start: true }
  - { key: "no", says: say no, stance: resist, start: true }
`)},
	}
}

func broken(t *testing.T, file, content string) fstest.MapFS {
	t.Helper()
	files := good()
	files[file] = &fstest.MapFile{Data: []byte(content)}
	return files
}

func TestValidContentParses(t *testing.T) {
	t.Parallel()
	v, err := parse(good())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(v.Questions()) != 1 {
		t.Fatalf("%d questions", len(v.Questions()))
	}
}

func TestMalformedContentIsRefusedAtLoad(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, file, content, wants string
	}{
		{
			// The one that matters most in practice: a misspelt field is
			// otherwise an authored line with no effect that nobody notices.
			name: "a misspelt field",
			file: "playbook/questions/help.yaml",
			content: `
key: help
order: 1
ask: Somebody asks you for help.
note: A note.
about: someone asks me for help
conditions:
  - { key: legal, says: the task is legal, starts: true }
actions:
  - { key: "yes", says: say yes, stance: yield, start: true }
  - { key: "no", says: say no, stance: resist, start: true }
`,
			wants: "starts",
		},
		{
			name: "a term drawing from a list nobody authored",
			file: "playbook/questions/help.yaml",
			content: `
key: help
order: 1
ask: Somebody asks you for help.
note: A note.
about: someone asks me for help
conditions:
  - { key: fee, says: "they are paying me {value}", values: guesswork, start: true }
actions:
  - { key: "yes", says: say yes, stance: yield, start: true }
  - { key: "no", says: say no, stance: resist, start: true }
`,
			wants: "guesswork",
		},
		{
			name: "a placeholder with no list behind it",
			file: "playbook/questions/help.yaml",
			content: `
key: help
order: 1
ask: Somebody asks you for help.
note: A note.
about: someone asks me for help
conditions:
  - { key: fee, says: "they are paying me {value}", start: true }
actions:
  - { key: "yes", says: say yes, stance: yield, start: true }
  - { key: "no", says: say no, stance: resist, start: true }
`,
			wants: "{value}",
		},
		{
			// "Give them above $20" is what this refuses: a condition tests and
			// an action hands over, and the value lists say which they are for.
			name: "an action drawing from a list that tests",
			file: "playbook/questions/help.yaml",
			content: `
key: help
order: 1
ask: Somebody asks you for help.
note: A note.
about: someone asks me for help
conditions:
  - { key: legal, says: the task is legal, start: true }
actions:
  - { key: pay, says: "give them {value}", values: fee, stance: yield, start: true }
  - { key: "no", says: say no, stance: resist, start: true }
`,
			wants: "at-least",
		},
		{
			name: "a condition drawing from a quantity",
			file: "playbook/questions/help.yaml",
			content: `
key: help
order: 1
ask: Somebody asks you for help.
note: A note.
about: someone asks me for help
conditions:
  - { key: much, says: "it is worth {value}", values: cap, start: true }
actions:
  - { key: "yes", says: say yes, stance: yield, start: true }
  - { key: "no", says: say no, stance: resist, start: true }
`,
			wants: "quantity",
		},
		{
			// A question whose opening move is locked is a question a new
			// player cannot answer and cannot see why.
			name: "a locked first action",
			file: "playbook/questions/help.yaml",
			content: `
key: help
order: 1
ask: Somebody asks you for help.
note: A note.
about: someone asks me for help
conditions:
  - { key: legal, says: the task is legal, start: true }
actions:
  - { key: "yes", says: say yes, stance: yield }
  - { key: "no", says: say no, stance: resist, start: true }
`,
			wants: "opens on",
		},
		{
			// Without one, the matcher has no way to tell whether two answers
			// to two questions are the same answer — and the failure would be
			// at runtime, on a moment nobody is watching.
			name: "an action with no stance",
			file: "playbook/questions/help.yaml",
			content: `
key: help
order: 1
ask: Somebody asks you for help.
note: A note.
about: someone asks me for help
conditions:
  - { key: legal, says: the task is legal, start: true }
actions:
  - { key: "yes", says: say yes, start: true }
  - { key: "no", says: say no, stance: resist, start: true }
`,
			wants: "no stance",
		},
		{
			name: "a stance nobody implements",
			file: "playbook/questions/help.yaml",
			content: `
key: help
order: 1
ask: Somebody asks you for help.
note: A note.
about: someone asks me for help
conditions:
  - { key: legal, says: the task is legal, start: true }
actions:
  - { key: "yes", says: say yes, stance: shrug, start: true }
  - { key: "no", says: say no, stance: resist, start: true }
`,
			wants: "shrug",
		},
		{
			// A condition states what is so. Giving it a posture would put what
			// a rule amounts to in two places.
			name: "a condition with a stance",
			file: "playbook/questions/help.yaml",
			content: `
key: help
order: 1
ask: Somebody asks you for help.
note: A note.
about: someone asks me for help
conditions:
  - { key: legal, says: the task is legal, stance: yield, start: true }
actions:
  - { key: "yes", says: say yes, stance: yield, start: true }
  - { key: "no", says: say no, stance: resist, start: true }
`,
			wants: "only an action",
		},
		{
			// A question whose every opening answer amounts to the same thing
			// is one the player cannot decide either way on their first day.
			name: "a question that opens on one posture",
			file: "playbook/questions/help.yaml",
			content: `
key: help
order: 1
ask: Somebody asks you for help.
note: A note.
about: someone asks me for help
conditions:
  - { key: legal, says: the task is legal, start: true }
actions:
  - { key: "yes", says: say yes, stance: yield, start: true }
  - { key: once, says: say yes this once, stance: yield, start: true }
  - { key: "no", says: say no, stance: resist }
`,
			wants: "posture",
		},
		{
			name: "a question numbered zero",
			file: "playbook/questions/help.yaml",
			content: `
key: help
order: 0
ask: Somebody asks you for help.
note: A note.
about: someone asks me for help
conditions:
  - { key: legal, says: the task is legal, start: true }
actions:
  - { key: "yes", says: say yes, stance: yield, start: true }
  - { key: "no", says: say no, stance: resist, start: true }
`,
			wants: "numbered from one",
		},
		{
			name: "two questions with the same number",
			file: "playbook/questions/voice.yaml",
			content: `
key: voice
order: 1
ask: Somebody raises their voice at you.
note: A note.
about: someone raises their voice at me
conditions:
  - { key: watching, says: other people are watching, start: true }
actions:
  - { key: quiet, says: stay quiet, stance: yield, start: true }
  - { key: back, says: raise my voice back, stance: resist, start: true }
`,
			wants: "both question 1",
		},
		{
			name: "a list of one",
			file: "playbook/values.yaml",
			content: `
- key: fee
  label: paid at least
  unit: cents
  compare: at-least
  options:
    - { key: low, says: above $20, amount: 2000 }
`,
			wants: "only a decision at two",
		},
		{
			name: "numbers that do not run least first",
			file: "playbook/values.yaml",
			content: `
- key: fee
  label: paid at least
  unit: cents
  compare: at-least
  options:
    - { key: high, says: above $60, amount: 6000 }
    - { key: low, says: above $20, amount: 2000 }
`,
			wants: "least first",
		},
		{
			name: "a comparison nobody implements",
			file: "playbook/values.yaml",
			content: `
- key: fee
  label: paid at least
  unit: cents
  compare: roughly
  options:
    - { key: low, says: above $20, amount: 2000 }
    - { key: high, says: above $60, amount: 6000 }
`,
			wants: "roughly",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := parse(broken(t, c.file, c.content))
			if err == nil {
				t.Fatalf("%s parsed", c.name)
			}
			if !strings.Contains(err.Error(), c.wants) {
				t.Errorf("error = %q, want it to mention %q", err, c.wants)
			}
		})
	}
}

// A content tree with nothing in it is a failed build rather than a game with
// no playbook in it.
func TestEmptyContentIsRefused(t *testing.T) {
	t.Parallel()
	if _, err := parse(fstest.MapFS{}); err == nil {
		t.Fatal("an empty content tree parsed")
	}
}

/* ── the sentence ──────────────────────────────────────────────────────── */

// Nothing partial ever runs (§3.14). This is that rule, on its own.
func TestASentenceWithAnyBlankInItIsNotComplete(t *testing.T) {
	t.Parallel()

	full := Ref{Term: "legal"}
	cases := []struct {
		name string
		s    Sentence
		want bool
	}{
		{"one condition, filled", Sentence{When: []Ref{full}, Join: And, Then: full, Else: full}, true},
		{"two conditions, both filled", Sentence{When: []Ref{full, full}, Join: Or, Then: full, Else: full}, true},
		{"the condition is a hole", Sentence{When: []Ref{{}}, Join: And, Then: full, Else: full}, false},
		// The distinction the stored count exists for: a player who opened a
		// second slot and has not filled it is mid-edit, and their rule must
		// not fire on the first condition alone.
		{"a second condition, still a hole", Sentence{When: []Ref{full, {}}, Join: And, Then: full, Else: full}, false},
		{"no then", Sentence{When: []Ref{full}, Join: And, Else: full}, false},
		{"no otherwise", Sentence{When: []Ref{full}, Join: And, Then: full}, false},
		{"no conditions at all", Sentence{Join: And, Then: full, Else: full}, false},
		{"three conditions", Sentence{When: []Ref{full, full, full}, Join: And, Then: full, Else: full}, false},
	}
	for _, c := range cases {
		if got := c.s.Complete(); got != c.want {
			t.Errorf("%s: Complete() = %v, want %v", c.name, got, c.want)
		}
	}
}
