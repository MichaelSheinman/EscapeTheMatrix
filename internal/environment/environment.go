/*
Package environment is who the player is among: the populations of the game, and
what living among each of them costs and affords.

HighLevelOverview.md §3.15 is the specification, and it opens with the sentence
this whole package is built to obey: **"an environment is defined by its people,
not its geography."** A trip needs no new country and no new culture — it needs a
different population with a different value system. So what is authored here is a
*set of characters with shared values*, reusing the locations internal/city
already has, and the thing that changes when a player relocates is who they are
among rather than what the ground looks like.

# Why this is its own package, and why it has no database

It is the third pure package in the backend, alongside internal/simulation and
internal/city, and it is pure for the same reason both of those are: there is no
state in a population. Which people exist, what they hold in common and what
their world affords are authored content (technical.md §5.2), identical in every
process, loaded and validated at boot so that a malformed population fails in CI
rather than at two in the morning.

What *is* state — how worn a particular player has made a particular population —
belongs to internal/factor, which owns `factor_tenure` and keys it by the [Key]
of an environment in this package. That split is the whole of §3.15's "decay is
per environment and per population": the curve is in the pure core, the wear is a
row, and the identity the row is filed under is a line of authored content.

# The four things a population is

  - **Its people.** Six of them apiece, each with a name, a note, somewhere in
    the city they turn up, and — the load-bearing field — one sentence of what
    they *do*. §3.15: "characters must be consistent within an environment; the
    population's values have to be legible purely through behaviour."
  - **What it holds in common.** The values, written as behaviour rather than as
    adjectives. This is what #30's daily conflicts are generated against and what
    makes a rule set right or wrong here.
  - **What it affords.** §3.15's factor table, per population: an economic
    ceiling in dollars, and caps on emotional stability and kindness that come
    from how much situational variety the place actually has. "Low money ceiling,
    high relational reward" versus "high money ceiling, punishes trust" is the
    design's own axis and these are it, as numbers.
  - **Where it is.** One district, so a population can be found and drawn.
    Geography is the *last* property of an environment here rather than the
    first, which is the inversion §3.15 asks for.

# Nothing is ever labelled

§3.2, §3.10 and §3.15 all say it and this package is where it would be easiest to
break, because the design document names the two registers V1 ships in as many
words. So [Labelled] is a load-bearing check that runs over every player-facing
string at boot: a population whose copy names a register, a stage, a framework or
a developmental colour is a **failed build**, not a review comment. Stage names
are internal production vocabulary; there is no file under content/ they are
allowed into.

The Go identifiers here are no exception. There is no Register type, no Colour
field and no comment in the shipped content naming which of §3.10's two registers
a population is — what is written down is what these people do, and working out
that different people need different rules is the game.
*/
package environment

