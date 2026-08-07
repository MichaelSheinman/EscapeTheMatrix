package conflict

import (
	"fmt"
	"hash/fnv"
	"sort"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/environment"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
What stands between the player and one person, and what follows from it.

HighLevelOverview.md §3.15 names the inputs and this file is that sentence,
typed: "each day the server generates friction that **follows from actual
state** — who owes whom, who was slighted, what was left unresolved, who has
been asked too often." Everything below is arithmetic over four counts, because
technical.md §7 forbids a model anywhere in this path and because a conflict a
player cannot trace back to something they did is a conflict that teaches
nothing (§3.6).

There is deliberately no affinity, no relationship level and no score. Those
would be the same mistake §3.2 spends its length refusing: a number that goes up
is a ladder, and a ladder is the thing this game is an argument against. What is
here is a ledger of things that happened.
*/

/*
Strain is a kind of pressure standing between the player and one person.

The closed set is §3.15's own list, plus [Easy] for the ordinary friction of
living near somebody when nothing at all is outstanding — which has to exist,
because a population where nothing happens until you owe somebody would be a
population that goes quiet exactly when a player is doing well.

A strain is what an authored pathway declares it follows from, and it is what a
conflict row records. It is *not* shown to the player as a word: what they read
is the moment, and working out that the fourth ask in a fortnight went badly
because it was the fourth is the diagnosis (§3.6) rather than the label.
*/
type Strain string

const (
	// Owing is what you have not paid back.
	Owing Strain = "owing"
	// Slighted is somebody you let down and did not put right.
	Slighted Strain = "slighted"
	// Unsettled is something left hanging between you.
	Unsettled Strain = "unsettled"
	// Owed is what they have not paid back.
	Owed Strain = "owed"
	// Asking is somebody who has been coming to you too often.
	Asking Strain = "asking"
	// Easy is nothing outstanding at all, which is most days with most people.
	Easy Strain = "easy"
)

/*
Strains is the closed set, in the order a person's pressures are read.

Strongest first, and the order is an argument rather than a list. What you owe
outranks what you are owed because a debt is yours to act on and a credit is
theirs; a slight outranks both because it is the only one of the four that is
about the other person's account of you; and being asked too often comes last of
the real ones because it is the mildest thing that is still a reason for
somebody to turn up.

[Easy] is last and always applies, which is what makes the walk in [Under]
terminate with an answer rather than with nothing.
*/
var Strains = []Strain{Owing, Slighted, Unsettled, Owed, Asking, Easy}

// Known reports whether a strain is one of the closed set.
func Known(s Strain) bool {
	for _, k := range Strains {
		if k == s {
			return true
		}
	}
	return false
}

/*
The bounds and thresholds the whole mechanic is tuned by.

Constants rather than authored content, because they are the shape of the
arithmetic rather than the writing: an author decides what a conflict says and
what it does to a standing, and how much pressure a debt is worth is a property
of the system that would be meaningless to state per file.
*/
const (
	// Most is how far any one count may run. §3.15 wants repetition to read as
	// "the same kind of person doing the same kind of thing"; an unbounded count
	// would instead make one person permanently the most pressing fact in the
	// world, which is how every relationship meter ever built fails.
	Most = 9
	// Crowding is how many asks in a month is too many. Three, because the
	// playbook's own vocabulary tops out at "they have asked me three times this
	// month" — the threshold a player can compose a rule about is the threshold
	// the world should mean by it.
	Crowding = 3
	// Stale is how many days without anything counts as fully due. A week is
	// §3.15's own unit for a pattern, and it is the point past which somebody
	// you live beside not turning up would be conspicuous.
	Stale = 7
)

/*
Standing is the whole of what is between the player and one person.

The zero value is a stranger, and it is a legitimate and common state: a
population is six people and somebody who has just moved in owes none of them
anything. Nothing has to be created for a person before they can be in a
conflict.
*/
type Standing struct {
	// Person is the key `content/environments` authors them under.
	Person string
	// Owed is the favour ledger, signed: positive is what they owe you, negative
	// is what you owe them. One column because it is one fact — "who owes whom"
	// has one answer at a time.
	Owed int
	// Slighted is how many times you have let them down without putting it
	// right.
	Slighted int
	// Unsettled is how many things have been left hanging between you.
	Unsettled int
	// Asked is how many times they have come to you this month, and Month is the
	// month that count belongs to.
	Asked int
	Month simulation.Date
	// Met is how many conflicts you have had with them at all, and Last is the
	// day of the most recent. Both keep a population from collapsing into one
	// person.
	Met  int
	Last simulation.Date
}

// Owing reports whether the player is the one in debt.
func (s Standing) Owing() int {
	if s.Owed < 0 {
		return -s.Owed
	}
	return 0
}

// Credit is what they owe the player.
func (s Standing) Credit() int {
	if s.Owed > 0 {
		return s.Owed
	}
	return 0
}

// Crowded reports whether they have been asking too often this month.
func (s Standing) Crowded() bool { return s.Asked >= Crowding }

// Stranger reports whether anything has ever passed between them.
func (s Standing) Stranger() bool { return s.Met == 0 }

/*
Under is every pressure this standing is under, strongest first.

Always non-empty and always ending in [Easy]: a person with nothing outstanding
is still somebody you live beside, and the day still has to be able to produce
something with them. That is not a fallback for missing content — it is the
majority case, and the pool is authored accordingly.
*/
func (s Standing) Under() []Strain {
	out := make([]Strain, 0, len(Strains))
	for _, strain := range Strains {
		switch strain {
		case Owing:
			if s.Owing() > 0 {
				out = append(out, strain)
			}
		case Slighted:
			if s.Slighted > 0 {
				out = append(out, strain)
			}
		case Unsettled:
			if s.Unsettled > 0 {
				out = append(out, strain)
			}
		case Owed:
			if s.Credit() > 0 {
				out = append(out, strain)
			}
		case Asking:
			if s.Crowded() {
				out = append(out, strain)
			}
		case Easy:
			out = append(out, strain)
		}
	}
	return out
}

/*
Pressure is how much this person is owed a day, as a number.

The weights are the same argument [Strains] makes, and the reason they are a sum
rather than a priority list is that two mild things genuinely do add up to a
pressing one: somebody you owe a little and have also left something hanging
with is more overdue than either on its own, and a strict ordering would never
say so.

The last term is the one that is not about the relationship at all. Days since
anything is what keeps the population a population — without it a single debt
would make one person the entire environment until it was paid, and §3.15's
"characters must be consistent within an environment" would be six people the
player met once. A stranger counts as fully due, which is how somebody who has
just moved in meets everybody in their first week.
*/
func (s Standing) Pressure(on simulation.Date) int {
	out := 5*s.Owing() + 4*s.Slighted + 3*s.Unsettled + 2*s.Credit()
	if s.Crowded() {
		out += 2 * (s.Asked - Crowding + 1)
	}
	return out + s.Since(on)
}

/*
Since is how many days it has been, capped at [Stale].

A stranger is [Stale] — fully due — rather than zero. The alternative reads as
"nothing has happened with them for no days", which would put the person nobody
has ever spoken to at the very back of the queue forever.
*/
func (s Standing) Since(on simulation.Date) int {
	if s.Stranger() || s.Last == (simulation.Date{}) {
		return Stale
	}
	days := s.Last.Until(on)
	switch {
	case days < 0:
		// A standing stamped with a day in the future. Reachable only from a
		// clock that moved backwards, and read as "today" rather than as a
		// negative debt of attention.
		return 0
	case days > Stale:
		return Stale
	}
	return days
}

/*
Turn is whose day it is: the person under the most pressure, and their standing.

Deterministic, and this is the whole of §4's "selected deterministically from
relationship state". The pressure decides it; the hash only breaks a tie, which
on the first day of a new life is every tie there is — and a tie broken by the
account and the date is the honest answer to "who turns up when nothing has
happened yet", because who you run into first genuinely is arbitrary.

Authored order is the last word, so two people the hash agrees on come out in
the order the population file lists them and never in map order (§7).
*/
func Turn(
	people []environment.Person, standings map[string]Standing,
	accountID string, on simulation.Date,
) (environment.Person, Standing, bool) {
	if len(people) == 0 {
		return environment.Person{}, Standing{}, false
	}

	type candidate struct {
		who      environment.Person
		standing Standing
		pressure int
		scatter  uint32
		order    int
	}
	all := make([]candidate, 0, len(people))
	for i, p := range people {
		st, held := standings[p.Key]
		if !held {
			st = Standing{Person: p.Key}
		}
		st.Person = p.Key
		all = append(all, candidate{
			who: p, standing: st, pressure: st.Pressure(on),
			scatter: shuffle(accountID, on, p.Key), order: i,
		})
	}
	sort.SliceStable(all, func(i, j int) bool {
		switch {
		case all[i].pressure != all[j].pressure:
			return all[i].pressure > all[j].pressure
		case all[i].scatter != all[j].scatter:
			return all[i].scatter < all[j].scatter
		default:
			return all[i].order < all[j].order
		}
	})
	return all[0].who, all[0].standing, true
}

/*
Shift is what one answer does to what stands between two people.

Signed, because every one of these moves both ways: paying somebody back and
borrowing from them are the same column, and an answer that puts a slight right
is as authorable as one that causes it. Applied through [Standing.After], which
is where the bounds live.
*/
type Shift struct {
	Owed      int
	Slighted  int
	Unsettled int
	Asked     int
}

// Moves reports whether this shift changes anything.
func (s Shift) Moves() bool { return s != Shift{} }

/*
After is this standing with a shift applied, clamped.

Clamped rather than refused, because the bound is a property of the world rather
than of the authoring: an answer that adds a slight is correct on a person who
already has nine, and the right behaviour is that it stays at nine rather than
that the day fails.
*/
func (s Standing) After(shift Shift) Standing {
	out := s
	out.Owed = clamp(s.Owed+shift.Owed, -Most, Most)
	out.Slighted = clamp(s.Slighted+shift.Slighted, 0, Most)
	out.Unsettled = clamp(s.Unsettled+shift.Unsettled, 0, Most)
	out.Asked = clamp(s.Asked+shift.Asked, 0, Most)
	return out
}

/*
Fresh is this standing with a count of asks that belongs to a past month
cleared.

The playbook asks "they have asked me {value} this month", so the count has to
mean a calendar month — and a month that rolled over since the last ask is a
count that starts again rather than one that decays. It is reset here, at the
moment the standing is next touched, rather than by anything scheduled: a person
nobody has seen since March should not need a nightly job to stop being somebody
who asked three times in March.
*/
func (s Standing) Fresh(on simulation.Date) Standing {
	out := s
	if out.Month.Year != on.Year || out.Month.Month != on.Month {
		out.Asked, out.Month = 0, simulation.Date{Year: on.Year, Month: on.Month, Day: 1}
	}
	return out
}

/*
Counted records that a conflict happened with them today.

Separate from [Standing.Fresh] and from [Standing.After] because it happens at a
different moment: the ask counts *before* the rules weigh the day, so that "they
have asked me three times this month" includes the asking that is going on right
now, while having-met-them counts *after*, so that "you have dealt with them
before" is not made true by the thing being dealt with.
*/
func (s Standing) Counted(on simulation.Date) Standing {
	out := s
	out.Met++
	out.Last = on
	return out
}

/*
Reads is what stands between you and somebody, in the words a player is shown.

Composed on the server for the same reason a rule's sentence is (§10.2): these
are the game's own account of its own state, and a client assembling them from
four integers would be a client that had to be told what the integers mean —
which is the same thing as shipping the mechanic to the browser.

Deliberately not a verdict and deliberately not a number. "You owe her one" is a
fact about a favour; "relationship: 62%" is a score, and §3.2 does not permit
one.
*/
func (s Standing) Reads() []string {
	out := []string{}
	switch {
	case s.Owing() == 1:
		out = append(out, "you owe them one")
	case s.Owing() > 1:
		out = append(out, fmt.Sprintf("you owe them %d", s.Owing()))
	case s.Credit() == 1:
		out = append(out, "they owe you one")
	case s.Credit() > 1:
		out = append(out, fmt.Sprintf("they owe you %d", s.Credit()))
	}
	if s.Slighted == 1 {
		out = append(out, "you let them down once and it was not put right")
	} else if s.Slighted > 1 {
		out = append(out, fmt.Sprintf("you have let them down %d times", s.Slighted))
	}
	if s.Unsettled == 1 {
		out = append(out, "something between you was left hanging")
	} else if s.Unsettled > 1 {
		out = append(out, fmt.Sprintf("%d things between you were left hanging", s.Unsettled))
	}
	if s.Asked == 1 {
		out = append(out, "they have asked you for something this month")
	} else if s.Asked > 1 {
		out = append(out, fmt.Sprintf("they have asked you %d times this month", s.Asked))
	}
	if len(out) == 0 && s.Stranger() {
		out = append(out, "you have never had anything to settle with them")
	}
	return out
}

/* ── determinism ──────────────────────────────────────────────────────── */

/*
shuffle is the tie-break: a hash of who is playing, which day it is, and who
they might run into.

Deterministic and not random (§7). A rollover that is retried, replayed or run
twice produces one world rather than two, and the same account on the same day
always meets the same person — which is what makes a conflict a thing that
happened rather than a roll nobody can check.

The account is in it so that every player in the game does not meet the same
neighbour on the same morning, which a rotation by date alone would do.
*/
func shuffle(accountID string, on simulation.Date, key string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(accountID))
	_, _ = fmt.Fprintf(h, "|%04d-%02d-%02d|", on.Year, int(on.Month), on.Day)
	_, _ = h.Write([]byte(key))
	return scatter(h.Sum32())
}

/*
scatter mixes a hash so that its low bits are worth taking a remainder of.

The same finalizer internal/situation uses, and here for the same reason: FNV's
low bits are a bijection of the low bits of the state before them, so two
accounts whose hashes agree in the bottom two bits agree in them forever. That
is not a weak tail, it is two players handed the same year. The standard 32-bit
avalanche keeps "the same input gives the same answer" while making one bit of
input move half the output.
*/
func scatter(x uint32) uint32 {
	x ^= x >> 16
	x *= 0x7feb352d
	x ^= x >> 15
	x *= 0x846ca68b
	x ^= x >> 16
	return x
}

func clamp(n, low, high int) int {
	switch {
	case n < low:
		return low
	case n > high:
		return high
	}
	return n
}
