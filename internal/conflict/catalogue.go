package conflict

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/MichaelSheinman/EscapeTheMatrix/content"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/environment"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/playbook"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

const Pool = 6

// Replies is how many answers a moment needs to be a decision.
const Replies = 2

type Source string

const (
	// The player is in debt.
	SourceOwing Source = "owing"
	// The "other" is in debt.
	SourceOwed Source = "owed"
	// The player has let "other" down.
	SourceSlighted Source = "slighted"
	// Something was left hanging
	SourceUnsettled Source = "unsettled"
	// Anything happenned between the characters
	SourceKnown Source = "known"
	// How much "other" owes a person
	SourceAsked Source = "asked"
)

var states = map[Source]func(Standing) bool{
	SourceOwing:     func(s Standing) bool { return s.Owing() > 0 },
	SourceOwed:      func(s Standing) bool { return s.Credit() > 0 },
	SourceSlighted:  func(s Standing) bool { return s.Slighted > 0 },
	SourceUnsettled: func(s Standing) bool { return s.Unsettled > 0 },
	SourceKnown:     func(s Standing) bool { return !s.Stranger() },
}

// readings is every source that answers a term that holds one.
var readings = map[Source]func(Standing) int64{
	SourceAsked: func(s Standing) int64 { return int64(s.Asked) },
}

// Who is where a person's name goes in an authored line.
//
// It crosses no wire: the substitution happens on the server, at the moment the
// conflict is read, because the name comes from `content/environments` and a
// client holding both halves would be a client assembling the game's own prose.
const Who = "{who}"

/*
Fact is one thing a pathway states about its moment.

Either stated — `so: true`, or a `reading:` — or taken from the standing. The
two are one type rather than two because they are one idea from the matcher's
side: by the time a moment reaches internal/playbook, "I owe them something" is
simply true or false, and where it came from is this package's business.
*/
type Fact struct {
	Term string
	// From is the standing this fact is read off, and empty for a stated one.
	From Source
	// Number is whether the term holds one, decided by the vocabulary at boot
	// rather than by the value written. A stated reading of nought is a real
	// statement — "they have asked me nothing this month" — and without this
	// field it would be indistinguishable from a term that takes no number at
	// all, which is the one mistake the matcher cannot report on its own.
	Number bool
	// So and Reading are what a stated fact says. Which of them is meaningful is
	// decided by Number.
	So      bool
	Reading int64
}

/*
Answer is one posture a conflict can be met with, and everything that follows.

`Shift` is what internal/situation's equivalent does not have, and it is the
whole loop: an answer that leaves somebody owing you changes what tomorrow's
conflict with them will be about. Without it a daily conflict would be a page of
authored moments selected by a hash, which is the thing §3.15 says it is not.
*/
type Answer struct {
	Stance playbook.Stance
	// Did is what your character does, in the second person.
	Did string
	// Came is what it came to, in their words: the reply they send back, which
	// is the only account of how it went the player gets. One string per chat
	// bubble, in the order they arrive.
	Came []string
	// Title is this answer as one line of somebody's record: what the character
	// did, in the second person, with the person named — "You did not cover
	// {who}'s $12". It is what an escalation about this answer is called, and
	// it is authored per answer because the same conflict answered four ways is
	// four different things to have done.
	//
	// Written about the doing and never about how it landed. "You turned down
	// the van" is a title; "{who} is upset with you" is the game telling the
	// player what to conclude from a moment they have not read yet (§3.2), and
	// the counts on the playbook's log are where a pattern is meant to surface.
	//
	// Optional, and empty falls back to the authored event's own name. A
	// pathway with no title still escalates.
	Title string
	// Outcome is how the world took it: good, mixed or poor. Never
	// [playbook.Pending]; an authored answer is a settled one by construction.
	Outcome playbook.Outcome
	// Moves is §3.5's authored hidden deltas, in the pure core's own order.
	Moves []simulation.Grant
	// Shift is what it does to what stands between the two of you.
	Shift Shift
}

