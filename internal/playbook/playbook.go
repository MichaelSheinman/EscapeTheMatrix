/*
Package playbook owns the rules a player composes for their own judgement.

HighLevelOverview.md §3.14 is the specification, and it starts from something
the rest of the game already assumes: **the player does not act day to day.**
Situations arise while they are elsewhere and the character handles them. So the
loop is *it happens without you → you review → you compose rules → a rule fails →
you revise*, and this package is the third step.

# A rule is a sentence

`when <condition> [and|or <condition>] then <action> otherwise <action>`, with
every part selected from an authored vocabulary (vocabulary.go). Never free
text, and technical.md §6 gives the design argument rather than the cost one:
consistency is free, attribution is exact, and — the one that matters — failure
is *legible*. A structured rule lets the player see which condition matched and
which action fired, which is what makes §3.15's transfer failure diagnosable
rather than a matter of interpretation. Prose would leave them unable to tell "my
rule was wrong" from "the model misread my rule", and that distinction is the
entire lesson.

Two conditions is the ceiling and there is no nesting. That is a design choice
rather than a technical limit: two terms is as much as a rule can carry and still
read as a sentence. It is held by the *shape of the table* — there are exactly
two condition columns — so no bug, retry or future caller can compose a third.

# It is not one list

§3.15 makes the playbook **a set of context-tagged rule sets**, one per
environment, because the whole thesis is that the right rules depend on where you
are. A set is not a folder the player creates; it is where they are standing.
There is one per environment the world has, always, so that arriving somewhere
new means finding an empty set rather than discovering the feature.

# There is no draft, and no save

One row per question per environment, and editing the sentence *is* the change.
A commit step would only be honest if the player could sit and think in a paused
world, and they cannot — the world is running behind the screen either way. What
replaces it is the blank: a sentence with a hole in it is simply not covering the
question, so an unfinished edit fails safe instead of half-running, and the
question escalates to the player the way it did before they answered it.

# What is not here

The matcher (#19) and the rule-fire records it writes. This package hands out
[Playbooks.InForce] — the complete rules for an environment, in order — and
decides nothing about what matches. The vocabulary a player may use is here; what
unlocks the rest is struggle-gating (§3.6), which calls [Playbooks.Unlock].

It owns `playbook_rule` and `playbook_unlock` (§3.1), takes the caller's
transaction and never opens one (§3.2), and reads no other module's tables: the
environments arrive through [Where].
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
	// ErrLocked is a term that exists and that this player has not unlocked
	// (§3.6). The request is fine and it is the world that is in the way, which
	// is a 409 at the edge.
	ErrLocked = errors.New("playbook: not unlocked yet")
)

/*
Environment is one context a rule set is tagged to.

§3.15 defines an environment by its people rather than its geography, so what
this names is a **population**: the set of characters a player lives among, which
is what makes one set of answers right and another set of the same answers
wrong. internal/environment is what authors them and internal/api is what binds
them; this package holds a key and a name and no opinion about either.
*/
type Environment struct {
	Key  string
	Name string
}

/*
Where is the set of environments this playbook has rule sets for.

An interface, and for the same reason internal/factor's is: which catalogue
defines a population is not this package's business, and a test wants two
invented ones. It is deliberately not a per-player question — every player has a
set for every environment, including the ones they have never been to, because
§3.15's story is that you arrive somewhere and find your old answers waiting to
fail.
*/
type Where interface {
	Environments() []Environment
}

// Nowhere is a Where for a world with one environment, for tests. Never bound
// in the server.
type Nowhere struct{}

func (Nowhere) Environments() []Environment {
	return []Environment{{Key: "nowhere", Name: "Nowhere in particular"}}
}

