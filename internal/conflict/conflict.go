/*
Package conflict is living among people, one day at a time (§3.15).

HighLevelOverview.md §3.15 calls this the engine of the whole game and says so
in as many words: **"daily conflict is the ongoing mechanic. Living among a
population means resolving conflicts with them every day. That is the texture of
an environment and the engine of the whole loop."** The four sentences under it
are this package:

 1. Each day the server generates friction that **follows from actual state** —
    who owes whom, who was slighted, what was left unresolved, who has been
    asked too often.
 2. **Rules auto-resolve what they cover.** The player reviews a log showing the
    situation, which rule fired, the action taken, and the consequence.
 3. **Anything the rules don't cover escalates to the player** as a live
    decision.
 4. So **good rules buy time, and gaps in the playbook are what demand
    attention.**

# It is internal/situation's shape, and deliberately so

A delegated role produces a moment while the player is elsewhere; a population
produces one while the player is living beside it. Both are weighed against the
rules in force where the player is (§3.15), both are recorded in
internal/playbook, both either settle themselves or wait. Where they differ is
the half that makes this the *core progression* rather than one job's texture:

  - **A conflict is with somebody.** It is keyed by a person out of
    `content/environments`, and it is selected by which of them the player has
    the most outstanding with.
  - **A conflict changes what stands between them.** Every answer moves the
    ledger, so tomorrow's friction follows from today's answer. That loop is why
    a rule set can be *wrong here*: an answer that reads as generous on one
    landing reads as a position conceded two floors up, and the ledger records
    the consequence either way without ever saying which it was.

# Two tables and no writing

`conflict_standing` is what is between the player and one person;
`conflict` is what happened on one day. Neither holds any prose — the moments,
the answers and what each of them came to are authored content under
`content/conflicts` (technical.md §5.2). Nothing here is random (§7): the day's
person and the day's pathway are both pure functions of state, the account and
the date, so a rollover that is retried, replayed or run twice produces one
world rather than two.

# Four seams, and none of them is a module

The same four internal/situation takes, minus the ladder and plus nothing. What
a population is comes from internal/environment, which is pure authored content
with no state in it — the same reason this package may read internal/city's
neighbour authored two doors up without an interface standing between them.
*/
package conflict

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/environment"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/playbook"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

// The refusals this module has. Go errors rather than status codes:
// internal/api maps them at the edge (§8), so a module never learns HTTP.
var (
	// ErrNoSuchConflict is a conflict that is not this player's, or not waiting.
	ErrNoSuchConflict = errors.New("conflict: no such conflict")
	// ErrNoSuchAnswer is a posture this conflict was not written an answer for.
	ErrNoSuchAnswer = errors.New("conflict: no such answer")
	// ErrWithdrawn is a conflict whose authored content has gone since it arose.
	// Reachable only across a deploy that removed a pathway somebody was
	// standing in front of.
	ErrWithdrawn = errors.New("conflict: that is no longer something the world has in it")
)

/*
Judgement is the playbook, as this package is allowed to use it.

The same four methods internal/situation takes, and the fence is the same one: a
conflict can put a question, be told what the rules made of it, write that down
and settle it, and it cannot edit the rules it was answered by. Composing the
answer that covers the *next* one is the player's own act on their own screen,
which is the loop (§3.14).
*/
type Judgement interface {
	Weigh(ctx context.Context, tx pgx.Tx, accountID, environment string, s playbook.Situation) (playbook.Decision, error)
	Record(ctx context.Context, tx pgx.Tx, accountID string, at time.Time,
		environment string, d playbook.Decision, outcome playbook.Outcome) (playbook.Fire, error)
	Settle(ctx context.Context, tx pgx.Tx, accountID string, fire int64,
		at time.Time, outcome playbook.Outcome) (playbook.Escalation, bool, error)
	Raised(ctx context.Context, tx pgx.Tx, accountID string, fire int64, at time.Time) (bool, error)
}

/*
Where is which population the player is living among (§3.15).

The same one-question interface internal/factor and internal/situation take for
the same fact, and for the same reason: who you are among follows from where you
live, which is internal/character's, so composing the two is internal/api's job
(§3.1) and this package holds a key.

It is deliberately *not* where the player is standing. §3.15's pressure is that
an environment's ceiling cannot be walked away from, and a conflict that
followed the player's feet would be a conflict they could avoid by spending the
afternoon downtown.
*/
type Where interface {
	Where(ctx context.Context, tx pgx.Tx, accountID string) (string, error)
}