/*
Authored is one pathway, whole.

It carries the questions resolved against the vocabulary rather than the raw
strings, so a misspelt term is a failed build rather than a rule that quietly
never matched.
*/
type Authored struct {
	Key string
	// Among is the population this belongs to, by environment key. A pathway is
	// authored *for* a population, which is §3.15's whole mechanism: the same
	// friction is not the same friction somewhere else.
	Among string
	// With is who may send it, by the key `content/environments` authors them
	// under. Required and never empty — see internal/conflict/thread.go for why
	// "anybody" is a thing an author has to write down rather than a default
	// they can fall into.
	With []string
	// After is what must already have passed with the same person before this
	// can. Empty is an opener, which is what a stranger is met with.
	After []Requirement
	// Every is how many days before it may be said again, or nought for a
	// pathway that is spent once it has been answered.
	Every int
	// Follows is the pressure this arises from.
	Follows Strain
	Name    string
	Line    []string
	// Puts is the one playbook question it raises.
	Puts string
	// Raises is what the moment does to the standing simply by happening —
	// somebody asking you for something has asked you, whatever you answer.
	Raises Shift

	facts   []Fact
	answers []Answer
}

// Sends reports whether this is a moment that person could have.
func (a Authored) Sends(person string) bool {
	for _, key := range a.With {
		if key == person {
			return true
		}
	}
	return false
}

// Facts is what this pathway states or reads, in authored order.
func (a Authored) Facts() []Fact {
	out := make([]Fact, len(a.facts))
	copy(out, a.facts)
	return out
}

// Answers is every posture this moment has an answer for, in the stance
// vocabulary's own order.
func (a Authored) Answers() []Answer {
	out := make([]Answer, len(a.answers))
	copy(out, a.answers)
	return out
}

// Answer finds the one written for a posture.
func (a Authored) Answer(s playbook.Stance) (Answer, bool) {
	for _, ans := range a.answers {
		if ans.Stance == s {
			return ans, true
		}
	}
	return Answer{}, false
}

/*
Moment is what the matcher is asked about, given what stands between the two
people.

This is the join the whole ticket turns on. The authored half says what kind of
thing happened; the standing says what is true of it; and internal/playbook is
handed one [playbook.Situation] that cannot tell which half a fact came from —
which is exactly right, because a rule reading "if I owe them something" should
fire on a debt the player actually incurred and never on one an author typed.
*/
func (a Authored) Moment(st Standing) playbook.Situation {
	out := playbook.Arises(a.Key, a.Puts)
	for _, f := range a.facts {
		switch {
		case f.From == "" && f.Number:
			out = out.Reading(f.Term, f.Reading)
		case f.From == "":
			out = out.So(f.Term, f.So)
		case states[f.From] != nil:
			out = out.So(f.Term, states[f.From](st))
		case readings[f.From] != nil:
			out = out.Reading(f.Term, readings[f.From](st))
		}
	}
	return out
}

// Says is an authored line with the person's name in it.
func Says(text, name string) string { return strings.ReplaceAll(text, Who, name) }

// SaysAll is every bubble of one message with the person's name in it.
func SaysAll(all []string, name string) []string {
	out := make([]string, 0, len(all))
	for _, bubble := range all {
		out = append(out, Says(bubble, name))
	}
	return out
}

// Said is every bubble of one message as a single string, for a surface that
// reads the whole of what was sent rather than the bubbles it arrived in.
func Said(all []string) string { return said(all) }

/*
catalogue is the one parsed copy, built at process start.

A package-level value for the same reason internal/situation's and
internal/playbook's are: authored content with no state in it, identical in
every process, and a second copy could only ever be a second opinion.
*/
var catalogue []Authored

/*
init parses and validates the shipped pathways, and panics if it cannot.

`parse` takes the environment keys rather than reading them out of
internal/environment itself, so that the failure paths below are testable
against invented populations — a test cannot catch an init that already ran, and
the checks here are the ones that decide whether the mechanic has holes in it.
*/
func init() {
	all, err := parse(content.FS, keys(environment.All()))
	if err != nil {
		panic("conflict: " + err.Error())
	}
	catalogue = all
}

// keys is every population the world has, by environment key, in the order
// their own file authors them — which is the order `with:` is normalised to, so
// that a pathway's senders read down the page the way the environment does.
func keys(all []environment.Environment) map[string][]string {
	out := make(map[string][]string, len(all))
	for _, e := range all {
		people := make([]string, 0, len(e.People()))
		for _, p := range e.People() {
			people = append(people, p.Key)
		}
		out[e.Key] = people
	}
	return out
}

