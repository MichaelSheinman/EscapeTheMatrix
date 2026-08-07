package challenge

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
`dialogue_branch`: the first mechanic type, extracted (#25).

§5.2 names it in its own list of the types V1 spans — "`dialogue_branch`,
`evidence_weigh`, `negotiation`, …" — and it is the first because it is the
shape §3.10's worked example turned out to have:

	You pass two people arguing; say a husband and wife. They ask for your help
	and lay out all the details. You weigh it and give advice by choosing which
	reading of the situation you commit to. You are effectively playing
	therapist. Hidden: your choice moves several factors. The characters then
	react according to how they *perceive* your interpretation.

A dialogue is people in a place, saying things, and a question the player answers
by committing to one of several readings — each of which is *received* rather
than understood, and leads somewhere different.

# Why this file is code and dialogue.yaml is not

§5.2 draws the line and this file is one side of it:

	**Mechanic types are code. Challenges are data.**
	- A mechanic type is implemented once: a Go handler plus a matching client
	  scene module, with a typed config schema.
	- A challenge is `mechanic type + config document + content files`.
	- New content in an existing type is data only. **A new mechanic is code.**

What is code here is the *shape*: that a scene has a place and beats and a
question, that an option carries how it was taken, that the whole thing projects
into a [Plan] the framework can save and score. Not one word of any challenge is
here. Two ship on it — `resolve_conflict/a`, the argument on the stoop, and
`resolve_conflict/b`, the Sunday at the laundromat — and the second one was
authored without touching this file, which is the only proof that the line is in
the right place.

§5.2's other half is the discipline that got here: **extract the type on the
second instance, not the first.** #24 wrote the argument on the stoop as Go, in
a file called `stoop.go` that is now only in the history, and this file is that
file with the words taken out of it and put in
`content/challenges/resolve_conflict/a/dialogue.yaml`. Nothing about that
challenge changed in the move. The next type arrives the same way — as one
challenge written in Go, and as a type once there are two of it.

# The schema

	opening: the-steps          # optional; the first scene otherwise
	scenes:
	  - key: the-steps
	    where: The steps of your building, a quarter to seven
	    beats:
	      - line: You come up the block with your keys already out.
	      - who: Marisol
	        line: You went there on your own.
	    ask: There is a way of coming into somebody else's argument.
	    options:
	      - key: sit
	        text: Sit down on the step below them.
	        taken: Marisol moves her bag off the step to make room for you.
	        then: laying-it-out       # empty ends the run
	        weight: 2                 # never shown
	        moves:
	          kindness: 30
	outcomes:
	  - key: talked-past
	    at-least: -2
	    reaction: |
	      You are on your own landing before you hear it start again below you.

Decoded strictly, so a misspelt field is a failed build rather than a line of
content silently ignored, and validated twice over: by [branching.Configure]
against the schema above, and by [Plan.check] against everything a challenge has
to be whatever produced it. Neither can pass what the other would refuse.

Nothing here is random (§7). The same passage always produces the same words, the
same hidden deltas and the same reaction — the mystery is in the weighting being
hidden, not in the scorer being nondeterministic (§5.1).
*/

// dialogueBranch is the name content asks for.
const dialogueBranch Type = "dialogue_branch"

// script is the one document this type reads, beside the challenge's own spec.
const script = "dialogue.yaml"

/*
branching is the mechanic type: registered once, and holding nothing.

It has no fields and cannot have any. One of these is shared by every challenge
that names it, so a challenge's words live on what [branching.Configure] hands
back rather than in here — see [Configured].
*/
type branching struct{}

func (branching) Type() Type { return dialogueBranch }

/*
Configure reads one challenge's dialogue.yaml and validates it against the
schema above.

It also refuses a document it does not read. A file beside a mechanic that never
opens one is content somebody wrote and nobody runs — invisible in a diff, and
exactly the kind of thing §5.2's validate-at-boot exists to catch.
*/
func (branching) Configure(dir fs.FS) (Configured, error) {
	found, err := fs.ReadDir(dir, ".")
	if err != nil {
		return nil, fmt.Errorf("read the challenge directory: %w", err)
	}
	for _, f := range found {
		switch f.Name() {
		case spec, script:
		default:
			return nil, fmt.Errorf("%s is not read by %s, which reads %s and nothing else",
				f.Name(), dialogueBranch, script)
		}
	}

	raw, err := fs.ReadFile(dir, script)
	if err != nil {
		return nil, fmt.Errorf("%s runs on %s and has no %s: %w", spec, dialogueBranch, script, err)
	}
	var file scriptFile
	if err := decode(raw, &file); err != nil {
		return nil, fmt.Errorf("%s: %w", script, err)
	}

	d, err := file.read()
	if err != nil {
		return nil, fmt.Errorf("%s %w", script, err)
	}
	return d, nil
}

/*
dialogue is one configured challenge of this type: the script, and the plan
projected out of it.

The two are made together and never separately, which is the point of them
being one value. A plan the framework would accept and this content has no words
for is a blank screen; words for an option the plan does not have is a button
that is refused when it is pressed. Both are impossible if there is one list,
and [dialogue.Say] reads the plan's.
*/
type dialogue struct {
	// acts is every scene, in authored order, with everything said in it.
	acts []act
	// plan is the same thing with the words taken out — the graph, the weights
	// and the outcome bands.
	plan Plan
}

// Plan is what the framework runs it by.
func (d *dialogue) Plan() Plan { return d.plan }

/*
Say is the words, for whichever moment of a run it is asked about.

The choices are built by walking the *plan's* options rather than the script's,
so the two cannot come apart: the plan is the list [Challenges.Choose] enforces,
and this attaches words to it by key.
*/
func (d *dialogue) Say(run Run) (Told, bool) {
	here, known := d.act(run.Scene.Key)
	if !known {
		// A run standing at a scene this content no longer has. The framework
		// already reads it back as a scene with no options; there is nothing
		// truthful to say about it, and inventing something would be worse.
		return Told{}, false
	}

	out := Told{
		Where:  here.where,
		Taken:  d.taken(run.Trail),
		Stage:  len(run.Trail) + 1,
		Stages: len(run.Trail) + run.Challenge.Plan.Ahead(run.Scene.Key),
	}

	if run.Done {
		// A finished run is standing on the scene it ended at, and what it has
		// to say is not that scene's question. It is how the last thing the
		// player committed to landed — already in Taken above — and then the
		// reaction authored for the outcome their trail scored into (§5.1),
		// which is the only account of how it went they ever get.
		out.Stage, out.Stages = len(run.Trail), len(run.Trail)
		out.Closing = run.Outcome.Reaction
		return out, true
	}

	out.Beats = here.beats
	out.Ask = here.ask
	for _, o := range run.Scene.Options {
		p, authored := here.position(o.Key)
		if !authored {
			continue
		}
		out.Choices = append(out.Choices, Choice{Key: p.key, Text: p.text})
	}
	return out, true
}

/*
taken is how the last thing the player committed to was received.

§3.10's "the characters then react according to how they *perceive* your
interpretation", and the reason it is a line on the option rather than on the
scene it leads to: two options can arrive at the same scene and land completely
differently. Empty at the opening, where nothing has been committed yet.
*/
func (d *dialogue) taken(trail []Step) string {
	if len(trail) == 0 {
		return ""
	}
	last := trail[len(trail)-1]
	a, known := d.act(last.Scene)
	if !known {
		return ""
	}
	p, authored := a.position(last.Option)
	if !authored {
		return ""
	}
	return p.taken
}

// act finds one scene of the script by key.
func (d *dialogue) act(key string) (act, bool) {
	for _, a := range d.acts {
		if a.key == key {
			return a, true
		}
	}
	return act{}, false
}

/* ── the script, as this type holds it ─────────────────────────────────── */

// act is one scene, with everything said in it.
type act struct {
	key string
	// where the run is standing, said as a place and a time rather than as a
	// stage direction.
	where string
	beats []Beat
	// ask is the question the positions answer, in the second person.
	ask       string
	positions []position
}

// position is one authored reading of the situation, and what it costs.
type position struct {
	key  string
	text string
	// taken is how the people in the room receive it — which is not the same as
	// what the player meant by it.
	taken string
	// then is the scene it leads to. Empty ends the run.
	then string
	// weight is what committing to this contributes to the outcome, and is
	// never shown.
	weight int
	deltas []simulation.Grant
}

func (a act) position(key string) (position, bool) {
	for _, p := range a.positions {
		if p.key == key {
			return p, true
		}
	}
	return position{}, false
}

/* ── the config document ───────────────────────────────────────────────── */

/*
scriptFile is dialogue.yaml, exactly — §5.2's "typed config schema".

Strict decoding ([decode]) makes this struct the authoring contract rather than
a suggestion: a field that is not here is a failed build. Note what is *not* in
it — the family, the variant, the mechanic type and where the challenge stands,
all of which belong to `challenge.yaml` or to the path, and none of which a
mechanic has any business reading.
*/
type scriptFile struct {
	// Opening is the scene a run begins at. Optional: the first authored scene
	// is the opening, which is what reading a script top to bottom means.
	Opening  string        `yaml:"opening"`
	Scenes   []sceneFile   `yaml:"scenes"`
	Outcomes []outcomeFile `yaml:"outcomes"`
}

type sceneFile struct {
	Key     string       `yaml:"key"`
	Where   string       `yaml:"where"`
	Beats   []beatFile   `yaml:"beats"`
	Ask     string       `yaml:"ask"`
	Options []optionFile `yaml:"options"`
}

// beatFile is one line. `who` is empty for the narration itself.
type beatFile struct {
	Who  string `yaml:"who"`
	Line string `yaml:"line"`
}

type optionFile struct {
	Key   string `yaml:"key"`
	Text  string `yaml:"text"`
	Taken string `yaml:"taken"`
	// Then is the scene this leads to. Absent ends the run, which is how a
	// script terminates — there is no ending node to forget to author.
	Then string `yaml:"then"`
	// Weight is what committing to this contributes to the outcome. Never
	// shown, and negative is allowed.
	Weight int `yaml:"weight"`
	// Moves is §5.1's authored hidden factor deltas, by the factor's own key.
	// A map rather than a list because the same factor twice in one option is
	// an author disagreeing with themselves, and YAML refuses a repeated key
	// before this code has to.
	Moves map[string]float64 `yaml:"moves"`
}

type outcomeFile struct {
	Key string `yaml:"key"`
	// AtLeast is the score this band starts at. A threshold rather than a range
	// because ranges have gaps and a score in one would be an outcome with
	// nothing to say.
	AtLeast  int    `yaml:"at-least"`
	Reaction string `yaml:"reaction"`
}

/*
read validates a decoded script and projects it into a dialogue.

Everything checked here is something [Plan.check] cannot see, because it is
about the *words*: a scene with no room, a question nobody asked, a choice with
nothing written on it, an option with no account of how it landed. The framework
would accept every one of those and run a challenge that showed a player a blank
screen. The graph itself — reachability, cycles, the outcome bounds — is
deliberately not re-checked here; it is the framework's, and doing it twice is
how two answers to one question start disagreeing.

The errors read as continuations of "dialogue.yaml …", which is how the loader
prefixes them.
*/
func (f scriptFile) read() (*dialogue, error) {
	if len(f.Scenes) == 0 {
		return nil, fmt.Errorf("has no scenes")
	}

	out := &dialogue{plan: Plan{Opening: strings.TrimSpace(f.Opening)}}
	if out.plan.Opening == "" {
		out.plan.Opening = strings.TrimSpace(f.Scenes[0].Key)
	}

	for _, s := range f.Scenes {
		a, err := s.read()
		if err != nil {
			return nil, err
		}
		out.acts = append(out.acts, a)

		scene := Scene{Key: a.key}
		for _, p := range a.positions {
			scene.Options = append(scene.Options, Option{
				Key: p.key, Then: p.then, Weight: p.weight, Deltas: p.deltas,
			})
		}
		out.plan.Scenes = append(out.plan.Scenes, scene)
	}

	for _, o := range f.Outcomes {
		if err := slug(strings.TrimSpace(o.Key)); err != nil {
			return nil, fmt.Errorf("has an outcome whose key %q %w", o.Key, err)
		}
		if strings.TrimSpace(o.Reaction) == "" {
			return nil, fmt.Errorf("outcome %q has no reaction, so landing in it says nothing", o.Key)
		}
		out.plan.Buckets = append(out.plan.Buckets, Bucket{
			Key:      strings.TrimSpace(o.Key),
			AtLeast:  o.AtLeast,
			Reaction: strings.TrimSpace(o.Reaction),
		})
	}
	return out, nil
}

// read validates one scene and its positions.
func (s sceneFile) read() (act, error) {
	key := strings.TrimSpace(s.Key)
	if err := slug(key); err != nil {
		return act{}, fmt.Errorf("has a scene whose key %q %w", s.Key, err)
	}
	if strings.TrimSpace(s.Where) == "" {
		return act{}, fmt.Errorf("scene %q says nowhere, so the player cannot be told where they are", key)
	}
	if len(s.Beats) == 0 {
		return act{}, fmt.Errorf("scene %q is a room with nobody in it", key)
	}
	if strings.TrimSpace(s.Ask) == "" {
		// §3.10's committed judgement is an answer to something. A scene that
		// offers positions and asks nothing is a list of sentences to pick from.
		return act{}, fmt.Errorf("scene %q asks nothing, so its options answer nothing", key)
	}
	if len(s.Options) == 0 {
		return act{}, fmt.Errorf("scene %q offers nothing", key)
	}

	out := act{key: key, where: strings.TrimSpace(s.Where), ask: strings.TrimSpace(s.Ask)}
	for _, b := range s.Beats {
		if strings.TrimSpace(b.Line) == "" {
			return act{}, fmt.Errorf("scene %q has a beat with no line in it", key)
		}
		out.beats = append(out.beats, Beat{Who: strings.TrimSpace(b.Who), Line: strings.TrimSpace(b.Line)})
	}
	for _, o := range s.Options {
		p, err := o.read(key)
		if err != nil {
			return act{}, err
		}
		out.positions = append(out.positions, p)
	}
	return out, nil
}

// read validates one authored position.
func (o optionFile) read(scene string) (position, error) {
	key := strings.TrimSpace(o.Key)
	if err := slug(key); err != nil {
		return position{}, fmt.Errorf("scene %q has an option whose key %q %w", scene, o.Key, err)
	}
	if strings.TrimSpace(o.Text) == "" {
		return position{}, fmt.Errorf("scene %q, option %q has nothing written on it", scene, key)
	}
	if strings.TrimSpace(o.Taken) == "" {
		// §3.10: "the characters then react according to how they perceive your
		// interpretation". A position with no account of how it landed is a
		// choice the player makes and never hears about.
		return position{}, fmt.Errorf("scene %q, option %q says nothing about how it was taken", scene, key)
	}

	out := position{
		key:    key,
		text:   strings.TrimSpace(o.Text),
		taken:  strings.TrimSpace(o.Taken),
		then:   strings.TrimSpace(o.Then),
		weight: o.Weight,
	}
	for _, name := range sorted(o.Moves) {
		which, err := moved(name)
		if err != nil {
			return position{}, fmt.Errorf("scene %q, option %q %w", scene, key, err)
		}
		out.deltas = append(out.deltas, simulation.Grant{Factor: which, Credit: o.Moves[name]})
	}
	return out, nil
}

// sorted is a `moves` block's factor names in the simulation's own order.
//
// Map iteration in Go is deliberately random, and a challenge whose authored
// grants arrived in a different order on every boot would be §7's determinism
// lost to a detail of the language rather than to a decision.
func sorted(moves map[string]float64) []string {
	out := make([]string, 0, len(moves))
	for name := range moves {
		out = append(out, name)
	}
	sort.Slice(out, func(i, j int) bool { return rank(out[i]) < rank(out[j]) })
	return out
}

// rank is where a factor key sits in simulation.Factors, or past the end.
func rank(name string) int {
	for i, f := range simulation.Factors {
		if f.Key() == name {
			return i
		}
	}
	return len(simulation.Factors)
}

/*
moved is the factor a `moves:` line names.

It refuses Health and Money by name rather than leaving it to [Plan.check],
because the message an author needs is which factors a challenge may move — and
the answer is the three that are scored from committed choices (§3.5). The
framework refuses them too, for any mechanic; this is the same rule said in the
words of the thing being authored.
*/
func moved(name string) (simulation.Factor, error) {
	var granted []string
	for _, f := range simulation.Factors {
		if f.Granted() {
			granted = append(granted, f.Key())
		}
	}
	for _, f := range simulation.Factors {
		if f.Key() != name {
			continue
		}
		if !f.Granted() {
			return 0, fmt.Errorf("moves %s, which is read from the world rather than granted (a challenge moves %s)",
				name, list(granted))
		}
		return f, nil
	}
	return 0, fmt.Errorf("moves %q, which is not a factor (a challenge moves %s)", name, list(granted))
}
