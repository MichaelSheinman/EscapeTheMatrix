/*
Package social owns peer endorsement: the one place in V1 where a factor is
moved by another human being.

§3.5 asks for an occasional "I like this user" button, rated rather than
always-available. It is in V1 because it is asynchronous by construction: nobody
has to be online, nobody has to answer, and the player being endorsed does
nothing. Everything else §3.7 wants between real players needs both present.

It is not a button beside somebody's name. That would be always available, and it
would need a way to address another player — which the leaderboards deliberately
do not hand out. What this ships is an offer: now and then the game names one
other player, for a while, and the only two things to do about it are press or let
it go. Nothing to type, nobody to look up, no way to choose who.

Four things stop it being farmed, and only one is a countdown:

  - [Cadence] — an offer is drawn at most once every twenty hours, and only one
    stands at a time (a partial unique index, not a guard in Go).
  - [Cap] over [Span] — at most five endorsements in any rolling week, checked
    independently of the cadence so a bug in one does not disable the other.
  - Once ever per pair: the primary key of endorsement is (from, to).
  - Never yourself: a CHECK constraint rather than a conditional.

The last two are in the schema on purpose — a rule a race can skip is not a rule.

The credit goes to the player who was liked, never the one who pressed. Being
liked is a statement about the person who was liked, and an endorsement that paid
the giver would be a free daily grant with a stranger's name attached (§7).

[Credits] is how the grant leaves, and its method cannot return a standing:
internal/factor's own Record hands back the recipient's composed Karma, and this
module is structurally unable to receive it.

It owns endorsement and endorsement_offer (§3.1), takes the caller's
transaction and never opens one (§3.2), and reads no other module's tables: who
else is playing arrives through [Crowd], and the factor grant leaves through
[Credits].
*/
package social

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
The knobs, all four of them, in one place.

§17 leaves tuning to implementation, and these are the numbers a playtest will
argue about. They are constants rather than configuration because they are game
rules: a server running a shorter cadence than another would be a different game,
and the boards those two populations share would not mean the same thing.
*/
const (
	// Cadence is the least time between two offers being drawn. Twenty hours
	// rather than twenty-four, so a player who opens the game at the same hour
	// each day is not permanently four minutes short of it.
	Cadence = 20 * time.Hour

	// Window is how long an offer stands before it lapses. An offer that sat
	// there until it was answered would be a button that is always available the
	// moment you next log in. Nothing is lost when one lapses except that
	// particular stranger, which is the point.
	Window = 8 * time.Hour

	// Cap is how many endorsements one player may give inside Span — the second,
	// independent bound. At one offer per Cadence a player could give about
	// eight a week, so this bites, and it is counted from the endorsement table
	// so it holds even if the cadence were wrong.
	Cap = 5

	// Span is the rolling window Cap is counted over.
	Span = 7 * 24 * time.Hour

	// Credit is what one endorsement is worth to the player who received it,
	// before the environment's familiarity is applied (§3.15). Modest against
	// [simulation.CreditHalf]: a first endorsement moves the reading by about a
	// ninth of its scale and the twentieth by almost nothing. A human signal
	// should be worth having and not worth organising a group around.
	Credit = 30.0

	// Remembered is how many of the people who have liked this player are named
	// back to them: a few, newest first. Not a feed — enough for the thing to
	// feel like people rather than like a counter (§3.2).
	Remembered = 5
)

// ErrNoOffer is an answer to an offer that is not standing: the wrong id,
// somebody else's, one already answered, or one whose window has passed.
// Deliberately one error — telling them apart would tell a client probing ids
// something about offers that were never theirs.
var ErrNoOffer = errors.New("social: there is no offer open by that name")

// ErrCapped is an endorsement past the rate limit. Defensive: an offer is not
// drawn while the player is capped, so reaching this means the two guards
// disagreed and the endorsement is refused by the one that counts real rows.
var ErrCapped = errors.New("social: that is as many as may be given for now")

// ErrAlready is an endorsement of somebody this player has already endorsed.
// The candidate filter excludes them, so this is the schema catching a case the
// selection was supposed to have removed.
var ErrAlready = errors.New("social: that player has already been endorsed")

