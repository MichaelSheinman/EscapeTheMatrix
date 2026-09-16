package challenge

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/MichaelSheinman/EscapeTheMatrix/content"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
salvage: what you carry out, and what you hand back. A room, more things in it
than you can carry, and a stranger outside who owns all of them.

	You have a bag that holds three. There is a dog under the kitchen table, a
	phone on the arm of the chair, an urn on the shelf, and a roll of notes in a
	drawer somebody left open. You are not going to get a second trip. Then you
	are on the pavement, and the woman whose flat it was is standing there, and
	everything in your bag is hers.

Two phases rather than one. The taking is a triage problem: everything is worth
something, you can have three, and the fourth thing you reach for is the one you
put down. A player who saves the urn over the cash is judging what a stranger
will miss rather than what kind of person they are.

The handing back is the moral question, and it works because the first phase
already happened. "Would you steal from a woman whose house burnt down" is not a
question anybody answers honestly; being asked it while holding her mother's
ashes, which you went into a fire to get, is a different question. §3.10's
committed judgement is the second one.

So the run is bag takings followed by bag decisions, and the second half is
authored per item, because what it costs to keep a chair is not what it costs to
keep an urn.

The framework runs a challenge as a DAG of scenes ([Plan]) and a mechanic hands
it one. A bag of three chosen from nine cannot be written out by hand, but it is
still a scene graph. What a scene is named after is what makes that work:

	take            → nothing in the bag yet
	take:cash       → the cash is in the bag
	take:cash.dog   → both are, and the next thing fills it
	give:cash.dog.urn → three in the bag, none decided
	give:dog.urn      → the cash has been settled, these two have not
	give:urn          → last one

A scene is the set rather than the sequence. Picking the dog then the cash
arrives at the same scene as picking the cash then the dog, so the graph
converges instead of branching forever and picking the same thing twice is
impossible by construction. Nine items and a bag of three is 46 take scenes and
129 give scenes, small enough to validate at boot like any other plan. Save,
resume, once-ever, the trail, the scoring and the outcome bands all work here
without the framework learning that this challenge is different.

The give scenes are keyed by what is left to decide rather than what has been
decided, which is the same convergence one level down: how you settled the cash
does not change the question about the dog, and [tally] reads the trail anyway.

The schema:

	where: The landing outside 3B, and the door is already warm
	room:
	  - line: The smoke is at head height and going down.
	  - who: Dolores
	    line: The dog. Please, the dog.
	ask: You can carry three things out of here.
	bag: 3

	out:
	  where: The pavement opposite, with the engine still running
	  beats:
	    - who: Dolores
	      line: You went in. God, you went in.
	  ask: Everything in the bag is hers.

	items:
	  - key: dog
	    name: The dog
	    text: The dog, under the kitchen table, not moving.
	    taken: He is heavier than he looks and he does not fight you.
	    weight: 3
	    moves: {kindness: 40}
	    asked: He has not let go of your sleeve since you put him down.
	    give:
	      text: Give him back.
	      taken: She does not say anything for a while.
	      weight: 4
	      moves: {kindness: 60}
	    keep:
	      text: Say nothing, and keep him.
	      taken: She looks at the dog, and then at you, and does not ask.
	      weight: -8
	      moves: {kindness: -70}
	      takings: 0

Decoded strictly, so a misspelt field is a failed build, and validated twice like
every mechanic: by [salvaging.Configure] against the schema above, and by
[Plan.check] against the graph it projects into. Nothing here is random (§7).
*/

// salvageType is the name content asks for.
const salvageType Type = "salvage"

// haul is the one document this type reads, beside the challenge's own spec.
const haul = "salvage.yaml"

