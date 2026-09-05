package knowledge

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Collect folds a service inventory and Beads knowledge facts, skipping blanks, non-.jsonl files and
// subdirectories.
func TestCollectHappyPath(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "docs", "SERVICE_INVENTORY.md"), "the inventory\n")
	write(t, filepath.Join(root, ".beads", "knowledge", "a.jsonl"), "\n{\"fact\":\"f-one\"}\nplain line\n")
	write(t, filepath.Join(root, ".beads", "knowledge", "notes.txt"), "ignored\n")
	if err := os.MkdirAll(filepath.Join(root, ".beads", "knowledge", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, err := Collect(root)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if ctx.ServiceInventoryPath == "" || !strings.Contains(ctx.ServiceInventory, "the inventory") {
		t.Errorf("service inventory not collected: %+v", ctx)
	}
	if len(ctx.Facts) != 2 {
		t.Fatalf("expected 2 facts (blank skipped), got %d: %+v", len(ctx.Facts), ctx.Facts)
	}
	if ctx.Facts[0].Text != "f-one" || ctx.Facts[1].Text != "plain line" {
		t.Errorf("fact text: %+v", ctx.Facts)
	}
}

func TestCollectNoBeadsKnowledge(t *testing.T) {
	// No .beads/knowledge -> empty facts, no error.
	ctx, err := Collect(t.TempDir())
	if err != nil || len(ctx.Facts) != 0 {
		t.Fatalf("no knowledge dir: %+v %v", ctx, err)
	}
}

func TestCollectServiceInventoryContainedPathError(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "docs", "SERVICE_INVENTORY.md"), "x\n")
	orig := filepathAbs
	t.Cleanup(func() { filepathAbs = orig })
	filepathAbs = func(string) (string, error) { return "", errors.New("abs boom") }
	if _, err := Collect(root); err == nil || !strings.Contains(err.Error(), "abs boom") {
		t.Fatalf("a containedPath failure on the service inventory should surface: %v", err)
	}
}

func TestCollectServiceInventoryReadError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("unreadable-file perms do not hold for root/Windows")
	}
	root := t.TempDir()
	si := filepath.Join(root, "docs", "SERVICE_INVENTORY.md")
	write(t, si, "x\n")
	if err := os.Chmod(si, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(si, 0o644) })
	if _, err := Collect(root); err == nil {
		t.Fatal("an unreadable service inventory should surface a read error")
	}
}

func TestCollectFactsReadDirError(t *testing.T) {
	root := t.TempDir()
	// .beads/knowledge is a file, so ReadDir errors (not NotExist).
	write(t, filepath.Join(root, ".beads", "knowledge"), "x")
	if _, err := Collect(root); err == nil {
		t.Fatal("a file at .beads/knowledge should surface a ReadDir error")
	}
}

func TestCollectFactsOpenError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("unreadable-file perms do not hold for root/Windows")
	}
	root := t.TempDir()
	f := filepath.Join(root, ".beads", "knowledge", "a.jsonl")
	write(t, f, "{}")
	if err := os.Chmod(f, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(f, 0o644) })
	if _, err := Collect(root); err == nil {
		t.Fatal("an unreadable knowledge file should surface an open error")
	}
}

func TestCollectFactsScannerError(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".beads", "knowledge", "huge.jsonl"), strings.Repeat("a", (1<<20)+16)+"\n")
	if _, err := Collect(root); err == nil {
		t.Fatal("an over-long knowledge line should surface a scanner error")
	}
}

func TestContainedPath(t *testing.T) {
	root := t.TempDir()
	// A path outside the root is rejected.
	if _, err := containedPath(root, filepath.Join("..", "outside")); err == nil || !strings.Contains(err.Error(), "outside repository root") {
		t.Errorf("outside path: %v", err)
	}
	// A candidate that does not exist fails EvalSymlinks.
	if _, err := containedPath(root, "does-not-exist.jsonl"); err == nil {
		t.Error("a nonexistent candidate should fail EvalSymlinks")
	}
	// A nonexistent root fails EvalSymlinks(root).
	if _, err := containedPath(filepath.Join(root, "nope"), "x"); err == nil {
		t.Error("a nonexistent root should fail EvalSymlinks")
	}
	// filepath.Abs failure surfaces (via the seam).
	orig := filepathAbs
	t.Cleanup(func() { filepathAbs = orig })
	calls := 0
	filepathAbs = func(p string) (string, error) {
		calls++
		if calls == 1 {
			return "", errors.New("abs root boom")
		}
		return orig(p)
	}
	if _, err := containedPath(root, "x"); err == nil {
		t.Error("an Abs(root) failure should surface")
	}
	calls = 0
	filepathAbs = func(p string) (string, error) {
		calls++
		if calls == 2 {
			return "", errors.New("abs candidate boom")
		}
		return orig(p)
	}
	if _, err := containedPath(root, "x"); err == nil {
		t.Error("an Abs(candidate) failure should surface")
	}
}

func TestFactText(t *testing.T) {
	// A non-JSON line is returned verbatim.
	if got := factText("just a line"); got != "just a line" {
		t.Errorf("non-JSON: %q", got)
	}
	// A JSON object with a known key returns that value.
	if got := factText(`{"summary":"the summary"}`); got != "the summary" {
		t.Errorf("keyed: %q", got)
	}
	// A JSON object with no known key returns the raw line.
	raw := `{"other":"x"}`
	if got := factText(raw); got != raw {
		t.Errorf("no known key -> raw line: %q", got)
	}
}
