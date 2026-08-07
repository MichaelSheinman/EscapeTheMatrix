package playbook

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store/storetest"
)

/*
The module, against a real Postgres (§14).

What is worth proving here is not that a row round-trips. It is the rules the
ticket is actually about: a rule is a sentence composed from a scoped
vocabulary, nothing partial is in force, the sets are per environment, a term
nobody has earned is unusable *and* unlisted, and there is one copy of a rule
rather than a draft and a committed one.
*/

// two environments, because one would let every set-scoping bug through.
type where struct{}

func (where) Environments() []Environment {
	return []Environment{{Key: "east-side", Name: "East Side"}, {Key: "downtown", Name: "Downtown"}}
}

// with runs f inside one transaction on a fresh database, which is the shape
// every command has (§3.2) and therefore the shape every test should have.
//
// The transaction is rolled back by a cleanup rather than by a closure around f,
// because a t.Fatalf inside f ends the goroutine outright — a closure would never
// return and the connection would be held until the pool gave up.
func with(t *testing.T, f func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string)) {
	t.Helper()
	st := storetest.New(t)
	ctx := t.Context()

	tx, err := st.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	f(ctx, tx, New(where{}), newAccount(t, ctx, tx))
}

// newAccount makes somebody for the rules to belong to. Both tables reference
// account, so there has to be one.
func newAccount(t *testing.T, ctx context.Context, tx pgx.Tx) string {
	t.Helper()
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO account (provider, subject, email, display_name)
		VALUES ('google', 'playbook-test', 'playbook@example.com', 'Playbook')
		RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("create account: %v", err)
	}
	return id
}

// set finds one rule set in a playbook.
func set(t *testing.T, p Playbook, environment string) Set {
	t.Helper()
	for _, s := range p.Sets {
		if s.Environment.Key == environment {
			return s
		}
	}
	t.Fatalf("there is no rule set for %q", environment)
	return Set{}
}

// rule finds one question's answer in one environment.
func rule(t *testing.T, p Playbook, environment, question string) Rule {
	t.Helper()
	r, ok := set(t, p, environment).Rule(question)
	if !ok {
		t.Fatalf("%q has no rule for %q", environment, question)
	}
	return r
}

// A legal starting sentence for `help`, composed only from terms everybody has.
func startingHelp() Sentence {
	return Sentence{
		When: []Ref{{Term: "fee", Value: "fair"}},
		Join: And,
		Then: Ref{Term: "yes"},
		Else: Ref{Term: "no"},
	}
}

/*
A player who has answered nothing still has a whole playbook: one set per
environment, one rule per question, none of them in force.

§3.15 is why the sets exist before the player does anything — you arrive
somewhere and find your old answers waiting to fail, and a set that appeared on
arrival would turn that into a feature announcement.
*/
func TestAFreshPlaybookHasASetPerEnvironmentAndNothingInForce(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		book, err := m.Of(ctx, tx, account)
		if err != nil {
			t.Fatalf("Of: %v", err)
		}

		if len(book.Sets) != len(where{}.Environments()) {
			t.Fatalf("%d rule sets, want one per environment", len(book.Sets))
		}
		for _, s := range book.Sets {
			if len(s.Rules) != len(Vocab().Questions()) {
				t.Fatalf("%q holds %d rules, want one per standing question", s.Environment.Key, len(s.Rules))
			}
			for _, r := range s.Rules {
				if r.Complete() {
					t.Errorf("%s/%s is in force before anybody answered it", s.Environment.Key, r.Question)
				}
			}
		}

		// The opening sentence: one hole where the condition goes, and the
		// obvious first move already written in. A sentence with three blanks
		// in it is a form again.
		opened := rule(t, book, "east-side", "help")
		if len(opened.When) != 1 || !opened.When[0].Blank() {
			t.Errorf("an unanswered question opens with when = %+v", opened.When)
		}
		if opened.Then.Blank() || !opened.Else.Blank() {
			t.Errorf("an unanswered question opens then=%+v otherwise=%+v", opened.Then, opened.Else)
		}
	})
}