// All is every authored pathway, in key order within population order.
func All() []Authored {
	out := make([]Authored, len(catalogue))
	copy(out, catalogue)
	return out
}

// Pathway finds one by population and key.
func Pathway(among, key string) (Authored, bool) {
	for _, a := range catalogue {
		if a.Among == among && a.Key == key {
			return a, true
		}
	}
	return Authored{}, false
}

/*
For is every pathway a population has for one pressure, in catalogue order.

The pool a day's conflict is drawn from. Empty is impossible for a shipped
population — the loader refuses one — and is handled by the caller walking on to
the next pressure rather than by a panic, because a build that authored a third
population badly should produce a duller day rather than a dead one.
*/
func For(among string, follows Strain) []Authored {
	out := []Authored{}
	for _, a := range catalogue {
		if a.Among == among && a.Follows == follows {
			out = append(out, a)
		}
	}
	return out
}

/* ── reading the files ─────────────────────────────────────────────────── */

const conflictsPath = "conflicts"

// The YAML shape. Separate from the types above on purpose, like every other
// loader here: what an author writes and what the rest of the program reads are
// two decisions, and a pathway that carried its file's field names would make
// renaming one a migration.
type conflictFile struct {
	Key   string   `yaml:"key"`
	Among string   `yaml:"among"`
	With  []string `yaml:"with"`
	// After is written either way round: a bare key where any answer will do,
	// or a mapping where the posture matters. Both decode into the same
	// [Requirement], which is why this is a node rather than a struct — the
	// choice is the author's convenience and nothing downstream should be able
	// to tell which was used.
	After   []yaml.Node `yaml:"after"`
	Every   int         `yaml:"every"`
	Follows string      `yaml:"follows"`
	Name    string      `yaml:"name"`
	Line    []string    `yaml:"line"`
	Puts    string      `yaml:"puts"`
	Facts   []struct {
		Term string `yaml:"term"`
		// From reads the fact off the standing. So and Reading state it
		// outright. Exactly one of the three is written.
		From    string `yaml:"from"`
		So      *bool  `yaml:"so"`
		Reading *int64 `yaml:"reading"`
	} `yaml:"facts"`
	Raises  map[string]int `yaml:"raises"`
	Answers []struct {
		Stance  string             `yaml:"stance"`
		Did     string             `yaml:"did"`
		Came    []string           `yaml:"came"`
		Title   string             `yaml:"title"`
		Outcome string             `yaml:"outcome"`
		Moves   map[string]float64 `yaml:"moves"`
		Shifts  map[string]int     `yaml:"shifts"`
	} `yaml:"answers"`
}

