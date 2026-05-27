package learning

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureLearningGitPolicyMakesDurableFallbackStateVisible(t *testing.T) {
	root := initGitRepo(t)
	mustWrite(t, filepath.Join(root, ".gitignore"), strings.Join([]string{
		"node_modules/",
		".metareview/",
		"*.log",
		"",
	}, "\n"))

	if err := EnsureLearningGitPolicy(root); err != nil {
		t.Fatalf("EnsureLearningGitPolicy returned error: %v", err)
	}

	assertNotIgnored(t, root, ".metareview/knowledge/metareview.jsonl")
	assertNotIgnored(t, root, ".metareview/calibration.jsonl")
	assertNotIgnored(t, root, ".metareview/learning-runs.jsonl")
	assertIgnored(t, root, ".metareview/runs.jsonl")
	assertIgnored(t, root, ".metareview/findings.jsonl")
	assertIgnored(t, root, ".metareview/knowledge/other.jsonl")
	assertIgnored(t, root, "debug.log")
}

func TestEnsureLearningGitPolicyIsIdempotent(t *testing.T) {
	root := initGitRepo(t)

	if err := EnsureLearningGitPolicy(root); err != nil {
		t.Fatalf("EnsureLearningGitPolicy returned error: %v", err)
	}
	first := readFile(t, filepath.Join(root, ".gitignore"))
	if err := EnsureLearningGitPolicy(root); err != nil {
		t.Fatalf("second EnsureLearningGitPolicy returned error: %v", err)
	}
	second := readFile(t, filepath.Join(root, ".gitignore"))

	if first != second {
		t.Fatalf("git policy should be idempotent.\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestLearningGitPolicyPreservesBeadsKnowledgeWrites(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}

	if path := knowledgeRelPath(root); path != ".beads/knowledge/metareview.jsonl" {
		t.Fatalf("expected Beads knowledge path, got %s", path)
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	runGit(t, root, "config", "user.email", "test-user")
	runGit(t, root, "config", "user.name", "Test User")
	return root
}

func assertIgnored(t *testing.T, root, path string) {
	t.Helper()
	cmd := exec.Command("git", "check-ignore", "--quiet", path)
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		t.Fatalf("expected %s to be ignored, got %v", path, err)
	}
}

func assertNotIgnored(t *testing.T, root, path string) {
	t.Helper()
	cmd := exec.Command("git", "check-ignore", "--quiet", path)
	cmd.Dir = root
	if err := cmd.Run(); err == nil {
		t.Fatalf("expected %s to be git-visible", path)
	}
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(bytes)
}