// The whole loop, once: compose a complete sentence and it is what the
// character does.
func TestAComposedSentenceIsInForceWithNoSaveStep(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		book, err := m.Compose(ctx, tx, account, "east-side", "help", startingHelp())
		if err != nil {
			t.Fatalf("Compose: %v", err)
		}

		got := rule(t, book, "east-side", "help")
		if !got.Complete() {
			t.Fatalf("the composed rule is not in force: %+v", got.Sentence)
		}
		if got.When[0].Term != "fee" || got.When[0].Value != "fair" || got.Then.Term != "yes" {
			t.Errorf("read back as %+v", got.Sentence)
		}

		// And it is what the matcher will be handed.
		inForce, err := m.InForce(ctx, tx, account, "east-side")
		if err != nil {
			t.Fatalf("InForce: %v", err)
		}
		if len(inForce) != 1 || inForce[0].Question != "help" {
			t.Fatalf("InForce = %+v, want the one answered question", inForce)
		}
	})
}

/*
Editing the sentence is the change: there is one copy of a rule, and the second
edit replaces the first rather than queueing behind it.
*/
func TestEditingReplacesRatherThanQueueing(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		if _, err := m.Compose(ctx, tx, account, "east-side", "help", startingHelp()); err != nil {
			t.Fatalf("Compose: %v", err)
		}

		tighter := startingHelp()
		tighter.When[0].Value = "serious"
		book, err := m.Compose(ctx, tx, account, "east-side", "help", tighter)
		if err != nil {
			t.Fatalf("Compose again: %v", err)
		}
		if got := rule(t, book, "east-side", "help"); got.When[0].Value != "serious" {
			t.Errorf("the edit read back as %+v", got.Sentence)
		}

		var rows int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM playbook_rule WHERE account_id = $1`, account,
		).Scan(&rows); err != nil {
			t.Fatalf("count rules: %v", err)
		}
		if rows != 1 {
			t.Errorf("%d rows, want one; there is no draft beside the committed copy", rows)
		}
	})
}

/*
A second condition that is still a hole keeps the rule out of force.

The distinction the stored count exists for: one condition is a complete
sentence, and two with the second blank is a player mid-edit whose rule must not
fire on the first alone.
*/
func TestASecondConditionStillBlankKeepsTheRuleOutOfForce(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		half := startingHelp()
		half.When = append(half.When, Ref{})

		book, err := m.Compose(ctx, tx, account, "east-side", "help", half)
		if err != nil {
			t.Fatalf("Compose: %v", err)
		}
		got := rule(t, book, "east-side", "help")
		if len(got.When) != 2 {
			t.Fatalf("read back with %d conditions", len(got.When))
		}
		if got.Complete() {
			t.Error("a sentence with a hole in it is in force")
		}

		inForce, err := m.InForce(ctx, tx, account, "east-side")
		if err != nil {
			t.Fatalf("InForce: %v", err)
		}
		if len(inForce) != 0 {
			t.Errorf("InForce = %+v, want nothing partial", inForce)
		}

		// Filling it in is the change, and nothing else has to happen.
		half.When[1] = Ref{Term: "legal"}
		half.Join = Or
		book, err = m.Compose(ctx, tx, account, "east-side", "help", half)
		if err != nil {
			t.Fatalf("Compose: %v", err)
		}
		if got := rule(t, book, "east-side", "help"); !got.Complete() || got.Join != Or {
			t.Errorf("filled in as %+v", got.Sentence)
		}
	})
}

// §3.15: the rules are not one list. Answering here does not answer there.
func TestRuleSetsAreScopedToTheirEnvironment(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		book, err := m.Compose(ctx, tx, account, "east-side", "help", startingHelp())
		if err != nil {
			t.Fatalf("Compose: %v", err)
		}

		if !rule(t, book, "east-side", "help").Complete() {
			t.Fatal("the rule is not in force where it was composed")
		}
		if rule(t, book, "downtown", "help").Complete() {
			t.Error("composing on the east side answered the question downtown too")
		}

		elsewhere, err := m.InForce(ctx, tx, account, "downtown")
		if err != nil {
			t.Fatalf("InForce: %v", err)
		}
		if len(elsewhere) != 0 {
			t.Errorf("downtown holds %+v", elsewhere)
		}
	})
}

/*
The vocabulary is scoped per question, and this is where that stops being a
description of the content files.

A condition that exists — under another question — is refused here exactly as
one that exists nowhere.
*/
func TestAVocabularyIsScopedToItsOwnQuestion(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		borrowed := startingHelp()
		borrowed.When[0] = Ref{Term: "watching"} // voice's, not help's

		_, err := m.Compose(ctx, tx, account, "east-side", "help", borrowed)
		if !errors.Is(err, ErrNoSuchTerm) {
			t.Errorf("err = %v, want ErrNoSuchTerm", err)
		}
	})
}

// Every way of getting a sentence wrong, and the refusal it earns.
func TestNothingOutsideTheVocabularyCanBeComposed(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		cases := []struct {
			name string
			of   func(Sentence) Sentence
			want error
		}{
			{"a term nobody authored", func(s Sentence) Sentence {
				s.When[0] = Ref{Term: "vibes"}
				return s
			}, ErrNoSuchTerm},
			{"a number outside the closed list", func(s Sentence) Sentence {
				s.When[0].Value = "above $137"
				return s
			}, ErrNoSuchValue},
			{"no number for a term that takes one", func(s Sentence) Sentence {
				s.When[0].Value = ""
				return s
			}, ErrNoSuchValue},
			{"a number on a term that takes none", func(s Sentence) Sentence {
				s.When[0] = Ref{Term: "legal", Value: "fair"}
				return s
			}, ErrNoSuchValue},
			{"a number with no term to hold it", func(s Sentence) Sentence {
				s.Else = Ref{Value: "fair"}
				return s
			}, ErrNoSuchValue},
			{"an action used as a condition", func(s Sentence) Sentence {
				s.When[0] = Ref{Term: "yes"}
				return s
			}, ErrNoSuchTerm},
			{"a condition used as an action", func(s Sentence) Sentence {
				s.Then = Ref{Term: "legal"}
				return s
			}, ErrNoSuchTerm},
			{"a third condition", func(s Sentence) Sentence {
				s.When = append(s.When, Ref{Term: "legal"}, Ref{Term: "legal"})
				return s
			}, ErrConditions},
			{"no conditions at all", func(s Sentence) Sentence {
				s.When = nil
				return s
			}, ErrConditions},
			{"a joining word nobody authored", func(s Sentence) Sentence {
				s.Join = "unless"
				return s
			}, ErrNoSuchJoin},
			{"a question the world does not ask", func(s Sentence) Sentence { return s }, ErrNoSuchQuestion},
		}

		for _, c := range cases {
			question := "help"
			if c.want == ErrNoSuchQuestion {
				question = "the-meaning-of-it-all"
			}
			_, err := m.Compose(ctx, tx, account, "east-side", question, c.of(startingHelp()))
			if !errors.Is(err, c.want) {
				t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
			}
		}

		// And an environment that is not one.
		if _, err := m.Compose(ctx, tx, account, "atlantis", "help", startingHelp()); !errors.Is(err, ErrNoSuchEnvironment) {
			t.Errorf("err = %v, want ErrNoSuchEnvironment", err)
		}
	})
}

/*
§3.6, held in one place: a term the player has not earned is unusable, and it is
not listed either.

Both halves matter and they are different failures. A term that were merely
refused would still be visible, which spends the surprise; a term that were
merely hidden could still be composed by anybody who read the wire once.
*/
func TestALockedTermIsNeitherUsableNorListed(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		// `useful` is authored without `start`.
		locked := startingHelp()
		locked.When[0] = Ref{Term: "useful"}

		if _, err := m.Compose(ctx, tx, account, "east-side", "help", locked); !errors.Is(err, ErrLocked) {
			t.Fatalf("err = %v, want ErrLocked", err)
		}

		book, err := m.Of(ctx, tx, account)
		if err != nil {
			t.Fatalf("Of: %v", err)
		}
		if named(book, "help", Condition, "useful") {
			t.Error("a locked term is listed")
		}
		if !named(book, "help", Condition, "fee") {
			t.Error("a starting term is missing")
		}

		// Earning it is the only thing that changes, and it changes both.
		if err := m.Unlock(ctx, tx, account, "help/condition/useful"); err != nil {
			t.Fatalf("Unlock: %v", err)
		}
		if book, err = m.Compose(ctx, tx, account, "east-side", "help", locked); err != nil {
			t.Fatalf("Compose after unlocking: %v", err)
		}
		if !named(book, "help", Condition, "useful") {
			t.Error("an earned term is still not listed")
		}
		if !rule(t, book, "east-side", "help").Complete() {
			t.Error("the rule composed from an earned term is not in force")
		}
	})
}

// An unlock naming nothing is refused rather than stored: a row nobody can ever
// spend is indistinguishable from a bug for as long as it sits there.
func TestUnlockingSomethingThatIsNotATermIsRefused(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		if err := m.Unlock(ctx, tx, account, "help/condition/telepathy"); !errors.Is(err, ErrNoSuchTerm) {
			t.Fatalf("err = %v, want ErrNoSuchTerm", err)
		}
		var rows int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM playbook_unlock WHERE account_id = $1`, account,
		).Scan(&rows); err != nil {
			t.Fatalf("count unlocks: %v", err)
		}
		if rows != 0 {
			t.Errorf("%d unlocks stored", rows)
		}
	})
}

