// Package storetest gives a test a real Postgres to run against.
//
// technical.md §14 puts domain modules and commands on integration tests
// against a real database rather than a mock, for the reason that the things
// worth testing here — one transaction per command, a unique index holding
// under concurrency, a rollback taking its idempotency claim with it — are
// exactly the behaviours a mock invents.
//
// Each call to New creates its own database, migrates it, and drops it when the
// test ends. Tests therefore share nothing and may run in parallel.
package storetest

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
)

// defaultURL matches compose.yaml, so `make db` is the whole of the setup.
const defaultURL = "postgres://etm:etm@localhost:5432/etm?sslmode=disable"

// scratch numbers the databases created by this process.
var scratch atomic.Int64

// New returns a Store on a freshly migrated, empty database.
//
// Without a reachable Postgres the test skips, so `go test ./...` still passes
// on a machine where Docker is not running — unless ETM_TEST_REQUIRE_POSTGRES
// is set, which turns the skip into a failure. CI sets it: a suite that quietly
// skips the test proving a command cannot double-apply is worse than no suite.
func New(t *testing.T) *store.Store {
	t.Helper()
	ctx := t.Context()

	base, err := url.Parse(baseURL())
	if err != nil {
		t.Fatalf("test database url is not a url: %v", err)
	}

	admin, err := pgx.Connect(ctx, withDatabase(base, "postgres"))
	if err != nil {
		if os.Getenv("ETM_TEST_REQUIRE_POSTGRES") != "" {
			t.Fatalf("postgres is required but unreachable: %v", err)
		}
		t.Skipf("postgres is not reachable (%v) — run `make db`", err)
	}
	defer admin.Close(ctx)

	name := scratchName(t)
	// Identifiers cannot be parameterised, which is why scratchName controls
	// every character that gets here.
	if _, err := admin.Exec(ctx, `CREATE DATABASE "`+name+`"`); err != nil {
		t.Fatalf("create test database %s: %v", name, err)
	}
	t.Cleanup(func() {
		// A separate connection: t.Context is already cancelled by now, and the
		// admin one above is closed.
		drop, err := pgx.Connect(context.Background(), withDatabase(base, "postgres"))
		if err != nil {
			t.Logf("could not drop test database %s: %v", name, err)
			return
		}
		defer drop.Close(context.Background())
		if _, err := drop.Exec(context.Background(), `DROP DATABASE "`+name+`" WITH (FORCE)`); err != nil {
			t.Logf("could not drop test database %s: %v", name, err)
		}
	})

	dsn := withDatabase(base, name)
	if _, err := store.Migrate(ctx, dsn, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}

	st, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(st.Close)
	return st
}

// baseURL says which Postgres to use. ETM_TEST_DATABASE_URL wins so that a
// developer can point tests at a server other than the one holding their
// development data; only its host and credentials are used, never its database.
func baseURL() string {
	for _, key := range []string{"ETM_TEST_DATABASE_URL", "ETM_DATABASE_URL"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return defaultURL
}

func withDatabase(base *url.URL, name string) string {
	u := *base
	u.Path = "/" + name
	return u.String()
}

// scratchName builds a database name from the test's own name, so that a
// database left behind by a crashed run says which test made it.
func scratchName(t *testing.T) string {
	var b strings.Builder
	b.WriteString("etm_test_")
	for _, c := range strings.ToLower(t.Name()) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteRune(c)
		default:
			b.WriteByte('_')
		}
	}

	// Postgres truncates identifiers at 63 bytes, which would collide two long
	// test names onto one database; the counter is what keeps them apart.
	name := b.String()
	suffix := fmt.Sprintf("_%d", scratch.Add(1))
	if len(name)+len(suffix) > 63 {
		name = name[:63-len(suffix)]
	}
	return name + suffix
}
