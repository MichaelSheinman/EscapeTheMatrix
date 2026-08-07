package account_test

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/account"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/oauth"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store/storetest"
)

// Accounts and sessions against a real Postgres (technical.md §14). The things
// worth proving here — a returning player is the same row, a signed-out token
// is dead everywhere, an expired one is refused by the database's clock rather
// than this process's — are the ones a mock would be inventing.

func newAccounts(t *testing.T) (*account.Accounts, *store.Store) {
	t.Helper()
	st := storetest.New(t)
	return account.New(st), st
}

func google(subject string) oauth.Identity {
	return oauth.Identity{
		Provider:  "google",
		Subject:   subject,
		Email:     subject + "@example.test",
		Name:      "Player " + subject,
		AvatarURL: "https://example.test/" + subject + ".png",
	}
}

func TestSignInOpensAResolvableSession(t *testing.T) {
	t.Parallel()
	accounts, _ := newAccounts(t)

	opened, token, err := accounts.SignIn(t.Context(), google("abc"))
	if err != nil {
		t.Fatalf("SignIn() = %v", err)
	}

	resolved, err := accounts.Resolve(t.Context(), token)
	if err != nil {
		t.Fatalf("Resolve() = %v, want the session just opened", err)
	}
	if resolved.Account.ID != opened.Account.ID {
		t.Errorf("resolved account %q, want %q", resolved.Account.ID, opened.Account.ID)
	}
	if resolved.Account.Email != "abc@example.test" || resolved.Account.Name != "Player abc" {
		t.Errorf("account = %+v, want the identity the provider gave", resolved.Account)
	}
	if !resolved.ExpiresAt.After(opened.ExpiresAt.Add(-1)) {
		t.Error("the resolved session does not carry its expiry")
	}
}

// The identity is (provider, subject). Everything else the provider says is a
// label, refreshed each time and never joined on.
func TestReturningPlayerIsTheSameAccount(t *testing.T) {
	t.Parallel()
	accounts, st := newAccounts(t)

	first, _, err := accounts.SignIn(t.Context(), google("abc"))
	if err != nil {
		t.Fatalf("SignIn() = %v", err)
	}

	renamed := google("abc")
	renamed.Email = "moved@example.test"
	renamed.Name = "Renamed"

	second, _, err := accounts.SignIn(t.Context(), renamed)
	if err != nil {
		t.Fatalf("SignIn() = %v", err)
	}

	if second.Account.ID != first.Account.ID {
		t.Errorf("signing in again minted a new account (%q then %q)", first.Account.ID, second.Account.ID)
	}
	if second.Account.Email != "moved@example.test" || second.Account.Name != "Renamed" {
		t.Errorf("account = %+v, want the labels refreshed from the provider", second.Account)
	}
	if got := countAccounts(t, st); got != 1 {
		t.Errorf("%d accounts, want 1", got)
	}
}

// Two providers' subjects live in different namespaces, so the same string from
// each is two people.
func TestSubjectsAreScopedToTheirProvider(t *testing.T) {
	t.Parallel()
	accounts, _ := newAccounts(t)

	fromGoogle, _, err := accounts.SignIn(t.Context(), google("abc"))
	if err != nil {
		t.Fatalf("SignIn() = %v", err)
	}
	elsewhere := google("abc")
	elsewhere.Provider = "discord"

	fromDiscord, _, err := accounts.SignIn(t.Context(), elsewhere)
	if err != nil {
		t.Fatalf("SignIn() = %v", err)
	}

	if fromGoogle.Account.ID == fromDiscord.Account.ID {
		t.Error("the same subject at two providers resolved to one account")
	}
}

// Signing in on a second device does not evict the first.
func TestSessionsAreIndependent(t *testing.T) {
	t.Parallel()
	accounts, _ := newAccounts(t)

	_, laptop, err := accounts.SignIn(t.Context(), google("abc"))
	if err != nil {
		t.Fatalf("SignIn() = %v", err)
	}
	_, phone, err := accounts.SignIn(t.Context(), google("abc"))
	if err != nil {
		t.Fatalf("SignIn() = %v", err)
	}
	if laptop == phone {
		t.Fatal("two sign-ins produced the same token")
	}

	if err := accounts.SignOut(t.Context(), phone); err != nil {
		t.Fatalf("SignOut() = %v", err)
	}
	if _, err := accounts.Resolve(t.Context(), phone); !errors.Is(err, account.ErrNoSession) {
		t.Errorf("Resolve(signed out) = %v, want ErrNoSession", err)
	}
	if _, err := accounts.Resolve(t.Context(), laptop); err != nil {
		t.Errorf("signing out one session killed another: %v", err)
	}
}

// The acceptance criterion, at the level the database can prove it: after
// signing out the token is inert, whatever the browser did with its cookie.
func TestSignOutInvalidatesServerSide(t *testing.T) {
	t.Parallel()
	accounts, _ := newAccounts(t)

	_, token, err := accounts.SignIn(t.Context(), google("abc"))
	if err != nil {
		t.Fatalf("SignIn() = %v", err)
	}
	if err := accounts.SignOut(t.Context(), token); err != nil {
		t.Fatalf("SignOut() = %v", err)
	}

	if _, err := accounts.Resolve(t.Context(), token); !errors.Is(err, account.ErrNoSession) {
		t.Errorf("Resolve() = %v, want ErrNoSession", err)
	}
	// Twice is not an error: there is no state in which the caller should be
	// told the difference.
	if err := accounts.SignOut(t.Context(), token); err != nil {
		t.Errorf("signing out twice = %v, want nil", err)
	}
}

