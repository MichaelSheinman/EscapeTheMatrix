package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
)

// These need no database: registration is a boot-time concern, and its failures
// are programming errors that must surface as a crash rather than as a route
// that quietly never matches.

func noop(context.Context, pgx.Tx, command.Actor, struct{}) (struct{}, error) {
	return struct{}{}, nil
}

func mustPanic(t *testing.T, why string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("no panic: %s", why)
		}
	}()
	fn()
}

func TestRegisterRejectsADuplicateName(t *testing.T) {
	t.Parallel()
	reg := command.NewRegistry()
	command.Register(reg, "probe", noop)

	mustPanic(t, "a second handler silently replaced the first", func() {
		command.Register(reg, "probe", noop)
	})
}

// The name is a URL segment, a log field, and a metric label. A name with a
// slash in it would register a route that can never be reached.
func TestRegisterRejectsUnusableNames(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"", "Probe", "challenge/advance", ".probe", "probe.", "probe..advance", "9lives", "probe advance"} {
		mustPanic(t, "accepted the name "+name, func() {
			command.Register(command.NewRegistry(), name, noop)
		})
	}
}

func TestRegisterAcceptsDottedNames(t *testing.T) {
	t.Parallel()
	reg := command.NewRegistry()

	for _, name := range []string{"probe", "challenge.advance", "playbook.rule2.enable"} {
		command.Register(reg, name, noop)
	}
	if got := len(reg.Names()); got != 3 {
		t.Errorf("registry holds %d commands (%s), want 3", got, strings.Join(reg.Names(), ", "))
	}
}
