/*
Package conflict is living among people, one day at a time (§3.15).

Each day the server generates friction that follows from actual state — who owes
whom, who was slighted, what was left unresolved, who has been asked too often.
The playbook's rules auto-resolve what they cover; anything they do not escalates
to the player as a live decision. Good rules buy time, and gaps in the playbook
are what demand attention.

The shape is internal/situation's, with two differences that make this the core
progression rather than one job's texture: a conflict is with somebody, keyed by
a person out of content/environments, and every answer moves what stands between
them, so tomorrow's friction follows from today's answer.

Two tables: conflict_standing is what is between the player and one person,
conflict is what happened on one day. Neither holds prose — the moments, answers
and outcomes are authored content under content/conflicts (technical.md §5.2).
Nothing here is random (§7): the day's person and pathway are pure functions of
state, the account and the date, so a retried or replayed rollover produces one
world rather than two.
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

// Judgement is the playbook, as this package is allowed to use it: a conflict
// can put a question, be told what the rules made of it, write that down and
// settle it. It cannot edit the rules it was answered by — composing the answer
// that covers the next one is the player's own act (§3.14).
type Judgement interface {
	Weigh(ctx context.Context, tx pgx.Tx, accountID, environment string, s playbook.Situation) (playbook.Decision, error)
	Record(ctx context.Context, tx pgx.Tx, accountID string, at time.Time,
		environment string, d playbook.Decision, outcome playbook.Outcome) (playbook.Fire, error)
	Settle(ctx context.Context, tx pgx.Tx, accountID string, fire int64,
		at time.Time, outcome playbook.Outcome) (playbook.Escalation, bool, error)
	Raised(ctx context.Context, tx pgx.Tx, accountID string, fire int64, at time.Time) (bool, error)
}

// Where is which population the player is living among (§3.15) — deliberately
// not where they are standing. An environment's ceiling cannot be walked away
// from, and a conflict that followed the player's feet could be avoided by
// spending the afternoon downtown.
type Where interface {
	Where(ctx context.Context, tx pgx.Tx, accountID string) (string, error)
}

// Awards is where an answer's authored deltas go (§3.5). One way, returning an
// error rather than the standing internal/factor hands back: a module that
// could read what it moved on a hidden factor is one somebody eventually
// renders a number for.
type Awards interface {
	Award(ctx context.Context, tx pgx.Tx, accountID string, grants ...simulation.Grant) error
}

// Presence is whether the player is at the keyboard right now (§3.15), which
// decides who answers a conflict: away, and the character handles it out of the
// playbook; here, and it goes to the phone.
//
// It takes the caller's transaction so the answer is the one true inside the
// transaction the conflict is written in. Asking outside it leaves a window in
// which a player who signed off between the two reads gets a message nobody was
// there to see.
type Presence interface {
	Present(ctx context.Context, tx pgx.Tx, accountID string) (bool, error)
}

// Alone is a Presence that says nobody is ever there, which routes every
// conflict through the playbook. For tests about the machinery rather than
// about the rule; never bound in the server.
type Alone struct{}

func (Alone) Present(context.Context, pgx.Tx, string) (bool, error) { return false, nil }

// Unpaid is an Awards that moves nothing. For tests about the machinery rather
// than about progression; never bound in the server.
type Unpaid struct{}

func (Unpaid) Award(context.Context, pgx.Tx, string, ...simulation.Grant) error { return nil }

// Arisen is what one day among a population came to. The zero value is a day
// nothing arose on: the player is among nobody, is already standing in front of
// something, or has had this day walked before.
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
	// Deferred is whether it is waiting because the player was there (§3.15's
	// presence rule) rather than because their rules could not settle it: a
	// conversation they are having, not a hole in their playbook.
	Deferred bool
	// Fallback is what the rules would have done, kept against the player
	// walking away without answering. Empty when the rules had no answer.
	Fallback playbook.Stance
}

// Arose reports whether the day produced anything at all.
func (a Arisen) Arose() bool { return a.Conflict != "" }

// Waiting is a conflict standing in front of the player, and what they may do
// about it. It carries everything needed to re-establish the moment, because
// this is what somebody who closed the tab lands back on (§5.1).
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
	// Deferred is whether this is waiting because the player was here rather
	// than because their rules failed. Not an escalation, and the surface has to
	// be able to say so.
	Deferred bool
	// Standing is what was between the two of them when it arose, in the words a
	// player is shown. A conflict that follows from a debt is only legible as
	// following from one if the debt is on the screen (§3.6).
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

// Telling is one settled conflict, as the log reads it back: the words rather
// than the keys, because a log line is the writing being reported rather than
// assembled (§10.2).
type Telling struct {
	Conflict string
	Name     string
	Line     string
	// Who is the person it was with, by name, and empty for somebody the world
	// no longer authors.
	Who string
	// Message is the row this was, and Environment and Person whose thread it is
	// in. Keys rather than words, so a log line can send the player to the
	// conversation it is part of rather than leaving them to scroll for it.
	Message     int64
	Environment string
	Person      string
	// Did and Came are the authored answer, and are empty for a conflict nobody
	// has answered yet.
	Did  string
	Came string
	// Title is what an escalation about this answer is called: what the
	// character did, in one line, with the person named — see [Answer.Title].
	// Empty for a conflict nobody has answered and for one whose answer
	// authors none.
	Title string
}

// Message is one dealing with one person, as a thread reads it back: what they
// said, what your character said back, and what they sent after that. All three
// are the writing being reported; this composes nothing.
//
// The distinction the phone is built on is Covered. A message your rules
// answered is one your character replied to while you were elsewhere, and
// reading the thread is how you find out what they said in your name (§3.14).
type Message struct {
	ID int64
	// At is when it arose, and On the player's own day it arose on.
	At time.Time
	On simulation.Date
	// Name is the pathway's title and Says the moment in their words, with the
	// name already in it.
	Name string
	Says string
	// Did and Came are the answer and what followed. Both empty while it is
	// still waiting on the player.
	Did  string
	Came string
	// Covered is whether the playbook answered without asking.
	Covered bool
	// Outcome is good | mixed | poor, and pending while it waits.
	Outcome playbook.Outcome
	// Waiting is whether this is the one still in front of the player. At most
	// one message in the whole phone is, because nothing new arises until it is
	// dealt with.
	Waiting bool
	// Answers is what the player may do about it, in the person's own words, and
	// empty for every message that is not waiting.
	Answers []Answer
}

// Thread is one person and everything that has passed between you —
// deliberately only what happened, with no line saying what the person is like.
// What a character is, this game says by what they do (§3.2).
//
// Composed on read from authored content and one table (§3.3), and fetched
// rather than hydrated: a thread grows for as long as somebody lives somewhere
// and a world model is bounded (§10.2).
type Thread struct {
	Person environment.Person
	// Environment is the population they belong to. Kept because a player who
	// has moved still has the threads from where they used to live, and two
	// populations may key a person the same way.
	Environment string
	// Among is that population as a person reads it, so a phone looking at
	// threads from two districts can say which people a quiet thread is quiet
	// with.
	Among string
	// Here is whether this is somebody they still live among. A thread with
	// somebody they left is history and cannot receive anything new.
	Here bool
	// Intro is how the two of them met, read straight off the authored person
	// (internal/environment) rather than out of a row. It heads the thread and
	// is the same for every player. Empty for somebody from a population being
	// retired.
	Intro []environment.Turn
	// Messages is oldest first, which is the order a conversation is read in.
	Messages []Message
	// Waiting is whether something in here is still in front of the player, and
	// Last when anything last passed between them.
	Waiting bool
	Last    time.Time
}

// Neighbour is one person and how living beside them has been going. The counts
// cover [Window] rather than all time: one bad outcome is noise, the same rule
// failing daily for a week is a diagnosable pattern (§3.15).
type Neighbour struct {
	Person   environment.Person
	Standing Standing
	// Met, Good and Poor are conflicts inside the window. Mixed is the
	// difference and is deliberately not sent: three numbers is a shape, four is
	// a scorecard.
	Met  int
	Good int
	Poor int
}

// Recurring is how one of the playbook's questions has been going among these
// people — §3.6's pattern, counted.
//
// Counts rather than a verdict: "the money question came up seven times, your
// rules covered two, and four went badly" is something a player can argue with.
// "Your money rule is performing poorly" is the game telling them what to think
// (§3.2).
type Recurring struct {
	Question string
	Met      int
	// Covered is how many the player's own rules settled without asking. The
	// rest is what reached them — the gaps in the playbook (§3.15).
	Covered int
	Good    int
	Poor    int
}

// Living is the whole of who the player is among, and how it has been going.
// Composed on read from authored content and two tables (§3.3): the population
// is content, the standings are rows, the counts are a GROUP BY over the days.
type Living struct {
	// Environment is the population, by key, and Name and Note are what the
	// player reads. Empty when they are among nobody.
	Environment string
	Name        string
	Note        string
	// Holds is what these people have in common, in authored order — the only
	// place the player is told what the rules here are up against. Written as
	// behaviour rather than adjectives, which is why it can be shown at all
	// (§3.15).
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

// Window is how far back [Conflicts.Living] counts. A fortnight rather than
// §3.15's week: a window exactly one pattern long shows a pattern only on the
// day it completes, and a longer one keeps counting rules the player has since
// revised.
const Window = 14

// Conflicts is the module. It holds no per-player state: the transaction comes
// from the caller and the three seams are injected (§3.1).
type Conflicts struct {
	clk    simulation.Clock
	rules  Judgement
	where  Where
	awards Awards
	here   Presence
}

func New(clk simulation.Clock, rules Judgement, where Where, awards Awards, here Presence) *Conflicts {
	return &Conflicts{clk: clk, rules: rules, where: where, awards: awards, here: here}
}

/*
Day is one day of living among a population, run with nobody watching. Called by
the rollover for every day it walks (§3.3), and it returns every conflict the
day produced, in order.

It walks the population, and two rules stop the walk rather than a count:

  - One conflict per person per day. A day is bounded by how many people the
    player lives among. That is also the unique key the insert contends on, so a
    retried job and a second tab hydrating produce one row between them.
  - Something is waiting. The walk stops at the first conflict the player's
    rules could not settle (see [Conflicts.hold]). A playbook that covers the
    population absorbs a morning of conversations quietly; one hole stops the
    day. That is also what bounds a long absence — a fortnight away with no
    rules is one decision waiting, not fourteen (§4).

Two ways it does nothing, both real states rather than guards: the player is
among nobody (two of the city's four districts have no population, §3.12), or
the population authored nothing usable, which the loader already refuses for a
shipped build.
*/
func (m *Conflicts) Day(
	ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date,
) ([]Arisen, error) {
	where, err := m.where.Where(ctx, tx, accountID)
	if err != nil {
		return nil, err
	}
	among, live := environment.Lookup(where)
	if !live {
		return nil, nil
	}

	// What the player was handed and did not answer, before anything new is
	// generated. A conflict deferred to somebody who has since left is a
	// conflict their rules should have taken, and the day it belongs to is over.
	if err := m.Lapse(ctx, tx, accountID); err != nil {
		return nil, err
	}

	var out []Arisen
	// Who has already had their turn today. Held here rather than read back off
	// the standings, because [Standing.Since] going to zero only *demotes* a
	// person — somebody carrying a real debt could still top the queue twice —
	// and "not twice in one day" is a rule about the day rather than a tendency
	// of the sort.
	met := map[string]bool{}
	for {
		arisen, more, err := m.arise(ctx, tx, accountID, where, among, met, on)
		if err != nil {
			return nil, err
		}
		if arisen.Arose() {
			out = append(out, arisen)
		}
		if !more {
			return out, nil
		}
	}
}