/*
scenesAllowed caps how large a generated graph may get.

The count is a sum of binomials, so it climbs fast and quietly: a bag of three
from nine items is 175 scenes, from fifteen it is 1,020, and from twenty-five it
is 5,150. None of those is a performance problem — the graph is built once at
boot and never touched again — but the largest of them is authored content
nobody has read, and a challenge with five thousand scenes in it is somebody
having made a mistake in a YAML file rather than somebody having written a very
big challenge. The number is generous enough that no honest challenge meets it
and small enough that the mistake is caught at the boot after it was made.
*/
const scenesAllowed = 2000

// salvaging is the mechanic type: registered once and holding nothing. One of
// these is shared by every challenge that names it, so a challenge's words live
// on what [salvaging.Configure] hands back.
type salvaging struct{}

func (salvaging) Type() Type { return salvageType }

// Configure reads one challenge's salvage.yaml and validates it. It refuses a
// document it does not read: a file beside a mechanic that never opens one is
// content somebody wrote and nobody runs.
func (salvaging) Configure(dir fs.FS) (Configured, error) {
	found, err := fs.ReadDir(dir, ".")
	if err != nil {
		return nil, fmt.Errorf("read the challenge directory: %w", err)
	}
	for _, f := range found {
		switch f.Name() {
		case spec, haul:
		default:
			return nil, fmt.Errorf("%s is not read by %s, which reads %s and nothing else",
				f.Name(), salvageType, haul)
		}
	}

	raw, err := fs.ReadFile(dir, haul)
	if err != nil {
		return nil, fmt.Errorf("%s runs on %s and has no %s: %w", spec, salvageType, haul, err)
	}
	var file haulFile
	if err := content.DecodeBytes(raw, &file); err != nil {
		return nil, fmt.Errorf("%s: %w", haul, err)
	}

	s, err := file.read()
	if err != nil {
		return nil, fmt.Errorf("%s %w", haul, err)
	}
	return s, nil
}

/* ── one configured salvage ────────────────────────────────────────────── */

// salvage is one configured challenge of this type: the words, and the plan
// generated from them. Made together, because a plan the framework accepts and
// this content has no words for is a blank screen. The plan is derived from the
// item list rather than written alongside it, so the two cannot disagree.
type salvage struct {
	// inside is the burning room: where it is, what is said in it, and the
	// question every take scene asks.
	inside room
	// outside is the pavement, and the question every give scene asks.
	outside room
	// items is everything that could be carried, in authored order. Bags are
	// sorted by that order, scene keys are composed in it and the graph is
	// walked in it, which is what makes the result deterministic (§7).
	items []item
	// bag is how many may be carried.
	bag int

	plan Plan
}

// room is a place in this challenge, and what is said on arriving in it.
type room struct {
	where string
	beats []Beat
	ask   string
}

// item is one thing in the house, and everything that can happen to it.
type item struct {
	key  string
	name string
	// text and taken are the choice in the burning room, and how it landed.
	text  string
	taken string
	// asked is what the pavement asks about this item. The scene's own ask
	// covers the moment; this covers the object.
	asked string

	weight int
	deltas []simulation.Grant

	give fate
	keep fate
}

// fate is one of the two things that can happen to a carried item.
type fate struct {
	text   string
	taken  string
	weight int
	deltas []simulation.Grant
	// takings is money this puts in the player's pocket — see [Option.Takings].
	// A roll of notes kept, a wallet, or a neighbour pressing something on you
	// for bringing the dog out.
	takings simulation.Money
}

// Plan is what the framework runs it by.
func (s *salvage) Plan() Plan { return s.plan }

/* ── generating the graph ──────────────────────────────────────────────── */

// takeKey names a scene by what is already in the bag. The empty bag is bare
// take rather than take:, so the opening reads as a word.
func takeKey(held []string) string {
	if len(held) == 0 {
		return "take"
	}
	return "take:" + strings.Join(held, ".")
}

// giveKey names a scene by what is left to decide.
func giveKey(left []string) string { return "give:" + strings.Join(left, ".") }

