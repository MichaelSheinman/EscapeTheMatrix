package challenge

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
`dialogue_branch`, as a type rather than as either of the challenges on it.

§5.2 gives a mechanic type "a typed config schema", and a schema is only worth
having for what it refuses. Everything below is a way of authoring a dialogue
that would load, run, and put a player in front of a blank line — a scene with
nowhere to be, a question nobody asked, a choice with no account of how it
landed — and every one of them is a failed build instead (§5.2: "production
validates at startup so a malformed challenge fails in CI, not at 2am").

What is deliberately *not* re-checked here is the graph. Reachability, cycles
and the outcome bounds belong to [Plan.check] and are the framework's for every
mechanic; a type that checked them a second time would be a second opinion, and
two opinions about one rule is how they start disagreeing.
*/

// dialogueIn configures a dialogue out of a directory holding these documents.
func dialogueIn(files map[string]string) (Configured, error) {
	dir := fstest.MapFS{spec: &fstest.MapFile{Data: []byte("name: x\nmechanic: dialogue_branch\n")}}
	for name, body := range files {
		dir[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return branching{}.Configure(dir)
}

// smallest is the least a dialogue can be and still be a challenge: a committed
// judgement, two ways out of it, and an outcome for each.
const smallest = `
scenes:
  - key: the-steps
    where: The steps, a quarter to seven
    beats:
      - line: They stop talking when they see you.
      - who: Marisol
        line: You went there on your own.
    ask: There is a way of coming into somebody else's argument.
    options:
      - key: sit
        text: Sit down on the step below them.
        taken: She moves her bag to make room.
        weight: 2
        moves:
          kindness: 30
      - key: walk-on
        text: Go up the inside stairs.
        taken: Neither of them says anything as you pass.
        weight: -1
outcomes:
  - key: talked-past
    at-least: -1
    reaction: It starts again below you before you have your door open.
  - key: heard
    at-least: 2
    reaction: It goes quiet about ten minutes after you leave.
`

func TestTheSmallestDialogueLoads(t *testing.T) {
	made, err := dialogueIn(map[string]string{script: smallest})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	plan := made.Plan()
	if err := plan.check(); err != nil {
		t.Fatalf("the framework refused it: %v", err)
	}
	// The opening is optional and defaults to the first authored scene, which
	// is what reading a script from the top means.
	if plan.Opening != "the-steps" {
		t.Errorf("Opening = %q, want the first scene", plan.Opening)
	}
	if len(plan.Scenes) != 1 || len(plan.Scenes[0].Options) != 2 {
		t.Fatalf("plan = %+v", plan.Scenes)
	}
	if got := plan.Scenes[0].Options[0].Deltas; len(got) != 1 ||
		got[0] != (simulation.Grant{Factor: simulation.Kindness, Credit: 30}) {
		t.Errorf("deltas = %+v", got)
	}
}

// An explicit opening is honoured, so that a script can be reordered without
// the run silently starting somewhere else.
func TestAnOpeningMayBeNamed(t *testing.T) {
	made, err := dialogueIn(map[string]string{script: "opening: second\n" + strings.Replace(smallest,
		"  - key: the-steps", "  - key: second", 1)})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if got := made.Plan().Opening; got != "second" {
		t.Errorf("Opening = %q, want the named scene", got)
	}
}

// §5.2: a challenge is "mechanic type + config document + content files", and a
// challenge of this type with no config document is a name and nothing else.
func TestADialogueWithNoDocumentIsAFailedBuild(t *testing.T) {
	_, err := dialogueIn(nil)
	if err == nil || !strings.Contains(err.Error(), script) {
		t.Fatalf("Configure = %v, want a refusal naming %s", err, script)
	}
}

// A file beside a mechanic that never opens one is content somebody wrote and
// nobody runs — invisible in a diff, and looking authored.
func TestADocumentNobodyReadsIsAFailedBuild(t *testing.T) {
	_, err := dialogueIn(map[string]string{script: smallest, "characters.yaml": "marisol: {}\n"})
	if err == nil || !strings.Contains(err.Error(), "characters.yaml") {
		t.Fatalf("Configure = %v, want a refusal naming the file nothing reads", err)
	}
}

// Strict decoding: a misspelt field is a failed build rather than a line of
// content silently ignored. `atleast` is the one an author will actually type.
func TestAFieldTheSchemaDoesNotHaveIsAFailedBuild(t *testing.T) {
	_, err := dialogueIn(map[string]string{script: strings.Replace(smallest, "at-least:", "atleast:", 1)})
	if err == nil || !strings.Contains(err.Error(), "atleast") {
		t.Fatalf("Configure = %v, want a refusal naming the field", err)
	}
}

/*
The words a challenge cannot be missing.

Each of these loads happily as YAML, produces a plan the framework would accept,
and shows a player nothing. They are the reason this type has a schema at all.
*/
func TestAScriptWithNothingToSayIsRefused(t *testing.T) {
	for _, tc := range []struct {
		what   string
		change func(string) string
		says   string
	}{
		{"a scene with nowhere to be", func(s string) string {
			return strings.Replace(s, "    where: The steps, a quarter to seven\n", "", 1)
		}, "says nowhere"},
		{"a scene with nobody in it", func(s string) string {
			return strings.Replace(s, `    beats:
      - line: They stop talking when they see you.
      - who: Marisol
        line: You went there on your own.
`, "", 1)
		}, "nobody in it"},
		{"a beat with no line", func(s string) string {
			return strings.Replace(s, "      - who: Marisol\n        line: You went there on your own.\n",
				"      - who: Marisol\n", 1)
		}, "no line in it"},
		{"a scene that asks nothing", func(s string) string {
			return strings.Replace(s, "    ask: There is a way of coming into somebody else's argument.\n", "", 1)
		}, "asks nothing"},
		{"a scene that offers nothing", func(s string) string {
			return s[:strings.Index(s, "    options:")] + "outcomes:" + s[strings.Index(s, "\n  - key: talked-past"):]
		}, "offers nothing"},
		{"a choice with nothing written on it", func(s string) string {
			return strings.Replace(s, "        text: Sit down on the step below them.\n", "", 1)
		}, "nothing written on it"},
		{"a choice with no account of how it landed", func(s string) string {
			return strings.Replace(s, "        taken: She moves her bag to make room.\n", "", 1)
		}, "how it was taken"},
		{"an outcome with no reaction", func(s string) string {
			return strings.Replace(s, "    reaction: It goes quiet about ten minutes after you leave.\n", "", 1)
		}, "no reaction"},
		{"a scene with no key", func(s string) string {
			return strings.Replace(s, "  - key: the-steps", "  - key: \"\"", 1)
		}, "is empty"},
		{"a key that is not a key", func(s string) string {
			return strings.Replace(s, "      - key: sit", "      - key: Sit Down", 1)
		}, "lower case"},
	} {
		t.Run(tc.what, func(t *testing.T) {
			_, err := dialogueIn(map[string]string{script: tc.change(smallest)})
			if err == nil {
				t.Fatalf("Configure = nil, want a refusal for %s", tc.what)
			}
			if !strings.Contains(err.Error(), tc.says) {
				t.Errorf("Configure = %q, want it to say %q", err, tc.says)
			}
		})
	}
}

/*
§3.5's three granted factors, and the two that are not.

Health and Money are read from the world rather than awarded, and an authored
`moves: {health: 10}` is an author expecting something that will never happen.
[Plan.check] refuses it too, for every mechanic; this refuses it in the words of
the thing being authored, and names what a challenge may actually move.
*/
func TestAChallengeMovesOnlyTheHiddenFactors(t *testing.T) {
	for _, tc := range []struct{ name, says string }{
		{"health", "read from the world"},
		{"money", "read from the world"},
		{"charisma", "is not a factor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := dialogueIn(map[string]string{
				script: strings.Replace(smallest, "          kindness: 30", "          "+tc.name+": 30", 1)})
			if err == nil {
				t.Fatalf("Configure = nil, want a refusal")
			}
			if !strings.Contains(err.Error(), tc.says) {
				t.Errorf("Configure = %q, want it to say %q", err, tc.says)
			}
			for _, f := range simulation.Factors {
				if f.Granted() && !strings.Contains(err.Error(), f.Key()) {
					t.Errorf("the refusal does not name %s, which a challenge may move", f.Key())
				}
			}
		})
	}
}

