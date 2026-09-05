package prready

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
	"github.com/dsifry/metareview/internal/githubcontext"
	"github.com/dsifry/metareview/internal/knowledge"
	"github.com/dsifry/metareview/internal/reviewers"
	"github.com/dsifry/metareview/internal/reviewlog"
	"github.com/dsifry/metareview/internal/runchain"
)

// --- git helpers (with the mandatory CI-flake-proof environment) ---

func campaignGitEnv() []string {
	return append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_OPTIONAL_LOCKS=0", "GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=gc.auto", "GIT_CONFIG_VALUE_0=0",
		"GIT_CONFIG_KEY_1=maintenance.auto", "GIT_CONFIG_VALUE_1=false")
}

func campaignGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	cmd.Env = campaignGitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func campaignRevParse(t *testing.T, root, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", ref)
	cmd.Dir = root
	cmd.Env = campaignGitEnv()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse %s: %v", ref, err)
	}
	return strings.TrimSpace(string(out))
}

// --- pure helpers in evidence.go ---

func TestListSkipsEmptyAndFallsBack(t *testing.T) {
	if got := list([]string{"a", "  ", "b"}, "empty"); got != "- a\n- b" {
		t.Fatalf("list should skip blank entries, got %q", got)
	}
	if got := list([]string{"", "   "}, "empty"); got != "empty" {
		t.Fatalf("all-blank input must render the empty fallback, got %q", got)
	}
	if got := list(nil, "empty"); got != "empty" {
		t.Fatalf("nil input must render the empty fallback, got %q", got)
	}
}

func TestFirstNonEmptyAllEmpty(t *testing.T) {
	if got := firstNonEmpty("", "  ", "\t"); got != "" {
		t.Fatalf("all-blank input must yield empty string, got %q", got)
	}
	if got := firstNonEmpty("", "x"); got != "x" {
		t.Fatalf("first non-blank must win, got %q", got)
	}
}

// --- pure helpers in review.go ---

func TestSameStrings(t *testing.T) {
	if sameStrings([]string{"a"}, []string{"a", "b"}) {
		t.Fatal("different lengths must not be equal")
	}
	if sameStrings([]string{"a", "x"}, []string{"a", "y"}) {
		t.Fatal("a differing element must not be equal")
	}
	if !sameStrings([]string{"a", "b"}, []string{"a", "b"}) {
		t.Fatal("identical slices must be equal")
	}
}

func TestValidHex(t *testing.T) {
	if !validHex("0123456789abcdef") {
		t.Fatal("lowercase hex must be valid")
	}
	if validHex("abcXdef") {
		t.Fatal("a non-hex char must be invalid")
	}
	if validHex("ABCDEF") {
		t.Fatal("uppercase must be rejected by the lowercase-only check")
	}
	if validHex("") {
		t.Fatal("empty must be invalid")
	}
}

func TestFirstInlineCodeValue(t *testing.T) {
	if got := firstInlineCodeValue("- Base: `deadbeef`"); got != "deadbeef" {
		t.Fatalf("value between backticks expected, got %q", got)
	}
	if got := firstInlineCodeValue("no code here"); got != "" {
		t.Fatalf("no opening backtick must yield empty, got %q", got)
	}
	if got := firstInlineCodeValue("`unclosed value"); got != "" {
		t.Fatalf("no closing backtick must yield empty, got %q", got)
	}
}

func TestReviewerKnowledgeCopiesFacts(t *testing.T) {
	got := reviewerKnowledge(knowledge.Context{
		ServiceInventoryPath: "docs/services.yaml",
		ServiceInventory:     "body",
		Facts: []knowledge.Fact{
			{Source: "beads", Text: "prefer DI"},
			{Source: "readme", Text: "run make test"},
		},
	})
	if got.ServiceInventoryPath != "docs/services.yaml" || got.ServiceInventory != "body" {
		t.Fatalf("service inventory not carried: %+v", got)
	}
	if len(got.Facts) != 2 || got.Facts[0].Source != "beads" || got.Facts[1].Text != "run make test" {
		t.Fatalf("facts not carried: %+v", got.Facts)
	}
}

func TestReviewerGitHubCopiesCommentsAndReviews(t *testing.T) {
	got := reviewerGitHub(githubcontext.Context{
		Available:      true,
		ReviewDecision: "APPROVED",
		Comments:       []githubcontext.Entry{{Author: "alice", URL: "u1", Body: "b1"}},
		Reviews:        []githubcontext.Entry{{Author: "bob", URL: "u2", State: "APPROVED", Body: "b2"}},
	})
	if !got.Available || got.ReviewDecision != "APPROVED" {
		t.Fatalf("top-level context lost: %+v", got)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("both a comment and a review entry expected, got %+v", got.Entries)
	}
	if got.Entries[0].Author != "alice" || got.Entries[1].State != "APPROVED" {
		t.Fatalf("entries not carried: %+v", got.Entries)
	}
}