// carried takes a scene key apart again: the items it names, and which half of
// the run it is in. The framework hands a narrator a scene, because the scene
// is what was checkpointed.
func carried(key string) (phase string, items []string, ok bool) {
	head, rest, cut := strings.Cut(key, ":")
	switch {
	case head == "take" && !cut:
		return "take", nil, true
	case head == "take", head == "give":
		if rest == "" {
			return "", nil, false
		}
		return head, strings.Split(rest, "."), true
	}
	return "", nil, false
}

// generate builds the whole scene graph by walking it. Walking rather than
// enumerating, so every scene it produces was arrived at from the opening and
// [Plan.check]'s reachability rule holds by construction. Items are visited in
// authored order, so two boots produce identical graphs.
func (s *salvage) generate() error {
	if reach := s.size(); reach > scenesAllowed {
		return fmt.Errorf("carries %d of %d things, which is %d scenes and more than the %d a challenge may have",
			s.bag, len(s.items), reach, scenesAllowed)
	}

	seen := map[string]bool{}

	// The pavement, walked from the fullest bag down. `left` is what is still
	// undecided, so the scene deciding the last item ends the run.
	var pavement func(left []int)
	pavement = func(left []int) {
		key := giveKey(s.names(left))
		if seen[key] {
			return
		}
		seen[key] = true

		var onward string
		if len(left) > 1 {
			onward = giveKey(s.names(left[1:]))
		}
		it := s.items[left[0]]
		s.plan.Scenes = append(s.plan.Scenes, Scene{Key: key, Options: []Option{
			{Key: "give", Then: onward, Weight: it.give.weight, Deltas: it.give.deltas, Takings: it.give.takings},
			{Key: "keep", Then: onward, Weight: it.keep.weight, Deltas: it.keep.deltas, Takings: it.keep.takings},
		}})

		if len(left) > 1 {
			pavement(left[1:])
		}
	}

	// The house, walked from the empty bag up.
	var house func(held []int)
	house = func(held []int) {
		key := takeKey(s.names(held))
		if seen[key] {
			return
		}
		seen[key] = true

		full := len(held)+1 == s.bag
		scene := Scene{Key: key}
		var onward [][]int
		for i, it := range s.items {
			if has(held, i) {
				continue
			}
			with := insert(held, i)
			then := takeKey(s.names(with))
			if full {
				then = giveKey(s.names(with))
			}
			scene.Options = append(scene.Options, Option{
				Key: it.key, Then: then, Weight: it.weight, Deltas: it.deltas,
			})
			onward = append(onward, with)
		}
		s.plan.Scenes = append(s.plan.Scenes, scene)

		for _, with := range onward {
			if full {
				pavement(with)
			} else {
				house(with)
			}
		}
	}

	s.plan.Opening = takeKey(nil)
	house(nil)
	return nil
}

// names is a set of item indices as their keys, in authored order.
func (s *salvage) names(of []int) []string {
	out := make([]string, len(of))
	for i, at := range of {
		out[i] = s.items[at].key
	}
	return out
}

// has reports whether a sorted set contains an index.
func has(set []int, want int) bool {
	for _, at := range set {
		if at == want {
			return true
		}
	}
	return false
}

// insert puts an index into a sorted set, copying rather than aliasing — the
// caller keeps walking with the original.
func insert(set []int, add int) []int {
	out := make([]int, 0, len(set)+1)
	out = append(out, set...)
	out = append(out, add)
	sort.Ints(out)
	return out
}

/*
size is how many scenes this configuration will generate, counted rather than
built.

Take scenes are every bag with fewer than `bag` things in it; give scenes are
every set of undecided items that a full bag can be worked down to. Both are
sums of binomials, and counting them first is what lets [salvage.generate]
refuse an unreasonable challenge before spending the memory on it.

The give count is an upper bound rather than exact — it counts every subset of
size 1..bag, and the smallest of those are only reachable when there are enough
items ahead of them to fill the rest of the bag. Over-counting is the right
direction for a guard.
*/
func (s *salvage) size() int {
	n, out := len(s.items), 0
	for k := 0; k < s.bag; k++ {
		out += choose(n, k)
	}
	for k := 1; k <= s.bag; k++ {
		out += choose(n, k)
	}
	return out
}

