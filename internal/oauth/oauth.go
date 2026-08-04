// Package oauth turns a sign-in at an external provider into an Identity.
//
// technical.md §9: OAuth is the only way in — Google first, Discord alongside
// it later — and there is no auth SaaS, because one would break the one-binary,
// one-datastore property and is hard to remove once every session depends on
// it. What that costs is this package, and the bill is small: the
// authorization-code flow is a redirect out and a form POST back.
//
// Nothing here touches the database or a cookie. It answers one question —
// "who did the provider say this is" — and internal/account decides what that
// means, so a second provider is a second file here and no change anywhere
// else.
package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// Identity is a provider's answer about the person who just signed in.
//
// Subject is the only field that identifies them. The rest are labels the
// provider is free to change between one sign-in and the next, and which this
// application therefore treats as display data rather than as a key.
type Identity struct {
	// Provider names the source — "google". Part of the identity, because two
	// providers' subjects live in different namespaces and may collide.
	Provider string
	// Subject is the provider's permanent id for the account.
	Subject string

	Email     string
	Name      string
	AvatarURL string
}

// Flow is the per-sign-in secret pair, created before the redirect out and
// needed again when the browser comes back.
//
// State is the CSRF defence: it is handed to the provider, echoed back in the
// callback, and compared against a copy held in a cookie. Without it, anyone
// can send a victim's browser to the callback carrying an attacker's code and
// sign them into the attacker's account.
//
// Verifier is PKCE. The redirect out carries only its hash, so an authorization
// code intercepted in transit is useless without the original — which never
// leaves this server. PKCE is not strictly required of a confidential client
// like this one, but it costs two fields and removes a whole class of attack.
type Flow struct {
	State    string
	Verifier string
}

// NewFlow draws a fresh state and verifier.
func NewFlow() (Flow, error) {
	state, err := secret()
	if err != nil {
		return Flow{}, err
	}
	verifier, err := secret()
	if err != nil {
		return Flow{}, err
	}
	return Flow{State: state, Verifier: verifier}, nil
}

// challenge is the S256 PKCE challenge for a verifier: what goes to the
// provider, from which the verifier cannot be recovered.
func challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// secretBytes is the entropy behind every value this package generates. 256
// bits is far past what is guessable and costs 43 characters of URL.
const secretBytes = 32

// secret returns a URL-safe random string.
func secret() (string, error) {
	b := make([]byte, secretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("oauth: draw random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
