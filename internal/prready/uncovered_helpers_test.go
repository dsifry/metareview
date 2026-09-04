package prready

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/gitcontext"
	"github.com/dsifry/metareview/internal/knowledge"
	"github.com/dsifry/metareview/internal/reviewlog"
)

func TestBranchSummary(t *testing.T) {
	cases := []struct {
		name string
		git  gitcontext.Context
		want string
	}{
		{
			name: "no changed files names the branch",
			git:  gitcontext.Context{Branch: "feature"},
			want: "feature has no committed file changes in the reviewed diff.",
		},
		{
			name: "changed files are listed",
			git:  gitcontext.Context{Branch: "feature", ChangedFiles: []string{"a.go", "b.go"}},
			want: "feature changes a.go, b.go",
		},
		{
			name: "empty branch falls back to current branch",
			git:  gitcontext.Context{},
			want: "current branch has no committed file changes in the reviewed diff.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := branchSummary(tc.git); got != tc.want {
				t.Fatalf("branchSummary = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestKnowledgeMarkdown(t *testing.T) {
	// Empty context: both the service-inventory and facts absent branches.
	empty := knowledgeMarkdown(knowledge.Context{})
	for _, want := range []string{"Service inventory: none", "No service inventory found.", "No Beads knowledge facts found."} {
		if !strings.Contains(empty, want) {
			t.Fatalf("empty knowledge markdown missing %q:\n%s", want, empty)
		}
	}

	// Populated context: exercises the inventory-present and facts-present concatenations.
	full := knowledgeMarkdown(knowledge.Context{
		ServiceInventoryPath: "docs/services.yaml",
		ServiceInventory:     "service body",
		Facts: []knowledge.Fact{
			{Source: "beads", Text: "prefer DI"},
			{Source: "readme", Text: "run make test"},
		},
	})
	for _, want := range []string{
		"Service inventory: `docs/services.yaml`",
		"service body",
		"- beads: prefer DI",
		"- readme: run make test",
	} {
		if !strings.Contains(full, want) {
			t.Fatalf("populated knowledge markdown missing %q:\n%s", want, full)
		}
	}
	if strings.Contains(full, "No service inventory found.") || strings.Contains(full, "No Beads knowledge facts found.") {
		t.Fatalf("populated knowledge markdown must not show the empty fallbacks:\n%s", full)
	}
}

func TestClassForDisplay(t *testing.T) {
	cases := []struct {
		name   string
		record findings.Record
		want   string
	}{
		{name: "blocking", record: findings.Record{Classification: "blocking", Severity: "high"}, want: "blocking"},
		{name: "spec-contract blocks regardless of severity", record: findings.Record{Classification: "spec-contract", Severity: "low"}, want: "blocking"},
		{name: "advisory", record: findings.Record{Classification: "advisory", Severity: "low"}, want: "advisory"},
		{name: "follow-up", record: findings.Record{Classification: "follow-up", Severity: "low"}, want: "follow-up"},
		{name: "unknown class is a warning", record: findings.Record{Classification: "novel", Severity: "high"}, want: "warning"},
		{name: "demoted low-severity blocking is a warning", record: findings.Record{Classification: "blocking", Severity: "low"}, want: "warning"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classForDisplay(tc.record); got != tc.want {
				t.Fatalf("classForDisplay = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFindingTargetID(t *testing.T) {
	cases := []struct {
		name   string
		target any
		want   string
	}{
		{name: "map[string]any id wins", target: map[string]any{"id": "task-1", "path": "p"}, want: "task-1"},
		{name: "map[string]any falls back to path", target: map[string]any{"path": "docs/x.md"}, want: "docs/x.md"},
		{name: "map[string]any empty id falls back to path", target: map[string]any{"id": "", "path": "docs/x.md"}, want: "docs/x.md"},
		{name: "map[string]any with neither is empty", target: map[string]any{}, want: ""},
		{name: "map[string]string id wins", target: map[string]string{"id": "task-2", "path": "p"}, want: "task-2"},
		{name: "map[string]string falls back to path", target: map[string]string{"path": "docs/y.md"}, want: "docs/y.md"},
		{name: "unknown type is empty", target: "just a string", want: ""},
		{name: "nil is empty", target: nil, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := findingTargetID(tc.target); got != tc.want {
				t.Fatalf("findingTargetID(%v) = %q, want %q", tc.target, got, tc.want)
			}
		})
	}
}

func TestLogSortKey(t *testing.T) {
	if got := logSortKey(reviewlog.Summary{RunID: "mrv-1", Path: "docs/x.md"}); got != "mrv-1" {
		t.Fatalf("run id present should sort by run id, got %q", got)
	}
	if got := logSortKey(reviewlog.Summary{Path: "docs/x.md"}); got != "docs/x.md" {
		t.Fatalf("no run id should sort by path, got %q", got)
	}
}

func TestLegacyPreviousRunIDsForPRReady(t *testing.T) {
	targetRecord := map[string]string{"type": "branch", "id": "feature"}
	git := gitcontext.Context{Branch: "feature", HeadSHA: "h1"}

	t.Run("recovers a matching pr-ready chain", func(t *testing.T) {
		logs := []reviewlog.Summary{{RunID: "mrv-1", Kind: "pr-ready", Target: "feature", Verdict: "NEEDS_REVISION"}}
		ids, err := legacyPreviousRunIDsForPRReady(t.TempDir(), logs, "mrv-1", targetRecord, git)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ids) != 1 || ids[0] != "mrv-1" {
			t.Fatalf("ids = %v, want [mrv-1]", ids)
		}
	})

	t.Run("no matching previous run yields nothing", func(t *testing.T) {
		logs := []reviewlog.Summary{{RunID: "mrv-1", Kind: "pr-ready", Target: "feature", Verdict: "NEEDS_REVISION"}}
		ids, err := legacyPreviousRunIDsForPRReady(t.TempDir(), logs, "ghost", targetRecord, git)
		if err != nil || ids != nil {
			t.Fatalf("unknown previous run should yield (nil,nil); got ids=%v err=%v", ids, err)
		}
	})

	t.Run("a mismatched target yields nothing", func(t *testing.T) {
		logs := []reviewlog.Summary{{RunID: "mrv-1", Kind: "pr-ready", Target: "other-branch", Verdict: "NEEDS_REVISION"}}
		ids, err := legacyPreviousRunIDsForPRReady(t.TempDir(), logs, "mrv-1", targetRecord, git)
		if err != nil || ids != nil {
			t.Fatalf("a different branch's log must not be adopted; got ids=%v err=%v", ids, err)
		}
	})

	t.Run("a non-pr-ready log yields nothing", func(t *testing.T) {
		logs := []reviewlog.Summary{{RunID: "mrv-1", Kind: "task-done", Target: "feature", Verdict: "NEEDS_REVISION"}}
		ids, err := legacyPreviousRunIDsForPRReady(t.TempDir(), logs, "mrv-1", targetRecord, git)
		if err != nil || ids != nil {
			t.Fatalf("a task-done log must not be adopted as a pr-ready ancestor; got ids=%v err=%v", ids, err)
		}
	})

	t.Run("an escalated ancestor is an error", func(t *testing.T) {
		logs := []reviewlog.Summary{{RunID: "mrv-1", Kind: "pr-ready", Target: "feature", Verdict: "ESCALATED"}}
		ids, err := legacyPreviousRunIDsForPRReady(t.TempDir(), logs, "mrv-1", targetRecord, git)
		if err == nil {
			t.Fatalf("an escalated ancestor must error; got ids=%v", ids)
		}
		if !strings.Contains(err.Error(), "escalated") {
			t.Fatalf("error should explain the escalation, got %v", err)
		}
	})
}

// TestResolveRunChainRecoversLegacyPreviousRun drives the legacy recovery branch of resolveRunChain: the
// run record for the named previous run is absent from runs.jsonl (so runchain.Resolve fails recoverably),
// but the committed review logs still describe the chain, so the ids and continued attempt identity are recovered.
func TestResolveRunChainRecoversLegacyPreviousRun(t *testing.T) {
	root := t.TempDir() // no .metareview/runs.jsonl, so the previous run is "not found" (a recoverable error)
	targetRecord := map[string]string{"type": "branch", "id": "feature"}
	git := gitcontext.Context{Branch: "feature", HeadSHA: "h1"}
	logs := []reviewlog.Summary{{RunID: "mrv-1", Kind: "pr-ready", Target: "feature", Verdict: "NEEDS_REVISION"}}

	chain, previousRunIDs, err := resolveRunChain(root, targetRecord, Options{PreviousRunID: "mrv-1"}, logs, git)
	if err != nil {
		t.Fatalf("legacy recovery should succeed, got %v", err)
	}
	if len(previousRunIDs) != 1 || previousRunIDs[0] != "mrv-1" {
		t.Fatalf("previousRunIDs = %v, want [mrv-1]", previousRunIDs)
	}
	if chain.AttemptNumber != 2 {
		t.Fatalf("fallback chain should continue after its recovered predecessor, got attempt %d", chain.AttemptNumber)
	}
}

// A non-recoverable runchain error must surface as-is, not be swallowed by legacy recovery. This exercises
// BOTH disjuncts of the guard `options.PreviousRunID == "" || !legacyRecoverableRunchainError(err)`.
func TestResolveRunChainSurfacesNonRecoverableErrors(t *testing.T) {
	targetRecord := map[string]string{"type": "branch", "id": "feature"}
	git := gitcontext.Context{Branch: "feature", HeadSHA: "h1"}

	t.Run("no previous run given short-circuits", func(t *testing.T) {
		root := t.TempDir()
		// An escalated run for this target makes a FRESH run (no --previous-run) refuse via runchain; the
		// PreviousRunID=="" disjunct returns the error directly. The run carries no head, so
		// escalatedForTarget matches regardless of the current head.
		writeRunsJSONL(t, root, `{"id":"mrv-esc","scope":"pr-ready","target":{"type":"branch","id":"feature"},"verdict":"ESCALATED"}`)
		if _, _, err := resolveRunChain(root, targetRecord, Options{}, nil, git); err == nil {
			t.Fatal("an escalated same-target run must make a fresh resolveRunChain error")
		}
	})

	t.Run("previous run given but error is not recoverable", func(t *testing.T) {
		root := t.TempDir()
		// The named previous run EXISTS but is itself ESCALATED, so runchain.Resolve fails with
		// "previous run ... already escalated" — which legacyRecoverableRunchainError does NOT match. With a
		// non-empty PreviousRunID the first disjunct is false, so the error surfaces only via the
		// !legacyRecoverableRunchainError disjunct; it must not be routed into legacy recovery.
		writeRunsJSONL(t, root, `{"id":"mrv-1","scope":"pr-ready","target":{"type":"branch","id":"feature"},"verdict":"ESCALATED"}`)
		_, _, err := resolveRunChain(root, targetRecord, Options{PreviousRunID: "mrv-1"}, nil, git)
		if err == nil {
			t.Fatal("an escalated previous run must surface a non-recoverable error")
		}
		if !strings.Contains(err.Error(), "escalated") {
			t.Fatalf("expected the escalation error to surface, got %v", err)
		}
	})
}

func writeRunsJSONL(t *testing.T, root, line string) {
	t.Helper()
	dir := filepath.Join(root, ".metareview")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "runs.jsonl"), []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