// choose is n-choose-k, computed so it cannot overflow for anything that would
// pass the guard it feeds.
func choose(n, k int) int {
	if k < 0 || k > n {
		return 0
	}
	if k > n-k {
		k = n - k
	}
	out := 1
	for i := 0; i < k; i++ {
		out = out * (n - i) / (i + 1)
		if out > scenesAllowed*scenesAllowed {
			// Far past anything the guard would allow. Saturating rather than
			// overflowing keeps the refusal a refusal.
			return scenesAllowed * scenesAllowed
		}
	}
	return out
}

/* ── saying what is happening ──────────────────────────────────────────── */

/*
Say is the words, for whichever moment of a run it is asked about.

Two rooms and one rule: the choices are built by walking the *plan's* options and
attaching words to them by key, so a scene can never offer something
[Challenges.Choose] would refuse. In the house the keys are item keys; on the
pavement they are `give` and `keep`, and the item they are about is the first one
the scene still has left to decide.
*/
func (s *salvage) Say(run Run) (Told, bool) {
	phase, named, ok := carried(run.Scene.Key)
	if !ok {
		// A run standing at a scene this content no longer generates — the bag
		// size changed under it, or an item was withdrawn. There is nothing
		// truthful to say, and inventing something would be worse.
		return Told{}, false
	}

	out := Told{
		Taken:  s.taken(run.Trail),
		Stage:  len(run.Trail) + 1,
		Stages: len(run.Trail) + run.Challenge.Plan.Ahead(run.Scene.Key),
		Holds:  s.bag,
	}
	// What is in the bag right now, whichever half of the run this is. In the
	// house the scene key names what has been picked up; on the pavement it
	// names what is left to decide.
	for _, key := range named {
		if it, known := s.item(key); known {
			out.Held = append(out.Held, Carried{Key: it.key, Name: it.name})
		}
	}

	where := s.inside
	if phase == "give" {
		where = s.outside
	}
	out.Where = where.where

	if run.Done {
		// A finished run has nothing left to ask. What it has to say is how the
		// last thing landed — already in Taken — and then the reaction authored
		// for the band the whole trail scored into (§5.1).
		out.Stage, out.Stages = len(run.Trail), len(run.Trail)
		out.Closing = run.Outcome.Reaction
		return out, true
	}

	out.Beats = append(out.Beats, where.beats...)
	out.Ask = where.ask

	if phase == "take" {
		for _, o := range run.Scene.Options {
			if it, known := s.item(o.Key); known {
				out.Choices = append(out.Choices, Choice{Key: it.key, Text: it.text})
			}
		}
		return out, true
	}

	it, known := s.item(named[0])
	if !known {
		return Told{}, false
	}
	out.Beats = append(out.Beats, Beat{Line: it.asked})
	for _, o := range run.Scene.Options {
		switch o.Key {
		case "give":
			out.Choices = append(out.Choices, Choice{Key: o.Key, Text: it.give.text})
		case "keep":
			out.Choices = append(out.Choices, Choice{Key: o.Key, Text: it.keep.text})
		}
	}
	return out, true
}

/*
taken is how the last thing the player committed to was received.

Read off the trail rather than off the scene, because the scene the run is
standing at now is the one *after* the choice — and on the pavement the same
option key (`give`) means a different sentence depending on which item it was
about, which is exactly what the trail's scene key records.
*/
func (s *salvage) taken(trail []Step) string {
	if len(trail) == 0 {
		return ""
	}
	last := trail[len(trail)-1]
	phase, named, ok := carried(last.Scene)
	if !ok {
		return ""
	}
	if phase == "take" {
		it, known := s.item(last.Option)
		if !known {
			return ""
		}
		return it.taken
	}

	it, known := s.item(named[0])
	if !known {
		return ""
	}
	switch last.Option {
	case "give":
		return it.give.taken
	case "keep":
		return it.keep.taken
	}
	return ""
}

