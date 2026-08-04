// Package web carries the built frontend into the Go binary.
//
// The Go file lives inside the npm package because //go:embed cannot reach
// upward through a parent directory — it can only embed what sits at or below
// its own directory. That is the whole reason this file is here and not under
// internal/.
//
// dist/ is a build product and is not committed; only the .gitkeep is, so that
// the embed directive always resolves and `go test ./...` works without a
// frontend build. Assets is what makes that safe: a binary built without
// running the frontend build refuses to start rather than serving a blank page.
package web

import (
	"embed"
	"errors"
	"io/fs"
)

// all: is required — without it the .gitkeep placeholder would be skipped and
// an unbuilt dist/ would fail to compile rather than fail to boot.
//
//go:embed all:dist
var distFS embed.FS

// ErrNotBuilt reports a binary compiled without a frontend build.
var ErrNotBuilt = errors.New("frontend assets missing from binary: run `make` rather than `go build`")

// Assets returns the built frontend rooted at dist/, or ErrNotBuilt if the
// build order was skipped.
func Assets() (fs.FS, error) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, err
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil, ErrNotBuilt
	}
	return sub, nil
}
