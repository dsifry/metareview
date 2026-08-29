package sandbox

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeShow(at map[string]string) ShowFunc {
	return func(rev, path string) ([]byte, bool, error) {
		if b, ok := at[rev+":"+path]; ok {
			return []byte(b), true, nil
		}
		return nil, false, nil
	}
}

func TestMaterializeWritesBothSides(t *testing.T) {
	root := t.TempDir()
	show := fakeShow(map[string]string{
		"H:a/one.go": "after\n", "B:a/one.go": "before\n",
		"H:b/added.go": "new\n", // absent at base: added on the branch
	})
	tree, err := Materialize(root, "B", "H", []string{"b/added.go", "a/one.go"}, show)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	for _, tc := range []struct{ path, want string }{
		{filepath.Join(root, Head, "a/one.go"), "after\n"},
		{filepath.Join(root, Base, "a/one.go"), "before\n"},
		{filepath.Join(root, Head, "b/added.go"), "new\n"},
	} {
		got, err := os.ReadFile(tc.path)
		if err != nil || string(got) != tc.want {
			t.Errorf("%s = %q, %v; want %q", tc.path, got, err, tc.want)
		}
	}
	if _, err := os.Stat(filepath.Join(root, Base, "b/added.go")); !os.IsNotExist(err) {
		t.Error("a file added on the branch must have no base side")
	}
	if tree.Files != 3 {
		t.Errorf("Files = %d, want 3", tree.Files)
	}
}

// The path list derives from a diff, which is data. A path that escapes the root must be
// refused rather than written - the whole point of a sandbox is that it contains.
func TestMaterializeRefusesEscapingPaths(t *testing.T) {
	for _, bad := range []string{"../outside.go", "a/../../up.go", "/etc/passwd", "."} {
		root := t.TempDir()
		if _, err := Materialize(root, "B", "H", []string{bad}, fakeShow(map[string]string{})); err == nil {
			t.Errorf("Materialize(%q) succeeded; want refusal", bad)
		}
	}
}

// TreeHash content-addresses what the judge could read, so a verdict stays replayable once
// the prompt no longer carries the evidence.
func TestTreeHashIsContentAddressed(t *testing.T) {
	files := map[string]string{"H:x.go": "one\n", "B:x.go": "zero\n"}
	a, err := Materialize(t.TempDir(), "B", "H", []string{"x.go"}, fakeShow(files))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Materialize(t.TempDir(), "B", "H", []string{"x.go"}, fakeShow(files))
	if a.TreeHash != b.TreeHash {
		t.Error("same content must give the same tree hash")
	}
	changed := map[string]string{"H:x.go": "ONE\n", "B:x.go": "zero\n"}
	c, _ := Materialize(t.TempDir(), "B", "H", []string{"x.go"}, fakeShow(changed))
	if a.TreeHash == c.TreeHash {
		t.Error("changed content must change the tree hash")
	}
	// The fixture must serve identical bytes at both base revisions, or this passes merely
	// because the base file went missing rather than because the revision is in the hash.
	twoBases := map[string]string{"H:x.go": "one\n", "B:x.go": "zero\n", "B2:x.go": "zero\n"}
	d, _ := Materialize(t.TempDir(), "B2", "H", []string{"x.go"}, fakeShow(twoBases))
	if a.TreeHash == d.TreeHash {
		t.Error("a different base revision must change the tree hash even when the bytes match")
	}
}

// Order of the input paths must not change the hash: the same evidence is the same evidence.
func TestTreeHashIgnoresInputOrder(t *testing.T) {
	files := map[string]string{"H:a.go": "a\n", "H:b.go": "b\n"}
	a, _ := Materialize(t.TempDir(), "B", "H", []string{"a.go", "b.go"}, fakeShow(files))
	b, _ := Materialize(t.TempDir(), "B", "H", []string{"b.go", "a.go"}, fakeShow(files))
	if a.TreeHash != b.TreeHash {
		t.Error("path order must not affect the tree hash")
	}
}

// A judge reads; it must not be able to edit the evidence it is judging.
func TestMaterializedFilesAreReadOnly(t *testing.T) {
	root := t.TempDir()
	if _, err := Materialize(root, "B", "H", []string{"x.go"}, fakeShow(map[string]string{"H:x.go": "v\n"})); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(root, Head, "x.go"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o222 != 0 {
		t.Errorf("mode = %v, want no write bits", fi.Mode().Perm())
	}
}

func TestMaterializeSurfacesShowErrors(t *testing.T) {
	boom := errors.New("git exploded")
	_, err := Materialize(t.TempDir(), "B", "H", []string{"x.go"},
		func(string, string) ([]byte, bool, error) { return nil, false, boom })
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "x.go") {
		t.Errorf("err = %v; want it to wrap the cause and name the path", err)
	}
}

// Filesystem failures must surface, not be swallowed: a half-written sandbox would give the
// judge partial evidence while the tree hash claimed the whole set was available.
func TestMaterializeSurfacesFilesystemErrors(t *testing.T) {
	show := fakeShow(map[string]string{"H:dir/x.go": "v\n"})

	t.Run("root is not a directory", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "notadir")
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Materialize(f, "B", "H", []string{"dir/x.go"}, show); err == nil {
			t.Error("want an error when the destination directory cannot be created")
		}
	})

	t.Run("destination is occupied by a directory", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, Head, "dir", "x.go"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := Materialize(root, "B", "H", []string{"dir/x.go"}, show); err == nil {
			t.Error("want an error when the file cannot be written")
		}
	})
}