// item finds one by key.
func (s *salvage) item(key string) (item, bool) {
	for _, it := range s.items {
		if it.key == key {
			return it, true
		}
	}
	return item{}, false
}

/* ── the config document ───────────────────────────────────────────────── */

// haulFile is salvage.yaml, exactly — §5.2's "typed config schema".
type haulFile struct {
	Where string     `yaml:"where"`
	Room  []beatFile `yaml:"room"`
	Ask   string     `yaml:"ask"`
	// Bag is how many things may be carried out. Required: a mechanic whose
	// whole subject is what you had to leave behind should not have a default
	// for how much you could take.
	Bag int `yaml:"bag"`

	Out      outFile       `yaml:"out"`
	Items    []itemFile    `yaml:"items"`
	Outcomes []outcomeFile `yaml:"outcomes"`
}

// outFile is the second room: where the player ends up, and who is there.
type outFile struct {
	Where string     `yaml:"where"`
	Beats []beatFile `yaml:"beats"`
	Ask   string     `yaml:"ask"`
}

type itemFile struct {
	Key  string `yaml:"key"`
	Name string `yaml:"name"`
	// Text is the choice in the burning room; Taken is how reaching for it went.
	Text  string `yaml:"text"`
	Taken string `yaml:"taken"`
	// Asked is what the pavement puts to the player about this item.
	Asked string `yaml:"asked"`

	Weight int                `yaml:"weight"`
	Moves  map[string]float64 `yaml:"moves"`

	Give fateFile `yaml:"give"`
	Keep fateFile `yaml:"keep"`
}

type fateFile struct {
	Text   string             `yaml:"text"`
	Taken  string             `yaml:"taken"`
	Weight int                `yaml:"weight"`
	Moves  map[string]float64 `yaml:"moves"`
	// Takings is money, in cents, this puts in the player's pocket. See
	// [Option.Takings] for why a challenge is allowed to do that at all.
	Takings int64 `yaml:"takings"`
}

