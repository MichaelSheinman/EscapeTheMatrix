package city_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
)

/*
The named spots inside a place, and the one thing that can go silently wrong
with them.

technical.md §10.3 says the trigger zones an author draws in Tiled and the
placement rules an event is authored with (§4) are **the same objects**. Two
files in two languages saying the same thing is exactly the arrangement that
drifts, so the last test in here reads the paintings that actually ship and
holds them to the city's vocabulary.
*/

// Every place has somewhere for something to happen. A place with no zones is
// a place §4 can never place an event in, which is not a design decision
// anybody would take one place at a time.
func TestEveryPlaceHasSomewhereForSomethingToHappen(t *testing.T) {
	t.Parallel()

	for _, p := range city.Places() {
		zones := city.ZonesIn(p.Key)
		if len(zones) == 0 {
			t.Errorf("%s names no zones", p.Key)
			continue
		}
		for _, z := range zones {
			if !city.Zone(z) {
				t.Errorf("%s names the zone %q, which is not a kind of spot", p.Key, z)
			}
			if !city.Spot(p.Key, z) {
				t.Errorf("%s has the zone %q but Spot says otherwise", p.Key, z)
			}
		}
	}
}

// Every district can be walked in, and every district is a stop. Both are the
// zones an event placed "anywhere out of doors" is authored against, so an
// absence here takes a whole family of events out of a district without any
// error being raised anywhere.
func TestEveryDistrictCanBeWalkedInAndCalledAt(t *testing.T) {
	t.Parallel()

	for _, p := range city.Places() {
		if p.Layer != city.District {
			continue
		}
		if !city.Spot(p.Key, "on-foot") {
			t.Errorf("%s is a district with nowhere to be on foot", p.Key)
		}
		if p.Stop && !city.Spot(p.Key, "transit-stop") {
			t.Errorf("%s is a stop with no stop to stand at", p.Key)
		}
	}
}

// A key that is not a place has no zones, and nothing in it is a spot. The
// event module asks both questions about a place read out of a row, so the
// answer for a place that has been withdrawn from the city matters.
func TestNowhereHasNoZones(t *testing.T) {
	t.Parallel()

	if got := city.ZonesIn("the-moon"); len(got) != 0 {
		t.Errorf("the moon has zones: %v", got)
	}
	if city.Spot("the-moon", "on-foot") {
		t.Error("you can stand on the moon")
	}
	if city.Zone("the-hollow-of-the-hand") {
		t.Error("that is not a kind of spot")
	}
}

/*
The paintings agree with the city about what is in them.

This is the drift §10.3 invites: a painter renames a trigger, the event authored
against it stops being placeable, and nothing fails — the event simply never
appears again. So every trigger key drawn in every shipped map is held to the
city's closed vocabulary here, in the build rather than in a playtest.

It is deliberately one-directional. The city names spots for places nobody has
painted yet (#35), and a painting that has not yet drawn a stoop the world says
is there is a painting that is not finished — which is a ticket, not a failure.
*/
func TestEveryTriggerDrawnInAPaintingIsAKindOfSpot(t *testing.T) {
	t.Parallel()

	maps, err := filepath.Glob(filepath.Join("..", "..", "web", "public", "locations", "*", "*.tmj"))
	if err != nil {
		t.Fatalf("look for paintings: %v", err)
	}
	if len(maps) == 0 {
		// A tree with no paintings in it is a checkout, not a failure.
		t.Skip("no paintings ship in this tree")
	}

	for _, path := range maps {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}

		var painting struct {
			Layers []struct {
				Type    string `json:"type"`
				Objects []struct {
					Class      string `json:"class"`
					Properties []struct {
						Name  string `json:"name"`
						Value any    `json:"value"`
					} `json:"properties"`
				} `json:"objects"`
			} `json:"layers"`
		}
		if err := json.Unmarshal(raw, &painting); err != nil {
			t.Fatalf("read %s: %v", path, err)
		}

		for _, layer := range painting.Layers {
			if layer.Type != "objectgroup" {
				continue
			}
			for _, object := range layer.Objects {
				if object.Class != "trigger" {
					continue
				}
				for _, property := range object.Properties {
					if property.Name != "key" {
						continue
					}
					key, _ := property.Value.(string)
					if !city.Zone(key) {
						t.Errorf("%s draws a trigger zone called %q, which the city does not name",
							filepath.Base(path), key)
					}
				}
			}
		}
	}
}
