package playbook

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/MichaelSheinman/EscapeTheMatrix/content"
)

/*
The authored vocabulary: the questions the world will ask, the terms a rule may
be composed from, and the numbers those terms may hold.

HighLevelOverview.md §3.14 calls this "the largest remaining design artifact",
and the reason is worth restating where the code is: **the vocabulary is the
space of thinkable strategies.** A player cannot compose a rule this file has no
words for, so what is in here decides what the game is about — and what is *not*
in here decides it just as much.

It is content rather than code (technical.md §5.2), which means adding a term is
a YAML edit and a deploy, and changing what a term *does* is neither.
*/

// Placeholder is where a parameterised term's number goes, in the sentence the
// term reads as.
//
// It crosses the wire intact rather than being substituted server-side, because
// the builder (#20) puts a menu *inside* the sentence — "they are paying me
// [above $60]" — and a client handed a finished string would have to find the
// number again by searching for it.
const Placeholder = "{value}"

// Compare is how a value list's amount is meant.
//
// It belongs to the list rather than to the term because the word is already in
// the sentence: a list whose options all begin "above" is a floor however it is
// used, and one whose options begin "under" is a ceiling. The matcher (#19)
// reads this; nothing on the wire does.
type Compare string

const (
	// AtLeast is a floor: the reading has to be this or more.
	AtLeast Compare = "at-least"
	// AtMost is a ceiling: the reading has to be this or less.
	AtMost Compare = "at-most"
	// Quantity is not a test at all — it is an amount an action hands over.
	Quantity Compare = "quantity"
)

// Kind is which half of a sentence a term belongs to.
//
// The two pools are separate for a reason that outlives the type system: a
// condition and an action are different parts of speech, and a vocabulary that
// let one stand in for the other would compose sentences nobody can read.
type Kind string

const (
	Condition Kind = "condition"
	Action    Kind = "action"
)

/*
Stance is what an action amounts to, out of a closed set every question shares.

It is the one authored property that crosses between questions, and it exists
because a moment can put more than one of them. Somebody raising their voice
while asking you to work late puts `voice` and `overtime` at once, and the two
rules the player wrote for those questions then both reach an action — so the
matcher has to be able to tell "accept it" and "say yes" (two words for the same
posture) from "accept it" and "say no" (a disagreement, which escalates).

Without it, agreement across questions would have to be a table of which action
pairs happen to be compatible: a list that grows as the square of the vocabulary
and that nobody would keep correct. A posture per action is one field per line
of content and it is the thing an author already knows when they write the line.

Like an amount, a stance never crosses the wire. The client composes sentences;
it does not work out what they would do.
*/
type Stance string

const (
	// Yield goes along with what the other person wants.
	Yield Stance = "yield"
	// Resist does not.
	Resist Stance = "resist"
	// Bargain answers with terms of your own.
	Bargain Stance = "bargain"
	// Defer puts it off without answering.
	Defer Stance = "defer"
	// Open turns it into a conversation rather than a decision.
	Open Stance = "open"
	// Withdraw removes you from the moment.
	Withdraw Stance = "withdraw"
)

/*
Stances is the closed set, in the order a moment offers them.

Exported because a situation is answered by *posture* rather than by an action
term (#28): the vocabulary has four ways to say yes and the world has one
consequence for saying it, so the authored consequences of a job situation are
keyed by what a rule amounted to. A catalogue that had to name the six itself
would be a second copy of this list, free to fall behind the day a seventh is
added.

It runs least committal first, which is the order an answer is offered in and
the order a log reads them in.
*/
var Stances = []Stance{Yield, Defer, Open, Bargain, Resist, Withdraw}

// stances is the closed set, with how each one reads in a log line — a clash is
// reported to the player in these words rather than in the actions' own, because
// the point being made is that the two answers were the same *kind* of answer to
// two different questions.
var stances = map[Stance]string{
	Yield:    "go along with it",
	Resist:   "refuse",
	Bargain:  "answer with terms of my own",
	Defer:    "put it off",
	Open:     "turn it into a conversation",
	Withdraw: "remove myself",
}

// Reads is the stance in the words a log line uses.
func (s Stance) Reads() string { return stances[s] }

/*
Value is one number a parameterised term may hold.

`Says` is the whole of what a player reads and the whole of what crosses the
wire. `Amount` is what a matcher compares against, in the unit its list declares
— and it stays on this side, because a client holding the number could work out
the rule the server is about to apply and §10.2's "the client computes no game
rules" would be a convention rather than a shape.
*/
type Value struct {
	Key    string
	Says   string
	Amount int64
}