/*
read validates a decoded document and generates the challenge from it.

Everything checked here is about the *content* — a room nobody is in, an item
with nothing written on it, a bag bigger than the house. The graph itself is
[Plan.check]'s and is deliberately not re-checked: the whole reason the plan is
generated is that nobody has to.

The errors read as continuations of "salvage.yaml …", which is how the loader
prefixes them.
*/
func (f haulFile) read() (*salvage, error) {
	out := &salvage{bag: f.Bag}

	var err error
	if out.inside, err = f.inside(); err != nil {
		return nil, err
	}
	if out.outside, err = f.outside(); err != nil {
		return nil, err
	}

	if f.Bag < 1 {
		return nil, fmt.Errorf("has a bag that holds %d, and a run that carries nothing has nothing to decide", f.Bag)
	}
	if len(f.Items) <= f.Bag {
		// The whole mechanic is the thing you had to put down. A house with
		// exactly as many things in it as the bag holds is a corridor with
		// scenery, and [Plan.check] would let it through because every scene
		// technically still offers something.
		return nil, fmt.Errorf("holds %d things and has %d in the house, so nothing is ever left behind",
			f.Bag, len(f.Items))
	}

	taken := map[string]bool{}
	for _, i := range f.Items {
		it, err := i.read()
		if err != nil {
			return nil, err
		}
		if taken[it.key] {
			return nil, fmt.Errorf("has two items called %q", it.key)
		}
		taken[it.key] = true
		out.items = append(out.items, it)
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

	if err := out.generate(); err != nil {
		return nil, err
	}
	return out, nil
}

func (f haulFile) inside() (room, error) {
	return readRoom("the house", f.Where, f.Room, f.Ask)
}

func (f haulFile) outside() (room, error) {
	return readRoom("the pavement", f.Out.Where, f.Out.Beats, f.Out.Ask)
}

// readRoom validates one of the two places this challenge happens in.
func readRoom(which, where string, beats []beatFile, ask string) (room, error) {
	if strings.TrimSpace(where) == "" {
		return room{}, fmt.Errorf("says nowhere about %s, so the player cannot be told where they are", which)
	}
	if len(beats) == 0 {
		return room{}, fmt.Errorf("has nothing happening on %s", which)
	}
	if strings.TrimSpace(ask) == "" {
		return room{}, fmt.Errorf("asks nothing on %s, so its options answer nothing", which)
	}
	out := room{where: strings.TrimSpace(where), ask: strings.TrimSpace(ask)}
	for _, b := range beats {
		if strings.TrimSpace(b.Line) == "" {
			return room{}, fmt.Errorf("has a beat with no line in it on %s", which)
		}
		out.beats = append(out.beats, Beat{Who: strings.TrimSpace(b.Who), Line: strings.TrimSpace(b.Line)})
	}
	return out, nil
}

// read validates one item and both things that can happen to it.
func (i itemFile) read() (item, error) {
	key := strings.TrimSpace(i.Key)
	if err := slug(key); err != nil {
		return item{}, fmt.Errorf("has an item whose key %q %w", i.Key, err)
	}
	if strings.ContainsAny(key, ".:") {
		// Unreachable while [slug] holds, and asserted anyway because the scene
		// keys are composed with both characters — an item key containing one
		// would make two different bags share a scene, which is the one failure
		// mode of the whole generated-graph idea.
		return item{}, fmt.Errorf("item %q has a . or a : in it, which is how bags are named", key)
	}

	out := item{
		key:    key,
		name:   strings.TrimSpace(i.Name),
		text:   strings.TrimSpace(i.Text),
		taken:  strings.TrimSpace(i.Taken),
		asked:  strings.TrimSpace(i.Asked),
		weight: i.Weight,
	}
	switch {
	case out.name == "":
		return item{}, fmt.Errorf("item %q has no name, so a full bag cannot be read back", key)
	case out.text == "":
		return item{}, fmt.Errorf("item %q has nothing written on it", key)
	case out.asked == "":
		return item{}, fmt.Errorf("item %q is never asked about once it is out of the house", key)
	}

	var err error
	if out.deltas, err = moves(i.Moves); err != nil {
		return item{}, fmt.Errorf("item %q %w", key, err)
	}
	if out.give, err = i.Give.read(); err != nil {
		return item{}, fmt.Errorf("item %q, giving it back, %w", key, err)
	}
	if out.keep, err = i.Keep.read(); err != nil {
		return item{}, fmt.Errorf("item %q, keeping it, %w", key, err)
	}
	return out, nil
}

// read validates one of an item's two fates.
func (f fateFile) read() (fate, error) {
	out := fate{
		text:    strings.TrimSpace(f.Text),
		taken:   strings.TrimSpace(f.Taken),
		weight:  f.Weight,
		takings: simulation.Money(f.Takings),
	}
	switch {
	case out.text == "":
		return fate{}, fmt.Errorf("has nothing written on it")
	case out.takings < 0:
		return fate{}, fmt.Errorf("takes money off the player, and a challenge only ever hands things over")
	}

	var err error
	if out.deltas, err = moves(f.Moves); err != nil {
		return fate{}, err
	}
	return out, nil
}

// moves reads a `moves:` block into grants, in the simulation's own order. See
// [sorted] and [moved], the pieces it is made of.
func moves(written map[string]float64) ([]simulation.Grant, error) {
	var out []simulation.Grant
	for _, name := range sorted(written) {
		which, err := moved(name)
		if err != nil {
			return nil, err
		}
		out = append(out, simulation.Grant{Factor: which, Credit: written[name]})
	}
	return out, nil
}
