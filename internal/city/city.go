/*
Package city is the world's geography: every place a player can stand, and every
way of getting from one to another.

Wilmington, built deep and freely walkable (§3.12): one district, the enterable
interiors inside it, and a home. Walking costs no game time (§3.1) and is where
things happen to you.

internal/character owns where a player is, which is a row in a table. What is
here is where there is to be: authored content with no state, no database and no
clock, so the graph can be edited without a migration. Its only import from the
rest of the repository is [simulation.Money], because a fare is money.

§3.12's layers are home grid → interior → district. Each is a place you can
stand in and is one of the [Layer] values below.

A portal, the object an author draws in Tiled (§10.3), is the physical placement
of a [Way]. The way is the truth and lives here. A place nobody has painted yet
still has its ways.

[places] and the doors derived from them are the whole of the city, and
everything else in this file reads them.
*/
package city

import (
	"fmt"
	"sort"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

// Layer is which of §3.12's layers a place belongs to. Strings rather than an
// integer, because these cross the wire and end up in a JSON body somebody reads
// while debugging.
type Layer string

const (
	// District is a street: the layer you walk along.
	District Layer = "district"
	// Interior is an enterable building inside a district (§3.12: 6–10 of them
	// per district, all hand-built).
	Interior Layer = "interior"
	// Home is where the player lives. Its own layer rather than an interior,
	// because it is shown on a grid (§2) and because "walk home" has to be
	// something the world can name.
	Home Layer = "home"
)

// Mode is how a step is taken. §3.12's trade is exactly these two.
type Mode string

const (
	// OnFoot is free, and is where encounters happen (§3.9, §3.12).
	OnFoot Mode = "foot"
	// ByTransit costs a fare and passes through nothing.
	ByTransit Mode = "transit"
)

// Modes is every mode, in the order a screen should offer them.
var Modes = []Mode{OnFoot, ByTransit}

// ModeOf reads a mode off the wire, refusing anything that is not one.
func ModeOf(s string) (Mode, bool) {
	for _, m := range Modes {
		if Mode(s) == m {
			return m, true
		}
	}
	return "", false
}

// Place is somewhere a player can be standing.
type Place struct {
	// Key is what the database and the wire carry. Stable; the label is not.
	Key string
	// Name is what a player reads.
	Name string
	// Note is the place's character in a few words.
	Note string

	Layer Layer
	// In is the district this place sits inside. Empty for a district, which
	// sits inside nothing — the city map is a view rather than a place.
	In string
	// Off is the place whose door this one opens off, when that door is not
	// onto the street. The place named sits in the same district, and the way
	// between the two is the only way in or out.
	Off string
	// Stop is whether the transit network calls here. Only streets have stops:
	// you cannot ride out of somebody's kitchen, so leaving an interior always
	// begins on foot.
	Stop bool

	// Zones are the named spots inside this place: the stoop, the bus stop,
	// the counter, the bed. The trigger zones an author draws in Tiled and the
	// placement rules an event is authored with are the same objects, and the
	// city names them.
	//
	// Order is priority. An event whose placement allows two of a place's
	// zones is put in the first one this list names.
	Zones []string
}

// Way is one step from one place to another, taken one way. Ways are directed
// and authored in pairs, because a door is a door from both sides but the
// sentence describing it is not.
type Way struct {
	From string
	To   string
	Mode Mode
	// Name is the step as a sentence fragment — what taking it reads as.
	Name string
	// Fare is what the step costs. Zero for every walk, by design (§3.12).
	Fare simulation.Money
}

/* ── the city, as authored ─────────────────────────────────────────────── */

// Fare is what one ride costs, anywhere on the network. Flat rather than by
// distance: neither mode costs game time, so what the player is buying is
// skipping what happens on the way, and that is worth the same everywhere.
const Fare = simulation.Money(275) // $2.75

/*
zones is every name a spot inside a place may have — the closed vocabulary the
paintings' trigger zones and §4's placement rules are both written in.

Closed, and validated at init, because the whole value of the two systems
sharing a vocabulary is that a typo is a failed build rather than an event that
silently never appears. It is small on purpose: a zone is a *kind* of spot, not
a piece of furniture, so an event authored "at the counter" is placeable in
every shop in the city without naming any of them.

Adding one is a line here and a trigger in whichever paintings have such a spot.
*/
var zones = map[string]string{
	// Out of doors.
	"on-foot":      "walking, out in the open",
	"transit-stop": "where the network calls",
	"home-door":    "the door to where you live, from the street",
	"corner-shop":  "the shopfront, from the pavement",

	// Indoors, anywhere.
	"doorway": "just inside the door",
	"counter": "where somebody serves you",
	"seats":   "where people wait",
	"desk":    "somebody's desk",
	"dock":    "where things are loaded",

	// Home.
	"at-home":  "in your own place",
	"bed":      "the bed",
	"kitchen":  "the counter you cook at",
	"window":   "the window",
	"washroom": "the basin, the pan and the shower",
}

// Zone reports whether a key names a kind of spot.
func Zone(key string) bool {
	_, ok := zones[key]
	return ok
}

// places is every place in the city. §3.12 asks for roughly 6–10 enterable
// interiors per district, and this grows by adding lines. An unpainted place is
// drawn as ground with its name on it rather than hidden.
var places = []Place{
	// ── the district ──────────────────────────────────────────────────────
	//
	// Wilmington's zones are exactly the trigger zones drawn in its painting
	// (web/public/locations/wilmington), which is the correspondence §10.3 is
	// about.
	{Key: "wilmington", Name: "Wilmington", Note: "walk-ups, corner shops, and thin walls",
		Layer: District, Stop: true,
		Zones: []string{"home-door", "corner-shop", "transit-stop", "on-foot"}},

	// ── the places inside it ──────────────────────────────────────────────
	{Key: "wilmington-apartment", Name: "Your apartment", Note: "three flights up, one room and a washroom",
		Layer: Home, In: "wilmington",
		Zones: []string{"bed", "kitchen", "window", "at-home"}},
	{Key: "wilmington-apartment-washroom", Name: "The washroom", Note: "a basin, a mirror, and a tiled floor",
		Layer: Home, In: "wilmington", Off: "wilmington-apartment",
		Zones: []string{"washroom", "doorway"}},
	{Key: "wilmington-corner-shop", Name: "The corner shop", Note: "open late, and the till knows your face",
		Layer: Interior, In: "wilmington",
		Zones: []string{"counter", "doorway"}},
	// The bodega on the corner, and what is inside it is a separate game —
	// served from web/public/the-house and opened over the world by
	// chrome/TheHouse when a character walks in.
	{Key: "wilmington-house", Name: "VR Games", Note: "boarded at the front, and the side door opens",
		Layer: Interior, In: "wilmington",
		Zones: []string{"doorway", "window"}},
	// The dance studio on the strip, and what is inside it is a separate game —
	// Bemuse, served from web/public/bemuse and opened over the world by
	// chrome/TheStudio when a character walks in.
	{Key: "wilmington-dance", Name: "Dance Studio", Note: "scuffed wood floor, folding chairs along the mirror, music loud",
		Layer: Interior, In: "wilmington",
		Zones: []string{"doorway", "window"}},
	{Key: "wilmington-laundromat", Name: "The laundromat", Note: "warm, loud, and nobody is in a hurry",
		Layer: Interior, In: "wilmington",
		Zones: []string{"seats", "doorway"}},
	{Key: "wilmington-clinic", Name: "Rowan St Clinic", Note: "a waiting room and a number on a screen",
		Layer: Interior, In: "wilmington",
		Zones: []string{"counter", "seats", "doorway"}},
	{Key: "wilmington-diner", Name: "Verona Diner", Note: "counter service, and coffee that keeps coming",
		Layer: Interior, In: "wilmington",
		Zones: []string{"counter", "seats", "doorway"}},
	{Key: "wilmington-bank", Name: "First Christina", Note: "marble, and a queue that moves",
		Layer: Interior, In: "wilmington",
		Zones: []string{"counter", "seats", "doorway"}},
	{Key: "wilmington-gym", Name: "Ironworks Gym", Note: "memberships, mirrors, and a fee",
		Layer: Interior, In: "wilmington",
		Zones: []string{"counter", "doorway"}},
	{Key: "wilmington-tower", Name: "The tower lobby", Note: "a desk, a turnstile, and a lift you need a card for",
		Layer: Interior, In: "wilmington",
		Zones: []string{"desk", "counter", "doorway"}},
	{Key: "wilmington-kestrel", Name: "Kestrel & Vane", Note: "billed by the hour, and everybody here knows it",
		Layer: Interior, In: "wilmington",
		Zones: []string{"desk", "seats", "doorway"}},
	{Key: "wilmington-freight", Name: "Corrin Freight", Note: "pallets, a forklift, and a clipboard",
		Layer: Interior, In: "wilmington",
		Zones: []string{"dock", "desk", "doorway"}},
	{Key: "wilmington-loading-bay", Name: "The loading bay", Note: "the night shift, and a roller door",
		Layer: Interior, In: "wilmington",
		Zones: []string{"dock", "doorway"}},
	{Key: "wilmington-fenn", Name: "Fenn Logistics", Note: "open plan, and somebody else's chair",
		Layer: Interior, In: "wilmington",
		Zones: []string{"desk", "counter", "doorway"}},
}

// Start is where a character stands and lives before anything else has happened.
// §3.12 names Wilmington as the player's own neighbourhood, and it is the
// same for everyone until onboarding makes it a choice.
const (
	Start = "wilmington"
	Lives = "wilmington-apartment"
)

/* ── the graph, derived once ───────────────────────────────────────────── */

var (
	byKey = map[string]Place{}
	ways  = map[string][]Way{}
)

func init() {
	for _, p := range places {
		if _, clash := byKey[p.Key]; clash {
			panic(fmt.Sprintf("city: two places called %q", p.Key))
		}
		if len(p.Zones) == 0 {
			// A place with no named spot is a place nothing can be placed in
			// (§4) — an unfinished line rather than an intention.
			panic(fmt.Sprintf("city: %q names no zones", p.Key))
		}
		seen := map[string]bool{}
		for _, z := range p.Zones {
			if !Zone(z) {
				panic(fmt.Sprintf("city: %q names the zone %q, which is not a kind of spot", p.Key, z))
			}
			if seen[z] {
				panic(fmt.Sprintf("city: %q names the zone %q twice", p.Key, z))
			}
			seen[z] = true
		}
		byKey[p.Key] = p
	}

	// Doors: every interior and every home is one step from the district it
	// sits inside, on foot and free. Derived rather than authored, so a place
	// that named a district it was not connected to cannot exist.
	for _, p := range places {
		if p.In == "" {
			continue
		}
		outer, ok := byKey[p.In]
		if !ok {
			panic(fmt.Sprintf("city: %q sits inside %q, which is not a place", p.Key, p.In))
		}
		if outer.Layer != District {
			panic(fmt.Sprintf("city: %q sits inside %q, which is not a district", p.Key, p.In))
		}
		// A place with a door onto another place is reached through that one
		// and no other way. Everything else opens onto its own district.
		through := outer
		if p.Off != "" {
			off, ok := byKey[p.Off]
			if !ok {
				panic(fmt.Sprintf("city: %q opens off %q, which is not a place", p.Key, p.Off))
			}
			if off.In != p.In {
				panic(fmt.Sprintf("city: %q opens off %q, which is in another district", p.Key, p.Off))
			}
			through = off
		}
		link(Way{From: through.Key, To: p.Key, Mode: OnFoot, Name: "in through the door"})
		link(Way{From: p.Key, To: through.Key, Mode: OnFoot, Name: "out to " + through.Name})
	}

	// Stable order, so the wire never reshuffles between identical requests.
	for key := range ways {
		out := ways[key]
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].Mode != out[j].Mode {
				return out[i].Mode == OnFoot
			}
			return order(out[i].To) < order(out[j].To)
		})
	}
}

