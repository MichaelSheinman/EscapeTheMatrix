package challenge

import (
	"strings"
	"testing"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
The framework's half of §5.2's validation: what a plan has to be, whatever
mechanic produced it.

Every case below is a way for an authored challenge to be broken that no single
content file shows and that no test of the mechanic itself would catch — a scene
whose only way in was renamed, a loop somebody could farm, an outcome nobody can
reach. §5.2 says a malformed challenge fails in CI rather than at 2am, and these
are the malformations the framework is responsible for noticing.
*/

func refuse(t *testing.T, p Plan, want string) {
	t.Helper()
	err := p.check()
	if err == nil {
		t.Fatalf("check() = nil, want a refusal mentioning %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("check() = %q, want it to mention %q", err, want)
	}
}

func TestARunnablePlanIsAccepted(t *testing.T) {
	p := twoScenes()
	if err := p.check(); err != nil {
		t.Fatalf("check() = %v, want nil", err)
	}
	// The outcomes come back in order however they were authored, so reading
	// one is not a function of the order somebody typed them in.
	for i := 1; i < len(p.Buckets); i++ {
		if p.Buckets[i-1].AtLeast >= p.Buckets[i].AtLeast {
			t.Fatalf("buckets are not in order: %+v", p.Buckets)
		}
	}
}

func TestAPlanNeedsSomewhereToBegin(t *testing.T) {
	p := twoScenes()
	p.Opening = "hallway"
	refuse(t, p, "not a scene")
}

func TestAnOptionCannotLeadNowhereInParticular(t *testing.T) {
	p := twoScenes()
	p.Scenes[0].Options[0].Then = "the-roof"
	refuse(t, p, "not a scene")
}

// A scene nothing leads to is authored and unplayable, and it is almost always
// the one whose only way in was renamed.
func TestASceneNothingLeadsToIsRefused(t *testing.T) {
	p := twoScenes()
	p.Scenes = append(p.Scenes, Scene{Key: "alley", Options: []Option{{Key: "leave"}}})
	refuse(t, p, "cannot be reached")
}

/*
A cycle is the one shape this framework cannot honour.

§5.1 caps a challenge at once ever and pays its deltas on completion, so a loop
is either a run that never finishes or a trail that can be extended without
bound — which is a hidden factor grant with a crank on it.
*/
func TestAPlanCannotGoRoundInACircle(t *testing.T) {
	p := twoScenes()
	p.Scenes[1].Options[1].Then = "stoop"
	refuse(t, p, "back to itself")
}

func TestASceneThatOffersNothingIsNotAnEnding(t *testing.T) {
	p := twoScenes()
	p.Scenes[1].Options = nil
	refuse(t, p, "offers nothing")
}

/*
§3.10's committed judgement — "the player selects the interpretation they will
act on from a set of authored positions" — never happens in a plan whose every
scene has one way out. A corridor is not a challenge.
*/
func TestAPlanWithNothingToChooseIsRefused(t *testing.T) {
	p := Plan{
		Opening: "stoop",
		Scenes: []Scene{
			{Key: "stoop", Options: []Option{{Key: "listen", Then: "kitchen"}}},
			{Key: "kitchen", Options: []Option{{Key: "leave"}}},
		},
		Buckets: []Bucket{{Key: "over", Reaction: "It is over."}},
	}
	refuse(t, p, "nothing is ever chosen")
}

/*
§3.5: Health and Money are read from the world, never granted. An authored delta
on either is an author expecting something that will never happen — and the
place to find that out is the build, not a player's total.
*/
func TestOnlyTheHiddenFactorsMayBeMoved(t *testing.T) {
	p := twoScenes()
	p.Scenes[0].Options[0].Deltas = []simulation.Grant{{Factor: simulation.Wealth, Credit: 5}}
	refuse(t, p, "read rather than granted")

	p = twoScenes()
	p.Scenes[0].Options[0].Deltas = []simulation.Grant{
		{Factor: simulation.Kindness, Credit: 3},
		{Factor: simulation.Kindness, Credit: -1},
	}
	refuse(t, p, "twice")
}

func TestKeysAreUniqueWhereTheyHaveToBe(t *testing.T) {
	p := twoScenes()
	p.Scenes[1].Key = "stoop"
	refuse(t, p, "two scenes")

	p = twoScenes()
	p.Scenes[0].Options[1].Key = "listen"
	refuse(t, p, "two options")

	p = twoScenes()
	p.Buckets[1].Key = "dismissed"
	refuse(t, p, "two outcomes")

	p = twoScenes()
	p.Buckets[1].AtLeast = p.Buckets[0].AtLeast
	refuse(t, p, "both start at")
}

/*
The two bounds, which are the checks worth having.

**Every trail lands in a bucket**: a player who made the worst run of choices in
the game still gets a reaction, rather than finishing a six-minute endeavour and
being told nothing. **Every bucket can be landed in**: reaction text nobody can
ever read is an author who believed the best outcome was attainable and was
wrong, which is worse than a typo because nothing ever says so.
*/
func TestEveryTrailLandsInAnOutcome(t *testing.T) {
	p := twoScenes()
	p.Buckets[0].AtLeast = 0 // the worst trail scores -1
	refuse(t, p, "a run can score")
}

func TestEveryOutcomeCanBeReached(t *testing.T) {
	p := twoScenes()
	p.Buckets[len(p.Buckets)-1].AtLeast = 99
	refuse(t, p, "no run can score more than")
}

func TestTheBoundsFollowTheBranching(t *testing.T) {
	p := twoScenes()
	worst, best := p.bounds()
	// walk-on ends it at -1; listen then side-with-her is 2 + 1.
	if worst != -1 || best != 3 {
		t.Fatalf("bounds() = (%d, %d), want (-1, 3)", worst, best)
	}
}

func TestAnOutcomeIsTheHighestBandTheScoreReaches(t *testing.T) {
	p := twoScenes()
	if err := p.check(); err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, tc := range []struct {
		score int
		want  string
	}{{-1, "dismissed"}, {0, "dismissed"}, {2, "heard"}, {3, "moved"}, {40, "moved"}} {
		got, ok := p.Outcome(tc.score)
		if !ok || got.Key != tc.want {
			t.Errorf("Outcome(%d) = %q, %v; want %q", tc.score, got.Key, ok, tc.want)
		}
	}
}

func TestAPlanWithNoOutcomesSaysNothing(t *testing.T) {
	p := twoScenes()
	p.Buckets = nil
	refuse(t, p, "no outcome buckets")

	p = twoScenes()
	p.Buckets[1].Reaction = "  "
	refuse(t, p, "has no reaction")
}
