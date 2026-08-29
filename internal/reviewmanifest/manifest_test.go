package reviewmanifest

import (
	"encoding/json"
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
	plan := singleShardPlan("internal/reviewmanifest/manifest.go")

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

func TestAggregateRejectsPathOverlapAndDuplicateDispositions(t *testing.T) {
	profile := contextprofile.Profile{Files: []contextprofile.FileProfile{{Path: "internal/a.go", DiffBytes: 10}}}
	manifest := Build(Input{
		Profile:   profile,
		ShardPlan: singleShardPlan("internal/a.go"),
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

func TestManifestHashIsPlanHash(t *testing.T) {
	profile := contextprofile.Profile{Files: []contextprofile.FileProfile{{Path: "internal/a.go", DiffBytes: 10}}}
	plan := singleShardPlan("internal/a.go")

	manifest := Build(Input{Profile: profile, ShardPlan: plan})

	if manifest.SourceManifestHash != plan.PlanHash {
		t.Fatalf("SourceManifestHash = %q, want plan hash %q", manifest.SourceManifestHash, plan.PlanHash)
	}
}

func TestManifestHashExcludesGeneratedPathsAndDispositions(t *testing.T) {
	profile := contextprofile.Profile{
		Files:                  []contextprofile.FileProfile{{Path: "internal/a.go", DiffBytes: 10}},
		GeneratedExcludedFiles: []string{"docs/metareview/context/a.md"},
	}
	plan := singleShardPlan("internal/a.go")
	bare := Build(Input{Profile: profile, ShardPlan: plan})
	withDispositions := Build(Input{
		Profile:   profile,
		ShardPlan: plan,
		PathDispositions: []PathDisposition{{
			Path:        "docs/metareview/context/a.md",
			Disposition: DispositionGenerated,
			Rationale:   "metareview generated context artifact",
		}},
	})

	if bare.SourceManifestHash != withDispositions.SourceManifestHash {
		t.Fatalf("manifest hash must not depend on generated paths or dispositions: %q vs %q",
			bare.SourceManifestHash, withDispositions.SourceManifestHash)
	}
}

func TestResultSchemaVersionDistinct(t *testing.T) {
	if ResultSchemaVersion != 1 {
		t.Fatalf("ResultSchemaVersion = %d, want 1", ResultSchemaVersion)
	}
	result := passingShardResult("shard-3a", "aaaaaaaaaaaaaaaa")
	result.SchemaVersion = ResultSchemaVersion + 1
	manifest := manifestWithShard(t, "aaaaaaaaaaaaaaaa")
	manifest.ShardResults = []ReviewResult{result}

	assertBlocker(t, Aggregate(manifest), "unsupported schema version")

	// Validation must read ResultSchemaVersion, not the manifest's own
	// SchemaVersion. The two are equal today, so only a result carrying
	// ResultSchemaVersion proves which constant the check uses.
	fresh := passingShardResult("shard-3a", "aaaaaaaaaaaaaaaa")
	fresh.SchemaVersion = ResultSchemaVersion
	accepted := manifestWithShard(t, "aaaaaaaaaaaaaaaa")
	accepted.ShardResults = []ReviewResult{fresh}
	for _, blocker := range Aggregate(accepted).Blockers {
		if strings.Contains(blocker, "unsupported schema version") {
			t.Fatalf("a result at ResultSchemaVersion must be accepted: %v", blocker)
		}
	}
}

func TestFreshByShardHash(t *testing.T) {
	manifest := manifestWithShard(t, "aaaaaaaaaaaaaaaa")
	manifest.ShardResults = []ReviewResult{passingShardResult("shard-3a", "aaaaaaaaaaaaaaaa")}

	aggregate := Aggregate(manifest)

	if aggregate.Verdict != VerdictPass {
		t.Fatalf("Verdict = %q, want %q; blockers=%v", aggregate.Verdict, VerdictPass, aggregate.Blockers)
	}
	if aggregate.ShardsCovered != 1 || aggregate.ShardCount != 1 {
		t.Fatalf("coverage = %d/%d, want 1/1", aggregate.ShardsCovered, aggregate.ShardCount)
	}
	if aggregate.PlanHash != manifest.ShardPlan.PlanHash {
		t.Fatalf("PlanHash = %q, want %q", aggregate.PlanHash, manifest.ShardPlan.PlanHash)
	}
}

func TestResultNeedsNoCoverageList(t *testing.T) {
	manifest := manifestWithShard(t, "aaaaaaaaaaaaaaaa")
	manifest.ShardResults = []ReviewResult{passingShardResult("shard-3a", "aaaaaaaaaaaaaaaa")}

	encoded, err := json.Marshal(manifest.ShardResults[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, absent := range []string{"coveredChunks", "coveredShardIds", "coveredPaths", "sourceManifestHash"} {
		if strings.Contains(string(encoded), absent) {
			t.Fatalf("result JSON still carries %q: %s", absent, encoded)
		}
	}
	if aggregate := Aggregate(manifest); aggregate.Verdict != VerdictPass {
		t.Fatalf("Verdict = %q, want %q; blockers=%v", aggregate.Verdict, VerdictPass, aggregate.Blockers)
	}
}

func TestUnmatchedResultIgnored(t *testing.T) {
	manifest := manifestWithShard(t, "aaaaaaaaaaaaaaaa")
	stale := passingShardResult("shard-3a", "bbbbbbbbbbbbbbbb")
	stale.Path = "docs/metareview/shards/task-done/t/shard-3a.bbbbbbbbbbbbbbbb.result.json"
	manifest.ShardResults = []ReviewResult{stale}

	aggregate := Aggregate(manifest)

	assertNoBlocker(t, aggregate, "stale")
	if len(aggregate.Ignored) != 1 || aggregate.Ignored[0].Reason == "" {
		t.Fatalf("want one ignored result with a reason, got %+v", aggregate.Ignored)
	}
	assertBlocker(t, aggregate, "missing shard result for shard-3a")
}

func TestOversizeResultIgnored(t *testing.T) {
	data := append([]byte(`{"schemaVersion":1,"note":"`), make([]byte, MaxResultBytes)...)
	_, reason, err := ParseResult(data)
	if err != nil {
		t.Fatalf("oversize file must be ignored, not unreadable: %v", err)
	}
	if reason == "" {
		t.Fatal("oversize result file must be ignored with a reason")
	}
}

func TestFilenameIdMismatchBlocks(t *testing.T) {
	manifest := manifestWithShard(t, "aaaaaaaaaaaaaaaa")
	result := passingShardResult("shard-3a", "aaaaaaaaaaaaaaaa")
	result.Path = "docs/metareview/shards/task-done/t/shard-3b.aaaaaaaaaaaaaaaa.result.json"
	manifest.ShardResults = []ReviewResult{result}

	assertBlocker(t, Aggregate(manifest), "does not match its file name")
}

func TestDuplicateShardResultBlocks(t *testing.T) {
	manifest := manifestWithShard(t, "aaaaaaaaaaaaaaaa")
	first := passingShardResult("shard-3a", "aaaaaaaaaaaaaaaa")
	second := passingShardResult("shard-3a", "aaaaaaaaaaaaaaaa")
	second.ID = "result-2"
	manifest.ShardResults = []ReviewResult{first, second}

	aggregate := Aggregate(manifest)

	assertBlocker(t, aggregate, "duplicate shard result shard-3a")
	if aggregate.ShardsCovered != 1 {
		t.Fatalf("ShardsCovered = %d, want 1 (distinct current shards)", aggregate.ShardsCovered)
	}
}

func TestCrossShardFreshByPlanHash(t *testing.T) {
	manifest := twoShardManifest(t)
	manifest.ShardResults = []ReviewResult{
		passingShardResult("shard-3a", manifest.ShardPlan.Shards[0].Hash),
		passingShardResult("shard-b1", manifest.ShardPlan.Shards[1].Hash),
	}
	stale := passingCrossShardResult("0000000000000000")
	manifest.CrossShardResult = &stale

	aggregate := Aggregate(manifest)
	assertBlocker(t, aggregate, "missing cross-shard result")
	if aggregate.CrossShard {
		t.Fatal("cross-shard result for another plan must not count")
	}

	fresh := passingCrossShardResult(manifest.ShardPlan.PlanHash)
	manifest.CrossShardResult = &fresh
	aggregate = Aggregate(manifest)
	if !aggregate.CrossShard || aggregate.Verdict != VerdictPass {
		t.Fatalf("fresh cross-shard result should pass: %+v", aggregate)
	}
}

func TestCrossShardRequiredOnlyForMultiShard(t *testing.T) {
	manifest := manifestWithShard(t, "aaaaaaaaaaaaaaaa")
	manifest.ShardResults = []ReviewResult{passingShardResult("shard-3a", "aaaaaaaaaaaaaaaa")}

	if aggregate := Aggregate(manifest); aggregate.Verdict != VerdictPass {
		t.Fatalf("single-shard plan must not require a cross-shard result: %+v", aggregate.Blockers)
	}

	two := twoShardManifest(t)
	two.ShardResults = []ReviewResult{
		passingShardResult("shard-3a", two.ShardPlan.Shards[0].Hash),
		passingShardResult("shard-b1", two.ShardPlan.Shards[1].Hash),
	}
	assertBlocker(t, Aggregate(two), "missing cross-shard result")
}

func TestWaivedMediumFindingBlocks(t *testing.T) {
	for _, disposition := range []string{DispositionWaived, DispositionAcceptedRisk, DispositionDeferred, DispositionOpen} {
		manifest := manifestWithShard(t, "aaaaaaaaaaaaaaaa")
		result := passingShardResult("shard-3a", "aaaaaaaaaaaaaaaa")
		result.Findings = []ResultFinding{{
			Severity:    SeverityMedium,
			Disposition: disposition,
			Note:        "medium severity issue",
			Evidence:    []EvidenceRef{{Path: "internal/a.go", Line: 3}},
		}}
		manifest.ShardResults = []ReviewResult{result}

		assertBlocker(t, Aggregate(manifest), "unresolved "+disposition+" finding")
	}
}

func TestFixedAndFalsePositiveClose(t *testing.T) {
	for _, disposition := range []string{DispositionFixed, DispositionFalsePositive} {
		manifest := manifestWithShard(t, "aaaaaaaaaaaaaaaa")
		result := passingShardResult("shard-3a", "aaaaaaaaaaaaaaaa")
		result.Findings = []ResultFinding{{
			Severity:    SeverityCritical,
			Disposition: disposition,
			Note:        "critical severity issue",
			Evidence:    []EvidenceRef{{Path: "internal/a.go", Line: 3}},
		}}
		manifest.ShardResults = []ReviewResult{result}

		if aggregate := Aggregate(manifest); aggregate.Verdict != VerdictPass {
			t.Fatalf("%s must close a finding: %+v", disposition, aggregate.Blockers)
		}
	}
}

func TestEvidenceRuleMatchesValidator(t *testing.T) {
	cases := []struct {
		ref  EvidenceRef
		want bool
	}{
		{EvidenceRef{Path: "internal/a.go", Line: 1}, true},
		{EvidenceRef{Path: "internal/a.go"}, false},
		{EvidenceRef{Note: "short"}, false},
		// The exact boundary in both directions.
		{EvidenceRef{Note: strings.Repeat("n", EvidenceNoteMinLength)}, true},
		{EvidenceRef{Note: strings.Repeat("n", EvidenceNoteMinLength-1)}, false},
	}
	for _, testCase := range cases {
		if got := evidenceRefValid(testCase.ref); got != testCase.want {
			t.Fatalf("evidenceRefValid(%+v) = %v, want %v", testCase.ref, got, testCase.want)
		}
		manifest := manifestWithShard(t, "aaaaaaaaaaaaaaaa")
		result := passingShardResult("shard-3a", "aaaaaaaaaaaaaaaa")
		result.Evidence = []EvidenceRef{testCase.ref}
		manifest.ShardResults = []ReviewResult{result}
		aggregate := Aggregate(manifest)
		if testCase.want {
			assertNoBlocker(t, aggregate, "missing evidence")
		} else {
			assertBlocker(t, aggregate, "missing evidence")
		}
	}
	if EvidenceNoteMinLength != 12 {
		t.Fatalf("EvidenceNoteMinLength = %d, want 12", EvidenceNoteMinLength)
	}
}

func TestLocalFilesNeverBlockAssignment(t *testing.T) {
	manifest := Build(Input{
		Profile: contextprofile.Profile{Files: []contextprofile.FileProfile{
			{Path: "internal/a.go", DiffBytes: 10, Source: contextprofile.SourceBranch},
			{Path: "local/staged.go", DiffBytes: 10, Source: contextprofile.SourceStaged},
			{Path: "local/new.go", DiffBytes: 10, Source: contextprofile.SourceUntracked},
		}},
		ShardPlan: singleShardPlan("internal/a.go"),
	})

	aggregate := Aggregate(manifest)

	assertNoBlocker(t, aggregate, "local/staged.go")
	assertNoBlocker(t, aggregate, "local/new.go")
	if len(manifest.LocalPaths) != 2 {
		t.Fatalf("LocalPaths = %v, want the two local files", manifest.LocalPaths)
	}
}

func TestChunkAssignedToExactlyOneShard(t *testing.T) {
	chunk := contextprofile.Chunk{Path: "internal/a.go", Part: 1, Parts: 1, ByteEnd: 10, Hash: "c1"}
	plan := contextprofile.ShardPlan{
		PlanHash: "0f0f0f0f0f0f0f0f",
		Shards: []contextprofile.Shard{
			{ID: "3a", Chunks: []contextprofile.Chunk{chunk}, Bytes: 10, Hash: "aaaaaaaaaaaaaaaa"},
			{ID: "b1", Chunks: []contextprofile.Chunk{chunk}, Bytes: 10, Hash: "bbbbbbbbbbbbbbbb"},
		},
	}
	manifest := Build(Input{
		Profile:   contextprofile.Profile{Files: []contextprofile.FileProfile{{Path: "internal/a.go", DiffBytes: 10}}},
		ShardPlan: plan,
	})

	assertBlocker(t, Aggregate(manifest), "internal/a.go part 1 assigned to multiple shards")

	manifest = Build(Input{
		Profile: contextprofile.Profile{Files: []contextprofile.FileProfile{
			{Path: "internal/a.go", DiffBytes: 10},
			{Path: "internal/b.go", DiffBytes: 10},
		}},
		ShardPlan: singleShardPlan("internal/a.go"),
	})
	assertBlocker(t, Aggregate(manifest), "internal/b.go is not assigned to a shard")
}

func TestUnreadableResultBlocks(t *testing.T) {
	manifest := manifestWithShard(t, "aaaaaaaaaaaaaaaa")
	manifest.ShardResults = []ReviewResult{passingShardResult("shard-3a", "aaaaaaaaaaaaaaaa")}
	manifest.UnreadableResults = []string{"docs/metareview/shards/task-done/t/shard-3a.deadbeefdeadbeef.result.json"}

	assertBlocker(t, Aggregate(manifest), "unreadable result file")
}

func TestMarkdownRendersResultsPlainText(t *testing.T) {
	manifest := manifestWithShard(t, "aaaaaaaaaaaaaaaa")
	result := passingShardResult("shard-3a", "aaaaaaaaaaaaaaaa")
	result.Reviewer = "agent\n| injected | row |"
	manifest.ShardResults = []ReviewResult{result}
	manifest.IgnoredResults = []IgnoredResult{{Path: "docs/x.json", Reason: "reason\nwith newline"}}

	body := Markdown(manifest, Aggregate(manifest))

	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "injected") && !strings.HasPrefix(line, "- ") {
			t.Fatalf("ingested string broke the layout: %q", line)
		}
	}
	for _, required := range []string{"## Review Manifest", "Manifest verdict:", "internal/a.go", "shard-3a"} {
		if !strings.Contains(body, required) {
			t.Fatalf("manifest markdown missing %q:\n%s", required, body)
		}
	}
}

func TestShardIDFromResultPath(t *testing.T) {
	cases := map[string]string{
		"docs/x/shard-3a.aaaaaaaaaaaaaaaa.result.json":    "shard-3a",
		"docs/x/shard-3a-2.aaaaaaaaaaaaaaaa.result.json":  "shard-3a-2",
		"docs/x/cross-shard.aaaaaaaaaaaaaaaa.result.json": CrossShardID,
		"docs/x/notes.md": "",
	}
	for path, want := range cases {
		if got := ShardIDFromResultPath(path); got != want {
			t.Fatalf("ShardIDFromResultPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestParseResultRejectsMalformedJSON(t *testing.T) {
	if _, _, err := ParseResult([]byte("{")); err == nil {
		t.Fatal("malformed JSON must be an error")
	}
	result, reason, err := ParseResult([]byte(`{"schemaVersion":1,"id":"r","kind":"shard"}`))
	if err != nil || reason != "" {
		t.Fatalf("valid JSON must parse: reason=%q err=%v", reason, err)
	}
	if result.ID != "r" || result.Kind != KindShard {
		t.Fatalf("unexpected parse: %+v", result)
	}
}

func TestAggregateValidatesResultFieldShape(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		mutate func(*ReviewResult)
		want   string
	}{
		{"missing id", func(r *ReviewResult) { r.ID = "" }, "missing result ID"},
		{"unknown kind", func(r *ReviewResult) { r.Kind = "sideways" }, "unknown kind"},
		{"unknown verdict", func(r *ReviewResult) { r.Verdict = "APPROVED" }, "unknown verdict"},
		{"missing reviewer", func(r *ReviewResult) { r.Reviewer = "" }, "missing reviewer"},
		{"bad time", func(r *ReviewResult) { r.ReviewedAt = "yesterday" }, "unparsable reviewedAt"},
		{"bad shard id", func(r *ReviewResult) { r.ShardID = "shard-zz"; r.Path = "" }, "invalid shard ID"},
		{"blocking count", func(r *ReviewResult) { r.BlockingCount = 2 }, "has blockers"},
		{"blocking verdict", func(r *ReviewResult) { r.Verdict = VerdictNeedsRevision }, "has blockers"},
		{"unknown severity", func(r *ReviewResult) {
			r.Findings = []ResultFinding{{Severity: "spicy", Disposition: DispositionFixed, Evidence: []EvidenceRef{{Path: "a", Line: 1}}}}
		}, "unknown severity"},
		{"unknown disposition", func(r *ReviewResult) {
			r.Findings = []ResultFinding{{Severity: SeverityLow, Disposition: "shrug", Evidence: []EvidenceRef{{Path: "a", Line: 1}}}}
		}, "unknown disposition"},
		{"finding evidence", func(r *ReviewResult) {
			r.Findings = []ResultFinding{{Severity: SeverityLow, Disposition: DispositionFixed}}
		}, "finding missing evidence"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			manifest := manifestWithShard(t, "aaaaaaaaaaaaaaaa")
			result := passingShardResult("shard-3a", "aaaaaaaaaaaaaaaa")
			testCase.mutate(&result)
			manifest.ShardResults = []ReviewResult{result}
			assertBlocker(t, Aggregate(manifest), testCase.want)
		})
	}
}

func TestCrossShardResultShapeValidated(t *testing.T) {
	manifest := twoShardManifest(t)
	manifest.ShardResults = []ReviewResult{
		passingShardResult("shard-3a", manifest.ShardPlan.Shards[0].Hash),
		passingShardResult("shard-b1", manifest.ShardPlan.Shards[1].Hash),
	}
	cross := passingCrossShardResult(manifest.ShardPlan.PlanHash)
	cross.ShardID = "shard-3a"
	cross.Path = "docs/x/cross-shard." + manifest.ShardPlan.PlanHash + ".result.json"
	manifest.CrossShardResult = &cross

	assertBlocker(t, Aggregate(manifest), "cross-shard result must not name a shard")
}

func TestGeneratedPathDispositionsAreCanonical(t *testing.T) {
	dispositions := GeneratedPathDispositions([]string{"b.md", "a.md", "a.md", " "})
	if len(dispositions) != 2 || dispositions[0].Path != "a.md" || dispositions[1].Path != "b.md" {
		t.Fatalf("unexpected dispositions: %+v", dispositions)
	}
	if dispositions[0].Disposition != DispositionGenerated {
		t.Fatalf("unexpected disposition: %+v", dispositions[0])
	}
}

func manifestWithShard(t *testing.T, hash string) Manifest {
	t.Helper()
	plan := singleShardPlan("internal/a.go")
	plan.Shards[0].Hash = hash
	return Build(Input{
		Profile:   contextprofile.Profile{Files: []contextprofile.FileProfile{{Path: "internal/a.go", DiffBytes: 10}}},
		ShardPlan: plan,
	})
}

func twoShardManifest(t *testing.T) Manifest {
	t.Helper()
	plan := contextprofile.ShardPlan{
		PlanHash: "0f0f0f0f0f0f0f0f",
		Shards: []contextprofile.Shard{
			{ID: "3a", Bytes: 10, Hash: "aaaaaaaaaaaaaaaa", Chunks: []contextprofile.Chunk{
				{Path: "internal/a.go", Part: 1, Parts: 1, ByteEnd: 10, Hash: "c1"},
			}},
			{ID: "b1", Bytes: 10, Hash: "bbbbbbbbbbbbbbbb", Chunks: []contextprofile.Chunk{
				{Path: "internal/b.go", Part: 1, Parts: 1, ByteEnd: 10, Hash: "c2"},
			}},
		},
	}
	return Build(Input{
		Profile: contextprofile.Profile{Files: []contextprofile.FileProfile{
			{Path: "internal/a.go", DiffBytes: 10},
			{Path: "internal/b.go", DiffBytes: 10},
		}},
		ShardPlan: plan,
	})
}

func singleShardPlan(path string) contextprofile.ShardPlan {
	return contextprofile.ShardPlan{
		SourceDiffHash: "0e0e0e0e0e0e0e0e",
		PlanHash:       "0f0f0f0f0f0f0f0f",
		Shards: []contextprofile.Shard{{
			ID:     "3a",
			Bytes:  100,
			Hash:   "aaaaaaaaaaaaaaaa",
			Chunks: []contextprofile.Chunk{{Path: path, Part: 1, Parts: 1, ByteEnd: 100, Hash: "chunk-1"}},
		}},
	}
}

func passingShardResult(shardID, shardHash string) ReviewResult {
	return ReviewResult{
		SchemaVersion: ResultSchemaVersion,
		ID:            "result-" + shardID,
		Kind:          KindShard,
		ShardID:       shardID,
		ShardHash:     shardHash,
		PlanHash:      "0f0f0f0f0f0f0f0f",
		Verdict:       VerdictPass,
		Reviewer:      "shard-reviewer",
		ReviewedAt:    "2026-08-27T10:00:00Z",
		Evidence:      []EvidenceRef{{Path: "internal/a.go", Line: 12, Note: "acceptance covered"}},
		Path:          "docs/metareview/shards/task-done/t/" + shardID + "." + shardHash + ".result.json",
	}
}

func passingCrossShardResult(planHash string) ReviewResult {
	return ReviewResult{
		SchemaVersion: ResultSchemaVersion,
		ID:            "cross-1",
		Kind:          KindCrossShard,
		PlanHash:      planHash,
		Verdict:       VerdictPass,
		Reviewer:      "cross-shard-reviewer",
		ReviewedAt:    "2026-08-27T10:00:00Z",
		Evidence:      []EvidenceRef{{Note: "integration seams checked"}},
		Path:          "docs/metareview/shards/task-done/t/cross-shard." + planHash + ".result.json",
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

// TestShardedReviewMarkdownNeutralisesResultDerivedText pins the reason the
// ingested helper exists. The `## Sharded Review` section is written into the
// review log, and reviewlog harvests every mrvf- token it finds there as one of
// the run's finding IDs. Any string that came out of a result file — the
// reviewer name, an ignore reason, and the file paths themselves — must be
// neutralised, or a result file can inject finding IDs into the run's record.
func TestShardedReviewMarkdownNeutralisesResultDerivedText(t *testing.T) {
	manifest := manifestWithShard(t, "aaaaaaaaaaaaaaaa")
	result := passingShardResult("shard-3a", "aaaaaaaaaaaaaaaa")
	result.Reviewer = "mrvf-injected-reviewer"
	result.Path = "docs/metareview/shards/mrvf-injected-path.result.json"
	manifest.ShardResults = []ReviewResult{result}
	manifest.UnreadableResults = []string{"docs/metareview/shards/mrvf-injected-unreadable.json"}
	aggregate := Aggregate(manifest)
	aggregate.Ignored = []IgnoredResult{{
		Path:   "docs/metareview/shards/mrvf-injected-ignored.json",
		Reason: "mrvf-injected-reason",
	}}

	rendered := ShardedReviewMarkdown(manifest, aggregate)
	if strings.Contains(rendered, "mrvf-") {
		t.Fatalf("the review log carries a harvestable mrvf- token:\n%s", rendered)
	}
	// The text still has to be readable, just not harvestable.
	if !strings.Contains(rendered, "mrvf_injected-reviewer") {
		t.Fatalf("the reviewer name was dropped rather than neutralised:\n%s", rendered)
	}
}

// TestManifestMarkdownNeutralisesResultDerivedText holds the same rule for the
// context pack, so the two renderers cannot drift apart.
func TestManifestMarkdownNeutralisesResultDerivedText(t *testing.T) {
	manifest := manifestWithShard(t, "aaaaaaaaaaaaaaaa")
	result := passingShardResult("shard-3a", "aaaaaaaaaaaaaaaa")
	result.Reviewer = "mrvf-injected-reviewer"
	manifest.ShardResults = []ReviewResult{result}
	aggregate := Aggregate(manifest)
	aggregate.Ignored = []IgnoredResult{{Path: "a.json", Reason: "mrvf-injected-reason"}}

	if rendered := Markdown(manifest, aggregate); strings.Contains(rendered, "mrvf-") {
		t.Fatalf("the context pack carries a harvestable mrvf- token:\n%s", rendered)
	}
}

// TestManifestHashOmittedWithoutAPlan keeps the context pack honest for an
// unsharded review: PlanShards returns an empty plan, so there is no plan hash
// to report and the field must not render as an empty value.
func TestManifestHashOmittedWithoutAPlan(t *testing.T) {
	manifest := Build(Input{Scope: "task-done", Target: map[string]string{"type": "task", "id": "t-1"}})
	if manifest.SourceManifestHash != "" {
		t.Fatalf("SourceManifestHash = %q, want empty without a plan", manifest.SourceManifestHash)
	}
	rendered := Markdown(manifest, Aggregate(manifest))
	if strings.Contains(rendered, "Source manifest hash: ``") {
		t.Fatalf("an empty hash is rendered as a value:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Source manifest hash: not sharded") {
		t.Fatalf("the unsharded case is not stated:\n%s", rendered)
	}
}

// TestRenderersNeutraliseShardIDAndVerdict closes the rest of the injection
// surface. ShardID and Verdict are decoded from the result file, and a result
// is accepted by its content hash before Aggregate reports invalid metadata, so
// both fields reach the review log even on a blocking run.
func TestRenderersNeutraliseShardIDAndVerdict(t *testing.T) {
	manifest := manifestWithShard(t, "aaaaaaaaaaaaaaaa")
	result := passingShardResult("shard-3a", "aaaaaaaaaaaaaaaa")
	result.ShardID = "mrvf-injected-shard"
	result.Verdict = "mrvf-injected-verdict"
	manifest.ShardResults = []ReviewResult{result}
	aggregate := Aggregate(manifest)

	for name, rendered := range map[string]string{
		"review log":   ShardedReviewMarkdown(manifest, aggregate),
		"context pack": Markdown(manifest, aggregate),
	} {
		if strings.Contains(rendered, "mrvf-") {
			t.Fatalf("%s carries a harvestable mrvf- token:\n%s", name, rendered)
		}
	}
}

// A result file is written by an agent, and the `## Sharded Review` table is the
// durable record of what that agent said. A pipe in any ingested string forges
// extra cells: a reviewer name of "evil | PASS | 0 | docs/fake.json" shifts the
// Verdict, Blocking and File columns, so a result can rewrite its own blocking
// count in the committed log. GFM splits table rows on | before it parses inline
// code, so wrapping a value in backticks does not contain it.
func TestShardedReviewMarkdownCannotForgeTableCells(t *testing.T) {
	manifest := manifestWithShard(t, "aaaaaaaaaaaaaaaa")
	result := passingShardResult("shard-3a", "aaaaaaaaaaaaaaaa")
	result.Reviewer = "evil | PASS | 0 | docs/fake.json"
	result.Verdict = "NEEDS_REVISION | forged"
	result.Path = "docs/real.json | docs/forged.json"
	result.BlockingCount = 4
	manifest.ShardResults = []ReviewResult{result}
	aggregate := Aggregate(manifest)

	rendered := ShardedReviewMarkdown(manifest, aggregate)

	var row string
	for _, line := range strings.Split(rendered, "\n") {
		if strings.Contains(line, "shard-3a") && strings.HasPrefix(line, "|") {
			row = line
			break
		}
	}
	if row == "" {
		t.Fatalf("no row for the shard:\n%s", rendered)
	}
	// Six columns means seven pipes. Every additional unescaped pipe is a cell
	// the result file invented.
	cells := 0
	for i, r := range row {
		if r == '|' && (i == 0 || row[i-1] != '\\') {
			cells++
		}
	}
	if cells != 7 {
		t.Fatalf("result-supplied text forged %d extra cell boundaries:\n%s", cells-7, row)
	}
	// And the real blocking count must still be the one rendered.
	if !strings.Contains(row, "| 4 |") {
		t.Fatalf("the true blocking count is not in the row:\n%s", row)
	}
}

// A file both committed on the branch and dirty locally must be disclosed as
// local: its committed bytes are in a shard pack, its uncommitted ones are in no
// pack at all. Classifying it purely as a branch file hid that second fact, so
// unreviewed bytes never appeared in the manifest's local disclosure.
func TestLocalPathsIncludeABranchFileWithUncommittedBytes(t *testing.T) {
	profile := contextprofile.Profile{Files: []contextprofile.FileProfile{
		{Path: "src/branch-only.go", Source: contextprofile.SourceBranch, DiffBytes: 100},
		{Path: "src/both.go", Source: contextprofile.SourceBranch, DiffBytes: 400, LocalBytes: 40},
		{Path: "src/worktree-only.go", Source: contextprofile.SourceWorktree, DiffBytes: 20},
	}}

	local := profilePaths(profile, false)
	if !contains(local, "src/both.go") {
		t.Fatalf("a branch file with uncommitted bytes must be disclosed as local: %v", local)
	}
	if !contains(local, "src/worktree-only.go") {
		t.Fatalf("a purely local file must still be disclosed: %v", local)
	}
	if contains(local, "src/branch-only.go") {
		t.Fatalf("a clean branch file is not local: %v", local)
	}

	branch := profilePaths(profile, true)
	if !contains(branch, "src/both.go") || !contains(branch, "src/branch-only.go") {
		t.Fatalf("branch files must still be listed as branch files: %v", branch)
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// The rule the packs publish is "each entry", and the validator was an any().
// TestEvidenceRuleMatchesValidator could not see the difference because it only
// ever built one-element slices — the single shape where the two rules agree.
// These are the multi-entry shapes that tell them apart.
func TestEvidenceRuleAppliesToEveryEntry(t *testing.T) {
	good := EvidenceRef{Path: "internal/a.go", Line: 12}
	bad := EvidenceRef{Path: "internal/a.go"} // no line, no note
	cases := []struct {
		name string
		refs []EvidenceRef
		want bool
	}{
		{"none at all", nil, false},
		{"one good", []EvidenceRef{good}, true},
		{"one bad", []EvidenceRef{bad}, false},
		{"all good", []EvidenceRef{good, good, good}, true},
		{"good then bad", []EvidenceRef{good, bad}, false},
		{"bad then good", []EvidenceRef{bad, good}, false},
		{"one good among many bad", []EvidenceRef{bad, bad, good, bad}, false},
	}
	for _, c := range cases {
		if got := hasValidEvidence(c.refs); got != c.want {
			t.Fatalf("%s: hasValidEvidence = %v, want %v", c.name, got, c.want)
		}
	}
}