/*
Awards is where an answer's authored deltas go (§3.5).

One method, one way, returning an error rather than the standing internal/factor
hands back — the same adapter internal/situation and internal/challenge take. A
module that could read what it moved on a hidden factor is a module somebody
eventually renders a number for.
*/
type Awards interface {
	Award(ctx context.Context, tx pgx.Tx, accountID string, grants ...simulation.Grant) error
}

// Unpaid is an Awards that moves nothing. For tests about the machinery rather
// than about progression; never bound in the server.
type Unpaid struct{}

func (Unpaid) Award(context.Context, pgx.Tx, string, ...simulation.Grant) error { return nil }

/*
Arisen is what one day among a population came to.

The zero value is a day nothing arose on: the player is among nobody, is already
standing in front of something, or has had this day walked before.
*/
type Arisen struct {
	// Conflict is the authored pathway's key, empty when nothing arose.
	Conflict string
	// Person is who it was with, and Strain which of §3.15's pressures put them
	// at the front of the queue.
	Person string
	Strain Strain
	// Fire is the rule-fire record this wrote, and zero when the moment put a
	// question in an environment nobody asks it in.
	Fire int64
	// Handled is the posture the rules settled it with, empty when they did not.
	Handled playbook.Stance
	// Waiting is whether it is now in front of the player as a live decision.
	Waiting bool
	// Cause is why it is waiting, and empty when it is not.
	Cause playbook.Cause
}

// Arose reports whether the day produced anything at all.
func (a Arisen) Arose() bool { return a.Conflict != "" }

/*
Waiting is a conflict standing in front of the player, and what they may do
about it.

Everything to re-establish it, because this is what somebody who closed the tab
lands back on — the same property §5.1 makes mandatory for a challenge run, and
for the same reason: the world did not pause while they were away.
*/
type Waiting struct {
	ID int64
	// Pathway is the authored moment, and Who the person it is with. Both,
	// because the writing says `{who}` and the name is the population's.
	Pathway Authored
	Who     environment.Person
	// Line is the moment with the name already in it.
	Line   string
	Strain Strain
	At     time.Time
	// On is the player's own day it arose on.
	On simulation.Date
	// Environment is where their rules were read from, by key (§3.15).
	Environment string
	// Cause is why it reached them: nothing covered it, or two answers
	// disagreed. Never [playbook.Backfired] — a rule that fired and went badly
	// has already been carried out, and is told about rather than asked about.
	Cause playbook.Cause
	// Note is that reason in the words the log uses.
	Note string
	// Standing is what was between the two of them when it arose, in the words a
	// player is shown. Sent because a conflict that follows from a debt is only
	// legible as following from one if the debt is on the screen (§3.6).
	Standing []string
}

// Answers is what the player may do about it, with the name filled in.
func (w Waiting) Answers() []Answer {
	out := w.Pathway.Answers()
	for i := range out {
		out[i].Did = Says(out[i].Did, w.Who.Name)
		out[i].Came = Says(out[i].Came, w.Who.Name)
	}
	return out
}

/*
Telling is one settled conflict, as the log reads it back.

The words rather than the keys, because a log line is the writing being reported
rather than assembled — the same argument internal/playbook makes about
rendering a rule on the server (§10.2).
*/
type Telling struct {
	Conflict string
	Name     string
	Line     string
	// Who is the person it was with, by name, and empty for somebody the world
	// no longer authors.
	Who string
	// Did and Came are the authored answer, and are empty for a conflict nobody
	// has answered yet.
	Did  string
	Came string
}

/*
Neighbour is one person and how living beside them has been going.

The counts are the window's rather than all time, because §3.15's pattern is a
week: "one bad outcome is noise, the same rule failing daily for a week is a
pattern — and a pattern is diagnosable."
*/
type Neighbour struct {
	Person   environment.Person
	Standing Standing
	// Met, Good and Poor are conflicts inside the window. Mixed is the
	// difference and is deliberately not sent: three numbers is a shape and four
	// is a scorecard.
	Met  int
	Good int
	Poor int
}

