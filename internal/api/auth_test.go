package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// Sign-in over the router it actually ships behind: httptest against the real
// routes, a real Postgres underneath, and a stand-in for Google in front. What
// is being proved is the whole path — a redirect out, a callback back, a cookie
// that a later request is recognised by, and a sign-out that kills it in the
// database rather than only in the browser.

// fakeGoogle answers the two calls the exchange makes.
//
// It identifies the player by the authorization code, so a test says who is
// signing in by naming a code.
func fakeGoogle(t *testing.T) string {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		if r.PostForm.Get("code") == "" || r.PostForm.Get("code_verifier") == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"for-` + r.PostForm.Get("code") + `","token_type":"Bearer"}`))
	})
	mux.HandleFunc("GET /userinfo", func(w http.ResponseWriter, r *http.Request) {
		who := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer for-")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sub":"` + who + `","email":"` + who + `@example.test",` +
			`"name":"Player ` + who + `","picture":"https://example.test/` + who + `.png"}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// signIn walks a player through the whole flow and returns their session
// cookie. subject becomes their identity at the provider.
func signIn(t *testing.T, s *Server, subject string) *http.Cookie {
	t.Helper()

	// The app sends the browser to Google, and keeps this attempt's state and
	// verifier in a cookie while it is away.
	start := httptest.NewRecorder()
	s.ServeHTTP(start, httptest.NewRequest(http.MethodGet, googleStartPath, nil))
	if start.Code != http.StatusFound {
		t.Fatalf("sign-in start = %d, want a redirect to the provider", start.Code)
	}
	flow := cookie(start.Result().Cookies(), signInCookie)
	if flow == nil {
		t.Fatal("sign-in start set no cookie, so the callback has nothing to check against")
	}

	authURL, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatalf("Location is not a URL: %v", err)
	}

	// Google sends the browser back with a code and the state it was given.
	back := httptest.NewRecorder()
	s.ServeHTTP(back, callback(subject, authURL.Query().Get("state"), flow))
	if back.Code != http.StatusSeeOther {
		t.Fatalf("callback = %d (%s), want a redirect into the app", back.Code, back.Body)
	}

	session := cookie(back.Result().Cookies(), sessionCookie)
	if session == nil {
		t.Fatal("the callback opened no session")
	}
	return session
}

// callback builds the request Google's redirect produces.
func callback(code, state string, with *http.Cookie) *http.Request {
	q := url.Values{"code": {code}, "state": {state}}
	r := httptest.NewRequest(http.MethodGet, googleCallbackPath+"?"+q.Encode(), nil)
	if with != nil {
		r.AddCookie(with)
	}
	return r
}

// cookie picks one out of a response, ignoring the ones being expired.
func cookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name && c.MaxAge >= 0 && c.Value != "" {
			return c
		}
	}
	return nil
}

func currentPlayer(t *testing.T, s *Server, as *http.Cookie) (name string, signedIn bool) {
	t.Helper()

	r := httptest.NewRequest(http.MethodGet, sessionPath, nil)
	if as != nil {
		r.AddCookie(as)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("%s = %d (%s), want 200", sessionPath, rec.Code, rec.Body)
	}
	var body struct {
		Player *struct {
			ID, Name, Email, AvatarURL string
		} `json:"player"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("session response is not the documented shape: %v (%s)", err, rec.Body)
	}
	if body.Player == nil {
		return "", false
	}
	return body.Player.Name, true
}

// The acceptance criterion end to end: sign in with Google, land as a known
// player, and still be that player on the next request — which is what a page
// reload is once the cookie is doing the remembering.
func TestSignInSurvivesAReload(t *testing.T) {
	t.Parallel()
	s, _ := newServer(t)

	as := signIn(t, s, "abc")

	name, signedIn := currentPlayer(t, s, as)
	if !signedIn {
		t.Fatal("the session endpoint reports nobody signed in")
	}
	if name != "Player abc" {
		t.Errorf("player = %q, want the identity Google gave", name)
	}

	// A second, entirely independent request carrying only the cookie.
	if _, signedIn := currentPlayer(t, s, as); !signedIn {
		t.Error("the identity did not survive a second request")
	}
}

func TestSessionEndpointReportsSignedOut(t *testing.T) {
	t.Parallel()
	s, _ := newServer(t)

	if _, signedIn := currentPlayer(t, s, nil); signedIn {
		t.Error("a request with no cookie reports a signed-in player")
	}
}

