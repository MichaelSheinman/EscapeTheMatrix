package playbook

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

/*
The matcher, with no database anywhere near it.

That is the point of the file as much as its contents. #19 asks for matching
logic that "lives in the pure core and is unit-tested without a database", and
every test below runs against the vocabulary that ships and values built in
memory — so the tuning loop §17 describes for authored content is a `go test`
rather than a playthrough.

What is worth proving here is not that the boolean algebra works. It is the
decisions the design actually rests on: silence is not falsehood, two answers to
two questions can agree, two that disagree stop the character rather than picking
a winner, and the same input can only ever give the same answer.
*/

// helper: the rules a player might hold, composed in the test's own words.
func says(question string, when []Ref, join Join, then, otherwise Ref) Rule {
	return Rule{Question: question, Sentence: Sentence{When: when, Join: join, Then: then, Else: otherwise}}
}

func at(t *testing.T, d Decision, question string) Consideration {
	t.Helper()
	for _, c := range d.Considered {
		if c.Question == question {
			return c
		}
	}
	t.Fatalf("%q was not considered; the moment put %v", question, put(d))
	return Consideration{}
}

func put(d Decision) []string {
	out := []string{}
	for _, c := range d.Considered {
		out = append(out, c.Question)
	}
	return out
}

/*
The ticket's first line: same rule set, same situation, same result.

Fifty runs rather than two, because the thing that would break this is a map
iteration leaking into the order questions are walked in — and a map with three
keys in it agrees with itself most of the time.
*/
func TestTheSameRuleSetAndTheSameMomentAlwaysDecideTheSameWay(t *testing.T) {
	t.Parallel()

	rules := []Rule{
		says("help", []Ref{{Term: "fee", Value: "fair"}, {Term: "legal"}}, And, Ref{Term: "yes"}, Ref{Term: "no"}),
		says("voice", []Ref{{Term: "above"}}, And, Ref{Term: "quiet"}, Ref{Term: "back"}),
		says("overtime", []Ref{{Term: "rate", Value: "half-again"}}, And, Ref{Term: "accept"}, Ref{Term: "refuse"}),
	}
	moment := Arises("a-late-favour", "help", "overtime", "voice").
		Reading("help/condition/fee", 7500).
		So("help/condition/legal", true).
		So("voice/condition/above", true).
		Reading("overtime/condition/rate", 160)

	first := Decide(rules, moment)
	for i := 0; i < 50; i++ {
		again := Decide(rules, moment)
		if !reflect.DeepEqual(first, again) {
			t.Fatalf("run %d decided differently:\n %+v\n %+v", i, first, again)
		}
	}
	// And the walk is the authored order, which is what makes "the first one
	// checked wins" a thing a player can be shown.
	if got := put(first); !reflect.DeepEqual(got, []string{"help", "voice", "overtime"}) {
		t.Errorf("considered %v; the questions are weighed in their authored order", got)
	}
}

/*
Attribution, which is the whole argument for composing rules rather than writing
them (technical.md §6).

A record that only kept the verdict would leave the player unable to tell which
half of their sentence was the one that was wrong.
*/
func TestTheRecordKnowsWhichConditionMatched(t *testing.T) {
	t.Parallel()

	rules := []Rule{says("help",
		[]Ref{{Term: "fee", Value: "good"}, {Term: "legal"}}, And,
		Ref{Term: "yes"}, Ref{Term: "no"})}

	// Paid well below the threshold, and legal. One condition of two.
	d := Decide(rules, Arises("a-cheap-favour", "help").
		Reading("help/condition/fee", 500).
		So("help/condition/legal", true))

	c := at(t, d, "help")
	if want := []Truth{Unmet, Met}; !reflect.DeepEqual(c.Met, want) {
		t.Errorf("met = %v, want %v — per condition, in the sentence's own order", c.Met, want)
	}
	if c.Held != Unmet || c.State != Fired || c.Action.Term != "no" {
		t.Errorf("held %q in state %q doing %q; an unheld `when` fires the otherwise",
			c.Held, c.State, c.Action.Term)
	}
	if c.Says != "when they are paying me above $150 and the task is legal then say yes otherwise say no" {
		t.Errorf("says %q", c.Says)
	}
	if c.Did != "say no" {
		t.Errorf("did %q, want the action alone", c.Did)
	}
}

