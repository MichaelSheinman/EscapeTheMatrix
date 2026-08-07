package environment

import (
	"strings"
	"testing"
	"testing/fstest"
)

/*
The loader, and the refusals that are the design rather than hygiene.

technical.md §5.2 puts the whole argument for authored content in one sentence —
"production validates at startup so a malformed challenge fails in CI, not at
2am" — and every case below is one of the ways a population can be wrong. They
are worth having as tests for the reason the loader is a function over an fs.FS
rather than over the embedded files: an init that has already run cannot be
caught, so the failure paths would otherwise only ever be exercised by breaking
the shipped content.
*/

// A population that parses, as the base every broken case below is a copy of.
const sound = `
key: the-yard
name: The yard
note: Six people who have all worked the same shift for eleven years.
around: the-strip
holds:
  - A thing said on the floor stays on the floor, and everybody knows the rule.
  - The newest person is wrong until they have been here a winter.
  - Nobody clocks anybody else off, and nobody has ever been asked to.
affords:
  money: 9000000
  stability: 64
  kindness: 71
people:
  - key: mo
    name: Mo
    note: Runs the roller door and has done since before the door was new.
    found: the-strip-loading-bay
    does: Puts you on the easy end of the pallet without saying anything about it.
  - key: sass
    name: Sass
    note: Drives the forklift and will not be hurried.
    found: the-strip-warehouse
    does: Finishes what she started before she answers you, every single time.
  - key: kit
    name: Kit
    note: Behind the bar four nights, on the dock the other three.
    found: the-strip-bar
    does: Remembers what you drink and what you said the last time you had four.
  - key: ade
    name: Ade
    note: Keeps the clipboard and the count, and the two always agree.
    found: the-strip
    does: Tells you the number before you ask, and tells your boss the same one.
`

// files builds a content filesystem out of whole population files, keyed the
// way the loader requires: one file per population, named for its key.
func files(t *testing.T, named map[string]string) fstest.MapFS {
	t.Helper()
	out := fstest.MapFS{}
	for name, body := range named {
		out[environmentsPath+"/"+name+".yaml"] = &fstest.MapFile{Data: []byte(body)}
	}
	return out
}

// A second sound population, so a case can be about the *world* rather than
// about one file.
func second() string {
	s := strings.ReplaceAll(sound, "key: the-yard", "key: the-desks")
	s = strings.ReplaceAll(s, "name: The yard", "name: The desks")
	s = strings.ReplaceAll(s, "around: the-strip", "around: downtown")
	s = strings.ReplaceAll(s, "the-strip-loading-bay", "downtown-office")
	s = strings.ReplaceAll(s, "the-strip-warehouse", "downtown-library")
	s = strings.ReplaceAll(s, "the-strip-bar", "downtown-diner")
	s = strings.ReplaceAll(s, "found: the-strip", "found: downtown")
	for was, now := range map[string]string{"mo": "bea", "sass": "col", "kit": "vin", "ade": "rho"} {
		s = strings.ReplaceAll(s, "key: "+was+"\n", "key: "+now+"\n")
	}
	return s
}