/*
Recurring is how one of the playbook's questions has been going among these
people — §3.6's pattern, counted.

Counts rather than a verdict, for exactly the reason internal/playbook's Trend
is: "the money question came up seven times this week, your rules covered two of
them, and four of the seven went badly" is a thing a player can argue with.
"Your money rule is performing poorly" is the game telling them what to think,
which §3.2 spends its whole length refusing to do.
*/
type Recurring struct {
	Question string
	Met      int
	// Covered is how many the player's own rules settled without asking. The
	// other half is what reached them, which is the number §3.15 says gaps in
	// the playbook are made of.
	Covered int
	Good    int
	Poor    int
}

/*
Living is the whole of who the player is among, and how it has been going.

Composed on read from authored content and two tables (§3.3). Nothing here is
stored: the population is content, the standings are rows, and the counts are a
GROUP BY over the days.
*/
type Living struct {
	// Environment is the population, by key, and Name and Note are what the
	// player reads. Empty when they are among nobody.
	Environment string
	Name        string
	Note        string
	// Holds is what these people have in common, in authored order. On this
	// surface because it is the only place the player is ever told what the
	// rules here are up against, and it is written as behaviour rather than as
	// adjectives — §3.15's "never labelled" is why it can be shown at all.
	Holds []string
	// People is everybody, in authored order, whether or not anything has ever
	// passed between them.
	People []Neighbour
	// Questions is the pattern, by question, over the window.
	Questions []Recurring
	// Since is the first day the counts include.
	Since simulation.Date
}

// Among reports whether the player is among anybody at all.
func (l Living) Among() bool { return l.Environment != "" }

/*
Window is how far back [Conflicts.Living] counts.

A fortnight rather than a week. §3.15's pattern is a week, and a window exactly
one pattern long would show a pattern only on the day it completed — two weeks
is long enough that a player who looks on Thursday can see the week that ended
on Tuesday, and short enough that a rule they revised a month ago is not still
being counted against them.
*/
const Window = 14

// Conflicts is the module. It holds no per-player state: the transaction comes
// from the caller and the three seams are injected (§3.1).
type Conflicts struct {
	clk    simulation.Clock
	rules  Judgement
	where  Where
	awards Awards
}

func New(clk simulation.Clock, rules Judgement, where Where, awards Awards) *Conflicts {
	return &Conflicts{clk: clk, rules: rules, where: where, awards: awards}
}

