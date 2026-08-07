package simulation

import (
	"testing"
	"time"
)

/*
The ladder, and the day a job leaves behind.

§17 lists "job tier thresholds — capability requirements per tier" as a thing to
settle during implementation, which is the argument for this file existing at
all: the tuning loop for a job is a table-driven test that runs in milliseconds,
not a fortnight of play. Every number in work.go is reachable from here.
*/

// nightShift is the bottom rung's post, read off the shipped ladder rather than
// written out — a tuning pass that changes the ladder has to change these tests
// or it has changed the game without saying so.
func rung(t *testing.T, tier Tier) Rung {
	t.Helper()
	r, ok := Rungs().At(tier)
	if !ok {
		t.Fatalf("there is no tier %d on the shipped ladder", tier)
	}
	return r
}

// day is a routine's day at a given arrangement, before any job touches it.
func day(sleep time.Duration, training Training, diet Diet, order Order) Day {
	return Day{Sleep: sleep, Training: training, Diet: diet, Order: order}
}

/*
§3.14: "you hold the job, it pays, and it costs you health **if your routines
are bad**."

The conditional is the mechanic, so it is the first thing asserted. The same
post, the same intended night, and the only difference is how the week was
arranged — and the difference that makes is most of a night's sleep, the
session, and the eating.
*/
func TestTheJobCostsWhatTheRoutinesCannotAbsorb(t *testing.T) {
	t.Parallel()

	post := rung(t, 1).Post
	// Eight and a half hours intended, and ten of them worked at night: the day
	// has 5h30m left in it against the six a routine needs, so half an hour of
	// room is short as well as the body clock.
	intended := 8*time.Hour + 30*time.Minute
	short := Spare - (Whole - intended - post.Hours)

	toll := func(held float64) time.Duration {
		return time.Duration(float64(NightToll)*held) + time.Duration(float64(short)*held)
	}

	cases := []struct {
		order Order
		sleep time.Duration
		train Training
		diet  Diet
	}{
		// Nothing arranged: the whole of it lands, and the session and the
		// eating go with it.
		{Unsaid, intended - toll(1), Light, Mixed},
		// Kept in the player's head: almost none of it is absorbed either.
		{Loose, intended - toll(0.85), Light, Mixed},
		// Written down before the week starts: the session and the eating
		// survive, and less of the night is taken.
		{Listed, intended - toll(0.60), Hard, Clean},
		{Regimented, intended - toll(0.40), Hard, Clean},
	}

	for _, c := range cases {
		got := Working(day(intended, Hard, Clean, c.order), post)
		if got.Sleep != c.sleep {
			t.Errorf("order %d: slept %v, want %v", c.order, got.Sleep, c.sleep)
		}
		if got.Training != c.train {
			t.Errorf("order %d: trained %d, want %d", c.order, got.Training, c.train)
		}
		if got.Diet != c.diet {
			t.Errorf("order %d: ate %d, want %d", c.order, got.Diet, c.diet)
		}
	}
}

/*
And the first step up is survivable, which is the other half of the claim and
the one that makes the ladder worth climbing.

A nine-hour day leaves the room a routine needs, so a player who has arranged
their week pays nothing for holding it — while a player who has not still loses
their eating to it. That is "it costs you health **if your routines are bad**"
read on the rung where the conditional is doing all of the work.
*/
func TestTheDeskJobIsFreeToAnArrangedLifeAndNotToAnyOther(t *testing.T) {
	t.Parallel()

	post := rung(t, 2).Post
	intended := 8*time.Hour + 30*time.Minute

	kept := day(intended, Hard, Clean, Listed)
	if got := Working(kept, post); got != kept {
		t.Errorf("an arranged week at the desk became %+v, want %+v", got, kept)
	}

	loose := day(intended, Hard, Clean, Loose)
	got := Working(loose, post)
	if got.Sleep != loose.Sleep {
		t.Errorf("the desk job took %v off the night", loose.Sleep-got.Sleep)
	}
	if got.Diet != worse(loose.Diet) {
		t.Errorf("ate %d, want %d — a shift you are not fed on is one you eat at", got.Diet, worse(loose.Diet))
	}
}

