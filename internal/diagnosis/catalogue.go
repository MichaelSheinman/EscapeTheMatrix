package diagnosis

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/MichaelSheinman/EscapeTheMatrix/content"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/environment"
)

/*
The catalogue: the causes a player may name, and the person they name one to.

Two kinds of document, and the split between them is the ticket's central rule
made structural.

  - **A cause** (`content/diagnoses/causes/*.yaml`) is an account of why
    something went badly, what evidence would speak to it, and what understanding
    it hands over. It is authored, because what a tool *is* has to be.
  - **A scene** (`content/diagnoses/<population>/scene.yaml`) is the person who
    sits down with the player and the four things they say. Authored per
    population for the same reason a daily conflict is (§3.15): the people you
    live among are the world, and a diagnosis delivered by a nameless coach
    would be the game talking.

What is **not** authored anywhere is an answer. §3.6's first rule is "point at
evidence, not at answer strings — no four authored options; the player selects
from their own data", so a cause file names the *observation keys* that could
speak to it and never a sentence the player picks from. The sentences a player
reads in the scene are readings of their own history, computed at the moment they
open it, by the modules that own the rows. This file cannot write one and has no
field to put one in.
*/

/*
Cause is one account of why something went badly, and what understanding it
hands over.

`Speaks` is the join this whole mechanic turns on. A cause may be named only by
somebody who has pointed at evidence that speaks to it, so a cause is reachable
exactly when the player's own history says something about it — and the same
observation may speak to more than one cause, which is §3.6's fourth rule
("allow multiple valid causes") as a data shape rather than as a promise. A
resting heart rate that has climbed for a fortnight is evidence for short nights
and evidence for training that never recovered, and which of them the player
commits to is the decision the mechanic exists to put.

`Unlocks` is the tool. §3.6: "naming the cause is what grants the tool. The
insight *is* the gate." A cause may unlock a routine, playbook vocabulary, or
both — and the acceptance criterion behind that variety is "unlocking different
tools of differing usefulness". Usefulness is not a field here and never will be:
what makes a tool useful is whether it addresses what was actually driving the
body, and that is the simulation's answer, arrived at by living with the tool.
*/
type Cause struct {
	Key string
	// Says is the cause in the player's own words — what they would say they
	// think happened. Second person, no verdict, and never a question.
	Says string
	// Because is the reasoning in one sentence: what would follow if this were
	// what happened. It is what the player is committing to, so it has to be
	// arguable rather than merely true.
	Because string
	// Speaks is the observation keys that could support this cause. Every one
	// of them has to be a key some module actually produces, which is checked
	// at boot — see [Check].
	Speaks []string
	// Routine is the routine key this cause unlocks the configuration of, or
	// empty for a cause that hands over nothing but words.
	Routine string
	// Terms is the playbook vocabulary this cause unlocks, by term ID.
	Terms []string
}

// Grants reports whether naming this cause hands anything over at all.
//
// A cause that unlocked nothing would be a conclusion with no consequence, which
// is the one thing §3.6 says a diagnosis never is. Checked at boot.
func (c Cause) Grants() bool { return c.Routine != "" || len(c.Terms) > 0 }

/*
Scene is the conversation one population stages a diagnosis in.

Four lines and a person, and the order of them is the mechanic. `Opens` is said
at the moment of consequence, with the failure already named by the world.
`Asks` puts the question. `Weighs` is what they say once the player has started
pointing at things — said before any cause is named, so it cannot be a reaction
to which one. `Closes` is after, and it is deliberately the same line whatever
was named.

That last one is §3.6's second rule at its most literal. The character does not
know whether the player is right, so they do not say. What they say is that the
player will find out, which is also what actually happens.
*/
type Scene struct {
	// Population is the environment key this scene belongs to, from the
	// directory name.
	Population string
	// Who is a person key from that population. Somebody the player already
	// knows off the A screen, rather than a specialist invented for the
	// occasion — checked at boot against internal/environment.
	Who    string
	Opens  string
	Asks   string
	Weighs string
	Closes string
}

