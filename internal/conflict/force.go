package conflict

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/environment"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
What the developer's toolbar is allowed to do to a conversation.

Three methods, kept in their own file so that what authoring needs and what the
game does are never one function with a flag in it. They exist because a
conflict pathway is content — a moment, a line, three answers and what each of
them comes to — and content takes forty passes to write, every one of which has
to start from a known state. Waiting a day per pass is not a workflow.

# What they are not

They are not a bypass. Everything here goes through the same [Conflicts.raise]
the rollover uses, so a forced conflict is weighed against the player's real
playbook, writes a real fire record, moves the real standing and pays whatever
its answer pays. The *only* thing supplied from outside is the selection — which
person, which pathway — which is the one decision [Conflicts.arise] makes that
somebody writing content already knows the answer to.

That is deliberate and it is the line. A toolbar that wrote its own conflict
rows would be a second implementation of what a conflict is, and the first time
it drifted, every pathway would have been proof-read against a moment the game
could not produce.

# Who may call them

Nothing in the game. `internal/api/devconflict.go` is the only caller and it is
not registered in a production binary — see [Server.registerCommands]. There is
no flag here to get wrong because there is no flag here.
*/

// ErrNoSuchPerson is somebody the player's population does not have in it.
var ErrNoSuchPerson = errors.New("conflict: no such person")

/*
ErrOutOfTurn is a moment this person could not send, could not send yet, or has
no reason to be sending.

The toolbar is held to the selection rules (internal/conflict/thread.go) rather
than exempted from them, because the whole reason it exists is to show somebody
the writing as the game will actually produce it. A button that could raise
`the-favour-back` before the allotment afternoon would let a pathway be
proof-read in a state no player can ever be in, which is worse than not being
able to proof-read it at all.

Three questions, which are [Reached]'s first two plus the strain the day's own
pool is drawn from — and the third is here because a line is written as though
what it follows from were true. `the-fourth-ask` says "fourth time, I hate
asking" and is only the fourth time because [Asking] is three asks in a month;
raised out from under that, the phone shows a message that counts wrong. Not a
detail of the arithmetic: it is the same claim-the-record-contradicts the
content SPEC bans, arriving from the toolbar rather than from the prose.

To read a pathway that follows a pressure, put the pressure there first — three
presses of an `easy` money ask make somebody genuinely crowded, and the fourth
ask then unlocks the way it does in a played week.

What none of this enforces is the cadence: a pathway already said and not due
back for a week may still be raised, because re-reading prose is the whole job
and nothing about the order is broken by reading it twice.
*/
var ErrOutOfTurn = errors.New("conflict: out of turn")

// under reports whether a standing is under one strain, by the same reading
// [Conflicts.pathway] takes when it draws the day's pool.
func under(s Standing, strain Strain) bool {
	for _, was := range s.Under() {
		if was == strain {
			return true
		}
	}
	return false
}

// list is a set of requirements as an author would read them back.
func list(all []Requirement) string {
	out := make([]string, 0, len(all))
	for _, need := range all {
		out = append(out, need.String())
	}
	return strings.Join(out, ", ")
}

