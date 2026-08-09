package diagnosis

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/environment"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/health"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/playbook"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/routine"
)

/*
The catalogue, and the refusals that keep §3.6's rules from being decorative.

The tests worth having here are the ones that fail the build for an authoring
mistake nobody would otherwise notice, because every one of those mistakes looks
identical from inside the game: a cause that can never be reached and a player
who has not struggled enough yet produce exactly the same screen.
*/

// The shipped content loads. It is an init() in production, so a panic here is
// what a broken file would do to the server at boot.
func TestTheAuthoredCataloguesLoad(t *testing.T) {
	t.Parallel()
	if len(Causes()) == 0 {
		t.Fatal("no causes are authored, so nothing could ever be named")
	}
	for _, c := range Causes() {
		if !c.Grants() {
			t.Errorf("%q unlocks nothing, and a conclusion with no consequence is not one", c.Key)
		}
	}
}

/*
Every routine has a cause that unlocks it.

The gate is total: with #32 wired, a routine nobody can name their way to is a
routine no player will ever configure, and the `Learn` sentence on it would be a
promise the content cannot keep. This is the test that catches a fifth routine
being added without an account of what would earn it.
*/
func TestEveryRoutineCanBeReached(t *testing.T) {
	t.Parallel()
	unlocked := map[string]bool{}
	for _, c := range Causes() {
		if c.Routine != "" {
			unlocked[c.Routine] = true
		}
	}
	for _, k := range routine.Kinds {
		if !unlocked[k.Key] {
			t.Errorf("no authored cause unlocks %q, so nobody could ever configure it", k.Key)
		}
	}
}

/*
Every authored cause speaks from evidence the bound ledgers can produce.

The real check runs at boot in internal/api with the ledgers it wires. This one
runs it against the two modules that exist, which is the same list — and it
fails here, in a package test, rather than in whichever handler first noticed
that a cause was unreachable.
*/
func TestEveryCauseSpeaksFromSomethingReal(t *testing.T) {
	t.Parallel()
	if err := Check(append(health.Observable(), playbook.Observable()...)); err != nil {
		t.Fatal(err)
	}
}

// And a misspelt key is refused, so the check above is doing something.
func TestACauseThatSpeaksFromNothingIsRefused(t *testing.T) {
	t.Parallel()
	if err := Check([]string{"health/short-nights"}); err == nil {
		t.Fatal("a catalogue whose evidence keys nothing produces was accepted")
	}
}

/*
More than one cause speaks from the same observation.

§3.6's fourth rule — "allow multiple valid causes; several diagnoses can be
partly right" — is the one rule of the five that cannot be enforced by a
refusal, because it is a property of how the content overlaps rather than of any
one file. So it is asserted: a catalogue in which every observation supports
exactly one cause would make the mechanic a lookup table with a conversation
drawn on it, and this is the test that notices somebody tidying it into one.
*/
func TestTheSameEvidenceSupportsMoreThanOneCause(t *testing.T) {
	t.Parallel()
	speaks := map[string][]string{}
	for _, c := range Causes() {
		for _, k := range c.Speaks {
			speaks[k] = append(speaks[k], c.Key)
		}
	}
	for _, of := range speaks {
		if len(of) > 1 {
			return
		}
	}
	t.Fatal("every observation supports exactly one cause, which makes naming one a lookup rather than a judgement")
}

/*
At least one cause hands over words and no routine.

The other half of "tools of differing usefulness". A catalogue of four causes
that each unlock one of the four routines is a four-way switch, and the player
learns to read it as one.
*/
func TestSomethingIsLearntThatIsNotARoutine(t *testing.T) {
	t.Parallel()
	for _, c := range Causes() {
		if c.Routine == "" && len(c.Terms) > 0 {
			return
		}
	}
	t.Fatal("every cause unlocks a routine, so a diagnosis is a way of choosing one rather than a way of understanding something")
}