/*
parse reads the catalogue out of a content filesystem and checks every rule the
rest of this package then gets to assume.

Strict twice over, in the same two ways every loader in this repository is.
Unknown YAML fields are refused, so a misspelt `follows:` is a failed build
rather than a pathway nothing ever reaches; and every cross-reference — a
population, a question, a term, a source, a posture, a factor — is resolved here
rather than at the moment a player happens to walk into it.

The two checks that are the ticket rather than hygiene are at the bottom: every
population has a pool worth the name, and every population can answer every
pressure its own people will be under.
*/
func parse(files fs.FS, among map[string][]string) ([]Authored, error) {
	if len(among) == 0 {
		return nil, fmt.Errorf("there are no populations for %s to belong to", conflictsPath)
	}

	dirs, err := fs.ReadDir(files, conflictsPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", conflictsPath, err)
	}

	var out []Authored
	seen := map[string]bool{}
	for _, dir := range dirs {
		if !dir.IsDir() {
			// Docs live beside the content they govern (SPEC.md, README.md),
			// the way content/jobs and content/challenges already carry them.
			// Anything else loose in here is still a mistake.
			if strings.HasSuffix(dir.Name(), ".md") {
				continue
			}
			return nil, fmt.Errorf("%s holds %q, which is not a population's directory",
				conflictsPath, dir.Name())
		}
		population, known := among[dir.Name()]
		if !known {
			return nil, fmt.Errorf("%s/%s is not a population", conflictsPath, dir.Name())
		}

		where := path.Join(conflictsPath, dir.Name())
		entries, err := fs.ReadDir(files, where)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", where, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
				continue
			}
			name := path.Join(where, entry.Name())

			var file conflictFile
			if err := content.Decode(files, name, &file); err != nil {
				return nil, err
			}
			a, err := pathway(file, population)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			// The key is the filename and the population is the directory, so a
			// conflict recorded against a player has one name in the table, in
			// the log and on disk.
			if stem := strings.TrimSuffix(entry.Name(), ".yaml"); a.Key != stem {
				return nil, fmt.Errorf("%s: is called %q", name, a.Key)
			}
			if a.Among != dir.Name() {
				return nil, fmt.Errorf("%s: says it is among %q", name, a.Among)
			}
			if seen[a.Among+"/"+a.Key] {
				return nil, fmt.Errorf("%s: two pathways called %q", name, a.Key)
			}
			seen[a.Among+"/"+a.Key] = true
			out = append(out, a)
		}
	}

	// By population, then by key, so the pool a day is drawn from is in the same
	// order in every process (§7). A directory walk is already sorted on every
	// filesystem this runs on; saying so is what makes the determinism a
	// decision rather than an accident.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Among != out[j].Among {
			return out[i].Among < out[j].Among
		}
		return out[i].Key < out[j].Key
	})

	// §3.15's content cost, enforced. Every population the world has — not only
	// the ones somebody remembered to write for.
	for key := range among {
		pool := 0
		for _, a := range out {
			if a.Among == key {
				pool++
			}
		}
		if pool < Pool {
			return nil, fmt.Errorf("%s/%s holds %d pathway(s); a population needs at least %d for "+
				"repetition to read as the same people rather than as the game running out",
				conflictsPath, key, pool, Pool)
		}
	}

	// And the checks that are the ordering rather than the pool: what waits on
	// what, whether a thread closes, and whether everybody has an opening line.
	// See internal/conflict/thread.go.
	if err := ordering(out, among); err != nil {
		return nil, fmt.Errorf("%s: %w", conflictsPath, err)
	}
	return out, nil
}

