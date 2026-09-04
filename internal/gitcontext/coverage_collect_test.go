package gitcontext

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// fakeGitFunc builds a git seam: `fail` decides which invocation errors (by its args), everything
// else returns a benign SHA for rev-parse calls and empty output otherwise, so collect/resolveBase
// run to the point under test without a real repository.
func fakeGitFunc(fail func(args []string) error) func(string, ...string) (string, error) {
	return func(_ string, args ...string) (string, error) {
		if fail != nil {
			if err := fail(args); err != nil {
				return "", err
			}
		}
		if len(args) > 0 && args[0] == "rev-parse" {
			return "deadbeefdeadbeef", nil
		}
		return "", nil
	}
}

func isDiff(args []string) bool { return len(args) > 0 && args[0] == "diff" }
func hasRange(args []string) bool {
	for _, a := range args {
		if strings.Contains(a, "..HEAD") {
			return true
		}
	}
	return false
}
func hasPathspec(args []string) bool { return argsHave(args, "--") }

// --- resolveBase branches ---------------------------------------------------

func TestResolveBaseRejectsInvalidRef(t *testing.T) {
	installFakeGit(t, fakeGitFunc(nil))
	if _, err := resolveBase("root", "a..b"); err == nil {
		t.Fatalf("an invalid base ref must be rejected before any git call")
	}
}

func TestResolveBaseRejectsUnverifiableRef(t *testing.T) {
	installFakeGit(t, fakeGitFunc(func(args []string) error {
		if args[0] == "rev-parse" && argsHave(args, "--verify") {
			return errors.New("unknown revision")
		}
		return nil
	}))
	if _, err := resolveBase("root", "v99.99.99"); err == nil {
		t.Fatalf("a ref that does not resolve to a commit must be rejected")
	}
}

func TestResolveBaseAbbrevRefTimeout(t *testing.T) {
	installFakeGit(t, fakeGitFunc(func(args []string) error {
		if args[0] == "rev-parse" && argsHave(args, "--abbrev-ref") {
			return fmt.Errorf("stalled: %w", ErrTimeout)
		}
		return nil
	}))
	if _, err := resolveBase("root", ""); !errors.Is(err, ErrTimeout) {
		t.Fatalf("an abbrev-ref timeout must abort resolution, got %v", err)
	}
}

func TestResolveBaseMergeBaseTimeout(t *testing.T) {
	installFakeGit(t, fakeGitFunc(func(args []string) error {
		if args[0] == "merge-base" {
			return fmt.Errorf("stalled: %w", ErrTimeout)
		}
		return nil
	}))
	if _, err := resolveBase("root", ""); !errors.Is(err, ErrTimeout) {
		t.Fatalf("a merge-base timeout must abort resolution, got %v", err)
	}
}

func TestResolveBaseHeadTilde1Timeout(t *testing.T) {
	// merge-base returns empty (no main/master) so resolution falls through to HEAD~1, which stalls.
	installFakeGit(t, fakeGitFunc(func(args []string) error {
		if args[0] == "rev-parse" && argsHave(args, "HEAD~1") {
			return fmt.Errorf("stalled: %w", ErrTimeout)
		}
		return nil
	}))
	if _, err := resolveBase("root", ""); !errors.Is(err, ErrTimeout) {
		t.Fatalf("a HEAD~1 timeout must abort resolution, got %v", err)
	}
}

func TestResolveBaseFallsThroughEmptyMergeBaseToHeadTilde1(t *testing.T) {
	// merge-base returns empty for both candidates (the continue at :340), so HEAD~1 answers.
	installFakeGit(t, func(_ string, args ...string) (string, error) {
		switch {
		case args[0] == "merge-base":
			return "", nil // empty -> continue
		case args[0] == "rev-parse" && argsHave(args, "HEAD~1"):
			return "cafebabecafebabe", nil
		case args[0] == "rev-parse":
			return "deadbeefdeadbeef", nil
		}
		return "", nil
	})
	base, err := resolveBase("root", "")
	if err != nil || base != "cafebabecafebabe" {
		t.Fatalf("empty merge-bases should fall through to HEAD~1, got base=%q err=%v", base, err)
	}
}

// --- collect interior error branches ---------------------------------------

func collectErr(t *testing.T, excludes []string, fail func(args []string) error) {
	t.Helper()
	installFakeGit(t, fakeGitFunc(fail))
	if _, err := collect("root", "base", excludes, nil); err == nil {
		t.Fatalf("collect must surface the injected git failure")
	}
}

func TestCollectSurfacesRevParseHeadError(t *testing.T) {
	collectErr(t, nil, func(args []string) error {
		if len(args) == 2 && args[0] == "rev-parse" && args[1] == "HEAD" {
			return errors.New("head boom")
		}
		return nil
	})
}

func TestCollectSurfacesBranchDiffError(t *testing.T) {
	collectErr(t, nil, func(args []string) error {
		if isDiff(args) && hasRange(args) && !argsHave(args, "--cached") {
			return errors.New("branch diff boom")
		}
		return nil
	})
}

func TestCollectSurfacesStagedDiffError(t *testing.T) {
	collectErr(t, nil, func(args []string) error {
		if isDiff(args) && argsHave(args, "--cached") {
			return errors.New("staged diff boom")
		}
		return nil
	})
}

func TestCollectSurfacesWorkingTreeDiffError(t *testing.T) {
	collectErr(t, nil, func(args []string) error {
		if isDiff(args) && !hasRange(args) && !argsHave(args, "--cached") {
			return errors.New("worktree diff boom")
		}
		return nil
	})
}

