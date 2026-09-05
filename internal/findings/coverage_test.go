package findings

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeFindings(t *testing.T, root, content string) string {
	t.Helper()
	p := filepath.Join(root, ".metareview", "findings.jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// --- readJSONL ---

func TestReadJSONLBranches(t *testing.T) {
	// Missing file -> empty, no error.
	if recs, err := readJSONL(filepath.Join(t.TempDir(), "none.jsonl")); err != nil || len(recs) != 0 {
		t.Fatalf("missing file: %v %v", recs, err)
	}
	// Blank lines skipped; a valid record parses.
	p := filepath.Join(t.TempDir(), "f.jsonl")
	if err := os.WriteFile(p, []byte("\n{\"id\":\"a\"}\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if recs, err := readJSONL(p); err != nil || len(recs) != 1 || recs[0].ID != "a" {
		t.Fatalf("valid: %v %v", recs, err)
	}
	// A corrupt line -> unmarshal error.
	p2 := filepath.Join(t.TempDir(), "bad.jsonl")
	if err := os.WriteFile(p2, []byte("{not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readJSONL(p2); err == nil {
		t.Fatal("corrupt line should error")
	}
	// An over-long line -> scanner error.
	p3 := filepath.Join(t.TempDir(), "huge.jsonl")
	if err := os.WriteFile(p3, []byte(strings.Repeat("a", (1<<20)+16)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readJSONL(p3); err == nil {
		t.Fatal("over-long line should surface a scanner error")
	}
}

func TestReadJSONLOpenError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("unreadable-file perms do not hold for root/Windows")
	}
	p := filepath.Join(t.TempDir(), "f.jsonl")
	if err := os.WriteFile(p, []byte("{}"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	if _, err := readJSONL(p); err == nil {
		t.Fatal("unreadable file should surface an open error")
	}
}

// --- writeJSONL ---

func TestWriteJSONLErrors(t *testing.T) {
	// MkdirAll error: a parent segment is a file.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONL(filepath.Join(blocker, "sub", "f.jsonl"), []Record{{ID: "a"}}); err == nil {
		t.Fatal("a file parent should fail MkdirAll")
	}
	// json.Marshal error: a Record with an unmarshalable Target.
	if err := writeJSONL(filepath.Join(t.TempDir(), "f.jsonl"), []Record{{ID: "a", Target: make(chan int)}}); err == nil {
		t.Fatal("an unmarshalable record should fail json.Marshal")
	}
}

// --- sameTarget / firstTarget ---

func TestSameTargetAndFirstTarget(t *testing.T) {
	if !sameTarget(map[string]string{"id": "x"}, map[string]string{"id": "x"}) {
		t.Error("equal targets")
	}
	// Unmarshalable a -> false; unmarshalable b -> false.
	if sameTarget(make(chan int), "x") {
		t.Error("unmarshalable a should be false")
	}
	if sameTarget("x", make(chan int)) {
		t.Error("unmarshalable b should be false")
	}
	if firstTarget(nil, "fallback") != "fallback" {
		t.Error("nil record target -> fallback")
	}
	if firstTarget("real", "fallback") != "real" {
		t.Error("non-nil record target -> itself")
	}
}

func TestClassForCountAndFirstNonEmpty(t *testing.T) {
	if classForCount("blocking", "low") != "warning" {
		t.Error("a low-severity blocking finding classifies as warning")
	}
	if classForCount("blocking", "high") != "blocking" {
		t.Error("a high-severity blocking finding classifies as blocking")
	}
	if firstNonEmpty("", "  ") != "" {
		t.Error("all-blank firstNonEmpty -> empty")
	}
}

// --- backupOnce ---

func TestBackupOnce(t *testing.T) {
	// Missing source -> no-op, no error.
	if err := backupOnce(filepath.Join(t.TempDir(), "gone.jsonl")); err != nil {
		t.Fatalf("missing source: %v", err)
	}
	// A pre-existing backup -> no-op.
	dir := t.TempDir()
	src := filepath.Join(dir, "f.jsonl")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src+".pre-0.8.3.bak", []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := backupOnce(src); err != nil {
		t.Fatalf("existing backup: %v", err)
	}
	if got, _ := os.ReadFile(src + ".pre-0.8.3.bak"); string(got) != "old" {
		t.Error("an existing backup must not be overwritten")
	}
	// A fresh backup is written.
	dir2 := t.TempDir()
	src2 := filepath.Join(dir2, "f.jsonl")
	if err := os.WriteFile(src2, []byte("current"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := backupOnce(src2); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(src2 + ".pre-0.8.3.bak"); string(got) != "current" {
		t.Errorf("fresh backup content: %q", got)
	}
}

func TestBackupOnceReadError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("unreadable-file perms do not hold for root/Windows")
	}
	src := filepath.Join(t.TempDir(), "f.jsonl")
	if err := os.WriteFile(src, []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(src, 0o644) })
	if err := backupOnce(src); err == nil {
		t.Fatal("an unreadable source should surface a read error")
	}
}

// --- RenderIndex / UnresolvedBlocking read errors ---

func TestRenderIndexAndUnresolvedReadErrors(t *testing.T) {
	root := t.TempDir()
	writeFindings(t, root, "{not json\n")
	if err := RenderIndex(root); err == nil {
		t.Error("RenderIndex should surface a corrupt-findings read error")
	}
	if _, err := UnresolvedBlocking(root); err == nil {
		t.Error("UnresolvedBlocking should surface a corrupt-findings read error")
	}
}

func TestRenderIndexWithRecordsMkdirError(t *testing.T) {
	root := t.TempDir()
	// docs is a file, so MkdirAll(docs/metareview) fails.
	if err := os.WriteFile(filepath.Join(root, "docs"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RenderIndexWithRecords(root, []Record{{ID: "a", Status: "open", Classification: "blocking", Severity: "high"}}); err == nil {
		t.Fatal("a docs file should fail the index MkdirAll")
	}
}

// --- Reconcile error branches ---

func TestReconcileReadError(t *testing.T) {
	root := t.TempDir()
	writeFindings(t, root, "{not json\n")
	if _, err := Reconcile(root, Run{ID: "r", Scope: "task-done", Target: map[string]string{"id": "x"}}, nil, Options{}); err == nil {
		t.Fatal("a corrupt ledger should fail Reconcile")
	}
}

func TestReconcileRenderIndexError(t *testing.T) {
	root := t.TempDir()
	// A clean ledger so read+write succeed, but docs is a file so the index render fails.
	writeFindings(t, root, "")
	if err := os.WriteFile(filepath.Join(root, "docs"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Reconcile(root, Run{ID: "r", Scope: "task-done", Target: map[string]string{"id": "x"}},
		[]Input{{Fingerprint: "fp1", Classification: "blocking", Severity: "high", Title: "t"}}, Options{})
	if err == nil {
		t.Fatal("a docs file should fail Reconcile's index render")
	}
}

func TestReconcileWriteAndBackupErrors(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("read-only-dir perms do not hold for root/Windows")
	}
	// writeJSONL error: the findings.jsonl file itself is read-only, so the rewrite (after a
	// successful read of the same file) fails on the truncating open.
	root := t.TempDir()
	p := writeFindings(t, root, `{"id":"a","status":"open"}`+"\n")
	if err := os.Chmod(p, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	if _, err := Reconcile(root, Run{ID: "r", Scope: "task-done", Target: map[string]string{"id": "x"}}, nil, Options{}); err == nil {
		t.Fatal("a read-only findings.jsonl should fail Reconcile's write")
	}
	_ = os.Chmod(p, 0o644)

	// supersedeLegacyContextRisk backup error: a legacy context-risk row present + a read-only
	// .metareview makes backupOnce's write fail.
	root2 := t.TempDir()
	mrv2 := filepath.Join(root2, ".metareview")
	writeFindings(t, root2, `{"id":"a","status":"open","scope":"task-done","target":{"id":"x"},"fingerprint":"architecture:context-risk:foo"}`+"\n")
	if err := os.Chmod(mrv2, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(mrv2, 0o755) })
	if _, err := Reconcile(root2, Run{ID: "r", Scope: "task-done", Target: map[string]string{"id": "x"}}, nil, Options{}); err == nil {
		t.Fatal("a read-only .metareview should fail the legacy-row backup")
	}
}