var (
	causes []Cause
	scenes []Scene
)

func init() {
	var err error
	causes, scenes, err = parse(content.FS)
	if err != nil {
		panic(fmt.Sprintf("diagnosis: %v", err))
	}
}

// Causes is every authored cause, in key order.
func Causes() []Cause {
	out := make([]Cause, len(causes))
	copy(out, causes)
	return out
}

// Known finds a cause by key.
func Known(key string) (Cause, bool) {
	for _, c := range causes {
		if c.Key == key {
			return c, true
		}
	}
	return Cause{}, false
}

// Staged finds the scene a population stages a diagnosis in.
func Staged(population string) (Scene, bool) {
	for _, s := range scenes {
		if s.Population == population {
			return s, true
		}
	}
	return Scene{}, false
}

/*
Check refuses a catalogue whose causes point at evidence nothing produces.

Called from the composition root once, with every observation key the wired
ledgers can emit, because that list is a fact about which modules are bound
rather than about content — this package cannot ask internal/health what it
knows how to say.

It is the check worth having. A cause whose evidence keys are all misspelt is a
tool no player can ever be handed, and it fails silently in exactly the way §5.2
says content must not: nobody notices, because the absence looks like a player
who has not struggled enough yet.
*/
func Check(produced []string) error {
	known := make(map[string]bool, len(produced))
	for _, k := range produced {
		known[k] = true
	}
	for _, c := range causes {
		for _, k := range c.Speaks {
			if !known[k] {
				return fmt.Errorf("diagnosis: cause %q speaks from %q, which nothing produces", c.Key, k)
			}
		}
	}
	return nil
}

/* ── loading ───────────────────────────────────────────────────────────── */

const dir = "diagnoses"

// causeFile is a cause as it is written.
type causeFile struct {
	Says    string   `yaml:"says"`
	Because string   `yaml:"because"`
	Speaks  []string `yaml:"speaks-from"`
	Unlocks struct {
		Routine string   `yaml:"routine"`
		Terms   []string `yaml:"terms"`
	} `yaml:"unlocks"`
}

// sceneFile is a scene as it is written.
type sceneFile struct {
	Who    string `yaml:"who"`
	Opens  string `yaml:"opens"`
	Asks   string `yaml:"asks"`
	Weighs string `yaml:"weighs"`
	Closes string `yaml:"closes"`
}

/*
parse reads the whole catalogue and refuses everything it can.

Takes an fs.FS rather than reading the embedded content directly so that the
failure paths are testable — the same shape internal/environment and
internal/conflict use, and for the same reason: a loader whose refusals can only
be reached by breaking the real content files is a loader nobody tests.
*/
func parse(files fs.FS) ([]Cause, []Scene, error) {
	entries, err := fs.ReadDir(files, dir)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", dir, err)
	}

	var (
		out    []Cause
		staged []Scene
	)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if e.Name() == "causes" {
			out, err = readCauses(files)
			if err != nil {
				return nil, nil, err
			}
			continue
		}
		s, err := readScene(files, e.Name())
		if err != nil {
			return nil, nil, err
		}
		staged = append(staged, s)
	}

	if len(out) == 0 {
		return nil, nil, fmt.Errorf("no causes are authored, so nothing could ever be named")
	}
	if err := everyPopulationStages(staged); err != nil {
		return nil, nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	sort.Slice(staged, func(i, j int) bool { return staged[i].Population < staged[j].Population })
	return out, staged, nil
}

