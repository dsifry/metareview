package mutationfresh

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// gitRepo creates a repository with the given files committed, and returns its root.
func gitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for path, text := range files {
		write(t, root, path, text)
	}
	git(t, root, "init", "-q")
	git(t, root, "config", "user.email", "t@example.com")
	git(t, root, "config", "user.name", "T")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-qm", "init")
	return root
}

func write(t *testing.T, root, path, text string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}
