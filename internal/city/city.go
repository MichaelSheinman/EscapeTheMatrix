/*
Package city is the world's geography: every place a player can stand, and every
way of getting from one to another.

HighLevelOverview.md §3.12 settles the shape. **One city, built deep, freely
walkable** — districts with real character, enterable interiors inside them, and
a home you live in. The player **walks** between locations or **takes transit**,
and neither costs game time (§3.1). What separates the two is what happens on
the way: walking is free and is where things happen to you; transit costs money
and skips them. "A player who always takes the fastest route sees less of their
own life."

# Why this is its own package

`internal/character` owns *where a player is*, which is a row in a table. What is
here is *where there is to be* — authored content with no state in it at all, no
database, and no clock. Keeping the two apart is what lets the graph be edited
without a migration, and what stops a place list ending up half in a schema.

It is the geographic counterpart of `internal/simulation`: pure, deterministic,
and testable in microseconds. The only thing it imports from the rest of the
repository is `simulation.Money`, because a fare is money and there is one type
for that.

# The layers, and what a portal is

§3.12's layers are home grid → interior → district → city map. Three of those
are places you can be standing in, and they are the [Layer] values below. **The
city map is not a place** — it is the district layer seen from above, a way of
choosing where to go rather than somewhere to go. Making it a place would have
made it a free teleport between districts, which is exactly the trade §3.12
asks for money to buy.

A **portal** — the object an author draws in Tiled (technical.md §10.3) — is the
*physical placement* of a [Way]. The way is the truth and lives here; the portal
is where in the painting you walk to in order to take it. That direction matters:
a district nobody has painted yet still has its ways, so the map screen works
everywhere and the paintings arrive one at a time (#35) without the world
shrinking to fit them.

# What is authored, and what is derived

The three tables below — [places], [streets], [doors] and the transit [lines] —
are the whole of the city. Everything else in this file reads them. §17 leaves
the city and its district list open, so adding a district is an entry in one
table and a line in another, and nothing anywhere else changes.
*/
package city

