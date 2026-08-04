// Package config parses the process environment once, at boot, into a typed
// struct — and fails before the server binds a port if anything is missing.
//
// The rule from technical.md §13 is "fail fast on missing values". The subtlety
// worth preserving: it reports *every* problem in one pass rather than dying on
// the first one. Booting a container four times to discover four missing
// variables is the failure mode this avoids.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Env is the deployment environment. It decides the log format (§13) and
// nothing else yet; later tickets hang dev-only behaviour off it, such as
// content hot-reload (§5.2).
type Env string

const (
	Development Env = "development"
	Production  Env = "production"
)

// Config is the whole of what the process reads from its environment.
// Every field is populated exactly once, in Load, and never re-read.
type Config struct {
	Env         Env
	Addr        string
	DatabaseURL string

	// PublicURL is the origin a browser reaches this server on, without a
	// trailing slash — "https://escapethematrix.app".
	//
	// It is configuration rather than something derived from the inbound
	// request because the OAuth redirect URI is built from it, and a Host
	// header is attacker-controlled: deriving it would let a request with a
	// forged Host send a player's sign-in somewhere else.
	PublicURL string

	// Google is the OAuth client this server signs in with (§9).
	Google OAuthClient
}

// OAuthClient is one provider's credentials.
type OAuthClient struct {
	ID     string
	Secret string
}

// IsProduction reports whether logs should be JSON and dev affordances off.
func (c Config) IsProduction() bool { return c.Env == Production }

// SecureCookies reports whether cookies must carry the Secure attribute.
//
// It follows the public URL's scheme rather than the environment, because
// Secure is a statement about the transport and a browser silently drops a
// Secure cookie sent over plain http — which would make local development
// impossible to sign in to. Load refuses to start a production process on
// anything but https, so in production this is always true.
func (c Config) SecureCookies() bool { return strings.HasPrefix(c.PublicURL, "https://") }

// loader accumulates problems instead of returning on the first one, so a
// misconfigured deploy reports its full story in a single line of output.
type loader struct{ problems []string }

func (l *loader) problem(format string, args ...any) {
	l.problems = append(l.problems, fmt.Sprintf(format, args...))
}

// required reads a variable that has no sensible default.
func (l *loader) required(key string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		l.problem("%s is required and was empty", key)
	}
	return v
}

// optional reads a variable, falling back to def when unset.
func (l *loader) optional(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// enum reads a variable constrained to a fixed set of values.
func (l *loader) enum(key, def string, allowed ...string) string {
	v := l.optional(key, def)
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	l.problem("%s must be one of %s, got %q", key, strings.Join(allowed, ", "), v)
	return def
}

// origin reads a variable that has to be an absolute http(s) origin, and
// normalises away a trailing slash so that paths appended to it never double up.
func (l *loader) origin(key, def string) string {
	raw := l.optional(key, def)

	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		l.problem("%s must be an absolute http(s) URL, got %q", key, raw)
		return ""
	}
	return u.Scheme + "://" + u.Host + strings.TrimSuffix(u.Path, "/")
}

// Load reads the environment into a Config, or returns an error naming every
// variable that is wrong. Call it once, in main, before anything else.
func Load() (Config, error) {
	var l loader

	cfg := Config{
		Env:         Env(l.enum("ETM_ENV", string(Development), string(Development), string(Production))),
		Addr:        l.optional("ETM_ADDR", ":8080"),
		DatabaseURL: l.required("ETM_DATABASE_URL"),
		PublicURL:   l.origin("ETM_PUBLIC_URL", "http://localhost:8080"),
		Google: OAuthClient{
			// Required, with no fallback and no anonymous mode to fall back
			// *to*: §9 makes an account the price of entry, so a process that
			// cannot sign anybody in has nothing to serve and should say so at
			// boot rather than at the first player.
			ID:     l.required("ETM_GOOGLE_CLIENT_ID"),
			Secret: l.required("ETM_GOOGLE_CLIENT_SECRET"),
		},
	}

	// The session cookie is Secure whenever the site is https, and this is what
	// makes that "always" in production. Plain http in production would send
	// every session token in the clear.
	if cfg.IsProduction() && !cfg.SecureCookies() {
		l.problem("ETM_PUBLIC_URL must be https in production, got %q", cfg.PublicURL)
	}

	if len(l.problems) > 0 {
		return Config{}, fmt.Errorf("invalid configuration:\n  - %s", strings.Join(l.problems, "\n  - "))
	}
	return cfg, nil
}
