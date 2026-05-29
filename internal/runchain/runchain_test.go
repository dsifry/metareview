package runchain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveStartsFirstAttempt(t *testing.T) {
	root := t.TempDir()
	decision, err := Resolve(root, Options{
		Scope:  "task-done",
		Target: map[string]string{"type": "path", "id": "docs/spec.md"},
	})
	if err != nil {
		t.Fatalf("resolve first attempt: %v", err)
	}
	if decision.AttemptNumber != 1 || decision.MaxAttempts != DefaultMaxAttempts {
		t.Fatalf("unexpected first attempt decision: %+v", decision)
	}
}

func TestResolveFollowsPreviousRunChainAndInheritsRootMax(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, `{"id":"mrv-a","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"NEEDS_REVISION","attemptNumber":1,"maxAttempts":2}`)
	writeRun(t, root, `{"id":"mrv-b","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"NEEDS_REVISION","previousRunId":"mrv-a","attemptNumber":2,"maxAttempts":2}`)
	decision, err := Resolve(root, Options{
		Scope:         "task-done",
		Target:        map[string]string{"type": "path", "id": "docs/spec.md"},
		PreviousRunID: "mrv-b",
	})
	if err != nil {
		t.Fatalf("resolve previous chain: %v", err)
	}
	if decision.AttemptNumber != 3 || decision.MaxAttempts != 2 || len(decision.Chain) != 2 || decision.RootRun.ID != "mrv-a" {
		t.Fatalf("expected inherited third attempt from root chain, got %+v", decision)
	}
}

func TestResolveRejectsMissingPreviousRun(t *testing.T) {
	root := t.TempDir()
	_, err := Resolve(root, Options{
		Scope:         "task-done",
		Target:        map[string]string{"type": "path", "id": "docs/spec.md"},
		PreviousRunID: "mrv-missing",
	})
	if err == nil || !strings.Contains(err.Error(), "previous run mrv-missing not found") {
		t.Fatalf("expected missing previous-run error, got %v", err)
	}
}

func TestResolveRejectsMismatchedPreviousRunTarget(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, `{"id":"mrv-a","scope":"task-done","target":{"type":"path","id":"docs/other.md"},"verdict":"NEEDS_REVISION","attemptNumber":1,"maxAttempts":3}`)
	_, err := Resolve(root, Options{
		Scope:         "task-done",
		Target:        map[string]string{"type": "path", "id": "docs/spec.md"},
		PreviousRunID: "mrv-a",
	})
	if err == nil || !strings.Contains(err.Error(), "does not match task-done docs/spec.md") {
		t.Fatalf("expected mismatched target error, got %v", err)
	}
}

func TestResolveRejectsMismatchedPreviousRunScope(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, `{"id":"mrv-a","scope":"epic-ready","target":{"type":"path","id":"docs/spec.md"},"verdict":"NEEDS_REVISION","attemptNumber":1,"maxAttempts":3}`)
	_, err := Resolve(root, Options{
		Scope:         "task-done",
		Target:        map[string]string{"type": "path", "id": "docs/spec.md"},
		PreviousRunID: "mrv-a",
	})
	if err == nil || !strings.Contains(err.Error(), "does not match task-done docs/spec.md") {
		t.Fatalf("expected mismatched scope error, got %v", err)
	}
}

func TestResolveRejectsMismatchedAncestorTarget(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, `{"id":"mrv-a","scope":"task-done","target":{"type":"path","id":"docs/other.md"},"verdict":"NEEDS_REVISION","attemptNumber":1,"maxAttempts":3}`)
	writeRun(t, root, `{"id":"mrv-b","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"NEEDS_REVISION","previousRunId":"mrv-a","attemptNumber":2,"maxAttempts":3}`)
	_, err := Resolve(root, Options{
		Scope:         "task-done",
		Target:        map[string]string{"type": "path", "id": "docs/spec.md"},
		PreviousRunID: "mrv-b",
	})
	if err == nil || !strings.Contains(err.Error(), "ancestor run mrv-a does not match task-done docs/spec.md") {
		t.Fatalf("expected mismatched ancestor error, got %v", err)
	}
}