/*
Day is one day of living among a population, run with nobody watching.

Called by the rollover for every day it walks (§3.3), which is what makes living
somewhere a thing that happens to the player rather than a thing they press.

Four ways it does nothing, and each of them is a rule rather than a guard:

  - **Among nobody.** A player whose home has no population authored around it
    has nobody to have a conflict with. It is a real state — the city has four
    districts and two of them nobody lives in (§3.12).
  - **Something is already waiting.** Nothing new comes up while the last thing
    is unresolved. §3.15's "good rules buy time, and the holes in your playbook
    are exactly what demands your attention", made mechanical: a playbook that
    covers the population keeps the population moving, and one full of holes
    stops it until the player does the work by hand. It is also what bounds a
    long absence — a fortnight away with no rules is one decision waiting, not
    fourteen (§4).
  - **This day has already been walked.** The unique key on (account, day) is
    what makes a retried job and a second tab hydrating produce one conflict
    between them rather than one each.
  - **Nothing authored.** Impossible for a shipped population — the loader
    refuses one with a hole in its pool — and a quiet day rather than a panic in
    a worker for a build that authored a third one badly.
*/
func (m *Conflicts) Day(
	ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date,
) (Arisen, error) {
	where, err := m.where.Where(ctx, tx, accountID)
	if err != nil {
		return Arisen{}, err
	}
	among, live := environment.Lookup(where)
	if !live {
		return Arisen{}, nil
	}

	standings, err := m.standings(ctx, tx, accountID, where)
	if err != nil {
		return Arisen{}, err
	}

	// Whose day it is, and what it is about. Both fall out of state: the person
	// is whoever the player has most outstanding with, and the pathway is drawn
	// from the pool authored for the pressure they are under (§3.15, §4).
	who, standing, anybody := Turn(among.People(), standings, accountID, on)
	if !anybody {
		return Arisen{}, nil
	}
	authored, written := m.pathway(where, standing, accountID, on)
	if !written {
		return Arisen{}, nil
	}

	// The row first, and the judgement after it. Written before anything is
	// weighed so that two rollovers arriving together contend on the unique key
	// rather than on the record — the loser writes no fire, moves no standing
	// and reports nothing, which is exactly what a second walk of the same day
	// is.
	//
	// `waiting` starts true because it is true until this function decides
	// otherwise, and a crash between here and the update leaves a conflict the
	// player can answer rather than one that vanished.
	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO conflict (account_id, on_day, environment, person, strain, conflict, question, waiting)
		VALUES ($1, make_date($2, $3, $4), $5, $6, $7, $8, $9, true)
		ON CONFLICT (account_id, on_day) DO NOTHING
		RETURNING id`,
		accountID, on.Year, int(on.Month), on.Day, where,
		who.Key, string(authored.Follows), authored.Key, authored.Puts,
	).Scan(&id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Arisen{}, nil
	case err != nil:
		return Arisen{}, fmt.Errorf("conflict: raise: %w", err)
	}

	// After the insert, so that the "one at a time" rule is decided by the same
	// row the insert just contended for. A conflict that arrives to find an
	// older one waiting takes itself back: the day produced nothing, which is
	// the state the player's own inaction put them in.
	standing, err = m.hold(ctx, tx, accountID, id, standing)
	if err != nil {
		return Arisen{}, err
	}
	if standing.Person == "" {
		return Arisen{}, nil
	}

	// What simply happening does to the ledger, before the rules see it.
	// Somebody asking you for something has asked you, whatever you answer —
	// and the matcher is entitled to know that this is the fourth time, because
	// "they have asked me three times this month" is a rule a player can write.
	standing = standing.Fresh(on).After(authored.Raises)
	moment := authored.Moment(standing)
	// And having-met-them is counted after the facts are read, so that "you have
	// dealt with them before" is not made true by the thing being dealt with.
	standing = standing.Counted(on)

	now := m.clk.Now()
	decision, err := m.rules.Weigh(ctx, tx, accountID, where, moment)
	if err != nil {
		return Arisen{}, err
	}

	out := Arisen{Conflict: authored.Key, Person: who.Key, Strain: authored.Follows}
	settled := playbook.Pending
	var answer Answer
	if fired, ok := decision.Fired(); ok {
		if answer, ok = authored.Answer(fired.Stance); ok {
			settled, out.Handled = answer.Outcome, fired.Stance
		}
	}

	// The record, before the consequence. §3.14's log is the whole review
	// surface, and a conflict that moved a factor and wrote nothing down would
	// be a consequence the player cannot trace back to the rule that caused it.
	fire, err := m.rules.Record(ctx, tx, accountID, now, where, decision, settled)
	if err != nil {
		return Arisen{}, err
	}
	out.Fire = fire.ID

	if out.Handled != "" {
		standing = standing.After(answer.Shift)
		if err := m.save(ctx, tx, accountID, where, standing); err != nil {
			return Arisen{}, err
		}
		if err := m.awards.Award(ctx, tx, accountID, answer.Moves...); err != nil {
			return Arisen{}, err
		}
		if err := m.close(ctx, tx, id, fire.ID, out.Handled, answer.Outcome, true, now); err != nil {
			return Arisen{}, err
		}
		return out, nil
	}

	// Nothing covered it, or two answers disagreed. It waits, and the standing
	// is saved as it stands — the ask happened whether or not it was answered,
	// and a player who leaves it unresolved should find tomorrow's ledger
	// showing that they did.
	if err := m.save(ctx, tx, accountID, where, standing); err != nil {
		return Arisen{}, err
	}

	// The fire is marked raised here rather than left for §4's release policy,
	// because the conflict *is* the escalation and it is already in front of the
	// player: queueing an event about it as well would put the same thing to
	// them twice under two different names. The third condition — a rule fired
	// and went badly — is not marked, and reaches them the ordinary way.
	out.Waiting, out.Cause = true, causeOf(decision)
	if fire.ID != 0 {
		if _, err := m.rules.Raised(ctx, tx, accountID, fire.ID, now); err != nil {
			return Arisen{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE conflict SET fire_id = $2 WHERE id = $1`, id, fire.ID); err != nil {
			return Arisen{}, fmt.Errorf("conflict: raise: %w", err)
		}
	}
	return out, nil
}

