package api

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/playbook"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
)

/*
The playbook on the wire (§3.14).

Three properties of this file are the ticket rather than the plumbing.

**Nothing here carries prose.** Every field a client may send is a key out of a
list the server just gave it — a question, a term inside that question's pool, a
number out of that term's closed list, and one of two joining words. There is no
string field a player could type into, which is what makes "never free text" a
shape rather than a validation rule.

**A locked term is absent, not hidden.** internal/playbook veils the vocabulary
on the way out, so the pools below contain only what this player has unlocked.
Nothing on this wire says how many terms there are, which of them are missing, or
that any are — a client cannot render a blanked-out row because there is no row.

**There is no save.** `playbook.compose` writes the whole sentence, blanks
included, and `inForce` is a statement about what was written rather than about a
second copy of it. A rule with a hole in it is stored exactly as composed and is
simply not covering its question, which is the state the screen says out loud.
*/

// playbookValueView is one number a term may hold. The amount behind it stays
// on the server: a client that could do arithmetic with a threshold is a client
// that could predict the matcher, and §10.2 says the client computes no rules.
type playbookValueView struct {
	Key  string `json:"key"`
	Says string `json:"says"`
}

/*
playbookTermView is one word of the vocabulary.

`says` keeps its `{value}` placeholder rather than arriving with the number
already written in, because the builder puts a menu *inside* the sentence —
"they are paying me [above $60]" — and a finished string would have to be taken
apart again to do that.
*/
type playbookTermView struct {
	Key  string `json:"key"`
	Says string `json:"says"`
	// Values is empty for a term that takes no number. Inlined per term rather
	// than sent once as a shared table: a list of thresholds beside a term
	// nobody has unlocked would say that the term exists.
	Values []playbookValueView `json:"values"`
}

// playbookQuestionView is one standing question and the whole vocabulary this
// player may answer it from.
type playbookQuestionView struct {
	Key string `json:"key"`
	// N is the question's authored number, which is how the game refers to an
	// answer in a log line ("answer 1 fired"). Sent rather than derived from
	// position, because a client that numbered a list would renumber every
	// answer the day a question is inserted.
	N     int    `json:"n"`
	Ask   string `json:"ask"`
	Note  string `json:"note"`
	About string `json:"about"`

	Conditions []playbookTermView `json:"conditions"`
	Actions    []playbookTermView `json:"actions"`
}

// refView is one part of a sentence, or null for a part still blank.
type refView struct {
	Term string `json:"term"`
	// Value is empty for a term that takes no number.
	Value string `json:"value"`
}

/*
ruleView is one question's answer inside one rule set.

`when` has one or two entries and either may be null. Two entries with the second
null is a player mid-edit — a different state from one entry, which is a complete
sentence — and the distinction is the whole reason the count is stored.
*/
type ruleView struct {
	Question  string     `json:"question"`
	When      []*refView `json:"when"`
	Join      string     `json:"join"`
	Then      *refView   `json:"then"`
	Otherwise *refView   `json:"otherwise"`
	// InForce is whether anything is covering this question. Stated by the
	// server rather than worked out here, because "nothing partial ever runs"
	// is a rule and rules are the server's (§10.2).
	InForce bool `json:"inForce"`
}

// ruleSetView is one environment's rules — §3.15's context-tagged set.
type ruleSetView struct {
	Environment string     `json:"environment"`
	Name        string     `json:"name"`
	Rules       []ruleView `json:"rules"`
}

type playbookView struct {
	Questions []playbookQuestionView `json:"questions"`
	Sets      []ruleSetView          `json:"sets"`
}

func playbookOf(p playbook.Playbook) playbookView {
	out := playbookView{
		Questions: make([]playbookQuestionView, 0, len(p.Questions)),
		Sets:      make([]ruleSetView, 0, len(p.Sets)),
	}
	for _, q := range p.Questions {
		out.Questions = append(out.Questions, playbookQuestionView{
			Key: q.Key, N: q.Order, Ask: q.Ask, Note: q.Note, About: q.About,
			Conditions: terms(q.Conditions()),
			Actions:    terms(q.Actions()),
		})
	}
	for _, set := range p.Sets {
		view := ruleSetView{
			Environment: set.Environment.Key,
			Name:        set.Environment.Name,
			Rules:       make([]ruleView, 0, len(set.Rules)),
		}
		for _, r := range set.Rules {
			rule := ruleView{
				Question:  r.Question,
				When:      make([]*refView, 0, len(r.When)),
				Join:      string(r.Join),
				Then:      ref(r.Then),
				Otherwise: ref(r.Else),
				InForce:   r.Complete(),
			}
			for _, w := range r.When {
				rule.When = append(rule.When, ref(w))
			}
			view.Rules = append(view.Rules, rule)
		}
		out.Sets = append(out.Sets, view)
	}
	return out
}

