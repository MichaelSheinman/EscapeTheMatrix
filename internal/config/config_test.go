package config

import (
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("ETM_DATABASE_URL", "postgres://etm:etm@localhost:5432/etm")

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
}

// The failure mode this guards is booting a container once per missing
// variable. One pass has to name every problem.
func TestLoadReportsEveryProblemAtOnce(t *testing.T) {
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
	// Whitespace is not a value. A quoted empty string in a compose file or a
	// secrets manager arrives looking exactly like this.
	t.Setenv("ETM_DATABASE_URL", "   ")

	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil, want error for whitespace-only value")
	}
}

func TestLoadProduction(t *testing.T) {
	t.Setenv("ETM_ENV", "production")
	t.Setenv("ETM_DATABASE_URL", "postgres://etm@db/etm")
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
}