import (
	"fmt"
	"sort"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

// Layer is which of §3.12's layers a place belongs to.
//
// Strings rather than an integer, because these cross the wire to the client and
// end up in a JSON body a person reads while debugging. The ordering that
// matters — home is inside an interior layer is inside a district — is a fact
// about the ways, not about this type.
type Layer string

const (
	// District is a street: the layer you walk along.
	District Layer = "district"
	// Interior is an enterable building inside a district (§3.12: 6–10 of them
	// per district, all hand-built).
	Interior Layer = "interior"
	// Home is where the player lives. Its own layer rather than an interior,
	// because §2's glossary gives it its own presentation — "shown on a grid"
	// — and because "walk home" has to be a thing the world can name.
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
	// Note is the place's character in a few words. It is copy, and it will
	// move to content/ (§5.2) with the rest of the authored text.
	Note string

	Layer Layer
	// In is the district this place sits inside. Empty for a district, which
	// sits inside nothing — the city map is a view rather than a place.
	In string
	// Stop is whether the transit network calls here.
	//
	// Only streets have stops, which is the rule that gives transit an edge
	// case worth having: you cannot ride out of somebody's kitchen, so leaving
	// an interior always begins on foot.
	Stop bool
}

// Way is one step from one place to another, taken one way.
//
// Ways are directed and authored in pairs, because a door is a door from both
// sides but the sentence describing it is not: "in through the door" and "out to
// the street" are different things to read on a button.
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

// Fare is what one ride costs, anywhere on the network.
//
// A flat fare rather than a distance one: §3.12's trade is not about speed —
// neither mode costs game time — so a fare that grew with distance would only
// be pricing something the player is not buying. What they are buying is
// *skipping what happens on the way*, and that is worth the same everywhere.
//
// §17 leaves the economy's numbers open. This is one constant, in one place.
const Fare = simulation.Money(275) // $2.75

// places is every place in the city.
//
// §3.12 asks for roughly 6–10 enterable interiors per district. What is here is
// the starting roster — enough for the graph to be a graph and for every
// district to have somewhere to go — and it grows by adding lines. None of them
// is painted yet except the home; #35 is where the paintings arrive, and an
// unpainted place is drawn as ground with its name on it rather than hidden.
var places = []Place{
	// ── the districts (§3.12's four) ──────────────────────────────────────
	{Key: "east-side", Name: "East Side", Note: "walk-ups, corner shops, and where you live",
		Layer: District, Stop: true},
	{Key: "downtown", Name: "Downtown", Note: "offices, transit, and everybody in a hurry",
		Layer: District, Stop: true},
	{Key: "riverfront", Name: "Riverfront", Note: "where the money lives",
		Layer: District, Stop: true},
	{Key: "the-strip", Name: "The Strip", Note: "warehouses, loading bays, night work",
		Layer: District, Stop: true},

	// ── the east side ─────────────────────────────────────────────────────
	{Key: "east-side-apartment", Name: "Your apartment", Note: "one room, and a window onto the fire escape",
		Layer: Home, In: "east-side"},
	{Key: "east-side-corner-shop", Name: "The corner shop", Note: "open late, and the till knows your face",
		Layer: Interior, In: "east-side"},
	{Key: "east-side-laundromat", Name: "The laundromat", Note: "warm, loud, and nobody is in a hurry",
		Layer: Interior, In: "east-side"},
	{Key: "east-side-clinic", Name: "The walk-in clinic", Note: "a waiting room and a number on a screen",
		Layer: Interior, In: "east-side"},

	// ── downtown ──────────────────────────────────────────────────────────
	{Key: "downtown-office", Name: "The office", Note: "open plan, and somebody else's chair",
		Layer: Interior, In: "downtown"},
	{Key: "downtown-station", Name: "Union Station", Note: "the concourse, and the board that never settles",
		Layer: Interior, In: "downtown"},
	{Key: "downtown-diner", Name: "The diner", Note: "counter service, and coffee that keeps coming",
		Layer: Interior, In: "downtown"},
	{Key: "downtown-library", Name: "The public library", Note: "quiet, free, and open to anyone",
		Layer: Interior, In: "downtown"},

	// ── the riverfront ────────────────────────────────────────────────────
	{Key: "riverfront-bank", Name: "The bank", Note: "marble, and a queue that moves",
		Layer: Interior, In: "riverfront"},
	{Key: "riverfront-gym", Name: "The gym", Note: "memberships, mirrors, and a fee",
		Layer: Interior, In: "riverfront"},
	{Key: "riverfront-tower", Name: "The tower lobby", Note: "a desk, a turnstile, and a lift you need a card for",
		Layer: Interior, In: "riverfront"},

	// ── the strip ─────────────────────────────────────────────────────────
	{Key: "the-strip-warehouse", Name: "The warehouse", Note: "pallets, a forklift, and a clipboard",
		Layer: Interior, In: "the-strip"},
	{Key: "the-strip-loading-bay", Name: "The loading bay", Note: "the night shift, and a roller door",
		Layer: Interior, In: "the-strip"},
	{Key: "the-strip-bar", Name: "The bar", Note: "late, dark, and everybody knows somebody",
		Layer: Interior, In: "the-strip"},
}

// streets is which districts are next to which, walkable in both directions.
//
// A shape rather than a clique: the city has a geography, and "the riverfront is
// two walks from the east side" is the fact that makes a $2.75 ride worth
// thinking about at all.
var streets = [][2]string{
	{"east-side", "downtown"},
	{"east-side", "the-strip"},
	{"downtown", "riverfront"},
	{"downtown", "the-strip"},
	{"riverfront", "the-strip"},
}

// lines is the transit network: each entry is a line, and every stop on it is
// one ride from every other.
//
// One line at V1, calling at all four districts. A second line would need
// interchange rules and a reason for them, and there is neither yet.
var lines = []struct {
	Name  string
	Stops []string
}{
	{Name: "the Crosstown", Stops: []string{"east-side", "downtown", "riverfront", "the-strip"}},
}

// Start is where a character stands and lives before anything else has happened.
//
// §3.12 names the east side as the player's own neighbourhood, and §3.13's
// onboarding (#33) is where this becomes a choice with visible consequences.
// Until then it is the same for everyone, stated once.
const (
	Start = "east-side"
	Lives = "east-side-apartment"
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
		byKey[p.Key] = p
	}

	// Doors: every interior and every home is one step from the district it
	// sits inside, on foot and free. Derived rather than authored, because a
	// place that named a district it was not connected to would be a place
	// nobody could reach, and there is no reading of §3.12 in which that is
	// something an author should be able to write by accident.
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
		link(Way{From: outer.Key, To: p.Key, Mode: OnFoot, Name: "in through the door"})
		link(Way{From: p.Key, To: outer.Key, Mode: OnFoot, Name: "out to " + outer.Name})
	}

	for _, s := range streets {
		a, b := must(s[0]), must(s[1])
		link(Way{From: a.Key, To: b.Key, Mode: OnFoot, Name: "on foot to " + b.Name})
		link(Way{From: b.Key, To: a.Key, Mode: OnFoot, Name: "on foot to " + a.Name})
	}

	for _, line := range lines {
		for _, from := range line.Stops {
			for _, to := range line.Stops {
				if from == to {
					continue
				}
				a, b := must(from), must(to)
				if !a.Stop || !b.Stop {
					panic(fmt.Sprintf("city: %s calls at %q, which is not a stop", line.Name, from))
				}
				link(Way{From: a.Key, To: b.Key, Mode: ByTransit,
					Name: line.Name + " to " + b.Name, Fare: Fare})
			}
		}
	}

	// Stable order, so the wire — and therefore a screen — never reshuffles
	// between two identical requests.
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

// WaysFrom is every step out of a place, walks first.
func WaysFrom(key string) []Way {
	out := make([]Way, len(ways[key]))
	copy(out, ways[key])
	return out
}

/* ── getting from here to there ────────────────────────────────────────── */

// Route is the steps from one place to another, taken one way.
//
// Breadth-first, so the answer is the fewest steps. Every walk is free and no
// step costs game time, so there is nothing else a route could be cheaper in —
// which is the whole reason this is a search over hops rather than a weighted
// one. Determinism comes from the authored order the ways are held in: two calls
// with the same arguments return the same route, always.
//
// An empty slice with ok = true is "you are already there", which is a real
// answer and not a refusal: a client that lost the response to its first attempt
// has to be able to repeat it.
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
