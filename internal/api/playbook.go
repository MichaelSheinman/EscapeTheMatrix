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
The playbook determines how rules are handled.

Nothing here carries prose. Every field a client may send is a key out of a list
the server just gave it — a question, a term in that question's pool, a number
from that term's closed list, and one of two joining words. There is no string
field a player could type into, which makes "never free text" a shape rather than
a validation rule.

A locked term is absent rather than blanked out. internal/playbook strips the
vocabulary on the way out, so the pools below contain only what this player has unlocked, and
nothing says how many terms there are or that any are missing.

There is no save. playbook.compose writes the whole sentence, blanks included,
and inForce describes what was written rather than a second copy of it. A rule
with a hole in it is stored as composed and simply does not cover its question.
*/

// playbookValueView is one number a term may hold. The amount behind it stays on
// the server: a client that could do arithmetic with a threshold could predict
// the matcher, and the client computes no rules (§10.2).
type playbookValueView struct {
	Key  string `json:"key"`
	Says string `json:"says"`
}

// TODO: Don't like "says" as a property name, can we rename to "label"?
// playbookTermView is one word of the vocabulary. says keeps its {value}
// placeholder rather than arriving with the number written in, because the
// builder puts a menu inside the sentence — "they are paying me [above $60]" —
// and a finished string would have to be taken apart again.
type playbookTermView struct {
	Key  string `json:"key"`
	Says string `json:"says"`
	// Values is empty for a term that takes no number. Inlined per term rather
	// than sent once as a shared table, where a list of thresholds beside a
	// locked term would say the term exists.
	Values []playbookValueView `json:"values"`
}

// playbookQuestionView is one standing question and the whole vocabulary this
// player may answer it from.
type playbookQuestionView struct {
	// Questions arrive in authored order; the list's order is the sequence.
	Key   string `json:"key"`
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

// ruleView is one question's answer inside one rule set.
type ruleView struct {
	Question  string     `json:"question"`
	When      []*refView `json:"when"`
	Join      string     `json:"join"`
	Then      *refView   `json:"then"`
	Otherwise *refView   `json:"otherwise"`
	// InForce is whether anything is covering this question. Stated by the
	// server, because "nothing partial ever runs" is a rule (§10.2).
	InForce bool `json:"inForce"`
}

/*
conflictView is two answers that could disagree about the same moment — §6's
statically detectable rule conflicts, on the wire. Sent beside the rules rather
than fetched, because it describes the set the player is looking at and changes
the instant they change a word.

says is written by the server: a client that assembled it would be deciding what
a conflict means. The keys are sent as well, so the screen can point at the two
answers.
*/
type conflictView struct {
	// Questions is the two, in authored order.
	Questions []string `json:"questions"`
	// Always is the pair that disagrees whichever way each falls: a moment the
	// player has answered twice rather than one they have not thought about.
	Always bool `json:"always"`
	// TODO says is a strange property name, what does this mean? who says, main character or NPC?
	Says string `json:"says"`
}

// ruleSetView is one environment's rules — §3.15's context-tagged set.
type ruleSetView struct {
	Environment string     `json:"environment"`
	Name        string     `json:"name"`
	Rules       []ruleView `json:"rules"`
	// What the set says about itself with no moment in hand. Empty for a set
	// nothing is wrong with, which is the ordinary case.
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
			Key: q.Key, Ask: q.Ask, Note: q.Note, About: q.About,
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
				Always:    c.Always,
				Says:      clash(c, asked),
			})
		}

		out.Sets = append(out.Sets, view)
	}
	return out
}