// Both directions of comparison, because which way a threshold runs belongs to
// the value list and a sign error there is invisible in the sentence.
func TestAThresholdIsReadTheWayItsListDeclares(t *testing.T) {
	t.Parallel()

	// `fee` is at-least: "above $60" holds at exactly $60 and above.
	floor := []Rule{says("help", []Ref{{Term: "fee", Value: "fair"}}, And, Ref{Term: "yes"}, Ref{Term: "no"})}
	// `hours` is at-most: "under 2 hours" holds at exactly 120 minutes and below.
	ceiling := []Rule{says("help", []Ref{{Term: "short", Value: "short"}}, And, Ref{Term: "yes"}, Ref{Term: "no"})}

	for _, tc := range []struct {
		name   string
		rules  []Rule
		term   string
		amount int64
		want   Truth
	}{
		{"a floor, exactly", floor, "help/condition/fee", 6000, Met},
		{"a floor, a penny under", floor, "help/condition/fee", 5999, Unmet},
		{"a ceiling, exactly", ceiling, "help/condition/short", 120, Met},
		{"a ceiling, a minute over", ceiling, "help/condition/short", 121, Unmet},
	} {
		d := Decide(tc.rules, Arises("m", "help").Reading(tc.term, tc.amount))
		if got := at(t, d, "help").Held; got != tc.want {
			t.Errorf("%s: %d read as %q, want %q", tc.name, tc.amount, got, tc.want)
		}
	}
}

/*
Silence is not falsehood.

The failure mode this prevents is the ugly one: a moment that never mentioned
whether the task was legal quietly firing the `otherwise`, and a player reading
back a rule that looks wrong when the world simply never said.
*/
func TestAMomentThatDidNotSayLeavesTheRuleUndecided(t *testing.T) {
	t.Parallel()

	rules := []Rule{says("help", []Ref{{Term: "legal"}}, And, Ref{Term: "yes"}, Ref{Term: "no"})}

	d := Decide(rules, Arises("a-vague-favour", "help"))
	c := at(t, d, "help")
	if c.State != Undecided || c.Held != Unsaid || !c.Action.Blank() {
		t.Errorf("state %q held %q action %q; an unstated condition applies no rule",
			c.State, c.Held, c.Action.Term)
	}
	if d.Verdict != Silent || !d.Escalates() {
		t.Errorf("verdict %q; a rule that could not be applied leaves the moment for the player", d.Verdict)
	}

	// Stated false is a different thing entirely, and it decides.
	d = Decide(rules, Arises("a-shady-favour", "help").So("help/condition/legal", false))
	if c := at(t, d, "help"); c.Held != Unmet || c.Action.Term != "no" {
		t.Errorf("held %q doing %q; stating a condition false is not the same as leaving it out",
			c.Held, c.Action.Term)
	}
}

/*
Kleene's two useful clauses: silence does not spread.

A rule that can be settled on what the moment did say is settled. Only one that
genuinely turns on something nobody stated comes out undecided — otherwise a
single unstated fact would silence a rule that was never going to depend on it.
*/
func TestSilenceOnlySpreadsWhenItCouldHaveChangedTheAnswer(t *testing.T) {
	t.Parallel()

	both := []Ref{{Term: "fee", Value: "fair"}, {Term: "legal"}}
	for _, tc := range []struct {
		name  string
		join  Join
		fee   int64
		state State
		want  Truth
	}{
		{"and, one condition already unmet", And, 100, Weighed, Unmet},
		{"and, one condition met and one unsaid", And, 9000, Undecided, Unsaid},
		{"or, one condition already met", Or, 9000, Weighed, Met},
		{"or, one condition unmet and one unsaid", Or, 100, Undecided, Unsaid},
	} {
		rules := []Rule{says("help", both, tc.join, Ref{Term: "yes"}, Ref{Term: "no"})}
		d := Decide(rules, Arises("m", "help").Reading("help/condition/fee", tc.fee))
		c := at(t, d, "help")
		// `Fired` is `Weighed` promoted once the verdict is known; the two are
		// the same statement about the rule.
		if c.State == Fired {
			c.State = Weighed
		}
		if c.Held != tc.want || c.State != tc.state {
			t.Errorf("%s: held %q in state %q, want %q / %q", tc.name, c.Held, c.State, tc.want, tc.state)
		}
	}
}

/*
Two questions, one moment, and answers that agree.

This is what [Stance] is for. "Accept it" and "say yes" are the same answer given
to two different questions, and a matcher that compared the words would escalate
every well-covered moment as a contradiction.
*/
func TestTwoAnswersThatAmountToTheSameThingAreNotADisagreement(t *testing.T) {
	t.Parallel()

	rules := []Rule{
		says("help", []Ref{{Term: "legal"}}, And, Ref{Term: "yes"}, Ref{Term: "no"}),
		says("overtime", []Ref{{Term: "owed"}}, And, Ref{Term: "accept"}, Ref{Term: "refuse"}),
	}
	moment := Arises("a-late-favour-from-the-boss", "help", "overtime").
		So("help/condition/legal", true).
		So("overtime/condition/owed", true)

	d := Decide(rules, moment)
	if d.Verdict != Answered || d.Escalates() {
		t.Fatalf("verdict %q; yield and yield are one answer in two vocabularies", d.Verdict)
	}
	fired, ok := d.Fired()
	if !ok || fired.Question != "help" {
		t.Errorf("fired %+v; the earlier question decides when they agree", fired)
	}
	if other := at(t, d, "overtime"); other.State != Weighed {
		t.Errorf("overtime is %q; the answer that did not decide was still weighed", other.State)
	}
	if d.Note() != "" {
		t.Errorf("note %q; nothing escalated", d.Note())
	}
}

