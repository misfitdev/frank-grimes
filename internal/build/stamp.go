// Package build reports which commit a binary was built from.
//
// The two binaries are one contract in two halves, and a mismatched pair fails
// in ways that read as engine defects. Nothing can prevent that; this is what
// lets someone check.
package build

import (
	"runtime/debug"
	"strings"
)

// Stamp is the revision this binary was built from, and whether the tree it was
// built from had uncommitted changes.
//
// Read from the build info Go embeds for a module built inside a repository,
// rather than from a linker flag: a stamp that has to be passed in is absent
// from exactly the build nobody was careful about.
func Stamp() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	var revision, modified string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				modified = " (with uncommitted changes)"
			}
		}
	}
	if revision == "" {
		// go run, or a build from outside a work tree. Saying so beats naming a
		// commit that is not where these bytes came from.
		return "unknown"
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	return revision + modified
}

// Line is what a --version flag prints: the binary's name and its stamp.
func Line(name string) string {
	return strings.TrimSpace(name + " " + Stamp())
}