func readCauses(files fs.FS) ([]Cause, error) {
	entries, err := fs.ReadDir(files, path.Join(dir, "causes"))
	if err != nil {
		return nil, fmt.Errorf("read causes: %w", err)
	}

	seen := map[string]bool{}
	var out []Cause
	for _, e := range entries {
		if e.IsDir() || path.Ext(e.Name()) != ".yaml" {
			continue
		}
		key := strings.TrimSuffix(e.Name(), ".yaml")

		var file causeFile
		if err := decode(files, path.Join(dir, "causes", e.Name()), &file); err != nil {
			return nil, err
		}

		c := Cause{
			Key:     key,
			Says:    trim(file.Says),
			Because: trim(file.Because),
			Speaks:  file.Speaks,
			Routine: file.Unlocks.Routine,
			Terms:   file.Unlocks.Terms,
		}
		if err := sound(c); err != nil {
			return nil, err
		}
		if seen[key] {
			return nil, fmt.Errorf("cause %q is authored twice", key)
		}
		seen[key] = true
		out = append(out, c)
	}
	return out, nil
}

// sound refuses a cause that could not do its job.
func sound(c Cause) error {
	switch {
	case c.Says == "":
		return fmt.Errorf("cause %q says nothing, so there is nothing to name", c.Key)
	case c.Because == "":
		return fmt.Errorf("cause %q gives no reasoning, so committing to it would be a guess", c.Key)
	case len(c.Speaks) == 0:
		return fmt.Errorf("cause %q speaks from no evidence, so nobody could ever reach it", c.Key)
	case !c.Grants():
		return fmt.Errorf("cause %q unlocks nothing, and a conclusion with no consequence is not one", c.Key)
	}
	// The same line every other piece of player-facing writing in this game is
	// held to (§3.15). A diagnosis is the most tempting place in the whole
	// design to name a stage out loud, because it is the one screen that is
	// explicitly about the player understanding something.
	for _, text := range []string{c.Says, c.Because} {
		if word := environment.Labelled(text); word != "" {
			return fmt.Errorf("cause %q says %q, and nothing player-facing may", c.Key, word)
		}
	}
	return nil
}

func readScene(files fs.FS, population string) (Scene, error) {
	var file sceneFile
	if err := decode(files, path.Join(dir, population, "scene.yaml"), &file); err != nil {
		return Scene{}, err
	}

	s := Scene{
		Population: population,
		Who:        file.Who,
		Opens:      trim(file.Opens),
		Asks:       trim(file.Asks),
		Weighs:     trim(file.Weighs),
		Closes:     trim(file.Closes),
	}

	env, ok := environment.Lookup(population)
	if !ok {
		return Scene{}, fmt.Errorf("scene %q is not a population anybody lives among", population)
	}
	if _, ok := env.Person(s.Who); !ok {
		return Scene{}, fmt.Errorf("scene %q is staged with %q, who does not live there", population, s.Who)
	}
	for what, text := range map[string]string{
		"opens": s.Opens, "asks": s.Asks, "weighs": s.Weighs, "closes": s.Closes,
	} {
		if text == "" {
			return Scene{}, fmt.Errorf("scene %q has no %s line", population, what)
		}
		if word := environment.Labelled(text); word != "" {
			return Scene{}, fmt.Errorf("scene %q says %q, and nothing player-facing may", population, word)
		}
	}
	return s, nil
}

/*
everyPopulationStages refuses a population with nowhere to have the conversation.

The same shape as internal/conflict's pool check, and for the same reason: a
population without a scene is a place where the game's primary
consciousness-raising mechanism (§3.6) silently does not exist, and the way that
would surface is a player who moved there and could never unlock another routine
again.
*/
func everyPopulationStages(staged []Scene) error {
	have := make(map[string]bool, len(staged))
	for _, s := range staged {
		have[s.Population] = true
	}
	for _, env := range environment.All() {
		if !have[env.Key] {
			return fmt.Errorf("nobody in %q stages a diagnosis, so nothing could be named there", env.Key)
		}
	}
	return nil
}

func trim(s string) string { return strings.TrimSpace(s) }

// decode reads one document strictly: a misspelt field is a failed build rather
// than a silently ignored line (§5.2).
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
