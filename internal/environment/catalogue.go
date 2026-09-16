package environment

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/MichaelSheinman/EscapeTheMatrix/content"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
The catalogue: every population the world has, parsed and checked at boot.

Adding one is a file in `content/environments/` named for its key, and nothing
else anywhere. That is the property this loader exists to buy — §3.15 leaves
"how many environments ship in V1" open, and an open question is only cheap to
answer later if answering it is content.

Everything a caller then gets to assume is checked here rather than at the
moment a player happens to reach it: that a population is somewhere real, that
its people are somewhere real inside that, that its affordances are on the scales
the pure core reads them on, and — the check that is the design rather than
hygiene — that **nothing in it is labelled**.
*/

// The shape of the world these files describe, as numbers this package refuses
// to build without.
const (
	// Populations is the fewest the world may ship with.
	Populations = 1
	// Company is the fewest people a population may have. §3.15 wants the
	// repetition to read as "the same kind of person doing the same kind of
	// thing" rather than as the game running out, and a handful is the floor
	// under that.
	Company = 4
	// Common is the fewest things a population must hold in common. Fewer than
	// three is not a value system, it is a trait.
	Common = 3
)

/*
catalogue is the one parsed copy, built at process start.

A package-level value for the same reason internal/city's graph is: authored
content with no state in it, identical in every process, and a second copy could
only ever be a second opinion.
*/
var catalogue []Environment

/*
init parses and validates the shipped populations, and panics if it cannot.

technical.md §5.2: "production validates at startup so a malformed challenge
fails in CI, not at 2am." A panic here is that, and it is why [parse] takes an
fs.FS rather than reading the embedded files itself — every failure path below is
worth a test, and a test cannot catch an init that already ran.
*/
func init() {
	all, err := parse(content.FS)
	if err != nil {
		panic("environment: " + err.Error())
	}
	catalogue = all
}

/* ── never labelled ────────────────────────────────────────────────────── */

/*
labels is the vocabulary no player-facing string may contain.

§3.2, §3.10 and §3.15 in agreement: "stage names are internal production
vocabulary only… the game never labels a framework for the player, and never
displays a register." §3.15 adds the sharpest version of it — "environments are
never labelled. No stage names, no framework vocabulary, ever."

Two families, and the list is short on purpose. The developmental colours, which
are the actual stage names V1 is authored against; and the words that would give
the shape of the model away even without a colour — a population described as
being at a *stage* has told the player there is a ladder, which is the one thing
§3.2 says this game never does.

Matched on whole words, case-insensitively. A whole-word match is what keeps
"evergreen" and "amberley" out of it while still catching the sentence an author
would actually write by accident, which is a colour standing on its own as a
noun.
*/
var labels = []string{
	// The stage names (§3.10's registers).
	"beige", "purple", "red", "amber", "blue", "orange", "green", "yellow",
	"turquoise", "coral",
	// The framework itself.
	"register", "registers", "stage", "stages", "spiral", "vmeme", "vmemes",
	"archetype", "archetypes", "developmental", "tier", "tiers",
}

var labelled = regexp.MustCompile(`(?i)\b(` + strings.Join(labels, "|") + `)\b`)

/*
Labelled reports the first framework word a piece of player-facing text uses, if
any.

Exported because it is a rule about *content*, not about this package, and the
ticket that authors the next kind of environment-scoped writing — #30's daily
conflicts, #31's arrival copy — should be able to hold itself to the same line
without reimplementing it. An empty string means the text says nothing about the
model, which is the only acceptable answer.
*/
func Labelled(text string) string {
	return labelled.FindString(text)
}

/* ── reading the files ─────────────────────────────────────────────────── */

const environmentsPath = "environments"

/*
The YAML shape.

Separate from [Environment] on purpose, the same way every other loader in this
repository keeps them apart: what an author writes and what the rest of the
program reads are two decisions, and a population that carried its file's field
names would make renaming one a migration.
*/
type environmentFile struct {
	// There is deliberately no `name` key: an environment goes by one term,
	// the map's, so what a player reads is the district's own name and an
	// author who wrote a second one would be reintroducing the vocabulary
	// this shape exists to refuse.
	Key   string   `yaml:"key"`
	Note  string   `yaml:"note"`
	Holds []string `yaml:"holds"`
	// Affords is §3.15's factor table. There is no `intelligence` key: that
	// ceiling is a fact about how many rule sets the player has built rather
	// than about anywhere. Strict decoding turns the attempt into a failed
	// build.
	Affords struct {
		// Money is the economic ceiling in cents, so that an author writes an
		// amount rather than a score (see simulation.Ceilings).
		Money int64 `yaml:"money"`
		// Stability and Kindness are caps on the reading, 0–100.
		Stability float64 `yaml:"stability"`
		Kindness  float64 `yaml:"kindness"`
	} `yaml:"affords"`
	// Nouns is every capitalised thing this population's prose may name that is
	// not one of its people: its streets, its shops, its landmarks. Declared
	// rather than inferred — see internal/environment/named.go.
	Nouns []string `yaml:"nouns"`
	// Deprecated is a population on its way out. It excuses exactly one thing —
	// its people need no introduction (internal/environment/people.go) — and it
	// is named on the environment so the excuse is one place rather than a soft
	// field every character file could quietly use.
	Deprecated bool `yaml:"deprecated"`

	// There is deliberately no `people:` key and no `elsewhere:` key. This file
	// describes the place; the people who live in it are authored one to a file
	// under `content/characters/`.
}