/*
And the body agrees, which is the claim that actually matters: the same routine
under the same job leaves two different people, and the arranged one is younger.

Read over a fortnight rather than a day, because a day of anything is noise —
the whole point of the health model is that a body has inertia.
*/
func TestTheSameJobAgesTwoRoutinesDifferently(t *testing.T) {
	t.Parallel()

	post := rung(t, 1).Post
	from := Date{Year: 2026, Month: time.March, Day: 2}

	loose := Routine{Sleep: 8*time.Hour + 30*time.Minute, Diet: Clean, Organization: Loose}
	listed := loose
	listed.Organization = Listed
	for i := range loose.Training {
		loose.Training[i], listed.Training[i] = Hard, Hard
	}

	body := Starting(1.78, 78)
	sloppy := Project(body, loose, post, from, 14)
	kept := Project(body, listed, post, from, 14)

	sloppyAge := BiologicalAge(sloppy[len(sloppy)-1].Body)
	keptAge := BiologicalAge(kept[len(kept)-1].Body)
	t.Logf("after a fortnight of nights: loose %.1f, listed %.1f", sloppyAge, keptAge)

	if sloppyAge <= keptAge {
		t.Errorf("the disorganised body reads %.1f and the arranged one %.1f, so the job cost nothing",
			sloppyAge, keptAge)
	}
	// And the job itself cost something, or "it costs you health" is a sentence
	// with no mechanic behind it.
	free := Project(body, listed, Post{}, from, 14)
	if freeAge := BiologicalAge(free[len(free)-1].Body); keptAge <= freeAge {
		t.Errorf("holding the job read %.1f and holding none read %.1f", keptAge, freeAge)
	}
}

// A player who holds no job lives exactly the day their routines produced. The
// zero post is the absence of a job rather than a job that asks for nothing.
func TestNoJobChangesNothingAboutTheDay(t *testing.T) {
	t.Parallel()

	for _, order := range []Order{Unsaid, Loose, Listed, Regimented} {
		d := day(6*time.Hour, Hard, Poor, order)
		if got := Working(d, Post{}); got != d {
			t.Errorf("order %d: an unemployed day became %+v, want %+v", order, got, d)
		}
	}
}

/*
The night is floored, because the arithmetic has to stay meaningful at the
bottom. A player who intends nothing and arranges nothing, on nights, does not
end up sleeping a negative number of hours.
*/
func TestTheNightHasAFloor(t *testing.T) {
	t.Parallel()

	got := Working(day(4*time.Hour, Rest, Poor, Unsaid), Post{Hours: 14 * time.Hour, Nights: true})
	if got.Sleep != ShortestNight {
		t.Errorf("slept %v, want the floor of %v", got.Sleep, ShortestNight)
	}
}

/*
§3.14: "the ladder itself teaches the trade: more money costs more of your
attention."

Asserted over the shipped ladder rather than over an example, so a tuning pass
that quietly makes a higher rung both richer *and* easier fails here. Pay rises
strictly the whole way up, and hours rise the whole way up **above the bottom
rung**.

The bottom rung is the exception on purpose and it is the more interesting half
of §3.14's table: the first step up is the one step that buys back an hour and a
whole night's worth of body clock, because the job at the bottom is worse than
the one above it in every way except that anybody can have it. That asymmetry is
what makes the first climb the one a player actually wants, and it is checked
below rather than left as prose.
*/
func TestEveryStepUpPaysMoreAndAsksMore(t *testing.T) {
	t.Parallel()

	ladder := Rungs()
	if len(ladder) < 5 {
		t.Fatalf("the ladder has %d rungs, and §3.14 tabulates five", len(ladder))
	}

	for i, r := range ladder {
		if r.Tier != Tier(i+1) {
			t.Fatalf("rung %d is tier %d, so the ladder is not in order", i, r.Tier)
		}
		if r.Pay <= 0 {
			t.Errorf("tier %d pays nothing", r.Tier)
		}
		if r.Post.Zero() {
			t.Errorf("tier %d asks nothing of the body, which is not a job", r.Tier)
		}
		if i == 0 {
			continue
		}
		below := ladder[i-1]
		if r.Pay <= below.Pay {
			t.Errorf("tier %d pays %d and tier %d pays %d", r.Tier, r.Pay, below.Tier, below.Pay)
		}
		if i > 1 && r.Post.Hours <= below.Post.Hours {
			t.Errorf("tier %d is %v and tier %d is %v, so the step up costs no more attention",
				r.Tier, r.Post.Hours, below.Tier, below.Post.Hours)
		}
	}
}

