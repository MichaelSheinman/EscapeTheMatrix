/*
Package situation owns the work a delegated role does while the player is
somewhere else (§3.14).

HighLevelOverview.md §3.14 splits the ladder in half and names both halves.
Tiers one and two are "genuinely passive — you hold the job, it pays"; that is
internal/career, and #27 built it. From [simulation.Delegated] upward the design
changes shape entirely:

	**Active jobs are delegated, not micromanaged.** The player does not sit
	inside their job. The role runs on its own and generates interactions while
	they are elsewhere. The loop:

	 1. **It happens without you.** Situations arise in the role — a staffing
	    call, a budget choice, someone quits — and the character handles them.
	 2. **You review.**
	 3. **You compose rules.**
	 4. **A rule fails.**
	 5. **You revise.** Back to step 3 — permanently.

This package is steps 1 and 4. Step 2 is the playbook's log, step 3 is
internal/playbook, and step 5 is the player.

# The loop closes on itself, deliberately

A situation arises with nobody asking; the rules in force *where the player is
standing* (§3.15) are weighed against it; if exactly one posture comes out of
that, the character acts and the consequence is applied. If nothing covers it,
or two answers disagree, the moment does not resolve — it waits, and the player
answers it themselves.

**And a role that is waiting produces nothing new.** That single rule is what
makes §3.14's "good rules buy time, and gaps in the playbook are what demand
attention" a mechanic rather than a slogan: a playbook that covers the job keeps
the job moving, and one full of holes stops it until the player does the work by
hand. It is also what bounds a long absence — a fortnight away with no rules is
one decision waiting, not fourteen (§4).

# What is authored and what is code

Everything a player reads is in `content/situations` (catalogue.go). What is
here is the arithmetic: which moment a day produces, what a decision does to the
record, and what the authored deltas are handed to. §7 forbids a model anywhere
in this path and nothing here is random — the day's moment is a pure function of
who the player is and what day it is, so the same world produces the same work
twice.

# Four seams, and none of them is a module

  - **[Holding]** is the rung the player is on. internal/career owns it, and
    this package is handed a tier rather than a career so that it cannot read a
    wage, a ledger or an interview.
  - **[Judgement]** is the playbook: weigh a moment, record what was made of it,
    settle it later. Exactly the four methods internal/playbook already exposes,
    named as an interface so that this package cannot reach a rule set, compose
    one, or read a term the player has not unlocked.
  - **[Where]** is which environment's rules are in force (§3.15).
  - **[Awards]** is where the authored deltas go — one way, returning an error
    rather than a standing, exactly as internal/challenge's does. A module that
    could read what it moved on a hidden factor is a module somebody eventually
    renders (§3.5).

It owns `job_situation` (§3.1), takes the caller's transaction and never opens
one (§3.2), and reads no other module's tables.
*/
package situation

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/career"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/playbook"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

// The refusals this module has. Go errors rather than status codes:
// internal/api maps them at the edge (§8), so a module never learns HTTP.
var (
	// ErrNoSuchSituation is a moment that is not this player's, or not waiting.
	ErrNoSuchSituation = errors.New("situation: no such situation")
	// ErrNoSuchAnswer is a posture this moment was not written an answer for.
	ErrNoSuchAnswer = errors.New("situation: no such answer")
	// ErrWithdrawn is a moment whose authored content has gone since it arose.
	// Reachable only across a deploy that removed a situation somebody was
	// standing in front of.
	ErrWithdrawn = errors.New("situation: that is no longer something the world has in it")
)

/*
Holding is the job a player is in.

A role key and nothing else. internal/career owns employment and this package has
no business reading a wage, a ledger, or whether an interview was passed — what
it needs is the name of the job, which it takes to the catalogue itself to ask
the only question it has: does this one run while the player is elsewhere.

It used to be a [simulation.Tier] compared against a constant. There is no rung
any more (2026-08-15) and "delegated" is authored on the job, so the seam carries
the job.
*/
type Holding interface {
	Holds(ctx context.Context, tx pgx.Tx, accountID string) (string, error)
}

// Unheld is a Holding for a player with no job. For tests about the catalogue
// rather than about employment; never bound in the server.
type Unheld struct{}

func (Unheld) Holds(context.Context, pgx.Tx, string) (string, error) { return "", nil }