// pathway reads and checks one file, against the population it is authored for.
func pathway(file conflictFile, population []string) (Authored, error) {
	if file.Key == "" {
		return Authored{}, fmt.Errorf("a pathway has no key")
	}
	if file.Among == "" {
		return Authored{}, fmt.Errorf("%q is among nobody", file.Key)
	}
	with, err := sends(file.Key, file.With, population)
	if err != nil {
		return Authored{}, err
	}
	after, err := waits(file.Key, file.After)
	if err != nil {
		return Authored{}, err
	}
	if file.Every < 0 {
		return Authored{}, fmt.Errorf("%q comes round every %d days", file.Key, file.Every)
	}
	if strings.TrimSpace(file.Name) == "" || len(bubbles(file.Line)) == 0 {
		return Authored{}, fmt.Errorf("%q needs a name and a line", file.Key)
	}
	// Omitted means [Easy], which every standing is under. A strain is named
	// when the line leans on one: "fourth time, I hate asking" needs [Asking].
	follows := Easy
	if file.Follows != "" {
		follows = Strain(file.Follows)
		if !Known(follows) {
			return Authored{}, fmt.Errorf("%q follows from %q, which is not one of %v", file.Key, file.Follows, Strains)
		}
	}
	// A line is the person's own words, first person, in a bubble the thread
	// header already names (content/conflicts/SPEC.md, 2026-08-10). A bubble
	// that names its own sender is narration wearing a costume — the exact
	// register failure the SPEC exists to kill. Since 2026-08-10 `came` is that
	// person's reply rather than an account of them, so [reply] holds it to the
	// same rule.
	for _, bubble := range file.Line {
		if strings.Contains(bubble, Who) {
			return Authored{}, fmt.Errorf("%q's line contains %s; a line is the person's own words and the thread names them", file.Key, Who)
		}
	}
	if err := budget(file.Key, "name", file.Name, nameBudget); err != nil {
		return Authored{}, err
	}

	vocab := playbook.Vocab()
	question, asks := vocab.Question(file.Puts)
	if !asks {
		return Authored{}, fmt.Errorf("%q puts %q, which the world does not ask", file.Key, file.Puts)
	}

	out := Authored{
		Key: file.Key, Among: file.Among, With: with, After: after, Every: file.Every,
		Follows: follows,
		Name:    strings.TrimSpace(file.Name), Line: bubbles(file.Line), Puts: file.Puts,
	}

	stated := map[string]bool{}
	for _, f := range file.Facts {
		fact, err := fact(file.Key, question, f.Term, Source(f.From), f.So, f.Reading)
		if err != nil {
			return Authored{}, err
		}
		if stated[fact.Term] {
			return Authored{}, fmt.Errorf("%q states %q twice", file.Key, fact.Term)
		}
		stated[fact.Term] = true
		out.facts = append(out.facts, fact)
	}

	// The matcher's own reading of the same moment, which catches the one thing
	// the loop above cannot: a term stated as the wrong sort of thing. A
	// standing with everything on it is used, so a fact read off the ledger is
	// checked in the shape it will actually arrive in.
	if faults := out.Moment(everything).Faults(); len(faults) > 0 {
		return Authored{}, fmt.Errorf("%q states %s in a way the vocabulary cannot read",
			file.Key, strings.Join(faults, ", "))
	}

	raises, err := shifts(file.Key, "raises", file.Raises)
	if err != nil {
		return Authored{}, err
	}
	out.Raises = raises

	written := map[playbook.Stance]bool{}
	for _, a := range file.Answers {
		answer, err := reply(file.Key, a.Stance, a.Did, a.Came, a.Title, a.Outcome, a.Moves, a.Shifts)
		if err != nil {
			return Authored{}, err
		}
		if written[answer.Stance] {
			return Authored{}, fmt.Errorf("%q answers %q twice", file.Key, answer.Stance)
		}
		written[answer.Stance] = true
		out.answers = append(out.answers, answer)
	}

	// A posture with no answer escalates to the player, so the floor is what
	// makes the moment a decision rather than what the question can reach.
	if len(out.answers) < Replies {
		return Authored{}, fmt.Errorf("%q offers %d answer(s); a moment is a decision and needs at least %d",
			file.Key, len(out.answers), Replies)
	}
	// An answer for a posture no rule could take is writing nobody will ever
	// read. The whole action pool rather than what a player has unlocked —
	// content outlives one player's vocabulary.
	reachable := postures(question)
	for _, a := range out.answers {
		if !contains(reachable, a.Stance) {
			return Authored{}, fmt.Errorf("%q answers %q, which %q cannot reach", file.Key, a.Stance, file.Puts)
		}
	}
	// The stance vocabulary's order rather than the file's, so two pathways
	// offer the same postures in the same order however they were written.
	sort.SliceStable(out.answers, func(i, j int) bool {
		return rankStance(out.answers[i].Stance) < rankStance(out.answers[j].Stance)
	})

	// Last, over everything a player can read. §3.2, §3.10 and §3.15 all say it
	// and this is where it would be easiest to break, because a conflict is
	// written *for* a population and the production vocabulary for a population
	// is exactly the set of words that must never reach a screen. The check is
	// internal/environment's own, held to rather than reimplemented.
	for what, text := range map[string]string{"its name": out.Name, "its line": said(out.Line)} {
		if word := environment.Labelled(text); word != "" {
			return Authored{}, fmt.Errorf("%q says %q in %s, which labels the framework (§3.2)", file.Key, word, what)
		}
	}
	for _, a := range out.answers {
		for what, text := range map[string]string{"what was done": a.Did, "what came of it": said(a.Came)} {
			if word := environment.Labelled(text); word != "" {
				return Authored{}, fmt.Errorf("%q's %q answer says %q in %s, which labels the framework (§3.2)",
					file.Key, a.Stance, word, what)
			}
		}
	}
	return out, nil
}