/*
Values is a closed, authored list of the numbers one kind of term may take.

§3.14 again: a free field or a slider would let a player tune "paid at least
$137" until it fitted last week, which is bookkeeping rather than judgement. Four
options make the interesting question *which* threshold and never how precisely
one can be named — and they make a rule comparable across players, which is what
lets an environment have an answer that works rather than a spread of near-misses.
*/
type Values struct {
	Key     string
	Label   string
	Unit    string
	Compare Compare

	options []Value
}

// Options is the list, least first.
func (v Values) Options() []Value {
	out := make([]Value, len(v.options))
	copy(out, v.options)
	return out
}

// Option finds one by key.
func (v Values) Option(key string) (Value, bool) {
	for _, o := range v.options {
		if o.Key == key {
			return o, true
		}
	}
	return Value{}, false
}

/*
Term is one word of the vocabulary: a condition a rule may test, or an action it
may take.

`Start` is whether a player arrives with it. Everything else is earned through
struggle-gating (§3.6) — and unlike a locked routine, a locked term carries no
sentence explaining itself, because it is not shown at all. A routine you cannot
configure is a visible absence with a reason attached; a term you have not
thought of yet is simply not a thing you have thought of.
*/
type Term struct {
	// Question is which question's pool this term belongs to. Terms are scoped
	// per question rather than drawn from one global list: "they are paying me
	// above $60" is not a thing you can say about somebody raising their voice
	// at you.
	Question string
	Kind     Kind
	Key      string
	// Says is the term as a sentence fragment, with Placeholder where its
	// number goes.
	Says string
	// Values is the closed list this term's number is drawn from, or nil for a
	// term that takes none.
	Values *Values
	// Stance is what this action amounts to, and is empty on a condition. A
	// condition states what is so; only an answer has a posture.
	Stance Stance
	Start  bool
}

// ID is the term's name across the whole vocabulary, which is what an unlock is
// recorded against. Keys are unique only inside one pool of one question — a
// question can have both a condition and an action called "leave", and does.
func (t Term) ID() string { return t.Question + "/" + string(t.Kind) + "/" + t.Key }

// Takes reports whether this term holds a number.
func (t Term) Takes() bool { return t.Values != nil }

/*
Question is one thing the world will definitely ask, and the whole vocabulary of
answers to it.

The playbook is not a list of rules the player thought of. It is a small fixed
set of these, and §3.14's loop — answer, walk around for a week, read what
happened, answer again — runs against each one separately. Nothing anywhere
states how many there are: the set grows, and copy that counted it would be
wrong the day another one lands.
*/
type Question struct {
	Key   string
	Order int
	// Ask is the question in the player's own words. Note is what it costs to
	// have no answer to it. About is how the question reads inside a log line —
	// "someone asks me for help" — where the sentence is being reported rather
	// than posed.
	Ask   string
	Note  string
	About string

	conditions []Term
	actions    []Term
}

// Conditions and Actions are the two pools, in authored order. The first action
// is the least committal one, because it is what an unanswered question opens
// with.
func (q Question) Conditions() []Term { return copyTerms(q.conditions) }
func (q Question) Actions() []Term    { return copyTerms(q.actions) }

// Terms is one pool by kind.
func (q Question) Terms(kind Kind) []Term {
	if kind == Action {
		return q.Actions()
	}
	return q.Conditions()
}

// Term finds one word of this question's vocabulary.
func (q Question) Term(kind Kind, key string) (Term, bool) {
	pool := q.conditions
	if kind == Action {
		pool = q.actions
	}
	for _, t := range pool {
		if t.Key == key {
			return t, true
		}
	}
	return Term{}, false
}

// only is this question with everything the player has not unlocked removed.
//
// Removed rather than blanked: §3.14's terms are not achievements to be shown
// greyed out. Reserving a row for a word nobody has lengthens every menu and
// spends the surprise early.
func (q Question) only(unlocked map[string]bool) Question {
	out := q
	out.conditions = keep(q.conditions, unlocked)
	out.actions = keep(q.actions, unlocked)
	return out
}

func keep(pool []Term, unlocked map[string]bool) []Term {
	out := make([]Term, 0, len(pool))
	for _, t := range pool {
		if t.Start || unlocked[t.ID()] {
			out = append(out, t)
		}
	}
	return out
}

func copyTerms(in []Term) []Term {
	out := make([]Term, len(in))
	copy(out, in)
	return out
}

