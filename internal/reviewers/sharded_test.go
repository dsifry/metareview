package reviewers

import (
	"strings"
	"testing"
)

func satisfiedManifest() ManifestContext {
	return ManifestContext{
		Present:       true,
		Verdict:       "PASS",
		ShardCount:    2,
		ShardsCovered: 2,
		CrossShard:    true,
		PlanHash:      "0f0f0f0f0f0f0f0f",
	}
}

// truncatedGit is a branch whose visible diff is truncated but whose measured
// full branch diff carries a TODO well past the 120 KB context limit.
func truncatedGit() GitContext {
	filler := strings.Repeat("+filler line to push the marker past the context cap\n", 3000)
	return GitContext{
		ChangedFiles:      []string{"internal/a.go"},
		Diff:              "+short visible diff\n",
		BranchDiffFull:    filler + "+// TODO: finish the sharded path\n",
		DiffTruncated:     true,
		RawDiffBytes:      1_372_619,
		FilteredDiffBytes: 1_372_619,
		RiskLevel:         "context-risk",
		RiskReasons:       []string{"DIFF_TRUNCATED", "LARGE_DIFF"},
	}
}

func byFingerprint(results []Finding, fingerprint string) (Finding, bool) {
	for _, item := range results {
		if item.Fingerprint == fingerprint {
			return item, true
		}
	}
	return Finding{}, false
}

func TestContextRiskSatisfiedEmitsAdvisoryAndRunsLints(t *testing.T) {
	git := truncatedGit()
	if len(git.BranchDiffFull) <= 120000 {
		t.Fatalf("fixture must exceed the context cap, got %d bytes", len(git.BranchDiffFull))
	}

	results := RunTaskDone(Context{Git: git, Manifest: satisfiedManifest(), EvidenceText: passingEvidence()})

	covered, ok := byFingerprint(results, "architecture:context-risk-covered")
	if !ok {
		t.Fatalf("no covered finding: %+v", fingerprints(results))
	}
	if covered.Classification != "advisory" {
		t.Fatalf("covered finding classification = %q, want advisory", covered.Classification)
	}
	if !strings.Contains(covered.Found, "0f0f0f0f0f0f0f0f") {
		t.Fatalf("covered finding must name the plan hash: %q", covered.Found)
	}
	if _, ok := byFingerprint(results, "architecture:context-risk"); ok {
		t.Fatal("the blocking context-risk finding must not also be emitted")
	}
	todo, ok := byFingerprint(results, "quality:todo")
	if !ok {
		t.Fatalf("lints did not run over the full branch diff: %+v", fingerprints(results))
	}
	if !strings.Contains(todo.Found, "finish the sharded path") {
		t.Fatalf("TODO beyond the cap not found: %q", todo.Found)
	}
}

func TestTruncatedDiffFindingAdvisoryOnSatisfiedPath(t *testing.T) {
	results := RunTaskDone(Context{Git: truncatedGit(), Manifest: satisfiedManifest(), EvidenceText: passingEvidence()})

	truncated, ok := byFingerprint(results, "architecture:truncated-diff")
	if !ok {
		t.Fatalf("truncated-diff finding missing: %+v", fingerprints(results))
	}
	if truncated.Classification != "advisory" {
		t.Fatalf("truncated-diff classification = %q, want advisory", truncated.Classification)
	}

	unsatisfied := RunTaskDone(Context{
		Git:          withReasons(truncatedGit(), "DIFF_TRUNCATED"),
		EvidenceText: passingEvidence(),
	})
	if _, ok := byFingerprint(unsatisfied, "architecture:truncated-diff"); ok {
		t.Fatal("the unsatisfied path still early-returns before the other findings")
	}
}

