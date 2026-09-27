// Package baseref resolves an explicitly requested review base (`--base <ref>`) to a commit, with one rule every
// command shares (#175). A branch name — local (`main`, `refs/heads/main`) or remote-tracking (`origin/main`,
// `refs/remotes/origin/main`) — names the line this work forked from, so it resolves to merge-base(HEAD, ref):
// once that branch advances, its tip would fold the branch's new commits, inverted, into the reviewed diff. Anything
// else — a SHA, a tag, `HEAD`, `HEAD~2` — names an exact commit and resolves to it.
//
// Ambiguity follows git's precedence for a short name: a branch whose name is also a SHA prefix resolves as the
// branch. A full 40- or 64-hex string is always the commit.
package baseref

import (
	"fmt"
	"regexp"
	"strings"
)

// Runner runs `git <args>` and returns its trimmed stdout, whether it exited 0, and an execution error (a timeout,
// a missing binary). A non-zero exit is ok=false with a nil error.
type Runner func(args ...string) (out string, ok bool, err error)

var fullSHA = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)

// Resolve returns the commit ref names as a review base. The caller validates ref first.
func Resolve(run Runner, ref string) (string, error) {
	tip, ok, err := run("rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	if !ok || tip == "" {
		return "", fmt.Errorf("invalid git base: %s", ref)
	}
	branch, err := isBranch(run, ref)
	if err != nil || !branch {
		return tip, err
	}
	base, ok, err := run("merge-base", "HEAD", tip)
	if err != nil {
		return "", err
	}
	if !ok || base == "" {
		return "", fmt.Errorf("invalid git base: %s has no merge base with HEAD", ref)
	}
	return base, nil
}

// isBranch reports whether ref names a local or remote-tracking branch.
func isBranch(run Runner, ref string) (bool, error) {
	if fullSHA.MatchString(ref) {
		return false, nil
	}
	candidates := []string{"refs/heads/" + ref, "refs/remotes/" + ref}
	if strings.HasPrefix(ref, "refs/heads/") || strings.HasPrefix(ref, "refs/remotes/") {
		candidates = []string{ref}
	}
	for _, candidate := range candidates {
		_, ok, err := run("show-ref", "--verify", "--quiet", candidate)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}
