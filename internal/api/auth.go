package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/account"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/oauth"
)

// The auth routes. Under /api/ with everything else the server owns, so the SPA
// fallback in assets.go cannot shadow one by answering a typo with the app
// shell.
const (
	googleStartPath    = "/api/auth/google/start"
	googleCallbackPath = "/api/auth/google/callback"
	sessionPath        = "/api/auth/session"
	signOutPath        = "/api/auth/signout"
)

// The two cookies. sessionCookie is the long-lived one and carries a bearer
// token: whoever holds it is the player. signInCookie lasts the seconds between
// leaving for Google and coming back, holding the state and PKCE verifier for
// one attempt.
const (
	sessionCookie = "etm_session"
	signInCookie  = "etm_signin"
)

// signInWindow is how long a sign-in attempt may stay open. Long enough to pick
// an account and type a password, short enough that an abandoned attempt is not
// still redeemable tomorrow.
const signInWindow = 10 * time.Minute

// actorKey is the context key the middleware stores the signed-in account under.
// Unexported and of a unique type, so nothing outside this package can put an
// actor on a request.
type actorKey struct{}

// actorFrom returns the account the middleware resolved for this request.
func actorFrom(ctx context.Context) (account.Account, bool) {
	a, ok := ctx.Value(actorKey{}).(account.Account)
	return a, ok
}

// authenticated is the auth check: one middleware rather than a per-handler
// check (§8). It wraps the single command route and the single socket, so no
// command can be written that forgets it. There is no anonymous mode to fall
// through to (§9) — a request without a live session is refused.
func (s *Server) authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, err := s.resolveSession(r)
		switch {
		case errors.Is(err, account.ErrNoSession):
			// The browser is holding a token that stands for nothing: expired,
			// or signed out on another device. Clearing it stops every later
			// request carrying a credential that cannot work.
			http.SetCookie(w, s.expireCookie(sessionCookie))
			writeJSON(w, http.StatusUnauthorized, errorBody("sign in to do that"))
			return
		case err != nil:
			s.log.Error("resolve session", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorBody("the command could not be completed"))
			return
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey{}, sess.Account)))
	})
}

// resolveSession looks up the session the request's cookie stands for.
func (s *Server) resolveSession(r *http.Request) (account.Session, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return account.Session{}, account.ErrNoSession
	}
	return s.accounts.Resolve(r.Context(), c.Value)
}

// startGoogleSignIn sends the browser to Google. The state and PKCE verifier go
// into a short-lived cookie rather than a table: they are worth one callback, and
// a row per abandoned sign-in would be a table to sweep. The cookie is httpOnly,
// so the page that triggered the redirect cannot read either value.
func (s *Server) startGoogleSignIn(w http.ResponseWriter, r *http.Request) {
	flow, err := oauth.NewFlow()
	if err != nil {
		s.log.Error("start sign-in", "error", err)
		s.failSignIn(w, r, "failed")
		return
	}

	http.SetCookie(w, s.cookie(signInCookie, flow.State+":"+flow.Verifier, signInWindow))
	http.Redirect(w, r, s.google.AuthCodeURL(flow), http.StatusFound)
}

