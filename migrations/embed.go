// Package migrations carries the schema into the binary.
//
// The files sit here rather than under internal/ for one mechanical reason:
// //go:embed cannot reach upward through a parent directory, so the embedding
// declaration has to live beside what it embeds. internal/store owns applying
// them (technical.md §13).
//
// Forward-only, never edited after merge. A mistake in a merged migration is
// corrected by a new migration, not by rewriting history — the schema of a
// deployed database can only move in one direction.
package migrations

import "embed"

// FS holds every .sql migration, in lexical order by filename.
//
//go:embed *.sql
var FS embed.FS
