package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Google's endpoints. Fields on the client rather than constants so a test can
// point the exchange at an httptest server; nothing else ever sets them.
const (
	googleAuthEndpoint     = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenEndpoint    = "https://oauth2.googleapis.com/token"
	googleUserinfoEndpoint = "https://openidconnect.googleapis.com/v1/userinfo"
)

// googleScopes asks for the least that identifies a player: a stable subject,
// an email, and a name and picture to show them. No Gmail, no Drive, no
// long-lived access — the token is used once, here, and then discarded.
const googleScopes = "openid email profile"

// maxUserinfoBody bounds what the provider is allowed to say. It is a handful
// of short strings; anything larger means the response is not what it claims.
const maxUserinfoBody = 64 << 10

// Google signs a player in with Google.
type Google struct {
	ClientID     string
	ClientSecret string
	// RedirectURL is where the provider sends the browser back. It has to match
	// what is registered in the Google Cloud console byte for byte, which is
	// why it is built from configuration rather than from the inbound request:
	// a Host header is attacker-controlled and would make it a moving target.
	RedirectURL string

	// HTTP is the client used to talk to Google. Zero value means a default
	// client with a short timeout.
	HTTP *http.Client

	// Endpoints, overridable so tests can stand in for Google. Empty means the
	// real ones.
	AuthEndpoint     string
	TokenEndpoint    string
	UserinfoEndpoint string
}

// AuthCodeURL is where the browser is sent to sign in.
func (g *Google) AuthCodeURL(f Flow) string {
	q := url.Values{
		"client_id":             {g.ClientID},
		"redirect_uri":          {g.RedirectURL},
		"response_type":         {"code"},
		"scope":                 {googleScopes},
		"state":                 {f.State},
		"code_challenge":        {challenge(f.Verifier)},
		"code_challenge_method": {"S256"},
		// Nothing here is done on the player's behalf while they are away, so
		// there is no refresh token to want and none is asked for.
		"access_type": {"online"},
		// A shared machine is the common case for a game. Landing on the
		// account chooser rather than being silently signed in as whoever used
		// the browser last is worth one extra click.
		"prompt": {"select_account"},
	}
	return endpoint(g.AuthEndpoint, googleAuthEndpoint) + "?" + q.Encode()
}

// Identity redeems an authorization code and returns who it belongs to.
//
// Two calls to Google, both over TLS from this server: the code is exchanged
// for an access token, and the token is spent on the one userinfo read. The
// exchange also returns an ID token, which this deliberately ignores — reading
// it would mean verifying a signature against a rotating key set to learn what
// the userinfo endpoint states directly.
func (g *Google) Identity(ctx context.Context, code string, f Flow) (Identity, error) {
	token, err := g.exchange(ctx, code, f)
	if err != nil {
		return Identity{}, err
	}
	return g.userinfo(ctx, token)
}

// exchange trades the authorization code for an access token.
func (g *Google) exchange(ctx context.Context, code string, f Flow) (string, error) {
	form := url.Values{
		"code":          {code},
		"client_id":     {g.ClientID},
		"client_secret": {g.ClientSecret},
		"redirect_uri":  {g.RedirectURL},
		"grant_type":    {"authorization_code"},
		"code_verifier": {f.Verifier},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		endpoint(g.TokenEndpoint, googleTokenEndpoint), strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("oauth: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	var body struct {
		AccessToken string `json:"access_token"`
		// Google reports a refused exchange as 400 with these fields. An
		// expired or already-redeemed code lands here, and the description is
		// the only thing that says which.
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if err := g.call(req, &body); err != nil {
		return "", err
	}
	if body.Error != "" {
		return "", fmt.Errorf("oauth: google refused the authorization code: %s (%s)", body.Error, body.Description)
	}
	if body.AccessToken == "" {
		return "", fmt.Errorf("oauth: google returned no access token")
	}
	return body.AccessToken, nil
}

// userinfo spends the access token on the one read it was obtained for.
func (g *Google) userinfo(ctx context.Context, token string) (Identity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		endpoint(g.UserinfoEndpoint, googleUserinfoEndpoint), nil)
	if err != nil {
		return Identity{}, fmt.Errorf("oauth: build userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	var body struct {
		Sub     string `json:"sub"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := g.call(req, &body); err != nil {
		return Identity{}, err
	}
	if body.Sub == "" {
		return Identity{}, fmt.Errorf("oauth: google returned no subject")
	}

	id := Identity{
		Provider:  "google",
		Subject:   body.Sub,
		Email:     body.Email,
		Name:      strings.TrimSpace(body.Name),
		AvatarURL: body.Picture,
	}
	// A Google account can have no name set. The player is about to see this
	// next to their own face, so it falls back to something rather than blank —
	// onboarding (§9) is where they choose what they are actually called.
	if id.Name == "" {
		id.Name = "Player"
	}
	return id, nil
}

// call performs a request and decodes its JSON body.
//
// A non-2xx from the token endpoint is still decoded, because that is how
// Google reports a refused code; every other failing status is an error whose
// body is not worth guessing at.
func (g *Google) call(req *http.Request, into any) error {
	res, err := g.client().Do(req)
	if err != nil {
		return fmt.Errorf("oauth: %s %s: %w", req.Method, req.URL.Host, err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, maxUserinfoBody))
	if err != nil {
		return fmt.Errorf("oauth: read response from %s: %w", req.URL.Host, err)
	}
	if res.StatusCode >= 500 || res.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("oauth: %s answered %d", req.URL.Host, res.StatusCode)
	}
	if err := json.Unmarshal(body, into); err != nil {
		return fmt.Errorf("oauth: %s answered %d with something other than JSON: %w",
			req.URL.Host, res.StatusCode, err)
	}
	return nil
}

// client returns the HTTP client to use.
//
// The timeout is the point: a provider that hangs must not hold a request
// goroutine — and the player staring at a spinner — open indefinitely.
func (g *Google) client() *http.Client {
	if g.HTTP != nil {
		return g.HTTP
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func endpoint(override, real string) string {
	if override != "" {
		return override
	}
	return real
}
