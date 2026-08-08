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
// Its key is the district it lives in — an environment goes by one term, the
// map's — so the fixture borrows two districts the shipped content leaves
// unpopulated.
const sound = `
key: the-strip
note: Six people who have all worked the same shift for eleven years.
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
	s := strings.ReplaceAll(sound, "key: the-strip\n", "key: downtown\n")
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

	all, err := parse(files(t, map[string]string{"the-strip": sound, "downtown": second()}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("%d populations", len(all))
	}
	// Key order, not directory order (§7).
	if all[0].Key != "downtown" || all[1].Key != "the-strip" {
		t.Fatalf("loaded as %q then %q", all[0].Key, all[1].Key)
	}
	strip := all[1]
	// The name is the map's own for the district, never authored here.
	if strip.Name != "The Strip" || len(strip.People()) != 4 {
		t.Fatalf("the strip loaded as %+v", strip)
	}
	if strip.Ceilings.Money != 9_000_000 || strip.Ceilings.Kindness != 71 {
		t.Fatalf("the strip affords %+v", strip.Ceilings)
	}
	// Intelligence is never authored: it is a fact about the player (§3.15).
	if strip.Ceilings.Intelligence != 0 {
		t.Fatalf("the strip authored an intelligence ceiling of %v", strip.Ceilings.Intelligence)
	}
	// Where you would actually run into them, most-used first with ties in
	// authored order — derived from the people rather than declared.
	if got := strip.Places(); len(got) != 4 || got[0] != "the-strip-loading-bay" {
		t.Fatalf("the strip turns up at %v", got)
	}
}

/*
Every way a population can be wrong, and the message an author gets for it.

The four worth reading twice, because they are the design rather than a missing
field: a population keyed by anything but a district on the map is refused (an
environment goes by one term, the map's), a second name for it is a refused
field for the same reason, a population whose people do nothing is refused
(behaviour is the only legible channel §3.15 allows), and a population that
names the framework is refused — the last being the one this codebase would
otherwise break by accident, since the design document uses the vocabulary
freely.
*/
func TestABrokenPopulationIsARefusal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		was   string
		now   string
		wants string
	}{
		{"no key", "key: the-strip", "key: \"\"", "no key"},
		{"the key for being among nobody", "key: the-strip", "key: nowhere", "among nobody"},
		{"somewhere that is not a place", "key: the-strip\n", "key: mars\n", "not a place"},
		{"a room rather than a district", "key: the-strip\n", "key: the-strip-bar\n", "not a district"},
		{"a second name for the place", "key: the-strip\n", "key: the-strip\nname: The yard\n", "field"},
		{"too little held in common", "  - The newest person is wrong until they have been here a winter.\n", "", "value system"},
		{"no economic ceiling", "money: 9000000", "money: 0", "economic ceiling"},
		{"a cap off the scale", "kindness: 71", "kindness: 140", "between 0 and 100"},
		{"a cap of nothing", "stability: 64", "stability: 0", "between 0 and 100"},
		{"somebody with no behaviour", "does: Tells you the number before you ask, and tells your boss the same one.", "does: \"\"", "legible from behaviour"},
		{"somebody nowhere", "found: the-strip-warehouse", "found: nowhere-at-all", "not a place"},
		{"somebody in another district", "found: the-strip-warehouse", "found: riverfront-gym", "not in \"the-strip\""},
		{"two people with one key", "key: sass", "key: mo", "two people"},
		{"a field nobody reads", "key: the-strip\n", "key: the-strip\nregisters: two\n", "field"},
		{"an authored intelligence ceiling", "money: 9000000", "money: 9000000\n  intelligence: 40", "field"},
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
			_, err := parse(files(t, map[string]string{"the-strip": broken, "downtown": second()}))
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
// It is also what makes a district with two populations unrepresentable: the
// key is the district, and a directory holds one file per name.
func TestAPopulationIsNamedByItsFile(t *testing.T) {
	t.Parallel()

	_, err := parse(files(t, map[string]string{"the-shed": sound, "downtown": second()}))
	if err == nil || !strings.Contains(err.Error(), "is called") {
		t.Fatalf("err = %v, want a refusal about the filename", err)
	}
}

/*
Two people who would be indistinguishable to the rest of the program.

A person key used twice makes a relationship row mean different people
depending on where its owner lives, which is what #30 selects a conflict from.
(Two populations in one district stopped being a case to refuse when the
district became the key: the filesystem cannot hold two files by one name.)
*/
func TestTheWorldRefusesTwoPopulationsThatCollide(t *testing.T) {
	t.Parallel()

	samePerson := strings.Replace(second(), "key: bea", "key: mo", 1)
	if _, err := parse(files(t, map[string]string{"the-strip": sound, "downtown": samePerson})); err == nil ||
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

	_, err := parse(files(t, map[string]string{"the-strip": sound}))
	if err == nil || !strings.Contains(err.Error(), "rules to be local") {
		t.Fatalf("err = %v, want a refusal about there being nowhere else", err)
	}
	if _, err := parse(fstest.MapFS{}); err == nil {
		t.Fatal("a content tree with no environments directory loaded")
	}
}