/*
The ticket's third line: two rules that both match and disagree, detected before
either fires.

"Before either fires" is structural rather than careful — Decide returns a value
and carries nothing out — but the verdict is the part that matters: the character
does not pick a side. Choosing one would be the game making exactly the judgement
it exists to teach.
*/
func TestTwoAnswersThatDisagreeStopTheCharacterRatherThanPickingOne(t *testing.T) {
	t.Parallel()

	rules := []Rule{
		says("help", []Ref{{Term: "legal"}}, And, Ref{Term: "yes"}, Ref{Term: "no"}),
		says("overtime", []Ref{{Term: "already"}}, And, Ref{Term: "refuse"}, Ref{Term: "accept"}),
	}
	moment := Arises("a-late-favour-again", "help", "overtime").
		So("help/condition/legal", true).
		So("overtime/condition/already", true)

	d := Decide(rules, moment)
	if d.Verdict != Clash || !d.Escalates() {
		t.Fatalf("verdict %q, want %q", d.Verdict, Clash)
	}
	if _, ok := d.Fired(); ok {
		t.Error("something fired; a contradiction is not resolved by the game")
	}
	for _, c := range d.Considered {
		if c.State == Fired {
			t.Errorf("%q is marked fired inside a clash", c.Question)
		}
		if !c.Applied() {
			t.Errorf("%q did not apply; both halves of a clash reached an action", c.Question)
		}
	}
	// The note names the answers by number and by what they amounted to, not by
	// the actions' own words — the point being made is that two questions got
	// the same kind of answer.
	if want := "answer 1 said to go along with it; answer 4 said to refuse."; d.Note() != want {
		t.Errorf("note = %q, want %q", d.Note(), want)
	}
}

// A question the moment put and the player has no answer to. §3.14: gaps in the
// playbook are what demand attention, so the gap is recorded rather than skipped.
func TestAQuestionWithNoRuleIsRecordedRatherThanSkipped(t *testing.T) {
	t.Parallel()

	rules := []Rule{says("help", []Ref{{Term: "legal"}}, And, Ref{Term: "yes"}, Ref{Term: "no"})}
	d := Decide(rules, Arises("a-shout", "help", "voice").So("help/condition/legal", true))

	c := at(t, d, "voice")
	if c.State != Unanswered || c.Applied() || len(c.Rule.When) != 0 {
		t.Errorf("voice is %+v; a question with no rule is a row, not a silence", c)
	}
	if d.Verdict != Answered {
		t.Errorf("verdict %q; one answered question still answers the moment", d.Verdict)
	}
}

// An incomplete rule is not in force. InForce filters them out, but Decide is
// exported and pure, and the invariant belongs where it is relied on.
func TestARuleWithAHoleInItIsNotConsidered(t *testing.T) {
	t.Parallel()

	rules := []Rule{says("help", []Ref{{Term: "legal"}}, And, Ref{Term: "yes"}, Ref{})}
	d := Decide(rules, Arises("m", "help").So("help/condition/legal", true))
	if c := at(t, d, "help"); c.State != Unanswered {
		t.Errorf("state %q; nothing partial ever runs", c.State)
	}
	if d.Verdict != Silent {
		t.Errorf("verdict %q, want %q", d.Verdict, Silent)
	}
}

// A moment that states something this vocabulary cannot read is an authoring
// mistake, and it surfaces as one rather than as a player with no rules.
func TestAFactTheVocabularyCannotReadIsAFault(t *testing.T) {
	t.Parallel()

	moment := Arises("m", "help").
		So("help/condition/legal", true).
		So("help/condition/nonsense", true).
		// `fee` takes a number; stating it as a plain truth is the other fault.
		So("help/condition/fee", true).
		// An action is not something a moment can state.
		So("help/action/yes", true)

	want := []string{"help/action/yes", "help/condition/fee", "help/condition/nonsense"}
	if got := moment.Faults(); !reflect.DeepEqual(got, want) {
		t.Errorf("faults = %v, want %v", got, want)
	}

	// The matcher stays total: an unreadable fact is a condition nobody spoke
	// to, which escalates rather than firing something nobody meant.
	rules := []Rule{says("help", []Ref{{Term: "fee", Value: "fair"}}, And, Ref{Term: "yes"}, Ref{Term: "no"})}
	if d := Decide(rules, moment); d.Verdict != Silent {
		t.Errorf("verdict %q; an unreadable fact must not decide anything", d.Verdict)
	}
}

