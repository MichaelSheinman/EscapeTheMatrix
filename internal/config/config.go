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
}

// IsProduction reports whether logs should be JSON and dev affordances off.
func (c Config) IsProduction() bool { return c.Env == Production }

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

// Load reads the environment into a Config, or returns an error naming every
// variable that is wrong. Call it once, in main, before anything else.
func Load() (Config, error) {
	var l loader

	cfg := Config{
		Env:         Env(l.enum("ETM_ENV", string(Development), string(Development), string(Production))),
		Addr:        l.optional("ETM_ADDR", ":8080"),
		DatabaseURL: l.required("ETM_DATABASE_URL"),
	}

	if len(l.problems) > 0 {
		return Config{}, fmt.Errorf("invalid configuration:\n  - %s", strings.Join(l.problems, "\n  - "))
	}
	return cfg, nil
}