/*
arise is one conversation, and whether the day may go on to the next.

The standings are re-read every pass rather than carried, because the conflict
just resolved moved them: who turns up next is decided by what the last one did,
so a morning is a sequence rather than a batch drawn against one snapshot.
*/
func (m *Conflicts) arise(
	ctx context.Context, tx pgx.Tx, accountID, where string,
	among environment.Environment, met map[string]bool, on simulation.Date,
) (Arisen, bool, error) {
	standings, err := m.standings(ctx, tx, accountID, where)
	if err != nil {
		return Arisen{}, false, err
	}
	// Rolled forward to the day being generated, before anything is ranked or
	// drawn. [Standing.Asked] counts within a month and the row keeps last
	// month's until something rewrites it, so a stale standing would read as
	// [Asking] on the 1st. [Turn] ranks on [Standing.Pressure] and
	// [Conflicts.pathway] draws its pool from [Standing.Under], so both readings
	// below depend on this.
	for key, was := range standings {
		standings[key] = was.Fresh(on)
	}
	// Re-read every pass: the conversation just resolved is now part of what has
	// passed with that person, and may have unlocked a pathway somebody else's
	// turn can reach this morning.
	past, err := m.history(ctx, tx, accountID, where)
	if err != nil {
		return Arisen{}, false, err
	}

	// Whose turn it is, and what it is about. Both fall out of state: the person
	// is whoever the player has most outstanding with among those who have not
	// had their turn today, and the pathway is drawn from what that person has
	// left to say under the pressure they are under (§3.15, §4).
	who, standing, anybody := Turn(remaining(among.People(), met), standings, accountID, on)
	if !anybody {
		return Arisen{}, false, nil
	}
	met[who.Key] = true
	authored, written := m.pathway(where, standing, past[who.Key], accountID, on)
	if !written {
		// Nothing left with this person today — every thread of theirs run out
		// or waiting on a day that has not come round. The walk goes on: the
		// next person still has their turn coming, and met is already set so the
		// loop cannot come back to this one.
		return Arisen{}, true, nil
	}
	return m.raise(ctx, tx, accountID, where, who, standing, authored, on)
}