func TestBlockerLogsFallsBackToFindingID(t *testing.T) {
	// A blocker whose target has no id/path falls back to the finding's own id as the log target.
	got := blockerLogs([]findings.Record{{ID: "mrvf-1", Target: map[string]string{}}})
	if len(got) != 1 || got[0].Target != "mrvf-1" {
		t.Fatalf("empty target must fall back to the finding ID: %+v", got)
	}
	// A blocker with a real target id uses it.
	got = blockerLogs([]findings.Record{{ID: "mrvf-2", Target: map[string]string{"id": "TASK-9"}}})
	if len(got) != 1 || got[0].Target != "TASK-9" {
		t.Fatalf("target id must be preferred: %+v", got)
	}
}

func TestTaskAndEpicReviewEvidenceSplit(t *testing.T) {
	logs := []reviewlog.Summary{
		{Target: "TASK-1", Path: "docs/metareview/reviews/task.md"},
		{Target: "EPIC-42", Path: "docs/metareview/reviews/epic.md"},
	}
	task := taskReviewEvidence(logs)
	if len(task) != 1 || task[0].Target != "TASK-1" {
		t.Fatalf("epic target must be skipped from task evidence: %+v", task)
	}
	epic := epicReviewEvidence(logs)
	if len(epic) != 1 || epic[0].Target != "EPIC-42" {
		t.Fatalf("only epic targets belong in epic evidence: %+v", epic)
	}
}

func TestUniqueStringsSkipsBlankAndDuplicate(t *testing.T) {
	got := uniqueStrings([]string{"a", "", "  ", "a", "b", "b"})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("blank and duplicate values must be dropped: %+v", got)
	}
}

func TestValidationLinesFallbackToRawText(t *testing.T) {
	// A line that declares schemaVersion but is invalid JSON makes evidence.Parse error, so
	// validationLines falls back to the non-blank-line split (trimming each surviving line).
	got := validationLines("{\"schemaVersion\": oops}\n\n  go test ./... exited 0  \n")
	if len(got) != 2 || got[1] != "go test ./... exited 0" {
		t.Fatalf("raw fallback must trim and drop blank lines: %+v", got)
	}
}

func TestFilterGeneratedFilesDropsMetareviewPaths(t *testing.T) {
	got := filterGeneratedFiles([]string{"internal/a.go", ".metareview/runs.jsonl", "docs/metareview/x.md", "cmd/b.go"})
	if len(got) != 2 || got[0] != "internal/a.go" || got[1] != "cmd/b.go" {
		t.Fatalf("generated metareview paths must be filtered: %+v", got)
	}
}

func TestIsGeneratedDiffSection(t *testing.T) {
	if isGeneratedDiffSection("too short") {
		t.Fatal("a header with fewer than four fields must not be treated as generated")
	}
	if !isGeneratedDiffSection("diff --git a/.metareview/runs.jsonl b/.metareview/runs.jsonl") {
		t.Fatal("a metareview diff section must be recognized as generated")
	}
	if isGeneratedDiffSection("diff --git a/internal/a.go b/internal/a.go") {
		t.Fatal("a source diff section must not be generated")
	}
}

