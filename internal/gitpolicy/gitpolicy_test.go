package gitpolicy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func repo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	run := func(args ...string) {
		c := exec.Command("git", args...)
		c.Dir = root
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	return root
}

func ignored(t *testing.T, root, path string) bool {
	t.Helper()
	c := exec.Command("git", "check-ignore", "--quiet", path)
	c.Dir = root
	return c.Run() == nil
}

// Ensure ignores the ephemeral state and leaves the durable learning state committable.
func TestEnsureAllowlist(t *testing.T) {
	root := repo(t)
	if err := Ensure(root); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{".metareview/runs.jsonl", ".metareview/findings.jsonl", ".metareview/git-hooks", ".metareview/shards/x", ".metareview/knowledge/other.jsonl"} {
		if !ignored(t, root, p) {
			t.Fatalf("ephemeral %q must be ignored", p)
		}
	}
	for _, p := range []string{".metareview/knowledge/metareview.jsonl", ".metareview/calibration.jsonl", ".metareview/learning-runs.jsonl"} {
		if ignored(t, root, p) {
			t.Fatalf("durable %q must remain committable", p)
		}
	}
}

// Ensure is idempotent: a second call produces byte-identical content (no duplication), preserving other lines.
func TestEnsureIdempotentPreservesOtherLines(t *testing.T) {
	root := repo(t)
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("node_modules/\n*.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(root); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err := Ensure(root); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	if string(first) != string(second) {
		t.Fatalf("not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	if strings.Count(string(second), ".metareview/*") != 1 {
		t.Fatalf("block must appear once; got:\n%s", second)
	}
	if !ignored(t, root, "debug.log") || !strings.Contains(string(second), "node_modules/") {
		t.Fatalf("pre-existing entries must be preserved; got:\n%s", second)
	}
}

// A pre-existing BLANKET `.metareview/` ignore is upgraded to the allowlist, making the durable files
// committable again (they were hidden by the blanket rule).
func TestEnsureUpgradesBlanketIgnore(t *testing.T) {
	root := repo(t)
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".metareview/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(root); err != nil {
		t.Fatal(err)
	}
	if ignored(t, root, ".metareview/calibration.jsonl") {
		t.Fatal("a blanket .metareview/ must be upgraded so durable state becomes committable")
	}
	if !ignored(t, root, ".metareview/runs.jsonl") {
		t.Fatal("ephemeral state must still be ignored after the upgrade")
	}
	// The bare blanket line is gone, replaced by the allowlist (`.metareview/*` then the re-includes).
	body, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	for _, line := range strings.Split(string(body), "\n") {
		if line == ".metareview/" {
			t.Fatalf("the bare blanket `.metareview/` must be replaced by the allowlist; got:\n%s", body)
		}
	}
	if !strings.Contains(string(body), ".metareview/*") {
		t.Fatalf("the allowlist block must be present; got:\n%s", body)
	}
}

// A .gitignore that cannot be read (here: it is a DIRECTORY, a non-"not exist" error) must SURFACE the error,
// not silently proceed and clobber it.
func TestEnsureSurfacesUnreadableGitignore(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".gitignore"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(root); err == nil {
		t.Fatal("an unreadable .gitignore (a directory) must return an error, not be silently overwritten")
	}
}

// Ensure works when there is no .gitignore yet (creates it) and ends with a trailing newline.
func TestEnsureCreatesFile(t *testing.T) {
	root := repo(t)
	if err := Ensure(root); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatalf("Ensure must create .gitignore: %v", err)
	}
	if !strings.HasPrefix(string(body), Marker) || !strings.HasSuffix(string(body), "\n") {
		t.Fatalf("fresh .gitignore must be exactly the block with a trailing newline; got %q", body)
	}
}
