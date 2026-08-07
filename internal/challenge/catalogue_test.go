package challenge

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
The catalogue, as content (§5.2).

What is worth proving here is not that YAML parses. It is the set of things a
build has to refuse — a mechanic type nobody implemented, a challenge that can
never begin, two challenges behind one event, a scene nothing leads to — because
each of them is a challenge that would otherwise be shipped and silently never
played. §5.2's own words: "production validates at startup so a malformed
challenge fails in CI, not at 2am."
*/

/* ── test mechanic types ───────────────────────────────────────────────── */

// staged is a mechanic type that hands back whatever plan a test gave it.
//
// It stands in for #24's real one. What it is not is a shortcut around the
// validation: the framework's own checks run over the plan exactly as they
// would over a real mechanic's, which is what these tests are about.
type staged struct {
	name Type
	plan Plan
	// refuse is what the mechanic's own config schema said, if it said no.
	refuse error
}

func (s staged) Type() Type { return s.name }

func (s staged) Configure(dir fs.FS) (Plan, error) {
	if s.refuse != nil {
		return Plan{}, s.refuse
	}
	return s.plan, nil
}

// rooted is a mechanic that insists on reading its own document, so that "the
// fs.FS a mechanic is handed is rooted at the variant directory" is a claim
// something checks rather than a comment.
type rooted struct{}

func (rooted) Type() Type { return "rooted" }

func (rooted) Configure(dir fs.FS) (Plan, error) {
	raw, err := fs.ReadFile(dir, "plan.yaml")
	if err != nil {
		return Plan{}, fmt.Errorf("read plan.yaml: %w", err)
	}
	if strings.TrimSpace(string(raw)) != "ok" {
		return Plan{}, fmt.Errorf("plan.yaml does not say ok")
	}
	return twoScenes(), nil
}

// twoScenes is the smallest plan that is a challenge: a committed judgement,
// two ways out of it, and an outcome for each.
func twoScenes() Plan {
	return Plan{
		Opening: "stoop",
		Scenes: []Scene{
			{Key: "stoop", Options: []Option{
				{Key: "listen", Then: "kitchen", Weight: 2,
					Deltas: []simulation.Grant{{Factor: simulation.Kindness, Credit: 3}}},
				{Key: "walk-on", Weight: -1},
			}},
			{Key: "kitchen", Options: []Option{
				{Key: "side-with-her", Weight: 1,
					Deltas: []simulation.Grant{{Factor: simulation.Stability, Credit: 2}}},
				{Key: "side-with-him", Weight: 0},
			}},
		},
		Buckets: []Bucket{
			{Key: "dismissed", AtLeast: -1, Reaction: "They stop talking when you speak."},
			{Key: "heard", AtLeast: 2, Reaction: "They both look at you."},
			{Key: "moved", AtLeast: 3, Reaction: "She sits down. He does too."},
		},
	}
}

func types(m ...Mechanic) map[Type]Mechanic {
	out := map[Type]Mechanic{}
	for _, one := range m {
		out[one.Type()] = one
	}
	return out
}

// tree composes a content tree with one challenge in it.
func tree(family, variant, spec string, extra map[string]string) fstest.MapFS {
	out := fstest.MapFS{
		"challenges/" + family + "/" + variant + "/challenge.yaml": &fstest.MapFile{Data: []byte(spec)},
	}
	for name, body := range extra {
		out["challenges/"+family+"/"+variant+"/"+name] = &fstest.MapFile{Data: []byte(body)}
	}
	return out
}

/* ── what loads ────────────────────────────────────────────────────────── */