func TestStagedEvalStillFoundOnSatisfiedPath(t *testing.T) {
	git := truncatedGit()
	git.StagedDiff = "+value := eval(input)\n"

	results := RunTaskDone(Context{Git: git, Manifest: satisfiedManifest(), EvidenceText: passingEvidence()})

	if _, ok := byFingerprint(results, "security:eval"); !ok {
		t.Fatalf("staged eval must still be found: %+v", fingerprints(results))
	}
}

func TestContextRiskNotSatisfiedByEmptyManifest(t *testing.T) {
	results := RunTaskDone(Context{Git: truncatedGit(), Manifest: ManifestContext{}})

	blocking, ok := byFingerprint(results, "architecture:context-risk")
	if !ok {
		t.Fatalf("an empty manifest must not satisfy context risk: %+v", fingerprints(results))
	}
	if blocking.Classification != "blocking" {
		t.Fatalf("classification = %q, want blocking", blocking.Classification)
	}
}

func TestContextRiskNotSatisfiedWithMissingShard(t *testing.T) {
	for name, manifest := range map[string]ManifestContext{
		"missing shard":       {Present: true, Verdict: "PASS", ShardCount: 3, ShardsCovered: 2, CrossShard: true},
		"no cross-shard":      {Present: true, Verdict: "PASS", ShardCount: 2, ShardsCovered: 2},
		"manifest blockers":   {Present: true, Verdict: "NEEDS_REVISION", ShardCount: 1, ShardsCovered: 1, Blockers: []string{"duplicate shard result shard-3a"}},
		"no shards planned":   {Present: true, Verdict: "PASS"},
		"no results ingested": {Verdict: "PASS", ShardCount: 1, ShardsCovered: 1},
	} {
		t.Run(name, func(t *testing.T) {
			results := RunTaskDone(Context{Git: truncatedGit(), Manifest: manifest})
			blocking, ok := byFingerprint(results, "architecture:context-risk")
			if !ok {
				t.Fatalf("%s must not satisfy context risk: %+v", name, fingerprints(results))
			}
			for _, blocker := range manifest.Blockers {
				if !strings.Contains(blocking.Found, blocker) {
					t.Fatalf("Found must carry the manifest blockers: %q", blocking.Found)
				}
			}
		})
	}
}

func TestManifestBlockersTruncatedToTen(t *testing.T) {
	manifest := ManifestContext{Present: true, Verdict: "NEEDS_REVISION", ShardCount: 1, ShardsCovered: 1}
	for i := 0; i < 14; i++ {
		manifest.Blockers = append(manifest.Blockers, "blocker-"+string(rune('a'+i)))
	}

	results := RunTaskDone(Context{Git: truncatedGit(), Manifest: manifest})
	blocking, _ := byFingerprint(results, "architecture:context-risk")

	if strings.Contains(blocking.Found, "blocker-k") {
		t.Fatalf("only the first ten blockers belong in Found: %q", blocking.Found)
	}
	if !strings.Contains(blocking.Found, "blocker-j") {
		t.Fatalf("the first ten blockers are missing: %q", blocking.Found)
	}
}

func TestMixedReasonsNeverSatisfied(t *testing.T) {
	git := withReasons(truncatedGit(), "DIFF_TRUNCATED", "UNTRACKED_TRUNCATED")

	results := RunTaskDone(Context{Git: git, Manifest: satisfiedManifest()})

	if _, ok := byFingerprint(results, "architecture:context-risk"); !ok {
		t.Fatalf("a non-shardable reason must keep the blocker: %+v", fingerprints(results))
	}
}

func TestLocalReasonsNeverSatisfied(t *testing.T) {
	for _, reason := range []string{"LOCAL_DIFF_TRUNCATED", "UNTRACKED_OMITTED", "DIFF_OVERSIZE"} {
		git := withReasons(truncatedGit(), reason)
		results := RunTaskDone(Context{Git: git, Manifest: satisfiedManifest()})
		if _, ok := byFingerprint(results, "architecture:context-risk"); !ok {
			t.Fatalf("%s must never be satisfied by shard results: %+v", reason, fingerprints(results))
		}
	}
}