/*
parse reads the catalogue out of a content filesystem and checks every rule the
rest of the package then gets to assume.

Strict twice over, like internal/situation's loader. Unknown YAML fields are
refused, so a misspelt `affords:` is a failed build rather than a population that
affords everything; and every cross-reference — a district, a location, a person
— is resolved against internal/city here rather than at the moment somebody walks
into it.
*/
func parse(files fs.FS) ([]Environment, error) {
	entries, err := fs.ReadDir(files, environmentsPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", environmentsPath, err)
	}

	var out []Environment
	// Which populations are being retired, read before the characters so that
	// [introduction] knows whose prose is excused, and what each may name that
	// is not one of its people.
	spent := map[string]bool{}
	nouns := map[string][]string{}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		name := path.Join(environmentsPath, entry.Name())

		var file environmentFile
		if err := content.Decode(files, name, &file); err != nil {
			return nil, err
		}
		e, err := population(file)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		// The key is the filename, so a population recorded against a player has
		// one name in the table, in the log and on disk — and since the key is
		// a district, one file per district is enforced by the filesystem.
		if stem := strings.TrimSuffix(entry.Name(), ".yaml"); e.Key != stem {
			return nil, fmt.Errorf("%s: is called %q", name, e.Key)
		}
		spent[e.Key] = file.Deprecated
		nouns[e.Key] = file.Nouns
		out = append(out, e)
	}

	if len(out) < Populations {
		return nil, fmt.Errorf("%s holds %d population(s); the world needs at least %d for rules to be local",
			environmentsPath, len(out), Populations)
	}
	// By key, so the world's populations are in the same order in every process
	// (§7). A directory walk is already sorted on every filesystem this runs on;
	// saying so is what makes the determinism a decision.
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })

	by, err := people(files, spent)
	if err != nil {
		return nil, err
	}
	// Last, so the rules that span files are checked against the whole world
	// rather than against whatever had been read when the offending file came
	// up in the directory.
	return attach(out, by, nouns)
}

// population reads and checks one file.
func population(file environmentFile) (Environment, error) {
	if file.Key == "" {
		return Environment{}, fmt.Errorf("a population has no key")
	}
	if file.Key == Nowhere {
		return Environment{}, fmt.Errorf("%q is the key for being among nobody and cannot be a population", Nowhere)
	}
	if strings.TrimSpace(file.Note) == "" {
		return Environment{}, fmt.Errorf("%q needs a note", file.Key)
	}

	// The key is the district the population lives in — one term for an
	// environment, the map's. Somewhere real, and a district rather than a
	// room: a population lives in a part of the city, and one keyed by the
	// laundromat would be six people who exist only while you are inside it.
	around, ok := city.Lookup(file.Key)
	if !ok {
		return Environment{}, fmt.Errorf("%q is not a place", file.Key)
	}
	if around.Layer != city.District {
		return Environment{}, fmt.Errorf("%q is not a district", file.Key)
	}

	if len(file.Holds) < Common {
		return Environment{}, fmt.Errorf("%q holds %d thing(s) in common; a value system is at least %d",
			file.Key, len(file.Holds), Common)
	}

	out := Environment{
		Key:  file.Key,
		Name: around.Name,
		Note: trim(file.Note),
	}
	for _, h := range file.Holds {
		held := trim(h)
		if held == "" {
			return Environment{}, fmt.Errorf("%q holds a blank", file.Key)
		}
		out.holds = append(out.holds, held)
	}

	ceilings, err := affords(file)
	if err != nil {
		return Environment{}, err
	}
	out.Ceilings = ceilings

	// Last, over everything a player can read. Late rather than early so the
	// message names a population that otherwise parsed, and total rather than
	// sampled: every string above is on a screen eventually.
	for what, text := range map[string]string{
		"its name": out.Name,
		"its note": out.Note,
	} {
		if word := Labelled(text); word != "" {
			return Environment{}, fmt.Errorf("%q says %q in %s, which labels the framework (§3.2)", file.Key, word, what)
		}
	}
	for i, held := range out.holds {
		if word := Labelled(held); word != "" {
			return Environment{}, fmt.Errorf("%q says %q in what it holds (%d), which labels the framework (§3.2)",
				file.Key, word, i+1)
		}
	}
	return out, nil
}

/*
affords reads §3.15's factor table for one population.

The bounds below are the pure core's rather than this package's opinion. Money
is optional: a population with no `money:` line affords no economic ceiling,
which the core reads as zero.
*/
func affords(file environmentFile) (simulation.Ceilings, error) {
	a := file.Affords
	if a.Money < 0 {
		return simulation.Ceilings{}, fmt.Errorf("%q affords an economic ceiling below zero", file.Key)
	}
	for what, most := range map[string]float64{"stability": a.Stability, "kindness": a.Kindness} {
		if most <= 0 || most > 100 {
			return simulation.Ceilings{}, fmt.Errorf("%q caps %s at %g, which is not a reading between 0 and 100",
				file.Key, what, most)
		}
	}
	return simulation.Ceilings{
		Money:     simulation.Money(a.Money),
		Stability: a.Stability,
		Kindness:  a.Kindness,
	}, nil
}

// trim flattens an authored block into one paragraph, so how an author wrapped
// their prose never reaches a screen.
func trim(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