/*
Force raises one named conflict with one named person, now.

The selection is the argument; everything after it is the ordinary path. Two
things it has to settle that [Conflicts.arise] gets for free:

**Which day it lands on.** One conflict per person per day is a real rule
(00032) and this does not break it — it finds the first day from today forward
on which this person has nothing, and writes there. So pressing the button four
times builds a four-message thread with four dates on it rather than failing on
the unique key three times. The dates march into the future, which is visible in
the thread and is the honest signal that a history was assembled rather than
lived.

**What is already waiting.** Nothing, on purpose: [Conflicts.hold] would
withdraw this the moment it landed. The caller clears the way with
[Conflicts.Withdraw] first, and says so in its notice — see devConflictRaise,
which does exactly what devEnable does about an already-played challenge, and
for the same reason: nobody presses "raise" wanting to be told to press
something else first.
*/
func (m *Conflicts) Force(
	ctx context.Context, tx pgx.Tx, accountID, person, key string, today simulation.Date,
) (Arisen, error) {
	where, err := m.where.Where(ctx, tx, accountID)
	if err != nil {
		return Arisen{}, err
	}
	among, live := environment.Lookup(where)
	if !live {
		return Arisen{}, fmt.Errorf("%w: nobody lives around %q", ErrNoSuchPerson, where)
	}
	who, known := among.Person(person)
	if !known {
		return Arisen{}, fmt.Errorf("%w: %q is not among %s", ErrNoSuchPerson, person, where)
	}
	authored, written := Pathway(where, key)
	if !written {
		return Arisen{}, fmt.Errorf("%w: %q", ErrWithdrawn, key)
	}

	// The ordering rule, on the toolbar's path too. This is the one thing Force
	// must *not* be allowed to do differently: a moment raised with somebody who
	// could not have sent it, or before the conversation it remembers, is exactly
	// the state the rule exists to make unreachable — and a toolbar that could
	// reach it would have somebody proof-reading a thread the game cannot
	// produce. Two refusals rather than one, because "she would never say this"
	// and "not yet" are different mistakes and want different fixes.
	if !authored.Sends(person) {
		return Arisen{}, fmt.Errorf("%w: %q is not %s's to send — it is sent by %s",
			ErrOutOfTurn, key, who.Name, strings.Join(authored.With, ", "))
	}

	past, err := m.history(ctx, tx, accountID, where)
	if err != nil {
		return Arisen{}, err
	}
	if wanting := Wanting(authored, past[person]); len(wanting) > 0 {
		return Arisen{}, fmt.Errorf("%w: %q comes after %s, and none of that has passed with %s yet",
			ErrOutOfTurn, key, list(wanting), who.Name)
	}

	standings, err := m.standings(ctx, tx, accountID, where)
	if err != nil {
		return Arisen{}, err
	}
	standing := standings[person]
	standing.Person = person

	// The pressure, which is the third of the three and the one that was
	// missing. A pathway declares what it follows from, and the line is written
	// as though that pressure were true: `the-fourth-ask` says "fourth time, I
	// hate asking", which is only the fourth time because [Asking] means three
	// asks this month have already landed. Raised out from under its strain it
	// is a message that counts wrong on the phone — the same lie the `after:`
	// gate exists to prevent, told about the ledger rather than about the
	// thread.
	if !under(standing.Fresh(today), authored.Follows) {
		return Arisen{}, fmt.Errorf(
			"%w: %q follows %s, and nothing between you and %s is %s yet",
			ErrOutOfTurn, key, authored.Follows, who.Name, authored.Follows)
	}

	on, err := m.free(ctx, tx, accountID, person, today)
	if err != nil {
		return Arisen{}, err
	}

	// Down the present-player branch, always — see [Toolbar].
	arisen, _, err := m.present().raise(ctx, tx, accountID, where, who, standing, authored, on)
	return arisen, err
}

/*
Toolbar is a [Presence] that says the player is always here.

It is what makes a raised conversation land on the phone rather than be answered
by the rules, and it is not a special case bolted onto [Conflicts.Force] — it is
the presence rule applied honestly. Somebody pressing a button in the developer's
toolbar *is* at the keyboard, by construction. There is no state of the world in
which they pressed it and were not there.

That matters because proof-reading a pathway means reading the moment and every
answer written for it. A conversation the rules settled on the way past is one
the author never saw, and a toolbar whose whole job is to show you the writing
would have skipped the writing.
*/
type Toolbar struct{}

func (Toolbar) Present(context.Context, pgx.Tx, string) (bool, error) { return true, nil }

// present is this module with somebody at the keyboard, whoever is really
// there. Used by [Conflicts.Force] so that the toolbar's raise goes down the
// same branch a present player's conflict does, rather than round it.
func (m *Conflicts) present() *Conflicts {
	out := *m
	out.here = Toolbar{}
	return &out
}

/*
Standings is what stands between this player and everybody around them.

The same read the selection makes, exported so the developer's toolbar can show
the numbers [Turn] actually sorted on rather than a second opinion about them —
"why did it pick her again" is most of what a toolbar over this mechanic is for,
and the answer is a column of pressures.

Read-only, and nothing in the game calls it: the module's own paths already have
the standings in hand where they need them.
*/
func (m *Conflicts) Standings(
	ctx context.Context, tx pgx.Tx, accountID, where string,
) (map[string]Standing, error) {
	return m.standings(ctx, tx, accountID, where)
}