/*
The ceiling is the shape of the table, not a check in Go.

Both halves are asserted through SQL that bypasses this package entirely,
because "a rule a bug can skip is a rule the design does not have" is only true
if the database refuses it.

Inside a savepoint, because a constraint violation aborts the whole transaction
and a second bare check would then fail for the wrong reason.
*/
func TestTheTableCannotHoldASentenceTheDesignForbids(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		refuses := func(name, sql string, args ...any) {
			t.Helper()
			inner, err := tx.Begin(ctx)
			if err != nil {
				t.Fatalf("savepoint: %v", err)
			}
			defer func() { _ = inner.Rollback(ctx) }()
			if _, err := inner.Exec(ctx, sql, args...); err == nil {
				t.Errorf("%s was accepted", name)
			}
		}

		const insert = `
			INSERT INTO playbook_rule (account_id, environment, question, conditions, join_word,
				when_1_term, when_1_value, when_2_term, when_2_value, then_term, else_term)
			VALUES ($1, 'east-side', 'help', $2, $3, 'legal', NULL, $4, NULL, 'yes', 'no')`

		refuses("a third condition", insert, account, 3, "and", "legal")
		refuses("a second condition in a one-condition sentence", insert, account, 1, "and", "legal")
		refuses("a joining word nobody authored", insert, account, 2, "unless", "legal")
		refuses("a number with no term to hold it", `
			INSERT INTO playbook_rule (account_id, environment, question, conditions, join_word, when_1_value)
			VALUES ($1, 'east-side', 'help', 1, 'and', 'fair')`, account)
	})
}