func TestIdentityIsThePathRatherThanAField(t *testing.T) {
	files := tree("resolve_conflict", "a", `
name: The argument on the stoop
mechanic: staged
where:
  districts: [east-side]
  zones: [on-foot]
`, nil)

	loaded, err := parse(files, types(staged{name: "staged", plan: twoScenes()}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("%d challenges, want 1", len(loaded))
	}

	d := loaded[0]
	// §5.3: three columns, not one flat string — and the two that are identity
	// come from where the content lives, so a file cannot disagree with it.
	if d.Family != "resolve_conflict" || d.Variant != "a" || d.Mechanic != "staged" {
		t.Errorf("identity = %q/%q on %q, want the directories and the authored mechanic",
			d.Family, d.Variant, d.Mechanic)
	}
	if d.Code != "resolve_conflict/a" {
		t.Errorf("Code = %q, want the derived code", d.Code)
	}
	if d.Code.Family() != d.Family || d.Code.Variant() != d.Variant {
		t.Errorf("the code does not take apart into the columns it was made from: %q", d.Code)
	}
}

// A mechanic reads *its own* documents, and it reads them from its own
// directory. Handing it the whole content tree would let one challenge's config
// reach another's.
func TestAMechanicIsHandedItsOwnDirectory(t *testing.T) {
	files := tree("resolve_conflict", "a", `
name: The argument on the stoop
mechanic: rooted
opens: a-shift-going-spare
`, map[string]string{"plan.yaml": "ok"})

	if _, err := parse(files, types(rooted{})); err != nil {
		t.Fatalf("parse: %v", err)
	}

	// The same tree with the document one level up: the mechanic must not find
	// it, because it is not in its directory.
	moved := fstest.MapFS{
		"challenges/resolve_conflict/a/challenge.yaml": &fstest.MapFile{Data: []byte(`
name: The argument on the stoop
mechanic: rooted
opens: a-shift-going-spare
`)},
		"challenges/resolve_conflict/plan.yaml": &fstest.MapFile{Data: []byte("ok")},
	}
	if _, err := parse(moved, types(rooted{})); err == nil {
		t.Fatal("parse = nil, want the mechanic to fail to find its own document")
	}
}

// §5.2: "a new mechanic is code". Content that asks for one nobody wrote is a
// failed build, and the message names what there is, because it is nearly
// always a typo.
func TestAnUnknownMechanicTypeIsAFailedBuild(t *testing.T) {
	files := tree("resolve_conflict", "a", `
name: The argument on the stoop
mechanic: dialouge_branch
opens: a-shift-going-spare
`, nil)

	_, err := parse(files, types(staged{name: "dialogue_branch", plan: twoScenes()}))
	if err == nil {
		t.Fatal("parse = nil, want a refusal")
	}
	if !strings.Contains(err.Error(), "dialogue_branch") {
		t.Errorf("error %q does not name the mechanic types there are", err)
	}
}

// The mechanic's own config schema is the mechanic's business, and its refusal
// is the loader's: whatever it says, the build stops.
func TestAMechanicsOwnRefusalStopsTheBuild(t *testing.T) {
	files := tree("resolve_conflict", "a", `
name: The argument on the stoop
mechanic: staged
opens: a-shift-going-spare
`, nil)

	_, err := parse(files, types(staged{name: "staged", refuse: fmt.Errorf("dialogue.yaml has no nodes")}))
	if err == nil || !strings.Contains(err.Error(), "no nodes") {
		t.Fatalf("parse = %v, want the mechanic's own complaint", err)
	}
}

// Strict decoding: a misspelt field is a failed build rather than a line of
// content silently ignored.
func TestAFieldThatIsNotInTheContractIsAFailedBuild(t *testing.T) {
	files := tree("resolve_conflict", "a", `
name: The argument on the stoop
mechanic: staged
opens: a-shift-going-spare
reward: 40
`, nil)

	if _, err := parse(files, types(staged{name: "staged", plan: twoScenes()})); err == nil {
		t.Fatal("parse = nil, want a refusal for a field nobody reads")
	}
}

/* ── availability and offer ────────────────────────────────────────────── */

/*
§4 keeps availability and offer as separate concepts. A challenge names one, the
other, or both — and naming neither is content nobody can ever reach, which is
exactly the failure resolving at boot exists to make impossible.
*/
func TestAChallengeNobodyCanEverBeginIsAFailedBuild(t *testing.T) {
	files := tree("resolve_conflict", "a", `
name: The argument on the stoop
mechanic: staged
`, nil)

	_, err := parse(files, types(staged{name: "staged", plan: twoScenes()}))
	if err == nil || !strings.Contains(err.Error(), "can never begin") {
		t.Fatalf("parse = %v, want a refusal for a challenge with no way in", err)
	}
}

func TestAStandingEntryPointResolvesToRealSpots(t *testing.T) {
	files := tree("resolve_conflict", "a", `
name: The argument on the stoop
mechanic: staged
where:
  districts: [east-side]
  zones: [home-door, on-foot]
`, nil)

	loaded, err := parse(files, types(staged{name: "staged", plan: twoScenes()}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	d := loaded[0]
	if !d.Standing() {
		t.Fatal("Standing() = false for a challenge with a where block")
	}
	zone, ok := d.Where.At("east-side")
	if !ok {
		t.Fatal("the east side is not one of its places")
	}
	// The city orders a place's zones most specific first, so the answer is the
	// doorstep rather than the street, with nobody authoring a priority.
	if zone != "home-door" {
		t.Errorf("At(east-side) = %q, want the more specific spot", zone)
	}
	if _, ok := d.Where.At("downtown"); ok {
		t.Error("it stands downtown, which it never named")
	}
}

func TestAnEntryPointThatResolvesToNowhereIsAFailedBuild(t *testing.T) {
	for _, tc := range []struct {
		name  string
		where string
		want  string
	}{
		{"a zone that is not one", "  districts: [east-side]\n  zones: [rooftop]", "not a kind of spot"},
		{"a place that is nowhere", "  places: [the-moon]\n  zones: [on-foot]", "which is nowhere"},
		{"a district that is not one", "  districts: [east-side-apartment]\n  zones: [at-home]", "not one"},
		{"no zone in any named place", "  places: [east-side-apartment]\n  zones: [counter]", "stands nowhere"},
		{"places and no zones", "  districts: [east-side]", "no zones"},
		{"zones and nowhere to find them", "  zones: [on-foot]", "nowhere to find them"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := tree("resolve_conflict", "a", "name: A\nmechanic: staged\nwhere:\n"+tc.where+"\n", nil)
			_, err := parse(files, types(staged{name: "staged", plan: twoScenes()}))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("parse = %v, want a refusal mentioning %q", err, tc.want)
			}
		})
	}
}

/*
The `opens` link is checked against the event catalogue, both ways.

An event that finishes on the spot has no doorway in it — internal/event's
`awaits` is what marks one that does — so a challenge behind it would be a
challenge that never begins, and the two files never say so on their own.
*/
func TestAChallengeIsOpenedOnlyByAnEventThatWaits(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opens string
		want  string
	}{
		{"an event that is not one", "a-knock-at-the-window", "not an event"},
		{"an event that finishes on the spot", "the-lights-left-on", "finishes on the spot"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := tree("resolve_conflict", "a", "name: A\nmechanic: staged\nopens: "+tc.opens+"\n", nil)
			_, err := parse(files, types(staged{name: "staged", plan: twoScenes()}))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("parse = %v, want a refusal mentioning %q", err, tc.want)
			}
		})
	}
}

