package conflict

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/environment"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/playbook"
)

/*
The loader, and the refusals that are the design rather than hygiene.

technical.md §5.2 puts the argument for authored content in one sentence —
"production validates at startup so a malformed challenge fails in CI, not at
2am" — and every case below is one of the ways a population's pathways can be
wrong. They are tests rather than trust for the reason [parse] is a function
over an fs.FS and a list of populations: an init that has already run cannot be
caught, so these paths would otherwise only be exercised by breaking the shipped
content.
*/

// A pathway that parses, and the base every broken case below is a copy of.
const sound = `
key: the-thing
among: somewhere
follows: owing
name: The thing
line: >-
  {who} mentions it on the way past, and does not have to say the rest.
puts: help
facts:
  - { term: help/condition/legal, so: true }
  - { term: help/condition/fee, reading: 0 }
  - { term: help/condition/short, reading: 60 }
  - { term: help/condition/owe, from: owing }
raises: { asked: 1 }
answers:
  - stance: yield
    did: Say yes.
    came: It takes an hour and nothing else is said about it.
    outcome: good
    moves: { kindness: 6 }
    shifts: { owed: 1 }
  - stance: bargain
    did: Say yes, for something back.
    came: It is agreed, and both halves happen.
    outcome: mixed
    moves: { intelligence: 6 }
  - stance: resist
    did: Say no.
    came: It is taken well and not raised again.
    outcome: poor
    moves: { kindness: -6 }
    shifts: { slighted: 1 }
`

// where is the one population the cases below are authored for.
var where = []string{"somewhere"}

/*
whole is a pool with something for every pressure, since the loader refuses a
population that cannot answer one.

Built out of copies of [sound] rather than written six times, because what these
cases are about is one broken file at a time and a hand-written pool would put
five more chances to be wrong behind each of them.
*/
func whole(t *testing.T, broken ...string) fstest.MapFS {
	t.Helper()
	out := fstest.MapFS{}
	for i, strain := range Strains {
		body := strings.ReplaceAll(sound, "key: the-thing", fmt.Sprintf("key: pathway-%d", i))
		body = strings.ReplaceAll(body, "follows: owing", "follows: "+string(strain))
		out[fmt.Sprintf("%s/somewhere/pathway-%d.yaml", conflictsPath, i)] = &fstest.MapFile{Data: []byte(body)}
	}
	for i, body := range broken {
		name := "broken"
		if head := strings.Index(body, "key: "); head >= 0 {
			name = strings.TrimSpace(body[head+5 : head+5+strings.Index(body[head+5:], "\n")])
		}
		out[fmt.Sprintf("%s/somewhere/%s.yaml", conflictsPath, name)] = &fstest.MapFile{Data: []byte(body)}
		_ = i
	}
	return out
}

// bad is [sound] with one thing changed, under a name of its own so the pool
// stays whole around it.
func bad(was, now string) string {
	return strings.ReplaceAll(strings.ReplaceAll(sound, "key: the-thing", "key: broken"), was, now)
}

func TestASoundPoolLoads(t *testing.T) {
	all, err := parse(whole(t), where)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(all) != len(Strains) {
		t.Fatalf("%d pathways, want %d", len(all), len(Strains))
	}
	for _, strain := range Strains {
		if len(pool(all, strain)) != 1 {
			t.Errorf("nothing follows from %q", strain)
		}
	}
}

func pool(all []Authored, strain Strain) []Authored {
	out := []Authored{}
	for _, a := range all {
		if a.Follows == strain {
			out = append(out, a)
		}
	}
	return out
}

/*
The checks that are §3.15's content cost rather than YAML hygiene.

"The pool per environment has to be large enough that repetition reads as the
same kind of person doing the same kind of thing rather than as the game running
out" is a sentence about scoping, and these two are what make it a build failure
instead of a note in a document.
*/
func TestAPopulationMustAnswerEveryPressure(t *testing.T) {
	// One spare, so that removing a pressure leaves a pool that is still deep
	// enough — otherwise this would be the floor check wearing a different name.
	files := whole(t, bad("key: broken", "key: spare"))
	delete(files, conflictsPath+"/somewhere/pathway-1.yaml")
	_, err := parse(files, where)
	if err == nil || !strings.Contains(err.Error(), string(Strains[1])) {
		t.Fatalf("a population with a hole in its pool loaded: %v", err)
	}
}