/*
sends reads and checks who may send a moment.

Required, and the error says why rather than only what: an author who left it
out is almost always an author who meant "anybody", and the answer is that
anybody is six names they have to look at. Checked against the real population,
so a pathway bound to somebody who has moved out of the environment file is a
failed build rather than a moment nobody can ever be sent.
*/
func sends(key string, with, population []string) ([]string, error) {
	if len(with) == 0 {
		return nil, fmt.Errorf("%q says nothing about who sends it; name them — and if it truly fits "+
			"everybody, name everybody, because that is a decision somebody makes rather than one they omit", key)
	}
	lives := map[string]bool{}
	for _, person := range population {
		lives[person] = true
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(with))
	for _, person := range with {
		switch {
		case !lives[person]:
			return nil, fmt.Errorf("%q is sent by %q, who does not live among %v", key, person, population)
		case seen[person]:
			return nil, fmt.Errorf("%q names %q twice as its sender", key, person)
		}
		seen[person] = true
		out = append(out, person)
	}
	// The population's own order rather than the file's, so two pathways sent by
	// the same people list them the same way however they were written.
	sort.SliceStable(out, func(i, j int) bool {
		return index(population, out[i]) < index(population, out[j])
	})
	return out, nil
}

// waits reads what must already have passed, in either of the two ways an
// author may write it.
func waits(key string, nodes []yaml.Node) ([]Requirement, error) {
	out := make([]Requirement, 0, len(nodes))
	for _, node := range nodes {
		var need Requirement
		switch node.Kind {
		case yaml.ScalarNode:
			// `after: [the-quick-hand]` — the conversation happened, and how it
			// went does not bear on this one.
			if err := node.Decode(&need.Conflict); err != nil {
				return nil, fmt.Errorf("%q waits on something that is not a pathway's key: %w", key, err)
			}
		case yaml.MappingNode:
			var written struct {
				Conflict string `yaml:"conflict"`
				Stance   string `yaml:"stance"`
			}
			if err := node.Decode(&written); err != nil {
				return nil, fmt.Errorf("%q waits on something it cannot read: %w", key, err)
			}
			need.Conflict, need.Stance = written.Conflict, playbook.Stance(written.Stance)
			if need.Stance != "" && !contains(playbook.Stances, need.Stance) {
				return nil, fmt.Errorf("%q waits on %q being met with %q, which is not a posture",
					key, need.Conflict, written.Stance)
			}
		default:
			return nil, fmt.Errorf("%q waits on something that is neither a key nor a key and a posture", key)
		}
		if need.Conflict == "" {
			return nil, fmt.Errorf("%q waits on a pathway with no key", key)
		}
		if need.Conflict == key {
			return nil, fmt.Errorf("%q waits on itself", key)
		}
		out = append(out, need)
	}
	return out, nil
}

// index is where a key sits in a list, or its length for one that is not in it.
func index(all []string, key string) int {
	for i, at := range all {
		if at == key {
			return i
		}
	}
	return len(all)
}

// everything is a standing under every pressure at once.
//
// Used to type-check a pathway's facts at boot: a fact read off the ledger has
// to be checked in the shape it will arrive in, and a zero standing would leave
// every `from:` fact reading false and every reading reading nought — which the
// matcher accepts and which would prove nothing.
var everything = Standing{Owed: 1, Slighted: 1, Unsettled: 1, Asked: Crowding, Met: 1}

// states1 reads one fact, resolving its term and deciding which of the three
// ways of stating it was used.
func fact(
	key string, question playbook.Question, id string, from Source, so *bool, reading *int64,
) (Fact, error) {
	if id == "" {
		return Fact{}, fmt.Errorf("%q states a fact with no term", key)
	}
	term, ok := playbook.Vocab().Term(id)
	if !ok {
		return Fact{}, fmt.Errorf("%q states %q, which is not a term", key, id)
	}
	// A fact about a question nobody is putting is a fact nothing can test. It
	// would read as an authored subtlety and behave as a typo.
	if term.Question != question.Key {
		return Fact{}, fmt.Errorf("%q states %q and puts %q", key, id, question.Key)
	}
	if term.Kind != playbook.Condition {
		return Fact{}, fmt.Errorf("%q states %q, which is an answer rather than a fact", key, id)
	}

	out := Fact{Term: id, From: from, Number: term.Takes()}
	switch {
	case from != "" && (so != nil || reading != nil):
		return Fact{}, fmt.Errorf("%q states %q both ways", key, id)

	case from != "":
		_, isState := states[from]
		_, isReading := readings[from]
		switch {
		case !isState && !isReading:
			return Fact{}, fmt.Errorf("%q reads %q from %q, which is not something the standing holds", key, id, from)
		case isReading && !term.Takes():
			return Fact{}, fmt.Errorf("%q reads %q from %q, which is a count, and %q holds no number",
				key, id, from, id)
		case isState && term.Takes():
			return Fact{}, fmt.Errorf("%q reads %q from %q, which is a state, and %q holds a number",
				key, id, from, id)
		}
		return out, nil

	case so != nil && reading != nil:
		return Fact{}, fmt.Errorf("%q states %q both ways", key, id)
	case reading != nil:
		if !term.Takes() {
			return Fact{}, fmt.Errorf("%q gives %q a number, and it holds none", key, id)
		}
		out.Reading = *reading
		return out, nil
	case so != nil:
		if term.Takes() {
			return Fact{}, fmt.Errorf("%q states %q as a yes or no, and it holds a number", key, id)
		}
		out.So = *so
		return out, nil
	default:
		return Fact{}, fmt.Errorf("%q states %q and says nothing about it", key, id)
	}
}

