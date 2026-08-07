package playbook

import (
	"sort"
	"strconv"
	"strings"
)

/*
The matcher: what the character does when the world puts a question and the
player is not there.

HighLevelOverview.md §3.14's loop begins "it happens without you", and this file
is the without-you part. A moment arises, the rules in force where the player is
standing are read, and one of them decides — deterministically, with no model
anywhere in the path (§7) and no dice: the same rule set and the same moment can
only produce the same answer, which is what makes a failed rule a thing the
player can be shown rather than a thing they have to take on trust.

# It is pure

Nothing in this file touches the database, the clock or the network. It takes a
rule set and a situation and returns a decision; the module writes the record and
the caller carries out the action. That is technical.md §3.1's functional core
applied one package down — internal/simulation cannot hold this, because it
imports nothing at all and the matcher is defined over an authored vocabulary,
so the rule is enforced here instead by TestTheMatcherIsPure.

The practical half of that: **[Decide] fires nothing.** It reports which rule
would act and what it would do, so "two rules both match and disagree" is
something the caller learns *before* either action is carried out, rather than
after one of them turned out to be the wrong half of a contradiction.

# A moment can put more than one question

Somebody raising their voice while asking you to work late puts `voice` and
`overtime` at once. There is exactly one rule per question per environment
(§3.14: no drafts, no alternatives), so two rules can only ever meet this way —
and when they do, [Stance] is what says whether they agree. Two answers with the
same posture are the same answer in two vocabularies; two with different postures
are a contradiction the character is not allowed to resolve on its own.

# Three ways a moment reaches the player

technical.md §4: "escalate on three conditions, not one — no rule matches, two
rules conflict, or a rule fired and the outcome was poor. Escalating only on 'no
match' lets one catch-all rule silence the game forever." The first two are
[Verdict] values below. The third is an [Outcome] on the record, because it is
not known when the rule fires.
*/

/*
Truth is what the matcher made of one condition, and it has three values rather
than two.

`Unsaid` is the one that matters. A moment that does not state whether the task
was legal has not made the condition false — it has failed to say — and a rule
resting on it cannot be applied. Treating silence as `no` would quietly turn
every unstated fact into a reason to fire the `otherwise`, which is the failure
mode where a player's rule looks wrong and the world was simply not asked.

The zero value is `Unsaid`, so a reading nobody filled in is the safe one.
*/
type Truth string

const (
	// Unsaid is a condition the moment did not speak to.
	Unsaid Truth = ""
	// Unmet is a condition the moment stated and that does not hold.
	Unmet Truth = "unmet"
	// Met is a condition the moment stated and that holds.
	Met Truth = "met"
)

/*
Fact is one thing the world states about a moment.

Addressed by the term's full id — `help/condition/fee` — the same name an unlock
is recorded against, because a fact and an unlock are both statements about one
word of one question's vocabulary and two naming schemes for that would be one
too many.

A term that takes a number is stated as a reading in that list's unit (cents,
minutes, hundredths of the player's rate); a term that takes none is stated as
true or false. The two are told apart by the vocabulary rather than by the fact,
and a fact that states the wrong one of them is a fault (see [Situation.Faults])
rather than something the matcher quietly reads past.
*/
type Fact struct {
	Term string
	// So is the truth of a term that takes no number.
	So bool
	// Amount is the reading of a term that takes one, in its list's unit.
	Amount int64
	// measured is which of the two above was actually stated. Unexported, so a
	// Fact can only be built by the two statements below and there is no zero
	// value that means "I said nothing but here is a number".
	measured bool
}

/*
Situation is one moment the world puts to a player's rules.

It is authored content in the end — technical.md §4: "daily conflicts and job
situations are authored pathways per environment, selected deterministically from
relationship state" — and #28 and #30 are the tickets that generate them. What it
carries is a key naming the moment, the questions the moment puts, and everything
that is true of it.

A question with no facts stated is still worth putting: it is how a moment says
"this came up and nothing about it was legible", and the rule for it comes out
undecided rather than absent.
*/
type Situation struct {
	key   string
	puts  []string
	facts map[string]Fact
}

// Arises is a moment, naming itself and the questions it puts. Question keys
// that the vocabulary does not have are ignored — the situation catalogue is
// content and outlives any one question, so a moment written for a question that
// was later withdrawn simply stops putting it.
func Arises(key string, puts ...string) Situation {
	return Situation{key: key, puts: append([]string(nil), puts...), facts: map[string]Fact{}}
}

