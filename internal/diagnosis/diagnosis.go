/*
Package diagnosis owns struggle-gated unlocking: the player gathering evidence,
naming the cause of a failure, and the naming being what grants the tool.

HighLevelOverview.md §3.6 calls this "the purest expression of the theme" and in
the same breath says it "carries a real risk of reading as a quiz". The five
rules it gives for keeping it from becoming one are not guidance this package
tries to honour — they are the reason it is shaped the way it is, so each of them
is written next to the thing that enforces it:

 1. **Point at evidence, not at answer strings.** Nothing authored is ever
    offered as an option. What the player selects from is [Observation]s, which
    are readings of their own rows produced by the modules that own them, and
    which this package receives through [Ledger] without being able to fetch
    them. There is no field anywhere in the catalogue for a sentence a player
    picks.
 2. **Never say "correct".** This package has no representation of a right
    answer. Not hidden, not unexported, not stored — absent. [Name] refuses a
    cause nothing the player pointed at speaks to, which is a statement about
    what was argued rather than about what is true, and it is the only refusal
    that ever touches the reasoning.
 3. **Wrong diagnoses are playable, not blocked.** Naming any reachable cause
    grants that cause's tool, at once, without qualification. What makes a wrong
    one wrong is that living with the tool does not help, which happens in the
    simulation over the following days and is not a thing this package is told
    about.
 4. **Allow multiple valid causes.** An observation may speak to several causes,
    so the same evidence supports several accounts, and the player commits to
    one. Naming a second later keeps the first: a tool arrived at by
    understanding something is not taken back by understanding something else.
 5. **Re-diagnosable.** A conclusion stores what was available at the time, and
    the occasion re-opens exactly when there is more — which is also what stops
    it being a retry loop, since pointing at the same evidence again is refused.

# The occasion is composed, not recorded

A diagnosis is about a failure, and this package stores no failures. It cannot:
the failures worth explaining are already recorded by the modules that own
them — a rule that backfired is a `playbook_fire` row, a run of readings that
turned is `health_sample` — and a copy here would be a second answer that goes
stale (§3.3). So a [Failure] is composed on read by the same ledgers that produce
the evidence, and the durable half of it is its key.

That is also what makes "at the moment of consequence" true rather than
decorative. The scene is available exactly when something of the player's own has
gone badly and they have not accounted for it, and it is unavailable the rest of
the time, because there is nothing to sit down about.

# Everything crosses through Ledger

§3.1 forbids reading another module's tables, and this package would otherwise
want to read four. [Ledger] is the whole of what it can reach: something that
knows a corner of the player's history and can say what went badly in it and
what it observes. internal/health knows what a resting heart rate climbing for a
fortnight means and this package does not, which is the correct distribution of
that knowledge — a threshold on a biomarker is health's business, and the join
between an observation and a cause is content's.

It owns the `diagnosis` table (§3.1), takes the caller's transaction and never
opens one (§3.2).
*/
package diagnosis

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

// The refusals this module has. Go errors rather than status codes: internal/api
// maps them at the edge (§8), so a module never learns HTTP.
var (
	// ErrNoOccasion is naming a cause for a failure that is not open — either
	// nothing went badly, or this one has already been accounted for and
	// nothing new has come in since. It is the request arriving after the world
	// moved, which is a 409 at the edge.
	ErrNoOccasion = errors.New("diagnosis: there is nothing open to account for")
	// ErrNoSuchCause is a cause key nobody authored.
	ErrNoSuchCause = errors.New("diagnosis: no such cause")
	// ErrNotYours is evidence the player does not have. Not a judgement about
	// the reasoning — a claim about rows, and the same refusal a playbook term
	// outside the vocabulary gets.
	ErrNotYours = errors.New("diagnosis: that is not evidence you have")
	// ErrDoesNotSpeak is a cause that nothing the player pointed at supports.
	//
	// The one refusal in this package that is about the argument, and it is
	// worth being exact about what it is not: it does not say the cause is
	// wrong. It says the player has not pointed at anything that bears on it,
	// which is the difference between an unsupported claim and a false one, and
	// the whole of §3.6's second rule lives in that gap.
	ErrDoesNotSpeak = errors.New("diagnosis: nothing you pointed at speaks to that")
	// ErrNowhereToSit is a player living among a population no scene is
	// authored for. A failed build in practice — the catalogue refuses one at
	// boot — kept as an error because a player can move somewhere between a
	// deploy and a request.
	ErrNowhereToSit = errors.New("diagnosis: there is nobody here to say it to")
)