/*
Nothing a player reads names a stage, a register or a colour (§3.15).

The same line internal/environment holds its own writing to, held here because a
diagnosis is the most tempting screen in the game to break it on: it is the one
place explicitly about the player understanding something, which is exactly where
an author reaches for the word.
*/
func TestNothingInTheCatalogueIsLabelled(t *testing.T) {
	t.Parallel()
	for _, c := range Causes() {
		for _, text := range []string{c.Says, c.Because} {
			if word := environment.Labelled(text); word != "" {
				t.Errorf("cause %q says %q", c.Key, word)
			}
		}
	}
	for _, env := range environment.All() {
		s, ok := Staged(env.Key)
		if !ok {
			t.Errorf("nobody in %q stages a diagnosis", env.Key)
			continue
		}
		for _, text := range []string{s.Opens, s.Asks, s.Weighs, s.Closes} {
			if word := environment.Labelled(text); word != "" {
				t.Errorf("scene %q says %q", env.Key, word)
			}
		}
	}
}

/*
No scene line tells the player they were right or wrong.

§3.6's second rule is the one this mechanic dies on, and it dies quietly: a
`closes` line reading "that's exactly it" would pass every other test in this
file and turn the whole thing into a quiz with a warm voice. So the words are
checked, and the list is short and blunt on purpose.
*/
func TestNoSceneConfirmsAnything(t *testing.T) {
	t.Parallel()
	verdicts := []string{
		"correct", "incorrect", "right answer", "wrong answer",
		"exactly it", "that's it", "well done", "good guess", "try again",
	}
	for _, env := range environment.All() {
		s, ok := Staged(env.Key)
		if !ok {
			continue
		}
		said := strings.ToLower(strings.Join([]string{s.Opens, s.Asks, s.Weighs, s.Closes}, " "))
		for _, v := range verdicts {
			if strings.Contains(said, v) {
				t.Errorf("scene %q says %q, and the game never tells anybody whether they were right", env.Key, v)
			}
		}
	}
}

/*
A scene staged with somebody who does not live there is refused.

The check that keeps the conversation inside §3.15's world. A person key that no
longer matches the population — because somebody was renamed in one file and not
the other — would otherwise ship as a scene with a blank name in it.
*/
func TestASceneWithAStrangerIsRefused(t *testing.T) {
	t.Parallel()
	_, _, err := parse(fstest.MapFS{
		"diagnoses/causes/a.yaml": &fstest.MapFile{Data: []byte(
			"says: something\nbecause: something else\nspeaks-from: [health/short-nights]\nunlocks:\n  routine: sleep\n")},
		"diagnoses/east-side/scene.yaml": &fstest.MapFile{Data: []byte(
			"who: nobody-by-that-name\nopens: a\nasks: b\nweighs: c\ncloses: d\n")},
	})
	if err == nil || !strings.Contains(err.Error(), "does not live there") {
		t.Fatalf("a scene staged with a stranger was accepted: %v", err)
	}
}

// A cause that hands nothing over is refused.
func TestACauseThatGrantsNothingIsRefused(t *testing.T) {
	t.Parallel()
	_, _, err := parse(fstest.MapFS{
		"diagnoses/causes/a.yaml": &fstest.MapFile{Data: []byte(
			"says: something\nbecause: something else\nspeaks-from: [health/short-nights]\n")},
	})
	if err == nil || !strings.Contains(err.Error(), "unlocks nothing") {
		t.Fatalf("a cause with no consequence was accepted: %v", err)
	}
}

// A population with nowhere to have the conversation is refused.
func TestAPopulationWithNoSceneIsRefused(t *testing.T) {
	t.Parallel()
	_, _, err := parse(fstest.MapFS{
		"diagnoses/causes/a.yaml": &fstest.MapFile{Data: []byte(
			"says: something\nbecause: something else\nspeaks-from: [health/short-nights]\nunlocks:\n  routine: sleep\n")},
	})
	if err == nil || !strings.Contains(err.Error(), "stages a diagnosis") {
		t.Fatalf("a population nobody could be diagnosed in was accepted: %v", err)
	}
}

// A misspelt field is a failed build rather than a silently ignored line (§5.2).
func TestAMisspeltFieldIsRefused(t *testing.T) {
	t.Parallel()
	_, _, err := parse(fstest.MapFS{
		"diagnoses/causes/a.yaml": &fstest.MapFile{Data: []byte(
			"say: something\nbecause: something else\nspeaks-from: [x]\n")},
	})
	if err == nil {
		t.Fatal("a cause with a misspelt field was accepted")
	}
}
