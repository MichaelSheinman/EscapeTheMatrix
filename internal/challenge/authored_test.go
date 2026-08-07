package challenge

import (
	"reflect"
	"strings"
	"testing"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/event"
)

/*
The authored challenges, as content rather than as machinery.

What is worth proving here is not that the framework works — that is what the
rest of this package's tests are for. It is the set of claims §3.10 makes about
what a challenge *is*, checked against the ones that exist: roughly six minutes,
multiple stages with genuine branching, a committed judgement chosen from
authored positions with no correct one, reactions authored per outcome bucket,
hidden scoring that never appears as a grade, and characters who react to how
they *perceive* the reading the player committed to.

Every one of those is a thing an author can quietly break while editing prose,
and none of them is visible in a diff.

**They run over every shipped challenge rather than over one**, which is the
half of #25 that outlives the extraction. Two exist — the argument on the stoop
and the Sunday at the laundromat — and the second one shipped as two YAML files
with no Go written for it. A test that named the first by hand would have gone on
passing while the second was authored into something that is not a challenge at
all.
*/

// shipped is every authored challenge, found the way anything finds one.
func shipped(t *testing.T) []Definition {
	t.Helper()
	all := All()
	if len(all) < 2 {
		// §5.2's whole line is drawn by there being a second instance. One
		// challenge cannot show that a type is a type.
		t.Fatalf("%d challenges are authored, and the boundary needs two", len(all))
	}
	return all
}

// theArgument is the first one, for the claims that are about it in particular.
func theArgument(t *testing.T) Definition {
	t.Helper()
	d, known := Known(Of("resolve_conflict", "a"))
	if !known {
		t.Fatal("the first authored challenge is not in the shipped catalogue")
	}
	return d
}

// words is a challenge's own script, which only its own type can read.
func words(t *testing.T, d Definition) *dialogue {
	t.Helper()
	script, ok := d.Made.(*dialogue)
	if !ok {
		t.Fatalf("%s does not run on %s", d.Code, dialogueBranch)
	}
	return script
}

/*
passage is one complete way through a plan: the steps, and what they scored.

Enumerated exhaustively rather than sampled. The graphs are small and acyclic by
construction, and the questions below — is every outcome reachable, does every
passage stand in five scenes — are about *all* of them.
*/
type passage struct {
	steps []Step
	score int
}

func passages(p Plan) []passage {
	var out []passage

	var walk func(at string, taken []Step, score int)
	walk = func(at string, taken []Step, score int) {
		scene, ok := p.Scene(at)
		if !ok {
			return
		}
		for _, o := range scene.Options {
			here := append(append([]Step{}, taken...), Step{Scene: at, Option: o.Key})
			if o.Ends() {
				out = append(out, passage{steps: here, score: score + o.Weight})
				continue
			}
			walk(o.Then, here, score+o.Weight)
		}
	}
	walk(p.Opening, nil, 0)
	return out
}

func TestEveryAuthoredChallengeIsRunnable(t *testing.T) {
	for _, d := range shipped(t) {
		if _, known := mechanics[d.Mechanic]; !known {
			t.Errorf("%s runs on %q, which is not a registered mechanic type", d.Code, d.Mechanic)
		}
		if d.Name == "" {
			t.Errorf("%s has no name", d.Code)
		}
		// The loader already ran [Plan.check] over these, so re-running it here
		// is not the point. What is worth saying is that the plan reached the
		// catalogue at all: a mechanic that returned nothing would have been a
		// failed build, and a mechanic registered too late would have been one
		// too.
		if len(d.Plan.Scenes) == 0 {
			t.Errorf("%s has no scenes", d.Code)
		}
		if d.Made == nil {
			t.Errorf("%s carries no configured challenge, so nothing can narrate it", d.Code)
		}
	}
}

/*
§5.2's line, held by the only thing that can hold it: a second instance.

	**Mechanic types are code. Challenges are data.**
	- A challenge is `mechanic type + config document + content files`.
	- New content in an existing type is data only. **A new mechanic is code.**

So: more challenges than mechanic types, and a type with two challenges on it. A
repo in which every challenge has its own type has not extracted anything — it
has renamed one file — and that is exactly the state #24 left and #25 was for.
*/
func TestThereAreMoreChallengesThanMechanicTypes(t *testing.T) {
	all := shipped(t)

	on := map[Type][]Code{}
	for _, d := range all {
		on[d.Mechanic] = append(on[d.Mechanic], d.Code)
	}
	if len(on) >= len(all) {
		t.Fatalf("%d challenges on %d mechanic types, which is a type per challenge", len(all), len(on))
	}

	shared := false
	for name, codes := range on {
		if len(codes) > 1 {
			shared = true
			t.Logf("%s runs %v", name, codes)
		}
	}
	if !shared {
		t.Fatal("no mechanic type runs more than one challenge, so none of them is a type yet")
	}
}