import (
	"sort"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
Nowhere is the environment key for a player who is not among anybody.

It is not a population and [Lookup] does not find it: what it names is the
absence of one, and what an absence affords is *no ceiling at all* rather than a
ceiling of nothing — which is the pure core's own convention for a zero
[simulation.Ceilings] and the reason this is a key rather than an entry.

It exists so that the wear a player accumulates is always filed under something.
A tenure row keyed by the empty string is a row nobody can read back.
*/
const Nowhere = "nowhere"

/*
Person is one member of a population.

`Does` is the one field that carries the design. §3.15 requires a population's
values to be legible "purely through behaviour, never labelled", so a person is
described by what they can be relied on to do and never by what they are like —
there is no disposition field, no alignment, and no adjective that could be
rendered as a badge.

`Found` is a key from internal/city: the district the population is around, or
one of the interiors inside it. It is where this person turns up, which is what
lets #30 place a conflict and #35 paint somebody standing there. Reusing existing
locations rather than authoring new ones is §3.15's own instruction.
*/
type Person struct {
	Key   string
	Name  string
	Note  string
	Found string
	Does  string
}

/*
Environment is one population, whole.

The unexported slices are behind methods for the same reason internal/city hands
back copies: this is a package-level value read by every request in the process,
and a caller that could append to it would be a caller that could edit the world
for everybody else.
*/
type Environment struct {
	// Key is the stable identifier. It is what `factor_tenure`, `playbook_rule`
	// and `job_situation` file a row under, so renaming one is a migration.
	Key string
	// Name is what a player reads. A population, never a place and never a
	// register — see [Labelled].
	Name string
	// Note is who these people are, in a sentence or two.
	Note string
	// Around is the internal/city district this population lives in. §3.15's V1
	// geography is one city with different homes and different people, so a
	// district is where a population *is* rather than what it is.
	Around string

	// Ceilings is §3.15's factor table for this population. Intelligence is
	// deliberately zero: it is a fact about the player's breadth rather than
	// about any place, and the pure core's [simulation.Reach] is what fills it.
	Ceilings simulation.Ceilings

	holds  []string
	people []Person
}

// Holds is what this population has in common, in authored order.
//
// Behaviour rather than values-as-nouns. It is the material #30's daily
// conflicts are written against and the reason a rule set that works here fails
// somewhere else.
func (e Environment) Holds() []string {
	out := make([]string, len(e.holds))
	copy(out, e.holds)
	return out
}

// People is everyone in this population, in authored order.
func (e Environment) People() []Person {
	out := make([]Person, len(e.people))
	copy(out, e.people)
	return out
}

// Person finds one member by key.
func (e Environment) Person(key string) (Person, bool) {
	for _, p := range e.people {
		if p.Key == key {
			return p, true
		}
	}
	return Person{}, false
}

/*
Places is every location in the city this population turns up in, most-used
first, ties broken by the order the people were authored in.

Derived rather than authored, because a population that named its own haunts
could name one none of its people is ever at. It is what a paint pass (#35) and a
conflict pool (#30) both need: not "where is the riverfront", but "where would
you actually run into these six".
*/
func (e Environment) Places() []string {
	count := map[string]int{}
	first := map[string]int{}
	var order []string
	for i, p := range e.people {
		if _, seen := count[p.Found]; !seen {
			first[p.Found] = i
			order = append(order, p.Found)
		}
		count[p.Found]++
	}
	sort.SliceStable(order, func(i, j int) bool {
		if count[order[i]] != count[order[j]] {
			return count[order[i]] > count[order[j]]
		}
		return first[order[i]] < first[order[j]]
	})
	return order
}

// All is every population, in key order.
//
// Key order rather than authored order, because these are separate files and a
// directory walk is the only thing that decided which came first. §7's
// determinism: two processes offer the world's populations in the same order.
func All() []Environment {
	out := make([]Environment, len(catalogue))
	copy(out, catalogue)
	return out
}

// Lookup finds a population by key. [Nowhere] is not one.
func Lookup(key string) (Environment, bool) {
	for _, e := range catalogue {
		if e.Key == key {
			return e, true
		}
	}
	return Environment{}, false
}

/*
Around is the population that lives in a district.

One at most: a district with two populations in it would make "where am I" a
question with two answers, and every table that files a row under an environment
key would have to guess. The loader refuses it.
*/
func Around(district string) (Environment, bool) {
	for _, e := range catalogue {
		if e.Around == district {
			return e, true
		}
	}
	return Environment{}, false
}

/*
Ceilings is what an environment key affords, as the pure core takes it.

An unknown key — [Nowhere], a district that lost its population, a row written by
a build that authored things differently — is **no ceiling**, which is the zero
value and is deliberately not an error. §3.15's caps limit what a place can
teach; a place the world no longer describes has stopped limiting anything, and a
player standing in one should not have their reading of themselves fail.
*/
func Ceilings(key string) simulation.Ceilings {
	e, ok := Lookup(key)
	if !ok {
		return simulation.Ceilings{}
	}
	return e.Ceilings
}