// reply reads one answer.
func reply(
	key, stance, did string, came []string, title, outcome string, moves map[string]float64, shift map[string]int,
) (Answer, error) {
	s := playbook.Stance(stance)
	if !contains(playbook.Stances, s) {
		return Answer{}, fmt.Errorf("%q answers with %q, which is not one of %v", key, stance, playbook.Stances)
	}
	back := bubbles(came)
	if strings.TrimSpace(did) == "" || len(back) == 0 {
		return Answer{}, fmt.Errorf("%q's %q answer needs both what was done and what came of it", key, stance)
	}
	if err := budget(key, stance+" did", did, didBudget); err != nil {
		return Answer{}, err
	}
	// What came of it is their bubbles back, so the same rule the line is held
	// to: the thread header names them, and a person typing their own name into
	// their own message is the narration this content type refuses.
	for _, bubble := range back {
		if strings.Contains(bubble, Who) {
			return Answer{}, fmt.Errorf("%q's %q answer contains %s in what came of it; it is the person's own reply and the thread names them",
				key, stance, Who)
		}
	}

	// A title *may* name them, which is the opposite of the rule directly
	// above and for the same reason. What came of it is their own message and a
	// person does not type their own name; a title is a line in the player's
	// record, written about them in the third person, and the whole point of it
	// is that it says who this was with.
	if err := budget(key, stance+" title", title, titleBudget); err != nil {
		return Answer{}, err
	}

	out := Answer{Stance: s, Did: trim(did), Came: back, Title: trim(title)}
	switch playbook.Outcome(outcome) {
	case playbook.Good, playbook.Mixed, playbook.Poor:
		out.Outcome = playbook.Outcome(outcome)
	default:
		return Answer{}, fmt.Errorf("%q's %q answer came to %q, which is not one of %q, %q, %q",
			key, stance, outcome, playbook.Good, playbook.Mixed, playbook.Poor)
	}

	for _, name := range sorted(moves) {
		which, err := moved(name)
		if err != nil {
			return Answer{}, fmt.Errorf("%q's %q answer %w", key, stance, err)
		}
		out.Moves = append(out.Moves, simulation.Grant{Factor: which, Credit: moves[name]})
	}

	shifted, err := shifts(key, string(s)+"'s shifts", shift)
	if err != nil {
		return Answer{}, err
	}
	out.Shift = shifted
	return out, nil
}

/*
shifts reads a `shifts:` or `raises:` block.

A closed key set, refused by name rather than ignored, for the reason strict
decoding exists everywhere else in this loader: a misspelt `unsetled: 1` that
was silently dropped would be a conflict that never stops coming back and no way
to see why from the file.
*/
func shifts(key, what string, block map[string]int) (Shift, error) {
	out := Shift{}
	for _, name := range sortedKeys(block) {
		n := block[name]
		if n < -Most || n > Most {
			return Shift{}, fmt.Errorf("%q's %s moves %s by %d, which is more than the ledger holds (±%d)",
				key, what, name, n, Most)
		}
		switch name {
		case "owed":
			out.Owed = n
		case "slighted":
			out.Slighted = n
		case "unsettled":
			out.Unsettled = n
		case "asked":
			out.Asked = n
		default:
			return Shift{}, fmt.Errorf("%q's %s moves %q, which is not part of a standing "+
				"(owed, slighted, unsettled, asked)", key, what, name)
		}
	}
	return out, nil
}

