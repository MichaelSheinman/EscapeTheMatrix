package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// stand is a provider that answers the two calls the exchange makes. It records
// what it was sent, because half of what is worth asserting here is what leaves
// this process rather than what comes back.
type stand struct {
	url  string
	form url.Values
	auth string

	tokenStatus int
	tokenBody   string
	userinfo    string
}

func newStand(t *testing.T) *stand {
	t.Helper()
	s := &stand{
		tokenStatus: http.StatusOK,
		tokenBody:   `{"access_token":"at-1","token_type":"Bearer","expires_in":3599}`,
		userinfo: `{"sub":"1234","email":"player@example.test","email_verified":true,
		            "name":"A Player","picture":"https://example.test/p.png"}`,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("token request is not a form: %v", err)
		}
		s.form = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(s.tokenStatus)
		_, _ = w.Write([]byte(s.tokenBody))
	})
	mux.HandleFunc("GET /userinfo", func(w http.ResponseWriter, r *http.Request) {
		s.auth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(s.userinfo))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	s.url = srv.URL
	return s
}

func (s *stand) google() *Google {
	return &Google{
		ClientID:         "client-1",
		ClientSecret:     "secret-1",
		RedirectURL:      "https://etm.test/api/auth/google/callback",
		AuthEndpoint:     s.url + "/authorize",
		TokenEndpoint:    s.url + "/token",
		UserinfoEndpoint: s.url + "/userinfo",
	}
}

func TestAuthCodeURLCarriesTheFlow(t *testing.T) {
	t.Parallel()
	g := (&Google{
		ClientID:    "client-1",
		RedirectURL: "https://etm.test/api/auth/google/callback",
	})
	flow := Flow{State: "state-1", Verifier: "verifier-1"}

	u, err := url.Parse(g.AuthCodeURL(flow))
	if err != nil {
		t.Fatalf("AuthCodeURL() is not a URL: %v", err)
	}
	q := u.Query()

	for _, c := range []struct{ key, want string }{
		{"client_id", "client-1"},
		{"redirect_uri", "https://etm.test/api/auth/google/callback"},
		{"response_type", "code"},
		{"state", "state-1"},
		{"code_challenge_method", "S256"},
	} {
		if got := q.Get(c.key); got != c.want {
			t.Errorf("%s = %q, want %q", c.key, got, c.want)
		}
	}
	if !strings.Contains(q.Get("scope"), "openid") {
		t.Errorf("scope = %q, want openid among them", q.Get("scope"))
	}
}

// The whole point of PKCE: what goes out over the redirect must not be enough
// to complete the exchange.
func TestTheVerifierNeverLeavesInTheRedirect(t *testing.T) {
	t.Parallel()
	flow := Flow{State: "state-1", Verifier: "verifier-1"}

	raw := (&Google{ClientID: "c"}).AuthCodeURL(flow)
	if strings.Contains(raw, flow.Verifier) {
		t.Fatal("the authorize URL carries the verifier itself")
	}

	sum := sha256.Sum256([]byte(flow.Verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	u, _ := url.Parse(raw)
	if got := u.Query().Get("code_challenge"); got != want {
		t.Errorf("code_challenge = %q, want the S256 hash %q", got, want)
	}
}

func TestNewFlowDrawsDistinctSecrets(t *testing.T) {
	t.Parallel()

	a, err := NewFlow()
	if err != nil {
		t.Fatalf("NewFlow() = %v", err)
	}
	b, err := NewFlow()
	if err != nil {
		t.Fatalf("NewFlow() = %v", err)
	}

	if a.State == a.Verifier {
		t.Error("state and verifier are the same value")
	}
	if a.State == b.State || a.Verifier == b.Verifier {
		t.Error("two flows drew the same secret")
	}
	if len(a.State) < 40 {
		t.Errorf("state is %d characters, want 256 bits' worth", len(a.State))
	}
}

func TestIdentityRedeemsTheCode(t *testing.T) {
	t.Parallel()
	s := newStand(t)

	id, err := s.google().Identity(t.Context(), "code-1", Flow{State: "state-1", Verifier: "verifier-1"})
	if err != nil {
		t.Fatalf("Identity() = %v", err)
	}

	want := Identity{
		Provider:  "google",
		Subject:   "1234",
		Email:     "player@example.test",
		Name:      "A Player",
		AvatarURL: "https://example.test/p.png",
	}
	if id != want {
		t.Errorf("Identity() = %+v, want %+v", id, want)
	}

	// The exchange has to prove it is the same party that started the flow —
	// the client secret and the PKCE verifier are both that proof.
	for _, c := range []struct{ key, want string }{
		{"code", "code-1"},
		{"code_verifier", "verifier-1"},
		{"client_secret", "secret-1"},
		{"grant_type", "authorization_code"},
		{"redirect_uri", "https://etm.test/api/auth/google/callback"},
	} {
		if got := s.form.Get(c.key); got != c.want {
			t.Errorf("token request %s = %q, want %q", c.key, got, c.want)
		}
	}
	if s.auth != "Bearer at-1" {
		t.Errorf("userinfo Authorization = %q, want the access token just issued", s.auth)
	}
}

// A code that was already redeemed, or has expired, comes back as a 400 with a
// reason. It must not read as a successful sign-in as nobody.
func TestRefusedCodeIsAnError(t *testing.T) {
	t.Parallel()
	s := newStand(t)
	s.tokenStatus = http.StatusBadRequest
	s.tokenBody = `{"error":"invalid_grant","error_description":"Bad Request"}`

	_, err := s.google().Identity(t.Context(), "code-1", Flow{Verifier: "verifier-1"})
	if err == nil {
		t.Fatal("Identity() = nil error for a refused code")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("error %q does not say why the provider refused", err)
	}
}

// Without a subject there is nothing to key an account on, so an identity
// cannot be built and must not be half-built.
func TestUserinfoWithoutASubjectIsAnError(t *testing.T) {
	t.Parallel()
	s := newStand(t)
	s.userinfo = `{"email":"player@example.test"}`

	if _, err := s.google().Identity(t.Context(), "code-1", Flow{Verifier: "v"}); err == nil {
		t.Fatal("Identity() = nil error for a response with no subject")
	}
}

// A Google account can have no name set, and the player is about to see this
// next to their own face.
func TestMissingNameFallsBack(t *testing.T) {
	t.Parallel()
	s := newStand(t)
	s.userinfo = `{"sub":"1234","email":"player@example.test"}`

	id, err := s.google().Identity(t.Context(), "code-1", Flow{Verifier: "v"})
	if err != nil {
		t.Fatalf("Identity() = %v", err)
	}
	if id.Name == "" {
		t.Error("Name is empty, want a fallback the player can be shown as")
	}
}
