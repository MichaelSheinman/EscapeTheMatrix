package conflict

import (
	"strings"
	"testing"
	"time"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/environment"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
Whose day it is, and why — with nothing running.

§4's sentence is "selected deterministically from relationship state", and it
has two halves that are worth separating: *from relationship state*, which is
what the pressure arithmetic below is, and *deterministically*, which is what
the last two cases are. Neither needs a database, a clock or a player, which is
the point of them being here rather than in the integration test.
*/

func on(day int) simulation.Date {
	return simulation.Date{Year: 2026, Month: time.March, Day: day}
}

// six is a population's worth of people, named for nothing in particular. The
// shipped ones are internal/environment's to test.
func six() []environment.Person {
	out := make([]environment.Person, 0, 6)
	for _, key := range []string{"one", "two", "three", "four", "five", "six"} {
		out = append(out, environment.Person{Key: key, Name: strings.ToUpper(key[:1]) + key[1:]})
	}
	return out
}

func TestADebtOutweighsACredit(t *testing.T) {
	// §3.15 lists them in an order and the weights are that order: what you owe
	// is yours to act on, and what you are owed is theirs.
	owing := Standing{Owed: -1, Last: on(9)}
	owed := Standing{Owed: 1, Last: on(9)}
	if owing.Pressure(on(10)) <= owed.Pressure(on(10)) {
		t.Errorf("owing %d is not more pressing than being owed %d",
			owing.Pressure(on(10)), owed.Pressure(on(10)))
	}
}

func TestTwoMildThingsCanOutweighOneSharpOne(t *testing.T) {
	// The reason pressure is a sum rather than a priority list: somebody you owe
	// a little and have also left something hanging with genuinely is more
	// overdue than somebody you have merely slighted, and a strict ordering
	// would never say so.
	both := Standing{Owed: -1, Unsettled: 2, Last: on(9)}
	one := Standing{Slighted: 1, Last: on(9)}
	if both.Pressure(on(10)) <= one.Pressure(on(10)) {
		t.Error("two outstanding things did not add up")
	}
}

func TestSomebodyNobodyHasSeenComesDue(t *testing.T) {
	// Without this the first debt would make one person the entire population
	// until it was paid, and §3.15's "characters must be consistent within an
	// environment" would be six people the player met once.
	fresh := Standing{Owed: -1, Met: 3, Last: on(10)}
	stale := Standing{Met: 3, Last: on(1)}
	if stale.Pressure(on(10)) <= fresh.Pressure(on(10)) {
		t.Errorf("a fortnight of silence (%d) did not outweigh a same-day debt (%d)",
			stale.Pressure(on(10)), fresh.Pressure(on(10)))
	}
	// And a stranger is fully due rather than not due at all, which is how
	// somebody who has just moved in meets everybody in their first week.
	if (Standing{}).Since(on(10)) != Stale {
		t.Error("somebody nobody has ever spoken to is at the back of the queue")
	}
}

func TestEverybodyIsMetInTheFirstWeek(t *testing.T) {
	// The property the staleness term exists for, stated as the thing a player
	// would notice: six strangers, six days, six different people.
	people := six()
	standings := map[string]Standing{}
	seen := map[string]bool{}
	for day := 1; day <= len(people); day++ {
		who, st, ok := Turn(people, standings, "account", on(day))
		if !ok {
			t.Fatal("nobody turned up")
		}
		seen[who.Key] = true
		standings[who.Key] = st.Counted(on(day))
	}
	if len(seen) != len(people) {
		t.Errorf("%d of %d people turned up in the first %d days", len(seen), len(people), len(people))
	}
}

func TestThePressedPersonIsTheOneWhoTurnsUp(t *testing.T) {
	people := six()
	standings := map[string]Standing{
		"four": {Person: "four", Owed: -3, Slighted: 2, Last: on(9)},
	}
	who, _, ok := Turn(people, standings, "account", on(10))
	if !ok || who.Key != "four" {
		t.Errorf("%q turned up; the person with everything outstanding is %q", who.Key, "four")
	}
}

func TestTheSameDayAlwaysProducesTheSamePerson(t *testing.T) {
	// §7: a rollover that is retried, replayed or run twice produces one world
	// rather than two.
	people, standings := six(), map[string]Standing{}
	first, _, _ := Turn(people, standings, "account", on(10))
	for i := 0; i < 20; i++ {
		again, _, _ := Turn(people, standings, "account", on(10))
		if again.Key != first.Key {
			t.Fatalf("the same day produced %q and then %q", first.Key, again.Key)
		}
	}
}

func TestTwoPlayersDoNotMeetTheSameNeighbourEveryMorning(t *testing.T) {
	// The account is in the hash for exactly this: a rotation by date alone
	// would hand every player in the game the same morning.
	people, standings := six(), map[string]Standing{}
	apart := 0
	for day := 1; day <= 14; day++ {
		mine, _, _ := Turn(people, standings, "one-account", on(day))
		theirs, _, _ := Turn(people, standings, "another-account", on(day))
		if mine.Key != theirs.Key {
			apart++
		}
	}
	if apart == 0 {
		t.Error("two players met the same person on all fourteen mornings")
	}
}

func TestUnderIsAlwaysAnswerableAndEndsWithTheOrdinary(t *testing.T) {
	for _, s := range []Standing{
		{},
		{Owed: -2},
		{Owed: 2, Slighted: 1, Unsettled: 1, Asked: Crowding},
	} {
		under := s.Under()
		if len(under) == 0 {
			t.Fatalf("%+v is under no pressure at all", s)
		}
		if under[len(under)-1] != Easy {
			t.Errorf("%+v ends on %q rather than on the ordinary friction of living near somebody",
				s, under[len(under)-1])
		}
	}
	// And the order is the design's: what you owe comes before what you are
	// owed, whatever else is true.
	both := Standing{Owed: -1}.Under()
	if both[0] != Owing {
		t.Errorf("a debt reads as %q first", both[0])
	}
}

func TestALedgerCannotRunAway(t *testing.T) {
	// §3.15's repetition has to read as the same people rather than as one
	// person becoming the whole world, which is how every relationship meter
	// ever built has failed.
	s := Standing{}
	for i := 0; i < 40; i++ {
		s = s.After(Shift{Owed: 1, Slighted: 1, Unsettled: 1, Asked: 1})
	}
	if s.Owed != Most || s.Slighted != Most || s.Unsettled != Most || s.Asked != Most {
		t.Errorf("%+v ran past the bounds", s)
	}
	for i := 0; i < 40; i++ {
		s = s.After(Shift{Owed: -1, Slighted: -1, Unsettled: -1, Asked: -1})
	}
	if s.Owed != -Most || s.Slighted != 0 || s.Unsettled != 0 || s.Asked != 0 {
		t.Errorf("%+v ran past the bounds the other way", s)
	}
}

func TestAMonthThatHasPassedStopsBeingCounted(t *testing.T) {
	// "They have asked me three times this month" is a rule a player composes,
	// so the count has to mean a calendar month — and it is reset when the
	// standing is next touched rather than by anything scheduled.
	march := Standing{Asked: 3, Month: on(1)}
	if march.Fresh(on(20)).Asked != 3 {
		t.Error("the same month's count was cleared")
	}
	april := march.Fresh(simulation.Date{Year: 2026, Month: time.April, Day: 2})
	if april.Asked != 0 {
		t.Errorf("a count of %d survived the month it belonged to", april.Asked)
	}
}

func TestHavingMetSomebodyIsNotMadeTrueByMeetingThem(t *testing.T) {
	// The ordering [Conflicts.Day] depends on: the ask counts before the rules
	// weigh the day, and having-dealt-with-them counts after — so "you have
	// dealt with them before" is never made true by the thing being dealt with.
	s := Standing{}
	asked := s.Fresh(on(10)).After(Shift{Asked: 1})
	if !asked.Stranger() {
		t.Error("the first conflict with somebody made them somebody you already knew")
	}
	if asked.Counted(on(10)).Stranger() {
		t.Error("having met them was not counted")
	}
}

func TestAStandingReadsAsSentencesRatherThanAsAScore(t *testing.T) {
	// §3.2: the game states what is so and never grades it. A number on this
	// surface would be a relationship meter, which is the one thing the design
	// spends its length refusing.
	reads := strings.Join(Standing{Owed: -1, Slighted: 2, Asked: 3, Met: 4}.Reads(), " | ")
	for _, want := range []string{"owe them one", "let them down", "asked you"} {
		if !strings.Contains(reads, want) {
			t.Errorf("%q says nothing about %q", reads, want)
		}
	}
	for _, forbidden := range []string{"%", "score", "level", "rating"} {
		if strings.Contains(reads, forbidden) {
			t.Errorf("%q reads as a score", reads)
		}
	}
	if len((Standing{Met: 2}).Reads()) != 0 {
		t.Error("somebody with nothing outstanding still has something outstanding")
	}
}
