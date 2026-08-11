package environment

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

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
// to build without. §3.15's V1 scope is "the Orange and Green registers… two
// environments is enough to prove the mechanic", and the mechanic is transfer:
// one population cannot demonstrate that rules are local, so a build with one is
// a build with the core progression missing rather than a small one.
const (
	// Populations is the fewest the world may ship with.
	Populations = 2
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
	// Affords is §3.15's factor table. There is deliberately no `intelligence`
	// key: that ceiling is a fact about how many rule sets the player has built
	// rather than about anywhere, so an author who wrote one would be stating
	// something no reading would use. Strict decoding turns the attempt into a
	// failed build.
	Affords struct {
		// Money is the economic ceiling in cents, so that an author writes an
		// amount rather than a score (see simulation.Ceilings).
		Money int64 `yaml:"money"`
		// Stability and Kindness are caps on the reading, 0–100.
		Stability float64 `yaml:"stability"`
		Kindness  float64 `yaml:"kindness"`
	} `yaml:"affords"`
	People []struct {
		Key   string `yaml:"key"`
		Name  string `yaml:"name"`
		Note  string `yaml:"note"`
		Found string `yaml:"found"`
		Does  string `yaml:"does"`
	} `yaml:"people"`
	// Elsewhere is §3.15's recommendation, as gossip. There is deliberately no
	// key for when it is said: that is decay's answer (simulation.Elsewhere),
	// and an author who could set it per population would be authoring the
	// pressure rather than the sentence.
	Elsewhere struct {
		Who   string `yaml:"who"`
		Where string `yaml:"where"`
		Says  string `yaml:"says"`
	} `yaml:"elsewhere"`
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
	// Person keys are unique across the whole world rather than within a
	// population. #30 selects a conflict from relationship state, and a
	// relationship is with a person: two Bezes in two populations would be one
	// row that means different people depending on where its owner lives.
	who := map[string]string{}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		name := path.Join(environmentsPath, entry.Name())

		var file environmentFile
		if err := decode(files, name, &file); err != nil {
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

		for _, p := range e.people {
			if other, taken := who[p.Key]; taken {
				return nil, fmt.Errorf("%s: %q is already somebody in %q", name, p.Key, other)
			}
			who[p.Key] = e.Key
		}
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

	// The one rule no file can answer about itself: where it points has to be
	// somewhere. Last, so it is checked against the whole world rather than
	// against whatever had been read by the time the offending file came up in
	// the directory.
	where := map[string]bool{}
	for _, e := range out {
		where[e.Key] = true
	}
	for _, e := range out {
		if e.Elsewhere.Where == e.Key {
			return nil, fmt.Errorf("%q points at itself as somewhere else", e.Key)
		}
		if !where[e.Elsewhere.Where] {
			return nil, fmt.Errorf("%q points at %q, which is not a population", e.Key, e.Elsewhere.Where)
		}
	}
	return out, nil
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

	if len(file.People) < Company {
		return Environment{}, fmt.Errorf("%q has %d person/people; a population is at least %d",
			file.Key, len(file.People), Company)
	}
	inside := map[string]bool{}
	for _, p := range file.People {
		if p.Key == "" {
			return Environment{}, fmt.Errorf("%q has somebody with no key", file.Key)
		}
		if inside[p.Key] {
			return Environment{}, fmt.Errorf("%q has two people called %q", file.Key, p.Key)
		}
		inside[p.Key] = true

		if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.Note) == "" {
			return Environment{}, fmt.Errorf("%q's %q needs a name and a note", file.Key, p.Key)
		}
		// The load-bearing one. A person with no behaviour is a person whose
		// values are only knowable by being told them, which is the one way
		// §3.15 says a population may never be legible.
		if trim(p.Does) == "" {
			return Environment{}, fmt.Errorf("%q's %q does nothing, so nothing about %q is legible from behaviour",
				file.Key, p.Key, file.Key)
		}

		// Reusing existing locations is §3.15's instruction, and somewhere in
		// the population's own district is what makes it a population rather
		// than a mailing list.
		found, ok := city.Lookup(p.Found)
		if !ok {
			return Environment{}, fmt.Errorf("%q's %q is found at %q, which is not a place", file.Key, p.Key, p.Found)
		}
		if found.Key != out.Key && found.In != out.Key {
			return Environment{}, fmt.Errorf("%q's %q is found at %q, which is not in %q",
				file.Key, p.Key, p.Found, out.Key)
		}

		out.people = append(out.people, Person{
			Key:   p.Key,
			Name:  strings.TrimSpace(p.Name),
			Note:  trim(p.Note),
			Found: p.Found,
			Does:  trim(p.Does),
		})
	}

	// §3.15's recommendation. Everything about it that can be checked without
	// the other files is checked here; that it points at a population that
	// exists, and not at this one, is [parse]'s — it is the only rule in this
	// package that one file cannot answer on its own.
	said := trim(file.Elsewhere.Says)
	switch {
	case file.Elsewhere.Where == "":
		return Environment{}, fmt.Errorf("%q says nothing about anywhere else; §3.15's loop has no last population", file.Key)
	case said == "":
		return Environment{}, fmt.Errorf("%q points at %q without a word about it", file.Key, file.Elsewhere.Where)
	case !inside[file.Elsewhere.Who]:
		// Somebody who lives here, or it is the game talking. The whole value of
		// a recommendation being gossip is that a person with their own reasons
		// said it, and a line attributed to nobody is a prompt.
		return Environment{}, fmt.Errorf("%q's word about %q is nobody's: %q is not one of these people",
			file.Key, file.Elsewhere.Where, file.Elsewhere.Who)
	}
	out.Elsewhere = Elsewhere{Who: file.Elsewhere.Who, Where: file.Elsewhere.Where, Says: said}

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
	for _, p := range out.people {
		for what, text := range map[string]string{"name": p.Name, "note": p.Note, "behaviour": p.Does} {
			if word := Labelled(text); word != "" {
				return Environment{}, fmt.Errorf("%q's %q says %q in their %s, which labels the framework (§3.2)",
					file.Key, p.Key, word, what)
			}
		}
	}
	if word := Labelled(out.Elsewhere.Says); word != "" {
		return Environment{}, fmt.Errorf("%q says %q about %q, which labels the framework (§3.2)",
			file.Key, word, out.Elsewhere.Where)
	}
	return out, nil
}

/*
affords reads §3.15's factor table for one population.

Every bound below is the pure core's rather than this package's opinion. A
ceiling of zero is *no ceiling* there, so an unstated affordance is refused here
rather than read as an unlimited one — a population that afforded infinite money
by leaving a line out is exactly the kind of quiet mistake §5.2 says validation
exists to catch.
*/
func affords(file environmentFile) (simulation.Ceilings, error) {
	a := file.Affords
	if a.Money <= 0 {
		return simulation.Ceilings{}, fmt.Errorf("%q affords no economic ceiling; every population has one (§3.15)", file.Key)
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

// trim flattens an authored block into one paragraph.
//
// YAML folding already turns a `>-` block into a single line; this is what makes
// a plain multi-line string behave the same way, so how an author wrapped their
// prose never reaches a screen.
func trim(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// decode reads one YAML file strictly: an unknown field is a failed build rather
// than an authored line nobody notices has no effect.
func decode(files fs.FS, name string, into any) error {
	raw, err := fs.ReadFile(files, name)
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	return nil
}
