// Command tsgen writes the client's message types from the Go definitions they
// have to agree with (technical.md §8, protocol rule 1).
//
// Run it through `make generate`. The registry it reads is built by package
// initialisation, so this binary imports every package that defines a message —
// which is the one thing to remember when adding a module: define the message
// where the table lives (§3.1), and import that package here.
//
// The output is committed. A stale file fails a test in this package rather
// than a build nobody ran, so the diff that changes a payload is the diff that
// changes the client's type.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	// Imported for their side effect: each defines message types into the
	// registry that TypeScript() renders.
	_ "github.com/MichaelSheinman/EscapeTheMatrix/internal/api"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
)

// output is where the generated module goes, relative to the module root.
const output = "web/src/realtime/messages.gen.ts"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "tsgen:", err)
		os.Exit(1)
	}
}

func run() error {
	// Run from anywhere else and the file would land somewhere else, silently,
	// leaving the committed one stale.
	if _, err := os.Stat("go.mod"); err != nil {
		return fmt.Errorf("run from the module root (`make generate`): %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(output, realtime.TypeScript(), 0o644); err != nil {
		return err
	}

	fmt.Println("wrote", output)
	return nil
}