/*
Observation is one thing the player's own history says, in their own words.

The unit the whole mechanic is built out of, and the thing §3.6's first rule
insists on: it is a *reading*, produced by the module that owns the rows, and
never a sentence anybody authored for a player to choose. "You slept under six
hours on nine of the last fourteen nights" is a fact about `health_sample`; it
reads like a log line because it is one.

`Key` is stable across days while the numbers in `Says` are not, which is what
lets a conclusion record what was pointed at and what was available without
storing the sentences. A conclusion is a claim about which readings mattered, not
a snapshot of how they were phrased.
*/
type Observation struct {
	// Key identifies the *kind* of reading — `health/short-nights`, not this
	// week's count. Namespaced by the ledger that produces it, which is what
	// keeps two modules from colliding and what [Check] validates causes
	// against.
	Key string
	// Says is the reading, second person, with the player's own numbers in it.
	Says string
	// From is where it was read off, in the player's words: which log, which
	// instrument. §3.16 ties this to money — a reading nobody bought the
	// ability to see is not an observation the player has, so a ledger simply
	// does not produce it, and this field is how the ones they do have say
	// where they came from.
	From string
}

/*
Failure is a moment the world already recorded going badly.

Composed by a ledger out of its own rows. `Key` has to be stable for as long as
the failure is, because it is the only part of it stored — see the migration.

`Says` is what happened, and it is deliberately a statement of fact with no
account attached. The account is the thing the player is about to give.
*/
type Failure struct {
	Key  string
	At   time.Time
	Says string
}

/*
Ledger is one corner of the player's history, and what it can say about itself.

Two methods, and the split is the one §3.1 asks for: the module that owns rows is
the module that knows what they mean. internal/health decides what counts as a
run of short nights and internal/playbook decides when an answer has been
failing; neither of them knows what a cause is, and this package could not
compute either without becoming a second copy of both.

Both are reads. A ledger is never told a diagnosis happened, which keeps the
dependency one-way and means no module can start behaving differently because
the player concluded something about it.
*/
type Ledger interface {
	// Failed is what has gone badly in this corner of the history, newest
	// first. An empty slice is the ordinary answer.
	Failed(ctx context.Context, tx pgx.Tx, accountID string) ([]Failure, error)
	// Observed is what this history currently says, in the player's words.
	Observed(ctx context.Context, tx pgx.Tx, accountID string) ([]Observation, error)
}

/*
Among is where the player lives, which decides who sits down with them.

The same one-question interface internal/factor reads a population through, and
here for the same reason: this package must not be able to reach a home, a
district or a wallet in order to find out whose kitchen table this happens at.
*/
type Among interface {
	Where(ctx context.Context, tx pgx.Tx, accountID string) (string, error)
}

/*
Granting is where an unlocked tool goes.

Naming a cause hands over playbook vocabulary, and internal/playbook owns the
table that records it. One method, one direction, and nothing comes back: a
package that could read which words the player already has would be a package
that could tell them how much of the vocabulary is left, and §3.14 is explicit
that a locked term is absent rather than hidden.

The routine half of a cause needs no counterpart. What unlocks a routine is
[Diagnoses.Named], read through the gate internal/routine already takes, so
there is nothing to hand over — which is why only one of the two tools has an
interface here.
*/
type Granting interface {
	Unlock(ctx context.Context, tx pgx.Tx, accountID string, terms ...string) error
}