/*
Content moves, and a player standing on the old version has to be able to walk
out of it.

A rule naming a term that no longer exists reads back as a blank — so the rule
stops being in force and the question comes to the player again, which is the
safe direction for that mistake to fall in.
*/
func TestARuleNamingATermContentNoLongerHasReadsBackAsABlank(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		if _, err := m.Compose(ctx, tx, account, "east-side", "help", startingHelp()); err != nil {
			t.Fatalf("Compose: %v", err)
		}
		// The content edit, simulated at the only place it would show.
		if _, err := tx.Exec(ctx, `
			UPDATE playbook_rule SET when_1_term = 'retired', when_1_value = NULL
			 WHERE account_id = $1`, account); err != nil {
			t.Fatalf("retire the term: %v", err)
		}

		book, err := m.Of(ctx, tx, account)
		if err != nil {
			t.Fatalf("Of: %v", err)
		}
		got := rule(t, book, "east-side", "help")
		if !got.When[0].Blank() || got.Complete() {
			t.Errorf("read back as %+v, want a blank and nothing in force", got.Sentence)
		}
	})
}

// named reports whether one term is in the vocabulary this player was handed.
func named(p Playbook, question string, kind Kind, key string) bool {
	for _, q := range p.Questions {
		if q.Key != question {
			continue
		}
		_, ok := q.Term(kind, key)
		return ok
	}
	return false
}