// Vocabulary is the whole authored set, parsed and validated once.
type Vocabulary struct {
	questions []Question
	values    map[string]Values
}

// Questions is every standing question, in authored order.
func (v Vocabulary) Questions() []Question {
	out := make([]Question, len(v.questions))
	copy(out, v.questions)
	return out
}

// Question finds one by key.
func (v Vocabulary) Question(key string) (Question, bool) {
	for _, q := range v.questions {
		if q.Key == key {
			return q, true
		}
	}
	return Question{}, false
}

// Term finds one by the id an unlock is recorded against.
func (v Vocabulary) Term(id string) (Term, bool) {
	parts := strings.Split(id, "/")
	if len(parts) != 3 {
		return Term{}, false
	}
	q, ok := v.Question(parts[0])
	if !ok {
		return Term{}, false
	}
	return q.Term(Kind(parts[1]), parts[2])
}

// starting is every term a player has from the beginning.
func (v Vocabulary) starting() map[string]bool {
	out := map[string]bool{}
	for _, q := range v.questions {
		for _, pool := range [][]Term{q.conditions, q.actions} {
			for _, t := range pool {
				if t.Start {
					out[t.ID()] = true
				}
			}
		}
	}
	return out
}

/*
vocabulary is the one parsed copy, built at process start.

A package-level value rather than something the server loads and passes around,
for the same reason internal/city's graph is: it is authored content with no
state in it, identical in every process, and a second copy could only ever be a
second opinion. Vocab reads it.
*/
var vocabulary Vocabulary

/*
init parses and validates the shipped content, and panics if it cannot.

technical.md §5.2: "production validates at startup so a malformed challenge
fails in CI, not at 2am." A panic here is that, and it is also why `parse` takes
an fs.FS rather than reading the embedded files itself — the failure paths are
worth testing, and a test cannot catch an init that already ran.
*/
func init() {
	v, err := parse(content.FS)
	if err != nil {
		panic("playbook: " + err.Error())
	}
	vocabulary = v
}

// Vocab is the authored vocabulary. Read-only; every accessor on it copies.
func Vocab() Vocabulary { return vocabulary }

/* ── reading the files ─────────────────────────────────────────────────── */

const (
	valuesPath    = "playbook/values.yaml"
	questionsPath = "playbook/questions"
)

// The YAML shapes. Separate from the types above on purpose: what an author
// writes and what the rest of the program reads are two decisions, and a term
// that carried its file's field names would make renaming one a migration.
type valuesFile struct {
	Key     string `yaml:"key"`
	Label   string `yaml:"label"`
	Unit    string `yaml:"unit"`
	Compare string `yaml:"compare"`
	Options []struct {
		Key    string `yaml:"key"`
		Says   string `yaml:"says"`
		Amount int64  `yaml:"amount"`
	} `yaml:"options"`
}

type questionFile struct {
	Key        string     `yaml:"key"`
	Order      int        `yaml:"order"`
	Ask        string     `yaml:"ask"`
	Note       string     `yaml:"note"`
	About      string     `yaml:"about"`
	Conditions []termFile `yaml:"conditions"`
	Actions    []termFile `yaml:"actions"`
}

type termFile struct {
	Key    string `yaml:"key"`
	Says   string `yaml:"says"`
	Values string `yaml:"values"`
	Stance string `yaml:"stance"`
	Start  bool   `yaml:"start"`
}

/*
parse reads the vocabulary out of a content filesystem and checks every rule the
rest of this package then gets to assume.

It is strict twice over. Unknown YAML fields are refused, so a misspelt `start:`
is a failed build rather than a term nobody can reach; and every cross-reference
— a term naming a value list, a list naming a comparison — is resolved here
rather than at the moment a player happens to select it.
*/
func parse(files fs.FS) (Vocabulary, error) {
	values, err := parseValues(files)
	if err != nil {
		return Vocabulary{}, err
	}

	entries, err := fs.ReadDir(files, questionsPath)
	if err != nil {
		return Vocabulary{}, fmt.Errorf("read %s: %w", questionsPath, err)
	}

	v := Vocabulary{values: values}
	orders := map[int]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		name := path.Join(questionsPath, entry.Name())

		var file questionFile
		if err := decode(files, name, &file); err != nil {
			return Vocabulary{}, err
		}
		q, err := question(file, values)
		if err != nil {
			return Vocabulary{}, fmt.Errorf("%s: %w", name, err)
		}
		if _, clash := v.Question(q.Key); clash {
			return Vocabulary{}, fmt.Errorf("%s: two questions called %q", name, q.Key)
		}
		if taken, clash := orders[q.Order]; clash {
			return Vocabulary{}, fmt.Errorf("%s: %q and %q are both question %d", name, taken, q.Key, q.Order)
		}
		orders[q.Order] = q.Key
		v.questions = append(v.questions, q)
	}

	if len(v.questions) == 0 {
		return Vocabulary{}, fmt.Errorf("%s holds no questions", questionsPath)
	}
	// Contiguous from one, so the order is a numbering a player can be shown
	// rather than a sort key with holes in it.
	for i := 1; i <= len(v.questions); i++ {
		if _, ok := orders[i]; !ok {
			return Vocabulary{}, fmt.Errorf("%s: there is no question %d", questionsPath, i)
		}
	}
	sort.SliceStable(v.questions, func(i, j int) bool { return v.questions[i].Order < v.questions[j].Order })

	return v, nil
}

