package gitcontext

import (
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
