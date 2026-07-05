package reviewmanifest

import (
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/contextprofile"
)

func TestAggregateRequiresCompletePathAccounting(t *testing.T) {
	profile := contextprofile.Profile{
		Files: []contextprofile.FileProfile{
			{Path: "internal/reviewmanifest/manifest.go", DiffBytes: 100},
		},
		GeneratedExcludedFiles: []string{"docs/metareview/context/run-context.md"},
	}
	plan := singleShardPlan("source-hash", "internal/reviewmanifest/manifest.go")

	manifest := Build(Input{Scope: "task-done", Target: map[string]string{"type": "task", "id": "wu4"}, Profile: profile, ShardPlan: plan})
	aggregate := Aggregate(manifest)

	assertBlocker(t, aggregate, "missing disposition for docs/metareview/context/run-context.md")

	manifest = Build(Input{
		Scope:     "task-done",
		Target:    map[string]string{"type": "task", "id": "wu4"},
		Profile:   profile,
		ShardPlan: plan,
		PathDispositions: []PathDisposition{{
			Path:        "docs/metareview/context/run-context.md",
			Disposition: DispositionGenerated,
			Rationale:   "metareview generated context artifact",
		}},
	})
	aggregate = Aggregate(manifest)

	assertNoBlocker(t, aggregate, "missing disposition")

	manifest.PathDispositions[0].Rationale = "tbd"
	aggregate = Aggregate(manifest)

	assertBlocker(t, aggregate, "invalid disposition rationale")
}

func TestAggregateBlocksZeroAndDuplicatePrimaryShardAssignments(t *testing.T) {
	profile := contextprofile.Profile{
		Files: []contextprofile.FileProfile{
			{Path: "internal/a.go", DiffBytes: 50},
			{Path: "internal/b.go", DiffBytes: 50},
		},
	}
	plan := contextprofile.ShardPlan{
		SourceDiffHash: "source-hash",
		Shards: []contextprofile.Shard{
			{ID: "shard-01", SourceDiffHash: "source-hash", Paths: []string{"internal/a.go"}, ByteCount: 50},
			{ID: "shard-02", SourceDiffHash: "source-hash", Paths: []string{"internal/a.go"}, ByteCount: 50},
		},
	}

	aggregate := Aggregate(Build(Input{Profile: profile, ShardPlan: plan}))

	assertBlocker(t, aggregate, "internal/a.go assigned to multiple primary shards")
	assertBlocker(t, aggregate, "internal/b.go is not assigned to a primary shard")
}

func TestSourceManifestHashIsStableAndSensitive(t *testing.T) {
	profile := contextprofile.Profile{
		Files: []contextprofile.FileProfile{
			{Path: "internal/b.go", DiffBytes: 20},
			{Path: "internal/a.go", DiffBytes: 10},
		},
		GeneratedExcludedFiles: []string{"docs/metareview/context/a.md"},
	}
	plan := contextprofile.ShardPlan{
		SourceDiffHash: "source-hash",
		Shards: []contextprofile.Shard{
			{ID: "shard-02", SourceDiffHash: "source-hash", Paths: []string{"internal/b.go"}, ByteCount: 20},
			{ID: "shard-01", SourceDiffHash: "source-hash", Paths: []string{"internal/a.go"}, ByteCount: 10},
		},
	}
	dispositions := []PathDisposition{{
		Path:        "docs/metareview/context/a.md",
		Disposition: DispositionGenerated,
		Rationale:   "metareview generated context artifact",
	}}

	first := Build(Input{Profile: profile, ShardPlan: plan, PathDispositions: dispositions})
	second := Build(Input{Profile: profile, ShardPlan: plan, PathDispositions: dispositions})
	if first.SourceManifestHash == "" || first.SourceManifestHash != second.SourceManifestHash {
		t.Fatalf("source manifest hash should be stable: first=%q second=%q", first.SourceManifestHash, second.SourceManifestHash)
	}
	taskDone := Build(Input{Scope: "task-done", Profile: profile, ShardPlan: plan, PathDispositions: dispositions})
	prReady := Build(Input{Scope: "pr-ready", Profile: profile, ShardPlan: plan, PathDispositions: dispositions})
	if taskDone.SourceManifestHash != prReady.SourceManifestHash {
		t.Fatalf("source manifest hash must not drift by gate scope: task=%q pr=%q", taskDone.SourceManifestHash, prReady.SourceManifestHash)
	}

	plan.Shards[0].Paths = []string{"internal/b.go", "internal/c.go"}
	changed := Build(Input{Profile: profile, ShardPlan: plan, PathDispositions: dispositions})
	if changed.SourceManifestHash == first.SourceManifestHash {
		t.Fatalf("source manifest hash should change when shard paths change: %q", first.SourceManifestHash)
	}
}