/*
Ref is one part of a sentence: which term, and which of its numbers.

The zero value is a blank — a part the player has not filled in — and a blank is
a first-class state rather than an error. It is what replaces a save button: a
sentence with a hole in it is not covering its question, which is a thing the
game says out loud rather than a thing it hides.
*/
type Ref struct {
	// Term is a key inside one pool of one question, not a global name.
	Term string
	// Value is a key from the term's closed list, empty for a term that takes
	// no number.
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

// MaxConditions is the ceiling §3.14 sets: two terms is as much as a rule can
// carry and still read as a sentence. Three with nesting — `(A and B) or C` —
// is deferred, and the table has nowhere to put it.
const MaxConditions = 2

/*
Sentence is a composed rule, holes and all.

`When` has one or two entries. One entry is a complete sentence; two entries with
the second still blank is a player mid-edit, and it is *not* in force — which is
why the count is stored rather than inferred from whether the second slot is
filled.
*/
type Sentence struct {
	When []Ref
	Join Join
	Then Ref
	Else Ref
}

// Complete reports whether every part of the sentence has been filled in.
//
// Nothing partial ever runs (§3.14). A rule with any blank in it is not in
// force, and the question it belongs to goes on escalating to the player.
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

// Set is every answer in force in one environment — §3.15's context-tagged rule
// set. It carries one rule per question, blank ones included, because a question
// with no answer is the state the game has the most to say about.
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

/*
Playbook is the whole of it for one player: the vocabulary they may compose
from, and every rule set they hold.

`Questions` is already veiled. A term the player has not unlocked is *absent*
from the pools here — not blanked, not greyed, not counted — so nothing
downstream has to remember to hide it, and nothing on the wire hints that there
is more to find in a particular place.
*/
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
Of reads a player's whole playbook.

A player with no rows is not an error: they are somebody who has answered
nothing, which is the state §3.14's loop begins in. What comes back is complete
either way — every environment, every question — so nothing downstream has to
decide what an absent row means.

Every stored part is re-resolved against the vocabulary as it is read. A term
that has since been renamed out of content, or that this player turns out not to
have, is read back as a *blank* rather than as itself: the rule stops being in
force and the question comes to the player again, which is the safe direction for
a mistake to fall in.
*/
func (m *Playbooks) Of(ctx context.Context, tx pgx.Tx, accountID string) (Playbook, error) {
	unlocked, err := m.unlocked(ctx, tx, accountID)
	if err != nil {
		return Playbook{}, err
	}
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
	for _, q := range questions {
		out.Questions = append(out.Questions, q.only(unlocked))
	}

	for _, env := range environments {
		set := Set{Environment: env, Rules: make([]Rule, 0, len(questions))}
		for _, q := range questions {
			rule := Rule{Question: q.Key}
			if held, ok := stored[key{environment: env.Key, question: q.Key}]; ok {
				rule.Sentence = resolve(q, held, unlocked)
			} else {
				rule.Sentence = opening(q)
			}
			set.Rules = append(set.Rules, rule)
		}
		out.Sets = append(out.Sets, set)
	}
	return out, nil
}

/*
InForce is every complete rule for one environment, in the order the questions
are asked.

The seam the matcher (#19) reads. Incomplete rules are not here at all — the
caller never has to ask whether a sentence with a hole in it counts — and the
order is authored, which is what makes "the first one checked wins" a statement
somebody can be shown when two rules match the same moment.
*/
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
Compose writes one question's sentence in one environment.

The whole sentence arrives, blanks included, and the whole of it is written.
There is no per-part command, and there is no save: the player changes a word and
that is what their character does from the next time the situation comes up. A
partial write would leave the stored sentence meaning something nobody composed.

Every named term is checked against *that question's* pool and against what this
player has unlocked, which is the same check twice for two different reasons —
the first is that vocabularies are scoped per question, and the second is that a
term nobody has been offered must be unusable rather than merely unlisted.
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

	unlocked, err := m.unlocked(ctx, tx, accountID)
	if err != nil {
		return Playbook{}, err
	}
	for _, w := range s.When {
		if err := check(q, Condition, w, unlocked); err != nil {
			return Playbook{}, err
		}
	}
	if err := check(q, Action, s.Then, unlocked); err != nil {
		return Playbook{}, err
	}
	if err := check(q, Action, s.Else, unlocked); err != nil {
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
Clear takes one question's answer back.

Not the same as composing a sentence of blanks, and the difference is the reason
this is a second entry point rather than a client sending four nulls. A stored
row of blanks is a rule the player wrote and left unfinished; no row at all is a
question they have never answered, which is the state [opening] describes and the
one a fresh player is in. Both are out of force, and only one of them is true.

Clearing a question that was never answered is not an error. It is the state the
caller asked for, and a refusal would make "take this back" depend on knowing
whether anything was there — which is exactly what the player is trying to stop
having to know.
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

/*
Unlock records that a player has earned words of the vocabulary.

§3.6's struggle-gating: a term arrives because the player went through the thing
it is an answer to, not because they reached a level. There is deliberately no
command for it and no way to ask — the modules that own the experiences call
this inside their own transaction, the way internal/factor's Record is called.

Unknown ids are refused rather than stored, because an unlock nobody can ever
spend is indistinguishable from a bug for as long as it sits in the table.
*/
func (m *Playbooks) Unlock(ctx context.Context, tx pgx.Tx, accountID string, ids ...string) error {
	for _, id := range ids {
		if _, ok := vocabulary.Term(id); !ok {
			return fmt.Errorf("%w: %q", ErrNoSuchTerm, id)
		}
	}
	for _, id := range ids {
		if _, err := tx.Exec(ctx, `
			INSERT INTO playbook_unlock (account_id, term) VALUES ($1, $2)
			ON CONFLICT (account_id, term) DO NOTHING`, accountID, id,
		); err != nil {
			return fmt.Errorf("playbook: unlock %q: %w", id, err)
		}
	}
	return nil
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

// unlocked is every term this player may compose with: the vocabulary everybody
// starts with, plus what they have earned.
//
// The starting terms are read out of content rather than written into the table
// on first sight, which is what lets an authored change to the opening
// vocabulary reach players who already exist. The table holds only what was
// *earned*, which is exactly what "tracked per player" has to mean for it to be
// worth a table at all.
func (m *Playbooks) unlocked(ctx context.Context, tx pgx.Tx, accountID string) (map[string]bool, error) {
	out := vocabulary.starting()

	rows, err := tx.Query(ctx, `SELECT term FROM playbook_unlock WHERE account_id = $1`, accountID)
	if err != nil {
		return nil, fmt.Errorf("playbook: read unlocks: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("playbook: read unlocks: %w", err)
		}
		// A term renamed out of content since it was earned is dropped rather
		// than carried, so a stale row cannot keep a word alive that no longer
		// exists.
		if _, ok := vocabulary.Term(id); ok {
			out[id] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("playbook: read unlocks: %w", err)
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
opening is the sentence a question nobody has answered starts as.

One hole in the condition, and the first action written in. Every one of these
questions has an obvious opening move, and a sentence with three blanks in it is
a form again — the point of composing in sentences is that the thing being edited
stays readable while it is being edited.

It is not stored. A question with no row has never been answered, and the moment
the player changes a word the whole sentence is written as they left it — which
is why an *edited* rule is read back exactly as stored, blanks and all, and never
re-defaulted underneath them.
*/
func opening(q Question) Sentence {
	return Sentence{When: []Ref{{}}, Join: And, Then: Ref{Term: q.actions[0].Key}}
}

// resolve turns stored columns into a sentence, checking every part against the
// vocabulary and against what this player has.
func resolve(q Question, h held, unlocked map[string]bool) Sentence {
	n := h.conditions
	if n < 1 || n > MaxConditions {
		n = 1
	}
	out := Sentence{
		When: make([]Ref, 0, n),
		Join: h.join,
		Then: settled(q, Action, h.then, unlocked),
		Else: settled(q, Action, h.otherwise, unlocked),
	}
	if out.Join != And && out.Join != Or {
		out.Join = And
	}
	for i := 0; i < n; i++ {
		out.When = append(out.When, settled(q, Condition, h.when[i], unlocked))
	}
	return out
}

// settled is one stored part, read back as itself or as a blank.
func settled(q Question, kind Kind, r Ref, unlocked map[string]bool) Ref {
	if r.Blank() {
		return Ref{}
	}
	t, ok := q.Term(kind, r.Term)
	if !ok || !(t.Start || unlocked[t.ID()]) {
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
func check(q Question, kind Kind, r Ref, unlocked map[string]bool) error {
	if r.Blank() {
		// A blank is allowed and is not an omission: it is the state the rule
		// is in while it is being written, and it is what keeps it out of force.
		if r.Value != "" {
			return fmt.Errorf("%w: a number with no term to hold it", ErrNoSuchValue)
		}
		return nil
	}
	t, ok := q.Term(kind, r.Term)
	if !ok {
		return fmt.Errorf("%w: %q is not a %s of %q", ErrNoSuchTerm, r.Term, kind, q.Key)
	}
	if !t.Start && !unlocked[t.ID()] {
		return fmt.Errorf("%w: %q", ErrLocked, t.ID())
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