// §9: no anonymous play. The middleware is the door, and it is on the one
// command route rather than inside each handler.
func TestCommandsRequireASession(t *testing.T) {
	t.Parallel()
	s, st := newServer(t)

	rec := post(t, s, nil, "probe", "key-1", `{"label":"first"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d (%s), want 401", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}
	// The refusal has to happen before the handler, not inside it.
	if got := probeCount(t, st); got != 0 {
		t.Errorf("%d rows in skeleton_probe, want 0 — the command ran anyway", got)
	}
}

// A cookie that no longer stands for anything is cleared on the way out, so the
// browser stops presenting a credential that cannot work.
func TestARevokedCookieIsRefusedAndCleared(t *testing.T) {
	t.Parallel()
	s, _ := newServer(t)
	as := signIn(t, s, "abc")

	signOut(t, s, as)

	rec := post(t, s, as, "probe", "key-1", `{}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for a signed-out cookie", rec.Code)
	}
	if cleared := expired(rec.Result().Cookies(), sessionCookie); !cleared {
		t.Error("the dead cookie was left in the browser")
	}
}

// Signing out has to kill the session in Postgres, not just drop the cookie:
// the same cookie value replayed afterwards must be worth nothing.
func TestSignOutInvalidatesTheSessionServerSide(t *testing.T) {
	t.Parallel()
	s, st := newServer(t)
	as := signIn(t, s, "abc")

	signOut(t, s, as)

	if _, signedIn := currentPlayer(t, s, as); signedIn {
		t.Error("the session survived signing out")
	}
	var revoked int
	if err := st.Pool().QueryRow(t.Context(),
		`SELECT count(*) FROM session WHERE revoked_at IS NOT NULL`).Scan(&revoked); err != nil {
		t.Fatalf("count revoked sessions: %v", err)
	}
	if revoked != 1 {
		t.Errorf("%d revoked sessions in the table, want 1", revoked)
	}
}

func signOut(t *testing.T, s *Server, as *http.Cookie) {
	t.Helper()

	r := httptest.NewRequest(http.MethodPost, signOutPath, nil)
	r.AddCookie(as)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, r)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("sign out = %d (%s), want 204", rec.Code, rec.Body)
	}
	if !expired(rec.Result().Cookies(), sessionCookie) {
		t.Error("signing out left the cookie in the browser")
	}
}

func expired(cookies []*http.Cookie, name string) bool {
	for _, c := range cookies {
		if c.Name == name && c.MaxAge < 0 {
			return true
		}
	}
	return false
}

func TestStartRedirectsToTheProviderWithStateAndPKCE(t *testing.T) {
	t.Parallel()
	s, _ := newServer(t)

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, googleStartPath, nil))

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	sent, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("Location is not a URL: %v", err)
	}
	q := sent.Query()
	if q.Get("state") == "" || q.Get("code_challenge") == "" {
		t.Errorf("authorize URL is missing state or PKCE: %s", sent)
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Errorf("code_challenge_method = %q, want S256", q.Get("code_challenge_method"))
	}

	// The state the provider is given has to be checkable when the browser
	// comes back, and the only thing carrying it is this cookie.
	flow := cookie(rec.Result().Cookies(), signInCookie)
	if flow == nil {
		t.Fatal("no sign-in cookie was set")
	}
	if !flow.HttpOnly {
		t.Error("the sign-in cookie is readable by scripts")
	}
	if !strings.HasPrefix(flow.Value, q.Get("state")+":") {
		t.Error("the cookie does not hold the state that was sent")
	}
	if strings.Contains(sent.String(), strings.SplitN(flow.Value, ":", 2)[1]) {
		t.Error("the PKCE verifier was sent to the provider")
	}
}

// Without the state check, anyone can hand a victim's browser a callback
// carrying their own code and have the victim signed in as them.
func TestCallbackRefusesAForeignState(t *testing.T) {
	t.Parallel()
	s, _ := newServer(t)

	start := httptest.NewRecorder()
	s.ServeHTTP(start, httptest.NewRequest(http.MethodGet, googleStartPath, nil))
	flow := cookie(start.Result().Cookies(), signInCookie)

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, callback("abc", "some-other-state", flow))

	if got := cookie(rec.Result().Cookies(), sessionCookie); got != nil {
		t.Fatal("a callback with a mismatched state opened a session")
	}
	if !strings.Contains(rec.Header().Get("Location"), "signin=") {
		t.Errorf("Location = %q, want the app told why", rec.Header().Get("Location"))
	}
}