/*
The bottom rung is the only one worked against the body clock, and the first
step up is the only step that buys a night back.

The one asymmetry [TestEveryStepUpPaysMoreAndAsksMore] deliberately allows,
stated as its own claim so that it is a decision rather than a gap. Everything
above tier 2 goes back to costing more the more it pays — the ladder's shape is
one step out of the hole and then a long climb that gets steadily more
expensive.
*/
func TestOnlyTheBottomRungIsWorkedAtNight(t *testing.T) {
	t.Parallel()

	ladder := Rungs()
	ordinary := day(8*time.Hour, Hard, Clean, Loose)

	if !ladder[0].Post.Nights {
		t.Fatal("the bottom rung is not the night shift, and §3.14 says it is")
	}
	for _, r := range ladder[1:] {
		if r.Post.Nights {
			t.Errorf("tier %d is also worked at night, so the bottom rung is not the exception", r.Tier)
		}
	}

	nights, desk := Working(ordinary, ladder[0].Post), Working(ordinary, ladder[1].Post)
	if desk.Sleep <= nights.Sleep {
		t.Errorf("the first step up leaves %v of the night and the night shift leaves %v",
			desk.Sleep, nights.Sleep)
	}
}

/*
The bottom rung asks for nothing and every rung above it asks for more.

"Nothing" is the load-bearing half: §3.4's ladder starts somewhere a player with
no capability at all can stand, or "actions build capability, capability raises
tier" has nowhere to begin.
*/
func TestTheBottomRungTakesAnybodyAndTheOnesAboveItDoNot(t *testing.T) {
	t.Parallel()

	ladder := Rungs()
	nobody := Reading{}

	first := ladder[0]
	if len(first.Needs.Wants()) != 0 {
		t.Errorf("the bottom rung asks about %v", first.Needs.Wants())
	}
	if !first.Needs.Met(nobody) {
		t.Error("the bottom rung refuses somebody who has done nothing")
	}

	for i, r := range ladder[1:] {
		if len(r.Needs.Wants()) == 0 {
			t.Errorf("tier %d is a step up that asks for nothing", r.Tier)
		}
		if r.Needs.Met(nobody) {
			t.Errorf("tier %d would hire somebody who has done nothing", r.Tier)
		}
		if i == 0 {
			continue
		}
		below := ladder[i]
		for _, f := range Factors {
			if r.Needs.Of(f) < below.Needs.Of(f) {
				t.Errorf("tier %d asks %.0f of %s and tier %d asks %.0f",
					r.Tier, r.Needs.Of(f), f, below.Tier, below.Needs.Of(f))
			}
		}
	}
}

/*
§3.5's hidden factors are part of the asking, and §3.4 is why: "actions build
capability — skills, health, reputation, relationships — which raises job tier."

A ladder that only asked about health and money would be a ladder a player could
climb without ever meeting the three factors the game is actually measuring,
which would take the question out of the question.
*/
func TestTheLadderAsksAboutTheFactorsNobodyIsShown(t *testing.T) {
	t.Parallel()

	hidden := map[Factor]bool{}
	for _, r := range Rungs() {
		for _, f := range r.Needs.Wants() {
			if !f.Shown() {
				hidden[f] = true
			}
		}
	}
	for _, f := range Factors {
		if f.Shown() || hidden[f] {
			continue
		}
		t.Errorf("no rung of the ladder ever asks about %s", f)
	}
}

