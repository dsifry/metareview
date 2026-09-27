// Package baseref resolves an explicitly requested review base (`--base <ref>`) to a commit, with one rule every
// command shares (#175). A branch name — local (`main`, `refs/heads/main`) or remote-tracking (`origin/main`,
// `refs/remotes/origin/main`) — names the line this work forked from, so it resolves to merge-base(HEAD, ref):
// once that branch advances, its tip would fold the branch's new commits, inverted, into the reviewed diff. Anything
// else — a SHA, a tag, `HEAD`, `HEAD~2` — names an exact commit and resolves to it.
//
// A short name that is both a branch and a SHA prefix resolves as the branch (git's precedence), and a branch that
// shares its name with a tag resolves as the branch — its own ref is merge-based, never the tag. A full 40- or
// 64-hex string is always the commit.
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
	branch, err := branchRef(run, ref)
	if err != nil {
		return "", err
	}
	if branch == "" {
		return commit(run, ref)
	}
	// The branch's own ref, not the short name: a tag of the same name would win rev-parse's lookup.
	tip, err := commit(run, branch)
	if err != nil {
		return "", err
	}
	base, ok, err := run("merge-base", "HEAD", tip)
	if err != nil {
		return "", err
	}
	if ok && base != "" {
		return base, nil
	}
	if shallow, _, err := run("rev-parse", "--is-shallow-repository"); err == nil && shallow == "true" {
		return "", fmt.Errorf("invalid git base: %s has no merge base with HEAD in this shallow clone; fetch full history (e.g. actions/checkout fetch-depth: 0)", ref)
	}
	return "", fmt.Errorf("invalid git base: %s has no merge base with HEAD", ref)
}

// commit resolves ref to exactly the commit it names.
func commit(run Runner, ref string) (string, error) {
	sha, ok, err := run("rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	if !ok || sha == "" {
		return "", fmt.Errorf("invalid git base: %s", ref)
	}
	return sha, nil
}

// branchRef returns the full ref of the local or remote-tracking branch ref names, or "" if it names none.
func branchRef(run Runner, ref string) (string, error) {
	if fullSHA.MatchString(ref) {
		return "", nil
	}
	candidates := []string{"refs/heads/" + ref, "refs/remotes/" + ref}
	if strings.HasPrefix(ref, "refs/heads/") || strings.HasPrefix(ref, "refs/remotes/") {
		candidates = []string{ref}
	}
	for _, candidate := range candidates {
		_, ok, err := run("show-ref", "--verify", "--quiet", candidate)
		if err != nil {
			return "", err
		}
		if ok {
			return candidate, nil
		}
	}
	return "", nil
}