/*
Judgement is the playbook, as this package is allowed to use it.

Four methods, and *only* four. internal/playbook exposes rather more — composing
a rule, reading the vocabulary, clearing an answer — and none of that is
something a job may do to a player's own judgement. The interface is the fence:
a delegated role can put a question, be told what the rules made of it, write
that down and settle it, and it cannot edit the rules it was answered by.
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
Where is which environment's rules are in force (§3.15).

The same one-question interface internal/factor takes for the same fact, and for
the same reason: which population a player lives among follows from where they
live, which is internal/character's, so composing the two is internal/api's job
(§3.1) and this package holds a key.
*/
type Where interface {
	Where(ctx context.Context, tx pgx.Tx, accountID string) (string, error)
}

/*
Awards is where an answer's authored deltas go (§3.5).

One method, one way. It returns an error rather than the standing
internal/factor actually hands back, which is the whole point of it being an
adapter: a job that could read what it moved on a hidden factor is a job
somebody eventually shows a number for, and §3.2 keeps the hidden three
unquantified everywhere outside the package that scores them.
*/
type Awards interface {
	Award(ctx context.Context, tx pgx.Tx, accountID string, grants ...simulation.Grant) error
}

// Unpaid is an Awards that moves nothing. For tests about the machinery rather
// than about progression; never bound in the server.
type Unpaid struct{}

func (Unpaid) Award(context.Context, pgx.Tx, string, ...simulation.Grant) error { return nil }

/*
Arisen is what one day of a delegated role came to.

The zero value is a day nothing arose on: the player holds no job, holds a
passive one, is already standing in front of something, or has had this day
walked before.
*/
type Arisen struct {
	// Situation is the authored key, empty when nothing arose.
	Situation string
	// Fire is the rule-fire record this moment wrote, and zero when the moment
	// put a question the player is standing in an environment nobody asks it in.
	Fire int64
	// Handled is the posture the rules settled it with, empty when they did not.
	Handled playbook.Stance
	// Waiting is whether this moment is now in front of the player as a live
	// decision (§3.14: "anything the rules don't cover escalates").
	Waiting bool
	// Cause is why it is waiting, and empty when it is not.
	Cause playbook.Cause
}

// Arose reports whether the role produced anything at all.
func (a Arisen) Arose() bool { return a.Situation != "" }

/*
Waiting is a moment standing in front of the player, and what they may do about
it.

Everything to re-establish it, because this is what somebody who closed the tab
lands back on — the same property §5.1 makes mandatory for a challenge run, and
for the same reason: the world did not pause while they were away, and a live
decision they cannot find again is a live decision that was never live.
*/
type Waiting struct {
	ID        int64
	Situation Authored
	At        time.Time
	// On is the player's own day the role produced it on.
	On simulation.Date
	// Environment is where their rules were read from, by key (§3.15).
	Environment string
	// Role is the job it arose in, by catalogue key. Kept on the row rather
	// than derived, because a player who has since left that job still has to
	// be able to read back what it was doing.
	Role string
	// Cause is why it reached them: nothing covered it, or two answers
	// disagreed. Never [playbook.Backfired] — a rule that fired and went badly
	// has already been carried out, and is told about rather than asked about.
	Cause playbook.Cause
	// Note is that reason in the words the log uses.
	Note string
}

/*
Telling is one settled moment, as the log reads it back.

The words rather than the keys, because a log line is the writing being reported
rather than assembled — the same argument internal/playbook makes about
rendering a rule on the server (§10.2).
*/
type Telling struct {
	Situation string
	Name      string
	Line      string
	// Did and Came are the authored answer, and are empty for a moment nobody
	// has answered yet.
	Did  string
	Came string
}

// Situations is the module. It holds no per-player state: the transaction comes
// from the caller and the four seams are injected (§3.1).
type Situations struct {
	clk    simulation.Clock
	holds  Holding
	rules  Judgement
	where  Where
	awards Awards
}

func New(clk simulation.Clock, holds Holding, rules Judgement, where Where, awards Awards) *Situations {
	return &Situations{clk: clk, holds: holds, rules: rules, where: where, awards: awards}
}