func TestASoundWorldLoads(t *testing.T) {
	t.Parallel()

	all, err := parse(files(t, map[string]string{"the-yard": sound, "the-desks": second()}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("%d populations", len(all))
	}
	// Key order, not directory order (§7).
	if all[0].Key != "the-desks" || all[1].Key != "the-yard" {
		t.Fatalf("loaded as %q then %q", all[0].Key, all[1].Key)
	}
	yard := all[1]
	if yard.Name != "The yard" || yard.Around != "the-strip" || len(yard.People()) != 4 {
		t.Fatalf("the yard loaded as %+v", yard)
	}
	if yard.Ceilings.Money != 9_000_000 || yard.Ceilings.Kindness != 71 {
		t.Fatalf("the yard affords %+v", yard.Ceilings)
	}
	// Intelligence is never authored: it is a fact about the player (§3.15).
	if yard.Ceilings.Intelligence != 0 {
		t.Fatalf("the yard authored an intelligence ceiling of %v", yard.Ceilings.Intelligence)
	}
	// Where you would actually run into them, most-used first with ties in
	// authored order — derived from the people rather than declared.
	if got := yard.Places(); len(got) != 4 || got[0] != "the-strip-loading-bay" {
		t.Fatalf("the yard turns up at %v", got)
	}
}

/*
Every way a population can be wrong, and the message an author gets for it.

The three worth reading twice, because they are the design rather than a missing
field: a population nobody could run into is refused (`found` outside its own
district), a population whose people do nothing is refused (behaviour is the only
legible channel §3.15 allows), and a population that names the framework is
refused — the last being the one this codebase would otherwise break by
accident, since the design document uses the vocabulary freely.
*/
func TestABrokenPopulationIsARefusal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		was   string
		now   string
		wants string
	}{
		{"no key", "key: the-yard", "key: \"\"", "no key"},
		{"the key for being among nobody", "key: the-yard", "key: nowhere", "among nobody"},
		{"no name", "name: The yard", "name: \"\"", "needs a name"},
		{"somewhere that is not a place", "around: the-strip", "around: mars", "not a place"},
		{"a room rather than a district", "around: the-strip", "around: the-strip-bar", "not a district"},
		{"too little held in common", "  - The newest person is wrong until they have been here a winter.\n", "", "value system"},
		{"no economic ceiling", "money: 9000000", "money: 0", "economic ceiling"},
		{"a cap off the scale", "kindness: 71", "kindness: 140", "between 0 and 100"},
		{"a cap of nothing", "stability: 64", "stability: 0", "between 0 and 100"},
		{"somebody with no behaviour", "does: Tells you the number before you ask, and tells your boss the same one.", "does: \"\"", "legible from behaviour"},
		{"somebody nowhere", "found: the-strip-warehouse", "found: nowhere-at-all", "not a place"},
		{"somebody in another district", "found: the-strip-warehouse", "found: riverfront-gym", "not in \"the-strip\""},
		{"two people with one key", "key: sass", "key: mo", "two people"},
		{"a field nobody reads", "name: The yard", "name: The yard\nregisters: two", "field"},
		{"an authored intelligence ceiling", "money: 9000000", "money: 9000000\n  intelligence: 40", "field"},
		{"a name that labels the framework", "name: The yard", "name: The orange yard", "labels the framework"},
		{"a value that labels the framework", "  - The newest person is wrong until they have been here a winter.",
			"  - These are green people and they behave accordingly.", "labels the framework"},
		{"a person who labels the framework", "note: Drives the forklift and will not be hurried.",
			"note: Drives the forklift, and is at a later stage than the rest.", "labels the framework"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			broken := strings.Replace(sound, c.was, c.now, 1)
			if broken == sound {
				t.Fatalf("the case did not change anything: %q is not in the file", c.was)
			}
			_, err := parse(files(t, map[string]string{"the-yard": broken, "the-desks": second()}))
			if err == nil {
				t.Fatalf("loaded anyway")
			}
			if !strings.Contains(err.Error(), c.wants) {
				t.Fatalf("refused with %q, want something about %q", err, c.wants)
			}
		})
	}
}

// A file whose key and filename disagree is refused, because the key is what a
// row in three tables is filed under and disk is where it is read back from.
func TestAPopulationIsNamedByItsFile(t *testing.T) {
	t.Parallel()

	_, err := parse(files(t, map[string]string{"the-shed": sound, "the-desks": second()}))
	if err == nil || !strings.Contains(err.Error(), "is called") {
		t.Fatalf("err = %v, want a refusal about the filename", err)
	}
}

/*
Two populations that would be indistinguishable to the rest of the program.

A district with two populations makes "who am I among" a question with two
answers, and the four columns that file a row under an environment key would
have to guess. A person key used twice makes a relationship row mean different
people depending on where its owner lives, which is what #30 selects a conflict
from.
*/
func TestTheWorldRefusesTwoPopulationsThatCollide(t *testing.T) {
	t.Parallel()

	// The same yard, in the same district, with different people in it: the one
	// collision left once every other field has been made distinct.
	sameDistrict := strings.ReplaceAll(sound, "key: the-yard", "key: the-desks")
	sameDistrict = strings.ReplaceAll(sameDistrict, "name: The yard", "name: The desks")
	for was, now := range map[string]string{"mo": "bea", "sass": "col", "kit": "vin", "ade": "rho"} {
		sameDistrict = strings.ReplaceAll(sameDistrict, "key: "+was+"\n", "key: "+now+"\n")
	}
	if _, err := parse(files(t, map[string]string{"the-yard": sound, "the-desks": sameDistrict})); err == nil ||
		!strings.Contains(err.Error(), "one population") {
		t.Fatalf("err = %v, want a refusal about two populations in one district", err)
	}

	samePerson := strings.Replace(second(), "key: bea", "key: mo", 1)
	if _, err := parse(files(t, map[string]string{"the-yard": sound, "the-desks": samePerson})); err == nil ||
		!strings.Contains(err.Error(), "already somebody") {
		t.Fatalf("err = %v, want a refusal about one person in two populations", err)
	}
}

/*
A world with one population does not load.

§3.15's V1 scope is two, and the reason is the mechanic rather than the content
budget: one population cannot demonstrate that rules are local, so a build with
one is a build with the core progression missing. It fails at boot rather than
being noticed by a player who never finds anywhere to go.
*/
func TestAWorldWithOnePopulationIsNotAWorld(t *testing.T) {
	t.Parallel()

	_, err := parse(files(t, map[string]string{"the-yard": sound}))
	if err == nil || !strings.Contains(err.Error(), "rules to be local") {
		t.Fatalf("err = %v, want a refusal about there being nowhere else", err)
	}
	if _, err := parse(fstest.MapFS{}); err == nil {
		t.Fatal("a content tree with no environments directory loaded")
	}
}