func parseValues(files fs.FS) (map[string]Values, error) {
	var list []valuesFile
	if err := decode(files, valuesPath, &list); err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("%s holds no value lists", valuesPath)
	}

	out := make(map[string]Values, len(list))
	for _, file := range list {
		v, err := values(file)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", valuesPath, err)
		}
		if _, clash := out[v.Key]; clash {
			return nil, fmt.Errorf("%s: two value lists called %q", valuesPath, v.Key)
		}
		out[v.Key] = v
	}
	return out, nil
}

func values(file valuesFile) (Values, error) {
	if file.Key == "" {
		return Values{}, fmt.Errorf("a value list has no key")
	}
	if file.Label == "" || file.Unit == "" {
		return Values{}, fmt.Errorf("%q needs a label and a unit", file.Key)
	}
	switch Compare(file.Compare) {
	case AtLeast, AtMost, Quantity:
	default:
		return Values{}, fmt.Errorf("%q compares %q, which is not one of %q, %q, %q",
			file.Key, file.Compare, AtLeast, AtMost, Quantity)
	}
	// Two is the fewest that is a choice. A one-option list is a constant
	// wearing a menu, and the player would be picking the only thing there is.
	if len(file.Options) < 2 {
		return Values{}, fmt.Errorf("%q offers %d numbers; a closed list is only a decision at two or more",
			file.Key, len(file.Options))
	}

	out := Values{Key: file.Key, Label: file.Label, Unit: file.Unit, Compare: Compare(file.Compare)}
	seen := map[string]bool{}
	var last int64
	for i, o := range file.Options {
		if o.Key == "" || o.Says == "" {
			return Values{}, fmt.Errorf("%q has an option with no key or nothing to say", file.Key)
		}
		if seen[o.Key] {
			return Values{}, fmt.Errorf("%q offers %q twice", file.Key, o.Key)
		}
		seen[o.Key] = true
		// Ascending, so a menu reads least-to-most everywhere without any screen
		// having to sort it — and so "tighten the threshold" is a direction
		// rather than a lookup.
		if i > 0 && o.Amount <= last {
			return Values{}, fmt.Errorf("%q lists %q after a larger number; the options run least first",
				file.Key, o.Key)
		}
		last = o.Amount
		out.options = append(out.options, Value{Key: o.Key, Says: o.Says, Amount: o.Amount})
	}
	return out, nil
}

