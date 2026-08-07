// Package content carries the game's authored content files into the binary.
//
// technical.md §5.2 decides that **repo files are the source of truth, not a
// CMS**: content is versioned, diffable, reviewable, and it travels with the
// code that interprets it. On boot the server loads and validates it, so a
// malformed file fails in CI rather than at two in the morning.
//
// The files sit here rather than under internal/ for the same mechanical reason
// migrations do: //go:embed cannot reach upward through a parent directory, so
// the embedding declaration has to live beside what it embeds. This package
// holds no game logic and knows nothing about what any of these files mean —
// the module that owns a kind of content parses and validates its own (§3.1).
// internal/playbook is the first of them.
package content

import "embed"

// FS holds every authored content file.
//
// The pattern names directories rather than `all:` — an editor's swap file or a
// stray dotfile is not content, and a loader that walks a directory should not
// have to know that.
//
//go:embed playbook
var FS embed.FS