func TestContextRiskFingerprintReasonIndependentAllScopes(t *testing.T) {
	first := withReasons(truncatedGit(), "DIFF_TRUNCATED")
	second := withReasons(truncatedGit(), "LARGE_DIFF", "UNTRACKED_OMITTED")

	taskA := RunTaskDone(Context{Git: first})
	taskB := RunTaskDone(Context{Git: second})
	if taskA[0].Fingerprint != "architecture:context-risk" || taskB[0].Fingerprint != "architecture:context-risk" {
		t.Fatalf("task-done fingerprints = %q / %q", taskA[0].Fingerprint, taskB[0].Fingerprint)
	}
	if !strings.Contains(taskB[0].Found, "LARGE_DIFF") {
		t.Fatalf("reasons must move to Found: %q", taskB[0].Found)
	}

	prA, _ := byFingerprint(RunPRReady(PRReadyContext{Git: first}), "pr:architecture:context-risk")
	prB, _ := byFingerprint(RunPRReady(PRReadyContext{Git: second}), "pr:architecture:context-risk")
	if prA.Fingerprint != prB.Fingerprint || prA.Fingerprint != "pr:architecture:context-risk" {
		t.Fatalf("pr-ready fingerprints = %q / %q", prA.Fingerprint, prB.Fingerprint)
	}

	epicGit := func(git GitContext) EpicGitContext {
		return EpicGitContext{ChangedFiles: git.ChangedFiles, Diff: git.Diff, RiskLevel: git.RiskLevel, RiskReasons: git.RiskReasons}
	}
	epicA, _ := byFingerprint(RunEpicReady(EpicReadyContext{Git: epicGit(first)}), "epic:context-risk")
	epicB, _ := byFingerprint(RunEpicReady(EpicReadyContext{Git: epicGit(second)}), "epic:context-risk")
	if epicA.Fingerprint != epicB.Fingerprint || epicA.Fingerprint != "epic:context-risk" {
		t.Fatalf("epic-ready fingerprints = %q / %q", epicA.Fingerprint, epicB.Fingerprint)
	}
}

func TestCoveredFingerprintStable(t *testing.T) {
	first := RunTaskDone(Context{Git: truncatedGit(), Manifest: satisfiedManifest(), EvidenceText: passingEvidence()})
	other := satisfiedManifest()
	other.PlanHash = "aaaaaaaaaaaaaaaa"
	other.ShardCount, other.ShardsCovered = 7, 7
	second := RunTaskDone(Context{Git: withReasons(truncatedGit(), "LARGE_DIFF"), Manifest: other, EvidenceText: passingEvidence()})

	a, _ := byFingerprint(first, "architecture:context-risk-covered")
	b, _ := byFingerprint(second, "architecture:context-risk-covered")
	if a.Fingerprint != b.Fingerprint || a.Fingerprint == "" {
		t.Fatalf("covered fingerprint must be stable: %q / %q", a.Fingerprint, b.Fingerprint)
	}

	pr := RunPRReady(PRReadyContext{Git: truncatedGit(), Manifest: satisfiedManifest(), EvidenceText: passingEvidence()})
	if _, ok := byFingerprint(pr, "pr:architecture:context-risk-covered"); !ok {
		t.Fatalf("pr-ready covered fingerprint missing: %+v", fingerprints(pr))
	}
}

func withReasons(git GitContext, reasons ...string) GitContext {
	git.RiskReasons = reasons
	return git
}

func fingerprints(results []Finding) []string {
	out := make([]string, 0, len(results))
	for _, item := range results {
		out = append(out, item.Fingerprint)
	}
	return out
}

func passingEvidence() string {
	return `{"schemaVersion":1,"kind":"validation","command":["go","test","./..."],"exitCode":0,"summary":"go test ./... exited 0"}`
}
