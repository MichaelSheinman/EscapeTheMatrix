package environment_test

import (
	"strings"
	"testing"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/environment"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
The shipped populations, checked as content rather than as code.

The same thing internal/challenge's authored_test.go and internal/situation's
catalogue_test.go do for their own catalogues: the loader proves a file is
well-formed, and this file proves the *world* is the one §3.15 describes. An
assertion here failing means somebody authored a population that parses and is
not the design.
*/

// Every population the world ships with, so a case below can name one without
// hard-coding a key that content is allowed to change.
func world(t *testing.T) []environment.Environment {
	t.Helper()
	all := environment.All()
	if len(all) < environment.Populations {
		t.Fatalf("the world has %d population(s)", len(all))
	}
	return all
}

/*
§3.15's opening sentence, as an assertion: an environment is its people.

Every population has a company of them, each one is somewhere in the city that
population actually is, and each one is described by something they *do*. The
last of those is the design — "the population's values have to be legible purely
through behaviour, never labelled" — and it is the only thing standing between a
population and a list of names with a mood attached.
*/
func TestAPopulationIsItsPeople(t *testing.T) {
	t.Parallel()

	for _, e := range world(t) {
		if len(e.People()) < environment.Company {
			t.Errorf("%q has %d people", e.Key, len(e.People()))
		}
		if len(e.Holds()) < environment.Common {
			t.Errorf("%q holds %d things in common", e.Key, len(e.Holds()))
		}
		for _, p := range e.People() {
			if p.Name == "" || p.Note == "" || p.Does == "" {
				t.Errorf("%q's %q arrived as %+v", e.Key, p.Key, p)
			}
			// Reusing existing locations is §3.15's own instruction, and being
			// somewhere in this population's own district is what makes them a
			// population rather than a mailing list.
			at, ok := city.Lookup(p.Found)
			if !ok {
				t.Errorf("%q's %q is found at %q, which is nowhere", e.Key, p.Key, p.Found)
				continue
			}
			if at.Key != e.Around && at.In != e.Around {
				t.Errorf("%q's %q is found in %q, not in %q", e.Key, p.Key, at.In, e.Around)
			}
		}
		// And the derived answer to "where would you actually run into these
		// six", which is what #30 places a conflict against.
		if len(e.Places()) == 0 {
			t.Errorf("%q turns up nowhere", e.Key)
		}
		for _, place := range e.Places() {
			if !city.Known(place) {
				t.Errorf("%q turns up at %q, which is not a place", e.Key, place)
			}
		}
	}
}

/*
§3.2, §3.10 and §3.15, in the one place they would be easiest to break.

"Environments are never labelled. No stage names, no framework vocabulary, ever."
The design document names the two registers V1 is authored against in as many
words, which is exactly why this runs over every string a player can reach
instead of trusting an author to remember.
*/
func TestNoPopulationIsEverLabelled(t *testing.T) {
	t.Parallel()

	for _, e := range world(t) {
		read := []string{e.Name, e.Note}
		read = append(read, e.Holds()...)
		for _, p := range e.People() {
			read = append(read, p.Name, p.Note, p.Does)
		}
		for _, text := range read {
			if word := environment.Labelled(text); word != "" {
				t.Errorf("%q says %q in %q", e.Key, word, text)
			}
		}
	}

	// And the check itself, both ways round, because a guard that never fires
	// and a guard that always fires look identical from inside the content.
	for _, said := range []string{
		"an orange setting",
		"They are Green people.",
		"the second STAGE of the model",
		"a register you have not met",
	} {
		if environment.Labelled(said) == "" {
			t.Errorf("%q was not caught", said)
		}
	}
	for _, fine := range []string{
		"Everybody knows what everybody is short of.",
		"An evergreen and a bag of oranges on the counter.",
		"He redeveloped the whole waterfront and never mentions it.",
	} {
		if word := environment.Labelled(fine); word != "" {
			t.Errorf("%q was caught on %q, which is ordinary English", fine, word)
		}
	}
}

/*
§3.15's factor affordances, as a real axis rather than as decoration.

"'Green setting, low money ceiling, high relational reward' versus 'Orange
setting, high money ceiling, punishes trust' is a real design axis for authoring
a population." So the world has to actually contain that contrast: two
populations that afford *opposite* things, or the first plateau-and-transfer
teaches nothing because the second place is the first place with new names.

Which population is which is deliberately not asserted. What matters is that they
differ, and in both directions.
*/
func TestPopulationsAffordOppositeThings(t *testing.T) {
	t.Parallel()

	all := world(t)
	var richest, kindest environment.Environment
	for _, e := range all {
		if e.Ceilings.Money > richest.Ceilings.Money {
			richest = e
		}
		if e.Ceilings.Kindness > kindest.Ceilings.Kindness {
			kindest = e
		}
	}
	if richest.Key == kindest.Key {
		t.Fatalf("%q affords both the most money and the most kindness; there is no trade to make", richest.Key)
	}
	// And the ceilings have to bite, or a plateau is a number nobody reaches.
	// The lowest money ceiling has to read visibly short of the top of the
	// curve, and the lowest relational cap short of the scale.
	for _, e := range all {
		if e.Ceilings.Money <= 0 || e.Ceilings.Stability <= 0 || e.Ceilings.Kindness <= 0 {
			t.Errorf("%q affords %+v, and a zero is no ceiling at all", e.Key, e.Ceilings)
		}
		if e.Ceilings.Stability > 100 || e.Ceilings.Kindness > 100 {
			t.Errorf("%q caps above the scale: %+v", e.Key, e.Ceilings)
		}
		// Intelligence is not a property of a place (§3.15) and no file may
		// pretend otherwise — the pure core's Reach is what fills it.
		if e.Ceilings.Intelligence != 0 {
			t.Errorf("%q authored an intelligence ceiling of %v", e.Key, e.Ceilings.Intelligence)
		}
	}
	if simulation.MoneyStanding(kindest.Ceilings.Money) > 40 {
		t.Errorf("the most relational population still affords money reading %v; the trade is not real",
			simulation.MoneyStanding(kindest.Ceilings.Money))
	}
}

// A population lives in one district, and a district holds one population.
// Both halves matter: the first is what lets somebody be drawn standing
// somewhere, the second is what stops "where am I" having two answers.
func TestEachPopulationIsAroundOneDistrictAndEachDistrictHoldsOne(t *testing.T) {
	t.Parallel()

	seen := map[string]string{}
	for _, e := range world(t) {
		at, ok := city.Lookup(e.Around)
		if !ok || at.Layer != city.District {
			t.Errorf("%q is around %q, which is not a district", e.Key, e.Around)
			continue
		}
		if other, taken := seen[e.Around]; taken {
			t.Errorf("%q and %q both live in %q", other, e.Key, e.Around)
		}
		seen[e.Around] = e.Key

		found, ok := environment.Around(e.Around)
		if !ok || found.Key != e.Key {
			t.Errorf("looking in %q found %q", e.Around, found.Key)
		}
	}
}

/*
Being among nobody is a state, not a failure.

Three districts out of four have no population authored for them today, and the
city has one home until #31 brings relocation. A player standing outside every
population must read as uncapped rather than as flattened — the pure core's zero
is "no ceiling", and this is the one place that convention is turned into an
answer.
*/
func TestBeingAmongNobodyAffordsNoCeilingRatherThanNone(t *testing.T) {
	t.Parallel()

	if _, ok := environment.Lookup(environment.Nowhere); ok {
		t.Fatalf("%q is a population", environment.Nowhere)
	}
	for _, key := range []string{environment.Nowhere, "", "downtown", "a-place-that-was-renamed"} {
		if got := environment.Ceilings(key); got != (simulation.Ceilings{}) {
			t.Errorf("%q affords %+v", key, got)
		}
	}
}

// Two processes describe the world in the same order, and neither can edit it
// (§7). The second half is what stops a request handler appending to the world.
func TestTheWorldIsStatedInOneOrderAndCannotBeEdited(t *testing.T) {
	t.Parallel()

	all := environment.All()
	for i := 1; i < len(all); i++ {
		if !(all[i-1].Key < all[i].Key) {
			t.Fatalf("populations arrived as %q then %q", all[i-1].Key, all[i].Key)
		}
	}

	first := environment.All()[0]
	people := first.People()
	people[0].Name = "somebody else"
	holds := first.Holds()
	holds[0] = "nothing at all"

	again, ok := environment.Lookup(first.Key)
	if !ok {
		t.Fatal("the first population stopped existing")
	}
	if again.People()[0].Name == "somebody else" || again.Holds()[0] == "nothing at all" {
		t.Fatal("a caller edited the world for everybody else")
	}
}

// Prose is prose: however an author wrapped a block, one paragraph is what
// reaches a screen.
func TestAuthoredProseArrivesAsOneParagraph(t *testing.T) {
	t.Parallel()

	for _, e := range world(t) {
		read := append([]string{e.Note}, e.Holds()...)
		for _, p := range e.People() {
			read = append(read, p.Note, p.Does)
		}
		for _, text := range read {
			if strings.ContainsAny(text, "\n\r") || strings.Contains(text, "  ") {
				t.Errorf("%q carries a line break or a double space: %q", e.Key, text)
			}
			if text != strings.TrimSpace(text) {
				t.Errorf("%q carries surrounding space: %q", e.Key, text)
			}
		}
	}
}