/*
§3.10: "multi-stage scenario with real branching", and §5.1's six minutes.

Five scenes on every passage, which is what makes the length a property of the
content rather than of how long somebody sat there. The branching is checked
separately below; this is the claim that no route through is a short one.
*/
func TestEveryPassageStandsInFiveScenes(t *testing.T) {
	for _, d := range shipped(t) {
		all := passages(d.Plan)
		if len(all) < 100 {
			t.Errorf("%s has %d ways through, which is not branching", d.Code, len(all))
		}
		for _, p := range all {
			if len(p.steps) != 5 {
				t.Fatalf("%s has a passage of %d scenes: %v", d.Code, len(p.steps), p.steps)
			}
		}
	}
}

/*
§3.10's committed judgement, checked as a shape.

	the player selects the interpretation they will act on from a set of
	authored positions. There is no correct one; each leads somewhere different

So: four readings, four *different* scenes behind them, and weights close enough
together that none of them is the answer. A reading worth two more than its
neighbours would be a right answer with the label taken off.

The scene is found rather than named: it is the one where the most is on offer,
which is what "the committed judgement" means in a graph.
*/
func TestTheReadingsAreFourAndNoneOfThemIsCorrect(t *testing.T) {
	for _, d := range shipped(t) {
		var judgement Scene
		for _, s := range d.Plan.Scenes {
			if len(s.Options) > len(judgement.Options) {
				judgement = s
			}
		}
		if len(judgement.Options) != 4 {
			t.Errorf("%s offers at most %d readings to choose between", d.Code, len(judgement.Options))
			continue
		}

		behind := map[string]string{}
		low, high := judgement.Options[0].Weight, judgement.Options[0].Weight
		for _, o := range judgement.Options {
			if o.Ends() {
				continue
			}
			if first, taken := behind[o.Then]; taken {
				t.Errorf("%s: %q and %q both lead to %q, so they are the same reading twice",
					d.Code, first, o.Key, o.Then)
			}
			behind[o.Then] = o.Key
			if o.Weight < low {
				low = o.Weight
			}
			if o.Weight > high {
				high = o.Weight
			}
		}
		if high-low > 1 {
			t.Errorf("%s: the readings at %q are worth between %d and %d, which makes one of them the right answer",
				d.Code, judgement.Key, low, high)
		}
	}
}

/*
§5.1: "reactions are authored per outcome bucket … with written reaction text
per branch" — and an outcome nobody can reach is reaction text nobody will read.

[Plan.buckets] already refuses a plan whose best passage cannot reach its
highest band. What it does not check, and cannot cheaply, is the middle: a band
between two others that no actual passage lands in. This walks every one.
*/
func TestEveryOutcomeIsReachedBySomePassage(t *testing.T) {
	for _, d := range shipped(t) {
		landed := map[string]int{}
		for _, p := range passages(d.Plan) {
			bucket, ok := d.Plan.Outcome(p.score)
			if !ok {
				t.Fatalf("%s: a passage scoring %d lands in no outcome: %v", d.Code, p.score, p.steps)
			}
			landed[bucket.Key]++
		}

		for _, b := range d.Plan.Buckets {
			if landed[b.Key] == 0 {
				t.Errorf("%s: nobody can ever be told %q, so its reaction is unreadable", d.Code, b.Key)
			}
			if strings.TrimSpace(b.Reaction) == "" {
				t.Errorf("%s: the outcome %q has nothing to say", d.Code, b.Key)
			}
		}
		if len(d.Plan.Buckets) < 3 {
			t.Errorf("%s has %d outcomes, which is not a band", d.Code, len(d.Plan.Buckets))
		}
	}
}