/*
Waiting is the conflict standing in front of this player, if one is.

Oldest first, though there is only ever one — the rule in [Conflicts.Day] is
what makes that true, and reading it this way means a database that somehow held
two hands back the one the player was owed first rather than whichever the
planner returned.
*/
func (m *Conflicts) Waiting(ctx context.Context, tx pgx.Tx, accountID string) (Waiting, bool, error) {
	var (
		out    Waiting
		key    string
		person string
		strain string
		on     time.Time
		fire   *int64
		verd   string
	)
	err := tx.QueryRow(ctx, `
		SELECT c.id, c.conflict, c.person, c.strain, c.on_day, c.created_at, c.environment,
		       c.fire_id, COALESCE(f.verdict, '')
		  FROM conflict c
		  LEFT JOIN playbook_fire f ON f.id = c.fire_id
		 WHERE c.account_id = $1 AND c.waiting
		 ORDER BY c.on_day, c.id
		 LIMIT 1`, accountID,
	).Scan(&out.ID, &key, &person, &strain, &on, &out.At, &out.Environment, &fire, &verd)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Waiting{}, false, nil
	case err != nil:
		return Waiting{}, false, fmt.Errorf("conflict: read waiting: %w", err)
	}

	authored, known := Pathway(out.Environment, key)
	if !known {
		// Authored content withdrawn from under somebody who was standing in
		// front of it. Reported as nothing waiting rather than as an error: the
		// alternative is a world snapshot that fails to load until a deploy is
		// rolled back.
		return Waiting{}, false, nil
	}
	among, live := environment.Lookup(out.Environment)
	if !live {
		return Waiting{}, false, nil
	}
	who, still := among.Person(person)
	if !still {
		// The pathway survived a deploy and the person did not. Same reading as
		// above: there is nobody to have this with any more.
		return Waiting{}, false, nil
	}

	out.Pathway, out.Who, out.Strain = authored, who, Strain(strain)
	out.Line = Says(authored.Line, who.Name)
	// A DATE carries no timezone and needs none: it is already the player's own
	// calendar day, written by whichever rollover walked it.
	out.On = simulation.On(on, time.UTC)
	out.Cause, _ = playbook.Because(playbook.Verdict(verd), playbook.Pending)
	out.Note = playbook.Decision{Verdict: playbook.Verdict(verd)}.Note()

	standings, err := m.standings(ctx, tx, accountID, out.Environment)
	if err != nil {
		return Waiting{}, false, err
	}
	out.Standing = standings[person].Reads()
	return out, true, nil
}

/*
Answer is the player settling one themselves — §3.15's step 3, by hand.

The one path in this package a command drives, and it is deliberately not
"compose a rule and let it run": the conflict already happened, the rules
already failed to cover it, and what is owed is a decision rather than a policy.
Writing the rule so that the *next* one is covered is a different command on a
different screen, which is the loop.

Everything that follows is the same as the covered path — the deltas are paid,
the ledger moves, the record is settled — because the player answering by hand
and their rules answering for them have to produce the same kind of consequence
or the log would be comparing two different things.
*/
func (m *Conflicts) Answer(
	ctx context.Context, tx pgx.Tx, accountID string, id int64, stance playbook.Stance,
) (Answer, error) {
	var (
		key    string
		person string
		where  string
		fire   *int64
	)
	err := tx.QueryRow(ctx, `
		SELECT conflict, person, environment, fire_id FROM conflict
		 WHERE id = $1 AND account_id = $2 AND waiting
		 FOR UPDATE`, id, accountID,
	).Scan(&key, &person, &where, &fire)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Answer{}, fmt.Errorf("%w: %d", ErrNoSuchConflict, id)
	case err != nil:
		return Answer{}, fmt.Errorf("conflict: read waiting: %w", err)
	}

	authored, known := Pathway(where, key)
	if !known {
		return Answer{}, fmt.Errorf("%w: %q", ErrWithdrawn, key)
	}
	answer, written := authored.Answer(stance)
	if !written {
		return Answer{}, fmt.Errorf("%w: %q", ErrNoSuchAnswer, stance)
	}

	now := m.clk.Now()
	standings, err := m.standings(ctx, tx, accountID, where)
	if err != nil {
		return Answer{}, err
	}
	standing := standings[person]
	standing.Person = person
	if err := m.save(ctx, tx, accountID, where, standing.After(answer.Shift)); err != nil {
		return Answer{}, err
	}
	if err := m.awards.Award(ctx, tx, accountID, answer.Moves...); err != nil {
		return Answer{}, err
	}
	// The record was written pending when the conflict arose, because how it
	// went was not known then and is now. A conflict that reached nobody's rules
	// has no record to settle, which is the one case this skips.
	if fire != nil {
		if _, _, err := m.rules.Settle(ctx, tx, accountID, *fire, now, answer.Outcome); err != nil {
			return Answer{}, err
		}
	}
	if err := m.close(ctx, tx, id, 0, stance, answer.Outcome, false, now); err != nil {
		return Answer{}, err
	}

	// The name, filled in, because this is what the player is shown next and the
	// authored line says `{who}`.
	if among, live := environment.Lookup(where); live {
		if who, still := among.Person(person); still {
			answer.Did, answer.Came = Says(answer.Did, who.Name), Says(answer.Came, who.Name)
		}
	}
	return answer, nil
}

