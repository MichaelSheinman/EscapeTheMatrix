package simulation

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

/*
The rules of the package, asserted rather than remembered.

technical.md §3.1 calls the pure core the most important structural rule in the
backend, and a structural rule that is only written down is one somebody breaks
in a hurry on a Tuesday. What follows reads this package's own source and fails
the build if it has grown a dependency, a database, a network call, or a clock.

Only the non-test files are examined. The tests are allowed a `testing` import
and a clock of their own — they are the caller, and every caller injects one.
*/

// sources parses this package's non-test files.
func sources(t *testing.T) map[string]*ast.File {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package directory: %v", err)
	}

	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files[name] = f
	}
	if len(files) == 0 {
		t.Fatal("no source files found — this test is not looking where it thinks it is")
	}
	return files
}

// The core imports nothing but the standard library — not even the rest of this
// repository. It is a build target for WASM (§1) and it is the package every
// other one is allowed to depend on, so the arrow has to point one way.
func TestThePackageImportsNothingButTheStandardLibrary(t *testing.T) {
	t.Parallel()

	// Deliberately narrow: not a list of what is banned, but of what is
	// allowed. A rule written as a denylist is one a new import slips past.
	allowed := map[string]bool{"math": true, "time": true}

	for name, file := range sources(t) {
		for _, imp := range file.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("%s: unquote import %s: %v", name, imp.Path.Value, err)
			}
			if !allowed[path] {
				t.Errorf("%s imports %q — the pure core may import only %v", name, path, keys(allowed))
			}
		}
	}
}

// No database, no network, no clock. The first two are covered by the import
// test above; this one catches the third, which is the only forbidden thing
// reachable through an allowed import.
func TestThePackageNeverReadsTheClock(t *testing.T) {
	t.Parallel()

	for name, file := range sources(t) {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			// time.Now and time.Since are the two ways a package accidentally
			// starts depending on when it happens to run.
			if pkg.Name == "time" && (sel.Sel.Name == "Now" || sel.Sel.Name == "Since") {
				t.Errorf("%s calls time.%s — time is injected through Clock (§3.1)", name, sel.Sel.Name)
			}
			return true
		})
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