func question(file questionFile, values map[string]Values) (Question, error) {
	if file.Key == "" {
		return Question{}, fmt.Errorf("a question has no key")
	}
	if file.Order < 1 {
		return Question{}, fmt.Errorf("%q is question %d; questions are numbered from one", file.Key, file.Order)
	}
	if file.Ask == "" || file.Note == "" || file.About == "" {
		return Question{}, fmt.Errorf("%q needs an ask, a note and an about", file.Key)
	}

	q := Question{Key: file.Key, Order: file.Order, Ask: file.Ask, Note: file.Note, About: file.About}

	var err error
	if q.conditions, err = pool(file.Key, Condition, file.Conditions, values); err != nil {
		return Question{}, err
	}
	if q.actions, err = pool(file.Key, Action, file.Actions, values); err != nil {
		return Question{}, err
	}

	if len(q.conditions) == 0 {
		return Question{}, fmt.Errorf("%q has nothing to test", file.Key)
	}
	// Two, because `then` and `otherwise` are both drawn from this pool and a
	// question whose only answer is the same one either way is not a question.
	if len(q.actions) < 2 {
		return Question{}, fmt.Errorf("%q offers %d actions; a rule needs a then and an otherwise",
			file.Key, len(q.actions))
	}

	// A player has to be able to finish a sentence on the day they arrive, or
	// the question is open with nothing they can do about it.
	if !q.actions[0].Start {
		return Question{}, fmt.Errorf("%q opens on %q, which is locked; the first action is what an "+
			"unanswered question opens with", file.Key, q.actions[0].Key)
	}
	if countStart(q.conditions) < 1 || countStart(q.actions) < 2 {
		return Question{}, fmt.Errorf("%q starts with %d conditions and %d actions; a rule cannot be "+
			"composed from fewer than one and two", file.Key, countStart(q.conditions), countStart(q.actions))
	}
	// Two postures, among what a player *arrives* with. A question whose every
	// opening answer amounts to the same thing is one the player cannot decide
	// either way on their first day, and their rule for it would settle nothing
	// however they composed it.
	if p := postures(q.actions); len(p) < 2 {
		return Question{}, fmt.Errorf("%q opens with %d posture(s) — %v; a rule needs a then and an "+
			"otherwise that differ in what they amount to", file.Key, len(p), p)
	}
	return q, nil
}

// postures is the distinct stances among the actions a player starts with.
func postures(pool []Term) []Stance {
	seen := map[Stance]bool{}
	out := []Stance{}
	for _, t := range pool {
		if t.Start && !seen[t.Stance] {
			seen[t.Stance] = true
			out = append(out, t.Stance)
		}
	}
	return out
}

// known is the closed stance set, sorted, for a message an author has to act on.
func known() []Stance {
	out := make([]Stance, 0, len(stances))
	for s := range stances {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func pool(question string, kind Kind, files []termFile, values map[string]Values) ([]Term, error) {
	out := make([]Term, 0, len(files))
	seen := map[string]bool{}
	for _, file := range files {
		if file.Key == "" || file.Says == "" {
			return nil, fmt.Errorf("%q has a %s with no key or nothing to say", question, kind)
		}
		if seen[file.Key] {
			return nil, fmt.Errorf("%q has two %ss called %q", question, kind, file.Key)
		}
		seen[file.Key] = true

		t := Term{Question: question, Kind: kind, Key: file.Key, Says: file.Says, Start: file.Start}

		// A posture belongs to an answer and to nothing else. A condition
		// carrying one would be a fact with an opinion in it, and the matcher
		// would have two places to look for what a rule amounts to.
		switch {
		case kind == Condition && file.Stance != "":
			return nil, fmt.Errorf("%q's condition %q takes a stance; only an action has one",
				question, file.Key)
		case kind == Condition:
		case file.Stance == "":
			return nil, fmt.Errorf("%q's action %q has no stance; the matcher compares answers across "+
				"questions by what they amount to", question, file.Key)
		default:
			if _, ok := stances[Stance(file.Stance)]; !ok {
				return nil, fmt.Errorf("%q's action %q takes the stance %q, which is not one of %v",
					question, file.Key, file.Stance, known())
			}
			t.Stance = Stance(file.Stance)
		}

		holds := strings.Contains(file.Says, Placeholder)
		switch {
		case file.Values == "" && holds:
			return nil, fmt.Errorf("%q's %s %q has a %s in it and no list to draw one from",
				question, kind, file.Key, Placeholder)
		case file.Values == "":
		default:
			list, ok := values[file.Values]
			if !ok {
				return nil, fmt.Errorf("%q's %s %q draws from %q, which is not a value list",
					question, kind, file.Key, file.Values)
			}
			if !holds {
				return nil, fmt.Errorf("%q's %s %q draws from %q but has nowhere to put it",
					question, kind, file.Key, file.Values)
			}
			// A condition tests and an action hands over, and the value lists
			// say which of those they are for. "Give them above $20" is what
			// this refuses.
			if (kind == Condition) == (list.Compare == Quantity) {
				return nil, fmt.Errorf("%q's %s %q draws from %q, which compares %q",
					question, kind, file.Key, list.Key, list.Compare)
			}
			t.Values = &list
		}
		out = append(out, t)
	}
	return out, nil
}

func countStart(pool []Term) int {
	n := 0
	for _, t := range pool {
		if t.Start {
			n++
		}
	}
	return n
}

// decode reads one YAML file strictly: an unknown field is a failed build
// rather than an authored line nobody notices has no effect.
func decode(files fs.FS, name string, into any) error {
	raw, err := fs.ReadFile(files, name)
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	return nil
}
