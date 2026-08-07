package challenge

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/event"
)

/*
The first authored challenge (#24), as content rather than as machinery.

What is worth proving about a challenge is not that the framework works — that
is what the rest of this package's tests are for. It is the set of claims §3.10
makes about what a challenge *is*, checked against the one that exists: roughly
six minutes, multiple stages with genuine branching, a committed judgement
chosen from authored positions with no correct one, reactions authored per
outcome bucket, hidden scoring that never appears as a grade, and characters who
react to how they *perceive* the reading the player committed to.

Every one of those is a thing an author can quietly break while editing prose,
and none of them is visible in a diff.
*/

// theArgument is the shipped definition, found the way anything finds it.
func theArgument(t *testing.T) Definition {
	t.Helper()
	d, known := Known(Of("resolve_conflict", "a"))
	if !known {
		t.Fatal("the first authored challenge is not in the shipped catalogue")
	}
	return d
}

/*
passage is one complete way through a plan: the steps, and what they scored.

Enumerated exhaustively rather than sampled. The graph is small and acyclic by
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

func TestTheFirstChallengeIsAuthoredAndRunnable(t *testing.T) {
	d := theArgument(t)

	if d.Mechanic != theStoop {
		t.Fatalf("it runs on %q", d.Mechanic)
	}
	if d.Name == "" {
		t.Fatal("it has no name")
	}
	// The loader already ran [Plan.check] over this, so re-running it here is
	// not the point. What is worth saying is that the plan reached the
	// catalogue at all: a mechanic that returned nothing would have been a
	// failed build, and a mechanic registered too late would have been one too.
	if len(d.Plan.Scenes) == 0 {
		t.Fatal("it has no scenes")
	}
}

/*
§3.10: "multi-stage scenario with real branching", and §5.1's six minutes.

Five scenes on every passage, which is what makes the length a property of the
content rather than of how long somebody sat there. The branching is checked
separately below; this is the claim that no route through is a short one.
*/
func TestEveryPassageStandsInFiveScenes(t *testing.T) {
	all := passages(theArgument(t).Plan)
	if len(all) < 100 {
		t.Fatalf("%d ways through, which is not branching", len(all))
	}
	for _, p := range all {
		if len(p.steps) != 5 {
			t.Fatalf("a passage of %d scenes: %v", len(p.steps), p.steps)
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
*/
func TestTheReadingsAreFourAndNoneOfThemIsCorrect(t *testing.T) {
	plan := theArgument(t).Plan
	judgement, ok := plan.Scene("laying-it-out")
	if !ok {
		t.Fatal("the scene the player commits at is not in the plan")
	}
	if len(judgement.Options) != 4 {
		t.Fatalf("%d readings to choose between", len(judgement.Options))
	}

	behind := map[string]string{}
	low, high := judgement.Options[0].Weight, judgement.Options[0].Weight
	for _, o := range judgement.Options {
		if first, taken := behind[o.Then]; taken {
			t.Fatalf("%q and %q both lead to %q, so they are the same reading twice", first, o.Key, o.Then)
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
		t.Fatalf("the readings are worth between %d and %d, which makes one of them the right answer", low, high)
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
	plan := theArgument(t).Plan

	landed := map[string]int{}
	for _, p := range passages(plan) {
		bucket, ok := plan.Outcome(p.score)
		if !ok {
			t.Fatalf("a passage scoring %d lands in no outcome: %v", p.score, p.steps)
		}
		landed[bucket.Key]++
	}

	for _, b := range plan.Buckets {
		if landed[b.Key] == 0 {
			t.Errorf("nobody can ever be told %q, so its reaction is unreadable", b.Key)
		}
		if strings.TrimSpace(b.Reaction) == "" {
			t.Errorf("the outcome %q has nothing to say", b.Key)
		}
	}
	if len(plan.Buckets) < 3 {
		t.Fatalf("%d outcomes, which is not a band", len(plan.Buckets))
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
	d := theArgument(t)

	shortest, longest := 0, 0
	for i, p := range passages(d.Plan) {
		words := 0
		for _, step := range p.steps {
			scene, ok := written(step.Scene)
			if !ok {
				t.Fatalf("%q is in the plan and not in the script", step.Scene)
			}
			for _, b := range scene.beats {
				words += len(strings.Fields(b.Line))
			}
			words += len(strings.Fields(scene.ask))
			// Every option at the scene, because they are all read before one
			// of them is committed to.
			for _, o := range scene.positions {
				words += len(strings.Fields(o.text))
			}
			if o, authored := scene.position(step.Option); authored {
				words += len(strings.Fields(o.taken))
			}
		}
		if bucket, ok := d.Plan.Outcome(p.score); ok {
			words += len(strings.Fields(bucket.Reaction))
		}

		if i == 0 || words < shortest {
			shortest = words
		}
		if words > longest {
			longest = words
		}
	}

	t.Logf("a passage reads between %d and %d words", shortest, longest)
	if shortest < 700 {
		t.Errorf("the shortest way through is %d words, which is not six minutes of anything", shortest)
	}
	if longest > 2400 {
		t.Errorf("the longest way through is %d words, which is a short story", longest)
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
	d := theArgument(t)

	said := map[string]string{}
	for _, scene := range argument() {
		for _, p := range scene.positions {
			if strings.TrimSpace(p.text) == "" {
				t.Errorf("%s/%s is a choice with nothing written on it", scene.key, p.key)
			}
			if strings.TrimSpace(p.taken) == "" {
				t.Errorf("%s/%s says nothing about how it was taken", scene.key, p.key)
				continue
			}
			if where, twice := said[p.taken]; twice {
				t.Errorf("%s/%s is received exactly as %s is", scene.key, p.key, where)
			}
			said[p.taken] = scene.key + "/" + p.key
		}
	}

	// And the run carries it: the same scene, arrived at by two different
	// readings, is narrated differently.
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
	d := theArgument(t)

	for _, scene := range d.Plan.Scenes {
		told, ok := Say(Run{Challenge: d, Scene: scene})
		if !ok {
			t.Fatalf("%q has no words", scene.Key)
		}
		if len(told.Choices) != len(scene.Options) {
			t.Fatalf("%q offers %d options and %d of them are written",
				scene.Key, len(scene.Options), len(told.Choices))
		}
		for i, c := range told.Choices {
			if c.Key != scene.Options[i].Key {
				t.Fatalf("%q offers %q where the plan has %q", scene.Key, c.Key, scene.Options[i].Key)
			}
			if strings.TrimSpace(c.Text) == "" {
				t.Fatalf("%q offers %q with nothing written on it", scene.Key, c.Key)
			}
		}
		if strings.TrimSpace(told.Where) == "" || strings.TrimSpace(told.Ask) == "" {
			t.Fatalf("%q is a scene with no room and no question", scene.Key)
		}
		if told.Stage < 1 || told.Stage > told.Stages {
			t.Fatalf("%q is stage %d of %d", scene.Key, told.Stage, told.Stages)
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
	d := theArgument(t)
	done := Run{
		Challenge: d, Done: true, Scene: sceneOf(t, d, "last-word-steps"),
		Trail:   []Step{{Scene: "last-word-steps", Option: "walk-up"}},
		Outcome: d.Plan.Buckets[0],
	}
	end, ok := Say(done)
	if !ok {
		t.Fatal("a finished run says nothing")
	}
	if end.Closing != d.Plan.Buckets[0].Reaction {
		t.Fatal("a finished run does not close on its authored reaction")
	}
	if len(end.Choices) != 0 || end.Ask != "" {
		t.Fatal("a finished run is still asking something")
	}
	if strings.Contains(end.Closing, d.Plan.Buckets[0].Key) {
		t.Fatal("the reaction names the bucket, which is the grade said out loud")
	}
}

/*
§4's two ways in, both authored and both resolved at boot.

The event is the diegetic one — "you pass two people arguing" — and the standing
entry point is the other half of the same section: they are on that step whether
or not the world has just put them in front of you.
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

// §5.2's "written as code": a config document beside a mechanic that reads none
// is content nobody runs, and it fails the build rather than looking authored.
func TestAContentDocumentNobodyReadsFailsTheBuild(t *testing.T) {
	_, err := stoop{}.Configure(fstest.MapFS{
		spec:            &fstest.MapFile{Data: []byte("name: x\n")},
		"dialogue.yaml": &fstest.MapFile{Data: []byte("scenes: []\n")},
	})
	if err == nil {
		t.Fatal("a dialogue.yaml nothing reads was accepted")
	}
	if !strings.Contains(err.Error(), "dialogue.yaml") {
		t.Fatalf("the refusal does not name the file: %v", err)
	}
}

// sceneOf is one scene of the shipped plan, by key.
func sceneOf(t *testing.T, d Definition, key string) Scene {
	t.Helper()
	s, ok := d.Plan.Scene(key)
	if !ok {
		t.Fatalf("%q is not a scene of %s", key, d.Code)
	}
	return s
}