func TestAggregateRejectsPathOverlapAndDuplicateDispositions(t *testing.T) {
	profile := contextprofile.Profile{Files: []contextprofile.FileProfile{{Path: "internal/a.go", DiffBytes: 10}}}
	plan := singleShardPlan("source-hash", "internal/a.go")
	manifest := Build(Input{
		Profile:   profile,
		ShardPlan: plan,
		PathDispositions: []PathDisposition{
			{Path: "internal/a.go", Disposition: DispositionOutOfScope, Rationale: "caller excluded duplicate source path"},
			{Path: "docs/generated.md", Disposition: DispositionGenerated, Rationale: "generated review output"},
			{Path: "docs/generated.md", Disposition: DispositionGenerated, Rationale: "generated review output duplicate"},
		},
	})

	aggregate := Aggregate(manifest)

	assertBlocker(t, aggregate, "internal/a.go has both source coverage and disposition")
	assertBlocker(t, aggregate, "duplicate disposition for docs/generated.md")
}

func TestAggregateRequiresFreshEvidenceBackedShardResults(t *testing.T) {
	profile := contextprofile.Profile{Files: []contextprofile.FileProfile{{Path: "internal/a.go", DiffBytes: 10}}}
	plan := singleShardPlan("source-hash", "internal/a.go")
	manifest := Build(Input{Profile: profile, ShardPlan: plan})

	aggregate := Aggregate(manifest)
	assertBlocker(t, aggregate, "missing shard result for shard-01")

	manifest.ShardResults = []ReviewResult{passingShardResult("shard-01", "stale-hash", "internal/a.go")}
	aggregate = Aggregate(manifest)
	assertBlocker(t, aggregate, "stale shard result shard-01")

	manifest.ShardResults = []ReviewResult{{
		SchemaVersion:      1,
		ID:                 "result-01",
		ShardID:            "shard-01",
		Verdict:            "APPROVED",
		SourceManifestHash: manifest.SourceManifestHash,
		Reviewer:           "shard-reviewer",
		CoveredPaths:       []string{"internal/a.go"},
		Evidence:           []EvidenceRef{{Path: "internal/a.go", Line: 12, Note: "acceptance covered"}},
	}}
	aggregate = Aggregate(manifest)
	assertBlocker(t, aggregate, "unknown verdict")

	manifest.ShardResults = []ReviewResult{passingShardResult("shard-01", manifest.SourceManifestHash, "internal/a.go")}
	manifest.ShardResults[0].Evidence = []EvidenceRef{{}}
	aggregate = Aggregate(manifest)
	assertBlocker(t, aggregate, "missing provenance or coverage evidence")

	manifest.ShardResults = []ReviewResult{passingShardResult("shard-01", manifest.SourceManifestHash, "internal/a.go", "internal/unreviewed.go")}
	aggregate = Aggregate(manifest)
	assertBlocker(t, aggregate, "shard result shard-01 covers unknown path internal/unreviewed.go")

	manifest.ShardResults = []ReviewResult{passingShardResult("shard-01", manifest.SourceManifestHash, "internal/a.go")}
	manifest.ShardResults[0].Findings = []ResultFinding{{Severity: SeverityMedium, Disposition: DispositionOpen, Evidence: []EvidenceRef{{Path: "internal/a.go", Line: 12, Note: "bug"}}}}
	aggregate = Aggregate(manifest)
	assertBlocker(t, aggregate, "unresolved medium finding")

	manifest.ShardResults[0].Findings[0].Disposition = DispositionAcceptedRisk
	aggregate = Aggregate(manifest)
	assertNoBlocker(t, aggregate, "unresolved medium finding")
	if aggregate.Verdict != VerdictPass {
		t.Fatalf("Verdict = %q, want %q; blockers=%v", aggregate.Verdict, VerdictPass, aggregate.Blockers)
	}
}

func TestAggregateRejectsDuplicateAndUnexpectedShardResults(t *testing.T) {
	profile := contextprofile.Profile{Files: []contextprofile.FileProfile{{Path: "internal/a.go", DiffBytes: 10}}}
	manifest := Build(Input{Profile: profile, ShardPlan: singleShardPlan("source-hash", "internal/a.go")})
	manifest.ShardResults = []ReviewResult{
		passingShardResult("shard-01", manifest.SourceManifestHash, "internal/a.go"),
		passingShardResult("shard-01", manifest.SourceManifestHash, "internal/a.go"),
		passingShardResult("shard-99", manifest.SourceManifestHash, "internal/a.go"),
		passingShardResult("", manifest.SourceManifestHash, "internal/a.go"),
	}

	aggregate := Aggregate(manifest)

	assertBlocker(t, aggregate, "duplicate shard result for shard-01")
	assertBlocker(t, aggregate, "unexpected shard result for shard-99")
	assertBlocker(t, aggregate, "shard result missing shard ID")
}

