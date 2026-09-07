package judge

// Issue #146 review finding (architecture, rounds 2 and 3): the pinned-rev git-seam
// contract existed in three hand-maintained copies — the cli's grepHead/showHead, the
// eval's repos.go twins, and the escalation sandbox's showFile — each re-implementing
// the same rules: grep exit 1 is no-matches, -z NUL parsing with the per-entry rev:
// prefix strip, ls-tree absence distinct from failure, cat-file raw byte-exact reads.
// Every one of those rules needed a repair during this branch; equivalence maintained
// by hand let the eval silently measure with a stale contract. These constructors are
// the ONE implementation: both call sites inject their own runner, and the contract's
// tests live here once.

import (
	"context"
	"fmt"
	"strings"
)

// RawGitRunner returns git's stdout byte for byte, git's exit code, and an error only
// for a failed process start. File content and NUL-separated listings must not be
// trimmed (a trimmed blob shifts every line below its leading blank lines — the
// showFile lesson).
type RawGitRunner func(ctx context.Context, dir string, args ...string) ([]byte, int, error)

// GrepSeam returns a GrepPaths that lists the paths at rev whose content matches the
// RE2 pattern, read raw and NUL-separated (-z). Exit 1 is "no matches", not a failure;
// any other nonzero exit is.
func GrepSeam(ctx context.Context, runRaw RawGitRunner, dir, rev string) GrepPaths {
	return func(pattern string) ([]string, error) {
		out, code, err := runRaw(ctx, dir, "grep", "-l", "-i", "-E", "-z", pattern, rev)
		if err != nil {
			return nil, err
		}
		if code != 0 && code != 1 {
			return nil, fmt.Errorf("git grep -l -i -E at %s failed with exit code %d", rev, code)
		}
		prefix := rev + ":"
		var paths []string
		for _, entry := range strings.Split(string(out), "\x00") {
			if entry == "" {
				continue
			}
			paths = append(paths, strings.TrimPrefix(entry, prefix))
		}
		return paths, nil
	}
}

// ShowSeam returns a ShowHead over runRaw: absence (the path is not in the tree) is
// (nil, false, nil) via ls-tree's listing, while a nonzero exit or a transport error is
// an error — the two must never merge, because mapping every failure to "absent" biases
// a covering-test search toward confirming absence.
func ShowSeam(ctx context.Context, runRaw RawGitRunner, dir, rev string) ShowHead {
	return func(path string) ([]byte, bool, error) {
		listed, code, err := runRaw(ctx, dir, "ls-tree", "--name-only", "-z", rev, "--", path)
		if err != nil {
			return nil, false, err
		}
		if code != 0 {
			return nil, false, fmt.Errorf("git ls-tree %s -- %s failed with exit code %d", rev, path, code)
		}
		if strings.Trim(string(listed), "\x00") == "" {
			return nil, false, nil
		}
		body, code, err := runRaw(ctx, dir, "cat-file", "blob", rev+":"+path)
		if err != nil {
			return nil, false, err
		}
		if code != 0 {
			return nil, false, fmt.Errorf("git cat-file blob %s:%s failed with exit code %d", rev, path, code)
		}
		return body, true, nil
	}
}