/*
raise is one conversation with the selection already made, and the bool is
whether the day may go on to the next person.

The game reaches it through [Conflicts.arise], which does the choosing;
[Conflicts.Force] reaches it with the choice supplied, which is the whole of
what the developer's toolbar does differently.
*/
func (m *Conflicts) raise(
	ctx context.Context, tx pgx.Tx, accountID, where string,
	who environment.Person, standing Standing, authored Authored, on simulation.Date,
) (Arisen, bool, error) {
	// The row first, and the judgement after it, so two rollovers arriving
	// together contend on the unique key rather than on the record: the loser
	// writes no fire, moves no standing and reports nothing. The key is
	// (account, day, person) — this person's one turn today is what is contended
	// for, not the day itself.
	//
	// waiting starts true because a crash between here and the update should
	// leave a conflict the player can answer rather than one that vanished.
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO conflict (account_id, on_day, environment, person, strain, conflict, question, waiting)
		VALUES ($1, make_date($2, $3, $4), $5, $6, $7, $8, $9, true)
		ON CONFLICT (account_id, on_day, person) DO NOTHING
		RETURNING id`,
		accountID, on.Year, int(on.Month), on.Day, where,
		who.Key, string(authored.Follows), authored.Key, authored.Puts,
	).Scan(&id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Somebody else already gave this person their turn today. The walk goes
		// on: contention is person by person, and the people it has not reached
		// yet still have their turn coming.
		return Arisen{}, true, nil
	case err != nil:
		return Arisen{}, false, fmt.Errorf("conflict: raise: %w", err)
	}

	// After the insert, so the "one at a time" rule is decided by the row the
	// insert just contended for. A conflict that arrives to find an older one
	// waiting takes itself back, and the day stops here because everything after
	// it would do the same.
	standing, err = m.hold(ctx, tx, accountID, id, standing)
	if err != nil {
		return Arisen{}, false, err
	}
	if standing.Person == "" {
		return Arisen{}, false, nil
	}

	// What simply happening does to the ledger, before the rules see it:
	// somebody asking you for something has asked you, whatever you answer, and
	// "they have asked me three times this month" is a rule a player can write.
	standing = standing.Fresh(on).After(authored.Raises)
	moment := authored.Moment(standing)
	// Counted after the facts are read, so "you have dealt with them before" is
	// not made true by the thing being dealt with.
	standing = standing.Counted(on)

	now := m.clk.Now()
	decision, err := m.rules.Weigh(ctx, tx, accountID, where, moment)
	if err != nil {
		return Arisen{}, false, err
	}

	out := Arisen{Conflict: authored.Key, Person: who.Key, Strain: authored.Follows}
	settled := playbook.Pending
	var answer Answer
	if fired, ok := decision.Fired(); ok {
		if answer, ok = authored.Answer(fired.Stance); ok {
			settled, out.Handled = answer.Outcome, fired.Stance
		}
	}

	// Who answers it: away, and the character handles it out of the playbook;
	// here, and it goes to the phone and waits for them.
	//
	// The rules are weighed either way, which is why this sits below the
	// decision. The log stays whole, so "my rules would have said yes and I said
	// no" is visible rather than a gap; and the answer they would have given is
	// what the conflict falls back on if they walk away (see [Conflicts.Lapse]).
	present, err := m.here.Present(ctx, tx, accountID)
	if err != nil {
		return Arisen{}, false, err
	}
	if present {
		// Held back rather than applied: nothing has happened yet, so the posture
		// the rules reached is carried to the row instead of to the world.
		out.Deferred, out.Fallback = true, out.Handled
		out.Handled, settled = "", playbook.Pending
	}

	// The record, before the consequence. A conflict that moved a factor and
	// wrote nothing down is a consequence the player cannot trace back to the
	// rule that caused it (§3.14).
	fire, err := m.rules.Record(ctx, tx, accountID, now, where, decision, settled)
	if err != nil {
		return Arisen{}, false, err
	}
	out.Fire = fire.ID

	if out.Handled != "" {
		standing = standing.After(answer.Shift)
		if err := m.save(ctx, tx, accountID, where, standing); err != nil {
			return Arisen{}, false, err
		}
		if err := m.awards.Award(ctx, tx, accountID, answer.Moves...); err != nil {
			return Arisen{}, false, err
		}
		if err := m.close(ctx, tx, id, fire.ID, out.Handled, answer.Outcome, true, now); err != nil {
			return Arisen{}, false, err
		}
		// The rules covered it, so the next person may have their turn — the
		// whole of "good rules buy time".
		return out, true, nil
	}

	// It waits — either because the player is here and it is theirs to answer,
	// or because nothing covered it. The standing is saved as it stands: the ask
	// happened whether or not it was answered.
	if err := m.save(ctx, tx, accountID, where, standing); err != nil {
		return Arisen{}, false, err
	}

	// Which of the two it is, on the row, because [Conflicts.Lapse] has to tell
	// them apart later.
	if out.Deferred {
		if _, err := tx.Exec(ctx,
			`UPDATE conflict SET deferred = true, fallback = $2 WHERE id = $1`,
			id, string(out.Fallback),
		); err != nil {
			return Arisen{}, false, fmt.Errorf("conflict: defer: %w", err)
		}
	}

	// The fire is marked raised here rather than left for §4's release policy:
	// the conflict is the escalation and is already in front of the player, so
	// queueing an event about it would put the same thing to them twice. A rule
	// that fired and went badly is not marked, and reaches them the ordinary
	// way.
	out.Waiting, out.Cause = true, causeOf(decision)
	if fire.ID != 0 {
		// The link first, and unconditionally: a deferred conflict is not an
		// escalation but it is still the record [Conflicts.Lapse] settles and
		// the log reads back.
		if _, err := tx.Exec(ctx, `UPDATE conflict SET fire_id = $2 WHERE id = $1`, id, fire.ID); err != nil {
			return Arisen{}, false, fmt.Errorf("conflict: raise: %w", err)
		}
		// Raised only for a genuine escalation. A conversation held back because
		// the player was at the keyboard is not a time their playbook let them
		// down, and §3.6's diagnosis counts these.
		if !out.Deferred || out.Fallback == "" {
			if _, err := m.rules.Raised(ctx, tx, accountID, fire.ID, now); err != nil {
				return Arisen{}, false, err
			}
		}
	}
	// And the day stops: [Conflicts.hold] would withdraw whatever came next
	// anyway, so walking on costs a row written and deleted per neighbour.
	return out, false, nil
}

/*
Lapse hands back to the playbook whatever the player was given and did not
answer, settling it with the answer the rules reached when it arrived.

Not answering is not an outcome: a player who walks away has gone out, and going
out is what their playbook exists to cover. The record says covered, because the
rules dealt with it.

Two things it will not touch. Anybody still here — presence is asked first, and
the mechanic is "your rules cover you while you are out", not "answer within five
minutes". And anything with no fallback: a conflict waiting because the playbook
was silent or contradicted itself has no answer to lapse to, so it stays in front
of the player.

Called at the top of [Conflicts.Day] and idempotent: a second pass finds nothing
waiting and deferred.
*/
func (m *Conflicts) Lapse(ctx context.Context, tx pgx.Tx, accountID string) error {
	present, err := m.here.Present(ctx, tx, accountID)
	if err != nil {
		return err
	}
	if present {
		return nil
	}

	rows, err := tx.Query(ctx, `
		SELECT id, environment, person, conflict, fallback, fire_id
		  FROM conflict
		 WHERE account_id = $1 AND waiting AND deferred AND fallback <> ''
		 ORDER BY on_day, id
		 FOR UPDATE`, accountID)
	if err != nil {
		return fmt.Errorf("conflict: read deferred: %w", err)
	}
	type lapsing struct {
		id     int64
		where  string
		person string
		key    string
		stance playbook.Stance
		fire   *int64
	}
	var all []lapsing
	for rows.Next() {
		var one lapsing
		var stance string
		if err := rows.Scan(&one.id, &one.where, &one.person, &one.key, &stance, &one.fire); err != nil {
			rows.Close()
			return fmt.Errorf("conflict: read deferred: %w", err)
		}
		one.stance = playbook.Stance(stance)
		all = append(all, one)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return fmt.Errorf("conflict: read deferred: %w", err)
	}

	now := m.clk.Now()
	for _, one := range all {
		authored, known := Pathway(one.where, one.key)
		if !known {
			// Content withdrawn from under a deferred conflict. Left waiting
			// rather than settled arbitrarily: [Conflicts.Waiting] reads an
			// unknown pathway as nothing waiting, so it leaves the phone without
			// this inventing an outcome for it.
			continue
		}
		answer, written := authored.Answer(one.stance)
		if !written {
			continue
		}

		standings, err := m.standings(ctx, tx, accountID, one.where)
		if err != nil {
			return err
		}
		standing := standings[one.person]
		standing.Person = one.person
		if err := m.save(ctx, tx, accountID, one.where, standing.After(answer.Shift)); err != nil {
			return err
		}
		if err := m.awards.Award(ctx, tx, accountID, answer.Moves...); err != nil {
			return err
		}
		if one.fire != nil {
			if _, _, err := m.rules.Settle(ctx, tx, accountID, *one.fire, now, answer.Outcome); err != nil {
				return err
			}
		}
		// covered is true: the player's own rules settled this, which is what
		// §3.15's "how many did your rules cover" count has to agree with.
		if err := m.close(ctx, tx, one.id, 0, one.stance, answer.Outcome, true, now); err != nil {
			return err
		}
	}
	return nil
}

/*
Deferring is every player holding a conflict their rules could take back — the
sweep's own read (see internal/jobs' Lapse).

[Conflicts.Lapse] runs at the top of [Conflicts.Day] and a day is walked once, so
a player handed a message at nine who shuts the laptop at five past would keep it
until midnight. Something has to notice they left.

It does not ask who is present: that is per-account and is asked inside the
transaction the lapse happens in, where it cannot change in between.
*/
func (m *Conflicts) Deferring(ctx context.Context, tx pgx.Tx) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT account_id::text
		  FROM conflict
		 WHERE waiting AND deferred AND fallback <> ''`)
	if err != nil {
		return nil, fmt.Errorf("conflict: read deferring: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("conflict: read deferring: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("conflict: read deferring: %w", err)
	}
	return out, nil
}