// Situations are values. A moment handed to two callers cannot be edited by one
// of them.
func TestStatingAFactDoesNotChangeTheMomentItWasStatedOn(t *testing.T) {
	t.Parallel()

	bare := Arises("m", "help")
	stated := bare.So("help/condition/legal", true)

	rules := []Rule{says("help", []Ref{{Term: "legal"}}, And, Ref{Term: "yes"}, Ref{Term: "no"})}
	if d := Decide(rules, bare); d.Verdict != Silent {
		t.Errorf("the original moment decided %q; it was never told anything", d.Verdict)
	}
	if d := Decide(rules, stated); d.Verdict != Answered {
		t.Errorf("the stated moment decided %q", d.Verdict)
	}
}

/*
The static half: what can be said about a rule set with no moment in hand.

technical.md §6 lists it as one of the four reasons composed rules beat written
ones — "these two both match and disagree is good UI, and impossible with prose."
*/
func TestAContradictionIsVisibleBeforeAnythingHappens(t *testing.T) {
	t.Parallel()

	// help answers by yielding or bargaining; voice by resisting or
	// withdrawing; overtime by yielding or resisting. So help and voice can
	// agree on nothing whatever happens, while help and overtime agree the day
	// they both yield.
	rules := []Rule{
		says("help", []Ref{{Term: "legal"}}, And, Ref{Term: "yes"}, Ref{Term: "price"}),
		says("voice", []Ref{{Term: "above"}}, And, Ref{Term: "back"}, Ref{Term: "leave"}),
		says("overtime", []Ref{{Term: "owed"}}, And, Ref{Term: "accept"}, Ref{Term: "refuse"}),
	}

	audit := Foresee(rules)
	found := map[string]bool{}
	for _, c := range audit.Conflicts {
		found[c.Questions[0]+"/"+c.Questions[1]] = c.Always
	}
	if len(found) != 3 {
		t.Errorf("%d conflicting pairs among three rules, want 3", len(found))
	}
	if always, ok := found["help/voice"]; !ok || !always {
		t.Errorf("help vs voice: %v/%v — yield-or-bargain agrees with resist-or-withdraw "+
			"on nothing, which is the pair worth a player's attention", ok, always)
	}
	if always, ok := found["help/overtime"]; !ok || always {
		t.Errorf("help vs overtime: %v/%v — they can agree, so the conflict is a sometimes", ok, always)
	}
	if len(audit.Idle) != 0 {
		t.Errorf("idle = %v; every rule here turns on its own conditions", audit.Idle)
	}

	// Scoped to the questions a moment would actually put, which is what an
	// authored situation declares.
	if scoped := Foresee(rules, "help", "overtime"); len(scoped.Conflicts) != 1 {
		t.Errorf("%d conflicts among two questions, want 1", len(scoped.Conflicts))
	}
}

// A rule whose two answers amount to the same posture is in force and settles
// nothing — which is almost never what the player meant, and is worth saying.
func TestARuleThatAnswersTheSameWayEitherWayIsCalledOut(t *testing.T) {
	t.Parallel()

	// `yes` and `once` both yield.
	rules := []Rule{says("help", []Ref{{Term: "legal"}}, And, Ref{Term: "yes"}, Ref{Term: "once"})}
	audit := Foresee(rules)
	if !reflect.DeepEqual(audit.Idle, []string{"help"}) {
		t.Errorf("idle = %v, want [help]", audit.Idle)
	}
	// It still decides the moment — it is a real answer, just an unconditional
	// one. Saying so is a remark, not a refusal.
	d := Decide(rules, Arises("m", "help").So("help/condition/legal", false))
	if d.Verdict != Answered {
		t.Errorf("verdict %q; an unconditional answer is still an answer", d.Verdict)
	}
}

/*
The matcher is pure.

#19 puts the matching logic in the core, and internal/simulation cannot hold it:
that package imports nothing at all (see its purity_test.go) and a matcher is
defined over an authored vocabulary. So the rule is kept here instead, on the one
file — no database, no clock, no other module.
*/
func TestTheMatcherIsPure(t *testing.T) {
	t.Parallel()

	f, err := parser.ParseFile(token.NewFileSet(), "match.go", nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse match.go: %v", err)
	}

	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			t.Fatalf("unquote %s: %v", imp.Path.Value, err)
		}
		if strings.Contains(path, ".") {
			t.Errorf("match.go imports %q — the matcher takes state in and hands a decision "+
				"back, so it needs no database, no driver and no other module", path)
		}
	}

	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if pkg.Name == "time" && (sel.Sel.Name == "Now" || sel.Sel.Name == "Since") {
			t.Errorf("match.go calls time.%s — the same moment must decide the same way "+
				"whenever it is replayed", sel.Sel.Name)
		}
		return true
	})
}
