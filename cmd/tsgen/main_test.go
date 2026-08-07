package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
)

// The generated file is committed, which is only worth anything if it is
// provably the file the current Go definitions produce. This is the test that
// makes "generate the TypeScript from the Go structs" a rule rather than a
// habit: change a payload without regenerating and the suite says so, in the
// commit that changed it, rather than a client decoding a field that is no
// longer there.
func TestGeneratedTypesMatchTheGoDefinitions(t *testing.T) {
	path := filepath.Join("..", "..", output)

	committed, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s is missing — run `make generate`: %v", output, err)
	}

	if want := realtime.TypeScript(); !bytes.Equal(committed, want) {
		t.Fatalf("%s is stale — run `make generate`.\n\nit should be:\n%s", output, want)
	}
}

// Rendering has to be a function of the definitions alone. If it were not, the
// staleness test above would fail at random and be turned off.
func TestGenerationIsDeterministic(t *testing.T) {
	if first, second := realtime.TypeScript(), realtime.TypeScript(); !bytes.Equal(first, second) {
		t.Fatalf("two renderings differ:\n%s\n---\n%s", first, second)
	}
}
