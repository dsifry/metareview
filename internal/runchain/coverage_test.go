package runchain

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveInputValidation(t *testing.T) {
	root := t.TempDir()
	if _, err := Resolve(root, Options{Target: map[string]string{"id": "x"}}); err == nil || !strings.Contains(err.Error(), "scope is required") {
		t.Errorf("missing scope: %v", err)
	}
	if _, err := Resolve(root, Options{Scope: "task-done"}); err == nil || !strings.Contains(err.Error(), "target is required") {
		t.Errorf("missing target: %v", err)
	}
	if _, err := Resolve(root, Options{Scope: "task-done", Target: map[string]string{"id": "x"}, MaxAttempts: -1}); err == nil {
		t.Errorf("negative max attempts should error")
	}
}

func TestResolveReadRunsErrorPropagates(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, "{not json") // corrupt line
	if _, err := Resolve(root, Options{Scope: "task-done", Target: map[string]string{"id": "x"}}); err == nil {
		t.Fatal("a corrupt runs.jsonl should fail Resolve")
	}
}

func TestReadRunsBranches(t *testing.T) {
	// A blank line is skipped; valid records parse with normalized attempt/max.
	root := t.TempDir()
	writeRun(t, root, "")
	writeRun(t, root, `{"id":"mrv-a","scope":"task-done","target":{"id":"x"},"verdict":"PASS"}`)
	records, err := ReadRuns(root)
	if err != nil || len(records) != 1 || records[0].AttemptNumber != 1 || records[0].MaxAttempts != DefaultMaxAttempts {
		t.Fatalf("ReadRuns normalization: %+v %v", records, err)
	}
	// A corrupt line surfaces an unmarshal error.
	root2 := t.TempDir()
	writeRun(t, root2, "{bad")
	if _, err := ReadRuns(root2); err == nil {
		t.Fatal("corrupt line should error")
	}
	// A missing runs.jsonl is not an error.
	if r, err := ReadRuns(t.TempDir()); err != nil || r != nil {
		t.Fatalf("missing file: %+v %v", r, err)
	}
}

func TestReadRunsOpenError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("unreadable-file perms do not hold for root/Windows")
	}
	root := t.TempDir()
	p := filepath.Join(root, ".metareview", "runs.jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{}"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	if _, err := ReadRuns(root); err == nil {
		t.Fatal("an unreadable runs.jsonl should surface an open error")
	}
}

// A previous-run chain is rejected when the same target already escalated in another run at this head.
func TestResolveRejectsEscalatedSibling(t *testing.T) {
	root := t.TempDir()
	tgt := `"target":{"type":"path","id":"docs/spec.md"}`
	writeRun(t, root, `{"id":"mrv-a","scope":"task-done",`+tgt+`,"verdict":"NEEDS_REVISION","attemptNumber":1,"maxAttempts":3,"headSha":"h1"}`)
	writeRun(t, root, `{"id":"mrv-b","scope":"task-done",`+tgt+`,"verdict":"NEEDS_REVISION","previousRunId":"mrv-a","attemptNumber":2,"maxAttempts":3,"headSha":"h1"}`)
	writeRun(t, root, `{"id":"mrv-esc","scope":"task-done",`+tgt+`,"verdict":"ESCALATED","attemptNumber":1,"maxAttempts":3,"headSha":"h1"}`)
	_, err := Resolve(root, Options{Scope: "task-done", Target: map[string]string{"type": "path", "id": "docs/spec.md"}, PreviousRunID: "mrv-b", HeadSHA: "h1"})
	if err == nil || !strings.Contains(err.Error(), "already escalated in run mrv-esc") {
		t.Fatalf("expected the escalated-sibling rejection, got %v", err)
	}
}

// escalatedResetRunIDs falls back to the single record when its previous-run chain is broken.
func TestEscalatedResetWithBrokenChain(t *testing.T) {
	root := t.TempDir()
	tgt := `"target":{"type":"path","id":"docs/spec.md"}`
	// An escalated run at an OLD head whose previous-run pointer is missing: reset must still include it.
	writeRun(t, root, `{"id":"mrv-old","scope":"task-done",`+tgt+`,"verdict":"ESCALATED","previousRunId":"gone","attemptNumber":2,"maxAttempts":3,"headSha":"hOLD"}`)
	dec, err := Resolve(root, Options{Scope: "task-done", Target: map[string]string{"type": "path", "id": "docs/spec.md"}, HeadSHA: "hNEW"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	found := false
	for _, id := range dec.ResetRunIDs {
		if id == "mrv-old" {
			found = true
		}
	}
	if !found {
		t.Fatalf("a broken-chain escalated run should still be reset: %v", dec.ResetRunIDs)
	}
}

func TestTargetIDAndEffectiveMax(t *testing.T) {
	if targetID(map[string]string{"id": "the-id"}) != "the-id" {
		t.Error("targetID id")
	}
	if targetID(map[string]string{"path": "the/path"}) != "the/path" {
		t.Error("targetID path fallback")
	}
	if effectiveMax(0) != DefaultMaxAttempts || effectiveMax(-1) != DefaultMaxAttempts {
		t.Error("effectiveMax non-positive -> default")
	}
	if effectiveMax(5) != 5 {
		t.Error("effectiveMax positive -> value")
	}
}
