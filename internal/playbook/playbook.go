/*
Package playbook owns the rules a player composes for their own judgement — step
three of §3.14's loop: it happens without you, you review, you compose rules, a
rule fails, you revise.

A rule is a sentence: when <condition> [and|or <condition>] then <action>
otherwise <action>, with every part selected from an authored vocabulary
(vocabulary.go) rather than typed. §6 gives the argument: consistency is free,
attribution is exact, and failure is legible. A structured rule lets the player
see which condition matched and which action fired, so §3.15's transfer failure
is diagnosable. Prose would leave them unable to tell "my rule was wrong" from
"the model misread my rule".

Two conditions is the maximum and there is no nesting: two terms is as much as a
rule can carry and still read as a sentence. The table has exactly two condition
columns, so no bug, retry or future caller can compose a third.

The playbook is a set of rules per environment (§3.15), because the right rules
depend on where you are. A set is where the player is standing rather than a
folder they create, and there is one per environment the world has, so arriving
somewhere new means finding an empty set.

There is no draft and no save. One row per question per environment, and editing
the sentence is the change. A sentence with a hole in it does not cover the
question, so an unfinished edit fails safe and the question escalates to the
player the way it did before they answered it.

The matcher and the rule-fire records it writes are elsewhere. This package hands
out [Playbooks.InForce] — the complete rules for an environment, in order — and
decides nothing about what matches. The whole vocabulary is available to
everybody.

It owns playbook_rule (§3.1), takes the caller's transaction and never opens one
(§3.2), and reads no other module's tables: the environments arrive through
[Where].
*/
package playbook

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// The refusals this module has. Go errors rather than status codes:
// internal/api maps them at the edge (§8), so a module never learns HTTP.
var (
	// ErrNoSuchEnvironment is a rule set tagged to somewhere that is not an
	// environment.
	ErrNoSuchEnvironment = errors.New("playbook: no such environment")
	// ErrNoSuchQuestion is a question the world does not ask.
	ErrNoSuchQuestion = errors.New("playbook: no such question")
	// ErrNoSuchTerm is a word that is not in *this question's* vocabulary. The
	// scoping is the point: a condition that exists under another question is
	// refused here exactly as one that exists nowhere.
	ErrNoSuchTerm = errors.New("playbook: no such term")
	// ErrNoSuchValue is a number that is not in the closed list its term draws
	// from — including a number given to a term that takes none.
	ErrNoSuchValue = errors.New("playbook: no such value")
	// ErrNoSuchJoin is a joining word that is not `and` or `or`.
	ErrNoSuchJoin = errors.New("playbook: no such join")
	// ErrConditions is a sentence with no conditions or with more than two.
	ErrConditions = errors.New("playbook: a rule tests one or two conditions")
)

// Environment is one context a rule set is tagged to. §3.15 defines an
// environment by its people rather than its geography, so this names a
// population: the characters a player lives among, which is what makes one set
// of answers right and the same answers wrong somewhere else.
// internal/environment authors them and internal/api binds them.
type Environment struct {
	Key  string
	Name string
}

// Where is the set of environments this playbook has rule sets for. An
// interface, because which catalogue defines a population is not this package's
// business. Not a per-player question: every player has a set for every
// environment including ones they have never been to, so arriving somewhere new
// means finding your old answers waiting to fail (§3.15).
type Where interface {
	Environments() []Environment
}

// Nowhere is a Where for a world with one environment, for tests. Never bound
// in the server.
type Nowhere struct{}

func (Nowhere) Environments() []Environment {
	return []Environment{{Key: "nowhere", Name: "Nowhere in particular"}}
}

// Ref is one part of a sentence: which term, and which of its numbers. The zero
// value is a blank — a part the player has not filled in — and a blank is a
// state rather than an error. It replaces a save button: a sentence with a hole
// in it does not cover its question, and the screen says so.
type Ref struct {
	Term  string
	Value string
}

// Blank reports whether this part of the sentence is still a hole.
func (r Ref) Blank() bool { return r.Term == "" }

// Join is the one word in a rule that changes the logic, which is why it is the
// only piece of grammar that is also a control.
type Join string

const (
	// And requires both conditions.
	And Join = "and"
	// Or requires either.
	Or Join = "or"
)

// Joins is both, in the order they are offered.
var Joins = []Join{And, Or}