func link(w Way) { ways[w.From] = append(ways[w.From], w) }

func must(key string) Place {
	p, ok := byKey[key]
	if !ok {
		panic(fmt.Sprintf("city: %q is not a place", key))
	}
	return p
}

// order is a place's position in the authored list, which is the order every
// list of places in this package comes out in.
func order(key string) int {
	for i, p := range places {
		if p.Key == key {
			return i
		}
	}
	return len(places)
}

/* ── reading the city ──────────────────────────────────────────────────── */

// Places is the whole city, in the order it was authored.
func Places() []Place {
	out := make([]Place, len(places))
	copy(out, places)
	return out
}

// Lookup finds a place by key.
func Lookup(key string) (Place, bool) {
	p, ok := byKey[key]
	return p, ok
}

// Known reports whether a key names a place.
func Known(key string) bool {
	_, ok := byKey[key]
	return ok
}

// ZonesIn is the named spots inside a place, most specific first. Empty for a
// key that is not a place.
func ZonesIn(key string) []string {
	p, ok := byKey[key]
	if !ok {
		return nil
	}
	out := make([]string, len(p.Zones))
	copy(out, p.Zones)
	return out
}

// Spot reports whether a place has a particular kind of spot in it.
func Spot(place, zone string) bool {
	for _, z := range ZonesIn(place) {
		if z == zone {
			return true
		}
	}
	return false
}

