// Package account owns who a player is and the sessions that speak for them.
//
// technical.md §9: account required up front, no anonymous play, sign-in by
// OAuth, sessions server-side in Postgres. This package is where those live —
// it owns the `account` and `session` tables and is the only thing that reads
// or writes them (§3.1).
//
// It knows nothing about HTTP. Cookies, redirects, and the middleware that puts
// an actor on a request belong to internal/api; what crosses the boundary is a
// token string in one direction and a Session in the other. That is what keeps
// "is this player signed in" answerable from a test with no server in it.
package account

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/oauth"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
)

// Lifetime is how long a session lasts from the moment it is opened.
//
// Absolute rather than sliding, and the cookie's Max-Age is set to match
// exactly. A sliding expiry means the server's idea of when the session ends
// and the browser's drift apart, and the player is signed out by whichever
// gives up first — for no gain over simply making the window long.
const Lifetime = 30 * 24 * time.Hour

// tokenBytes is the entropy in a session token. It is a bearer credential:
// whoever holds it is the player, so it has to be unguessable rather than
// merely unique.
const tokenBytes = 32

// ErrNoSession reports a token that stands for nothing — never issued, expired,
// or signed out. The three are deliberately one error: telling them apart would
// tell a caller holding a stolen token something about it.
var ErrNoSession = errors.New("account: no live session")

// Account is a signed-in player's identity.
//
// ID is a uuid this application minted, not anything the provider chose. It is
// what everything downstream keys on — the idempotency record's actor, and the
// tables later tickets hang off a player — so that changing or adding a sign-in
// provider never rewrites another module's rows.
type Account struct {
	ID        string
	Email     string
	Name      string
	AvatarURL string
}

// Session is a live sign-in: who, and until when.
type Session struct {
	Account   Account
	ExpiresAt time.Time
}

// Accounts reads and writes the account and session tables.
type Accounts struct {
	st *store.Store
}

// New binds the module to its database.
func New(st *store.Store) *Accounts { return &Accounts{st: st} }

// SignIn records an identity a provider vouched for and opens a session for it,
// returning the session and the token that stands for it.
//
// The token is returned once and never again: only its hash is stored, so this
// is the sole moment it exists anywhere but the player's cookie.
//
// Both writes are one transaction. A session pointing at an account that was
// not committed, or an account created for a sign-in that then failed to open a
// session, are states that cannot be reached rather than states to handle.
func (a *Accounts) SignIn(ctx context.Context, id oauth.Identity) (Session, string, error) {
	token, err := newToken()
	if err != nil {
		return Session{}, "", err
	}
	hash := hashToken(token)

	var sess Session
	err = a.st.InTx(ctx, func(tx pgx.Tx) error {
		// Returning a player who signed in before is an UPDATE, not an INSERT:
		// the same (provider, subject) is the same person, and their name,
		// email, and picture are refreshed from what the provider says now.
		if err := tx.QueryRow(ctx, `
			INSERT INTO account (provider, subject, email, display_name, avatar_url)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (provider, subject) DO UPDATE
			SET email        = excluded.email,
			    display_name = excluded.display_name,
			    avatar_url   = excluded.avatar_url,
			    last_seen_at = now()
			RETURNING id, email, display_name, coalesce(avatar_url, '')
		`, id.Provider, id.Subject, id.Email, id.Name, nullable(id.AvatarURL),
		).Scan(&sess.Account.ID, &sess.Account.Email, &sess.Account.Name, &sess.Account.AvatarURL); err != nil {
			return fmt.Errorf("upsert account: %w", err)
		}

		// The expiry comes off the database clock, like every other timestamp
		// in the schema, so a server whose clock has drifted cannot issue a
		// session that outlives what the rest of the system believes.
		if err := tx.QueryRow(ctx, `
			INSERT INTO session (token_hash, account_id, expires_at)
			VALUES ($1, $2, now() + make_interval(secs => $3))
			RETURNING expires_at
		`, hash, sess.Account.ID, Lifetime.Seconds()).Scan(&sess.ExpiresAt); err != nil {
			return fmt.Errorf("open session: %w", err)
		}
		return nil
	})
	if err != nil {
		return Session{}, "", err
	}
	return sess, token, nil
}