/*
Player is who an offer names, and who is named back.

What a name and a face and nothing else buys is the same thing it buys the
leaderboards: "could endorsement leak an email address" becomes a question about
a type rather than about a code path. There is no account id on the wire either
— an offer is answered by the offer's id, and the server already knows who it
named.
*/
type Player struct {
	ID     string
	Name   string
	Avatar string
}

/*
Crowd is the population an offer may name, and who they are.

An interface for §3.1's reason: this package has no business reading the account
table. internal/api binds it in one small adapter, the same way it binds
internal/factor's Roster — and the two are separate interfaces over the same
accounts on purpose, because a leaderboard and an endorsement want different
guarantees from the same list and neither should inherit the other's.
*/
type Crowd interface {
	// Everyone is every account that may be offered.
	Everyone(ctx context.Context, tx pgx.Tx) ([]string, error)
	// Identify names the given accounts. Order need not match, and an account
	// that has since been deleted may simply be absent.
	Identify(ctx context.Context, tx pgx.Tx, ids []string) ([]Player, error)
}

// Alone is a Crowd with nobody else in it: there is never anybody to like.
// Never bound in the server.
type Alone struct{}

func (Alone) Everyone(context.Context, pgx.Tx) ([]string, error) { return nil, nil }

func (Alone) Identify(context.Context, pgx.Tx, []string) ([]Player, error) { return nil, nil }

/*
Credits is where an endorsement lands: the hidden factor §3.5 pairs it with.

Deliberately write-only, and deliberately returning nothing but an error.
internal/factor's Record hands back the account's composed standing, and this
module must not be able to see it — the endorser is in the middle of a command,
and a standing that reached this package could reach that command's response.
Narrowing it here is the whole guard.
*/
type Credits interface {
	Record(ctx context.Context, tx pgx.Tx, accountID string, grants ...simulation.Grant) error
}

// Unrecorded is a Credits that keeps none: endorsement happens and moves
// nothing. For tests that are about the offer rather than about the factor.
type Unrecorded struct{}

func (Unrecorded) Record(context.Context, pgx.Tx, string, ...simulation.Grant) error { return nil }

/*
Offer is one person the player may say they like, and until when.

The id is what an answer names. It is this player's offer or it is not an offer
at all — every read of it is keyed by (id, account) — which is why handing a
bigint to the browser discloses nothing: guessing one belonging to somebody else
answers ErrNoOffer exactly as guessing one that never existed does.
*/
type Offer struct {
	ID  int64
	Who Player
	// OfferedAt and ExpiresAt are the window, as the injected clock stated it.
	OfferedAt time.Time
	ExpiresAt time.Time
}

/*
Standing is the whole of the endorsement surface for one player.

Note the two halves. Above the line is what they may *do* — an offer, or the
reason there is not one. Below it is what has been done *to* them, which is the
part §3.5 is actually about: a human signal, arriving without being asked for.
*/
type Standing struct {
	// Offer is nil when there is nobody to like right now, which is most of the
	// time and is the design rather than a gap.
	Offer *Offer

	// Given inside Span, and the ceiling it is counted against. Both are sent
	// to the player: a limit nobody can see reads as the game being broken.
	Given int
	Cap   int

	// Opens is when the next offer may be drawn. Zero while one is standing,
	// and zero when one may be drawn the moment the player looks.
	Opens time.Time

	// Received is how many people have said they like this player, ever.
	Received int
	// Liked is the most recent few of them, newest first.
	Liked []Player
}

/*
Liked is what one endorsement did, for the benefit of the player who received it.

It exists so that internal/api can tell the *other* player — the one who did
nothing, whose tabs are open somewhere else, and for whom this is the clearest
case in the game of a message no response could have carried (§8).

Credit is on it because the transport publishes the standing that moved, not
because anybody is shown a factor: internal/factor strips the hidden ones on the
way out, exactly as it does for every other grant (§3.5).
*/
type Liked struct {
	// To is the account that was endorsed.
	To string
	// From is who liked them, by name. Named rather than anonymous, because an
	// anonymous endorsement is a notification. Safe to name, because the pairing
	// is one-directional and once-only, so there is no reciprocation to
	// arrange.
	From Player
	// Credit is the authored figure, before familiarity (§3.15).
	Credit float64
}

