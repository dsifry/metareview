package gitcontext

import (
	"errors"
	"strings"
)

// PathsSince reports whether from is an ancestor of to and, when it is, every path the commits from..to change (a
// rename counts as both of its paths). It lets the review gate tell a marker's head from a HEAD that only added the
// gate's own artifacts on top of it (#161). git's "no" (not an ancestor) is false; a commit git does not have, or any
// other failure, is an error — the caller then treats the marker as not current (fail closed).
func PathsSince(root, from, to string) ([]string, bool, error) {
	if _, err := git(root, "merge-base", "--is-ancestor", from, to); err != nil {
		var exit *gitExitError
		if errors.As(err, &exit) && exit.code == 1 {
			return nil, false, nil
		}
		return nil, false, err
	}
	// Every submodule pointer counts, whatever diff.ignoreSubmodules or .gitmodules say: a gitlink bump is not an artifact.
	out, err := git(root, "diff", "--name-only", "--no-renames", "--ignore-submodules=none", "-z", from, to, "--")
	if err != nil {
		return nil, false, err
	}
	var paths []string
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths, true, nil
}

// ChangedSince is PathsSince bound to root, the shape reviewstate.CurrentReviewEvidence takes.
func ChangedSince(root string) func(from, to string) ([]string, bool, error) {
	return func(from, to string) ([]string, bool, error) { return PathsSince(root, from, to) }
}
