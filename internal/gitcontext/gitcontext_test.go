package gitcontext

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaxDiffBytesAccommodatesMediumDeletionReviews(t *testing.T) {
	if maxDiffBytes < 100_000 {
		t.Fatalf("maxDiffBytes = %d, want at least 100000 for medium deletion reviews", maxDiffBytes)
	}
}

func TestCollectWithExcludesKeepsGeneratedArtifactsOutOfTruncation(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	writeFile := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	git("init", "-b", "main")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test User")
	writeFile("internal/evidence/receipt.go", "package evidence\n")
	git("add", ".")
	git("commit", "-m", "base")
	base := strings.TrimSpace(commandOutput(t, root, "git", "rev-parse", "HEAD"))

	writeFile("internal/evidence/receipt.go", "package evidence\n\nfunc marker() {}\n")
	writeFile("docs/metareview/context/generated.md", strings.Repeat("generated artifact\n", maxDiffBytes/10))
	git("add", ".")
	git("commit", "-m", "change")

	context, err := CollectWithExcludes(root, base, []string{"docs/metareview/**"})
	if err != nil {
		t.Fatalf("collect filtered context: %v", err)
	}
	if context.DiffTruncated {
		t.Fatalf("generated artifact diff should not force truncation:\n%s", context.DiffStat)
	}
	if strings.Contains(context.Diff, "generated artifact") {
		t.Fatalf("generated artifact leaked into filtered diff")
	}
	if len(context.ChangedFiles) != 1 || context.ChangedFiles[0] != "internal/evidence/receipt.go" {
		t.Fatalf("changed files = %#v, want only real code file", context.ChangedFiles)
	}
}

func TestGeneratedMetareviewArtifactsDoNotConsumeSourceBudget(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	writeFile := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	git("init", "-b", "main")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test User")
	writeFile("internal/evidence/receipt.go", "package evidence\n")
	git("add", ".")
	git("commit", "-m", "base")
	base := strings.TrimSpace(commandOutput(t, root, "git", "rev-parse", "HEAD"))

	writeFile("internal/evidence/receipt.go", "package evidence\n\nfunc marker() {}\n")
	writeFile("docs/metareview/reviews/generated.md", strings.Repeat("generated artifact\n", maxDiffBytes/4))
	git("add", ".")
	git("commit", "-m", "source plus generated")

	context, err := CollectWithExcludes(root, base, []string{"docs/metareview/**"})
	if err != nil {
		t.Fatalf("collect filtered context: %v", err)
	}
	if context.RawDiffBytes <= maxDiffBytes {
		t.Fatalf("RawDiffBytes = %d, want generated-heavy raw diff over maxDiffBytes", context.RawDiffBytes)
	}
	if context.FilteredDiffBytes >= context.RawDiffBytes {
		t.Fatalf("FilteredDiffBytes = %d, RawDiffBytes = %d; filtered budget should exclude generated artifact", context.FilteredDiffBytes, context.RawDiffBytes)
	}
	if !containsString(context.GeneratedExcludedFiles, "docs/metareview/reviews/generated.md") {
		t.Fatalf("GeneratedExcludedFiles = %#v, want generated review artifact", context.GeneratedExcludedFiles)
	}
	if context.DiffTruncated {
		t.Fatalf("filtered source diff should not be truncated")
	}
}

func TestCollectWithExcludesExceptIncludesOnlyExplicitGeneratedTarget(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	writeFile := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	git("init", "-b", "main")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test User")
	writeFile("docs/metareview/reviews/target.md", "target base\n")
	writeFile("docs/metareview/context/noise.md", "noise base\n")
	git("add", ".")
	git("commit", "-m", "base")
	base := strings.TrimSpace(commandOutput(t, root, "git", "rev-parse", "HEAD"))

	writeFile("docs/metareview/reviews/target.md", "target changed\n")
	writeFile("docs/metareview/context/noise.md", strings.Repeat("noise artifact\n", 1000))
	git("add", ".")
	git("commit", "-m", "target plus unrelated generated")

	context, err := CollectWithExcludesExcept(root, base, []string{"docs/metareview/**"}, []string{"docs/metareview/reviews/target.md"})
	if err != nil {
		t.Fatalf("collect context: %v", err)
	}
	if !containsString(context.ChangedFiles, "docs/metareview/reviews/target.md") {
		t.Fatalf("ChangedFiles = %#v, want explicit generated target included", context.ChangedFiles)
	}
	if containsString(context.ChangedFiles, "docs/metareview/context/noise.md") {
		t.Fatalf("ChangedFiles = %#v, want unrelated generated artifact excluded", context.ChangedFiles)
	}
	if !strings.Contains(context.Diff, "target changed") {
		t.Fatalf("diff missing explicit target change:\n%s", context.Diff)
	}
	if strings.Contains(context.Diff, "noise artifact") {
		t.Fatalf("diff included unrelated generated artifact")
	}
	if !containsString(context.GeneratedExcludedFiles, "docs/metareview/context/noise.md") {
		t.Fatalf("GeneratedExcludedFiles = %#v, want unrelated generated artifact recorded", context.GeneratedExcludedFiles)
	}
}

func TestMoreThanUntrackedLimitRecordsOmissions(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	writeFile := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	git("init", "-b", "main")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test User")
	writeFile("README.md", "base\n")
	git("add", ".")
	git("commit", "-m", "base")
	base := strings.TrimSpace(commandOutput(t, root, "git", "rev-parse", "HEAD"))

	for i := 0; i < maxUntrackedFiles+5; i++ {
		writeFile(fmt.Sprintf("notes/untracked-%02d.txt", i), fmt.Sprintf("note %02d\n", i))
	}

	context, err := Collect(root, base)
	if err != nil {
		t.Fatalf("collect context: %v", err)
	}
	if context.UntrackedOmittedCount != 5 {
		t.Fatalf("UntrackedOmittedCount = %d, want 5", context.UntrackedOmittedCount)
	}
	if len(context.UntrackedFiles) != maxUntrackedFiles+5 {
		t.Fatalf("len(UntrackedFiles) = %d, want %d", len(context.UntrackedFiles), maxUntrackedFiles+5)
	}
}