/*
Sitting is the scene as the player meets it: what went badly, who is here, what
they say, and everything the player's own history can offer them.

Composed on read, entirely (§3.3). Nothing on it is stored and nothing on it is
authored except the four lines and the causes' words.
*/
type Sitting struct {
	// Open is the failure being accounted for. Zero when nothing is, which is
	// most days and is not an error — the scene simply is not available.
	Open Failure
	// With is the person, by key, and Scene is what they say.
	With  string
	Scene Scene
	// Evidence is everything the player could point at, newest history first
	// within each ledger, in the order the ledgers were bound.
	Evidence []Observation
	// Causes is every cause at least one available observation speaks to.
	//
	// Note what this is not: a list of options attached to the failure. It is
	// derived from the player's own data, so two players sitting down over the
	// same kind of failure with different histories are not offered the same
	// accounts of it — which is the difference between selecting from your data
	// and selecting from a menu (§3.6, rule 1).
	Causes []Cause
	// Again is whether this is a revisit: the player has concluded something
	// about this failure before, and there is evidence now that they did not
	// have then.
	Again bool
	// Said is what they concluded last time, empty unless Again.
	Said string
}

// Available reports whether there is anything to sit down about.
func (s Sitting) Available() bool { return s.Open.Key != "" }

/*
Conclusion is one thing the player named, kept.

What is not on it is any account of how it turned out. §3.6's second rule again:
the world's response is what reveals whether a conclusion was right, and the
world responds by being the world rather than by writing a verdict here.
*/
type Conclusion struct {
	Occasion string
	Cause    string
	At       time.Time
	Pointed  []string
	had      []string
}

// Diagnoses is the module. It holds no per-player state: the transaction comes
// from the caller and everything else is injected (§3.1).
type Diagnoses struct {
	clk     simulation.Clock
	among   Among
	grants  Granting
	ledgers []Ledger
}

// New binds the module to the clock, to where the player lives, to where an
// unlocked word goes, and to the corners of history a diagnosis is argued from.
func New(clk simulation.Clock, among Among, grants Granting, ledgers ...Ledger) *Diagnoses {
	return &Diagnoses{clk: clk, among: among, grants: grants, ledgers: ledgers}
}

/*
Sit composes the scene: what is open, who is here, and what the player has to
go on.

The order it works in is the order §3.6 puts it. What went badly comes first,
because a diagnosis with no moment of consequence is a form; the evidence comes
second, because it is the player's; and which causes are reachable comes last,
out of the evidence, because causes derived from anything else would be a menu.
*/
func (m *Diagnoses) Sit(ctx context.Context, tx pgx.Tx, accountID string) (Sitting, error) {
	open, err := m.open(ctx, tx, accountID)
	if err != nil {
		return Sitting{}, err
	}

	held, err := m.observed(ctx, tx, accountID)
	if err != nil {
		return Sitting{}, err
	}

	out := Sitting{
		Open:     open.failure,
		Evidence: held,
		Causes:   reachable(held),
		Again:    open.again,
		Said:     open.said,
	}
	if !out.Available() {
		// Nobody is asked for, because nothing is being asked about. A scene
		// composed for a player with nothing open would put a person in a room
		// waiting for a conversation that has no subject.
		return out, nil
	}

	where, err := m.among.Where(ctx, tx, accountID)
	if err != nil {
		return Sitting{}, err
	}
	scene, ok := Staged(where)
	if !ok {
		return Sitting{}, fmt.Errorf("%w: %q", ErrNowhereToSit, where)
	}
	out.With, out.Scene = scene.Who, scene
	return out, nil
}

