package challenge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

/*
The gate: what has to be true before a challenge reaches a player at all.

§4 gives a challenge two ways to be reachable — it stands somewhere ([Definition.Where])
or an event offers it ([Definition.Opens]) — and neither of them is a *when*. A
challenge that stands in a doorway stands there from the first minute, for
everybody, until somebody plays it. That is the right answer for the small ones,
and it is obviously the wrong one for the big ones: a house on fire two doors
down is not a fixture of the neighbourhood, it is something the world decided has
happened to you, today, once.

So a definition declares which of the two it is, and the answer is a [Gate].

# Why this is a placeholder, and why it is a real one

What eventually decides that today is the day is §4's release policy grown up —
the same machinery that already decides which event to surface, asked a harder
question. That is not this ticket, and pretending otherwise by writing half a
selector would be worse than admitting it.

What *is* this ticket is the seam. A gated challenge asks one question —
[Challenges.Ungated] — and everything downstream of that question is finished:
the surface hides what is shut, the begin command refuses it, the run path is
untouched, and the tests are written against the question rather than against
whatever answers it. Today the answer comes from a row a developer's toolbar
wrote. When the policy arrives it writes the same row, and nothing above this
line changes.

That is the difference between a placeholder and a stub. The stub would be a
boolean somewhere that everybody agrees to remove later.

# Absence is the closed state

There is no row per (player, challenge) waiting to be flipped. A challenge
nobody has been given has no rows at all, and turning one off deletes rather than
writes false — which is what keeps `challenge_flag` from becoming a copy of the
catalogue with a boolean on it, and from being read by anybody as the list of
challenges there are. §4 keeps the world a sandbox rather than a quest list, and
a table you could SELECT the quest list out of is a quest list.
*/

// Gate is the condition on a challenge being reachable at all.
//
// Text rather than a boolean, and for the reason [Type] is text: the set grows.
// The next value is not `flag`'s opposite — it is `after: some-other-challenge`,
// or a struggle the player has actually had — and a boolean would have to be
// migrated into an enum on the day that arrives.
type Gate string

const (
	// Open is a challenge that reaches anybody who meets it. The default, and
	// what every challenge authored before the gate existed is.
	Open Gate = "open"

	// Flagged is a challenge that reaches nobody until it has been turned on for
	// them, one player at a time.
	Flagged Gate = "flag"
)

// Gates is every gate an author may write, in the order they are documented.
var Gates = []Gate{Open, Flagged}

// gate reads what an author wrote, defaulting an empty field to [Open].
//
// Empty is the default rather than an error because the field arrived after the
// content did: five authored challenges predate it, and a required field would
// have made adding the gate a rewrite of all of them. It is the only field in
// challenge.yaml with a default, and it earns it by being the one whose absence
// has an obviously correct meaning.
func gate(written string) (Gate, error) {
	switch got := Gate(strings.TrimSpace(written)); got {
	case "":
		return Open, nil
	case Open, Flagged:
		return got, nil
	default:
		return "", fmt.Errorf("is gated %q, which is not %s or %s", got, Open, Flagged)
	}
}

// Gated reports whether this challenge waits to be turned on for a player.
func (d Definition) Gated() bool { return d.Gate == Flagged }

/*
Ungated reports whether this challenge may reach this player yet.

The one question the gate exists to be asked, and the only place anything above
this package learns that gates exist. An [Open] challenge answers true without a
query — which matters more than it looks: the surface asks this of every
definition standing where the player is, on every walk, and a round trip per
challenge to learn what the definition already said would be a table read to
answer a question content had answered at boot.

It is deliberately not "is there a flag row": a caller that asked *that* would
have to know which challenges care, and would get it wrong for the one that was
regated between two deploys.
*/
func (c *Challenges) Ungated(ctx context.Context, tx pgx.Tx, accountID string, d Definition) (bool, error) {
	if !d.Gated() {
		return true, nil
	}
	var on bool
	err := tx.QueryRow(ctx, `
		SELECT true FROM challenge_flag
		 WHERE account_id = $1 AND challenge = $2`, accountID, string(d.Code)).Scan(&on)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("challenge: read the gate on %s: %w", d.Code, err)
	}
	return true, nil
}

