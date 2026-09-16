// Command tsgen writes the client's message types from the Go definitions they
// have to agree with. Run it through `make generate`.
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
