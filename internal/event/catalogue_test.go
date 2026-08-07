package event

import (
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
The catalogue, tested as authored content.

No database and no clock here — the parse is pure, which is the point of it
being a separate file. What is asserted is what a content edit can break
silently: an event that can never be placed anywhere, a chain that points at
nothing, and the shipped catalogue itself.

The tests are in-package because `parse` is the testable half and `build` is
where every refusal is decided. What ships is exercised through the exported
[All] beside them.
*/

// The catalogue that actually ships loads, and every event in it can happen
// somewhere. This is the test that fails when somebody renames a zone.
func TestTheShippedCatalogueCanAllHappenSomewhere(t *testing.T) {
	t.Parallel()

	all := All()
	if len(all) == 0 {
		t.Fatal("no events ship at all")
	}

	for _, e := range all {
		if len(e.Where.Spots()) == 0 {
			t.Errorf("%s has nowhere to happen", e.Key)
		}
		for _, spot := range e.Where.Spots() {
			zone, ok := e.Where.Where(spot.Place)
			if !ok {
				t.Errorf("%s may be placed at %s and also may not", e.Key, spot.Place)
			}
			if !e.Where.Allows(spot.Place, spot.Zone) {
				t.Errorf("%s lists %s/%s and disallows it", e.Key, spot.Place, spot.Zone)
			}
			// The first zone offered for a place is one of the ones allowed
			// there — the priority is the city's order, not an arbitrary pick.
			if !e.Where.Allows(spot.Place, zone) {
				t.Errorf("%s is placed at %s/%s, which it does not allow", e.Key, spot.Place, zone)
			}
		}
	}
}

// Every shipped event is answerable and says what it comes to, or says why it
// cannot. The pair of fields is the one place two lines of content have to
// agree, and an author only finds out here.
func TestEveryShippedEventIsAnswerable(t *testing.T) {
	t.Parallel()

	for _, e := range All() {
		if e.Accept == "" || e.Decline == "" {
			t.Errorf("%s cannot be answered", e.Key)
		}
		if e.Awaits && e.After != "" {
			t.Errorf("%s waits on something else and already says how it went", e.Key)
		}
		if !e.Awaits && e.After == "" {
			t.Errorf("%s finishes on the spot and says nothing about it", e.Key)
		}
		if _, ok := KindOf(string(e.Kind)); !ok {
			t.Errorf("%s is a %q", e.Key, e.Kind)
		}
	}
}

// At least one shipped event chains, and every successor is itself shipped.
// §4 makes chaining a property of the system rather than of one pathway, so a
// catalogue where nothing chains is a catalogue that has stopped exercising it.
func TestTheShippedCatalogueChains(t *testing.T) {
	t.Parallel()

	chains := 0
	for _, e := range All() {
		if e.Then == "" {
			continue
		}
		chains++
		next, ok := Known(e.Then)
		if !ok {
			t.Errorf("%s is followed by %q, which is not an event", e.Key, e.Then)
			continue
		}
		if !next.Chained {
			t.Errorf("%s follows %s and is not marked as chained", next.Key, e.Key)
		}
	}
	if chains == 0 {
		t.Error("nothing in the catalogue chains (§4)")
	}
}

// A chained event is never generated spontaneously, which is the property the
// generator relies on to keep the second half of a conversation from arriving
// first. It is derived from the catalogue rather than authored, so this is the
// test that it is derived correctly.
func TestASuccessorIsMarkedChainedAndNothingElseIs(t *testing.T) {
	t.Parallel()

	named := map[string]bool{}
	for _, e := range All() {
		if e.Then != "" {
			named[e.Then] = true
		}
	}
	for _, e := range All() {
		if e.Chained != named[e.Key] {
			t.Errorf("%s: chained is %v, and it is named as a successor %v times",
				e.Key, e.Chained, len(named))
		}
	}
}

/* ── what the loader refuses ──────────────────────────────────────────── */

// one is a minimal valid file, for tests that change one thing about it.
const one = `
key: a-thing
name: A thing
kind: interpersonal
surface: world
line: Somebody is standing there.
accept: Say hello
decline: Keep walking
after: You say hello.
where:
  zones: [on-foot]
  layers: [district]
patience: 7
weight: 1
`

func tree(files map[string]string) fstest.MapFS {
	out := fstest.MapFS{}
	for name, body := range files {
		out["events/"+name] = &fstest.MapFile{Data: []byte(body)}
	}
	return out
}

func refuses(t *testing.T, files map[string]string, because string) {
	t.Helper()
	_, err := parse(tree(files))
	if err == nil {
		t.Fatalf("that loaded, and it should not have: %s", because)
	}
	if !strings.Contains(err.Error(), because) {
		t.Fatalf("refused with %q, wanted something about %q", err, because)
	}
}

// The minimal file is actually valid, so every refusal below is about the one
// thing it changed.
func TestTheMinimalEventLoads(t *testing.T) {
	t.Parallel()

	all, err := parse(tree(map[string]string{"a-thing.yaml": one}))
	if err != nil {
		t.Fatalf("the minimal event does not load: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("loaded %d events, want 1", len(all))
	}
	if got := all[0].Patience; got != 7*Day {
		t.Errorf("patience is %v, want %v", got, 7*Day)
	}
	// Every district, walkable — four places at V1, and the point is that the
	// author wrote "any street" rather than four names.
	if len(all[0].Where.Places()) < 4 {
		t.Errorf("placed in %v, want every district", all[0].Where.Places())
	}
}

// An event with no legitimate spot is a failed build rather than an event that
// silently never appears. This is the single most important refusal in the
// file: it is what makes "an event is never placed somewhere its rules
// disallow" a shape rather than a check.
func TestAnEventThatCanNeverBePlacedIsRefused(t *testing.T) {
	t.Parallel()

	// A zone that exists, in a place that has no such spot.
	refuses(t, map[string]string{"a-thing.yaml": strings.Replace(one,
		"  zones: [on-foot]\n  layers: [district]",
		"  zones: [bed]\n  layers: [district]", 1)}, "no spot anywhere")

	// A zone that is not a kind of spot at all.
	refuses(t, map[string]string{"a-thing.yaml": strings.Replace(one,
		"zones: [on-foot]", "zones: [the-rafters]", 1)}, "not a kind of spot")

	// Nowhere named at all.
	refuses(t, map[string]string{"a-thing.yaml": strings.Replace(one,
		"  zones: [on-foot]\n  layers: [district]", "  zones: [on-foot]", 1)}, "nowhere to happen")
}

// A successor that names nothing, and one that names itself.
func TestAChainThatGoesNowhereIsRefused(t *testing.T) {
	t.Parallel()

	refuses(t, map[string]string{"a-thing.yaml": one + "then: something-else\n"},
		"not an event")
	refuses(t, map[string]string{"a-thing.yaml": one + "then: a-thing\n"},
		"its own successor")
}

// A chain that eventually points back at itself would be a player answering the
// same run of events forever, and it is invisible in either file on its own.
func TestAChainThatLoopsIsRefused(t *testing.T) {
	t.Parallel()

	second := strings.Replace(one, "key: a-thing", "key: another-thing", 1)
	second = strings.Replace(second, "name: A thing", "name: Another thing", 1)

	refuses(t, map[string]string{
		"a.yaml": one + "then: another-thing\n",
		"b.yaml": second + "then: a-thing\n",
	}, "followed, eventually, by itself")
}

// Two events cannot share a successor: it would be queued twice, or once by
// whichever answer came first, which is not a chain anybody authored.
func TestTwoEventsCannotShareASuccessor(t *testing.T) {
	t.Parallel()

	second := strings.Replace(one, "key: a-thing", "key: another-thing", 1)
	third := strings.Replace(one, "key: a-thing", "key: a-third-thing", 1)

	refuses(t, map[string]string{
		"a.yaml": one + "then: a-third-thing\n",
		"b.yaml": second + "then: a-third-thing\n",
		"c.yaml": third,
	}, "both followed by")
}

// A cooldown on an event that is only ever chained says nothing, and an author
// who wrote one meant something else.
func TestAChainedEventCannotNameACooldown(t *testing.T) {
	t.Parallel()

	second := strings.Replace(one, "key: a-thing", "key: another-thing", 1)

	refuses(t, map[string]string{
		"a.yaml": one + "then: another-thing\n",
		"b.yaml": second + "again: 5\n",
	}, "only ever chained and names a cooldown")
}

// The two fields that have to agree, and the closed sets.
func TestTheLoaderRefusesContentThatContradictsItself(t *testing.T) {
	t.Parallel()

	refuses(t, map[string]string{"a.yaml": one + "awaits: true\n"},
		"already says how it went")
	refuses(t, map[string]string{"a.yaml": strings.Replace(one, "after: You say hello.\n", "", 1)},
		"does not say what it came to")
	refuses(t, map[string]string{"a.yaml": strings.Replace(one, "kind: interpersonal", "kind: romance", 1)},
		"not a kind of event")
	refuses(t, map[string]string{"a.yaml": strings.Replace(one, "surface: world", "surface: everywhere", 1)},
		"not a surface")
	refuses(t, map[string]string{"a.yaml": strings.Replace(one, "patience: 7", "patience: 0", 1)},
		"never gives up waiting")
	refuses(t, map[string]string{"a.yaml": strings.Replace(one, "weight: 1", "weight: 0", 1)},
		"worth nothing")
	refuses(t, map[string]string{"a.yaml": one + "needs:\n  money-at-least: 500\n  money-at-most: 100\n"},
		"more money than it allows")
}

// A field nobody defined is a failed build rather than a line of content
// quietly ignored — the same strict decoding internal/playbook uses.
func TestAMisspeltFieldIsRefused(t *testing.T) {
	t.Parallel()

	refuses(t, map[string]string{"a.yaml": one + "wieght: 4\n"}, "field wieght not found")
}

/* ── preconditions ────────────────────────────────────────────────────── */

// Needs is three-valued in effect: no bound, a floor, a ceiling, or both. The
// zero value asks for nothing, which is what most events want.
func TestPreconditionsAreCheckedAgainstWhatIsHeld(t *testing.T) {
	t.Parallel()

	var none Needs
	if none.Any() {
		t.Error("a Needs with nothing in it wants something")
	}
	if !none.Met(Circumstances{}) {
		t.Error("a player with nothing fails a precondition that asks for nothing")
	}

	floor := Needs{Least: true, AtLeast: 500}
	for _, c := range []struct {
		money simulation.Money
		want  bool
	}{{499, false}, {500, true}, {900, true}} {
		if got := floor.Met(Circumstances{Money: c.money}); got != c.want {
			t.Errorf("holding %d against a floor of 500: %v, want %v", c.money.Cents(), got, c.want)
		}
	}

	ceiling := Needs{Most: true, AtMost: 500}
	if ceiling.Met(Circumstances{Money: 501}) {
		t.Error("501 is under a ceiling of 500")
	}
	if !ceiling.Met(Circumstances{Money: 500}) {
		t.Error("500 is over a ceiling of 500")
	}
}

/* ── the release policy's floor ───────────────────────────────────────── */

// Steady is three floors and nothing else. Asserted here because #22 replaces
// it, and the thing it replaces should be legible.
func TestSteadyHoldsTheThreeFloors(t *testing.T) {
	t.Parallel()

	ok := Attention{Since: Never}
	if !(Steady{}).Release(ok) {
		t.Error("a quiet world releases nothing")
	}

	standing := ok
	standing.Standing = true
	if (Steady{}).Release(standing) {
		t.Error("something was released on top of what is already on screen")
	}

	busy := ok
	busy.Released = Daily
	if (Steady{}).Release(busy) {
		t.Errorf("a %dth event was released today", Daily+1)
	}

	soon := ok
	soon.Since = Quiet - time.Minute
	if (Steady{}).Release(soon) {
		t.Error("two events were released inside the quiet period")
	}
}