/*
Told is what a page of the playbook's log needs about the conflicts behind its
records.

Keyed by fire rather than by pathway, because the log is a page of records and
the same friction can have arisen on many days. What comes back is the writing —
the moment, who it was with, and the answer it was met with — which
internal/playbook has no way to know and no business knowing: it holds which
rule fired and this holds what was going on at the time.
*/
func (m *Conflicts) Told(
	ctx context.Context, tx pgx.Tx, accountID string, fires []int64,
) (map[int64]Telling, error) {
	out := map[int64]Telling{}
	if len(fires) == 0 {
		return out, nil
	}

	rows, err := tx.Query(ctx, `
		SELECT fire_id, environment, conflict, person, stance FROM conflict
		 WHERE account_id = $1 AND fire_id = ANY($2)`, accountID, fires)
	if err != nil {
		return nil, fmt.Errorf("conflict: read tellings: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			fire   int64
			where  string
			key    string
			person string
			stance string
		)
		if err := rows.Scan(&fire, &where, &key, &person, &stance); err != nil {
			return nil, fmt.Errorf("conflict: read tellings: %w", err)
		}
		authored, known := Pathway(where, key)
		if !known {
			// A pathway withdrawn from content since it happened. The key is
			// still the truth about what arose, so it is reported as itself
			// rather than dropped — a log line that quietly lost a moment would
			// be a smaller lie than a wrong one but a lie all the same.
			out[fire] = Telling{Conflict: key}
			continue
		}
		name := ""
		if among, live := environment.Lookup(where); live {
			if who, still := among.Person(person); still {
				name = who.Name
			}
		}
		told := Telling{
			Conflict: key, Name: authored.Name,
			Line: Says(authored.Line, name), Who: name,
		}
		if answer, written := authored.Answer(playbook.Stance(stance)); written {
			told.Did, told.Came = Says(answer.Did, name), Says(answer.Came, name)
		}
		out[fire] = told
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("conflict: read tellings: %w", err)
	}
	return out, nil
}

