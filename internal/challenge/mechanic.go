package challenge

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
Mechanic types are code and challenges are data (§5.2). A mechanic type is
implemented once — a Go handler plus a matching client scene module, with a typed
config schema — and a challenge is a mechanic type, a config document and content
files. New content in an existing type is data only.

This file holds no mechanic. A type is extracted on its second instance rather
than its first, so what is here is the registry a type registers into and the
vocabulary every type speaks to be run by the same machinery: scenes, options,
authored factor deltas, and outcome buckets.

Those four belong to the framework rather than to any mechanic, because §5.1
decides them for every challenge: player input is multiple choice rather than
free text, each option carries authored hidden factor deltas, and reactions are
authored per outcome bucket. A type that wanted different answers is one this
framework cannot save, resume or score.
*/

// Type is a mechanic type's name, as content names it. Text rather than an enum
// because the set grows in Go rather than in SQL (§5.3). [Register] is what
// closes it: an unregistered type is a failed build rather than a challenge that
// never runs.
type Type string

// Mechanic is one mechanic type, registered once in code. [Mechanic.Type] is the
// name content asks for. [Mechanic.Configure] reads the type's own documents out
// of the challenge's directory and either produces a plan or refuses the build.
// The framework never parses a mechanic's config, and a mechanic never sees the
// database (§5.2).
type Mechanic interface {
	// Type is the name authored in challenge.yaml.
	Type() Type
	// Configure reads this type's own content documents out of one challenge's
	// directory and validates them against its schema. The fs.FS is rooted at
	// the variant directory, so a mechanic names its files (`dialogue.yaml`)
	// rather than paths. An error here is a failed build.
	Configure(dir fs.FS) (Configured, error)
}

/*
Configured is one challenge as its mechanic made it.

A [Mechanic] is registered once and shared by every challenge that names it, so
it cannot hold one challenge's words in a field. This is where they go instead:
Configure is handed a directory and hands back the whole of what it made of it,
the framework keeps that beside the definition, and it comes back to the same
mechanic — through [Narrator] — when a run needs narrating.

The one method is the only part the framework reads. Everything else a type put
here is behind an interface this package cannot see past.

It also keeps the hot reload whole. A mechanic holding its own map of challenge
code to script could show half of one reload and half of the next at the same
moment; a definition carries its plan and its words together, and [swap] replaces
both or neither.
*/
type Configured interface {
	// Plan is the graph the framework runs this challenge by. It is validated
	// by [Plan.check] before anybody is put in one, whatever produced it.
	Plan() Plan
}

// mechanics is every registered type, by name. Written only by [Register] at
// init and read only after, so no lock: a mechanic cannot arrive at runtime
// (§5.2).
var mechanics = map[Type]Mechanic{}

// Register adds a mechanic type. Call it from an init, once per type. It panics
// on a name already taken or an empty one, because two types answering to one
// name leaves it undecided which code runs a player's challenge.
func Register(m Mechanic) {
	name := m.Type()
	if strings.TrimSpace(string(name)) == "" {
		panic("challenge: a mechanic type needs a name")
	}
	if _, taken := mechanics[name]; taken {
		panic("challenge: two mechanic types called " + string(name))
	}
	mechanics[name] = m
}