/*
§3.10's "roughly 6 minutes", as far as authored words can be held to it.

Reading is about 200 words a minute and this is content that is read rather than
watched, so a passage is its prose plus the time spent deciding five times over
what to commit to. The bounds are wide because the claim is "roughly": what they
actually catch is the two ways this stops being a six-minute endeavour — an act
quietly gutted while editing, and one grown into a novel.
*/
func TestAPassageIsAboutSixMinutes(t *testing.T) {
	for _, d := range shipped(t) {
		script := words(t, d)

		shortest, longest := 0, 0
		for i, p := range passages(d.Plan) {
			count := 0
			for _, step := range p.steps {
				scene, ok := script.act(step.Scene)
				if !ok {
					t.Fatalf("%s: %q is in the plan and not in the script", d.Code, step.Scene)
				}
				for _, b := range scene.beats {
					count += len(strings.Fields(b.Line))
				}
				count += len(strings.Fields(scene.ask))
				// Every option at the scene, because they are all read before
				// one of them is committed to.
				for _, o := range scene.positions {
					count += len(strings.Fields(o.text))
				}
				if o, authored := scene.position(step.Option); authored {
					count += len(strings.Fields(o.taken))
				}
			}
			if bucket, ok := d.Plan.Outcome(p.score); ok {
				count += len(strings.Fields(bucket.Reaction))
			}

			if i == 0 || count < shortest {
				shortest = count
			}
			if count > longest {
				longest = count
			}
		}

		t.Logf("%s reads between %d and %d words", d.Code, shortest, longest)
		if shortest < 700 {
			t.Errorf("%s: the shortest way through is %d words, which is not six minutes of anything",
				d.Code, shortest)
		}
		if longest > 2400 {
			t.Errorf("%s: the longest way through is %d words, which is a short story", d.Code, longest)
		}
	}
}

/*
§3.10: "the characters then react according to how they perceive your
interpretation."

Authored per option rather than per scene, which is the thing worth holding: two
readings that arrive at the same room are received completely differently, and
the sentence that says so has to survive an edit that moves a scene about.
*/
func TestTheCharactersReactToWhatTheyThinkYouMeant(t *testing.T) {
	for _, d := range shipped(t) {
		said := map[string]string{}
		for _, scene := range words(t, d).acts {
			for _, p := range scene.positions {
				if where, twice := said[p.taken]; twice {
					t.Errorf("%s: %s/%s is received exactly as %s is", d.Code, scene.key, p.key, where)
				}
				said[p.taken] = scene.key + "/" + p.key
			}
		}
	}

	// And the run carries it: the same scene, arrived at by two different
	// readings, is narrated differently.
	d := theArgument(t)
	first := Run{Challenge: d, Scene: sceneOf(t, d, "kitchen"),
		Trail: []Step{{Scene: "laying-it-out", Option: "leaving"}}}
	second := first
	second.Trail = []Step{{Scene: "laying-it-out", Option: "money"}}

	one, ok := Say(first)
	if !ok {
		t.Fatal("the mechanic says nothing about its own scene")
	}
	two, _ := Say(second)
	if one.Taken == "" || one.Taken == two.Taken {
		t.Fatalf("both readings were taken the same way: %q", one.Taken)
	}
	if one.Ask != two.Ask || len(one.Choices) != len(two.Choices) {
		t.Fatal("the scene itself changed, which is branching rather than perception")
	}
}

// The words and the plan are one list, so a screen cannot offer a choice the
// run would refuse, or refuse to offer one it would take.
func TestTheWordsAndThePlanCannotComeApart(t *testing.T) {
	for _, d := range shipped(t) {
		for _, scene := range d.Plan.Scenes {
			told, ok := Say(Run{Challenge: d, Scene: scene})
			if !ok {
				t.Fatalf("%s: %q has no words", d.Code, scene.Key)
			}
			if len(told.Choices) != len(scene.Options) {
				t.Fatalf("%s: %q offers %d options and %d of them are written",
					d.Code, scene.Key, len(scene.Options), len(told.Choices))
			}
			for i, c := range told.Choices {
				if c.Key != scene.Options[i].Key {
					t.Fatalf("%s: %q offers %q where the plan has %q",
						d.Code, scene.Key, c.Key, scene.Options[i].Key)
				}
				if strings.TrimSpace(c.Text) == "" {
					t.Fatalf("%s: %q offers %q with nothing written on it", d.Code, scene.Key, c.Key)
				}
			}
			if strings.TrimSpace(told.Where) == "" || strings.TrimSpace(told.Ask) == "" {
				t.Fatalf("%s: %q is a scene with no room and no question", d.Code, scene.Key)
			}
			if told.Stage < 1 || told.Stage > told.Stages {
				t.Fatalf("%s: %q is stage %d of %d", d.Code, scene.Key, told.Stage, told.Stages)
			}
		}
	}
}