/*
§7: the same passage always produces the same deltas.

`moves` is a YAML mapping and Go's map iteration is deliberately random, so an
option's grants are emitted in the simulation's own order rather than in
whatever order the runtime handed them over. Without that, two boots of the same
binary would hand internal/factor the same grants in different orders — which is
determinism lost to a detail of the language rather than to a decision.
*/
func TestGrantsComeOutInTheSimulationsOrder(t *testing.T) {
	body := strings.Replace(smallest, "          kindness: 30",
		"          kindness: 30\n          stability: 20\n          intelligence: 10", 1)

	want := []simulation.Grant{
		{Factor: simulation.Intelligence, Credit: 10},
		{Factor: simulation.Stability, Credit: 20},
		{Factor: simulation.Kindness, Credit: 30},
	}
	for range 8 {
		made, err := dialogueIn(map[string]string{script: body})
		if err != nil {
			t.Fatalf("Configure: %v", err)
		}
		got := made.Plan().Scenes[0].Options[0].Deltas
		if len(got) != len(want) {
			t.Fatalf("deltas = %+v", got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("deltas = %+v, want %+v", got, want)
			}
		}
	}
}

/*
A run standing at a scene the content no longer has says nothing, rather than
saying something invented.

The ordinary case for it is an author deleting a scene somebody is part way
through, which is a thing that will happen in development every week and in
production whenever a challenge is edited between two sittings.
*/
func TestAScriptSaysNothingAboutASceneItNoLongerHas(t *testing.T) {
	made, err := dialogueIn(map[string]string{script: smallest})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	d := Definition{Code: "resolve_conflict/a", Mechanic: dialogueBranch, Plan: made.Plan(), Made: made}

	if _, ok := Say(Run{Challenge: d, Scene: Scene{Key: "the-kitchen"}}); ok {
		t.Fatal("the mechanic had something to say about a scene it does not have")
	}
	if _, ok := Say(Run{Challenge: d, Scene: d.Plan.Scenes[0]}); !ok {
		t.Fatal("the mechanic says nothing about a scene it does have")
	}

	// And a definition nothing configured — a challenge withdrawn from content
	// with a run still pointing at it — is the same answer for the same reason.
	if _, ok := Say(Run{Challenge: Definition{Code: "gone/a"}, Scene: d.Plan.Scenes[0]}); ok {
		t.Fatal("a challenge with no mechanic behind it still narrated itself")
	}
}