/*
Day is one day of a delegated role, run with nobody watching.

Called by the rollover for every day it walks (§3.3), which is what makes it
"while the player is elsewhere" rather than a thing that happens when they look.

Four ways it does nothing, and each of them is a rule rather than a guard:

  - **No job, or one nobody delegated.** A job that does not run itself pays
    without being played, and that is most of them.
  - **Something is already waiting.** A role does not pile work on a player who
    has not dealt with the last of it — see the package comment.
  - **This day has already been walked.** The unique key on (account, day) is
    what makes a retried job and a second tab hydrating produce one moment
    between them rather than one each.
  - **Nothing is authored for the job.** Impossible for a delegated one, and
    checked at boot; a job that somehow reached here with an empty pool gets a
    quiet day rather than a panic in a worker.
*/
func (m *Situations) Day(
	ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date,
) (Arisen, error) {
	role, err := m.holds.Holds(ctx, tx, accountID)
	if err != nil {
		return Arisen{}, err
	}
	if job, known := career.ByKey(role); !known || !job.Delegated {
		return Arisen{}, nil
	}

	pool := For(role)
	if len(pool) == 0 {
		return Arisen{}, nil
	}
	authored := pool[pick(accountID, on, len(pool))]

	where, err := m.where.Where(ctx, tx, accountID)
	if err != nil {
		return Arisen{}, err
	}

	// The row first, and the judgement after it. Written before anything is
	// weighed so that two rollovers arriving together contend on the unique key
	// rather than on the record — the loser writes no fire, moves no factor and
	// reports nothing, which is exactly what a second walk of the same day is.
	//
	// `waiting` starts true because it is true until this function decides
	// otherwise, and a crash between here and the update leaves a moment the
	// player can answer rather than one that vanished.
	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO job_situation (account_id, on_day, situation, role, environment, waiting)
		VALUES ($1, make_date($2, $3, $4), $5, $6, $7, true)
		ON CONFLICT (account_id, on_day) DO NOTHING
		RETURNING id`,
		accountID, on.Year, int(on.Month), on.Day, authored.Key, role, where,
	).Scan(&id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Arisen{}, nil
	case err != nil:
		return Arisen{}, fmt.Errorf("situation: raise: %w", err)
	}

	// After the insert, so that the "one at a time" rule is decided by the same
	// row the insert just contended for. A moment that arrives to find an older
	// one waiting takes itself back: the day produced nothing, which is the
	// state the player's own inaction put the role in.
	waiting, err := m.waiting(ctx, tx, accountID, id)
	if err != nil {
		return Arisen{}, err
	}
	if waiting {
		if _, err := tx.Exec(ctx, `DELETE FROM job_situation WHERE id = $1`, id); err != nil {
			return Arisen{}, fmt.Errorf("situation: withdraw: %w", err)
		}
		return Arisen{}, nil
	}

	now := m.clk.Now()
	decision, err := m.rules.Weigh(ctx, tx, accountID, where, authored.Moment())
	if err != nil {
		return Arisen{}, err
	}

	out := Arisen{Situation: authored.Key}
	settled := playbook.Pending
	var answer Answer
	if fired, ok := decision.Fired(); ok {
		if answer, ok = authored.Answer(fired.Stance); ok {
			settled, out.Handled = answer.Outcome, fired.Stance
		}
	}

	// The record, before the consequence. §3.14's log is the whole review
	// surface and a moment that moved a factor and wrote nothing down would be a
	// consequence the player cannot trace back to the rule that caused it.
	fire, err := m.rules.Record(ctx, tx, accountID, now, where, decision, settled)
	if err != nil {
		return Arisen{}, err
	}
	out.Fire = fire.ID

	if out.Handled != "" {
		if err := m.awards.Award(ctx, tx, accountID, answer.Moves...); err != nil {
			return Arisen{}, err
		}
		if err := m.close(ctx, tx, id, fire.ID, out.Handled, answer.Outcome, now); err != nil {
			return Arisen{}, err
		}
		return out, nil
	}

	// Nothing covered it, or two answers disagreed. It waits.
	//
	// The fire is marked raised here rather than left for §4's release policy,
	// because the moment *is* the escalation and it is already in front of the
	// player: queueing an event about it as well would put the same conflict to
	// them twice under two different names. The third condition — a rule fired
	// and went badly — is not marked, and reaches them the ordinary way.
	out.Waiting, out.Cause = true, causeOf(decision)
	if fire.ID != 0 {
		if _, err := m.rules.Raised(ctx, tx, accountID, fire.ID, now); err != nil {
			return Arisen{}, err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE job_situation SET fire_id = $2 WHERE id = $1`, id, fire.ID); err != nil {
			return Arisen{}, fmt.Errorf("situation: raise: %w", err)
		}
	}
	return out, nil
}