func TestResolveRejectsBrokenPreviousRunChain(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, `{"id":"mrv-b","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"NEEDS_REVISION","previousRunId":"mrv-missing","attemptNumber":2,"maxAttempts":3}`)
	_, err := Resolve(root, Options{
		Scope:         "task-done",
		Target:        map[string]string{"type": "path", "id": "docs/spec.md"},
		PreviousRunID: "mrv-b",
	})
	if err == nil || !strings.Contains(err.Error(), "previous run chain missing mrv-missing") {
		t.Fatalf("expected broken chain error, got %v", err)
	}
}

func TestResolveRejectsPreviousRunCycle(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, `{"id":"mrv-a","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"NEEDS_REVISION","previousRunId":"mrv-b","attemptNumber":1,"maxAttempts":3}`)
	writeRun(t, root, `{"id":"mrv-b","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"NEEDS_REVISION","previousRunId":"mrv-a","attemptNumber":2,"maxAttempts":3}`)
	_, err := Resolve(root, Options{
		Scope:         "task-done",
		Target:        map[string]string{"type": "path", "id": "docs/spec.md"},
		PreviousRunID: "mrv-b",
	})
	if err == nil || !strings.Contains(err.Error(), "previous run chain cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestChainToReturnsRootToLeaf(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, `{"id":"mrv-a","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"NEEDS_REVISION","attemptNumber":1,"maxAttempts":3}`)
	writeRun(t, root, `{"id":"mrv-b","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"ESCALATED","previousRunId":"mrv-a","attemptNumber":2,"maxAttempts":3}`)
	records, err := ReadRuns(root)
	if err != nil {
		t.Fatalf("read runs: %v", err)
	}
	chain, err := ChainTo(records, "mrv-b")
	if err != nil {
		t.Fatalf("chain to run: %v", err)
	}
	if len(chain) != 2 || chain[0].ID != "mrv-a" || chain[1].ID != "mrv-b" {
		t.Fatalf("expected root-to-leaf chain, got %+v", chain)
	}
}

func TestResolveRejectsEscalatedPreviousRun(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, `{"id":"mrv-a","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"ESCALATED","attemptNumber":3,"maxAttempts":3}`)
	_, err := Resolve(root, Options{
		Scope:         "task-done",
		Target:        map[string]string{"type": "path", "id": "docs/spec.md"},
		PreviousRunID: "mrv-a",
	})
	if err == nil || !strings.Contains(err.Error(), "already escalated") {
		t.Fatalf("expected escalated previous-run error, got %v", err)
	}
}

func TestResolveRejectsAnyPriorEscalatedSameTargetRestart(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, `{"id":"mrv-a","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"ESCALATED","attemptNumber":3,"maxAttempts":3}`)
	writeRun(t, root, `{"id":"mrv-manual","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"NEEDS_REVISION","attemptNumber":1,"maxAttempts":3}`)
	_, err := Resolve(root, Options{
		Scope:  "task-done",
		Target: map[string]string{"type": "path", "id": "docs/spec.md"},
	})
	if err == nil || !strings.Contains(err.Error(), "same target already escalated in run mrv-a") {
		t.Fatalf("expected older escalated run to block same-target restart, got %v", err)
	}
}

func TestResolveRejectsInvalidMaxAttempts(t *testing.T) {
	root := t.TempDir()
	_, err := Resolve(root, Options{
		Scope:       "task-done",
		Target:      map[string]string{"type": "path", "id": "docs/spec.md"},
		MaxAttempts: -1,
	})
	if err == nil || !strings.Contains(err.Error(), "max attempts must be at least 1") {
		t.Fatalf("expected invalid max attempts error, got %v", err)
	}
}

func writeRun(t *testing.T, root, line string) {
	t.Helper()
	path := filepath.Join(root, ".metareview", "runs.jsonl")
	if err := appendLine(path, line); err != nil {
		t.Fatal(err)
	}
}

func appendLine(path, line string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(line + "\n")
	return err
}
