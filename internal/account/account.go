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
