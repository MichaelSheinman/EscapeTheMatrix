// Package content carries the game's authored content files into the binary.
package content

import "embed"

// FS holds every authored content file.
//
//go:embed playbook events challenges situations environments characters conflicts jobs
var FS embed.FS
