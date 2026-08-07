package challenge

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

/*
Hot reload in development (§5.2).

	Dev mode hot-reloads on file change; production validates at startup so a
	malformed challenge fails in CI, not at 2am.

Both halves of that sentence are here. The reload is deliberately *only* the
development path — the binary that ships embeds its content (see package
content) and has nothing on disk to watch — and what it reloads is the whole
catalogue at once, validated in full, or nothing.

# Why nothing rather than what parsed

An author mid-edit produces broken YAML constantly, and a reloader that took the
valid half would silently take a challenge out of the game and put it back a
keystroke later. So a failed reload logs the error and **keeps the last catalogue
that loaded**: the editor's terminal says what is wrong, the running process
keeps working, and the only way to get broken content into a build is to commit
it — where CI's copy of this same loader refuses it.

# Why polling

The alternative is fsnotify, which is a dependency, a platform matrix, and a
watch registration per directory that has to be re-registered when an author
creates one. Content is a few dozen small files and this loop runs only in
development, where a second of latency is not a cost anybody can perceive. The
whole thing is one WalkDir and a comparison.
*/

// Poll is how often the content tree is checked for changes.
//
// A second: fast enough that saving a file and alt-tabbing is enough, slow
// enough that it is not a busy loop on somebody's laptop.
const Poll = time.Second

/*
Watch reloads the catalogue whenever the content tree changes.

`tree` is the content directory in the working copy — the one with `challenges/`
inside it. `after` runs on every successful reload with the new catalogue
already in place, and is where the caller re-publishes to the catalog table; a
reload that added a challenge and did not publish it would be a definition no
run could reference.

It blocks until ctx is done, so callers run it in a goroutine. It returns
without watching anything if the directory is not there, which is the ordinary
case for a binary running outside its own source tree.
*/
func Watch(ctx context.Context, tree string, log *slog.Logger, after func(context.Context) error) {
	where := filepath.Join(tree, root)
	if info, err := os.Stat(where); err != nil || !info.IsDir() {
		log.Debug("not watching challenge content", "path", where)
		return
	}
	log.Info("watching challenge content", "path", where, "every", Poll)

	// Once before the loop, because the catalogue in memory came from the
	// *embedded* copy at init and the working copy is what an author is
	// editing. A process started after an edit and before a rebuild would
	// otherwise be serving the last build's content until somebody touched a
	// file — which is the one moment a reloader is most expected to help.
	last, err := fingerprint(where)
	if err != nil {
		log.Warn("could not read challenge content", "error", err)
	}
	reload(ctx, tree, log, after)

	ticker := time.NewTicker(Poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		next, err := fingerprint(where)
		if err != nil {
			log.Warn("could not read challenge content", "error", err)
			continue
		}
		if next == last {
			continue
		}
		last = next

		reload(ctx, tree, log, after)
	}
}

// reload takes the whole working copy, or none of it.
func reload(ctx context.Context, tree string, log *slog.Logger, after func(context.Context) error) {
	loaded, err := Load(os.DirFS(tree))
	if err != nil {
		// Kept, not cleared. See the note at the top of this file: an author
		// mid-edit is the ordinary case, and a process that emptied itself
		// every time a file was half-saved would be worse than no reload at all.
		log.Error("challenge content is not valid; keeping the last that was", "error", err)
		return
	}
	swap(loaded)
	log.Info("challenge content reloaded", "challenges", len(loaded))

	if after == nil {
		return
	}
	if err := after(ctx); err != nil {
		log.Error("could not publish the reloaded challenge content", "error", err)
	}
}

/*
fingerprint is a cheap summary of the tree: every path, its size and its
modification time.

Names as well as times, so creating an empty file, deleting one, or renaming a
variant directory all register — which a "newest modification time" check would
miss, and each of those is a real edit an author makes.
*/
func fingerprint(where string) (string, error) {
	var out string
	err := filepath.WalkDir(where, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			out += path + "/\n"
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out += fmt.Sprintf("%s %d %d\n", path, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	return out, err
}