// Key is what this moment is called, and is carried into the record so that a
// pattern across days is a query rather than a reading exercise.
func (s Situation) Key() string { return s.key }

// Puts is the questions this moment asks, as given.
func (s Situation) Puts() []string { return append([]string(nil), s.puts...) }

// So states a term that takes no number, true or false. Stating it false is not
// the same as leaving it out, and both are things a moment may mean.
func (s Situation) So(term string, so bool) Situation {
	return s.with(Fact{Term: term, So: so})
}

// Reading states a term that takes a number, in its value list's unit.
func (s Situation) Reading(term string, amount int64) Situation {
	return s.with(Fact{Term: term, Amount: amount, measured: true})
}

// with returns a copy carrying one more fact. Situations are values: a moment
// half-built is not a thing a caller should be able to hold a reference to.
func (s Situation) with(f Fact) Situation {
	facts := make(map[string]Fact, len(s.facts)+1)
	for k, v := range s.facts {
		facts[k] = v
	}
	facts[f.Term] = f
	return Situation{key: s.key, puts: s.puts, facts: facts}
}

/*
Faults is every fact this vocabulary cannot read: a term that is not in it, and a
term stated as the wrong sort of thing.

The matcher itself is total — an unreadable fact is simply a condition nobody
spoke to, so the rule comes out undecided and the moment escalates, which is the
safe direction. This is how the caller finds out anyway, and [Playbooks.Weigh]
refuses a situation with any, so an authoring mistake surfaces as a refusal
rather than as a game that quietly stopped applying somebody's rules.
*/
func (s Situation) Faults() []string {
	out := []string{}
	for id, f := range s.facts {
		t, ok := vocabulary.Term(id)
		switch {
		case !ok, t.Kind != Condition:
			out = append(out, id)
		case t.Takes() != f.measured:
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// read is what the moment says about one condition of one question.
func (s Situation) read(t Term) Truth {
	f, ok := s.facts[t.ID()]
	if !ok || t.Takes() != f.measured {
		return Unsaid
	}
	if !t.Takes() {
		if f.So {
			return Met
		}
		return Unmet
	}
	return Unsaid
}

// measure is what the moment reads on a term that takes a number.
func (s Situation) measure(t Term) (int64, bool) {
	f, ok := s.facts[t.ID()]
	if !ok || !f.measured || !t.Takes() {
		return 0, false
	}
	return f.Amount, true
}

// puts reports whether this moment asks one particular question.
func (s Situation) asks(question string) bool {
	for _, q := range s.puts {
		if q == question {
			return true
		}
	}
	return false
}

/*
State is what became of one question the moment put.

`Unanswered` and `Undecided` are kept apart deliberately, because they are two
different things to say to a player: one is "you have no rule for this" and the
other is "you have one and this moment did not say enough to apply it". §3.6's
diagnosis is the player pointing at their own accumulated evidence, and evidence
that collapsed those two would point at the wrong repair.
*/
type State string

const (
	// Unanswered is a question with no complete rule covering it.
	Unanswered State = "unanswered"
	// Undecided is a rule whose conditions the moment did not settle.
	Undecided State = "undecided"
	// Weighed is a rule that reached an action and was not the one acted on.
	Weighed State = "weighed"
	// Fired is the rule the character acted on.
	Fired State = "fired"
)

/*
Consideration is one question the moment put, and everything the matcher made of
the rule for it.

This is the attribution the whole design of §3.14 rests on: `Met` is per
condition, in the order the sentence tests them, so "which condition matched" is
recorded rather than reconstructed. A structured rule makes failure legible, and
a record that only kept the verdict would have thrown the legibility away on the
way to the table.
*/
type Consideration struct {
	Question string
	// N is the question's authored number, which is how a log line cites an
	// answer ("answer 1 fired").
	N     int
	State State
	// Rule is the sentence as it stood at the moment, so that editing it
	// afterwards does not rewrite what happened.
	Rule Sentence
	// Met is one reading per condition, in the sentence's own order.
	Met []Truth
	// Held is the whole `when`, combined by the join.
	Held Truth
	// Action is what the rule called for — `then` when the conditions held,
	// `otherwise` when they did not, and blank when it could not be applied.
	Action Ref
	Stance Stance
	// Says is the rule as a sentence, and Did is the action alone. Rendered here
	// because a log line is the sentence being reported rather than composed,
	// and the client has no business assembling one out of parts.
	Says string
	Did  string
}

// Applied reports whether this rule reached an action.
func (c Consideration) Applied() bool { return c.State == Weighed || c.State == Fired }

/*
Verdict is what the playbook did with the moment as a whole.
*/
type Verdict string

const (
	// Answered is exactly one posture among the rules that applied. The
	// character acts, and the player finds out by reading back.
	Answered Verdict = "answered"
	// Silent is nothing in force covering the moment. It escalates.
	Silent Verdict = "silent"
	// Clash is two rules that both applied and disagree about what to do. It
	// escalates, and the character does nothing — picking one would be the game
	// making the judgement the game exists to teach.
	Clash Verdict = "clash"
)

// Decision is the whole of what the matcher decided, and it is a value: nothing
// has happened yet.
type Decision struct {
	// Situation is the key of the moment that was put.
	Situation string
	Verdict   Verdict
	// Considered is every question the moment put, in the order the questions
	// are asked. That order is authored, which is what makes "the first one
	// checked wins" something a player can be shown.
	Considered []Consideration
	// fired indexes Considered, or -1.
	fired int
}

// Fired is the rule the character acted on, if any.
func (d Decision) Fired() (Consideration, bool) {
	if d.fired < 0 || d.fired >= len(d.Considered) {
		return Consideration{}, false
	}
	return d.Considered[d.fired], true
}

// Escalates reports whether this moment goes to the player as a live decision
// (§3.14: "anything the rules don't cover escalates"). Two of technical.md §4's
// three escalation conditions are here; the third is a poor outcome, which is
// not known yet.
func (d Decision) Escalates() bool { return d.Verdict != Answered }

/*
Note is why the moment escalated, in the words the log uses.

Empty when it did not. The clash line names the two answers by their authored
numbers and by what they amounted to rather than by the actions' own words,
because the point being made is that two different questions got the same kind of
answer — and a player reading "you said to accept it and you said to say no"
would have to work out for themselves that those were both answers to one moment.
*/
func (d Decision) Note() string {
	switch d.Verdict {
	case Silent:
		return "nothing in your playbook covered this."
	case Clash:
		var said []string
		for _, c := range d.Considered {
			if c.Applied() {
				said = append(said, "answer "+strconv.Itoa(c.N)+" said to "+c.Stance.Reads())
			}
		}
		return strings.Join(said, "; ") + "."
	default:
		return ""
	}
}

/*
Decide is the matcher.

`rules` is what [Playbooks.InForce] hands out: the complete rules for one
environment, in the order the questions are asked. Incomplete ones are not here
at all, which is why nothing below has to decide what a sentence with a hole in
it counts as.

The walk is over the vocabulary rather than over `rules`, so the authored order
is the one that decides precedence even if a caller hands the rules over shuffled
— and so a question the moment puts with no rule for it is *seen* rather than
skipped, which is the difference between "you have no answer to this" and
silence.
*/
func Decide(rules []Rule, s Situation) Decision {
	held := map[string]Rule{}
	for _, r := range rules {
		if r.Complete() {
			held[r.Question] = r
		}
	}

	out := Decision{Situation: s.key, Considered: []Consideration{}, fired: -1}
	for _, q := range vocabulary.Questions() {
		if !s.asks(q.Key) {
			continue
		}
		r, ok := held[q.Key]
		if !ok {
			out.Considered = append(out.Considered, Consideration{
				Question: q.Key, N: q.Order, State: Unanswered, Held: Unsaid, Met: []Truth{},
			})
			continue
		}
		out.Considered = append(out.Considered, weigh(q, r, s))
	}

	// One posture among everything that applied, or the character is not
	// allowed to act. Counting distinct postures rather than distinct actions
	// is the whole reason a stance exists: "accept it" and "say yes" are one
	// answer given to two questions, and escalating that would make every
	// well-covered moment a decision the player has to make by hand.
	seen := map[Stance]bool{}
	first := -1
	for i, c := range out.Considered {
		if !c.Applied() {
			continue
		}
		if first < 0 {
			first = i
		}
		seen[c.Stance] = true
	}
	switch {
	case first < 0:
		out.Verdict = Silent
	case len(seen) > 1:
		out.Verdict = Clash
	default:
		out.Verdict = Answered
		out.fired = first
		out.Considered[first].State = Fired
	}
	return out
}

// weigh runs one rule against the moment.
func weigh(q Question, r Rule, s Situation) Consideration {
	out := Consideration{
		Question: q.Key, N: q.Order, Rule: r.Sentence,
		Met:  make([]Truth, 0, len(r.When)),
		Says: q.Read(r.Sentence),
	}
	for _, w := range r.When {
		out.Met = append(out.Met, test(q, w, s))
	}
	out.Held = combine(r.Join, out.Met)

	action := r.Then
	if out.Held == Unmet {
		action = r.Else
	}
	if out.Held == Unsaid {
		out.State = Undecided
		return out
	}

	t, ok := q.Term(Action, action.Term)
	if !ok {
		// A term withdrawn from content between the rule being stored and this
		// moment. InForce already re-resolves against the vocabulary, so this
		// is unreachable through the module — but Decide is exported and pure,
		// and a matcher that panicked on stale input would be the wrong kind of
		// strict for the one path that runs while nobody is watching.
		out.State = Undecided
		out.Held = Unsaid
		return out
	}
	out.State = Weighed
	out.Action = action
	out.Stance = t.Stance
	out.Did = q.Reads(Action, action)
	return out
}

// test is one condition against the moment.
func test(q Question, r Ref, s Situation) Truth {
	t, ok := q.Term(Condition, r.Term)
	if !ok {
		return Unsaid
	}
	if !t.Takes() {
		return s.read(t)
	}
	option, ok := t.Values.Option(r.Value)
	if !ok {
		return Unsaid
	}
	reading, ok := s.measure(t)
	if !ok {
		return Unsaid
	}
	// Which way the comparison runs belongs to the value list rather than to
	// the term, because the word is already in the sentence: a list whose
	// options all begin "above" is a floor however it is used.
	switch t.Values.Compare {
	case AtLeast:
		return truth(reading >= option.Amount)
	case AtMost:
		return truth(reading <= option.Amount)
	default:
		// Quantity is an amount an action hands over, and the vocabulary
		// refuses one on a condition at parse time. Reaching here would mean
		// content and code disagreed, and the safe answer is that nothing was
		// tested.
		return Unsaid
	}
}

func truth(b bool) Truth {
	if b {
		return Met
	}
	return Unmet
}

/*
combine is the join, over three-valued readings.

Kleene's logic, and the two clauses that make it worth spelling out are the ones
where silence does not spread: `and` is `Unmet` the moment any condition is
unmet, however much of the rest went unsaid, and `or` is `Met` the moment any is
met. A rule that can be settled on what the moment *did* say is settled, and only
a rule that genuinely turns on something nobody stated comes out undecided.
*/
func combine(j Join, met []Truth) Truth {
	if len(met) == 0 {
		return Unsaid
	}
	unsaid := false
	for _, m := range met {
		switch {
		case m == Unsaid:
			unsaid = true
		case j == Or && m == Met:
			return Met
		case j != Or && m == Unmet:
			return Unmet
		}
	}
	if unsaid {
		return Unsaid
	}
	if j == Or {
		return Unmet
	}
	return Met
}

/* ── what can be said without a moment ─────────────────────────────────── */

/*
Conflict is two rules that could disagree about a moment putting both their
questions.

technical.md §6 lists this as one of the four reasons a composed rule beats a
written one: "rule conflicts are statically detectable — these two both match and
disagree is good UI, and impossible with prose." It is static in the strong
sense — no moment is needed, only the rule set and the shape of a moment the
world is able to produce.

`Always` is the pair that disagrees whichever way each rule falls. That is the
one worth a player's attention: a `Sometimes` conflict is a moment they have not
thought about yet, and an `Always` one is a moment they have answered twice.
*/
type Conflict struct {
	// Questions are the two, in authored order.
	Questions [2]string
	N         [2]int
	Always    bool
}

/*
Audit is everything that can be said about a rule set with no moment in hand.

It is the review surface §3.14's loop step 2 needs before anything has gone
wrong: a rule that settles nothing and a pair that will contradict each other are
both things the player can be shown the day they compose them, rather than a week
later in a log.
*/
type Audit struct {
	// Idle is the questions whose rule calls for the same posture either way.
	// Such a rule is in force and does something; what it does not do is depend
	// on its own conditions, which is almost never what the player meant.
	Idle []string
	// Conflicts is the pairs that could disagree, in authored order.
	Conflicts []Conflict
}

/*
Foresee audits a rule set against the questions a moment would put.

`puts` is the shape of a situation rather than a situation: #30's authored
conflicts declare which questions they raise, so this can be answered about a
whole catalogue before any of it happens. With no questions given, every question
the rule set covers is considered, which is the widest honest reading of "could
these ever meet".
*/
func Foresee(rules []Rule, puts ...string) Audit {
	held := map[string]Rule{}
	for _, r := range rules {
		if r.Complete() {
			held[r.Question] = r
		}
	}

	type reachable struct {
		question string
		n        int
		stances  []Stance
	}
	var live []reachable
	out := Audit{Idle: []string{}, Conflicts: []Conflict{}}

	for _, q := range vocabulary.Questions() {
		r, ok := held[q.Key]
		if !ok || (len(puts) > 0 && !contains(puts, q.Key)) {
			continue
		}
		then, ok1 := q.Term(Action, r.Then.Term)
		other, ok2 := q.Term(Action, r.Else.Term)
		if !ok1 || !ok2 {
			continue
		}
		if then.Stance == other.Stance {
			out.Idle = append(out.Idle, q.Key)
			live = append(live, reachable{q.Key, q.Order, []Stance{then.Stance}})
			continue
		}
		live = append(live, reachable{q.Key, q.Order, []Stance{then.Stance, other.Stance}})
	}

	for i := 0; i < len(live); i++ {
		for j := i + 1; j < len(live); j++ {
			a, b := live[i], live[j]
			some, all := false, true
			for _, x := range a.stances {
				for _, y := range b.stances {
					if x == y {
						all = false
					} else {
						some = true
					}
				}
			}
			if !some {
				continue
			}
			out.Conflicts = append(out.Conflicts, Conflict{
				Questions: [2]string{a.question, b.question},
				N:         [2]int{a.n, b.n},
				Always:    all,
			})
		}
	}
	return out
}

/*
Audit is what one rule set can be told about itself with no moment in hand.

Every question the set covers, which is the widest honest reading of "could these
two ever meet". Until a catalogue of authored moments exists (#28, #30) the
world's answer to "which questions arrive together" is "any of them" — and a
review surface that showed nothing until that catalogue landed would be
withholding the one reading the player can have on the day they compose, which is
the day it is worth having.
*/
func (s Set) Audit() Audit { return Foresee(s.Rules) }

/* ── reading a rule back ───────────────────────────────────────────────── */

/*
Read renders a whole sentence, for a log line.

Rendered on the server rather than assembled by a client, and for the same reason
the amounts stay here: a screen that put a rule back together out of parts would
be a second implementation of what a rule means, and the first time the two
disagreed the player would be reading something the matcher never ran.

A part whose term is no longer in the vocabulary reads as the bare key rather
than disappearing. A record is a statement about what happened, and a log line
that quietly dropped a word would be a smaller lie than a wrong one but a lie all
the same.
*/
func (q Question) Read(s Sentence) string {
	var b strings.Builder
	b.WriteString("when ")
	for i, w := range s.When {
		if i > 0 {
			b.WriteString(" " + string(s.Join) + " ")
		}
		b.WriteString(q.Reads(Condition, w))
	}
	b.WriteString(" then " + q.Reads(Action, s.Then))
	b.WriteString(" otherwise " + q.Reads(Action, s.Else))
	return b.String()
}

// Reads renders one part of a sentence, with its number written in where the
// placeholder is.
func (q Question) Reads(kind Kind, r Ref) string {
	if r.Blank() {
		return "…"
	}
	t, ok := q.Term(kind, r.Term)
	if !ok {
		return r.Term
	}
	if !t.Takes() {
		return t.Says
	}
	option, ok := t.Values.Option(r.Value)
	if !ok {
		return strings.ReplaceAll(t.Says, Placeholder, "…")
	}
	return strings.ReplaceAll(t.Says, Placeholder, option.Says)
}

/* ── small things ─────────────────────────────────────────────────────── */

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
