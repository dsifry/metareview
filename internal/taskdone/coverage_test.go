package taskdone

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/contextprofile"
	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/gitcontext"
	"github.com/dsifry/metareview/internal/knowledge"
	"github.com/dsifry/metareview/internal/reviewers"
	"github.com/dsifry/metareview/internal/runchain"
	"github.com/dsifry/metareview/internal/tasksource"
)

// smallTaskRepo builds a minimal git repo (config isolated for determinism) with a base commit and a
// feature branch carrying a small task file and one small change — resolvable by task-done, and
// under the shard cap so no shard plan is produced.
func smallTaskRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	env := append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_OPTIONAL_LOCKS=0", "GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=gc.auto", "GIT_CONFIG_VALUE_0=0", "GIT_CONFIG_KEY_1=maintenance.auto", "GIT_CONFIG_VALUE_1=false")
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")
	write("seed.txt", "seed\n")
	run("add", ".")
	run("commit", "-m", "initial")
	run("checkout", "-q", "-b", "feature")
	write("docs/tasks/small.md", "# Small task\n\nA small change.\n")
	write("src/small.go", "package src\n\nvar X = 1\n")
	run("add", "-A")
	run("commit", "-m", "small change")
	return root
}

const smallTarget = "docs/tasks/small.md"

// --- pure helpers ---

func TestFilterGeneratedFiles(t *testing.T) {
	got := filterGeneratedFiles([]string{".metareview/runs.jsonl", "docs/metareview/x.md", "src/a.go"})
	if len(got) != 1 || got[0] != "src/a.go" {
		t.Fatalf("expected only src/a.go to survive, got %v", got)
	}
}

func TestIsGeneratedDiffSection(t *testing.T) {
	if isGeneratedDiffSection("diff --git only-three") {
		t.Error("a header with fewer than 4 fields must not be treated as generated")
	}
	if !isGeneratedDiffSection("diff --git a/.metareview/runs.jsonl b/.metareview/runs.jsonl") {
		t.Error("a .metareview diff section must be treated as generated")
	}
	if isGeneratedDiffSection("diff --git a/src/a.go b/src/a.go") {
		t.Error("a source diff section must not be treated as generated")
	}
}

func TestGeneratedTargetExceptions(t *testing.T) {
	if got := generatedTargetExceptions("docs/metareview/x.md"); len(got) != 1 || got[0] != "docs/metareview/x.md" {
		t.Fatalf("expected the metareview path returned as an exception, got %v", got)
	}
	if got := generatedTargetExceptions("docs/tasks/a.md"); got != nil {
		t.Fatalf("expected nil for a non-metareview path, got %v", got)
	}
}

func TestReviewerKnowledgeMapsFacts(t *testing.T) {
	ctx := knowledge.Context{
		ServiceInventoryPath: "docs/SERVICE_INVENTORY.md",
		ServiceInventory:     "inv",
		Facts:                []knowledge.Fact{{Source: "k.jsonl", Text: "fact"}},
	}
	got := reviewerKnowledge(ctx)
	if got.ServiceInventoryPath != "docs/SERVICE_INVENTORY.md" || got.ServiceInventory != "inv" {
		t.Fatalf("service inventory not carried: %+v", got)
	}
	if len(got.Facts) != 1 || got.Facts[0].Source != "k.jsonl" || got.Facts[0].Text != "fact" {
		t.Fatalf("facts not mapped: %+v", got.Facts)
	}
}