/*
Taking an answer back is not the same as writing an empty one.

Both are out of force, and only one of them is true. A cleared question is one
nobody has answered — which is the state a fresh player is in, opening action and
all — while a sentence composed of blanks is somebody mid-edit. The distinction
survives a round trip through the table, and it is the reason there are two entry
points rather than one command with four nulls in it.
*/
func TestClearingAnAnswerIsNotTheSameAsBlankingIt(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		if _, err := m.Compose(ctx, tx, account, "east-side", "help", startingHelp()); err != nil {
			t.Fatalf("Compose: %v", err)
		}

		// Blanked: the row is still there, and the sentence is as the player left
		// it — every part empty, including the action a fresh question opens with.
		blank := Sentence{When: []Ref{{}}, Join: And}
		book, err := m.Compose(ctx, tx, account, "east-side", "help", blank)
		if err != nil {
			t.Fatalf("Compose a blank: %v", err)
		}
		if got := rule(t, book, "east-side", "help"); got.Complete() || !got.Then.Blank() {
			t.Fatalf("blanked rule = %+v, want a sentence with nothing in it", got.Sentence)
		}
		if rows := stored(t, ctx, tx, account); rows != 1 {
			t.Fatalf("%d rows after blanking; a blanked rule is still a rule", rows)
		}

		// Cleared: the row is gone, and the question reads as one nobody has
		// answered — which is the opening sentence rather than a hole.
		book, err = m.Clear(ctx, tx, account, "east-side", "help")
		if err != nil {
			t.Fatalf("Clear: %v", err)
		}
		got := rule(t, book, "east-side", "help")
		if got.Complete() || got.Then.Blank() {
			t.Fatalf("cleared rule = %+v, want the opening sentence back", got.Sentence)
		}
		if rows := stored(t, ctx, tx, account); rows != 0 {
			t.Errorf("%d rows after clearing", rows)
		}
	})
}

/*
Clearing is scoped and forgiving in the two ways that matter.

It reaches one question in one environment — §3.15's whole mechanism is that the
same question has different answers in different places, and a clear that took
the answer everywhere would be a way to lose a week's work with one click. And
clearing something nobody ever answered is not a refusal: it is the state the
caller asked for either way, and refusing would make taking an answer back depend
on remembering whether it was there.
*/
func TestClearingReachesOneQuestionInOneEnvironment(t *testing.T) {
	t.Parallel()
	with(t, func(ctx context.Context, tx pgx.Tx, m *Playbooks, account string) {
		for _, environment := range []string{"east-side", "downtown"} {
			if _, err := m.Compose(ctx, tx, account, environment, "help", startingHelp()); err != nil {
				t.Fatalf("Compose in %q: %v", environment, err)
			}
		}

		book, err := m.Clear(ctx, tx, account, "east-side", "help")
		if err != nil {
			t.Fatalf("Clear: %v", err)
		}
		if rule(t, book, "east-side", "help").Complete() {
			t.Error("the answer was not cleared where it was asked for")
		}
		if !rule(t, book, "downtown", "help").Complete() {
			t.Error("clearing on the east side cleared downtown too (§3.15)")
		}

		// Twice over, and on a question that never had an answer.
		if _, err := m.Clear(ctx, tx, account, "east-side", "help"); err != nil {
			t.Errorf("clearing an already-clear question: %v", err)
		}
		if _, err := m.Clear(ctx, tx, account, "east-side", "money"); err != nil {
			t.Errorf("clearing a question nobody answered: %v", err)
		}

		if _, err := m.Clear(ctx, tx, account, "atlantis", "help"); !errors.Is(err, ErrNoSuchEnvironment) {
			t.Errorf("clearing nowhere = %v", err)
		}
		if _, err := m.Clear(ctx, tx, account, "east-side", "love"); !errors.Is(err, ErrNoSuchQuestion) {
			t.Errorf("clearing a question the world does not ask = %v", err)
		}
	})
}

// stored counts this player's rule rows, which is the only way to tell a blanked
// answer from a cleared one from outside the module.
func stored(t *testing.T, ctx context.Context, tx pgx.Tx, account string) int {
	t.Helper()
	var n int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM playbook_rule WHERE account_id = $1`, account).Scan(&n); err != nil {
		t.Fatalf("count rules: %v", err)
	}
	return n
}