// completeGoogleSignIn is where Google sends the browser back. Every refusal
// sends the player to the app with a reason in the query string: this is a
// top-level navigation rather than a fetch, so a JSON error body would render as
// a page of text.
func (s *Server) completeGoogleSignIn(w http.ResponseWriter, r *http.Request) {
	// Whatever happens next, this attempt is over: the cookie goes now, so a
	// replayed callback has no verifier to work with.
	flow, ok := s.takeSignInFlow(w, r)

	if reason := r.URL.Query().Get("error"); reason != "" {
		// The player pressed cancel on Google's consent screen, or Google
		// declined. Not an error worth logging as one.
		s.failSignIn(w, r, "declined")
		return
	}
	if !ok {
		// No cookie: the attempt expired, the browser is blocking cookies, or
		// this callback was not started here at all.
		s.failSignIn(w, r, "expired")
		return
	}

	// The state check is what makes this callback provably the continuation of
	// a sign-in this browser started.
	if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("state")), []byte(flow.State)) != 1 {
		s.log.Warn("sign-in callback with a state that does not match", "path", r.URL.Path)
		s.failSignIn(w, r, "failed")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		s.failSignIn(w, r, "failed")
		return
	}

	identity, err := s.google.Identity(r.Context(), code, flow)
	if err != nil {
		s.log.Error("google sign-in", "error", err)
		s.failSignIn(w, r, "failed")
		return
	}

	sess, token, err := s.accounts.SignIn(r.Context(), identity)
	if err != nil {
		s.log.Error("open session", "error", err)
		s.failSignIn(w, r, "failed")
		return
	}

	// A player who is signed in has to land in a world. The character is
	// created here, before the cookie is set, so every handler after this point
	// can assume one exists.
	if err := s.store.InTx(r.Context(), func(tx pgx.Tx) error {
		_, err := s.characters.Ensure(r.Context(), tx, sess.Account.ID)
		return err
	}); err != nil {
		s.log.Error("create character", "error", err)
		s.failSignIn(w, r, "failed")
		return
	}

	// A player who has been away long enough to fall off the schedule is put
	// back on it here, and the days they missed are applied before they see
	// anything. After the character exists, because the catch-up needs the
	// timezone.
	s.wake(r.Context(), sess.Account.ID)

	http.SetCookie(w, s.cookie(sessionCookie, token, account.Lifetime))
	s.log.Info("signed in", "provider", identity.Provider)

	// 303 so the browser follows with a GET and the callback URL — which
	// carries a spent authorization code — is gone from the address bar.
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// takeSignInFlow reads the sign-in cookie and expires it in the same breath.
func (s *Server) takeSignInFlow(w http.ResponseWriter, r *http.Request) (oauth.Flow, bool) {
	c, err := r.Cookie(signInCookie)
	if err != nil {
		return oauth.Flow{}, false
	}
	http.SetCookie(w, s.expireCookie(signInCookie))

	state, verifier, ok := strings.Cut(c.Value, ":")
	if !ok || state == "" || verifier == "" {
		return oauth.Flow{}, false
	}
	return oauth.Flow{State: state, Verifier: verifier}, true
}

// failSignIn returns the player to the app with a reason the client can render.
func (s *Server) failSignIn(w http.ResponseWriter, r *http.Request, reason string) {
	http.Redirect(w, r, "/?signin="+reason, http.StatusSeeOther)
}

// currentSession is how the app learns who it is serving, on every load. A GET
// rather than a command, and being signed out is a 200 with no player.
func (s *Server) currentSession(w http.ResponseWriter, r *http.Request) {
	type player struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatarUrl"`
	}
	type body struct {
		Player *player `json:"player"`
	}

	// Never cached anywhere: a shared cache holding this would hand one
	// player's identity to the next.
	w.Header().Set("Cache-Control", "no-store")

	sess, err := s.resolveSession(r)
	switch {
	case errors.Is(err, account.ErrNoSession):
		writeJSON(w, http.StatusOK, body{})
		return
	case err != nil:
		s.log.Error("resolve session", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorBody("the session could not be read"))
		return
	}

	writeJSON(w, http.StatusOK, body{Player: &player{
		ID:        sess.Account.ID,
		Name:      sess.Account.Name,
		Email:     sess.Account.Email,
		AvatarURL: sess.Account.AvatarURL,
	}})
}

// signOut invalidates the session server-side and clears the cookie. A route
// rather than a command, and POST so that SameSite=Lax withholds the cookie
// from a cross-site attempt. Signing out when already signed out succeeds.
func (s *Server) signOut(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := s.accounts.SignOut(r.Context(), c.Value); err != nil {
			// The cookie is deliberately not cleared: the token is still live
			// server-side, and clearing it would leave the player believing
			// they had signed out when they had not.
			s.log.Error("sign out", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorBody("the session could not be ended"))
			return
		}
	}

	http.SetCookie(w, s.expireCookie(sessionCookie))
	w.WriteHeader(http.StatusNoContent)
}

// cookie builds one of this server's cookies: httpOnly, Secure, and
// SameSite=Lax so it rides top-level navigations but is withheld from
// cross-site POSTs.
func (s *Server) cookie(name, value string, maxAge time.Duration) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies(),
		SameSite: http.SameSiteLaxMode,
	}
}

// expireCookie is the same cookie, told to go.
//
// The attributes have to match the ones it was set with or the browser treats
// it as a different cookie and keeps the original.
func (s *Server) expireCookie(name string) *http.Cookie {
	c := s.cookie(name, "", 0)
	c.MaxAge = -1
	c.Expires = time.Unix(0, 0)
	return c
}