// Endorsements is the module. It holds no per-player state: the transaction
// comes from the caller and everything below is fixed at boot (§3.1, §3.2).
type Endorsements struct {
	clk     simulation.Clock
	crowd   Crowd
	credits Credits
}

// New binds the module to the clock it reads and the two things it cannot reach
// otherwise.
func New(clk simulation.Clock, crowd Crowd, credits Credits) *Endorsements {
	return &Endorsements{clk: clk, crowd: crowd, credits: credits}
}

/*
Of reads the endorsement surface without changing it.

The plain read: what a command's response restates once it has done its work. It
never draws an offer and never lapses one, so calling it is free of consequence
and it can be used from anywhere a standing is wanted.
*/
func (e *Endorsements) Of(ctx context.Context, tx pgx.Tx, accountID string) (Standing, error) {
	return e.standing(ctx, tx, accountID, e.clk.Now())
}

/*
Look is the read that may offer somebody — the moment the button appears.

A player looking at their world is the only trigger there is. There is no job
handing out offers to people who are not playing, which is what keeps the cadence
honest: a fortnight away does not accrue a fortnight of strangers, because an
offer is a thing to answer *now* rather than a resource to bank.

Three things happen here, in order, and each of them is a rule from §3.5:

 1. An offer whose window has passed is closed. It lapses rather than waiting,
    which is what stops the button from being always-available-on-next-login.
 2. If one is still standing, nothing is drawn. One at a time, held by a partial
    unique index rather than by this check — two tabs opening the world in the
    same instant is the ordinary case, not the exotic one.
 3. Otherwise, if the cadence has elapsed and the player is under the cap, one
    person is named.

The bool is whether step three actually happened, so that the transport can
announce a new offer to this player's other tabs and stay quiet the rest of the
time (§8: a message per page load would be the polling the protocol exists
instead of).

It writes, which is why it takes a transaction and not a pool — the same "a read
that may write" internal/health does for the body.
*/
func (e *Endorsements) Look(ctx context.Context, tx pgx.Tx, accountID string) (Standing, bool, error) {
	now := e.clk.Now()

	if err := e.lapse(ctx, tx, accountID, now); err != nil {
		return Standing{}, false, err
	}

	open, err := e.open(ctx, tx, accountID, now)
	if err != nil {
		return Standing{}, false, err
	}
	if open != nil {
		out, err := e.standing(ctx, tx, accountID, now)
		return out, false, err
	}

	drawn, err := e.draw(ctx, tx, accountID, now)
	if err != nil {
		return Standing{}, false, err
	}

	out, err := e.standing(ctx, tx, accountID, now)
	return out, drawn, err
}