// WaysFrom is every step out of a place, walks first.
func WaysFrom(key string) []Way {
	out := make([]Way, len(ways[key]))
	copy(out, ways[key])
	return out
}

/* ── getting from here to there ────────────────────────────────────────── */

// Route is the steps from one place to another, taken one way. Breadth-first,
// so the answer is the fewest steps, and determinism comes from the authored
// order of the ways. An empty slice with ok = true is "you are already there".
func Route(from, to string, mode Mode) ([]Way, bool) {
	if !Known(from) || !Known(to) {
		return nil, false
	}
	if from == to {
		return []Way{}, true
	}

	// The step that first reached each place, which is what the path is
	// reconstructed backwards from.
	came := map[string]Way{}
	seen := map[string]bool{from: true}
	queue := []string{from}

	for len(queue) > 0 {
		at := queue[0]
		queue = queue[1:]

		for _, w := range ways[at] {
			if w.Mode != mode || seen[w.To] {
				continue
			}
			seen[w.To] = true
			came[w.To] = w
			if w.To == to {
				return trace(came, from, to), true
			}
			queue = append(queue, w.To)
		}
	}
	return nil, false
}

// trace walks the arrival steps backwards and hands them back in order.
func trace(came map[string]Way, from, to string) []Way {
	var back []Way
	for at := to; at != from; {
		w := came[at]
		back = append(back, w)
		at = w.From
	}
	out := make([]Way, 0, len(back))
	for i := len(back) - 1; i >= 0; i-- {
		out = append(out, back[i])
	}
	return out
}

// Cost is what a route's fares add up to.
func Cost(route []Way) simulation.Money {
	var total simulation.Money
	for _, w := range route {
		total += w.Fare
	}
	return total
}