// postures is every stance this question's actions can amount to, in the stance
// vocabulary's order.
func postures(q playbook.Question) []playbook.Stance {
	seen := map[playbook.Stance]bool{}
	for _, t := range q.Actions() {
		seen[t.Stance] = true
	}
	out := []playbook.Stance{}
	for _, s := range playbook.Stances {
		if seen[s] {
			out = append(out, s)
		}
	}
	return out
}

// sorted is a `moves` block's factor names in the simulation's own order.
//
// Map iteration in Go is deliberately random, and authored grants that arrived
// in a different order on every boot would be §7's determinism lost to a detail
// of the language rather than to a decision.
func sorted(moves map[string]float64) []string {
	out := make([]string, 0, len(moves))
	for name := range moves {
		out = append(out, name)
	}
	sort.Slice(out, func(i, j int) bool { return rank(out[i]) < rank(out[j]) })
	return out
}

// sortedKeys is a shift block's keys in a stable order, so that a file with two
// mistakes in it reports the same one every time.
func sortedKeys(block map[string]int) []string {
	out := make([]string, 0, len(block))
	for name := range block {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func rank(name string) int {
	for i, f := range simulation.Factors {
		if f.Key() == name {
			return i
		}
	}
	return len(simulation.Factors)
}

func rankStance(s playbook.Stance) int {
	for i, known := range playbook.Stances {
		if known == s {
			return i
		}
	}
	return len(playbook.Stances)
}

/*
moved is the factor a `moves:` line names.

It refuses Health and Money by name rather than with a generic message, for the
same reason internal/situation's does: what an author needs told is *which*
factors this may move — the three scored from committed choices (§3.5). A
conflict that moved a body or a balance would be this module writing behind
internal/health and internal/character, which own them.
*/
func moved(name string) (simulation.Factor, error) {
	var granted []string
	for _, f := range simulation.Factors {
		if f.Granted() {
			granted = append(granted, f.Key())
		}
	}
	for _, f := range simulation.Factors {
		if f.Key() != name {
			continue
		}
		if !f.Granted() {
			return 0, fmt.Errorf("moves %s, which is read from the world rather than granted (a conflict moves %s)",
				name, strings.Join(granted, ", "))
		}
		return f, nil
	}
	return 0, fmt.Errorf("moves %q, which is not a factor (a conflict moves %s)",
		name, strings.Join(granted, ", "))
}

func contains[T comparable](list []T, want T) bool {
	for _, x := range list {
		if x == want {
			return true
		}
	}
	return false
}

// trim flattens an authored block into one paragraph, so how somebody wrapped
// their prose never reaches a screen.
func trim(s string) string { return strings.Join(strings.Fields(s), " ") }

// bubbles is the authored bubbles of one message, trimmed, with the empty ones
// dropped. A message is one string per bubble in the order they arrive.
func bubbles(said []string) []string {
	out := make([]string, 0, len(said))
	for _, bubble := range said {
		if trimmed := trim(bubble); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// said is every bubble of one message as a single string, for a check or a
// record that reads the whole of what was sent rather than the bubbles it
// arrived in.
func said(all []string) string { return strings.Join(all, " ") }

/*
The prose budgets (content/conflicts/SPEC.md). Over budget is where the old
register creeps back in — a fourth sentence of line is a theatrics sentence —
so the budgets are boot errors rather than style advice, the same deal the
job postings struck.
*/
const (
	nameBudget = 50
	didBudget  = 80
	// Tighter than `did`, because a title has to survive being a row in a list
	// beside a date and a stamp. `{who}` counts against it before the name is
	// substituted, which is the wrong way round by a few characters and the
	// safe way round: a budget measured on the placeholder can only be met by a
	// title that also fits with the longest name in it.
	titleBudget = 64
)

// budget holds one authored field to its SPEC length.
func budget(key, field, text string, most int) error {
	if n := utf8.RuneCountInString(trim(text)); n > most {
		return fmt.Errorf("%q's %s runs %d characters; the budget is %d", key, field, n, most)
	}
	return nil
}