/*
Endorse records that this player likes the one they were offered.

`offer` is the offer's own id and is the whole of the addressing: there is no
field here naming another player, because a command that took one would be a
command that let a client choose. Somebody else's offer, an expired one and one
already answered are all ErrNoOffer, and the refusal is decided by an UPDATE
rather than by a read — the row is claimed and answered in one statement, so two
tabs racing the same offer produce one endorsement and one refusal rather than
two of anything.

The grant goes to the player who was liked, in the caller's transaction, so an
endorsement whose command rolled back never moved anybody's factor.
*/
func (e *Endorsements) Endorse(ctx context.Context, tx pgx.Tx, accountID string, offer int64) (Standing, Liked, error) {
	now := e.clk.Now()

	// Claimed and answered in one statement. Whoever gets the row gets the
	// endorsement; there is no window between checking and writing for a second
	// delivery to land in.
	var candidate string
	err := tx.QueryRow(ctx, `
		UPDATE endorsement_offer
		   SET answered_at = $3, answer = 'liked'
		 WHERE id = $1 AND account_id = $2
		   AND answered_at IS NULL AND expires_at > $3
		RETURNING candidate_id`, offer, accountID, now).Scan(&candidate)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Standing{}, Liked{}, ErrNoOffer
	case err != nil:
		return Standing{}, Liked{}, fmt.Errorf("social: answer offer: %w", err)
	}

	// The second bound, counted over real endorsements rather than over offers.
	given, err := e.given(ctx, tx, accountID, now)
	if err != nil {
		return Standing{}, Liked{}, err
	}
	if given >= Cap {
		return Standing{}, Liked{}, ErrCapped
	}

	var kept bool
	err = tx.QueryRow(ctx, `
		INSERT INTO endorsement (from_account, to_account, at, credit)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (from_account, to_account) DO NOTHING
		RETURNING true`, accountID, candidate, now, Credit).Scan(&kept)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// The primary key held. Reported rather than swallowed: the candidate
		// filter should have excluded this person, and silence would hide a
		// selection bug behind a button that appeared to work.
		return Standing{}, Liked{}, ErrAlready
	case err != nil:
		return Standing{}, Liked{}, fmt.Errorf("social: record endorsement: %w", err)
	}

	// §3.5 pairs endorsement with the kindness factor, and internal/factor
	// applies §3.15's familiarity itself: the environment the recipient is
	// standing in decides what the grant is worth.
	if err := e.credits.Record(ctx, tx, candidate,
		simulation.Grant{Factor: simulation.Kindness, Credit: Credit}); err != nil {
		return Standing{}, Liked{}, err
	}

	who, err := e.name(ctx, tx, accountID)
	if err != nil {
		return Standing{}, Liked{}, err
	}

	out, err := e.standing(ctx, tx, accountID, now)
	if err != nil {
		return Standing{}, Liked{}, err
	}
	return out, Liked{To: candidate, From: who, Credit: Credit}, nil
}

/*
Pass declines the offer.

It exists because the alternative is worse: without it, a player who does not
like the look of somebody either presses a button they mean nothing by or leaves
the offer to rot for eight hours. Passing is not a judgement anybody is told
about — nothing is written against the candidate and nobody is notified — and it
does not buy a replacement, because the next one comes on the cadence either way.

Somebody passed over is filtered out of this player's candidates for a week
rather than forever ([Span]), so a small population does not empty its pool.
*/
func (e *Endorsements) Pass(ctx context.Context, tx pgx.Tx, accountID string, offer int64) (Standing, error) {
	now := e.clk.Now()

	var candidate string
	err := tx.QueryRow(ctx, `
		UPDATE endorsement_offer
		   SET answered_at = $3, answer = 'passed'
		 WHERE id = $1 AND account_id = $2
		   AND answered_at IS NULL AND expires_at > $3
		RETURNING candidate_id`, offer, accountID, now).Scan(&candidate)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Standing{}, ErrNoOffer
	case err != nil:
		return Standing{}, fmt.Errorf("social: pass offer: %w", err)
	}

	return e.standing(ctx, tx, accountID, now)
}

/* ── drawing somebody ─────────────────────────────────────────────────── */