func TestAggregateRequiresFreshCrossShardReviewForMultiShardManifest(t *testing.T) {
	profile := contextprofile.Profile{
		Files: []contextprofile.FileProfile{
			{Path: "internal/a.go", DiffBytes: 10},
			{Path: "internal/b.go", DiffBytes: 10},
		},
	}
	plan := contextprofile.ShardPlan{
		SourceDiffHash: "source-hash",
		Shards: []contextprofile.Shard{
			{ID: "shard-01", SourceDiffHash: "source-hash", Paths: []string{"internal/a.go"}, ByteCount: 10},
			{ID: "shard-02", SourceDiffHash: "source-hash", Paths: []string{"internal/b.go"}, ByteCount: 10},
		},
	}
	manifest := Build(Input{Profile: profile, ShardPlan: plan})
	manifest.ShardResults = []ReviewResult{
		passingShardResult("shard-01", manifest.SourceManifestHash, "internal/a.go"),
		passingShardResult("shard-02", manifest.SourceManifestHash, "internal/b.go"),
	}

	aggregate := Aggregate(manifest)
	assertBlocker(t, aggregate, "missing cross-shard result")

	manifest.CrossShardResult = &ReviewResult{
		SchemaVersion:      1,
		ID:                 "cross-01",
		ShardID:            CrossShardID,
		Verdict:            VerdictPass,
		SourceManifestHash: manifest.SourceManifestHash,
		Reviewer:           "cross-shard-reviewer",
		CoveredShardIDs:    []string{"shard-01"},
		Evidence:           []EvidenceRef{{Path: "internal/a.go", Line: 1, Note: "integration checked"}},
	}
	aggregate = Aggregate(manifest)
	assertBlocker(t, aggregate, "cross-shard result does not cover shard-02")

	manifest.CrossShardResult.CoveredShardIDs = []string{"shard-01", "shard-02"}
	aggregate = Aggregate(manifest)
	if aggregate.Verdict != VerdictPass {
		t.Fatalf("Verdict = %q, want %q; blockers=%v", aggregate.Verdict, VerdictPass, aggregate.Blockers)
	}

	manifest.CrossShardResult.CoveredShardIDs = []string{"shard-01", "shard-02", "shard-99"}
	aggregate = Aggregate(manifest)
	assertBlocker(t, aggregate, "cross-shard result covers unknown shard shard-99")
}

func TestAggregateValidatesSuppliedCrossShardResultForSingleShardManifest(t *testing.T) {
	profile := contextprofile.Profile{Files: []contextprofile.FileProfile{{Path: "internal/a.go", DiffBytes: 10}}}
	manifest := Build(Input{Profile: profile, ShardPlan: singleShardPlan("source-hash", "internal/a.go")})
	manifest.ShardResults = []ReviewResult{passingShardResult("shard-01", manifest.SourceManifestHash, "internal/a.go")}
	manifest.CrossShardResult = &ReviewResult{
		SchemaVersion:      1,
		ID:                 "cross-01",
		ShardID:            CrossShardID,
		Verdict:            VerdictPass,
		SourceManifestHash: "stale",
		Reviewer:           "cross-shard-reviewer",
		CoveredShardIDs:    []string{"shard-01"},
		Evidence:           []EvidenceRef{{Path: "internal/a.go", Line: 1, Note: "integration checked"}},
	}

	aggregate := Aggregate(manifest)

	assertBlocker(t, aggregate, "cross-shard result is stale")
}

func TestMarkdownRendersStaticOnlyReviewManifest(t *testing.T) {
	profile := contextprofile.Profile{Files: []contextprofile.FileProfile{{Path: "internal/a.go", DiffBytes: 10}}}
	manifest := Build(Input{Profile: profile, ShardPlan: singleShardPlan("source-hash", "internal/a.go")})
	aggregate := Aggregate(manifest)
	body := Markdown(manifest, aggregate)

	for _, required := range []string{
		"## Review Manifest",
		"Manifest verdict:",
		"Runtime assessment: static-only; runtime not assessed",
		"internal/a.go",
		"missing shard result for shard-01",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("manifest markdown missing %q:\n%s", required, body)
		}
	}
}

func singleShardPlan(sourceHash, path string) contextprofile.ShardPlan {
	return contextprofile.ShardPlan{
		SourceDiffHash: sourceHash,
		Shards: []contextprofile.Shard{{
			ID:             "shard-01",
			SourceDiffHash: sourceHash,
			Paths:          []string{path},
			ByteCount:      100,
		}},
	}
}

func passingShardResult(shardID, sourceManifestHash string, paths ...string) ReviewResult {
	return ReviewResult{
		SchemaVersion:      1,
		ID:                 "result-" + shardID,
		ShardID:            shardID,
		Verdict:            VerdictPass,
		SourceManifestHash: sourceManifestHash,
		Reviewer:           "shard-reviewer",
		CoveredPaths:       paths,
		Evidence:           []EvidenceRef{{Path: paths[0], Line: 12, Note: "acceptance covered"}},
	}
}

func assertBlocker(t *testing.T, aggregate AggregateResult, want string) {
	t.Helper()
	for _, blocker := range aggregate.Blockers {
		if strings.Contains(blocker, want) {
			return
		}
	}
	t.Fatalf("missing blocker %q in %+v", want, aggregate.Blockers)
}

func assertNoBlocker(t *testing.T, aggregate AggregateResult, unwanted string) {
	t.Helper()
	for _, blocker := range aggregate.Blockers {
		if strings.Contains(blocker, unwanted) {
			t.Fatalf("unexpected blocker %q in %+v", unwanted, aggregate.Blockers)
		}
	}
}