func TestAPoolBelowTheFloorIsRefused(t *testing.T) {
	// Every strain covered and still too few, which is the case a coverage
	// check alone would miss.
	files := fstest.MapFS{}
	for i, strain := range Strains[:2] {
		body := strings.ReplaceAll(sound, "key: the-thing", fmt.Sprintf("key: pathway-%d", i))
		body = strings.ReplaceAll(body, "follows: owing", "follows: "+string(strain))
		files[fmt.Sprintf("%s/somewhere/pathway-%d.yaml", conflictsPath, i)] = &fstest.MapFile{Data: []byte(body)}
	}
	if _, err := parse(files, where); err == nil {
		t.Fatal("a population with two pathways loaded")
	}
}

func TestAPopulationNobodyAuthoredIsRefused(t *testing.T) {
	if _, err := parse(whole(t), []string{"somewhere", "somewhere-else"}); err == nil {
		t.Fatal("a world where one population has no pathways loaded")
	}
}

func TestPathwaysForAPopulationTheWorldDoesNotHaveAreRefused(t *testing.T) {
	files := whole(t)
	files[conflictsPath+"/elsewhere/the-thing.yaml"] = &fstest.MapFile{Data: []byte(sound)}
	if _, err := parse(files, where); err == nil {
		t.Fatal("pathways for nobody loaded")
	}
}

// Every way one file can be wrong, and the message an author is owed for each.
func TestOneBrokenPathwayIsARefusal(t *testing.T) {
	for name, body := range map[string]string{
		"a pressure nobody has":  bad("follows: owing", "follows: sulking"),
		"a question nobody asks": bad("puts: help", "puts: weather"),
		"a term nobody has":      bad("term: help/condition/legal", "term: help/condition/weather"),
		"a fact about another question": bad("term: help/condition/legal",
			"term: voice/condition/above"),
		"an answer stated as a fact": bad("term: help/condition/legal", "term: help/action/yes"),
		"a source the standing does not hold": bad("term: help/condition/owe, from: owing",
			"term: help/condition/owe, from: mood"),
		"a count read into a term that holds none": bad("term: help/condition/owe, from: owing",
			"term: help/condition/owe, from: asked"),
		"a state read into a term that holds a number": bad("term: help/condition/short, reading: 60",
			"term: help/condition/short, from: owing"),
		"a number given to a term that holds none": bad("term: help/condition/legal, so: true",
			"term: help/condition/legal, reading: 2"),
		"a term stated as both":    bad("term: help/condition/owe, from: owing", "term: help/condition/owe, from: owing, so: true"),
		"a term stated twice":      bad("term: help/condition/legal, so: true", "term: help/condition/legal, so: false }\n  - { term: help/condition/legal, so: true"),
		"a posture nobody has":     bad("stance: yield", "stance: sulk"),
		"a posture answered twice": bad("stance: bargain", "stance: yield"),
		"a posture with no answer": strings.Replace(bad("key: the-thing", "key: broken"),
			`  - stance: resist
    did: Say no.
    came: It is taken well and not raised again.
    outcome: poor
    moves: { kindness: -6 }
    shifts: { slighted: 1 }
`, "", 1),
		"an outcome outside the set":        bad("outcome: good", "outcome: excellent"),
		"a factor read rather than granted": bad("moves: { kindness: 6 }", "moves: { money: 6 }"),
		"a factor that is not one":          bad("moves: { kindness: 6 }", "moves: { charisma: 6 }"),
		"a part of a standing nobody has":   bad("shifts: { owed: 1 }", "shifts: { grudge: 1 }"),
		"a shift larger than the ledger":    bad("shifts: { owed: 1 }", "shifts: { owed: 40 }"),
		"a line that does not say who":      bad("{who} mentions", "Somebody mentions"),
		"a field nobody reads":              bad("puts: help", "puts: help\nmood: sour"),
		"a name that labels the framework":  bad("name: The thing", "name: The orange thing"),
		"an answer that labels the framework": bad("came: It is taken well and not raised again.",
			"came: It is taken well, at this stage, and not raised again."),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parse(whole(t, body), where); err == nil {
				t.Fatalf("%s loaded", name)
			}
		})
	}
}