// MaxConditions is §3.14's maximum: two terms is as much as a rule can carry and
// still read as a sentence. Nesting is deferred, and the table has nowhere to
// put a third.
const MaxConditions = 2

// Sentence is a composed rule, holes and all. When has one or two entries: one
// is a complete sentence, and two with the second still blank is a player
// mid-edit and is not in force. That is why the count is stored rather than
// inferred from whether the second slot is filled.
type Sentence struct {
	When []Ref
	Join Join
	Then Ref
	Else Ref
}

// Complete reports whether every part of the sentence has been filled in.
// Nothing partial runs (§3.14): a rule with any blank in it is out of force, and
// the question it belongs to goes on escalating to the player.
func (s Sentence) Complete() bool {
	if len(s.When) == 0 || len(s.When) > MaxConditions {
		return false
	}
	for _, w := range s.When {
		if w.Blank() {
			return false
		}
	}
	return !s.Then.Blank() && !s.Else.Blank()
}

// Rule is one question's answer inside one rule set.
type Rule struct {
	Question string
	Sentence
}

// Set is every answer in force in one environment (§3.15). It carries one rule
// per question, blank ones included, because a question with no answer is the
// state the game has the most to say about.
type Set struct {
	Environment Environment
	Rules       []Rule
}

// Rule finds one question's answer.
func (s Set) Rule(question string) (Rule, bool) {
	for _, r := range s.Rules {
		if r.Question == question {
			return r, true
		}
	}
	return Rule{}, false
}

// Playbook is one player's: the vocabulary they may compose from, and every rule
// set they hold. Questions carries every authored term, so a word in the
// catalogue is a word in the menu.
type Playbook struct {
	Questions []Question
	Sets      []Set
}

// Playbooks is the module. It holds no per-player state: the transaction comes
// from the caller and the environments are injected (§3.1).
type Playbooks struct {
	where Where
}

func New(where Where) *Playbooks { return &Playbooks{where: where} }

/*
Of reads a player's whole playbook. A player with no rows has answered nothing,
which is where §3.14's loop begins, and what comes back is complete either way —
every environment, every question — so nothing downstream has to decide what an
absent row means.

Every stored part is re-resolved against the vocabulary as it is read. A term
renamed out of content reads back as a blank, so the rule stops being in force
and the question comes to the player again.
*/
func (m *Playbooks) Of(ctx context.Context, tx pgx.Tx, accountID string) (Playbook, error) {
	stored, err := m.stored(ctx, tx, accountID)
	if err != nil {
		return Playbook{}, err
	}

	questions := vocabulary.Questions()
	environments := m.where.Environments()
	out := Playbook{
		Questions: make([]Question, 0, len(questions)),
		Sets:      make([]Set, 0, len(environments)),
	}
	out.Questions = append(out.Questions, questions...)

	for _, env := range environments {
		set := Set{Environment: env, Rules: make([]Rule, 0, len(questions))}
		for _, q := range questions {
			rule := Rule{Question: q.Key}
			if held, ok := stored[key{environment: env.Key, question: q.Key}]; ok {
				rule.Sentence = resolve(q, held)
			} else {
				rule.Sentence = opening(q)
			}
			set.Rules = append(set.Rules, rule)
		}
		out.Sets = append(out.Sets, set)
	}
	return out, nil
}