/*
draw names one person, if the game owes this player one.

The cadence is measured from the last offer *made* rather than from the last one
answered, so letting one lapse costs nothing but that stranger — a player who
ignores the button is not punished with a longer wait, and one who answers
instantly is not rewarded with a shorter one.

The insert can lose to a concurrent tab, which the partial unique index catches;
that is not an error, it means somebody has already been drawn and the standing
read a moment later will carry them.
*/
func (e *Endorsements) draw(ctx context.Context, tx pgx.Tx, accountID string, now time.Time) (bool, error) {
	given, err := e.given(ctx, tx, accountID, now)
	if err != nil {
		return false, err
	}
	if given >= Cap {
		// Capped, so no offer is drawn: showing somebody a person they are not
		// allowed to like is a worse screen than showing them nobody.
		return false, nil
	}

	opens, err := e.opens(ctx, tx, accountID)
	if err != nil {
		return false, err
	}
	if now.Before(opens) {
		return false, nil
	}

	candidate, err := e.candidate(ctx, tx, accountID, now)
	if err != nil || candidate == "" {
		return false, err
	}

	tag, err := tx.Exec(ctx, `
		INSERT INTO endorsement_offer (account_id, candidate_id, offered_at, expires_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT DO NOTHING`, accountID, candidate, now, now.Add(Window))
	if err != nil {
		return false, fmt.Errorf("social: make offer: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

/*
candidate picks who to offer.

Three exclusions and one ordering, and both halves are design decisions rather
than query convenience.

The exclusions are: never yourself; never somebody already endorsed (the pairing
is once-ever, so offering them again could only produce a button that fails); and
never somebody offered inside the last [Span], so that passing on a person means
not seeing them tomorrow. The third is a week rather than forever because a small
population would otherwise run out of strangers, and running out permanently is a
worse failure than seeing somebody again in a week.

The ordering is **fewest endorsements received first**, tie-broken by how long
they have been playing. That is the part worth arguing about, and the argument is
§3.5's: endorsement exists to give kindness a human signal, and a signal that
concentrated on whoever was already visible would measure visibility instead. It
also means every player is reachable, which "offer a random stranger" does not
guarantee in a population where most people never look.

It is O(players), like the leaderboard rebuild and for the same reason: the
account table is not this module's to select from, so the roster arrives as a
list and the sort happens here. A population that outgrows that wants a windowed
candidate pool, not a join into somebody else's tables.
*/
func (e *Endorsements) candidate(ctx context.Context, tx pgx.Tx, accountID string, now time.Time) (string, error) {
	everyone, err := e.crowd.Everyone(ctx, tx)
	if err != nil {
		return "", err
	}

	skip := map[string]bool{accountID: true}

	endorsed, err := ids(ctx, tx,
		`SELECT to_account FROM endorsement WHERE from_account = $1`, accountID)
	if err != nil {
		return "", err
	}
	for _, id := range endorsed {
		skip[id] = true
	}

	seen, err := ids(ctx, tx,
		`SELECT candidate_id FROM endorsement_offer
		  WHERE account_id = $1 AND offered_at > $2`, accountID, now.Add(-Span))
	if err != nil {
		return "", err
	}
	for _, id := range seen {
		skip[id] = true
	}

	pool := make([]string, 0, len(everyone))
	for _, id := range everyone {
		if !skip[id] {
			pool = append(pool, id)
		}
	}
	if len(pool) == 0 {
		return "", nil
	}

	received, err := e.receivedBy(ctx, tx, pool)
	if err != nil {
		return "", err
	}

	// pool is already in the roster's own order, oldest account first, so the
	// first element with the lowest count is the answer.
	best, at := pool[0], received[pool[0]]
	for _, id := range pool[1:] {
		if received[id] < at {
			best, at = id, received[id]
		}
	}
	return best, nil
}

/* ── reading ──────────────────────────────────────────────────────────── */

// standing composes the whole surface at one instant.
func (e *Endorsements) standing(ctx context.Context, tx pgx.Tx, accountID string, now time.Time) (Standing, error) {
	out := Standing{Cap: Cap, Liked: []Player{}}

	open, err := e.open(ctx, tx, accountID, now)
	if err != nil {
		return Standing{}, err
	}
	out.Offer = open

	if out.Given, err = e.given(ctx, tx, accountID, now); err != nil {
		return Standing{}, err
	}

	// When the next one may be drawn, and only while there is nothing on screen:
	// an offer standing now is already the answer to "when".
	if out.Offer == nil {
		opens, err := e.opens(ctx, tx, accountID)
		if err != nil {
			return Standing{}, err
		}
		if opens.After(now) {
			out.Opens = opens
		}
	}

	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM endorsement WHERE to_account = $1`, accountID,
	).Scan(&out.Received); err != nil {
		return Standing{}, fmt.Errorf("social: count endorsements received: %w", err)
	}

	admirers, err := ids(ctx, tx, `
		SELECT from_account FROM endorsement
		 WHERE to_account = $1 ORDER BY at DESC, from_account LIMIT $2`, accountID, Remembered)
	if err != nil {
		return Standing{}, err
	}
	if len(admirers) > 0 {
		named, err := e.crowd.Identify(ctx, tx, admirers)
		if err != nil {
			return Standing{}, err
		}
		by := make(map[string]Player, len(named))
		for _, p := range named {
			by[p.ID] = p
		}
		// Identify may answer in any order and may drop an account that has
		// since been deleted, so the ordering is reapplied from the ids.
		for _, id := range admirers {
			if p, ok := by[id]; ok {
				out.Liked = append(out.Liked, p)
			}
		}
	}
	return out, nil
}

// open reads the offer standing right now, or nil.
func (e *Endorsements) open(ctx context.Context, tx pgx.Tx, accountID string, now time.Time) (*Offer, error) {
	var (
		out       Offer
		candidate string
	)
	err := tx.QueryRow(ctx, `
		SELECT id, candidate_id, offered_at, expires_at
		  FROM endorsement_offer
		 WHERE account_id = $1 AND answered_at IS NULL AND expires_at > $2`,
		accountID, now).Scan(&out.ID, &candidate, &out.OfferedAt, &out.ExpiresAt)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("social: read offer: %w", err)
	}

	who, err := e.name(ctx, tx, candidate)
	if err != nil {
		return nil, err
	}
	if who.ID == "" {
		// Offered somebody who has since deleted their account. Not an error:
		// there is nobody to like, which is a screen the player already has.

		return nil, nil
	}
	out.Who = who
	return &out, nil
}

// lapse closes an offer whose window has passed. Written rather than left
// implicit, because "there is one open" is a partial unique index and an
// unanswered offer would hold the slot forever.
func (e *Endorsements) lapse(ctx context.Context, tx pgx.Tx, accountID string, now time.Time) error {
	_, err := tx.Exec(ctx, `
		UPDATE endorsement_offer
		   SET answered_at = $2, answer = 'lapsed'
		 WHERE account_id = $1 AND answered_at IS NULL AND expires_at <= $2`, accountID, now)
	if err != nil {
		return fmt.Errorf("social: lapse offer: %w", err)
	}
	return nil
}

// given counts what this player has handed out inside Span.
func (e *Endorsements) given(ctx context.Context, tx pgx.Tx, accountID string, now time.Time) (int, error) {
	var n int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM endorsement
		 WHERE from_account = $1 AND at > $2`, accountID, now.Add(-Span)).Scan(&n); err != nil {
		return 0, fmt.Errorf("social: count endorsements given: %w", err)
	}
	return n, nil
}

// opens is when the next offer may be drawn: Cadence after the last one was
// made. The zero time for a player who has never been offered anybody, which
// compares before every real instant and therefore reads as "now".
func (e *Endorsements) opens(ctx context.Context, tx pgx.Tx, accountID string) (time.Time, error) {
	var last *time.Time
	if err := tx.QueryRow(ctx,
		`SELECT max(offered_at) FROM endorsement_offer WHERE account_id = $1`,
		accountID).Scan(&last); err != nil {
		return time.Time{}, fmt.Errorf("social: read last offer: %w", err)
	}
	if last == nil {
		return time.Time{}, nil
	}
	return last.Add(Cadence), nil
}

// receivedBy counts endorsements received, for a set of accounts. Absent from
// the answer means none, which is the state most players are in.
func (e *Endorsements) receivedBy(ctx context.Context, tx pgx.Tx, accounts []string) (map[string]int, error) {
	out := make(map[string]int, len(accounts))
	rows, err := tx.Query(ctx, `
		SELECT to_account, count(*) FROM endorsement
		 WHERE to_account = ANY($1) GROUP BY to_account`, accounts)
	if err != nil {
		return nil, fmt.Errorf("social: count endorsements: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id string
			n  int
		)
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("social: count endorsements: %w", err)
		}
		out[id] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("social: count endorsements: %w", err)
	}
	return out, nil
}

// name is one player, as far as this package is allowed to know them. The zero
// Player for an account that is no longer there.
func (e *Endorsements) name(ctx context.Context, tx pgx.Tx, accountID string) (Player, error) {
	named, err := e.crowd.Identify(ctx, tx, []string{accountID})
	if err != nil {
		return Player{}, err
	}
	for _, p := range named {
		if p.ID == accountID {
			return p, nil
		}
	}
	return Player{}, nil
}

// ids runs a query answering one uuid column.
func ids(ctx context.Context, tx pgx.Tx, sql string, args ...any) ([]string, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("social: read accounts: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("social: read accounts: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("social: read accounts: %w", err)
	}
	return out, nil
}