/*
Raisable is what the toolbar may put to each person right now, by person key.

The same three questions [Conflicts.Force] refuses on — is it theirs, has the
thread got here, and is this person under the pressure it follows from — asked
of everybody at once so that a button whose only outcome is a refusal is never
drawn. A toolbar that offered the whole catalogue for every neighbour and then
explained itself afterwards is a toolbar that spends most of its presses
teaching the rule; the rule is better expressed as the absence of a button.

`on` is the day the pressures are read as of, because [Asking] is a count
within a month and a standing carrying August's three asks is not crowded in
September. The same [Standing.Fresh] the raise itself applies, so the strip and
the row it writes agree about what somebody is under.

The cadence is deliberately not asked, exactly as [Conflicts.Force] does not
ask it. A pathway already said and not due back until next week is still there
to be re-read, and re-reading is the whole job. What is out of turn is a moment
belonging to somebody else or a sequel to a conversation that has not happened —
neither of which is a thing time fixes.

Read-only, and nothing in the game calls it.
*/
func (m *Conflicts) Raisable(
	ctx context.Context, tx pgx.Tx, accountID, where string, on simulation.Date,
) (map[string][]string, error) {
	among, live := environment.Lookup(where)
	if !live {
		return map[string][]string{}, nil
	}
	past, err := m.history(ctx, tx, accountID, where)
	if err != nil {
		return nil, err
	}
	standings, err := m.standings(ctx, tx, accountID, where)
	if err != nil {
		return nil, err
	}

	out := make(map[string][]string, len(among.People()))
	for _, who := range among.People() {
		standing := standings[who.Key].Fresh(on)
		keys := []string{}
		for _, a := range All() {
			if a.Among != where || !a.Sends(who.Key) {
				continue
			}
			if len(Wanting(a, past[who.Key])) == 0 && under(standing, a.Follows) {
				keys = append(keys, a.Key)
			}
		}
		out[who.Key] = keys
	}
	return out, nil
}

/*
free is the first day from `today` forward that this person has nothing on.

One query rather than a walk: any day past the last one they already have is
free by construction, and a gap earlier in the week is not worth a round trip to
find. A person with nothing at all gets today, which is the case that matters —
the first press of the button should land on the day the player is standing in.
*/
func (m *Conflicts) free(
	ctx context.Context, tx pgx.Tx, accountID, person string, today simulation.Date,
) (simulation.Date, error) {
	var day time.Time
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(on_day) + 1, make_date($2, $3, $4))
		  FROM conflict
		 WHERE account_id = $1 AND person = $5 AND on_day >= make_date($2, $3, $4)`,
		accountID, today.Year, int(today.Month), today.Day, person,
	).Scan(&day)
	if err != nil {
		return simulation.Date{}, fmt.Errorf("conflict: free day: %w", err)
	}
	// UTC because a DATE came back, which carries no zone and is already the
	// player's own local day — the same reading every other date column in this
	// module gets.
	return simulation.On(day, time.UTC), nil
}

/*
Withdraw takes back whatever conflict is standing in front of the player.

The row is deleted rather than settled, because settling it would write a stance
nobody chose and an outcome nobody earned into the record the log reads back.
What it leaves behind is the standing as it stood — the ask happened, and this
is not a time machine (the same rule dev.reset works under).

Returns the pathway it withdrew, or empty if nothing was waiting.
*/
func (m *Conflicts) Withdraw(ctx context.Context, tx pgx.Tx, accountID string) (string, error) {
	var key string
	err := tx.QueryRow(ctx, `
		DELETE FROM conflict
		 WHERE id = (
		   SELECT id FROM conflict
		    WHERE account_id = $1 AND waiting
		    ORDER BY on_day, id
		    LIMIT 1
		 )
		 RETURNING conflict`, accountID,
	).Scan(&key)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return "", nil
	case err != nil:
		return "", fmt.Errorf("conflict: withdraw: %w", err)
	}
	return key, nil
}

/*
Forget puts a whole conversation back to never having happened.

Both halves of it, and the second is the one that is easy to leave out: the
messages *and* the standing they moved. A thread deleted on its own leaves the
ledger saying this person is owed four favours and has been slighted twice,
which means the next conflict with them is selected out of a history that no
longer exists — and somebody proof-reading an opening line would be handed a
pathway from the middle of a relationship they just erased.

What it does not undo is anything the answers paid. Karma, money and factors
went out through interfaces this module deliberately cannot read back, exactly
as they do for dev.reset. Forgetting replays the *content*; it does not rewind
the world.

Returns how many messages went.
*/
func (m *Conflicts) Forget(
	ctx context.Context, tx pgx.Tx, accountID, where, person string,
) (int64, error) {
	tag, err := tx.Exec(ctx,
		`DELETE FROM conflict WHERE account_id = $1 AND environment = $2 AND person = $3`,
		accountID, where, person)
	if err != nil {
		return 0, fmt.Errorf("conflict: forget: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM conflict_standing WHERE account_id = $1 AND environment = $2 AND person = $3`,
		accountID, where, person); err != nil {
		return 0, fmt.Errorf("conflict: forget standing: %w", err)
	}
	return tag.RowsAffected(), nil
}