/*
Name records what the player concluded and hands over what it grants.

Four refusals, and none of them is about whether the conclusion is right:
the occasion has to be open, the cause has to exist, the evidence has to be
the player's, and the cause has to be something the evidence speaks to.

The last one is the closest this package comes to judging an argument, and it
judges only its form. A player pointing at three weeks of collapsing recovery
and naming what they eat is refused — not because that is false, but because
they have not pointed at anything about eating. Pointing at a glucose reading
and naming what they eat is accepted whether or not eating is what is actually
driving the body, and finding out is what the next fortnight is for.

Everything happens in the caller's transaction, so a conclusion that was
recorded without its vocabulary being unlocked is not a state this game has.
*/
func (m *Diagnoses) Name(
	ctx context.Context, tx pgx.Tx, accountID, occasion, cause string, pointed []string,
) (Cause, error) {
	sitting, err := m.Sit(ctx, tx, accountID)
	if err != nil {
		return Cause{}, err
	}
	if !sitting.Available() || sitting.Open.Key != occasion {
		return Cause{}, fmt.Errorf("%w: %q", ErrNoOccasion, occasion)
	}

	named, ok := Known(cause)
	if !ok {
		return Cause{}, fmt.Errorf("%w: %q", ErrNoSuchCause, cause)
	}

	held := make(map[string]bool, len(sitting.Evidence))
	for _, o := range sitting.Evidence {
		held[o.Key] = true
	}
	seen := map[string]bool{}
	kept := make([]string, 0, len(pointed))
	for _, k := range pointed {
		if !held[k] {
			return Cause{}, fmt.Errorf("%w: %q", ErrNotYours, k)
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		kept = append(kept, k)
	}
	if len(kept) == 0 {
		return Cause{}, fmt.Errorf("%w: nothing was pointed at", ErrDoesNotSpeak)
	}
	if !speaks(named, seen) {
		return Cause{}, fmt.Errorf("%w: %q", ErrDoesNotSpeak, cause)
	}

	had := make([]string, 0, len(sitting.Evidence))
	for _, o := range sitting.Evidence {
		had = append(had, o.Key)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO diagnosis (account_id, occasion, cause, at, pointed, had)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		accountID, occasion, named.Key, m.clk.Now(), kept, had,
	); err != nil {
		return Cause{}, fmt.Errorf("diagnosis: record: %w", err)
	}

	if len(named.Terms) > 0 {
		if err := m.grants.Unlock(ctx, tx, accountID, named.Terms...); err != nil {
			return Cause{}, fmt.Errorf("diagnosis: unlock: %w", err)
		}
	}
	return named, nil
}

/*
Named is every routine this player has earned the configuration of.

The read internal/routine's gate is bound to (§3.6). It answers from conclusions
alone: a cause named once is named for good, including one the player has since
revisited and disagreed with, because they did conclude it and they did chase it
and the tool is what that cost them.
*/
func (m *Diagnoses) Named(ctx context.Context, tx pgx.Tx, accountID string) (map[string]bool, error) {
	rows, err := tx.Query(ctx,
		`SELECT DISTINCT cause FROM diagnosis WHERE account_id = $1`, accountID)
	if err != nil {
		return nil, fmt.Errorf("diagnosis: read named: %w", err)
	}
	defer rows.Close()

	out := map[string]bool{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("diagnosis: read named: %w", err)
		}
		// A cause withdrawn from content since it was named unlocks nothing,
		// the same way a renamed playbook term stops being held. The row stays:
		// it is a thing the player did.
		if c, ok := Known(key); ok && c.Routine != "" {
			out[c.Routine] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("diagnosis: read named: %w", err)
	}
	return out, nil
}

/*
Concluded is what this player has named, newest first — the record §3.6's fifth
rule is argued against and the one surface a past conclusion is visible on.
*/
func (m *Diagnoses) Concluded(ctx context.Context, tx pgx.Tx, accountID string) ([]Conclusion, error) {
	rows, err := tx.Query(ctx, `
		SELECT occasion, cause, at, pointed, had
		  FROM diagnosis
		 WHERE account_id = $1
		 ORDER BY at DESC, id DESC`, accountID)
	if err != nil {
		return nil, fmt.Errorf("diagnosis: read conclusions: %w", err)
	}
	defer rows.Close()

	out := []Conclusion{}
	for rows.Next() {
		var c Conclusion
		if err := rows.Scan(&c.Occasion, &c.Cause, &c.At, &c.Pointed, &c.had); err != nil {
			return nil, fmt.Errorf("diagnosis: read conclusions: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("diagnosis: read conclusions: %w", err)
	}
	return out, nil
}

/* ── what is open ──────────────────────────────────────────────────────── */

// opening is a failure and what the player has already made of it.
type opening struct {
	failure Failure
	again   bool
	said    string
}

/*
open finds the failure in front of the player, if there is one.

Newest first across every ledger, and the first one that is either unaccounted
for or revisitable is the one. Only one is ever open: §3.6 stages this at the
moment of consequence, and a queue of failures waiting to be explained would be
a backlog of homework, which is the opposite of a moment.

Revisiting is the fifth rule, and it is a comparison rather than a timer. A
conclusion recorded what was available when it was drawn; if the player's history
now says something it did not say then, there is more to go on and the question
re-opens. If it says the same things, it does not — which is what keeps this from
being the retry loop rule 3 forbids, without anything having to check whether the
last answer was any good.
*/
func (m *Diagnoses) open(ctx context.Context, tx pgx.Tx, accountID string) (opening, error) {
	var failures []Failure
	for _, l := range m.ledgers {
		theirs, err := l.Failed(ctx, tx, accountID)
		if err != nil {
			return opening{}, err
		}
		failures = append(failures, theirs...)
	}
	if len(failures) == 0 {
		return opening{}, nil
	}
	sort.SliceStable(failures, func(i, j int) bool { return failures[i].At.After(failures[j].At) })

	drawn, err := m.Concluded(ctx, tx, accountID)
	if err != nil {
		return opening{}, err
	}
	last := make(map[string]Conclusion, len(drawn))
	for _, c := range drawn {
		// Concluded is newest first, so the first one seen for an occasion is
		// the standing conclusion about it.
		if _, ok := last[c.Occasion]; !ok {
			last[c.Occasion] = c
		}
	}

	held, err := m.observed(ctx, tx, accountID)
	if err != nil {
		return opening{}, err
	}

	for _, f := range failures {
		prior, accounted := last[f.Key]
		if !accounted {
			return opening{failure: f}, nil
		}
		if moved(held, prior.had) {
			said := prior.Cause
			if c, ok := Known(prior.Cause); ok {
				said = c.Says
			}
			return opening{failure: f, again: true, said: said}, nil
		}
	}
	return opening{}, nil
}

// observed is everything every bound ledger currently says.
func (m *Diagnoses) observed(ctx context.Context, tx pgx.Tx, accountID string) ([]Observation, error) {
	var out []Observation
	for _, l := range m.ledgers {
		theirs, err := l.Observed(ctx, tx, accountID)
		if err != nil {
			return nil, err
		}
		out = append(out, theirs...)
	}
	return out, nil
}

// moved reports whether the player has an observation now that they did not have
// when a conclusion was drawn.
//
// One-directional on purpose. Evidence that has *gone* — a run of short nights
// that ended — is not more to go on, and re-opening on it would put the question
// back in front of somebody for having fixed something.
func moved(held []Observation, had []string) bool {
	before := make(map[string]bool, len(had))
	for _, k := range had {
		before[k] = true
	}
	for _, o := range held {
		if !before[o.Key] {
			return true
		}
	}
	return false
}

// reachable is every cause at least one of these observations speaks to, in
// authored order.
func reachable(held []Observation) []Cause {
	have := make(map[string]bool, len(held))
	for _, o := range held {
		have[o.Key] = true
	}
	out := []Cause{}
	for _, c := range causes {
		if speaks(c, have) {
			out = append(out, c)
		}
	}
	return out
}

// speaks reports whether any of these observation keys bears on this cause.
func speaks(c Cause, have map[string]bool) bool {
	for _, k := range c.Speaks {
		if have[k] {
			return true
		}
	}
	return false
}