/*
Living is who the player is among and how it has been going — §3.6's evidence,
for the population rather than for the rule set.

It is the review surface §3.15 asks for on the side the playbook's log cannot
see: the log says which rule fired, and this says which of the six people it
keeps firing about and which question keeps arriving. One bad morning is noise;
"the money question has come up nine times in a fortnight, your rules covered
two of them, and six went badly" is a pattern, and a pattern is diagnosable.

Nothing here is a verdict and nothing here is a score. The counts are counts.
*/
func (m *Conflicts) Living(
	ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date,
) (Living, error) {
	where, err := m.where.Where(ctx, tx, accountID)
	if err != nil {
		return Living{}, err
	}
	among, live := environment.Lookup(where)
	if !live {
		return Living{}, nil
	}

	since := on.Plus(-(Window - 1))
	out := Living{
		Environment: among.Key, Name: among.Name, Note: among.Note,
		Holds: among.Holds(), Since: since,
	}

	standings, err := m.standings(ctx, tx, accountID, where)
	if err != nil {
		return Living{}, err
	}

	met, err := m.tally(ctx, tx, accountID, where, since)
	if err != nil {
		return Living{}, err
	}
	for _, p := range among.People() {
		st := standings[p.Key]
		st.Person = p.Key
		count := met.people[p.Key]
		out.People = append(out.People, Neighbour{
			Person: p, Standing: st,
			Met: count.met, Good: count.good, Poor: count.poor,
		})
	}
	// In the vocabulary's own order rather than by how often each arose, so the
	// pattern is read against a stable list — a table that reordered itself as
	// the week went would be one nobody could compare with last week's.
	for _, q := range playbook.Vocab().Questions() {
		count, asked := met.questions[q.Key]
		if !asked {
			continue
		}
		out.Questions = append(out.Questions, Recurring{
			Question: q.Key, Met: count.met, Covered: count.covered,
			Good: count.good, Poor: count.poor,
		})
	}
	return out, nil
}

/* ── the columns ──────────────────────────────────────────────────────── */

// close settles a row: what was done, how it went, whether the player's own
// rules did it, and that it is no longer waiting. `fire` is written only when
// the caller has one to attach.
func (m *Conflicts) close(
	ctx context.Context, tx pgx.Tx, id, fire int64,
	stance playbook.Stance, outcome playbook.Outcome, covered bool, at time.Time,
) error {
	if _, err := tx.Exec(ctx, `
		UPDATE conflict
		   SET stance     = $2,
		       outcome    = $3,
		       covered    = $4,
		       waiting    = false,
		       decided_at = $5,
		       fire_id    = COALESCE(NULLIF($6, 0), fire_id)
		 WHERE id = $1`,
		id, string(stance), string(outcome), covered, at, fire,
	); err != nil {
		return fmt.Errorf("conflict: settle: %w", err)
	}
	return nil
}

/*
hold takes the day back if the player is already standing in front of something.

It returns the standing untouched when the day may go ahead, and a zero one when
it may not — so the caller has one branch rather than two, and the row that was
just written is deleted here rather than left for a sweeper.
*/
func (m *Conflicts) hold(
	ctx context.Context, tx pgx.Tx, accountID string, not int64, standing Standing,
) (Standing, error) {
	var any bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM conflict
			 WHERE account_id = $1 AND waiting AND id <> $2
		)`, accountID, not,
	).Scan(&any); err != nil {
		return Standing{}, fmt.Errorf("conflict: read waiting: %w", err)
	}
	if !any {
		return standing, nil
	}
	if _, err := tx.Exec(ctx, `DELETE FROM conflict WHERE id = $1`, not); err != nil {
		return Standing{}, fmt.Errorf("conflict: withdraw: %w", err)
	}
	return Standing{}, nil
}

// standings reads what stands between this player and everybody in one
// population, by person key.
//
// Absent is the zero value rather than an error: a row is written the first time
// anything passes between two people, and a population the player has just moved
// among has none.
func (m *Conflicts) standings(
	ctx context.Context, tx pgx.Tx, accountID, where string,
) (map[string]Standing, error) {
	rows, err := tx.Query(ctx, `
		SELECT person, owed, slighted, unsettled, asked, asked_in, met, last_day
		  FROM conflict_standing
		 WHERE account_id = $1 AND environment = $2`, accountID, where)
	if err != nil {
		return nil, fmt.Errorf("conflict: read standing: %w", err)
	}
	defer rows.Close()

	out := map[string]Standing{}
	for rows.Next() {
		var (
			s     Standing
			month *time.Time
			last  *time.Time
		)
		if err := rows.Scan(&s.Person, &s.Owed, &s.Slighted, &s.Unsettled,
			&s.Asked, &month, &s.Met, &last); err != nil {
			return nil, fmt.Errorf("conflict: read standing: %w", err)
		}
		// DATEs carry no timezone and need none: both are already the player's
		// own calendar days, written by whichever rollover walked them.
		if month != nil {
			s.Month = simulation.On(*month, time.UTC)
		}
		if last != nil {
			s.Last = simulation.On(*last, time.UTC)
		}
		out[s.Person] = s
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("conflict: read standing: %w", err)
	}
	return out, nil
}

// save writes one standing, creating it if this is the first thing that has ever
// passed between the two of them.
func (m *Conflicts) save(
	ctx context.Context, tx pgx.Tx, accountID, where string, s Standing,
) error {
	if s.Person == "" {
		return nil
	}
	var month, last *time.Time
	if s.Month != (simulation.Date{}) {
		at := s.Month.At()
		month = &at
	}
	if s.Last != (simulation.Date{}) {
		at := s.Last.At()
		last = &at
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO conflict_standing
			(account_id, environment, person, owed, slighted, unsettled, asked, asked_in, met, last_day)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (account_id, environment, person) DO UPDATE
		   SET owed       = EXCLUDED.owed,
		       slighted   = EXCLUDED.slighted,
		       unsettled  = EXCLUDED.unsettled,
		       asked      = EXCLUDED.asked,
		       asked_in   = EXCLUDED.asked_in,
		       met        = EXCLUDED.met,
		       last_day   = EXCLUDED.last_day,
		       updated_at = now()`,
		accountID, where, s.Person, s.Owed, s.Slighted, s.Unsettled, s.Asked, month, s.Met, last,
	); err != nil {
		return fmt.Errorf("conflict: write standing: %w", err)
	}
	return nil
}

