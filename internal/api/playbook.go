package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/conflict"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/environment"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/playbook"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/situation"
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

/*
conflictView is two answers that could disagree about the same moment.

technical.md §6's fourth argument for composed rules over written ones — "rule
conflicts are statically detectable" — arriving on the wire. It is sent beside
the rules rather than fetched, because it is a statement about the set the player
is looking at and it changes the instant they change a word.

`says` is written here for the same reason a log line is: a client that
assembled this sentence would be deciding what a conflict means, and the first
time the two disagreed the player would be reading something the matcher never
did. The keys are sent as well so the screen can point at the two answers.
*/
type conflictView struct {
	// Questions is the two, in authored order, and N is what they are called.
	Questions []string `json:"questions"`
	N         []int    `json:"n"`
	// Always is the pair that disagrees whichever way each of them falls — a
	// moment the player has answered twice, rather than one they have not
	// thought about yet.
	Always bool   `json:"always"`
	Says   string `json:"says"`
}

// ruleSetView is one environment's rules — §3.15's context-tagged set.
type ruleSetView struct {
	Environment string     `json:"environment"`
	Name        string     `json:"name"`
	Rules       []ruleView `json:"rules"`
	// What the set says about itself with no moment in hand. Empty for a set
	// nothing is wrong with, which is the ordinary case and the one the screen
	// has to read as ordinary.
	Conflicts []conflictView `json:"conflicts"`
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
	asked := map[string]playbook.Question{}
	for _, q := range p.Questions {
		asked[q.Key] = q
	}

	for _, set := range p.Sets {
		view := ruleSetView{
			Environment: set.Environment.Key,
			Name:        set.Environment.Name,
			Rules:       make([]ruleView, 0, len(set.Rules)),
			Conflicts:   []conflictView{},
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

		audit := set.Audit()
		for _, c := range audit.Conflicts {
			view.Conflicts = append(view.Conflicts, conflictView{
				Questions: []string{c.Questions[0], c.Questions[1]},
				N:         []int{c.N[0], c.N[1]},
				Always:    c.Always,
				Says:      clash(c, asked),
			})
		}

		out.Sets = append(out.Sets, view)
	}
	return out
}

/*
clash is a conflict, written out.

It says what will happen rather than that something is wrong. Nothing in this
game labels a choice good or bad (§3.2), and a pair of answers that contradict
each other is a legitimate thing to hold — what the player is owed is the
consequence: the character does nothing, and the moment comes to them.
*/
func clash(c playbook.Conflict, asked map[string]playbook.Question) string {
	when := "could disagree"
	if c.Always {
		when = "always disagree"
	}
	sentence := fmt.Sprintf("Your answers to %d and %d %s.", c.N[0], c.N[1], when)
	first, second := asked[c.Questions[0]].About, asked[c.Questions[1]].About
	if first != "" && second != "" {
		sentence += fmt.Sprintf(" A moment that is both — %s, %s — reaches you instead:"+
			" your character will not pick a side.", first, second)
	} else {
		sentence += " A moment that puts both reaches you instead:" +
			" your character will not pick a side."
	}
	return sentence
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
		return composeResult{}, command.Invalid("that is not one of the things this question can say")
	case errors.Is(err, playbook.ErrNoSuchValue):
		return composeResult{}, command.Invalid("that is not one of the numbers that term takes")
	case errors.Is(err, playbook.ErrNoSuchJoin):
		return composeResult{}, command.Invalid("two conditions are joined by and, or by or")
	case errors.Is(err, playbook.ErrConditions):
		return composeResult{}, command.Invalid("a rule tests one or two conditions")
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
Taking an answer back.

The other half of editing, and it is a command rather than a compose with every
part left null because those two mean different things. A stored sentence full of
blanks is a rule somebody wrote and did not finish; no rule at all is a question
they have never answered. Both are out of force and only one of them is true, and
a client that had to blank four parts one at a time to say "I take that back"
would leave the first meaning behind while it did.

There is no refusal for clearing a question nothing was written for. It is the
state the player asked for either way, and a 409 would make taking something back
depend on remembering whether it was there.
*/
type clearInput struct {
	Environment string `json:"environment"`
	Question    string `json:"question"`
}

type clearResult struct {
	Playbook playbookView `json:"playbook"`
}

func (s *Server) clearRule(ctx context.Context, tx pgx.Tx, actor command.Actor, in clearInput) (clearResult, error) {
	book, err := s.playbooks.Clear(ctx, tx, actor.ID, in.Environment, in.Question)
	switch {
	case errors.Is(err, playbook.ErrNoSuchEnvironment):
		return clearResult{}, command.Invalid("there is no rule set for that environment")
	case errors.Is(err, playbook.ErrNoSuchQuestion):
		return clearResult{}, command.Invalid("the world does not ask that question")
	case err != nil:
		return clearResult{}, err
	}

	out := clearResult{Playbook: playbookOf(book)}
	// The same message a compose publishes, because the other tabs are being told
	// the same thing: this is the playbook now. A second message type would make
	// them handle "one rule went away" as its own case, which is a rule about the
	// playbook living in a client (§10.2).
	if err := playbookComposedMessage.Publish(ctx, tx,
		realtime.To{Account: actor.ID, Origin: actor.Session},
		playbookComposed{Playbook: out.Playbook},
	); err != nil {
		return clearResult{}, err
	}
	return out, nil
}

/*
The log, which is what the rules did while the player was elsewhere (§3.14).

# Why this is a GET and not part of the world snapshot

It is history, and it grows for as long as somebody plays. §10.2 hydrates the
client's world model once and keeps it current over the socket, which works
because a world model is bounded — a year of moments is not, and putting the last
twenty of them in the snapshot would put the twenty-first out of reach forever.
So this is the second pull-shaped surface in the client, after the leaderboards,
and for the same reason each of them is one: a paginated list read when somebody
looks at it.

# What it does not carry

No amount and no stance. The client is told which condition was met and what the
character did, in words; it is not told the threshold behind a term or the posture
behind an action, because either would let a screen work out what the matcher was
going to do (§10.2: the client computes no game rules). Where the posture matters
— two answers that disagreed — the server says so in `note`, which is a sentence
rather than a pair of keys.
*/

// logPath is the record: this player's moments, newest first.
const logPath = "/api/playbook/log"

// consideredView is one question the moment put, and what the playbook made of
// it. `met` is per condition, in the sentence's own order, which is §3.14's
// "exactly which condition matched" made into a field rather than a claim.
type consideredView struct {
	Question string `json:"question"`
	N        int    `json:"n"`
	State    string `json:"state"`
	// Says is the rule as one sentence and Did is the action alone, both
	// rendered by the server: a screen that reassembled a rule out of parts
	// would be a second implementation of what a rule means.
	Says string   `json:"says"`
	Did  string   `json:"did"`
	Met  []string `json:"met"`
	Held string   `json:"held"`
}

type fireView struct {
	ID          int64  `json:"id"`
	At          string `json:"at"`
	Environment string `json:"environment"`
	Situation   string `json:"situation"`
	Verdict     string `json:"verdict"`
	Outcome     string `json:"outcome"`
	// Note is why the moment escalated, in the words the log uses, and empty
	// when it did not.
	Note       string           `json:"note"`
	Considered []consideredView `json:"considered"`

	// The four fields below are the moment as it was *written*, joined here
	// from internal/situation (§3.14's job situations) and internal/conflict
	// (§3.15's daily conflicts). §3.14's loop step 2 —
	// "you review… they read back what happened and how their character handled
	// it" — needs four things on one line, and internal/playbook can only ever
	// supply two of them: it holds which rule fired and on which condition, and
	// it holds a key where the moment was. The words are content the module
	// that authored the moment owns.
	//
	// All four are empty for a record that came from neither — a rule weighed
	// for any other reason still has its own line, with the key it always had.

	// Name is the moment's title and Line is the moment itself.
	Name string `json:"name"`
	Line string `json:"line"`
	// Who is the person it was with, and is empty for a moment that was with
	// nobody in particular. §3.15's pattern is with *people*, so a log that read
	// back four conflicts without saying they were all with the same neighbour
	// would have dropped the thing the player is meant to notice.
	Who string `json:"who"`
	// Came is what the answer came to, and is empty while nothing has answered
	// it yet. The consequence §3.14's log is for, in the content's own words —
	// never a verdict: the counts in `trends` are where a pattern is meant to
	// become visible, and a sentence saying "this went badly" would be the game
	// doing §3.6's diagnosis for the player.
	Came string `json:"came"`
}

/*
trendView is how one answer has been going over the window — §3.6's pattern.

Counts, not a grade. "One bad outcome is noise, the same rule failing daily for a
week is a pattern" is the whole reason this is on the same screen as the log: the
player is meant to point at their own evidence, and a server that had already
concluded something from it would be doing the diagnosis for them (§3.2).
*/
type trendView struct {
	Question   string `json:"question"`
	N          int    `json:"n"`
	Put        int    `json:"put"`
	Fired      int    `json:"fired"`
	Weighed    int    `json:"weighed"`
	Unanswered int    `json:"unanswered"`
	Undecided  int    `json:"undecided"`
	Good       int    `json:"good"`
	Poor       int    `json:"poor"`
}

type logView struct {
	Fires []fireView `json:"fires"`
	Page  int        `json:"page"`
	Pages int        `json:"pages"`
	Total int        `json:"total"`
	Size  int        `json:"size"`
	// Since is the start of the window the trends cover, so a screen can say
	// what "this week" meant rather than assuming.
	Since  string      `json:"since"`
	Trends []trendView `json:"trends"`
}

/*
LogWindow is how far back the pattern is counted.

A week, because §3.15 says so: "one bad outcome is noise, the same rule failing
daily for a week is a **pattern** — and a pattern is diagnosable." It is the
server's window rather than a query parameter for the same reason the page size
is the server's: a client that chose it could choose one that made any rule look
fine.
*/
const LogWindow = 7 * 24 * time.Hour

/*
playbookLog answers "what did my rules do".

Both halves in one response, because they are one question asked at two scales —
the moments themselves, and what they add up to. A screen that had to fetch the
pattern separately would render a week's counts beside a page of days they do not
cover.
*/
func (s *Server) playbookLog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	actor, ok := actorFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusInternalServerError, errorBody("the log could not be read"))
		return
	}

	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			// 422 for something that is not a number. A number past the end of
			// the log is clamped, the way a board page is: the client asked a
			// reasonable question about a world that moved.
			writeJSON(w, http.StatusUnprocessableEntity, errorBody("page must be a number"))
			return
		}
		page = n
	}

	since := s.clk.Now().Add(-LogWindow)
	var (
		read     playbook.Log
		trends   []playbook.Trend
		told     map[int64]situation.Telling
		friction map[int64]conflict.Telling
	)
	if err := s.store.InTx(r.Context(), func(tx pgx.Tx) error {
		var err error
		if read, err = s.playbooks.Fires(r.Context(), tx, actor.ID, page); err != nil {
			return err
		}
		if trends, err = s.playbooks.Evidence(r.Context(), tx, actor.ID, since); err != nil {
			return err
		}
		// The other half of a log line (§3.14): internal/playbook knows which
		// rule fired and internal/situation knows what was happening. Joined
		// here, in the composition root, because neither module may name the
		// other — and in the same transaction, so a page cannot show a record
		// beside a moment that was rolled back.
		ids := make([]int64, 0, len(read.Fires))
		for _, f := range read.Fires {
			ids = append(ids, f.ID)
		}
		if told, err = s.situations.Told(r.Context(), tx, actor.ID, ids); err != nil {
			return err
		}
		// And the other half of the same join, from the other module that
		// writes records (§3.15). Two lookups rather than one because a fire
		// comes from a job or from a neighbour and never from both, and a
		// module that could answer for either would be a module that knew about
		// the other.
		friction, err = s.conflicts.Told(r.Context(), tx, actor.ID, ids)
		return err
	}); err != nil {
		s.log.Error("read playbook log", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorBody("the log could not be read"))
		return
	}

	out := logView{
		Fires:  make([]fireView, 0, len(read.Fires)),
		Page:   read.Page,
		Pages:  read.Pages,
		Total:  read.Total,
		Size:   playbook.LogPage,
		Since:  stamp(since),
		Trends: make([]trendView, 0, len(trends)),
	}
	for _, f := range read.Fires {
		view := fireView{
			ID: f.ID, At: stamp(f.At), Environment: f.Environment, Situation: f.Situation,
			Verdict: string(f.Verdict), Outcome: string(f.Outcome), Note: f.Note,
			Considered: make([]consideredView, 0, len(f.Considered)),
		}
		if moment, authored := told[f.ID]; authored {
			view.Name, view.Line, view.Came = moment.Name, moment.Line, moment.Came
		}
		if moment, authored := friction[f.ID]; authored {
			view.Name, view.Line, view.Came = moment.Name, moment.Line, moment.Came
			view.Who = moment.Who
		}
		for _, c := range f.Considered {
			row := consideredView{
				Question: c.Question, N: c.N, State: string(c.State),
				Says: c.Says, Did: c.Did, Held: string(c.Held),
				Met: make([]string, 0, len(c.Met)),
			}
			for _, m := range c.Met {
				row.Met = append(row.Met, string(m))
			}
			view.Considered = append(view.Considered, row)
		}
		out.Fires = append(out.Fires, view)
	}
	for _, t := range trends {
		out.Trends = append(out.Trends, trendView{
			Question: t.Question, N: t.N, Put: t.Put, Fired: t.Fired, Weighed: t.Weighed,
			Unanswered: t.Unanswered, Undecided: t.Undecided, Good: t.Good, Poor: t.Poor,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

/*
populations answers playbook.Where: which environments have rule sets.

§3.15's "rule sets are environment-scoped, not global… the playbook is not one
list, it is a set of context-tagged rule sets", with the context being a
population. One set per population the world has, and the count is therefore the
number of files in `content/environments` rather than the number of districts on
the map — a rule set is a set of answers about *people*, and two districts with
the same people in them would not need two of them.

Every population, not only the ones the player has lived among. §3.15's story is
that you arrive somewhere and find your old answers waiting to fail, and a set
that appeared on arrival would turn that into a feature announcement.

It lives here rather than in internal/playbook because that package must not
depend on which catalogue happens to define an environment — the interface is
what lets a test give it two invented ones.
*/
type populations struct{}

func (populations) Environments() []playbook.Environment {
	all := environment.All()
	out := make([]playbook.Environment, 0, len(all))
	for _, e := range all {
		out = append(out, playbook.Environment{Key: e.Key, Name: e.Name})
	}
	return out
}
