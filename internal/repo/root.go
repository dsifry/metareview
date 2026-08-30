package repo

import (
	"errors"
	"os"
	"path/filepath"
)

// Root walks up from start to the repository it belongs to.
//
// Every command resolved paths against the process's working directory, which is correct when a
// human types the command from the top of a checkout and wrong in the one case the enforcement
// model depends on: a Stop hook inherits the session's cwd, which is wherever the agent happened
// to be. `metareview status` then discovered no review logs, reported blocked:false, and the
// hook exited 0 — so the gate was bypassed by the entirely ordinary act of working in a
// subdirectory, and bypassed SILENTLY, which is the worst way for a gate to fail.
//
// The markers, in order: `.metareview` because a repository metareview has run in names itself;
// `docs/metareview` because that is the durable, committed directory reviewlog.Discover actually
// reads, so a checkout carrying review logs is a repository even where the transient state has
// been cleaned away; and `.git` as the fallback for the first run, matched as either a directory
// or a file so that worktrees and submodules resolve too.
func Root(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		for _, marker := range []string{".metareview", "docs/metareview", ".git"} {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached the filesystem root without finding either marker.
			return "", errors.New("metareview: no repository found at or above " + start + " (looked for .metareview, docs/metareview or .git)")
		}
		dir = parent
	}
}

// RootOr returns Root(start), falling back to start when there is no repository above it. Used
// where "not in a repository" is a condition the caller reports for itself rather than an error.
func RootOr(start string) string {
	if root, err := Root(start); err == nil {
		return root
	}
	return start
}