/*
§3.10's hidden scoring: "never surfaced as a grade, never confirmed as right."

The guard is structural rather than textual, because the thing to prevent is a
field being added rather than a word being written. What may cross out of this
package is text, the keys a choice is made by, and how far through the run is —
and a weight, a running total or the name of the band the player is currently in
is none of those.
*/
func TestNothingToldIsAGrade(t *testing.T) {
	allowed := map[string]bool{
		"Where": true, "Taken": true, "Beats": true, "Ask": true,
		"Choices": true, "Closing": true, "Stage": true, "Stages": true,
	}
	told := reflect.TypeOf(Told{})
	for i := range told.NumField() {
		if !allowed[told.Field(i).Name] {
			t.Errorf("Told carries %s, which the player would be reading as a mark",
				told.Field(i).Name)
		}
	}

	// A finished run says the outcome's reaction and never its key: the key is
	// production vocabulary, and §3.2 does not label a framework for a player.
	for _, d := range shipped(t) {
		last := d.Plan.Scenes[len(d.Plan.Scenes)-1]
		done := Run{
			Challenge: d, Done: true, Scene: last,
			Trail:   []Step{{Scene: last.Key, Option: last.Options[0].Key}},
			Outcome: d.Plan.Buckets[0],
		}
		end, ok := Say(done)
		if !ok {
			t.Fatalf("%s: a finished run says nothing", d.Code)
		}
		if end.Closing != d.Plan.Buckets[0].Reaction {
			t.Errorf("%s: a finished run does not close on its authored reaction", d.Code)
		}
		if len(end.Choices) != 0 || end.Ask != "" {
			t.Errorf("%s: a finished run is still asking something", d.Code)
		}
		for _, b := range d.Plan.Buckets {
			if strings.Contains(end.Closing, b.Key) {
				t.Errorf("%s: the reaction names the bucket %q, which is the grade said out loud",
					d.Code, b.Key)
			}
		}
	}
}

/*
§4's two ways in, both authored and both resolved at boot.

The argument on the stoop has both: the event is the diegetic one — "you pass two
people arguing" — and the standing entry point is the other half of the same
section, because they are on that step whether or not the world has just put them
in front of you.
*/
func TestItIsBothOfferedAndAvailable(t *testing.T) {
	d := theArgument(t)

	if d.Opens == "" {
		t.Fatal("nothing opens it")
	}
	opener, known := event.Known(d.Opens)
	switch {
	case !known:
		t.Fatalf("%q opens it and is not an event", d.Opens)
	case !opener.Awaits:
		t.Fatalf("%q finishes on the spot, so there is no moment this begins in", d.Opens)
	}
	if behind, ok := Opened(d.Opens); !ok || behind.Code != d.Code {
		t.Fatalf("the event does not lead back to the challenge")
	}

	if !d.Standing() {
		t.Fatal("it stands nowhere of its own accord")
	}
	if zone, there := d.Where.At("east-side"); !there || zone == "" {
		t.Fatal("it does not stand where the player lives")
	}
}

/*
And §4's other half on its own, which nothing had shown until there were two
challenges: "some challenges are simply always available".

The Sunday at the laundromat names no event. Nobody is put in front of you; you
walk in with your washing and they are already at it — so the only thing that can
begin it is standing somewhere, and that somewhere has to resolve to a real spot
in a real place at boot.
*/
func TestOneStandsThereWithNothingOfferingIt(t *testing.T) {
	d, known := Known(Of("resolve_conflict", "b"))
	if !known {
		t.Fatal("the second authored challenge is not in the shipped catalogue")
	}
	if d.Opens != "" {
		t.Fatalf("it is opened by %q, so it is not the availability-only case", d.Opens)
	}
	if !d.Standing() {
		t.Fatal("it stands nowhere and nothing opens it, which the loader should have refused")
	}

	places := d.Where.Places()
	if len(places) == 0 {
		t.Fatal("its entry point resolved to nowhere")
	}
	for _, place := range places {
		zone, there := d.Where.At(place)
		if !there || zone == "" {
			t.Errorf("it is listed at %q and stands in no spot there", place)
		}
	}
	t.Logf("%s stands at %v", d.Code, places)
}

// sceneOf is one scene of a shipped plan, by key.
func sceneOf(t *testing.T, d Definition, key string) Scene {
	t.Helper()
	s, ok := d.Plan.Scene(key)
	if !ok {
		t.Fatalf("%q is not a scene of %s", key, d.Code)
	}
	return s
}