func terms(pool []playbook.Term) []playbookTermView {
	out := make([]playbookTermView, 0, len(pool))
	for _, t := range pool {
		view := playbookTermView{Key: t.Key, Says: t.Says, Values: []playbookValueView{}}
		if t.Takes() {
			for _, v := range t.Values.Options() {
				view.Values = append(view.Values, playbookValueView{Key: v.Key, Says: v.Says})
			}
		}
		out = append(out, view)
	}
	return out
}

func ref(r playbook.Ref) *refView {
	if r.Blank() {
		return nil
	}
	return &refView{Term: r.Term, Value: r.Value}
}

func composed(r *refView) playbook.Ref {
	if r == nil {
		return playbook.Ref{}
	}
	return playbook.Ref{Term: r.Term, Value: r.Value}
}

// What a player's other tabs are told when a rule changes.
type playbookComposed struct {
	Playbook playbookView `json:"playbook"`
}

var playbookComposedMessage = realtime.Define[playbookComposed]("playbook.composed")

/*
Composing a rule.

The whole sentence arrives and the whole of it is written. There is no per-part
command, because a client that could set one word at a time could leave the
stored rule in a state the player never composed — and no save command, because
there is nothing to save: this *is* the edit, and the character behaves this way
from the next time the situation comes up.

Every refusal that is about the vocabulary is a 422 and stays one however often
it is sent — a term that is not in this question's pool will never be. The one
409 is a term that exists and that this player has not unlocked yet (§3.6): the
request is fine, and the same command works once they have been through what
unlocks it.
*/
type composeInput struct {
	Environment string     `json:"environment"`
	Question    string     `json:"question"`
	When        []*refView `json:"when"`
	Join        string     `json:"join"`
	Then        *refView   `json:"then"`
	Otherwise   *refView   `json:"otherwise"`
}

type composeResult struct {
	Playbook playbookView `json:"playbook"`
}

func (s *Server) composeRule(ctx context.Context, tx pgx.Tx, actor command.Actor, in composeInput) (composeResult, error) {
	sentence := playbook.Sentence{
		Join: playbook.Join(in.Join),
		Then: composed(in.Then),
		Else: composed(in.Otherwise),
	}
	for _, w := range in.When {
		sentence.When = append(sentence.When, composed(w))
	}

	book, err := s.playbooks.Compose(ctx, tx, actor.ID, in.Environment, in.Question, sentence)
	switch {
	case errors.Is(err, playbook.ErrNoSuchEnvironment):
		return composeResult{}, command.Invalid("there is no rule set for that environment")
	case errors.Is(err, playbook.ErrNoSuchQuestion):
		return composeResult{}, command.Invalid("the world does not ask that question")
	case errors.Is(err, playbook.ErrNoSuchTerm):
		// Deliberately the same sentence a locked term would get if it leaked
		// this far, and it cannot: what is not in the pool and what is not
		// unlocked are told apart above this line and never below it.
		return composeResult{}, command.Invalid("that is not one of the things this question can say")
	case errors.Is(err, playbook.ErrNoSuchValue):
		return composeResult{}, command.Invalid("that is not one of the numbers that term takes")
	case errors.Is(err, playbook.ErrNoSuchJoin):
		return composeResult{}, command.Invalid("two conditions are joined by and, or by or")
	case errors.Is(err, playbook.ErrConditions):
		return composeResult{}, command.Invalid("a rule tests one or two conditions")
	case errors.Is(err, playbook.ErrLocked):
		return composeResult{}, command.Conflict("you have not learned to say that yet")
	case err != nil:
		return composeResult{}, err
	}

	out := composeResult{Playbook: playbookOf(book)}
	if err := playbookComposedMessage.Publish(ctx, tx,
		realtime.To{Account: actor.ID, Origin: actor.Session},
		playbookComposed(out),
	); err != nil {
		return composeResult{}, err
	}
	return out, nil
}

/*
districts answers playbook.Where: which environments have rule sets.

The same substitution internal/factor's environment binding makes, and for the
same reason (§3.15 defines an environment by its people, #29 brings them). It
lives here rather than in internal/playbook because it is a composition of two
modules, which is what the transport layer is for — and the day #29 lands, this
binding is replaced rather than edited.

Every district, not only the ones the player has been to. §3.15's story is that
you arrive somewhere and find your old answers waiting to fail, and a set that
appeared on arrival would turn that into a feature announcement.
*/
type districts struct{}

func (districts) Environments() []playbook.Environment {
	all := city.Places()
	out := make([]playbook.Environment, 0, len(all))
	for _, p := range all {
		if p.Layer != city.District {
			continue
		}
		out = append(out, playbook.Environment{Key: p.Key, Name: p.Name})
	}
	return out
}