// No cookie means the attempt expired, or was never started here.
func TestCallbackWithoutItsCookieIsRefused(t *testing.T) {
	t.Parallel()
	s, _ := newServer(t)

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, callback("abc", "any-state", nil))

	if got := cookie(rec.Result().Cookies(), sessionCookie); got != nil {
		t.Fatal("a callback with no sign-in cookie opened a session")
	}
}

// The attempt is worth one callback. Replaying it must not work, or an
// authorization code lingering in a history entry stays redeemable.
func TestTheSignInCookieIsSpentByTheCallback(t *testing.T) {
	t.Parallel()
	s, _ := newServer(t)

	start := httptest.NewRecorder()
	s.ServeHTTP(start, httptest.NewRequest(http.MethodGet, googleStartPath, nil))
	flow := cookie(start.Result().Cookies(), signInCookie)
	state, _, _ := strings.Cut(flow.Value, ":")

	first := httptest.NewRecorder()
	s.ServeHTTP(first, callback("abc", state, flow))
	if cookie(first.Result().Cookies(), sessionCookie) == nil {
		t.Fatal("the first callback did not sign in")
	}
	if !expired(first.Result().Cookies(), signInCookie) {
		t.Error("the sign-in cookie outlived the callback that used it")
	}
}

// §9 fixes the attributes, and each one is load-bearing: httpOnly keeps the
// token away from any script that reaches the page, SameSite=Lax is what makes
// every command CSRF-safe without a token of its own, and Secure follows the
// scheme so it is on wherever the site is https.
func TestSessionCookieAttributes(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		publicURL  string
		wantSecure bool
	}{
		{"http://localhost:8080", false},
		{"https://etm.example", true},
	} {
		s := serverOn(t, c.publicURL)
		got := signIn(t, s, "abc")

		if !got.HttpOnly {
			t.Errorf("%s: the session cookie is readable by scripts", c.publicURL)
		}
		if got.SameSite != http.SameSiteLaxMode {
			t.Errorf("%s: SameSite = %v, want Lax", c.publicURL, got.SameSite)
		}
		if got.Secure != c.wantSecure {
			t.Errorf("%s: Secure = %v, want %v", c.publicURL, got.Secure, c.wantSecure)
		}
		if got.Path != "/" {
			t.Errorf("%s: Path = %q, want /", c.publicURL, got.Path)
		}
	}
}

// The idempotency record holds a response body, so a key is scoped to the
// player. Two players choosing the same key are two commands, not a duplicate —
// and until sign-in existed there was one actor for whom this was untestable.
func TestIdempotencyKeysAreScopedToThePlayer(t *testing.T) {
	t.Parallel()
	s, st := newServer(t)

	abc := signIn(t, s, "abc")
	xyz := signIn(t, s, "xyz")

	first := post(t, s, abc, "probe", "shared-key", `{"label":"one"}`)
	second := post(t, s, xyz, "probe", "shared-key", `{"label":"two"}`)

	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("statuses = %d, %d; want 200, 200 (%s / %s)", first.Code, second.Code, first.Body, second.Body)
	}
	if second.Header().Get(replayHeader) == "true" {
		t.Error("one player's command was answered from another player's record")
	}
	if got := probeCount(t, st); got != 2 {
		t.Errorf("%d rows in skeleton_probe, want 2 — one per player", got)
	}
}

// The actor a command runs as is the account, not a name the client chose.
func TestCommandsRunAsTheSignedInAccount(t *testing.T) {
	t.Parallel()
	s, st := newServer(t)
	as := signIn(t, s, "abc")

	post(t, s, as, "probe", "key-1", `{}`)

	var actor, accountID string
	if err := st.Pool().QueryRow(t.Context(),
		`SELECT actor FROM command_idempotency`).Scan(&actor); err != nil {
		t.Fatalf("read the idempotency record: %v", err)
	}
	if err := st.Pool().QueryRow(t.Context(),
		`SELECT id::text FROM account WHERE subject = 'abc'`).Scan(&accountID); err != nil {
		t.Fatalf("read the account: %v", err)
	}
	if actor != accountID {
		t.Errorf("the command ran as %q, want the account %q", actor, accountID)
	}
}

func TestExpiredSessionIsRefused(t *testing.T) {
	t.Parallel()
	s, st := newServer(t)
	as := signIn(t, s, "abc")

	if _, err := st.Pool().Exec(t.Context(),
		`UPDATE session SET expires_at = now() - interval '1 second'`); err != nil {
		t.Fatalf("age the session: %v", err)
	}

	if rec := post(t, s, as, "probe", "key-1", `{}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for an expired session", rec.Code)
	}
}
