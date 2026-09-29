package gitcontext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// PathsSince (#161): the paths from..to changes when from is an ancestor of to (a rename counts as both paths); git's
// "no" is false with no error; a commit git does not have is an error, so the caller fails closed.
func TestPathsSince(t *testing.T) {
	root := initRepo(t)
	base, _ := gitReal(root, "rev-parse", "HEAD")
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "a b.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, root, "add", ".")
	runGitCmd(t, root, "commit", "-qm", "doc")
	runGitCmd(t, root, "mv", "f.txt", "g.txt")
	runGitCmd(t, root, "commit", "-qm", "rename")
	head, _ := gitReal(root, "rev-parse", "HEAD")

	paths, ok, err := ChangedSince(root)(base, head)
	if err != nil || !ok || strings.Join(paths, "|") != "docs/a b.md|f.txt|g.txt" {
		t.Fatalf("PathsSince = %q %v %v", paths, ok, err)
	}
	if paths, ok, err := PathsSince(root, head, base); err != nil || ok || paths != nil {
		t.Fatalf("a descendant is not an ancestor: %q %v %v", paths, ok, err)
	}
	if _, _, err := PathsSince(root, strings.Repeat("1", 40), head); err == nil {
		t.Fatal("a commit git does not have must be an error")
	}
	orig := git
	t.Cleanup(func() { git = orig })
	git = func(root string, args ...string) (string, error) {
		if args[0] == "diff" {
			return "", &gitExitError{message: "diff failed", code: 128}
		}
		return orig(root, args...)
	}
	if _, _, err := PathsSince(root, base, head); err == nil {
		t.Fatal("a failing diff must be an error")
	}
}