/*
Waiting is the moment standing in front of this player, if one is.

Oldest first, though there is only ever one — the rule in [Day] is what makes
that true, and reading it this way means a database that somehow held two hands
back the one the player was owed first rather than whichever the planner
returned.
*/
func (m *Situations) Waiting(ctx context.Context, tx pgx.Tx, accountID string) (Waiting, bool, error) {
	var (
		out   Waiting
		key   string
		on    time.Time
		fire  *int64
		verd  string
		known bool
	)
	err := tx.QueryRow(ctx, `
		SELECT s.id, s.situation, s.role, s.on_day, s.created_at, s.environment, s.fire_id,
		       COALESCE(f.verdict, '')
		  FROM job_situation s
		  LEFT JOIN playbook_fire f ON f.id = s.fire_id
		 WHERE s.account_id = $1 AND s.waiting
		 ORDER BY s.on_day, s.id
		 LIMIT 1`, accountID,
	).Scan(&out.ID, &key, &out.Role, &on, &out.At, &out.Environment, &fire, &verd)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Waiting{}, false, nil
	case err != nil:
		return Waiting{}, false, fmt.Errorf("situation: read waiting: %w", err)
	}

	if out.Situation, known = Known(key); !known {
		// Authored content withdrawn from under somebody who was standing in
		// front of it. Reported as nothing waiting rather than as an error: the
		// alternative is a world snapshot that fails to load until a deploy is
		// rolled back.
		return Waiting{}, false, nil
	}
	// A DATE carries no timezone and needs none: it is already the player's own
	// calendar day, written by whichever rollover walked it.
	out.On = simulation.On(on, time.UTC)
	out.Cause, _ = playbook.Because(playbook.Verdict(verd), playbook.Pending)
	out.Note = playbook.Decision{Verdict: playbook.Verdict(verd)}.Note()
	return out, true, nil
}

/*
Answer is the player deciding one themselves — §3.14's step 4, by hand.

It is the one path in this package a command drives, and it is deliberately not
"compose a rule and let it run": the moment already happened, the rules already
failed to cover it, and what is owed is a decision rather than a policy. Writing
the rule so that the *next* one is covered is a different command on a different
screen, which is the loop.

The answer is a posture out of the ones this moment was written for. Everything
that follows is the same as the handled path — the deltas are paid, the record
is settled, the row is closed — because the player answering by hand and their
rules answering for them have to produce the same kind of consequence or the log
would be comparing two different things.
*/
func (m *Situations) Answer(
	ctx context.Context, tx pgx.Tx, accountID string, id int64, stance playbook.Stance,
) (Answer, error) {
	var (
		key  string
		fire *int64
	)
	err := tx.QueryRow(ctx, `
		SELECT situation, fire_id FROM job_situation
		 WHERE id = $1 AND account_id = $2 AND waiting
		 FOR UPDATE`, id, accountID,
	).Scan(&key, &fire)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Answer{}, fmt.Errorf("%w: %d", ErrNoSuchSituation, id)
	case err != nil:
		return Answer{}, fmt.Errorf("situation: read waiting: %w", err)
	}

	authored, known := Known(key)
	if !known {
		return Answer{}, fmt.Errorf("%w: %q", ErrWithdrawn, key)
	}
	answer, written := authored.Answer(stance)
	if !written {
		return Answer{}, fmt.Errorf("%w: %q", ErrNoSuchAnswer, stance)
	}

	now := m.clk.Now()
	if err := m.awards.Award(ctx, tx, accountID, answer.Moves...); err != nil {
		return Answer{}, err
	}
	// The record was written pending when the moment arose, because how it went
	// was not known then and is now. A moment that reached nobody's rules has no
	// record to settle, which is the one case this skips.
	if fire != nil {
		if _, _, err := m.rules.Settle(ctx, tx, accountID, *fire, now, answer.Outcome); err != nil {
			return Answer{}, err
		}
	}
	if err := m.close(ctx, tx, id, 0, stance, answer.Outcome, now); err != nil {
		return Answer{}, err
	}
	return answer, nil
}