func TestReadEvidence(t *testing.T) {
	if got, err := readEvidence(""); err != nil || got != "" {
		t.Fatalf("empty path should return empty, no error; got %q, %v", got, err)
	}
	if _, err := readEvidence(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("a missing evidence file must surface an error")
	}
	big := filepath.Join(t.TempDir(), "big.txt")
	if err := os.WriteFile(big, []byte(strings.Repeat("x", 15000)), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readEvidence(big)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 12000 {
		t.Fatalf("evidence should be truncated to 12000 bytes, got %d", len(got))
	}
}

func TestUniquePathsBumpsOnCollision(t *testing.T) {
	root := t.TempDir()
	at := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	runID1, _, reviewRel1 := uniquePaths(root, smallTarget, at)
	// Materialise the first review log so the next call at the same instant must bump.
	p := filepath.Join(root, filepath.FromSlash(reviewRel1))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	runID2, _, _ := uniquePaths(root, smallTarget, at)
	if runID1 == runID2 {
		t.Fatalf("collision not resolved: both runs share ID %s", runID1)
	}
}

func TestKnowledgeMarkdown(t *testing.T) {
	full := knowledgeMarkdown(knowledge.Context{
		ServiceInventoryPath: "docs/SERVICE_INVENTORY.md",
		ServiceInventory:     "the-inventory",
		Facts:                []knowledge.Fact{{Source: "k.jsonl", Text: "a-fact"}},
	})
	if !strings.Contains(full, "docs/SERVICE_INVENTORY.md") || !strings.Contains(full, "the-inventory") {
		t.Errorf("service inventory branch not rendered: %s", full)
	}
	if !strings.Contains(full, "k.jsonl: a-fact") {
		t.Errorf("facts branch not rendered: %s", full)
	}
	empty := knowledgeMarkdown(knowledge.Context{})
	if !strings.Contains(empty, "No service inventory found.") || !strings.Contains(empty, "No Beads knowledge facts found.") {
		t.Errorf("empty context should report both absences: %s", empty)
	}
}

func TestVerdictForCounts(t *testing.T) {
	// ESCALATED: blocking remains at the last attempt.
	if v, _, blk, reason := verdictForCounts(findings.ClassCounts{Blocking: 1}, "gate", 3, 3); v != "ESCALATED" || !blk || reason == "" {
		t.Fatalf("expected ESCALATED with a reason, got %q blk=%v reason=%q", v, blk, reason)
	}
	// NEEDS_REVISION: blocking with attempts left.
	if v, _, blk, _ := verdictForCounts(findings.ClassCounts{Blocking: 1}, "gate", 1, 3); v != "NEEDS_REVISION" || !blk {
		t.Fatalf("expected NEEDS_REVISION, got %q blk=%v", v, blk)
	}
	// PASS_ADVISORY: advisory-only under a real gate.
	if v, _, blk, _ := verdictForCounts(findings.ClassCounts{Advisory: 1}, "gate", 1, 3); v != "PASS_ADVISORY" || blk {
		t.Fatalf("expected PASS_ADVISORY, got %q blk=%v", v, blk)
	}
	// PASS: clean under a real gate.
	if v, _, blk, _ := verdictForCounts(findings.ClassCounts{}, "gate", 1, 3); v != "PASS" || blk {
		t.Fatalf("expected PASS, got %q blk=%v", v, blk)
	}
}

func TestRestoreSnapshots(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(existing, []byte("current"), 0o644); err != nil {
		t.Fatal(err)
	}
	created := filepath.Join(dir, "sub", "new.txt")
	if err := os.MkdirAll(filepath.Dir(created), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(created, []byte("should be removed"), 0o644); err != nil {
		t.Fatal(err)
	}
	restoreSnapshots(map[string]fileSnapshot{
		existing: {existed: true, content: []byte("ORIGINAL")},
		created:  {existed: false},
	})
	got, err := os.ReadFile(existing)
	if err != nil || string(got) != "ORIGINAL" {
		t.Fatalf("existing file not restored to original: %q, %v", got, err)
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Fatalf("a file absent in the snapshot must be removed, stat err=%v", err)
	}
}

func TestMarkdownList(t *testing.T) {
	if got := markdownList(nil, "EMPTY"); got != "EMPTY" {
		t.Errorf("nil list should be the empty sentinel, got %q", got)
	}
	if got := markdownList([]string{"a", "", "a", "b"}, "EMPTY"); got != "- a\n- b" {
		t.Errorf("dedup/skip-empty failed: %q", got)
	}
	if got := markdownList([]string{"", ""}, "EMPTY"); got != "EMPTY" {
		t.Errorf("all-empty values should collapse to the sentinel, got %q", got)
	}
}

func TestSourceRefs(t *testing.T) {
	withPath := sourceRefs(tasksource.Source{Kind: "markdown", ID: "id1", Path: "docs/tasks/a.md"})
	if len(withPath) != 1 || withPath[0]["path"] != "docs/tasks/a.md" || withPath[0]["type"] != "markdown" {
		t.Fatalf("path-form source ref wrong: %v", withPath)
	}
	withID := sourceRefs(tasksource.Source{Kind: "beads", ID: "bd-1"})
	if len(withID) != 1 || withID[0]["id"] != "bd-1" || withID[0]["type"] != "beads" {
		t.Fatalf("id-form source ref wrong: %v", withID)
	}
}

func TestTaskTargetType(t *testing.T) {
	cases := map[string]string{"beads": "beads-task", "markdown": "path", "something-else": "advisory"}
	for kind, want := range cases {
		if got := taskTargetType(tasksource.Source{Kind: kind}); got != want {
			t.Errorf("taskTargetType(%q) = %q, want %q", kind, got, want)
		}
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "  ", "x", "y"); got != "x" {
		t.Errorf("expected first non-blank 'x', got %q", got)
	}
	if got := firstNonEmpty("", "  "); got != "" {
		t.Errorf("all-blank should return empty, got %q", got)
	}
}

// --- Create error branches (deterministic seams + fixtures) ---

func TestCreateTaskResolveError(t *testing.T) {
	root := smallTaskRepo(t)
	if _, err := Create(root, "docs/tasks/does-not-exist.md", Options{Base: "main"}); err == nil {
		t.Fatal("expected an error for an unresolvable target")
	}
}

func TestCreatePlanShardsError(t *testing.T) {
	root := smallTaskRepo(t)
	orig := planShards
	t.Cleanup(func() { planShards = orig })
	sentinel := errors.New("plan boom")
	planShards = func(contextprofile.Profile, []gitcontext.BranchFile, contextprofile.ShardOptions) (contextprofile.ShardPlan, error) {
		return contextprofile.ShardPlan{}, sentinel
	}
	if _, err := Create(root, smallTarget, Options{Base: "main"}); !errors.Is(err, sentinel) {
		t.Fatalf("expected the plan-shards error, got %v", err)
	}
}

func TestCreateKnowledgeError(t *testing.T) {
	root := smallTaskRepo(t)
	orig := collectKnowledge
	t.Cleanup(func() { collectKnowledge = orig })
	sentinel := errors.New("knowledge boom")
	collectKnowledge = func(string) (knowledge.Context, error) { return knowledge.Context{}, sentinel }
	if _, err := Create(root, smallTarget, Options{Base: "main"}); !errors.Is(err, sentinel) {
		t.Fatalf("expected the knowledge error, got %v", err)
	}
}

func TestCreateEvidenceError(t *testing.T) {
	root := smallTaskRepo(t)
	if _, err := Create(root, smallTarget, Options{Base: "main", EvidencePath: filepath.Join(root, "nope.md")}); err == nil {
		t.Fatal("expected an error for a missing evidence file")
	}
}

func TestCreateDiscoverError(t *testing.T) {
	root := shardedTaskRepo(t)
	writer := &fakeWriter{discoverErr: errors.New("discover boom")}
	if _, err := Create(root, "docs/tasks/big-task.md", Options{Base: "main", ShardWriter: writer}); err == nil || !strings.Contains(err.Error(), "discover boom") {
		t.Fatalf("expected the discover error, got %v", err)
	}
}

func TestCreateMutationReportError(t *testing.T) {
	root := smallTaskRepo(t)
	if _, err := Create(root, smallTarget, Options{Base: "main", MutationReportPaths: []string{filepath.Join(root, "missing-report.json")}}); err == nil {
		t.Fatal("expected an error for an unreadable mutation report")
	}
}

func TestCreateMkdirContextError(t *testing.T) {
	root := smallTaskRepo(t)
	orig := mkdirAll
	t.Cleanup(func() { mkdirAll = orig })
	sentinel := errors.New("mkdir context boom")
	mkdirAll = func(path string, perm os.FileMode) error {
		if strings.Contains(path, filepath.Join("metareview", "context")) {
			return sentinel
		}
		return orig(path, perm)
	}
	if _, err := Create(root, smallTarget, Options{Base: "main"}); !errors.Is(err, sentinel) {
		t.Fatalf("expected the context-mkdir error, got %v", err)
	}
}

func TestCreateMkdirReviewsError(t *testing.T) {
	root := smallTaskRepo(t)
	orig := mkdirAll
	t.Cleanup(func() { mkdirAll = orig })
	sentinel := errors.New("mkdir reviews boom")
	mkdirAll = func(path string, perm os.FileMode) error {
		if strings.Contains(path, filepath.Join("metareview", "reviews")) {
			return sentinel
		}
		return orig(path, perm)
	}
	if _, err := Create(root, smallTarget, Options{Base: "main"}); !errors.Is(err, sentinel) {
		t.Fatalf("expected the reviews-mkdir error, got %v", err)
	}
}

// A write failure on the context pack triggers the rollback path: with no shard plan the pack
// rollback is the default no-op closure, so this also exercises that closure and restoreSnapshots.
func TestCreateWriteContextErrorRollsBack(t *testing.T) {
	root := smallTaskRepo(t)
	orig := writeFile
	t.Cleanup(func() { writeFile = orig })
	sentinel := errors.New("write context boom")
	writeFile = func(path string, data []byte, perm os.FileMode) error {
		if strings.Contains(path, filepath.Join("metareview", "context")) {
			return sentinel
		}
		return orig(path, data, perm)
	}
	if _, err := Create(root, smallTarget, Options{Base: "main"}); !errors.Is(err, sentinel) {
		t.Fatalf("expected the context-write error, got %v", err)
	}
}

func TestCreateResolveChainError(t *testing.T) {
	root := smallTaskRepo(t)
	orig := resolveChain
	t.Cleanup(func() { resolveChain = orig })
	sentinel := errors.New("chain boom")
	resolveChain = func(string, runchain.Options) (runchain.Decision, error) {
		return runchain.Decision{}, sentinel
	}
	if _, err := Create(root, smallTarget, Options{Base: "main"}); !errors.Is(err, sentinel) {
		t.Fatalf("expected the resolve-chain error, got %v", err)
	}
}

func TestCreateReconcileError(t *testing.T) {
	root := smallTaskRepo(t)
	orig := reconcileFindings
	t.Cleanup(func() { reconcileFindings = orig })
	sentinel := errors.New("reconcile boom")
	reconcileFindings = func(string, findings.Run, []findings.Input, findings.Options) (findings.Result, error) {
		return findings.Result{}, sentinel
	}
	if _, err := Create(root, smallTarget, Options{Base: "main"}); !errors.Is(err, sentinel) {
		t.Fatalf("expected the reconcile error, got %v", err)
	}
}

func TestCreateAppendRunError(t *testing.T) {
	root := smallTaskRepo(t)
	orig := appendJSONL
	t.Cleanup(func() { appendJSONL = orig })
	sentinel := errors.New("append boom")
	appendJSONL = func(string, any) error { return sentinel }
	if _, err := Create(root, smallTarget, Options{Base: "main"}); !errors.Is(err, sentinel) {
		t.Fatalf("expected the append error, got %v", err)
	}
}

func TestCreateWriteReviewError(t *testing.T) {
	root := smallTaskRepo(t)
	orig := writeFile
	t.Cleanup(func() { writeFile = orig })
	sentinel := errors.New("write review boom")
	writeFile = func(path string, data []byte, perm os.FileMode) error {
		if strings.Contains(path, filepath.Join("metareview", "reviews")) {
			return sentinel
		}
		return orig(path, data, perm)
	}
	if _, err := Create(root, smallTarget, Options{Base: "main"}); !errors.Is(err, sentinel) {
		t.Fatalf("expected the review-write error, got %v", err)
	}
}

// When a real shard pack was written and then the review fails, Create runs the pack rollback; if
// that rollback ALSO fails, both errors are joined rather than the rollback masking the cause.
func TestCreateRollbackErrorIsJoined(t *testing.T) {
	root := shardedTaskRepo(t)
	writer := &fakeWriter{satisfy: true, rollbackErr: errors.New("rollback boom")}
	orig := writeFile
	t.Cleanup(func() { writeFile = orig })
	sentinel := errors.New("write context boom")
	writeFile = func(path string, data []byte, perm os.FileMode) error {
		if strings.Contains(path, filepath.Join("metareview", "context")) {
			return sentinel
		}
		return orig(path, data, perm)
	}
	_, err := Create(root, "docs/tasks/big-task.md", Options{Base: "main", ShardWriter: writer})
	if !errors.Is(err, sentinel) {
		t.Fatalf("the causing error must be preserved, got %v", err)
	}
	if !strings.Contains(err.Error(), "rollback boom") {
		t.Fatalf("the rollback error must be joined in, got %v", err)
	}
	if writer.rollbacks == 0 {
		t.Fatal("the pack rollback was never run")
	}
}

// A repo with Beads capability makes the gate effect "gate" rather than "advisory". This also drives
// the nil-ShardWriter default (a real writer) and the previous-run chain link.
func TestCreateBeadsGateAndPreviousRunChain(t *testing.T) {
	root := smallTaskRepo(t)
	if err := os.MkdirAll(filepath.Join(root, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".beads", "issues.jsonl"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	first, err := Create(root, smallTarget, Options{Base: "main", Now: at})
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	// The context pack records the gate effect for a Beads repo.
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(first.ContextRel)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Gate effect: `gate`") {
		t.Errorf("expected gate effect 'gate' in a Beads repo:\n%s", body)
	}
	// A second run naming the first as previous exercises the run-chain link append.
	if _, err := Create(root, smallTarget, Options{Base: "main", Now: at.Add(time.Hour), PreviousRunID: first.RunID, MaxAttempts: 3}); err != nil {
		t.Fatalf("second Create with previous run: %v", err)
	}
}

// A target that is itself a generated metareview path takes the exceptions branch (no filtering).
func TestCreateMetareviewTargetException(t *testing.T) {
	root := smallTaskRepo(t)
	// Add a committed doc under docs/metareview so it resolves as a markdown task on the branch.
	env := append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_OPTIONAL_LOCKS=0", "GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=gc.auto", "GIT_CONFIG_VALUE_0=0", "GIT_CONFIG_KEY_1=maintenance.auto", "GIT_CONFIG_VALUE_1=false")
	p := filepath.Join(root, "docs", "metareview", "note.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("# note\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-m", "add metareview note"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if _, err := Create(root, "docs/metareview/note.md", Options{Base: "main"}); err != nil {
		t.Fatalf("Create with a metareview target: %v", err)
	}
}

var _ = reviewers.KnowledgeContext{}

func TestCreateGitContextError(t *testing.T) {
	root := smallTaskRepo(t)
	if _, err := Create(root, smallTarget, Options{Base: "no-such-base-ref"}); err == nil {
		t.Fatal("expected an error for an invalid base ref")
	}
}

func TestClassForDisplay(t *testing.T) {
	cases := []struct {
		classification, severity, want string
	}{
		{"blocking", "high", "blocking"},
		{"advisory", "", "advisory"},
		{"follow-up", "", "follow-up"},
		{"warning", "", "warning"},
	}
	for _, c := range cases {
		got := classForDisplay(findings.Record{Classification: c.classification, Severity: c.severity})
		if got != c.want {
			t.Errorf("classForDisplay(%q,%q) = %q, want %q", c.classification, c.severity, got, c.want)
		}
	}
}
