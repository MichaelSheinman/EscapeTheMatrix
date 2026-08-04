package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/account"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/oauth"
)

// The auth routes. They sit under /api/ with everything else the server owns,
// so the SPA fallback in assets.go can never shadow one by answering a typo
// with the app shell.
const (
	googleStartPath    = "/api/auth/google/start"
	googleCallbackPath = "/api/auth/google/callback"
	sessionPath        = "/api/auth/session"
	signOutPath        = "/api/auth/signout"
)

// The two cookies.
//
// sessionCookie is the only long-lived one and carries a bearer token: whoever
// holds it is the player. signInCookie exists for the seconds between leaving
// for Google and coming back, and holds the state and PKCE verifier for that
// one attempt.
const (
	sessionCookie = "etm_session"
	signInCookie  = "etm_signin"
)

// signInWindow is how long a sign-in attempt may stay open. Long enough to pick
// an account and type a password, short enough that an abandoned attempt is not
// still redeemable tomorrow.
const signInWindow = 10 * time.Minute

// actorKey is the context key the middleware stores the signed-in account
// under. Unexported and of a unique type, so nothing outside this package can
// put an actor on a request.
type actorKey struct{}

// actorFrom returns the account the middleware resolved for this request.
func actorFrom(ctx context.Context) (account.Account, bool) {
	a, ok := ctx.Value(actorKey{}).(account.Account)
	return a, ok
}

// authenticated is the auth check.
//
// technical.md §8 names this as one of the things commands over REST buy:
// **auth as one middleware**, not a per-handler check. It wraps the single
// command route, so a command cannot be written that forgets it, and a handler
// has no way to run for a player who is not signed in. There is no anonymous
// mode to fall through to (§9) — a request without a live session is refused,
// not downgraded.
func (s *Server) authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, err := s.resolveSession(r)
		switch {
		case errors.Is(err, account.ErrNoSession):
			// The browser is holding a token that stands for nothing — expired,
			// or signed out on another device. Clearing it stops every
			// subsequent request carrying a credential that cannot work.
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

// startGoogleSignIn sends the browser to Google.
//
// The state and PKCE verifier for this attempt go into a short-lived cookie
// rather than a table: they are worth exactly one callback, and a row per
// abandoned sign-in is a table to sweep for no benefit. The cookie is httpOnly,
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

// completeGoogleSignIn is where Google sends the browser back.
//
// Everything it can refuse, it refuses by sending the player to the app with a
// reason in the query string. This is a top-level navigation, not a fetch — a
// JSON error body would render as a page of text, and the app is the only place
// that can say "that did not work" in a way a person can act on.
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
	// a sign-in *this* browser started. Without it, anyone can hand a victim's
	// browser a callback URL carrying their own authorization code and have the
	// victim signed in as them — after which everything the victim does happens
	// in the attacker's account.
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

	_, token, err := s.accounts.SignIn(r.Context(), identity)
	if err != nil {
		s.log.Error("open session", "error", err)
		s.failSignIn(w, r, "failed")
		return
	}

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

// currentSession is how the app learns who it is serving, on every load.
//
// It is a GET rather than a command because it changes nothing: a command
// carries an idempotency key and records its answer, and a page refresh has no
// business writing a row. Being signed out is a legitimate answer, not a
// failure, so it is a 200 with no player rather than a 401.
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

// signOut invalidates the session server-side and clears the cookie.
//
// It is a route rather than a command for the same reason sign-in is: it is a
// cookie operation, and a command handler never sees an http.ResponseWriter. It
// is POST so that SameSite=Lax withholds the cookie from a cross-site attempt —
// a link that signs a player out is a nuisance worth closing off.
//
// Signing out when already signed out succeeds. There is no state in which the
// caller should be told the difference.
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

// cookie builds one of this server's cookies.
//
// §9 fixes the attributes: httpOnly so no script can read the token, even one
// that got onto the page; Secure so it never crosses the network in the clear;
// SameSite=Lax so it rides top-level navigations — which is what brings a
// player back from Google — but is withheld from cross-site POSTs, which is
// what makes every command CSRF-safe without a token of its own.
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