/*
Told is what a page of the log needs about the moments behind its records.

Keyed by fire rather than by situation, because the log is a page of records and
the same situation can have arisen on many days. What comes back is the writing
— the moment, and the answer it was met with — which internal/playbook has no
way to know and no business knowing: it holds which rule fired and this holds
what the world was doing at the time.
*/
func (m *Situations) Told(
	ctx context.Context, tx pgx.Tx, accountID string, fires []int64,
) (map[int64]Telling, error) {
	out := map[int64]Telling{}
	if len(fires) == 0 {
		return out, nil
	}

	rows, err := tx.Query(ctx, `
		SELECT fire_id, situation, stance FROM job_situation
		 WHERE account_id = $1 AND fire_id = ANY($2)`, accountID, fires)
	if err != nil {
		return nil, fmt.Errorf("situation: read tellings: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			fire   int64
			key    string
			stance string
		)
		if err := rows.Scan(&fire, &key, &stance); err != nil {
			return nil, fmt.Errorf("situation: read tellings: %w", err)
		}
		authored, known := Known(key)
		if !known {
			// A situation withdrawn from content since it happened. The key is
			// still the truth about what arose, so it is reported as itself
			// rather than dropped — a log line that quietly lost a moment would
			// be a smaller lie than a wrong one but a lie all the same.
			out[fire] = Telling{Situation: key}
			continue
		}
		told := Telling{Situation: key, Name: authored.Name, Line: authored.Line}
		if answer, written := authored.Answer(playbook.Stance(stance)); written {
			told.Did, told.Came = answer.Did, answer.Came
		}
		out[fire] = told
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("situation: read tellings: %w", err)
	}
	return out, nil
}

/* ── the columns ──────────────────────────────────────────────────────── */

// close settles a row: what was done, how it went, and that it is no longer
// waiting. `fire` is written only when the caller has one to attach.
func (m *Situations) close(
	ctx context.Context, tx pgx.Tx, id, fire int64,
	stance playbook.Stance, outcome playbook.Outcome, at time.Time,
) error {
	if _, err := tx.Exec(ctx, `
		UPDATE job_situation
		   SET stance     = $2,
		       outcome    = $3,
		       waiting    = false,
		       decided_at = $4,
		       fire_id    = COALESCE(NULLIF($5, 0), fire_id)
		 WHERE id = $1`,
		id, string(stance), string(outcome), at, fire,
	); err != nil {
		return fmt.Errorf("situation: settle: %w", err)
	}
	return nil
}

// waiting reports whether this player is already standing in front of something
// other than the moment just written.
func (m *Situations) waiting(ctx context.Context, tx pgx.Tx, accountID string, not int64) (bool, error) {
	var any bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM job_situation
			 WHERE account_id = $1 AND waiting AND id <> $2
		)`, accountID, not,
	).Scan(&any); err != nil {
		return false, fmt.Errorf("situation: read waiting: %w", err)
	}
	return any, nil
}

/*
pick is which moment of a pool a player's day produces.

Deterministic and not random (§7): the same player on the same day gets the same
work, so a rollover that is retried, replayed or run twice produces one world
rather than two. FNV over the account and the date is enough — what is being
avoided is every player in the game meeting the same situation on the same
morning, which a rotation by day alone would do.

§4's "selected deterministically from relationship state" is the shape this will
take when there is relationship state to select from (#30 — internal/environment
has the people, and nothing yet records what any of them are owed). Until then
the honest inputs are who and when.
*/
func pick(accountID string, on simulation.Date, n int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(accountID))
	_, _ = fmt.Fprintf(h, "|%04d-%02d-%02d", on.Year, int(on.Month), on.Day)
	return int(scatter(h.Sum32()) % uint32(n))
}

/*
scatter mixes a hash so that its low bits are worth taking a remainder of.

FNV's low bits are a bijection of the low bits of the state before them, so two
accounts whose hashes happen to agree in the bottom two bits agree in them
forever — every day, for a pool of four, identically. That is not a distribution
with a weak tail; it is two players handed the same year. The finalizer below is
the standard 32-bit avalanche, and it is here rather than a different hash
because what is wanted is exactly this: the same input still gives the same
answer, and one bit of input moves half the output.
*/
func scatter(x uint32) uint32 {
	x ^= x >> 16
	x *= 0x7feb352d
	x ^= x >> 15
	x *= 0x846ca68b
	x ^= x >> 16
	return x
}

// causeOf is why a moment is waiting, in internal/playbook's own vocabulary.
//
// It reads the decision rather than being told, so that the reason the player
// is looking at this and the reason the record gives are one answer.
func causeOf(d playbook.Decision) playbook.Cause {
	cause, _ := d.Escalation()
	return cause
}