func TestLargeUntrackedFileRecordsTruncationAndBudget(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	writeFile := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	git("init", "-b", "main")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test User")
	writeFile("README.md", "base\n")
	git("add", ".")
	git("commit", "-m", "base")
	base := strings.TrimSpace(commandOutput(t, root, "git", "rev-parse", "HEAD"))

	content := "package review\n\nvar Big = `" + strings.Repeat("x", maxUntrackedFileBytes+250) + "`\n"
	writeFile("internal/review/untracked_big.go", content)

	context, err := Collect(root, base)
	if err != nil {
		t.Fatalf("collect context: %v", err)
	}
	if context.UntrackedTruncatedCount != 1 {
		t.Fatalf("UntrackedTruncatedCount = %d, want 1", context.UntrackedTruncatedCount)
	}
	if context.FilteredDiffBytes < len(content) {
		t.Fatalf("FilteredDiffBytes = %d, want at least full untracked file size %d", context.FilteredDiffBytes, len(content))
	}
	if context.RawDiffBytes != context.FilteredDiffBytes {
		t.Fatalf("RawDiffBytes = %d, FilteredDiffBytes = %d, want equal without excludes", context.RawDiffBytes, context.FilteredDiffBytes)
	}
}

func TestWorkingTreeDiffBytesContributeToFilteredBudget(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	writeFile := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	git("init", "-b", "main")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test User")
	writeFile("internal/review/working.go", "package review\n")
	git("add", ".")
	git("commit", "-m", "base")
	base := strings.TrimSpace(commandOutput(t, root, "git", "rev-parse", "HEAD"))

	writeFile("internal/review/staged.go", "package review\n\nvar Staged = `"+strings.Repeat("s", 60_000)+"`\n")
	git("add", "internal/review/staged.go")
	writeFile("internal/review/working.go", "package review\n\nvar Working = `"+strings.Repeat("w", 60_000)+"`\n")

	context, err := Collect(root, base)
	if err != nil {
		t.Fatalf("collect context: %v", err)
	}
	if context.FilteredDiffBytes <= 120_000 {
		t.Fatalf("FilteredDiffBytes = %d, want staged and working diffs to both contribute", context.FilteredDiffBytes)
	}
	if context.RawDiffBytes != context.FilteredDiffBytes {
		t.Fatalf("RawDiffBytes = %d, FilteredDiffBytes = %d, want equal without excludes", context.RawDiffBytes, context.FilteredDiffBytes)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func commandOutput(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s %s: %v", name, strings.Join(args, " "), err)
	}
	return string(out)
}

// diff.noprefix is an ordinary, documented gitconfig setting. With it set, git writes
// `diff --git f.go f.go` and every consumer on this branch keys on a `b/` prefix to find the
// post-image path - so ChangedPaths comes back empty, DiffHasFile is false for every file, and
// each candidate is judged with no evidence. hardenDiff exists to make the payload independent of
// local git configuration; pinning the prefixes is part of that or the guarantee is not delivered.
func TestHardenDiffPinsPathPrefixes(t *testing.T) {
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@x", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@x")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	// The setting under test, in the repository the review will read.
	run("config", "diff.noprefix", "true")
	if err := os.WriteFile(filepath.Join(root, "f.go"), []byte("package f\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "f.go")
	run("commit", "-q", "-m", "base")
	head := exec.Command("git", "rev-parse", "HEAD")
	head.Dir = root
	baseOut, err := head.Output()
	if err != nil {
		t.Fatal(err)
	}
	base := strings.TrimSpace(string(baseOut))
	if err := os.WriteFile(filepath.Join(root, "f.go"), []byte("package f\n\nfunc Added() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "f.go")
	run("commit", "-q", "-m", "change")

	ctx, err := Collect(root, base)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if !strings.Contains(ctx.Diff, "diff --git a/f.go b/f.go") {
		first, _, _ := strings.Cut(ctx.Diff, "\n")
		t.Errorf("diff header must carry a/ and b/ prefixes regardless of diff.noprefix; got %q", first)
	}
}

// gitcontext-1: a bare-directory exclude must match files UNDER it, because git's :(exclude)<dir>
// pathspec is recursive. The exceptions path (exactExcludesExcept) converts each matched changed file
// into an exact :(exclude)<file>; if matchesExclude only did exact matching, a bare "build" exclude
// plus any exception would silently un-exclude the whole build/ directory into the reviewed diff.
func TestMatchesExcludeDirectoryIsRecursive(t *testing.T) {
	if !matchesExclude("build/foo.js", "build") {
		t.Error("a bare directory exclude must match files under it (git :(exclude) is recursive)")
	}
	if !matchesExclude("build/sub/deep.js", "build") {
		t.Error("directory exclude must match nested files")
	}
	if !matchesExclude("build", "build") {
		t.Error("directory exclude must still match the exact path")
	}
	// A file exclude must not over-match a sibling that merely shares its prefix.
	if matchesExclude("config/app.yaml.bak", "config/app.yaml") {
		t.Error("a file exclude must not match a prefix-sharing sibling")
	}
	// Unrelated files and the /** form are unchanged.
	if matchesExclude("other/foo.js", "build") {
		t.Error("unrelated path must not match")
	}
	if !matchesExclude("docs/x/y.md", "docs/x/**") {
		t.Error("the /** recursive form must still match")
	}
}