// Resolve returns the live session a token stands for, or ErrNoSession.
//
// This runs on every command, so it is one read and no write. Whether the
// session is live is decided in the WHERE clause rather than in Go: the
// comparison is against the database's clock, and a row that fails it is a row
// this process never sees.
func (a *Accounts) Resolve(ctx context.Context, token string) (Session, error) {
	if token == "" {
		return Session{}, ErrNoSession
	}

	var sess Session
	err := a.st.Pool().QueryRow(ctx, `
		SELECT a.id, a.email, a.display_name, coalesce(a.avatar_url, ''), s.expires_at
		FROM session s
		JOIN account a ON a.id = s.account_id
		WHERE s.token_hash = $1
		  AND s.revoked_at IS NULL
		  AND s.expires_at > now()
	`, hashToken(token)).Scan(
		&sess.Account.ID, &sess.Account.Email, &sess.Account.Name, &sess.Account.AvatarURL, &sess.ExpiresAt)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Session{}, ErrNoSession
	case err != nil:
		return Session{}, fmt.Errorf("resolve session: %w", err)
	}
	return sess, nil
}

/*
Seen records that this account is being played right now.

§3.3 gates scheduled rollover work on recent activity so that dormant accounts
cost nothing, and `account.last_seen_at` is the column that question is asked
of. Sign-in already touches it; this is what keeps it current for a player who
stays signed in for a month, which is most of them.

It takes the caller's transaction rather than opening one because the thing that
calls it is also scheduling the next rollover off the answer, and an activity
record that committed while the schedule it justified rolled back would leave a
dormant player looking awake forever.

It also closes the gap it observes — see [Sitting] and [Accounts.Away]. That is
one statement rather than two because the two are the same act: the moment this
row says the player is here, the absence that preceded them is over and the only
place left to read it is a column.
*/
func (a *Accounts) Seen(ctx context.Context, tx pgx.Tx, accountID string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE account
		   SET last_away = CASE
		           WHEN now() - last_seen_at >= make_interval(secs => $2)
		           THEN now() - last_seen_at
		           ELSE last_away
		       END,
		       last_seen_at = now()
		 WHERE id = $1`, accountID, Sitting.Seconds(),
	); err != nil {
		return fmt.Errorf("account: touch last seen: %w", err)
	}
	return nil
}

/*
Sitting is the gap that ends one sitting and begins the next.

Below it, somebody is still here: a page reloaded, a tab reopened, a walk taken
after making a cup of tea. Above it they went away and came back, which is a
different fact about the same player and the one §4's release policy is asking
about.

Four hours rather than a day, because the thing being distinguished is a session
rather than a date. Two sessions in one evening are two sittings, and a player
who plays every morning has been away every night — which is true, and is not
the same kind of away as a fortnight.
*/
const Sitting = 4 * time.Hour

/*
Away is how long this account had been away when its current sitting began.

§4 lists "time away" as one of the three things release is decided from, and
this is the only honest way to ask it: [Accounts.Seen] runs on every page load,
so a policy that measured `now() - last_seen_at` for itself would find a gap of
milliseconds no matter how long the player had actually been gone. What is
wanted is the absence that *preceded* them, and it has to be kept because
measuring it destroys it.

Zero for an account that has never been away — a first day, or somebody who has
not yet left. Zero rather than an error, because "no absence" is a real answer
and every policy that reads this compares it against a threshold.
*/
func (a *Accounts) Away(ctx context.Context, tx pgx.Tx, accountID string) (time.Duration, error) {
	var secs *float64
	err := tx.QueryRow(ctx, `
		SELECT extract(epoch FROM last_away) FROM account WHERE id = $1
	`, accountID).Scan(&secs)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// An account that is not there has not come back from anywhere. The
		// same answer, and for the same reason, as Active's.
		return 0, nil
	case err != nil:
		return 0, fmt.Errorf("account: read time away: %w", err)
	case secs == nil:
		return 0, nil
	}
	return time.Duration(*secs * float64(time.Second)), nil
}

// Active reports whether this account has been played inside the given window.
//
// The read half of the gate above, and the only thing another module is told
// about an account's activity — internal/rollover asks it through a two-method
// interface rather than selecting from a table it does not own (§3.1).
//
// The comparison is made in the WHERE-clause rather than in Go on purpose. Every
// timestamp in this schema comes off the database's clock, so the question "how
// long ago was that" has to be asked of the same clock — asking a process whose
// own time has drifted would make a player dormant on one instance and awake on
// the next.
func (a *Accounts) Active(ctx context.Context, tx pgx.Tx, accountID string, within time.Duration) (bool, error) {
	var active bool
	err := tx.QueryRow(ctx, `
		SELECT last_seen_at > now() - make_interval(secs => $2)
		  FROM account WHERE id = $1
	`, accountID, within.Seconds()).Scan(&active)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// An account that is not there cannot have been seen. Not an error: to
		// the gate this is simply somebody who is not playing, which is the
		// correct answer and the one that schedules nothing.
		return false, nil
	case err != nil:
		return false, fmt.Errorf("account: read activity: %w", err)
	}
	return active, nil
}

/*
Profile is a player as other players may see them.

technical.md's module map calls this "leaderboard identity", and the type exists
so that the thing which needs it gets a name and a picture and nothing else. An
[Account] carries an email address, which is fine for the session that belongs to
it and is not fine on a public board — so the narrow type is what makes "could a
leaderboard leak an address" a question with a structural answer rather than a
review comment.
*/
type Profile struct {
	ID        string
	Name      string
	AvatarURL string
}

/*
Everyone lists every account, oldest first.

The population a leaderboard ranks (§3.5). Every account, not only the active
ones: a rank the player fell out of because they took a fortnight off would be a
board that quietly rewarded playing daily, which is the treadmill §3.2 is written
against. Dormancy decides what the world *schedules* (§3.3), not who exists.

It takes the caller's transaction because the rebuild that calls it is one
transaction over the whole table, and a roster read outside it could name an
account that is gone by the time a row is written for them.
*/
func (a *Accounts) Everyone(ctx context.Context, tx pgx.Tx) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT id FROM account ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("account: list accounts: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("account: list accounts: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

/*
Profiles names the given accounts.

One query for a page rather than one per row, and an account that has since been
deleted is simply absent from the answer rather than an error — a board built a
few minutes ago is allowed to name somebody who has left, and the caller's job is
to render what came back.
*/
func (a *Accounts) Profiles(ctx context.Context, tx pgx.Tx, ids []string) ([]Profile, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	rows, err := tx.Query(ctx, `
		SELECT id, display_name, coalesce(avatar_url, '')
		  FROM account WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("account: read profiles: %w", err)
	}
	defer rows.Close()

	out := make([]Profile, 0, len(ids))
	for rows.Next() {
		var p Profile
		if err := rows.Scan(&p.ID, &p.Name, &p.AvatarURL); err != nil {
			return nil, fmt.Errorf("account: read profiles: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SignOut invalidates a session server-side.
//
// After this the token is inert everywhere, immediately and on every instance,
// which is the property a cookie the client merely deletes does not have.
// Signing out twice, or signing out a token that was never issued, is not an
// error — there is no state in which the caller should be told the difference.
func (a *Accounts) SignOut(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	if _, err := a.st.Pool().Exec(ctx, `
		UPDATE session SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL
	`, hashToken(token)); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

// newToken draws a session token.
func newToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("account: draw session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// hashToken is what goes in the table.
//
// A plain sha256 rather than a password hash on purpose: the input is 256 bits
// of this server's own randomness, so there is no dictionary to slow an
// attacker down through, and a slow hash on the read path would tax every
// single command for nothing.
func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// nullable keeps an absent optional field out of the table as NULL rather than
// as an empty string, so "no picture" has one representation.
func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