// remaining is everybody who has not had their turn today, in authored order.
func remaining(people []environment.Person, met map[string]bool) []environment.Person {
	out := make([]environment.Person, 0, len(people))
	for _, p := range people {
		if !met[p.Key] {
			out = append(out, p)
		}
	}
	return out
}

// Waiting is the conflict standing in front of this player, if one is. A day
// produces several conflicts but at most one is ever waiting, since
// [Conflicts.hold] stops the walk at the first the rules could not settle.
// Oldest first regardless, so a database holding two hands back the one the
// player was owed first rather than whichever the planner returned.
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
		       c.fire_id, COALESCE(f.verdict, ''), c.deferred
		  FROM conflict c
		  LEFT JOIN playbook_fire f ON f.id = c.fire_id
		 WHERE c.account_id = $1 AND c.waiting
		 ORDER BY c.on_day, c.id
		 LIMIT 1`, accountID,
	).Scan(&out.ID, &key, &person, &strain, &on, &out.At, &out.Environment, &fire, &verd, &out.Deferred)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Waiting{}, false, nil
	case err != nil:
		return Waiting{}, false, fmt.Errorf("conflict: read waiting: %w", err)
	}

	authored, known := Pathway(out.Environment, key)
	if !known {
		// Authored content withdrawn from under somebody standing in front of
		// it. Nothing waiting rather than an error: the alternative is a world
		// snapshot that fails to load until a deploy is rolled back.
		return Waiting{}, false, nil
	}
	among, live := environment.Lookup(out.Environment)
	if !live {
		return Waiting{}, false, nil
	}
	who, still := among.Person(person)
	if !still {
		// The pathway survived a deploy and the person did not: there is nobody
		// to have this with any more.
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

Deliberately not "compose a rule and let it run": the conflict already happened
and what is owed is a decision rather than a policy. Writing the rule that covers
the next one is a different command on a different screen.

Everything that follows is the same as the covered path — deltas paid, ledger
moved, record settled — because otherwise the log would be comparing two
different things.
*/
func (m *Conflicts) Answer(
	ctx context.Context, tx pgx.Tx, accountID string, id int64,
	stance playbook.Stance, on simulation.Date,
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
	// The record was written pending when the conflict arose. A conflict that
	// reached nobody's rules has no record to settle.
	if fire != nil {
		if _, _, err := m.rules.Settle(ctx, tx, accountID, *fire, now, answer.Outcome); err != nil {
			return Answer{}, err
		}
	}
	if err := m.close(ctx, tx, id, 0, stance, answer.Outcome, false, now); err != nil {
		return Answer{}, err
	}

	// And the day goes on. Answering unblocks the walk [Conflicts.hold] stopped,
	// and it runs after the standing has moved, so who turns up next follows
	// from what was just said. The walk stops again the moment the next one is
	// deferred, so this is one more conversation and never a queue of them.
	if _, err := m.Day(ctx, tx, accountID, on); err != nil {
		return Answer{}, err
	}

	// The name, filled in: the authored line says {who}.
	if among, live := environment.Lookup(where); live {
		if who, still := among.Person(person); still {
			answer.Did, answer.Came = Says(answer.Did, who.Name), Says(answer.Came, who.Name)
		}
	}
	return answer, nil
}

// Told is what a page of the playbook's log needs about the conflicts behind
// its records: the moment, who it was with, and the answer it was met with.
// Keyed by fire rather than by pathway, because the same friction can have
// arisen on many days. internal/playbook holds which rule fired; this holds what
// was going on at the time.
func (m *Conflicts) Told(
	ctx context.Context, tx pgx.Tx, accountID string, fires []int64,
) (map[int64]Telling, error) {
	out := map[int64]Telling{}
	if len(fires) == 0 {
		return out, nil
	}

	rows, err := tx.Query(ctx, `
		SELECT fire_id, id, environment, conflict, person, stance FROM conflict
		 WHERE account_id = $1 AND fire_id = ANY($2)`, accountID, fires)
	if err != nil {
		return nil, fmt.Errorf("conflict: read tellings: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			fire    int64
			message int64
			where   string
			key     string
			person  string
			stance  string
		)
		if err := rows.Scan(&fire, &message, &where, &key, &person, &stance); err != nil {
			return nil, fmt.Errorf("conflict: read tellings: %w", err)
		}
		authored, known := Pathway(where, key)
		if !known {
			// A pathway withdrawn from content since it happened. The key is
			// still the truth about what arose, so it is reported as itself
			// rather than dropped.
			out[fire] = Telling{
				Conflict: key, Message: message, Environment: where, Person: person,
			}
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
			Message: message, Environment: where, Person: person,
		}
		if answer, written := authored.Answer(playbook.Stance(stance)); written {
			told.Did, told.Came = Says(answer.Did, name), Says(answer.Came, name)
			// Left empty when the world no longer authors them, rather than
			// substituted with nothing. A title is composed once and stored,
			// so "You did not cover 's $12" would be frozen into somebody's
			// record for good; falling back to the authored event name is the
			// honest version of not knowing who this was with.
			if name != "" {
				told.Title = Says(answer.Title, name)
			}
		}
		out[fire] = told
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("conflict: read tellings: %w", err)
	}
	return out, nil
}

// Living is who the player is among and how it has been going — §3.6's
// evidence, for the population rather than for the rule set. The playbook's log
// says which rule fired; this says which of the people around the player it
// keeps firing about and which question keeps arriving. Nothing here is a
// verdict: the counts are counts.
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
	// table can be compared with last week's.
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

// Threaded is how many messages back [Conflicts.Threads] reads: long enough
// that a thread has a shape, short enough that the read is one bounded query.
// The phone is a surface somebody opens, not an archive.
const Threaded = 180

/*
Threads is the phone: every person anything has ever passed with, and what.

The read behind §3.14's step 1. [Living] answers "how has the fortnight gone" in
counts for the population; this answers "what happened, and what did I say", one
person at a time, in their own words and in order.

It reads across environments rather than within one: a player who moved off the
east side still has what passed with Yolanda, and the thread going quiet is
itself true, since [Conflicts.Day] only picks somebody they live among.

Two deliberate absences. [environment.Person] carries an authored does and this
does not send it — a read on somebody is the conclusion the thread is supposed to
lead to (§3.2). And there is no unread count, since at most one message in the
phone is ever waiting.
*/
func (m *Conflicts) Threads(
	ctx context.Context, tx pgx.Tx, accountID string,
) ([]Thread, error) {
	where, err := m.where.Where(ctx, tx, accountID)
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `
		SELECT id, on_day, created_at, environment, person, conflict,
		       stance, outcome, covered, waiting
		  FROM conflict
		 WHERE account_id = $1
		 ORDER BY on_day DESC, id DESC
		 LIMIT $2`, accountID, Threaded)
	if err != nil {
		return nil, fmt.Errorf("conflict: read threads: %w", err)
	}
	defer rows.Close()

	// Keyed by population *and* person: two environments may name somebody the
	// same way, and they are not the same somebody.
	type where2 struct{ among, person string }
	held := map[where2]*Thread{}
	var order []where2

	for rows.Next() {
		var (
			msg     Message
			on      time.Time
			among   string
			person  string
			key     string
			stance  string
			outcome string
		)
		if err := rows.Scan(&msg.ID, &on, &msg.At, &among, &person, &key,
			&stance, &outcome, &msg.Covered, &msg.Waiting); err != nil {
			return nil, fmt.Errorf("conflict: read threads: %w", err)
		}

		live, still := environment.Lookup(among)
		if !still {
			continue
		}
		who, authored := live.Person(person)
		if !authored {
			// The population survived a deploy and this person did not. Dropped
			// rather than reported under their key: a thread headed yolanda is
			// not a conversation with anybody.
			continue
		}
		pathway, written := Pathway(among, key)
		if !written {
			continue
		}

		msg.On = simulation.On(on, time.UTC)
		msg.Name = pathway.Name
		msg.Says = Says(pathway.Line, who.Name)
		msg.Outcome = playbook.Outcome(outcome)
		if answer, has := pathway.Answer(playbook.Stance(stance)); has {
			msg.Did, msg.Came = Says(answer.Did, who.Name), Says(answer.Came, who.Name)
		}
		if msg.Waiting {
			// The options belong to the message rather than to the screen: a
			// phone told separately what could be said back would be composing
			// the conversation.
			for _, answer := range pathway.Answers() {
				answer.Did, answer.Came = Says(answer.Did, who.Name), Says(answer.Came, who.Name)
				msg.Answers = append(msg.Answers, answer)
			}
		}

		at := where2{among, person}
		thread, open := held[at]
		if !open {
			thread = &Thread{
				Person: who, Environment: among, Among: live.Name,
				Here: among == where, Intro: who.Intro,
			}
			held[at] = thread
			order = append(order, at)
		}
		// The query walks newest first so that the LIMIT takes the recent end of
		// the history; a conversation is read the other way round.
		thread.Messages = append([]Message{msg}, thread.Messages...)
		thread.Waiting = thread.Waiting || msg.Waiting
		if msg.At.After(thread.Last) {
			thread.Last = msg.At
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("conflict: read threads: %w", err)
	}

	// Everybody the player lives among has a thread, whether or not anything
	// has passed with them: the introduction is already in it. Appended after
	// the walk so the people who have said something keep their place at the
	// top, and in authored order so a phone opened on the first morning of a
	// life reads the same in every process (§7).
	if live, among := environment.Lookup(where); among {
		for _, who := range live.People() {
			at := where2{where, who.Key}
			if _, met := held[at]; met || len(who.Intro) == 0 {
				continue
			}
			held[at] = &Thread{
				Person: who, Environment: where, Among: live.Name,
				Here: true, Intro: who.Intro,
			}
			order = append(order, at)
		}
	}

	// order is first-seen over a newest-first walk, which is already most recent
	// thread first. Nothing is sorted afterwards, so threads whose last message
	// landed in the same second keep a stable order between reads (§7).
	out := make([]Thread, 0, len(order))
	for _, at := range order {
		out = append(out, *held[at])
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

// hold takes the day back if the player is already standing in front of
// something: the standing untouched when the day may go ahead, a zero one when
// it may not. The row just written is deleted here rather than left for a
// sweeper.
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

/*
history is every conversation already settled with everybody in a population,
and what [Reached] is read against. No table of its own: conflict carries the
person, the pathway, the posture and the day, so this is a query rather than a
second record free to disagree with the first.

Settled only — a conflict still waiting has not happened yet, and its sequel must
not unlock on a question the player has not answered.

The whole population in one query rather than one per person, since the day walks
everybody.
*/
func (m *Conflicts) history(
	ctx context.Context, tx pgx.Tx, accountID, where string,
) (History, error) {
	rows, err := tx.Query(ctx, `
		SELECT person, conflict, stance, on_day
		  FROM conflict
		 WHERE account_id = $1 AND environment = $2 AND NOT waiting
		 ORDER BY on_day, id`, accountID, where)
	if err != nil {
		return nil, fmt.Errorf("conflict: read history: %w", err)
	}
	defer rows.Close()

	out := History{}
	for rows.Next() {
		var (
			person string
			p      Passed
			on     time.Time
		)
		if err := rows.Scan(&person, &p.Conflict, &p.Stance, &on); err != nil {
			return nil, fmt.Errorf("conflict: read history: %w", err)
		}
		p.On = simulation.On(on, time.UTC)
		out[person] = append(out[person], p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("conflict: read history: %w", err)
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

// tally counts a window of days among one population, both ways at once. Two
// queries rather than one pass in Go: counting is what a database is for.
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

The walk follows [Standing.Under]'s order — what is most overdue between the two
of them — and stops at the first pressure the population has writing for this
person now. [Reached] filters the pool before anything is drawn, so no moment can
arrive out of turn.

A pressure can therefore come up empty for somebody with plenty of writing, and
the walk goes on rather than stopping. [Easy] is last and every person is
guaranteed an opener under it by the loader, so this finds something for anybody
in a shipped build.

Which of a pool is taken is a hash of the account, the day, the person and the
pressure — deterministic (§7), and including the person stops two people under
the same pressure being handed the same moment on the same morning.
*/
func (m *Conflicts) pathway(
	where string, standing Standing, past []Passed, accountID string, on simulation.Date,
) (Authored, bool) {
	for _, strain := range standing.Under() {
		pool := reachable(For(where, strain), standing.Person, past, on)
		if len(pool) == 0 {
			continue
		}
		at := shuffle(accountID, on, standing.Person+"|"+string(strain)) % uint32(len(pool))
		return pool[at], true
	}
	return Authored{}, false
}

// reachable is everything in a pool this person could be sent today. Catalogue
// order is preserved, so the hash above indexes into a pool whose order is the
// same in every process.
func reachable(pool []Authored, person string, past []Passed, on simulation.Date) []Authored {
	out := pool[:0:0]
	for _, a := range pool {
		if Reached(a, person, past, on) {
			out = append(out, a)
		}
	}
	return out
}

// causeOf is why a conflict is waiting, in internal/playbook's own vocabulary.
// It reads the decision rather than being told, so the reason the player sees
// and the reason the record gives are one answer.
func causeOf(d playbook.Decision) playbook.Cause {
	cause, _ := d.Escalation()
	return cause
}