func TestExpiredSessionIsRefused(t *testing.T) {
	t.Parallel()
	accounts, st := newAccounts(t)

	_, token, err := accounts.SignIn(t.Context(), google("abc"))
	if err != nil {
		t.Fatalf("SignIn() = %v", err)
	}
	if _, err := st.Pool().Exec(t.Context(),
		`UPDATE session SET expires_at = now() - interval '1 second'`); err != nil {
		t.Fatalf("age the session: %v", err)
	}

	if _, err := accounts.Resolve(t.Context(), token); !errors.Is(err, account.ErrNoSession) {
		t.Errorf("Resolve(expired) = %v, want ErrNoSession", err)
	}
}

func TestUnknownTokenIsRefused(t *testing.T) {
	t.Parallel()
	accounts, _ := newAccounts(t)

	for _, token := range []string{"", "not-a-token"} {
		if _, err := accounts.Resolve(t.Context(), token); !errors.Is(err, account.ErrNoSession) {
			t.Errorf("Resolve(%q) = %v, want ErrNoSession", token, err)
		}
	}
}

// The token is a bearer credential: a dump of this table must not be replayable
// as a set of sign-ins.
func TestTokensAreStoredHashed(t *testing.T) {
	t.Parallel()
	accounts, st := newAccounts(t)

	_, token, err := accounts.SignIn(t.Context(), google("abc"))
	if err != nil {
		t.Fatalf("SignIn() = %v", err)
	}

	var stored []byte
	if err := st.Pool().QueryRow(t.Context(), `SELECT token_hash FROM session`).Scan(&stored); err != nil {
		t.Fatalf("read the session row: %v", err)
	}
	if bytes.Contains(stored, []byte(token)) {
		t.Fatal("the session table holds the token itself")
	}
	if want := sha256.Sum256([]byte(token)); !bytes.Equal(stored, want[:]) {
		t.Error("the stored value is not the token's sha256")
	}
}

func countAccounts(t *testing.T, st *store.Store) int {
	t.Helper()
	var n int
	if err := st.Pool().QueryRow(t.Context(), `SELECT count(*) FROM account`).Scan(&n); err != nil {
		t.Fatalf("count accounts: %v", err)
	}
	return n
}

/*
§4's "time away", and the reason it is a column rather than a subtraction.

`last_seen_at` is touched by every catch-up rollover, which is every page load —
so a release policy that measured the gap for itself would find milliseconds
however long the player had actually been gone. Measuring the absence is what
ends it, so the gap is kept.

What this asserts is the whole of that: a fresh account has been nowhere, a real
absence is remembered, and being seen again inside one sitting does not erase
the absence that sitting began with.
*/
func TestTimeAwayIsKeptBecauseMeasuringItEndsIt(t *testing.T) {
	t.Parallel()
	accounts, st := newAccounts(t)

	opened, _, err := accounts.SignIn(t.Context(), google("away"))
	if err != nil {
		t.Fatalf("SignIn() = %v", err)
	}
	who := opened.Account.ID

	away := func() time.Duration {
		t.Helper()
		var out time.Duration
		if err := st.InTx(t.Context(), func(tx pgx.Tx) error {
			var err error
			out, err = accounts.Away(t.Context(), tx, who)
			return err
		}); err != nil {
			t.Fatalf("Away: %v", err)
		}
		return out
	}
	seen := func() {
		t.Helper()
		if err := st.InTx(t.Context(), func(tx pgx.Tx) error {
			return accounts.Seen(t.Context(), tx, who)
		}); err != nil {
			t.Fatalf("Seen: %v", err)
		}
	}

	if got := away(); got != 0 {
		t.Errorf("somebody who just signed in has been away %v", got)
	}

	// A fortnight, out of the database's clock rather than this process's.
	if _, err := st.Pool().Exec(t.Context(),
		`UPDATE account SET last_seen_at = now() - interval '14 days' WHERE id = $1`, who,
	); err != nil {
		t.Fatalf("age the account: %v", err)
	}
	seen()

	gap := away()
	if gap < 13*24*time.Hour || gap > 15*24*time.Hour {
		t.Fatalf("a fortnight away reads back as %v", gap)
	}

	// Still here a moment later. The absence that this sitting began with is
	// the same absence — a policy that read it as "back a second ago" would
	// treat somebody who has just returned like somebody who never left.
	seen()
	if got := away(); got != gap {
		t.Errorf("a second page load inside one sitting rewrote the absence: %v, was %v", got, gap)
	}

	// And a genuinely new sitting replaces it.
	if _, err := st.Pool().Exec(t.Context(),
		`UPDATE account SET last_seen_at = now() - $2::interval WHERE id = $1`,
		who, (account.Sitting + time.Hour).String(),
	); err != nil {
		t.Fatalf("age the account: %v", err)
	}
	seen()
	if got := away(); got >= gap || got < account.Sitting {
		t.Errorf("the next sitting began after %v, want a little over %v", got, account.Sitting)
	}

	// Somebody who is not there has not come back from anywhere.
	if err := st.InTx(t.Context(), func(tx pgx.Tx) error {
		got, err := accounts.Away(t.Context(), tx, "00000000-0000-0000-0000-000000000000")
		if got != 0 {
			t.Errorf("an account that does not exist has been away %v", got)
		}
		return err
	}); err != nil {
		t.Errorf("Away(nobody) = %v, want the same silence Active gives", err)
	}
}