/*
Enable turns a gated challenge on for one player.

`by` is who decided — `dev` today, a release policy later — and it is recorded
rather than assumed because the whole point of the seam is that the answerer
changes and the question does not. Idempotent: turning on something already on
leaves the original decision's timestamp alone, so a toolbar pressed twice does
not rewrite the moment the world chose you.

Enabling a challenge that is not gated is refused rather than ignored. It would
be a no-op — an [Open] challenge already reaches everybody — and a no-op that
looks like it worked is how somebody spends an afternoon wondering why their
flag did nothing.
*/
func (c *Challenges) Enable(ctx context.Context, tx pgx.Tx, accountID string, code Code, by string) error {
	authored, known := Known(code)
	switch {
	case !known:
		return fmt.Errorf("%w: %q", ErrUnknown, code)
	case !authored.Gated():
		return fmt.Errorf("%w: %q is %s and reaches everybody already", ErrNotGated, code, authored.Gate)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO challenge_flag (account_id, challenge, enabled_at, by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (account_id, challenge) DO NOTHING`,
		accountID, string(code), c.clk.Now(), by,
	); err != nil {
		return fmt.Errorf("challenge: open the gate on %s: %w", code, err)
	}
	return nil
}

// Disable shuts the gate again.
//
// A delete, because absence is the closed state. Silent on a challenge that was
// never on: shutting a shut gate is what the caller wanted either way, and the
// distinction is not one anybody can act on.
func (c *Challenges) Disable(ctx context.Context, tx pgx.Tx, accountID string, code Code) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM challenge_flag WHERE account_id = $1 AND challenge = $2`,
		accountID, string(code),
	); err != nil {
		return fmt.Errorf("challenge: shut the gate on %s: %w", code, err)
	}
	return nil
}

// ErrNotGated is turning a gate on something that has none.
var ErrNotGated = errors.New("challenge: that challenge is not gated")

/*
Opened is every gated challenge currently on for one player.

Deliberately not part of the player-facing surface, and it is worth saying why
out loud so nobody wires it into one later. §4's argument against sending a
client the backlog applies here word for word: a player who could read which
challenges had been turned on for them could route towards them, and the sandbox
would become a list of things to go and do. The only caller is the developer
toolbar, which is not a player.
*/
func (c *Challenges) Opened(ctx context.Context, tx pgx.Tx, accountID string) (map[Code]time.Time, error) {
	rows, err := tx.Query(ctx, `
		SELECT challenge, enabled_at FROM challenge_flag WHERE account_id = $1`, accountID)
	if err != nil {
		return nil, fmt.Errorf("challenge: read the gates: %w", err)
	}
	defer rows.Close()

	out := map[Code]time.Time{}
	for rows.Next() {
		var (
			code string
			at   time.Time
		)
		if err := rows.Scan(&code, &at); err != nil {
			return nil, fmt.Errorf("challenge: read the gates: %w", err)
		}
		out[Code(code)] = at
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("challenge: read the gates: %w", err)
	}
	return out, nil
}

/*
Forget deletes a player's run of one challenge, so it can be played again.

One caller in the game, and it is interviews (internal/api's begin). Once ever
per player (§5.1) is a design commitment: a challenge is a committed judgement
with authored consequences, and a replay would tell the player none of it was
load-bearing. That holds for everything the world hands somebody unasked, and not
for a job application — an interview is a door the player chose to walk at, and
one bad answer sealing a career forever is harsher than the content was written
to carry. Adding a second caller means arguing this case again.

It also exists because authoring a six-minute challenge means playing it forty
times, and the alternative is a developer editing challenge_run by hand.
internal/api registers that caller only outside production.

It does not un-pay the run. The factors granted stay granted, the money stays
handed over and the event stays settled: those went to other modules through
interfaces this package cannot read back (see [Awards], [Purse], [Doorway]).
Replaying a challenge replays the content rather than rewinding the world.
*/
func (c *Challenges) Forget(ctx context.Context, tx pgx.Tx, accountID string, code Code) (bool, error) {
	tag, err := tx.Exec(ctx, `
		DELETE FROM challenge_run WHERE account_id = $1 AND challenge = $2`,
		accountID, string(code))
	if err != nil {
		return false, fmt.Errorf("challenge: forget the run of %s: %w", code, err)
	}
	return tag.RowsAffected() > 0, nil
}