// One event, one doorway. Two challenges behind one accepted event would be a
// choice nothing in the content decides — settled, if it were allowed, by
// directory order.
func TestTwoChallengesCannotHideBehindOneEvent(t *testing.T) {
	files := fstest.MapFS{
		"challenges/resolve_conflict/a/challenge.yaml": &fstest.MapFile{
			Data: []byte("name: A\nmechanic: staged\nopens: a-shift-going-spare\n")},
		"challenges/resolve_conflict/b/challenge.yaml": &fstest.MapFile{
			Data: []byte("name: B\nmechanic: staged\nopens: a-shift-going-spare\n")},
	}

	_, err := parse(files, types(staged{name: "staged", plan: twoScenes()}))
	if err == nil || !strings.Contains(err.Error(), "both opened by") {
		t.Fatalf("parse = %v, want a refusal for two challenges behind one event", err)
	}
	if found, ok := Opened("a-shift-going-spare"); ok {
		t.Errorf("the shipped catalogue already has %s behind that event", found.Code)
	}
}

/* ── the shape of the tree ─────────────────────────────────────────────── */

func TestTheDirectoryNamesHaveToSurviveBeingHalfOfACode(t *testing.T) {
	files := fstest.MapFS{
		"challenges/Resolve Conflict/a/challenge.yaml": &fstest.MapFile{
			Data: []byte("name: A\nmechanic: staged\nopens: a-shift-going-spare\n")},
	}
	if _, err := parse(files, types(staged{name: "staged", plan: twoScenes()})); err == nil {
		t.Fatal("parse = nil, want a refusal for a family that is not a slug")
	}
}

func TestAFamilyWithNoVariantsIsAFailedBuild(t *testing.T) {
	files := fstest.MapFS{
		"challenges/resolve_conflict/notes.md": &fstest.MapFile{Data: []byte("later")},
	}
	_, err := parse(files, types(staged{name: "staged", plan: twoScenes()}))
	if err == nil || !strings.Contains(err.Error(), "no variants") {
		t.Fatalf("parse = %v, want a refusal for a family halfway authored", err)
	}
}

// A README beside the families is documentation, not content. The loader steps
// over files at the top level rather than refusing them.
func TestFilesBesideTheFamiliesAreNotChallenges(t *testing.T) {
	files := fstest.MapFS{
		"challenges/README.md": &fstest.MapFile{Data: []byte("# Challenges")},
		"challenges/resolve_conflict/a/challenge.yaml": &fstest.MapFile{
			Data: []byte("name: A\nmechanic: staged\nopens: a-shift-going-spare\n")},
	}

	loaded, err := parse(files, types(staged{name: "staged", plan: twoScenes()}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("%d challenges, want the one that is a directory", len(loaded))
	}
}

/*
The catalogue that ships loads, which is the whole of §5.2's CI promise.

It stopped being empty with #24 (`resolve_conflict/a`, the argument on the
stoop), so this asserts that there is something in it as well as that what is in
it is runnable — a loader that silently found nothing would otherwise pass every
check in this file.
*/
func TestTheShippedContentLoads(t *testing.T) {
	all := All()
	if len(all) == 0 {
		t.Fatal("nothing is authored, so nobody can play anything")
	}
	for _, d := range all {
		if _, known := mechanics[d.Mechanic]; !known {
			t.Errorf("%s runs on %q, which is not registered", d.Code, d.Mechanic)
		}
	}
	t.Logf("%d challenges are authored; the framework loaded all of them", len(all))
}

// The registry is closed by [Register] rather than by a list, and registering
// the same name twice is a coin toss over whose code runs somebody's challenge.
func TestTwoMechanicsCannotShareAName(t *testing.T) {
	t.Cleanup(func() { delete(mechanics, "twice") })
	defer func() {
		if recover() == nil {
			t.Fatal("Register did not panic on a name already taken")
		}
	}()
	Register(staged{name: "twice"})
	Register(staged{name: "twice"})
}
