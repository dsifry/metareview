package mutationfresh

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestWorktreeReadsFilesSymlinksAndAbsence(t *testing.T) {
	root := gitRepo(t, map[string]string{"a.txt": "a", "dir/b.txt": "b"})
	if err := os.Symlink("a.txt", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	w := Worktree(root)
	if w.Head() {
		t.Error("the working tree is not HEAD mode")
	}
	got, err := w.Read([]string{"a.txt", "link", "missing", "dir", "a.txt/under-a-file"})
	if err != nil {
		t.Fatal(err)
	}
	if string(got["a.txt"].Data) != "a" || got["a.txt"].Symlink {
		t.Errorf("a.txt = %+v", got["a.txt"])
	}
	if string(got["link"].Data) != "a.txt" || !got["link"].Symlink {
		t.Errorf("link = %+v", got["link"])
	}
	for _, absent := range []string{"missing", "dir", "a.txt/under-a-file"} {
		if _, ok := got[absent]; ok {
			t.Errorf("%s must be absent (missing, a directory, or under a file)", absent)
		}
	}
}

func TestWorktreeReadErrorsStopTheReview(t *testing.T) {
	root := gitRepo(t, map[string]string{"locked/a.txt": "a", "b.txt": "b"})
	if err := os.Chmod(filepath.Join(root, "b.txt"), 0o000); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root, "locked"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "locked"), 0o755) })
	if _, err := Worktree(root).Read([]string{"b.txt"}); err == nil {
		t.Error("an unreadable file stops the review")
	}
	if _, err := Worktree(root).Read([]string{"locked/a.txt"}); err == nil {
		t.Error("an unreadable directory stops the review")
	}
}

func TestWorktreePathsListsTrackedAndUntrackedButNotNestedRepositories(t *testing.T) {
	root := gitRepo(t, map[string]string{"a.txt": "a", ".gitignore": "ignored.txt\n"})
	write(t, root, "new.txt", "n")
	write(t, root, "ignored.txt", "i")
	nested := filepath.Join(root, "sub")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, nested, "init", "-q")
	got, err := Worktree(root).Paths()
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	if !slices.Equal(got, []string{".gitignore", "a.txt", "new.txt"}) {
		t.Errorf("paths = %v", got)
	}
	if _, err := Worktree(t.TempDir()).Paths(); err == nil {
		t.Error("outside a repository is an error")
	}
}

// §6.2 and §7.2: HEAD mode reads the committed content through the repository's filters, sees
// symlinks as link text, excludes gitlinks, and ignores uncommitted edits.
func TestHeadContentReadsCommittedContentThroughFilters(t *testing.T) {
	root := gitRepo(t, map[string]string{".gitattributes": "*.txt eol=crlf\n", "a.txt": "one\ntwo\n", "b.bin": "b"})
	if err := os.Symlink("a.txt", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	sha := git(t, root, "rev-parse", "HEAD")
	git(t, root, "update-index", "--add", "--cacheinfo", "160000,"+sha[:40]+",gitlink")
	git(t, root, "add", "link")
	git(t, root, "commit", "-qm", "link and gitlink")
	write(t, root, "b.bin", "uncommitted")
	h := Head(root)
	if !h.Head() {
		t.Error("HEAD mode")
	}
	got, err := h.Read([]string{"a.txt", "b.bin", "link", "gitlink", "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if string(got["a.txt"].Data) != "one\r\ntwo\r\n" {
		t.Errorf("a.txt through eol=crlf = %q", got["a.txt"].Data)
	}
	if string(got["b.bin"].Data) != "b" {
		t.Errorf("b.bin must be the committed content, got %q", got["b.bin"].Data)
	}
	if string(got["link"].Data) != "a.txt" || !got["link"].Symlink {
		t.Errorf("link = %+v", got["link"])
	}
	for _, absent := range []string{"gitlink", "missing"} {
		if _, ok := got[absent]; ok {
			t.Errorf("%s is absent in HEAD mode", absent)
		}
	}
	paths, err := h.Paths()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(paths, []string{".gitattributes", "a.txt", "b.bin", "link"}) {
		t.Errorf("HEAD paths = %v (gitlinks excluded, sorted)", paths)
	}
	if none, err := h.Read([]string{"missing"}); err != nil || len(none) != 0 {
		t.Errorf("reading only absent paths runs nothing: %v %v", none, err)
	}
}

func TestHeadContentErrors(t *testing.T) {
	if _, err := Head(t.TempDir()).Paths(); err == nil {
		t.Error("outside a repository is an error")
	}
	if _, err := Head(t.TempDir()).Read([]string{"a"}); err == nil {
		t.Error("outside a repository is an error")
	}
	root := gitRepo(t, map[string]string{"a.txt": "a"})
	saved := runGit
	t.Cleanup(func() { runGit = saved })
	runGit = func(dir string, stdin []byte, args ...string) ([]byte, error) {
		if args[0] == "cat-file" {
			return nil, errors.New("cat-file failed")
		}
		return saved(dir, stdin, args...)
	}
	if _, err := Head(root).Read([]string{"a.txt"}); err == nil {
		t.Error("a cat-file failure stops the review")
	}
}
