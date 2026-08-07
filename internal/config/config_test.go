package config

import (
	"strings"
	"testing"
)

// setRequired supplies the variables that have no default, so each test can
// vary the one thing it is about.
func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("ETM_DATABASE_URL", "postgres://etm:etm@localhost:5432/etm")
	t.Setenv("ETM_GOOGLE_CLIENT_ID", "client.apps.googleusercontent.com")
	t.Setenv("ETM_GOOGLE_CLIENT_SECRET", "secret")
}

func TestLoadDefaults(t *testing.T) {
	setRequired(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if cfg.Env != Development {
		t.Errorf("Env = %q, want %q", cfg.Env, Development)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", cfg.Addr)
	}
	if cfg.IsProduction() {
		t.Error("IsProduction() = true, want false")
	}
	if cfg.PublicURL != "http://localhost:8080" {
		t.Errorf("PublicURL = %q, want the local default", cfg.PublicURL)
	}
	// Plain http in development: a Secure cookie over http is dropped by the
	// browser, which would make local sign-in impossible.
	if cfg.SecureCookies() {
		t.Error("SecureCookies() = true over plain http")
	}
}

// A trailing slash would produce "http://host//api/auth/..." as the redirect
// URI, which no longer matches what is registered with the provider.
func TestPublicURLIsNormalised(t *testing.T) {
	setRequired(t)
	t.Setenv("ETM_PUBLIC_URL", "https://etm.example/")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if cfg.PublicURL != "https://etm.example" {
		t.Errorf("PublicURL = %q, want the trailing slash gone", cfg.PublicURL)
	}
	if !cfg.SecureCookies() {
		t.Error("SecureCookies() = false over https")
	}
}

func TestPublicURLMustBeAbsolute(t *testing.T) {
	setRequired(t)
	t.Setenv("ETM_PUBLIC_URL", "localhost:8080")

	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil, want error for a URL with no scheme")
	}
}

// Sign-in is the price of entry (§9), so a process that cannot sign anybody in
// has nothing to serve and must say so at boot.
func TestGoogleCredentialsAreRequired(t *testing.T) {
	t.Setenv("ETM_DATABASE_URL", "postgres://etm:etm@localhost:5432/etm")
	// Explicitly emptied rather than assumed absent: `make check` exports
	// .env, so on a developer's own machine these are set, and a test that only
	// passes where the file is missing is a test that passes in CI and nowhere
	// else.
	t.Setenv("ETM_GOOGLE_CLIENT_ID", "")
	t.Setenv("ETM_GOOGLE_CLIENT_SECRET", "")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() = nil, want error")
	}
	for _, want := range []string{"ETM_GOOGLE_CLIENT_ID", "ETM_GOOGLE_CLIENT_SECRET"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

// Plain http in production would put every session token on the wire in the
// clear, because the cookie's Secure attribute follows the scheme.
func TestProductionRefusesPlainHTTP(t *testing.T) {
	setRequired(t)
	t.Setenv("ETM_ENV", "production")
	t.Setenv("ETM_PUBLIC_URL", "http://etm.example")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() = nil, want error")
	}
	if !strings.Contains(err.Error(), "ETM_PUBLIC_URL") {
		t.Errorf("error %q does not name the variable at fault", err)
	}
}

// The failure mode this guards is booting a container once per missing
// variable. One pass has to name every problem.
func TestLoadReportsEveryProblemAtOnce(t *testing.T) {
	setRequired(t)
	t.Setenv("ETM_ENV", "staging")
	t.Setenv("ETM_DATABASE_URL", "")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() = nil, want error")
	}
	for _, want := range []string{"ETM_ENV", "ETM_DATABASE_URL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestLoadRejectsBlankRequiredValue(t *testing.T) {
	setRequired(t)
	// Whitespace is not a value. A quoted empty string in a compose file or a
	// secrets manager arrives looking exactly like this.
	t.Setenv("ETM_DATABASE_URL", "   ")

	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil, want error for whitespace-only value")
	}
}

func TestLoadProduction(t *testing.T) {
	setRequired(t)
	t.Setenv("ETM_ENV", "production")
	t.Setenv("ETM_DATABASE_URL", "postgres://etm@db/etm")
	t.Setenv("ETM_PUBLIC_URL", "https://etm.example")
	t.Setenv("ETM_ADDR", ":9000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if !cfg.IsProduction() {
		t.Error("IsProduction() = false, want true")
	}
	if cfg.Addr != ":9000" {
		t.Errorf("Addr = %q, want :9000", cfg.Addr)
	}
	if !cfg.SecureCookies() {
		t.Error("SecureCookies() = false in production")
	}
}
