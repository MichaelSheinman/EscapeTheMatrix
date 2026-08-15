// Command wirecheck fails the build if the HTTP layer can encode a response
// without going through internal/wire.
//
// internal/wire exists so that no collection the server sends is `null` — the
// bug that comment discipline failed to prevent, sixty-two hand-written guards
// on the client notwithstanding. But routing today's four encode sites through
// it only fixes today. A handler added next month that reaches for
// `json.NewEncoder(w).Encode(view)` restores the bug, compiles, passes review,
// and is found by a player.
//
// So the guarantee is held by a rule a machine checks rather than by everyone
// remembering: package api does not import encoding/json. It does not need to
// — request bodies are decoded in internal/command, responses are encoded by
// wire.Encode — so the ban costs nothing and there is no legitimate use to
// argue about at the point somebody hits it.
//
// Deliberately not a ban across internal/. The other three json.Marshal calls
// are not response encoding and must keep working: internal/command decodes
// request bodies and encodes an error map that has no nil to get wrong,
// internal/realtime marshals an envelope whose payload is already bytes, and
// internal/routine and internal/challenge serialise to Postgres columns, where
// null and [] mean the same thing to the reader on the other side. A check
// that flags those trains people to ignore it.
//
// Run by `make check`.
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
// reason reported when it does. One entry today; the shape is here so that
// adding a second is a line rather than a rewrite.
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
	// Same guard as tsgen: run from anywhere else and the directories below
	// resolve to nothing, which would pass silently.
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
//
// It reads the directory itself rather than using parser.ParseDir, which is
// deprecated, and only needs the import block of each file — ImportsOnly stops
// the parse at the first declaration.
func check(dir string, banned map[string]string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", dir, err)
	}

	set := token.NewFileSet()
	var found int
	// os.ReadDir sorts by filename, so two offences are reported in the same
	// order on every machine.
	for _, entry := range entries {
		name := entry.Name()
		// Tests are exempt: decoding a response in order to assert on it is
		// the opposite of the mistake this is guarding against.
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