// Types is every registered mechanic type, in name order.
func Types() []Type {
	out := make([]Type, 0, len(mechanics))
	for name := range mechanics {
		out = append(out, name)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

/* ── what every mechanic produces ──────────────────────────────────────── */

// Plan is one challenge's run as the framework sees it: a directed acyclic graph
// of scenes, the options at each, and the outcome buckets a finished trail scores
// into. A value rather than an interface. The words, the characters, the art
// direction and whatever config a mechanic's own schema names stay with the
// mechanic.
type Plan struct {
	// Opening is the scene a run begins at.
	Opening string
	// Scenes is every scene, in authored order.
	Scenes []Scene
	// Buckets is §5.1's outcome buckets, lowest floor first after validation.
	Buckets []Bucket
}

// Scene is one boundary the run may be saved at, which is why it is the unit the
// framework knows about. Run state is checkpointed at scene boundaries (§5.1), so
// a scene is somewhere a player can be put back down and a mechanic that wants a
// save point puts one there.
type Scene struct {
	Key string
	// Options are the authored positions the player chooses between. None is
	// correct, and each leads somewhere different (§3.10).
	Options []Option
}

// Option is one authored position.
type Option struct {
	Key string
	// Then is the scene this leads to. Empty ends the run, so there is no ending
	// node to forget to author.
	Then string
	// Weight is what committing to this contributes to the outcome score, and
	// negative is allowed. Never shown (§3.10): what the player sees is the
	// reaction text of the bucket the total fell in.
	Weight int
	// Deltas are §5.1's authored hidden factor deltas, applied on completion
	// rather than on the choice — see [Challenges.Choose]. Authored does not mean
	// visible: the weighting is hidden and the scorer is deterministic.
	Deltas []simulation.Grant

	// Takings is money this option puts in the player's pocket, in cents. Its
	// own field rather than a hidden factor, and not a factor grant:
	// [Plan.check] makes a money entry in Deltas a failed build. Negative is
	// refused.
	Takings simulation.Money
}

// Bucket is one outcome band, with the reaction authored for it (§5.1). A
// threshold rather than a range, because ranges have gaps and a score that fell
// in one would be an outcome with nothing to say.
type Bucket struct {
	Key string
	// AtLeast is the score this bucket starts at. The loader checks that the
	// lowest bucket is reachable from below by every possible trail, so nobody
	// can finish a challenge with no outcome.
	AtLeast int
	// Reaction is what the characters say, in the content's own words.
	Reaction string
}

/* ── reading a plan ────────────────────────────────────────────────────── */

// Scene finds a scene by key.
func (p Plan) Scene(key string) (Scene, bool) {
	for _, s := range p.Scenes {
		if s.Key == key {
			return s, true
		}
	}
	return Scene{}, false
}

// Option finds one of a scene's options by key.
func (s Scene) Option(key string) (Option, bool) {
	for _, o := range s.Options {
		if o.Key == key {
			return o, true
		}
	}
	return Option{}, false
}

// Ends reports whether committing to this option finishes the run.
func (o Option) Ends() bool { return o.Then == "" }

/*
Outcome is the bucket a score falls in.

The highest bucket the score reaches, which — because [Plan.check] refuses a
plan whose lowest bucket is above the worst possible trail — is always one of
them. The false is therefore unreachable for a validated plan, and it is
returned anyway rather than panicking, because "unreachable" is a claim about
the loader and this is the run path.
*/
func (p Plan) Outcome(score int) (Bucket, bool) {
	var (
		out   Bucket
		found bool
	)
	for _, b := range p.Buckets {
		if score >= b.AtLeast {
			out, found = b, true
		}
	}
	return out, found
}

/*
check is every rule a plan has to satisfy to be runnable, whatever mechanic
produced it.

The list is long on purpose. Every one of these is a way for an authored
challenge to be broken in a manner no single file shows and no test of the
mechanic would catch, and every one of them is a failed build rather than a
player standing in a room with no doors (§5.2: "production validates at startup
so a malformed challenge fails in CI, not at 2am").
*/
// The receiver is a pointer because [Plan.buckets] puts the outcome bands in
// order as it checks them, and a plan that came back from validation unsorted
// would leave [Plan.Outcome] depending on the order somebody typed them in.
func (p *Plan) check() error {
	if len(p.Scenes) == 0 {
		return fmt.Errorf("has no scenes")
	}

	seen := map[string]bool{}
	choices := 0
	for _, s := range p.Scenes {
		if strings.TrimSpace(s.Key) == "" {
			return fmt.Errorf("has a scene with no key")
		}
		if seen[s.Key] {
			return fmt.Errorf("has two scenes called %q", s.Key)
		}
		seen[s.Key] = true

		if len(s.Options) == 0 {
			// Not an ending: an ending is an option with nothing after it, so a
			// scene with no options at all is somewhere a run stops without
			// finishing — which is the incoherent resume §5.1 rules out.
			return fmt.Errorf("scene %q offers nothing", s.Key)
		}
		if len(s.Options) > 1 {
			choices++
		}

		taken := map[string]bool{}
		for _, o := range s.Options {
			if strings.TrimSpace(o.Key) == "" {
				return fmt.Errorf("scene %q has an option with no key", s.Key)
			}
			if taken[o.Key] {
				return fmt.Errorf("scene %q has two options called %q", s.Key, o.Key)
			}
			taken[o.Key] = true

			if o.Takings < 0 {
				// See [Option.Takings]: an option that took money would have to
				// know whether the player had any, and a challenge that could
				// read a balance is a challenge that can be authored around one.
				return fmt.Errorf("scene %q, option %q takes %d from the player, and a challenge only ever hands things over",
					s.Key, o.Key, -o.Takings)
			}

			factors := map[simulation.Factor]bool{}
			for _, d := range o.Deltas {
				if !d.Valid() {
					// §3.5: Health and Money are read from the world, never
					// granted. An authored delta on one is an author expecting
					// something that will not happen.
					return fmt.Errorf("scene %q, option %q moves %s, which is read rather than granted",
						s.Key, o.Key, d.Factor)
				}
				if factors[d.Factor] {
					return fmt.Errorf("scene %q, option %q moves %s twice", s.Key, o.Key, d.Factor)
				}
				factors[d.Factor] = true
			}
		}
	}
	if choices == 0 {
		// §3.10's committed judgement — "the player selects the interpretation
		// they will act on from a set of authored positions" — never happens in
		// a plan whose every scene has one way out. That is a corridor, and a
		// corridor is not a challenge.
		return fmt.Errorf("has no scene with more than one option, so nothing is ever chosen")
	}

	if _, ok := p.Scene(p.Opening); !ok {
		return fmt.Errorf("opens at %q, which is not a scene", p.Opening)
	}
	for _, s := range p.Scenes {
		for _, o := range s.Options {
			if o.Ends() {
				continue
			}
			if _, ok := p.Scene(o.Then); !ok {
				return fmt.Errorf("scene %q, option %q leads to %q, which is not a scene", s.Key, o.Key, o.Then)
			}
		}
	}

	reach := p.reachable()
	for _, s := range p.Scenes {
		if !reach[s.Key] {
			// Authored and unplayable. Almost always a scene whose only way in
			// was renamed, which is invisible in either file on its own.
			return fmt.Errorf("scene %q cannot be reached from %q", s.Key, p.Opening)
		}
	}
	if err := p.acyclic(); err != nil {
		return err
	}

	return p.buckets()
}

// reachable is every scene a run can actually arrive at.
func (p Plan) reachable() map[string]bool {
	out := map[string]bool{}
	var walk func(key string)
	walk = func(key string) {
		if out[key] {
			return
		}
		out[key] = true
		scene, ok := p.Scene(key)
		if !ok {
			return
		}
		for _, o := range scene.Options {
			if !o.Ends() {
				walk(o.Then)
			}
		}
	}
	walk(p.Opening)
	return out
}

/*
acyclic refuses a plan a player could go round in.

A loop is not a design decision this framework can honour. §5.1 caps a challenge
at "once ever, per player" and pays its deltas on completion, so a cycle is
either a run that never finishes or — worse — a trail that can be extended
without bound, which is a factor grant with a crank on it.
*/
func (p Plan) acyclic() error {
	const (
		open = 1
		shut = 2
	)
	state := map[string]int{}

	var walk func(key string) error
	walk = func(key string) error {
		switch state[key] {
		case shut:
			return nil
		case open:
			return fmt.Errorf("scene %q leads, eventually, back to itself", key)
		}
		state[key] = open
		scene, ok := p.Scene(key)
		if !ok {
			return nil
		}
		for _, o := range scene.Options {
			if o.Ends() {
				continue
			}
			if err := walk(o.Then); err != nil {
				return err
			}
		}
		state[key] = shut
		return nil
	}
	return walk(p.Opening)
}

/*
buckets checks the outcome bands, and it is the one check worth reading twice.

Sorting them is bookkeeping. What matters is the two bounds: **every trail lands
in a bucket** and **every bucket can be landed in**. The first is a challenge
somebody finishes and is told nothing; the second is reaction text nobody will
ever read, which is worse than it sounds — the author wrote it believing the
best outcome was attainable, and it was not.

The bounds come from the plan's own graph rather than from a guess, so they hold
however the branching is shaped.
*/
func (p *Plan) buckets() error {
	if len(p.Buckets) == 0 {
		return fmt.Errorf("has no outcome buckets, so finishing it says nothing")
	}

	sort.SliceStable(p.Buckets, func(i, j int) bool { return p.Buckets[i].AtLeast < p.Buckets[j].AtLeast })

	seen := map[string]bool{}
	floors := map[int]string{}
	for _, b := range p.Buckets {
		if strings.TrimSpace(b.Key) == "" {
			return fmt.Errorf("has an outcome with no key")
		}
		if seen[b.Key] {
			return fmt.Errorf("has two outcomes called %q", b.Key)
		}
		seen[b.Key] = true
		if strings.TrimSpace(b.Reaction) == "" {
			return fmt.Errorf("outcome %q has no reaction", b.Key)
		}
		if taken, clash := floors[b.AtLeast]; clash {
			return fmt.Errorf("outcomes %q and %q both start at %d", taken, b.Key, b.AtLeast)
		}
		floors[b.AtLeast] = b.Key
	}

	worst, best := p.bounds()
	if lowest := p.Buckets[0]; lowest.AtLeast > worst {
		return fmt.Errorf("the lowest outcome %q starts at %d, and a run can score %d",
			lowest.Key, lowest.AtLeast, worst)
	}
	if highest := p.Buckets[len(p.Buckets)-1]; highest.AtLeast > best {
		return fmt.Errorf("the outcome %q starts at %d, and no run can score more than %d",
			highest.Key, highest.AtLeast, best)
	}
	return nil
}

// bounds is the worst and best score any complete trail can total.
//
// A shortest and a longest path over a graph already known to be acyclic, so
// one memoised walk answers both.
func (p Plan) bounds() (worst, best int) {
	type reach struct{ low, high int }
	known := map[string]reach{}

	var walk func(key string) reach
	walk = func(key string) reach {
		if got, ok := known[key]; ok {
			return got
		}
		scene, ok := p.Scene(key)
		if !ok {
			return reach{}
		}
		out := reach{}
		for i, o := range scene.Options {
			from := reach{low: o.Weight, high: o.Weight}
			if !o.Ends() {
				onward := walk(o.Then)
				from.low, from.high = o.Weight+onward.low, o.Weight+onward.high
			}
			if i == 0 {
				out = from
				continue
			}
			if from.low < out.low {
				out.low = from.low
			}
			if from.high > out.high {
				out.high = from.high
			}
		}
		known[key] = out
		return out
	}

	got := walk(p.Opening)
	return got.low, got.high
}
