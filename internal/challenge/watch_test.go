package challenge

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

/*
Hot reload (§5.2's "dev mode hot-reloads on file change").

Two claims worth holding, and the second is the one that matters. Editing a file
puts the change into the running process without a restart — and editing it
*badly* does not, because an author mid-edit produces broken YAML constantly and
a reloader that took the valid half would silently pull a challenge out of the
game and put it back a keystroke later.
*/

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// write puts one challenge into a working copy on disk.
func write(t *testing.T, tree, variant, body string) {
	t.Helper()
	dir := filepath.Join(tree, "challenges", "resolve_conflict", variant)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("make %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "challenge.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", dir, err)
	}
}

const watched = `
name: The argument on the stoop
mechanic: watched
where:
  districts: [east-side]
  zones: [on-foot]
`

// until waits for a condition the reloader is expected to bring about.
func until(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * Poll)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(Poll / 4)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestContentIsReloadedWhenAFileChanges(t *testing.T) {
	Register(staged{name: "watched", plan: twoScenes()})
	t.Cleanup(func() { delete(mechanics, "watched") })

	held := catalogue.Load()
	t.Cleanup(func() { catalogue.Store(held) })

	tree := t.TempDir()
	write(t, tree, "a", watched)

	ctx, stop := context.WithCancel(t.Context())
	// Stopped and *waited for*, before the cleanup above puts the shipped
	// catalogue back: a reloader still mid-iteration would otherwise swap in a
	// temporary directory's content after the test that made it had finished.
	watching := make(chan struct{})
	t.Cleanup(func() { stop(); <-watching })

	var published atomic.Int64
	go func() {
		defer close(watching)
		Watch(ctx, tree, quiet(), func(context.Context) error {
			published.Add(1)
			return nil
		})
	}()

	// The working copy is taken once at the start, because a process booted
	// after an edit and before a rebuild is holding the last build's content.
	until(t, "the working copy to be taken", func() bool { return len(All()) == 1 })

	// A second variant appears, as an author writing one does.
	was := published.Load()
	write(t, tree, "b", watched)
	until(t, "the new variant to be loaded", func() bool { return len(All()) == 2 })
	until(t, "the reload to re-publish the catalogue", func() bool { return published.Load() > was })

	// The same author, mid-edit. The process keeps what it had.
	write(t, tree, "b", "name: half a thought\nmechanic: watch")
	deadline := time.Now().Add(4 * Poll)
	for time.Now().Before(deadline) {
		if len(All()) != 2 {
			t.Fatalf("broken content emptied the catalogue: %d challenges", len(All()))
		}
		time.Sleep(Poll / 2)
	}

	// And finishing the edit takes.
	if err := os.RemoveAll(filepath.Join(tree, "challenges", "resolve_conflict", "b")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	until(t, "the withdrawn variant to go", func() bool { return len(All()) == 1 })
}

// A binary running outside its own source tree has nothing to watch, which is
// every deployed one: content is embedded, and the loader read it at init.
func TestWatchingNothingIsNotAnError(t *testing.T) {
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	Watch(ctx, filepath.Join(t.TempDir(), "nowhere"), quiet(), nil)
}