/*
The step up the first rung asks for, priced against what a challenge actually
pays — because a threshold nothing in the game can reach is a ladder with two
rungs and a wall.

§5.1's authored deltas are worth roughly 60–200 credit across a passage, and
[CreditHalf] is 240, so the arithmetic is stated here rather than believed.
*/
func TestTheFirstStepUpIsWithinReachOfAFewCommittedChoices(t *testing.T) {
	t.Parallel()

	second := rung(t, 2)
	reachable := Karma(Standing{
		Body:  Starting(1.78, 78),
		Purse: Dollars(1200),
		// Two challenges' worth of authored deltas, at the lower end of what
		// §5.1's passages pay.
		Credit: Credit{Intelligence: 70, Stability: 60},
	})
	if !second.Needs.Met(reachable) {
		t.Errorf("two challenges' worth of credit does not reach tier 2: %+v against %+v",
			reachable, second.Needs)
	}

	// And a body that has been wrecked by the night shift does not, which is
	// the trap §3.14 is describing: the job that pays you is the job that takes
	// the thing you need in order to leave it.
	wrecked := reachable
	wrecked.Health = second.Needs.Health - 1
	if second.Needs.Met(wrecked) {
		t.Error("a body below the floor is still hired")
	}
}

// Requirement is a floor per factor, and it is only ever asked about the
// factors it names. Zero is "no opinion", exactly as a zero ceiling is.
func TestARequirementOnlyAsksAboutWhatItNames(t *testing.T) {
	t.Parallel()

	need := Requirement{Health: 50, Kindness: 20}

	if want := []Factor{Health, Kindness}; len(need.Wants()) != len(want) {
		t.Fatalf("asks about %v, want %v", need.Wants(), want)
	}
	if need.Asks(Wealth) || need.Asks(Intelligence) || need.Asks(Stability) {
		t.Error("asks about a factor it names no floor for")
	}
	if !need.Met(Reading{Health: 50, Kindness: 20}) {
		t.Error("a reading exactly at the floor is refused")
	}
	if need.Met(Reading{Health: 50, Kindness: 19.9}) {
		t.Error("a reading below the floor is accepted")
	}
	if !(Requirement{}).Met(Reading{}) {
		t.Error("a requirement that asks nothing refuses somebody")
	}
}

// The tiers §3.14 calls passive, told from the ones it calls active.
func TestTheLadderKnowsWhichRungsAreStillPassive(t *testing.T) {
	t.Parallel()

	if Unemployed.Held() {
		t.Error("holding nothing counts as holding a job")
	}
	if Unemployed.Passive() {
		t.Error("holding nothing counts as a passive job")
	}
	for _, tier := range []Tier{1, 2} {
		if !Rungs()[tier-1].Tier.Passive() {
			t.Errorf("tier %d is not passive, and §3.14 says it is", tier)
		}
	}
	for _, tier := range []Tier{3, 4, 5} {
		if Rungs()[tier-1].Tier.Passive() {
			t.Errorf("tier %d is passive, and §3.14 says it demands real involvement", tier)
		}
	}
}

// Above is the step up, and it answers for somebody holding nothing — which is
// where every career starts.
func TestTheStepUpIsAnsweredFromTheBottom(t *testing.T) {
	t.Parallel()

	ladder := Rungs()
	next, ok := ladder.Above(Unemployed)
	if !ok || next.Tier != 1 {
		t.Fatalf("the step up from holding nothing is %+v, %v", next, ok)
	}
	if next, ok := ladder.Above(1); !ok || next.Tier != 2 {
		t.Fatalf("the step up from tier 1 is %+v, %v", next, ok)
	}
	if _, ok := ladder.Above(ladder[len(ladder)-1].Tier); ok {
		t.Error("there is a step up from the top of the ladder")
	}
	if !ladder.Post(Unemployed).Zero() {
		t.Error("holding no job has a post")
	}
}

// Income is structural, which in this package means one thing: a day's pay is a
// property of the tier and of nothing else. There is no argument to it that a
// player could push on.
func TestPayIsAPropertyOfTheTierAndNothingElse(t *testing.T) {
	t.Parallel()

	for _, r := range Rungs() {
		again, _ := Rungs().At(r.Tier)
		if again.Pay != r.Pay {
			t.Errorf("tier %d paid %d and then %d", r.Tier, r.Pay, again.Pay)
		}
	}
}