// counted is one row of the window's arithmetic.
type counted struct{ met, covered, good, poor int }

// tallied is the window, by person and by question.
type tallied struct {
	people    map[string]counted
	questions map[string]counted
}

// tally counts a window of days among one population, both ways at once.
//
// Two queries rather than one pass in Go, because counting is what a database
// is for and because a client that counted rows itself would be computing a
// game rule (§10.2) — the same argument internal/playbook's Evidence makes.
func (m *Conflicts) tally(
	ctx context.Context, tx pgx.Tx, accountID, where string, since simulation.Date,
) (tallied, error) {
	out := tallied{people: map[string]counted{}, questions: map[string]counted{}}
	for _, q := range []struct {
		by   string
		into map[string]counted
	}{
		{by: "person", into: out.people},
		{by: "question", into: out.questions},
	} {
		rows, err := tx.Query(ctx, `
			SELECT `+q.by+`,
			       count(*),
			       count(*) FILTER (WHERE covered),
			       count(*) FILTER (WHERE outcome = 'good'),
			       count(*) FILTER (WHERE outcome = 'poor')
			  FROM conflict
			 WHERE account_id = $1 AND environment = $2 AND on_day >= make_date($3, $4, $5)
			 GROUP BY `+q.by, accountID, where, since.Year, int(since.Month), since.Day)
		if err != nil {
			return tallied{}, fmt.Errorf("conflict: count days: %w", err)
		}
		for rows.Next() {
			var (
				key string
				c   counted
			)
			if err := rows.Scan(&key, &c.met, &c.covered, &c.good, &c.poor); err != nil {
				rows.Close()
				return tallied{}, fmt.Errorf("conflict: count days: %w", err)
			}
			q.into[key] = c
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return tallied{}, fmt.Errorf("conflict: count days: %w", err)
		}
	}
	return out, nil
}

/*
pathway is which authored moment a person's pressure produces.

The walk is [Standing.Under]'s order, which is the design's own ranking of what
is most overdue between two people, and it stops at the first pressure the
population has writing for. [Easy] is last and every population has some, so
this always finds something for a shipped build.

Which of a pool is taken is a hash of the account, the day, the person and the
pressure — deterministic (§7), and including the person is what stops two people
under the same pressure being handed the same moment on the same morning.
*/
func (m *Conflicts) pathway(
	where string, standing Standing, accountID string, on simulation.Date,
) (Authored, bool) {
	for _, strain := range standing.Under() {
		pool := For(where, strain)
		if len(pool) == 0 {
			continue
		}
		at := shuffle(accountID, on, standing.Person+"|"+string(strain)) % uint32(len(pool))
		return pool[at], true
	}
	return Authored{}, false
}

// causeOf is why a conflict is waiting, in internal/playbook's own vocabulary.
//
// It reads the decision rather than being told, so that the reason the player is
// looking at this and the reason the record gives are one answer.
func causeOf(d playbook.Decision) playbook.Cause {
	cause, _ := d.Escalation()
	return cause
}