// InForce is every complete rule for one environment, in the order the questions
// are asked — what the matcher reads. Incomplete rules are absent, so the caller
// never has to ask whether a sentence with a hole in it counts, and the order is
// authored, so "the first one checked wins" is something the player can be
// shown.
func (m *Playbooks) InForce(ctx context.Context, tx pgx.Tx, accountID, environment string) ([]Rule, error) {
	if _, ok := m.environment(environment); !ok {
		return nil, fmt.Errorf("%w: %q", ErrNoSuchEnvironment, environment)
	}
	book, err := m.Of(ctx, tx, accountID)
	if err != nil {
		return nil, err
	}
	for _, set := range book.Sets {
		if set.Environment.Key != environment {
			continue
		}
		out := make([]Rule, 0, len(set.Rules))
		for _, r := range set.Rules {
			if r.Complete() {
				out = append(out, r)
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("%w: %q", ErrNoSuchEnvironment, environment)
}

/*
Compose writes one question's sentence in one environment. The whole sentence
arrives, blanks included, and the whole of it is written: there is no per-part
command and no save, so the player changes a word and that is what their
character does from the next time the situation comes up.

Every named term is checked against that question's pool. A condition that exists
under another question is refused here exactly as one that exists nowhere.
*/
func (m *Playbooks) Compose(
	ctx context.Context, tx pgx.Tx, accountID, environment, question string, s Sentence,
) (Playbook, error) {
	if _, ok := m.environment(environment); !ok {
		return Playbook{}, fmt.Errorf("%w: %q", ErrNoSuchEnvironment, environment)
	}
	q, ok := vocabulary.Question(question)
	if !ok {
		return Playbook{}, fmt.Errorf("%w: %q", ErrNoSuchQuestion, question)
	}
	if len(s.When) == 0 || len(s.When) > MaxConditions {
		return Playbook{}, fmt.Errorf("%w: %d given", ErrConditions, len(s.When))
	}
	if s.Join == "" {
		s.Join = And
	}
	if s.Join != And && s.Join != Or {
		return Playbook{}, fmt.Errorf("%w: %q", ErrNoSuchJoin, s.Join)
	}

	for _, w := range s.When {
		if err := check(q, Condition, w); err != nil {
			return Playbook{}, err
		}
	}
	if err := check(q, Action, s.Then); err != nil {
		return Playbook{}, err
	}
	if err := check(q, Action, s.Else); err != nil {
		return Playbook{}, err
	}

	second := Ref{}
	if len(s.When) > 1 {
		second = s.When[1]
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO playbook_rule (
			account_id, environment, question, conditions, join_word,
			when_1_term, when_1_value, when_2_term, when_2_value,
			then_term, then_value, else_term, else_value)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (account_id, environment, question) DO UPDATE SET
			conditions   = EXCLUDED.conditions,
			join_word    = EXCLUDED.join_word,
			when_1_term  = EXCLUDED.when_1_term,
			when_1_value = EXCLUDED.when_1_value,
			when_2_term  = EXCLUDED.when_2_term,
			when_2_value = EXCLUDED.when_2_value,
			then_term    = EXCLUDED.then_term,
			then_value   = EXCLUDED.then_value,
			else_term    = EXCLUDED.else_term,
			else_value   = EXCLUDED.else_value,
			updated_at   = now()`,
		accountID, environment, question, len(s.When), string(s.Join),
		null(s.When[0].Term), null(s.When[0].Value), null(second.Term), null(second.Value),
		null(s.Then.Term), null(s.Then.Value), null(s.Else.Term), null(s.Else.Value),
	); err != nil {
		return Playbook{}, fmt.Errorf("playbook: write rule: %w", err)
	}

	return m.Of(ctx, tx, accountID)
}

/*
Clear takes one question's answer back, which is not the same as composing a
sentence of blanks. A stored row of blanks is a rule the player wrote and left
unfinished; no row at all is a question they have never answered, which is what
[opening] describes. Both are out of force and only one is true.

Clearing a question that was never answered is the state the caller asked for
rather than an error.
*/
func (m *Playbooks) Clear(
	ctx context.Context, tx pgx.Tx, accountID, environment, question string,
) (Playbook, error) {
	if _, ok := m.environment(environment); !ok {
		return Playbook{}, fmt.Errorf("%w: %q", ErrNoSuchEnvironment, environment)
	}
	if _, ok := vocabulary.Question(question); !ok {
		return Playbook{}, fmt.Errorf("%w: %q", ErrNoSuchQuestion, question)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM playbook_rule
		 WHERE account_id = $1 AND environment = $2 AND question = $3`,
		accountID, environment, question,
	); err != nil {
		return Playbook{}, fmt.Errorf("playbook: clear rule: %w", err)
	}
	return m.Of(ctx, tx, accountID)
}

/* ── reading and writing ───────────────────────────────────────────────── */

// key addresses one rule: which set, which question.
type key struct{ environment, question string }

// held is one rule as the columns hold it, before the vocabulary has been
// consulted about any of it.
type held struct {
	conditions int
	join       Join
	when       [MaxConditions]Ref
	then       Ref
	otherwise  Ref
}

func (m *Playbooks) stored(ctx context.Context, tx pgx.Tx, accountID string) (map[key]held, error) {
	rows, err := tx.Query(ctx, `
		SELECT environment, question, conditions, join_word,
		       when_1_term, when_1_value, when_2_term, when_2_value,
		       then_term, then_value, else_term, else_value
		  FROM playbook_rule
		 WHERE account_id = $1`, accountID)
	if err != nil {
		return nil, fmt.Errorf("playbook: read rules: %w", err)
	}
	defer rows.Close()

	out := map[key]held{}
	for rows.Next() {
		var (
			k key
			h held
			j string
			c [4]*string
			a [4]*string
		)
		if err := rows.Scan(&k.environment, &k.question, &h.conditions, &j,
			&c[0], &c[1], &c[2], &c[3], &a[0], &a[1], &a[2], &a[3]); err != nil {
			return nil, fmt.Errorf("playbook: read rules: %w", err)
		}
		h.join = Join(j)
		h.when[0] = Ref{Term: text(c[0]), Value: text(c[1])}
		h.when[1] = Ref{Term: text(c[2]), Value: text(c[3])}
		h.then = Ref{Term: text(a[0]), Value: text(a[1])}
		h.otherwise = Ref{Term: text(a[2]), Value: text(a[3])}
		out[k] = h
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("playbook: read rules: %w", err)
	}
	return out, nil
}

func (m *Playbooks) environment(key string) (Environment, bool) {
	for _, e := range m.where.Environments() {
		if e.Key == key {
			return e, true
		}
	}
	return Environment{}, false
}

/*
opening is the sentence a question nobody has answered starts as: one hole in the
condition, and the first action written in. A sentence with three blanks in it is
a form, and composing in sentences only works if what is being edited stays
readable.

It is not stored. A question with no row has never been answered, and the moment
the player changes a word the whole sentence is written as they left it, so an
edited rule reads back exactly as stored.
*/
func opening(q Question) Sentence {
	return Sentence{When: []Ref{{}}, Join: And, Then: Ref{Term: q.actions[0].Key}}
}

// resolve turns stored columns into a sentence, checking every part against the
// vocabulary.
func resolve(q Question, h held) Sentence {
	n := h.conditions
	if n < 1 || n > MaxConditions {
		n = 1
	}
	out := Sentence{
		When: make([]Ref, 0, n),
		Join: h.join,
		Then: settled(q, Action, h.then),
		Else: settled(q, Action, h.otherwise),
	}
	if out.Join != And && out.Join != Or {
		out.Join = And
	}
	for i := 0; i < n; i++ {
		out.When = append(out.When, settled(q, Condition, h.when[i]))
	}
	return out
}

// settled is one stored part, read back as itself or as a blank.
func settled(q Question, kind Kind, r Ref) Ref {
	if r.Blank() {
		return Ref{}
	}
	t, ok := q.Term(kind, r.Term)
	if !ok {
		return Ref{}
	}
	if !t.Takes() {
		return Ref{Term: t.Key}
	}
	if _, ok := t.Values.Option(r.Value); !ok {
		return Ref{}
	}
	return r
}

// check is the one place a part of a sentence is admitted.
func check(q Question, kind Kind, r Ref) error {
	if r.Blank() {
		// A blank is the state the rule is in while it is being written, and
		// what keeps it out of force.
		if r.Value != "" {
			return fmt.Errorf("%w: a number with no term to hold it", ErrNoSuchValue)
		}
		return nil
	}
	t, ok := q.Term(kind, r.Term)
	if !ok {
		return fmt.Errorf("%w: %q is not a %s of %q", ErrNoSuchTerm, r.Term, kind, q.Key)
	}
	switch {
	case !t.Takes() && r.Value != "":
		return fmt.Errorf("%w: %q takes no number", ErrNoSuchValue, t.ID())
	case t.Takes() && r.Value == "":
		return fmt.Errorf("%w: %q needs one of %q", ErrNoSuchValue, t.ID(), t.Values.Key)
	case t.Takes():
		if _, ok := t.Values.Option(r.Value); !ok {
			return fmt.Errorf("%w: %q is not one of %q", ErrNoSuchValue, r.Value, t.Values.Key)
		}
	}
	return nil
}

// null writes an empty part as SQL NULL, so that "unanswered" is the absence of
// a value in the database as well as in the sentence.
func null(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func text(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