// The key is the filename and the population is the directory, so a conflict
// recorded against a player has one name in the table, in the log and on disk.
func TestAPathwayIsNamedAfterItsFileAndItsDirectory(t *testing.T) {
	files := whole(t)
	files[conflictsPath+"/somewhere/mislabelled.yaml"] = &fstest.MapFile{Data: []byte(sound)}
	if _, err := parse(files, where); err == nil {
		t.Fatal("a pathway whose key is not its filename loaded")
	}

	files = whole(t)
	files[conflictsPath+"/somewhere/wandered.yaml"] = &fstest.MapFile{
		Data: []byte(bad("key: broken", "key: wandered\namong: elsewhere")),
	}
	if _, err := parse(files, where); err == nil {
		t.Fatal("a pathway that says it is somewhere else loaded")
	}
}

/*
The shipped content, held to the same rules the loader enforces plus the ones it
cannot.

The catalogue has already been parsed by init or this test binary would not be
running, so what is left is what the loader has no opinion about: that every
population the world actually has is covered, that the writing is in the second
person about somebody, and that no answer is authored good everywhere — which is
§3.15's whole thesis and the one thing a spreadsheet of deltas could quietly
break.
*/
func TestTheShippedPathwaysCoverTheShippedPopulations(t *testing.T) {
	for _, among := range environment.All() {
		held := 0
		for _, a := range All() {
			if a.Among == among.Key {
				held++
			}
		}
		if held < Pool {
			t.Errorf("%q has %d pathways, want at least %d", among.Key, held, Pool)
		}
		for _, strain := range Strains {
			if len(For(among.Key, strain)) == 0 {
				t.Errorf("%q has nothing that follows from %q", among.Key, strain)
			}
		}
	}
}

func TestNoPostureIsAlwaysTheRightAnswer(t *testing.T) {
	// §3.15: "neither rule set is correct — that is the entire point." A posture
	// that never came to a poor outcome anywhere would be a dominant strategy,
	// and a player who found it would have finished the game.
	good := map[playbook.Stance]int{}
	poor := map[playbook.Stance]int{}
	for _, a := range All() {
		for _, answer := range a.Answers() {
			switch answer.Outcome {
			case playbook.Good:
				good[answer.Stance]++
			case playbook.Poor:
				poor[answer.Stance]++
			}
		}
	}
	for _, s := range playbook.Stances {
		if good[s]+poor[s] == 0 {
			continue
		}
		if poor[s] == 0 {
			t.Errorf("%q is authored good %d times and poor never; it is a dominant answer", s, good[s])
		}
		// And the same the other way. A posture that is punished everywhere is
		// not a choice either — it is a trap with a button on it, which teaches
		// the player to stop reading rather than to read more carefully.
		if good[s] == 0 {
			t.Errorf("%q is authored poor %d times and good never; it is an answer nobody may take", s, poor[s])
		}
	}
}

func TestThePopulationsDisagreeAboutWhatWorks(t *testing.T) {
	// The load-bearing property of the whole ticket: the same posture has to be
	// authored well in one population and badly in the other, or a rule set
	// would transfer and §3.15 would have nothing to teach.
	kind := map[string]map[playbook.Stance]map[playbook.Outcome]int{}
	for _, a := range All() {
		if kind[a.Among] == nil {
			kind[a.Among] = map[playbook.Stance]map[playbook.Outcome]int{}
		}
		for _, answer := range a.Answers() {
			if kind[a.Among][answer.Stance] == nil {
				kind[a.Among][answer.Stance] = map[playbook.Outcome]int{}
			}
			kind[a.Among][answer.Stance][answer.Outcome]++
		}
	}
	disagreements := 0
	for _, s := range playbook.Stances {
		for one := range kind {
			for other := range kind {
				if one >= other {
					continue
				}
				if kind[one][s][playbook.Good] > 0 && kind[other][s][playbook.Poor] > 0 {
					disagreements++
				}
			}
		}
	}
	if disagreements == 0 {
		t.Error("no posture is authored well in one population and badly in another; " +
			"a rule set that worked everywhere would make §3.15 a story rather than a mechanic")
	}
}

func TestEveryShippedLineSaysWhoItIsWith(t *testing.T) {
	for _, a := range All() {
		if !strings.Contains(a.Line, Who) {
			t.Errorf("%s/%s does not say who it is with", a.Among, a.Key)
		}
		if Says(a.Line, "Bez") == a.Line {
			t.Errorf("%s/%s did not take a name", a.Among, a.Key)
		}
	}
}

func TestAConflictPutsOneQuestion(t *testing.T) {
	// §3.15's "clearly lighter than an authored challenge", made structural.
	for _, a := range All() {
		if got := a.Moment(everything).Puts(); len(got) != 1 {
			t.Errorf("%s/%s puts %v", a.Among, a.Key, got)
		}
	}
}