// clash is a conflict, written out. It says what will happen rather than that
// something is wrong: nothing here labels a choice (§3.2), and a contradicting
// pair is a legitimate thing to hold. The two answers are cited by their
// questions' about texts.
func clash(c playbook.Conflict, asked map[string]playbook.Question) string {
	first, second := asked[c.Questions[0]].About, asked[c.Questions[1]].About
	if c.Always {
		return fmt.Sprintf("My answer for when %s and my answer for when %s disagree;"+
			" every moment that is both comes to me to decide.", first, second)
	}
	return fmt.Sprintf("My answer for when %s and my answer for when %s can disagree;"+
		" a moment that is both comes to me to decide.", first, second)
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

// playbookChanged is what a player's other tabs are told when a rule changes.
type playbookComposed struct {
	Playbook playbookView `json:"playbook"`
}

var playbookComposedMessage = realtime.Define[playbookComposed]("playbook.composed")

/*
composeInput is a rule, whole. The entire sentence arrives and the entire
sentence is written: no per-part command, which could leave a stored rule in a
state the player never composed, and no save, because this is the edit.

Every refusal about the vocabulary is a 422 and stays one — a term not in this
question's pool never will be. The one 409 is a term that exists and this player
has not unlocked yet (§3.6): the request is fine, and the same command works once
they have been through what unlocks it.
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

// TODO: I don't think the playbook rule composition is comprehensive for all cases.
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
		return composeResult{}, command.Invalid("The conditions are joined by an operation that's not and / or")
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
clearInput takes an answer back — a command rather than a compose with every part
null, because those mean different things. A stored sentence full of blanks is a
rule somebody wrote and did not finish; no rule at all is a question they never
answered.

There is no refusal for clearing a question nothing was written for: it is the
state the player asked for either way.
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
	// The same message a compose publishes, because the other tabs are being
	// told the same thing: this is the playbook now.
	if err := playbookComposedMessage.Publish(ctx, tx,
		realtime.To{Account: actor.ID, Origin: actor.Session},
		playbookComposed{Playbook: out.Playbook},
	); err != nil {
		return clearResult{}, err
	}
	return out, nil
}

/*
The log: what the rules did while the player was elsewhere (§3.14).

A GET rather than part of the world snapshot, because it is history and grows for
as long as somebody plays. A world model is bounded and a year of moments is not,
so this is a paginated list read when somebody looks at it (§10.2).

It carries no amount and no stance. The client is told which condition was met
and what the character did, in words, and never the threshold behind a term or
the posture behind an action — either would let a screen work out what the
matcher was going to do. Where the posture matters, the server says so in note.
*/

// logPath is the record: this player's moments, newest first.
const logPath = "/api/playbook/log"

type consideredView struct {
	Question string   `json:"question"`
	State    string   `json:"state"`
	Says     string   `json:"says"`
	Did      string   `json:"did"`
	Met      []string `json:"met"`
	Held     string   `json:"held"`
}

// TODO: what is fireView?
// the name conflicts conflicts with the fire challenge?
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
	// All four are empty for a record that came from neither.

	// Name is the moment's title and Line is the moment itself.
	Name string `json:"name"`
	Line string `json:"line"`
	// Who is the person it was with, and empty for a moment with nobody in
	// particular. §3.15's pattern is with people, so a log that read back four
	// conflicts without naming the neighbour would drop what matters.
	Who string `json:"who"`
	// Came is what the answer came to, empty while nothing has answered it. The
	// consequence §3.14's log is for, in the content's own words and never a
	// verdict — the counts in trends are where a pattern becomes visible.
	Came string `json:"came"`
}

type trendView struct {
	Question   string `json:"question"`
	Put        int    `json:"put"`
	Fired      int    `json:"fired"`
	Weighed    int    `json:"weighed"`
	Unanswered int    `json:"unanswered"`
	Undecided  int    `json:"undecided"`
	Good       int    `json:"good"`
	Poor       int    `json:"poor"`
}

type logView struct {
	Fires  []fireView  `json:"fires"`
	Page   int         `json:"page"`
	Pages  int         `json:"pages"`
	Total  int         `json:"total"`
	Size   int         `json:"size"`
	Since  string      `json:"since"`
	Trends []trendView `json:"trends"`
}

// LogWindow is how far back the pattern is counted: a week, because that is
// where §3.15 says a pattern becomes diagnosable. The server's window rather
// than a query parameter, or a client could choose one that made any rule look
// fine.
const LogWindow = 7 * 24 * time.Hour

// playbookLog answers "what did my rules do". Both halves in one response,
// because they are one question at two scales: the moments themselves, and what
// they add up to. Fetching the pattern separately would render a week's counts
// beside a page of days they do not cover.
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
			// 422 for something that is not a number. A page past the end of the
			// log is clamped, like a board page.
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
		// rule fired and internal/situation knows what was happening. Joined in
		// the composition root, in one transaction, so a page cannot show a
		// record beside a moment that was rolled back.
		ids := make([]int64, 0, len(read.Fires))
		for _, f := range read.Fires {
			ids = append(ids, f.ID)
		}
		if told, err = s.situations.Told(r.Context(), tx, actor.ID, ids); err != nil {
			return err
		}
		// And the same join from the other module that writes records (§3.15).
		// Two lookups, because a fire comes from a job or from a neighbour and
		// never both.
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
				Question: c.Question, State: string(c.State),
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
			Question: t.Question, Put: t.Put, Fired: t.Fired, Weighed: t.Weighed,
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