func TestUniquePathsAdvancesPastCollision(t *testing.T) {
	root := t.TempDir()
	at := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	// Occupy the path uniquePaths would first compute so it must advance one nanosecond.
	first, _, firstReview := uniquePaths(root, at)
	occupied := filepath.Join(root, filepath.FromSlash(firstReview))
	if err := os.MkdirAll(filepath.Dir(occupied), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(occupied, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	runID, _, _ := uniquePaths(root, at)
	if runID == first {
		t.Fatal("a collision must force uniquePaths to advance to a new run id")
	}
}

func TestRepositoryHealthMarkdownDefaultsEmptyTitle(t *testing.T) {
	got := repositoryHealthMarkdown([]findings.Record{
		{Title: "   ", Target: map[string]string{"id": "TASK-7"}},
	})
	if !strings.Contains(got, "Unresolved historical finding (TASK-7)") {
		t.Fatalf("a blank title must default and keep the target: %q", got)
	}
	if repositoryHealthMarkdown(nil) != "" {
		t.Fatal("no records must render nothing")
	}
}

func TestRestoreSnapshotsWritesAndRemoves(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "nested", "existing.md")
	created := filepath.Join(dir, "created.md")
	// A file that "did not exist" before the run must be removed on restore.
	if err := os.WriteFile(created, []byte("should be removed"), 0o644); err != nil {
		t.Fatal(err)
	}
	restoreSnapshots(map[string]fileSnapshot{
		existing: {existed: true, content: []byte("restored")},
		created:  {existed: false},
	})
	body, err := os.ReadFile(existing)
	if err != nil || string(body) != "restored" {
		t.Fatalf("an existed snapshot must be rewritten: body=%q err=%v", body, err)
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Fatal("a not-existed snapshot must be removed")
	}
}

func TestMarkdownList(t *testing.T) {
	if got := markdownList(nil, "none"); got != "none" {
		t.Fatalf("empty slice must render the empty fallback, got %q", got)
	}
	if got := markdownList([]string{"", ""}, "none"); got != "none" {
		t.Fatalf("all-blank/duplicate must render the empty fallback, got %q", got)
	}
	if got := markdownList([]string{"a", "a", "b"}, "none"); got != "- a\n- b" {
		t.Fatalf("duplicates must be dropped, got %q", got)
	}
}

func TestValidateExplicitPreviousRunNotFound(t *testing.T) {
	err := validateExplicitPreviousRunInput(t.TempDir(), nil, "ghost", map[string]string{"type": "branch", "id": "feature"}, gitcontext.Context{Branch: "feature"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("an unknown explicit previous run must be not-found, got %v", err)
	}
	// An empty previous-run id short-circuits with no error.
	if err := validateExplicitPreviousRunInput(t.TempDir(), nil, "  ", nil, gitcontext.Context{}); err != nil {
		t.Fatalf("blank previous run must be a no-op, got %v", err)
	}
}

func TestReusableVerdictContinueBranches(t *testing.T) {
	target := map[string]string{"type": "branch", "id": "feature"}
	base, head, digest := "b1", "h1", "sha256:abc"
	good := reviewlog.Summary{
		RunRecordAuthenticated: true, Kind: "pr-ready", Status: "passed", Verdict: "PASS",
		BaseSHA: base, HeadSHA: head, ReviewInputDigest: digest,
		TargetRecord: map[string]string{"type": "branch", "id": "feature"},
		Reviewers:    append([]string(nil), reviewerNames...), ExecutionMode: "deterministic-local",
	}
	if _, ok := reusableVerdict([]reviewlog.Summary{good}, target, base, head, digest); !ok {
		t.Fatal("a fully matching authenticated PASS must be reusable")
	}

	// Status passed but a non-PASS verdict: the verdict guard continues.
	badVerdict := good
	badVerdict.Verdict = "NEEDS_REVISION"
	if _, ok := reusableVerdict([]reviewlog.Summary{badVerdict}, target, base, head, digest); ok {
		t.Fatal("a non-PASS verdict must not be reused")
	}

	// Everything matches but the reviewer set differs: the sameStrings guard continues.
	badReviewers := good
	badReviewers.Reviewers = []string{"only-one"}
	if _, ok := reusableVerdict([]reviewlog.Summary{badReviewers}, target, base, head, digest); ok {
		t.Fatal("a differing reviewer set must not be reused")
	}

	// Everything matches but the execution mode is not a deterministic-local variant.
	badMode := good
	badMode.ExecutionMode = "subagent-adjudicated"
	if _, ok := reusableVerdict([]reviewlog.Summary{badMode}, target, base, head, digest); ok {
		t.Fatal("a non-deterministic execution mode must not be reused")
	}
}

// --- resolveRunChain branches (direct) ---

func TestResolveRunChainFreshRunEscalationLock(t *testing.T) {
	// runchain.Resolve succeeds (no runs.jsonl, empty chain) but the committed logs show an ESCALATED
	// pr-ready run for this target at the current head: the fresh-run escalation branch errors.
	root := t.TempDir()
	target := map[string]string{"type": "branch", "id": "feature"}
	git := gitcontext.Context{Branch: "feature", HeadSHA: "h1"}
	logs := []reviewlog.Summary{{RunID: "esc", Kind: "pr-ready", Target: "feature", Verdict: "ESCALATED", HeadSHA: "h1"}}
	_, _, err := resolveRunChain(root, target, Options{}, logs, git)
	if err == nil || !strings.Contains(err.Error(), "already escalated in run esc") {
		t.Fatalf("a fresh run against an escalated target must error, got %v", err)
	}
}

func TestResolveRunChainLegacyEscalatedAncestorErrors(t *testing.T) {
	// Recoverable "previous run not found" (no runs.jsonl) routes into legacy recovery, where the named
	// ancestor is itself ESCALATED: legacyPreviousRunIDsForPRReady errors and it surfaces.
	root := t.TempDir()
	target := map[string]string{"type": "branch", "id": "feature"}
	git := gitcontext.Context{Branch: "feature", HeadSHA: "h1"}
	logs := []reviewlog.Summary{{RunID: "mrv-1", Kind: "pr-ready", Target: "feature", Verdict: "ESCALATED"}}
	_, _, err := resolveRunChain(root, target, Options{PreviousRunID: "mrv-1"}, logs, git)
	if err == nil || !strings.Contains(err.Error(), "escalated") {
		t.Fatalf("an escalated legacy ancestor must surface an error, got %v", err)
	}
}

func TestResolveRunChainLegacyNoIDsReturnsOriginalError(t *testing.T) {
	// Recoverable error, but the named previous run belongs to a different branch, so legacy recovery
	// yields no ids and the original runchain error is returned.
	root := t.TempDir()
	target := map[string]string{"type": "branch", "id": "feature"}
	git := gitcontext.Context{Branch: "feature", HeadSHA: "h1"}
	logs := []reviewlog.Summary{{RunID: "mrv-1", Kind: "pr-ready", Target: "other-branch", Verdict: "NEEDS_REVISION"}}
	_, _, err := resolveRunChain(root, target, Options{PreviousRunID: "mrv-1"}, logs, git)
	if err == nil {
		t.Fatal("a recoverable error with no legacy ids must return the original error")
	}
	if strings.Contains(err.Error(), "escalated") {
		t.Fatalf("the surfaced error should be the original not-found, got %v", err)
	}
}

func TestResolveRunChainFallbackResolveError(t *testing.T) {
	// Force the first runchain.Resolve to fail recoverably and the fallback Resolve to fail, so the
	// fallback error surfaces. Legacy id recovery (real) yields [mrv-1] from the committed logs.
	root := t.TempDir()
	target := map[string]string{"type": "branch", "id": "feature"}
	git := gitcontext.Context{Branch: "feature", HeadSHA: "h1"}
	logs := []reviewlog.Summary{{RunID: "mrv-1", Kind: "pr-ready", Target: "feature", Verdict: "NEEDS_REVISION"}}

	original := resolveChainFn
	calls := 0
	resolveChainFn = func(r string, o runchain.Options) (runchain.Decision, error) {
		calls++
		if calls == 1 {
			return runchain.Decision{}, errors.New("previous run mrv-1 not found")
		}
		return runchain.Decision{}, errors.New("fallback boom")
	}
	t.Cleanup(func() { resolveChainFn = original })

	_, _, err := resolveRunChain(root, target, Options{PreviousRunID: "mrv-1"}, logs, git)
	if err == nil || !strings.Contains(err.Error(), "fallback boom") {
		t.Fatalf("the fallback resolve error must surface, got %v", err)
	}
}

func TestResolveRunChainLegacyEscalatedForTargetAfterMatch(t *testing.T) {
	// All ancestors in the chain match and none are escalated in the loop, but a separate escalated
	// pr-ready log for the same target trips the post-loop guard.
	root := t.TempDir()
	target := map[string]string{"type": "branch", "id": "feature"}
	git := gitcontext.Context{Branch: "feature", HeadSHA: "h1"}
	logs := []reviewlog.Summary{
		{RunID: "mrv-1", Kind: "pr-ready", Target: "feature", Verdict: "NEEDS_REVISION"},
		{RunID: "esc", Kind: "pr-ready", Target: "feature", Verdict: "ESCALATED", HeadSHA: "h1"},
	}
	_, err := legacyPreviousRunIDsForPRReady(root, logs, "mrv-1", target, git)
	if err == nil || !strings.Contains(err.Error(), "already escalated in run esc") {
		t.Fatalf("a same-target escalated log must trip the post-loop guard, got %v", err)
	}
}

func TestAuthenticatedLegacyRootMaxAttemptsEmpty(t *testing.T) {
	if got := authenticatedLegacyRootMaxAttempts(t.TempDir(), nil, nil, nil, gitcontext.Context{}); got != 0 {
		t.Fatalf("no previous ids must yield 0, got %d", got)
	}
}

// --- legacyPRReadyTargetMatch branches (context-identity path) ---

func writeCampaignContext(t *testing.T, root, rel, doc string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyPRReadyTargetMatchCurrentBranchPaths(t *testing.T) {
	target := map[string]string{"type": "branch", "id": "feature"}

	t.Run("unreadable context is unknown", func(t *testing.T) {
		log := reviewlog.Summary{Target: "current branch", ContextRel: "docs/metareview/context/missing.md"}
		matches, known := legacyPRReadyTargetMatch(t.TempDir(), log, target, gitcontext.Context{Branch: "feature", HeadSHA: "h1"})
		if matches || known {
			t.Fatalf("an unreadable context must be (false,false), got (%v,%v)", matches, known)
		}
	})

	t.Run("empty identity branch is unknown", func(t *testing.T) {
		root := t.TempDir()
		rel := "docs/metareview/context/c.md"
		// Head present, Branch bullet absent: readLegacyPRReadyContextIdentity succeeds but identity.Branch is "".
		writeCampaignContext(t, root, rel, "# ctx\n\n## Git\n\n- Head: `abcabc`\n")
		log := reviewlog.Summary{Target: "current branch", ContextRel: rel}
		matches, known := legacyPRReadyTargetMatch(root, log, target, gitcontext.Context{Branch: "feature", HeadSHA: "h1"})
		if matches || known {
			t.Fatalf("an empty identity branch must be (false,false), got (%v,%v)", matches, known)
		}
	})

	t.Run("branch mismatch is a known non-match", func(t *testing.T) {
		root := t.TempDir()
		rel := "docs/metareview/context/c.md"
		writeCampaignContext(t, root, rel, contextIdentityDoc("mrv-1", "b", "h1", "other", "sha256:x"))
		log := reviewlog.Summary{Target: "current branch", ContextRel: rel}
		matches, known := legacyPRReadyTargetMatch(root, log, target, gitcontext.Context{Branch: "feature", HeadSHA: "h1"})
		if matches || !known {
			t.Fatalf("a branch mismatch must be (false,true), got (%v,%v)", matches, known)
		}
	})

	t.Run("head is not an ancestor is a known non-match", func(t *testing.T) {
		root := t.TempDir() // not a git repo, so merge-base --is-ancestor always fails
		rel := "docs/metareview/context/c.md"
		oldHead := strings.Repeat("a", 40)
		newHead := strings.Repeat("b", 40)
		writeCampaignContext(t, root, rel, contextIdentityDoc("mrv-1", "b", oldHead, "feature", "sha256:x"))
		log := reviewlog.Summary{Target: "current branch", ContextRel: rel}
		matches, known := legacyPRReadyTargetMatch(root, log, target, gitcontext.Context{Branch: "feature", HeadSHA: newHead})
		if matches || !known {
			t.Fatalf("a non-ancestor head must be (false,true), got (%v,%v)", matches, known)
		}
	})

	t.Run("matching branch and head is a match", func(t *testing.T) {
		root := t.TempDir()
		rel := "docs/metareview/context/c.md"
		writeCampaignContext(t, root, rel, contextIdentityDoc("mrv-1", "b", "h1", "feature", "sha256:x"))
		log := reviewlog.Summary{Target: "current branch", ContextRel: rel}
		matches, known := legacyPRReadyTargetMatch(root, log, target, gitcontext.Context{Branch: "feature", HeadSHA: "h1"})
		if !matches || !known {
			t.Fatalf("an exact branch+head identity must be (true,true), got (%v,%v)", matches, known)
		}
	})
}

func TestHistoricalPRReadyRunIDsForCurrentTarget(t *testing.T) {
	target := map[string]string{"type": "branch", "id": "feature"}
	git := gitcontext.Context{Branch: "feature", HeadSHA: "h1"}
	logs := []reviewlog.Summary{
		// Authenticated but a different target: historical.
		{RunID: "auth-other", Kind: "pr-ready", RunRecordAuthenticated: true, TargetRecord: map[string]string{"type": "branch", "id": "other"}},
		// Authenticated same target: current, not historical.
		{RunID: "auth-same", Kind: "pr-ready", RunRecordAuthenticated: true, TargetRecord: map[string]string{"type": "branch", "id": "feature"}},
		// Legacy, a known non-match (different branch id): historical.
		{RunID: "legacy-other", Kind: "pr-ready", Target: "other-branch"},
		// A non-pr-ready log is ignored.
		{RunID: "task", Kind: "task-done", Target: "feature"},
	}
	got := historicalPRReadyRunIDsForCurrentTarget(t.TempDir(), logs, target, git)
	has := func(id string) bool {
		for _, g := range got {
			if g == id {
				return true
			}
		}
		return false
	}
	if !has("auth-other") || !has("legacy-other") {
		t.Fatalf("historical ids must include the off-target authenticated and legacy runs: %+v", got)
	}
	if has("auth-same") || has("task") {
		t.Fatalf("current-target and non-pr-ready runs must not be historical: %+v", got)
	}
}

// --- readLegacyPRReadyContextIdentity error branches ---

func TestReadLegacyPRReadyContextIdentityErrors(t *testing.T) {
	if _, err := readLegacyPRReadyContextIdentity(t.TempDir(), "  "); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("a blank rel must be required, got %v", err)
	}
	if _, err := readLegacyPRReadyContextIdentity(t.TempDir(), "../escape.md"); err == nil || !strings.Contains(err.Error(), "escapes repository") {
		t.Fatalf("a path escape must be rejected, got %v", err)
	}
	root := t.TempDir()
	rel := "docs/metareview/context/c.md"
	// Neither a branch nor a head: lacks identity.
	writeCampaignContext(t, root, rel, "# ctx\n\n## Git\n\n- Base: `b`\n")
	if _, err := readLegacyPRReadyContextIdentity(root, rel); err == nil || !strings.Contains(err.Error(), "lacks branch and head") {
		t.Fatalf("a context with no branch/head must error, got %v", err)
	}
}

func TestCommittedFileRejectsPathEscape(t *testing.T) {
	if _, ok := committedFile(t.TempDir(), "../escape.md"); ok {
		t.Fatal("a path escaping the repo must be refused")
	}
	if _, ok := committedFile(t.TempDir(), "  "); ok {
		t.Fatal("a blank path must be refused")
	}
}

// --- committedPRReadyInputAuthenticated branches ---

// committedAuthFixture builds a git repo with a committed review log and context pack wired for the
// authenticated-success path, and returns the log/git/target the checker consumes.
func committedAuthFixture(t *testing.T) (root string, log reviewlog.Summary, git gitcontext.Context, target map[string]string) {
	t.Helper()
	root = t.TempDir()
	campaignGit(t, root, "init", "-b", "main")
	campaignGit(t, root, "config", "user.email", "test@example.com")
	campaignGit(t, root, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(root, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	campaignGit(t, root, "add", ".")
	campaignGit(t, root, "commit", "-m", "initial")
	base := campaignRevParse(t, root, "HEAD")
	campaignGit(t, root, "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(root, "seed.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	campaignGit(t, root, "add", ".")
	campaignGit(t, root, "commit", "-m", "work")
	oldHead := campaignRevParse(t, root, "HEAD") // the head the review attests; an ancestor of the final HEAD

	digest := "sha256:" + strings.Repeat("a", 64)
	reviewRel := "docs/metareview/reviews/r.md"
	contextRel := "docs/metareview/context/c.md"
	writeCampaignContext(t, root, reviewRel, "# pr-ready review\n\nRun ID: `mrv-auth-1`\n")
	writeCampaignContext(t, root, contextRel, contextIdentityDoc("mrv-auth-1", base, oldHead, "feature", digest))
	campaignGit(t, root, "add", "docs")
	campaignGit(t, root, "commit", "-m", "record review")
	head := campaignRevParse(t, root, "HEAD")

	log = reviewlog.Summary{
		Kind: "pr-ready", Target: "current branch", RunID: "mrv-auth-1",
		DeclaredInputDigest: digest, Path: reviewRel, ContextRel: contextRel,
	}
	git = gitcontext.Context{Branch: "feature", BaseSHA: base, HeadSHA: head}
	target = map[string]string{"type": "branch", "id": "feature"}
	return root, log, git, target
}

func TestCommittedPRReadyInputAuthenticated(t *testing.T) {
	t.Run("authenticated success", func(t *testing.T) {
		root, log, git, target := committedAuthFixture(t)
		if !committedPRReadyInputAuthenticated(root, log, target, git) {
			t.Fatal("a fully committed, matching review+context must authenticate")
		}
	})

	t.Run("uncommitted review is not authenticated", func(t *testing.T) {
		root, log, git, target := committedAuthFixture(t)
		log.Path = "docs/metareview/reviews/ghost.md"
		if committedPRReadyInputAuthenticated(root, log, target, git) {
			t.Fatal("a review not present in HEAD must not authenticate")
		}
	})

	t.Run("working review differs from committed", func(t *testing.T) {
		root, log, git, target := committedAuthFixture(t)
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(log.Path)), []byte("tampered\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if committedPRReadyInputAuthenticated(root, log, target, git) {
			t.Fatal("a working review differing from HEAD must not authenticate")
		}
	})

	t.Run("uncommitted context is not authenticated", func(t *testing.T) {
		root, log, git, target := committedAuthFixture(t)
		log.ContextRel = "docs/metareview/context/ghost.md"
		if committedPRReadyInputAuthenticated(root, log, target, git) {
			t.Fatal("a context not present in HEAD must not authenticate")
		}
	})

	t.Run("identity mismatch is not authenticated", func(t *testing.T) {
		root, log, git, target := committedAuthFixture(t)
		git.BaseSHA = "a-different-base" // identity.Base no longer equals git.BaseSHA
		if committedPRReadyInputAuthenticated(root, log, target, git) {
			t.Fatal("a base mismatch between context identity and git must not authenticate")
		}
	})

	t.Run("first guard rejects a non-current-branch target", func(t *testing.T) {
		root, log, git, target := committedAuthFixture(t)
		log.Target = "TASK-1"
		if committedPRReadyInputAuthenticated(root, log, target, git) {
			t.Fatal("a non-current-branch target must fail the first guard")
		}
	})
}

// --- Create error branches via collaborator seams ---

func TestCreateGitContextErrorFromBadBase(t *testing.T) {
	root := smallPRReadyRepo(t)
	if _, err := Create(root, Options{Base: "this-ref-does-not-exist"}); err == nil {
		t.Fatal("a bad --base ref must make gitcontext error")
	}
}

func TestCreateReadEvidenceError(t *testing.T) {
	root := smallPRReadyRepo(t)
	if _, err := Create(root, Options{Base: "main", EvidencePath: filepath.Join(t.TempDir(), "nope.md")}); err == nil {
		t.Fatal("a missing evidence path must make readEvidence error")
	}
}

func TestCreateMutationReportError(t *testing.T) {
	root := smallPRReadyRepo(t)
	if _, err := Create(root, Options{Base: "main", MutationReportPaths: []string{filepath.Join(t.TempDir(), "missing.json")}}); err == nil {
		t.Fatal("a missing mutation report must stop the review")
	}
}

var errSeam = errors.New("seam boom")

func TestCreateCollaboratorSeamErrors(t *testing.T) {
	cases := []struct {
		name    string
		install func(t *testing.T)
	}{
		{"planShards", func(t *testing.T) {
			orig := planShards
			planShards = func(contextprofile.Profile, []gitcontext.BranchFile, contextprofile.ShardOptions) (contextprofile.ShardPlan, error) {
				return contextprofile.ShardPlan{}, errSeam
			}
			t.Cleanup(func() { planShards = orig })
		}},
		{"collectKnowledge", func(t *testing.T) {
			orig := collectKnowledge
			collectKnowledge = func(string) (knowledge.Context, error) { return knowledge.Context{}, errSeam }
			t.Cleanup(func() { collectKnowledge = orig })
		}},
		{"discoverLogs", func(t *testing.T) {
			orig := discoverLogs
			discoverLogs = func(string) ([]reviewlog.Summary, error) { return nil, errSeam }
			t.Cleanup(func() { discoverLogs = orig })
		}},
		{"resolveChain", func(t *testing.T) {
			orig := resolveChainFn
			resolveChainFn = func(string, runchain.Options) (runchain.Decision, error) { return runchain.Decision{}, errSeam }
			t.Cleanup(func() { resolveChainFn = orig })
		}},
		{"unresolvedBlocking", func(t *testing.T) {
			orig := unresolvedBlocking
			unresolvedBlocking = func(string) ([]findings.Record, error) { return nil, errSeam }
			t.Cleanup(func() { unresolvedBlocking = orig })
		}},
		{"allFindings", func(t *testing.T) {
			orig := allFindingsFn
			allFindingsFn = func(string) ([]findings.Record, error) { return nil, errSeam }
			t.Cleanup(func() { allFindingsFn = orig })
		}},
		{"collectGitHub", func(t *testing.T) {
			orig := collectGitHub
			collectGitHub = func(string, string) (githubcontext.Context, error) { return githubcontext.Context{}, errSeam }
			t.Cleanup(func() { collectGitHub = orig })
		}},
		{"marshalJSON", func(t *testing.T) {
			orig := marshalJSON
			marshalJSON = func(any) ([]byte, error) { return nil, errSeam }
			t.Cleanup(func() { marshalJSON = orig })
		}},
		{"reconcileFindings", func(t *testing.T) {
			orig := reconcileFindings
			reconcileFindings = func(string, findings.Run, []findings.Input, findings.Options) (findings.Result, error) {
				return findings.Result{}, errSeam
			}
			t.Cleanup(func() { reconcileFindings = orig })
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := smallPRReadyRepo(t)
			t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
			tc.install(t)
			_, err := Create(root, Options{Base: "main"})
			if err == nil || !errors.Is(err, errSeam) {
				t.Fatalf("Create must surface the %s seam error, got %v", tc.name, err)
			}
		})
	}
}

func TestCreateMkdirAndWriteErrors(t *testing.T) {
	t.Run("first mkdir fails", func(t *testing.T) {
		root := smallPRReadyRepo(t)
		t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
		orig := mkdirAll
		mkdirAll = func(string, os.FileMode) error { return errSeam }
		t.Cleanup(func() { mkdirAll = orig })
		if _, err := Create(root, Options{Base: "main"}); !errors.Is(err, errSeam) {
			t.Fatalf("a failing context mkdir must surface, got %v", err)
		}
	})

	t.Run("second mkdir fails", func(t *testing.T) {
		root := smallPRReadyRepo(t)
		t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
		orig := mkdirAll
		calls := 0
		mkdirAll = func(path string, mode os.FileMode) error {
			calls++
			if calls >= 2 {
				return errSeam
			}
			return orig(path, mode)
		}
		t.Cleanup(func() { mkdirAll = orig })
		if _, err := Create(root, Options{Base: "main"}); !errors.Is(err, errSeam) {
			t.Fatalf("a failing review-dir mkdir must surface, got %v", err)
		}
	})

	t.Run("first write fails", func(t *testing.T) {
		root := smallPRReadyRepo(t)
		t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
		orig := writeFile
		writeFile = func(string, []byte, os.FileMode) error { return errSeam }
		t.Cleanup(func() { writeFile = orig })
		if _, err := Create(root, Options{Base: "main"}); !errors.Is(err, errSeam) {
			t.Fatalf("a failing context write must surface, got %v", err)
		}
	})
}

func TestCreateAppendJSONLErrors(t *testing.T) {
	t.Run("fresh run append fails", func(t *testing.T) {
		root := smallPRReadyRepo(t)
		t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
		orig := appendJSONL
		appendJSONL = func(string, any) error { return errSeam }
		t.Cleanup(func() { appendJSONL = orig })
		if _, err := Create(root, Options{Base: "main"}); !errors.Is(err, errSeam) {
			t.Fatalf("a failing runs.jsonl append must surface, got %v", err)
		}
	})

	t.Run("reused run append fails", func(t *testing.T) {
		root := smallPRReadyRepo(t)
		t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
		evidence := filepath.Join(t.TempDir(), "evidence.md")
		if err := os.WriteFile(evidence, []byte("go test ./... exited 0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		// Establish an authenticated reusable PASS.
		if _, err := Create(root, Options{Base: "main", EvidencePath: evidence, Now: time.Date(2026, 9, 4, 6, 0, 0, 0, time.UTC)}); err != nil {
			t.Fatal(err)
		}
		orig := appendJSONL
		var appended any
		appendJSONL = func(_ string, record any) error { appended = record; return errSeam }
		t.Cleanup(func() { appendJSONL = orig })
		// The second, byte-identical run takes the reuse path and its append is the one that fails.
		if _, err := Create(root, Options{Base: "main", EvidencePath: evidence, Now: time.Date(2026, 9, 4, 6, 0, 1, 0, time.UTC)}); !errors.Is(err, errSeam) {
			t.Fatalf("a failing reused-run append must surface, got %v", err)
		}
		// Prove the failure was the reuse branch's append (line 458), not a fall-through to the fresh
		// path (line 518): only the reuse record carries ReusedFromRunID.
		record, ok := appended.(runRecord)
		if !ok {
			t.Fatalf("appendJSONL got %T, want runRecord", appended)
		}
		if record.ReusedFromRunID == "" {
			t.Fatalf("the failing append must be the reuse record (ReusedFromRunID set), got %+v", record)
		}
	})
}

func TestCreateIngestShardResultsError(t *testing.T) {
	root := shardedRepo(t)
	writer := &fakeWriter{discoverErr: errSeam}
	if _, err := Create(root, Options{Base: "main", ShardWriter: writer}); !errors.Is(err, errSeam) {
		t.Fatalf("a shard-result discovery error must stop the review, got %v", err)
	}
}

func TestCreatePackRollbackErrorIsJoined(t *testing.T) {
	root := shardedRepo(t)
	// A pack writer whose rollback also fails, combined with a review-body write failure, exercises the
	// errors.Join branch that preserves both the original failure and the rollback error.
	writer := &fakeWriter{rollback: func() error { return errors.New("rollback boom") }}
	orig := writeFile
	writeFile = func(string, []byte, os.FileMode) error { return errSeam }
	t.Cleanup(func() { writeFile = orig })
	_, err := Create(root, Options{Base: "main", ShardWriter: writer})
	if err == nil || !strings.Contains(err.Error(), "rollback boom") || !errors.Is(err, errSeam) {
		t.Fatalf("both the body error and the rollback error must be joined, got %v", err)
	}
	if writer.rollbacks != 1 {
		t.Fatalf("the pack rollback must run exactly once, got %d", writer.rollbacks)
	}
}

func TestCreateRunsGCAfterPassingShardedGate(t *testing.T) {
	root := shardedRepo(t)
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
	// No findings at all => a passing, non-blocking verdict, so the sharded GC housekeeping runs.
	orig := runPRReadyReviewers
	runPRReadyReviewers = func(reviewers.PRReadyContext) []reviewers.Finding { return nil }
	t.Cleanup(func() { runPRReadyReviewers = orig })
	writer := &fakeWriter{}
	result, err := Create(root, Options{Base: "main", ShardWriter: writer})
	if err != nil {
		t.Fatal(err)
	}
	if result.Blocking {
		t.Fatalf("with no findings the gate must pass, got %q", result.Verdict)
	}
	if writer.collections != 1 {
		t.Fatalf("GC must run once after a passing sharded gate, got %d", writer.collections)
	}
}

func TestCreateGateEffectForBeadsRepo(t *testing.T) {
	root := smallPRReadyRepo(t)
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
	if err := os.MkdirAll(filepath.Join(root, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := runPRReadyReviewers
	runPRReadyReviewers = func(reviewers.PRReadyContext) []reviewers.Finding { return nil }
	t.Cleanup(func() { runPRReadyReviewers = orig })
	result, err := Create(root, Options{Base: "main", Now: time.Date(2026, 9, 4, 7, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.ReviewRel)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Gate effect: `gate`") {
		t.Fatalf("a Beads-capable repo must set the gate effect to gate:\n%s", body)
	}
}
