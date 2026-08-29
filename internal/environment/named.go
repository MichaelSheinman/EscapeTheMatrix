package environment

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

/*
Who a line is allowed to name.

A player reading "have Callum Thursday" can look Callum up, find nothing, and
learn that the name went nowhere. Every name that goes nowhere is a thread the
phone implies and does not have, so player-visible prose may name only people
the player has in their contacts — which is the population, and nothing else.

Somebody who is not in the contacts is referred to by relation or role: my son,
our lass, the landlord, the girl who does Fridays. That costs the writing
nothing and it means a name is always a lead.

# How a name is told from a place

It is not, by inspection: "Denny" and "Callum" are the same shape. So the
non-people are declared. [Nouns] is what a population's prose may say that is
capitalised and not a person — its streets, its shops, its landmarks — and
anything capitalised that is neither a neighbour nor on that list fails the
build. Declaring is the point: an author adding a proper noun says which kind it
is once, and the check does the rest.
*/

// Nouns is every capitalised word the world's prose may use that is not one of
// its people. Days, months and the handful of words that only ever appear
// capitalised; a population's own places come from its `nouns:`.
var Nouns = []string{
	"Monday", "Tuesday", "Wednesday", "Thursday", "Thursdays", "Friday", "Fridays",
	"Saturday", "Saturdays", "Sunday", "Sundays",
	"January", "February", "March", "April", "May", "June", "July", "August",
	"September", "October", "November", "December",
	"Christmas", "Easter", "God", "Mum", "Dad", "Mr", "Mrs", "Ms",
}

var capitalised = regexp.MustCompile(`[A-Z][a-z]+`)

/*
Named reports the first person a piece of prose names who is not among `known`,
or empty if it names nobody it should not.

`known` is every name the reader can look up — the population's people, by full
name and by first name — plus whatever the population declares as places.

A word that opens a sentence is skipped rather than resolved: "Everybody in the
neighbourhood calls her Auntie Pat" starts with a capital because sentences do,
and a check that read that as a person would refuse ordinary English.
*/
func Named(text string, known map[string]bool) string {
	for _, at := range capitalised.FindAllStringIndex(text, -1) {
		word := text[at[0]:at[1]]
		if known[word] || opens(text, at[0]) {
			continue
		}
		return word
	}
	return ""
}

// opens reports whether the word at `i` is the first of a sentence.
func opens(text string, i int) bool {
	for i--; i >= 0; i-- {
		switch text[i] {
		case ' ', '\t', '\n', '"', '\'', '(':
			continue
		case '.', '!', '?':
			return true
		default:
			return false
		}
	}
	return true
}

// vocabulary is every capitalised word this population's prose may use.
func vocabulary(e Environment, nouns []string) map[string]bool {
	out := map[string]bool{}
	for _, n := range Nouns {
		out[n] = true
	}
	for _, n := range nouns {
		for _, word := range strings.Fields(n) {
			out[strings.Trim(word, ".,'s")] = true
		}
	}
	for _, p := range e.people {
		for _, word := range strings.Fields(p.Name) {
			out[word] = true
		}
	}
	return out
}

/*
naming checks every player-visible string a population has against its own
vocabulary.

Run over the whole population at once because the answer depends on who else
lives there: a neighbour may be named by anybody in the building, and which
names those are is not knowable from one file.
*/
func naming(e Environment, nouns []string) error {
	known := vocabulary(e, nouns)
	for _, p := range e.people {
		said := map[string]string{
			"note": p.Note, "where they live": p.Lives,
			"how you know them": p.Known,
		}
		for i, turn := range p.Intro {
			said[fmt.Sprintf("how you met (%d)", i+1)] = turn.Says
		}
		// Sorted, so a file with two problems in it reports the same one every
		// time and a fix is not chasing a moving message (§7).
		for _, what := range sorted(said) {
			if who := Named(said[what], known); who != "" {
				return fmt.Errorf(
					"%q's %s names %q, who is nobody the player can look up; "+
						"say what they are to them instead, or add %q to the population's nouns",
					p.Key, what, who, who)
			}
		}
	}
	return nil
}

func sorted(of map[string]string) []string {
	out := make([]string, 0, len(of))
	for k := range of {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
