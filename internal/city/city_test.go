package city_test

import (
	"testing"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
The city, tested as authored content.

There is no database here and no clock, so these are microseconds — which is the
point of the package being pure. What is asserted is the part a level author or
a content edit can break silently: that everywhere can be reached, that a fare is
only ever charged for a ride, and that the walk-versus-transit trade is a real
one rather than two words in a design document.
*/

// Everywhere in the city is walkable from everywhere else. §3.12 says "freely
// walkable", and a place that is authored but unreachable is a place the player
// will never see and nobody will notice is missing.
func TestEveryPlaceCanBeWalkedToFromEveryOther(t *testing.T) {
	t.Parallel()
	all := city.Places()

	for _, from := range all {
		for _, to := range all {
			route, ok := city.Route(from.Key, to.Key, city.OnFoot)
			if !ok {
				t.Fatalf("no walk from %s to %s", from.Key, to.Key)
			}
			if from.Key == to.Key {
				if len(route) != 0 {
					t.Errorf("walking to where you stand is %d steps, want none", len(route))
				}
				continue
			}
			if len(route) == 0 {
				t.Errorf("walking from %s to %s is no steps at all", from.Key, to.Key)
			}
			if got := city.Cost(route); got != 0 {
				t.Errorf("walking from %s to %s costs %d cents — walking is free (§3.12)",
					from.Key, to.Key, got.Cents())
			}
			if end := route[len(route)-1].To; end != to.Key {
				t.Errorf("the walk to %s ends at %s", to.Key, end)
			}
		}
	}
}

// Each step of a route starts where the last one finished. A route that teleports
// halfway through would still arrive, and nothing else would notice.
func TestARouteIsAChain(t *testing.T) {
	t.Parallel()

	route, ok := city.Route("east-side-apartment", "riverfront-gym", city.OnFoot)
	if !ok {
		t.Fatal("no walk from the apartment to the gym")
	}
	at := "east-side-apartment"
	for i, w := range route {
		if w.From != at {
			t.Fatalf("step %d starts at %s, but the walk had reached %s", i, w.From, at)
		}
		if w.Mode != city.OnFoot {
			t.Errorf("step %d of a walk is %q", i, w.Mode)
		}
		if w.Name == "" {
			t.Errorf("step %d has nothing to read on a button", i)
		}
		at = w.To
	}
	if at != "riverfront-gym" {
		t.Errorf("the walk ended at %s", at)
	}
}

// The shortest route is the shortest route: out of the flat, down the street,
// across town, in through the door. Four steps, and BFS finds no fifth.
func TestTheShortestWalkIsTaken(t *testing.T) {
	t.Parallel()

	route, _ := city.Route("east-side-apartment", "riverfront-gym", city.OnFoot)
	if len(route) != 4 {
		t.Fatalf("the walk to the gym is %d steps, want 4: %v", len(route), keys(route))
	}
	if got := keys(route); got[0] != "east-side" || got[len(got)-1] != "riverfront-gym" {
		t.Errorf("the walk goes %v", got)
	}
}

// §3.12's trade, as one assertion: the same journey is free on foot and costs a
// fare on the train, and neither takes any longer than the other.
func TestTransitCostsMoneyAndWalkingCostsNothing(t *testing.T) {
	t.Parallel()

	walk, ok := city.Route("east-side", "riverfront", city.OnFoot)
	if !ok {
		t.Fatal("no walk across town")
	}
	ride, ok := city.Route("east-side", "riverfront", city.ByTransit)
	if !ok {
		t.Fatal("no ride across town")
	}

	if city.Cost(walk) != 0 {
		t.Errorf("the walk costs %d cents", city.Cost(walk).Cents())
	}
	if got, want := city.Cost(ride), city.Fare; got != want {
		t.Errorf("the ride costs %d cents, want %d", got.Cents(), want.Cents())
	}
	if len(ride) != 1 {
		t.Errorf("the ride is %d steps, want one — every stop is one ride from every other", len(ride))
	}
	if len(walk) <= len(ride) {
		t.Errorf("the walk is %d steps and the ride is %d — the fare buys nothing", len(walk), len(ride))
	}
}

// You cannot ride out of somebody's kitchen. Leaving an interior always begins
// on foot, which is the rule that keeps "transit skips what happens on the way"
// from also meaning "transit skips the door".
func TestThereIsNoRideOutOfAnInterior(t *testing.T) {
	t.Parallel()

	if _, ok := city.Route("east-side-apartment", "riverfront", city.ByTransit); ok {
		t.Error("the transit network calls at the player's apartment")
	}
	if _, ok := city.Route("east-side", "riverfront-gym", city.ByTransit); ok {
		t.Error("the transit network puts you down inside the gym")
	}
	// Out of the flat, and then the ride is there.
	if _, ok := city.Route("east-side", "riverfront", city.ByTransit); !ok {
		t.Error("no ride between two streets that both have a stop")
	}
}

// Every interior sits in a district, is one step from it, and that step is a
// walk. This is the layering §3.12 asks for, asserted rather than assumed.
func TestEveryInteriorIsOneStepFromItsDistrict(t *testing.T) {
	t.Parallel()

	for _, p := range city.Places() {
		switch p.Layer {
		case city.District:
			if p.In != "" {
				t.Errorf("%s is a district inside %s", p.Key, p.In)
			}
			if !p.Stop {
				t.Errorf("%s is a district with no transit stop", p.Key)
			}
		case city.Interior, city.Home:
			outer, ok := city.Lookup(p.In)
			if !ok || outer.Layer != city.District {
				t.Fatalf("%s sits inside %q, which is not a district", p.Key, p.In)
			}
			if p.Stop {
				t.Errorf("%s is indoors and has a transit stop", p.Key)
			}
			route, ok := city.Route(p.Key, outer.Key, city.OnFoot)
			if !ok || len(route) != 1 {
				t.Errorf("%s is %d steps from %s, want one", p.Key, len(route), outer.Key)
			}
		default:
			t.Errorf("%s is on the %q layer, which is not one of §3.12's", p.Key, p.Layer)
		}
	}
}

// There is exactly one home, and it is where a new character lives.
func TestTheStartingSituationIsSomewhereRealToStand(t *testing.T) {
	t.Parallel()

	at, ok := city.Lookup(city.Start)
	if !ok || at.Layer != city.District {
		t.Errorf("a new character starts at %q, which is not a district", city.Start)
	}
	lives, ok := city.Lookup(city.Lives)
	if !ok || lives.Layer != city.Home {
		t.Errorf("a new character lives at %q, which is not a home", city.Lives)
	}
	if lives.In != at.Key {
		t.Errorf("home is in %s but the character starts in %s", lives.In, at.Key)
	}

	homes := 0
	for _, p := range city.Places() {
		if p.Layer == city.Home {
			homes++
		}
	}
	if homes != 1 {
		t.Errorf("%d homes in the city, want one until §3.12's relocation (#31)", homes)
	}
}

// Nowhere leads nowhere, and nowhere is not a place.
func TestSomewhereThatIsNotInTheCity(t *testing.T) {
	t.Parallel()

	if city.Known("atlantis") {
		t.Error("atlantis is in the city")
	}
	if _, ok := city.Route("atlantis", "downtown", city.OnFoot); ok {
		t.Error("there is a walk out of atlantis")
	}
	if _, ok := city.Route("downtown", "atlantis", city.ByTransit); ok {
		t.Error("the train stops at atlantis")
	}
	if got := city.WaysFrom("atlantis"); len(got) != 0 {
		t.Errorf("atlantis has %d ways out", len(got))
	}
}

// The same question asked twice gets the same answer, in the same order —
// which is what lets a screen render a list of ways without it reshuffling.
func TestTheSameQuestionGetsTheSameAnswer(t *testing.T) {
	t.Parallel()

	for i := range 20 {
		first, _ := city.Route("east-side-apartment", "the-strip-bar", city.OnFoot)
		second, _ := city.Route("east-side-apartment", "the-strip-bar", city.OnFoot)
		if len(first) != len(second) {
			t.Fatalf("run %d found routes of %d and %d steps", i, len(first), len(second))
		}
		for j := range first {
			if first[j] != second[j] {
				t.Fatalf("run %d, step %d: %+v then %+v", i, j, first[j], second[j])
			}
		}
	}

	ways := city.WaysFrom("east-side")
	for i := 1; i < len(ways); i++ {
		if ways[i-1].Mode == city.ByTransit && ways[i].Mode == city.OnFoot {
			t.Error("the ways out of the east side are not walks first")
		}
	}
}

// The list handed out is a copy. A caller that sorted it would otherwise be
// re-authoring the city for every other caller in the process.
func TestTheCityCannotBeEditedByReadingIt(t *testing.T) {
	t.Parallel()

	all := city.Places()
	all[0] = city.Place{Key: "atlantis"}
	if again := city.Places(); again[0].Key == "atlantis" {
		t.Error("reading the city changed it")
	}

	out := city.WaysFrom("east-side")
	if len(out) > 0 {
		out[0] = city.Way{To: "atlantis", Fare: simulation.Dollars(1000)}
		if again := city.WaysFrom("east-side"); again[0].To == "atlantis" {
			t.Error("reading the ways out of the east side changed them")
		}
	}
}

func keys(route []city.Way) []string {
	out := make([]string, 0, len(route))
	for _, w := range route {
		out = append(out, w.To)
	}
	return out
}