func isLsFiles(args []string) bool { return len(args) > 0 && args[0] == "ls-files" }

// An untracked path that escapes the root makes readUntrackedExcerpts fail inside collect (the
// filtered ls-files at :136/:137).
func TestCollectSurfacesUntrackedError(t *testing.T) {
	installFakeGit(t, func(_ string, args ...string) (string, error) {
		if isLsFiles(args) {
			return ".." + string('/') + "escape", nil
		}
		if len(args) > 0 && args[0] == "rev-parse" {
			return "deadbeefdeadbeef", nil
		}
		return "", nil
	})
	if _, err := collect("root", "base", nil, nil); err == nil {
		t.Fatalf("an escaping untracked path must surface an error")
	}
}

// With excludes present, the RAW untracked scan (bare ls-files, no pathspec) escaping the root makes
// the second readUntrackedExcerpts fail (:157/:159), while the filtered scan stays clean.
func TestCollectSurfacesRawUntrackedError(t *testing.T) {
	installFakeGit(t, func(_ string, args ...string) (string, error) {
		if isLsFiles(args) && !hasPathspec(args) { // the raw scan only
			return ".." + string('/') + "escape", nil
		}
		if len(args) > 0 && args[0] == "rev-parse" {
			return "deadbeefdeadbeef", nil
		}
		return "", nil
	})
	if _, err := collect("root", "base", []string{"docs/**"}, nil); err == nil {
		t.Fatalf("an escaping raw untracked path must surface an error")
	}
}

// --- collect raw-measurement error branches (excludes present) --------------

func TestCollectSurfacesRawBranchMeasureError(t *testing.T) {
	collectErr(t, []string{"docs/**"}, func(args []string) error {
		if isDiff(args) && hasRange(args) && !hasPathspec(args) {
			return errors.New("raw branch boom")
		}
		return nil
	})
}

func TestCollectSurfacesRawStagedMeasureError(t *testing.T) {
	collectErr(t, []string{"docs/**"}, func(args []string) error {
		if isDiff(args) && argsHave(args, "--cached") && !hasPathspec(args) {
			return errors.New("raw staged boom")
		}
		return nil
	})
}

func TestCollectSurfacesRawWorkingTreeMeasureError(t *testing.T) {
	collectErr(t, []string{"docs/**"}, func(args []string) error {
		if isDiff(args) && !hasRange(args) && !argsHave(args, "--cached") && !hasPathspec(args) {
			return errors.New("raw worktree boom")
		}
		return nil
	})
}

// collect surfaces a resolveBase failure (invalid base ref), and CollectWith propagates it.
func TestCollectAndCollectWithSurfaceResolveBaseError(t *testing.T) {
	installFakeGit(t, fakeGitFunc(nil))
	if _, err := collect("root", "a..b", nil, nil); err == nil {
		t.Fatalf("collect must surface a resolveBase error")
	}
	if _, err := CollectWith("root", Options{Base: "a..b"}); err == nil {
		t.Fatalf("CollectWith must propagate the collect error")
	}
}

// truncatingGit makes collect report a truncated branch diff (so collectWith enters the
// branch-file path) and surfaces one generated file matching the excludes.
func truncatingGit(_ string, args ...string) (string, error) {
	switch {
	case len(args) > 0 && args[0] == "rev-parse":
		return "deadbeefdeadbeef", nil
	case isDiff(args) && hasRange(args) && hasPathspec(args): // the filtered branch diff
		return strings.Repeat("x", maxDiffBytes+10), nil
	case argsHave(args, "--name-only"): // raw file lists feeding exactExcludesExcept/generated
		return "docs/gen.go", nil
	}
	return "", nil
}

// With Exceptions set and a truncated diff, collectWith recomputes the effective excludes via
// exactExcludesExcept and then materializes the branch files.
func TestCollectWithExceptionsRecomputesOnTruncation(t *testing.T) {
	installFakeGit(t, truncatingGit)
	runGit := func(_ string, _ []string, args ...string) ([]byte, error) {
		if argsHave(args, "--name-only") {
			return []byte("a.go\x00"), nil
		}
		return []byte("diff body"), nil
	}
	ctx, err := CollectWith("root", Options{
		Base: "base", Excludes: []string{"docs/**"}, Exceptions: []string{"keep.go"}, RunGit: runGit,
	})
	if err != nil {
		t.Fatalf("CollectWith: %v", err)
	}
	if !ctx.DiffTruncated || len(ctx.BranchFiles) == 0 {
		t.Fatalf("a truncated diff must produce branch files: %+v", ctx.DiffTruncated)
	}
}

// A failure in the raw branch-file measurement (the nil-excludes measure) surfaces from collectWith.
func TestCollectWithSurfacesMeasureBranchFilesError(t *testing.T) {
	installFakeGit(t, truncatingGit)
	// measureBranchFiles(nil) issues a --name-only WITHOUT a pathspec; fail exactly that.
	runGit := func(_ string, _ []string, args ...string) ([]byte, error) {
		if argsHave(args, "--name-only") && !hasPathspec(args) {
			return nil, errors.New("raw measure boom")
		}
		if argsHave(args, "--name-only") {
			return []byte("a.go\x00"), nil
		}
		return []byte("diff body"), nil
	}
	if _, err := CollectWith("root", Options{
		Base: "base", Excludes: []string{"docs/**"}, RunGit: runGit,
	}); err == nil {
		t.Fatalf("a raw branch-file measurement failure must surface")
	}
}
