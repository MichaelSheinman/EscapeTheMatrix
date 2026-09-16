// Command wirecheck fails the build if the HTTP layer can encode a response
// without going through internal/wire. Run by `make check`.
package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// restricted maps a package directory to the imports it may not have, with the
// reason reported when it does.
var restricted = map[string]map[string]string{
	"internal/api": {
		"encoding/json": "encode responses with wire.Encode — encoding/json emits `null` for a nil slice,\n" +
			"    which the client cannot call .length on (see internal/wire)",
	},
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "wirecheck:", err)
		os.Exit(1)
	}
}

func run() error {
	if _, err := os.Stat("go.mod"); err != nil {
		return fmt.Errorf("run from the module root (`make check`): %w", err)
	}

	var found int
	for dir, banned := range restricted {
		n, err := check(dir, banned)
		if err != nil {
			return err
		}
		found += n
	}

	if found > 0 {
		return fmt.Errorf("%d forbidden import(s)", found)
	}
	return nil
}

// check reports every banned import in dir, and returns how many it found.
func check(dir string, banned map[string]string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", dir, err)
	}

	set := token.NewFileSet()
	var found int
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}

		path := filepath.Join(dir, name)
		file, err := parser.ParseFile(set, path, nil, parser.ImportsOnly)
		if err != nil {
			return 0, fmt.Errorf("parse %s: %w", path, err)
		}

		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				continue
			}
			why, forbidden := banned[imported]
			if !forbidden {
				continue
			}
			at := set.Position(spec.Pos())
			fmt.Fprintf(os.Stderr, "%s:%d: %s must not import %q\n    %s\n",
				at.Filename, at.Line, dir, imported, why)
			found++
		}
	}
	return found, nil
}
