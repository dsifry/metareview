package reviewmanifest

import (
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/contextprofile"
)

func TestIsResultFileName(t *testing.T) {
	valid := []string{
		"shard-1.0123456789abcdef.result.json",
		"shard-1a2.0123456789abcdef.result.json",
		"shard-1-2.0123456789abcdef.result.json",
		"cross-shard.0123456789abcdef.result.json",
	}
	for _, name := range valid {
		if !IsResultFileName(name) {
			t.Errorf("expected %q to be a result file name", name)
		}
	}
	invalid := []string{
		"random.txt",
		"shard-1.short.result.json",
		"shard-1.0123456789abcdef.json",
		"",
	}
	for _, name := range invalid {
		if IsResultFileName(name) {
			t.Errorf("expected %q not to be a result file name", name)
		}
	}
}

func TestMatchShardEmptyHash(t *testing.T) {
	plan := contextprofile.ShardPlan{Shards: []contextprofile.Shard{{ID: "1", Hash: "h1"}}}
	if _, ok := MatchShard(plan, ReviewResult{ShardHash: "  "}); ok {
		t.Error("a blank shard hash must not match any shard")
	}
	if _, ok := MatchShard(plan, ReviewResult{ShardHash: "nope"}); ok {
		t.Error("an unknown shard hash must not match")
	}
	if shard, ok := MatchShard(plan, ReviewResult{ShardHash: "h1"}); !ok || shard.ID != "1" {
		t.Errorf("expected to match shard 1, got %+v ok=%v", shard, ok)
	}
}

func TestBuildCarriesCrossShardResult(t *testing.T) {
	cross := &ReviewResult{ID: "cross-shard.abc", Kind: KindCrossShard, PlanHash: "ph", Verdict: VerdictPass}
	m := Build(Input{Scope: "task-done", CrossShardResult: cross})
	if m.CrossShardResult == nil || m.CrossShardResult.ID != "cross-shard.abc" {
		t.Fatalf("cross-shard result not carried into the manifest: %+v", m.CrossShardResult)
	}
	// It must be a copy, not the caller's pointer.
	if m.CrossShardResult == cross {
		t.Error("Build must copy the cross-shard result, not alias the caller's pointer")
	}
}

func TestMarkdownRendersOptionalSections(t *testing.T) {
	m := Manifest{
		SourcePaths:      []string{"src/a.go"},
		LocalPaths:       []string{"src/local.go"},
		PathDispositions: []PathDisposition{{Path: "docs/x.md", Disposition: DispositionGenerated, Rationale: "generated review artifact excluded"}},
		ShardResults:     []ReviewResult{{ShardID: "shard-1", Verdict: VerdictPass, Reviewer: "r"}},
		CrossShardResult: &ReviewResult{Verdict: VerdictPass, Reviewer: "r"},
	}
	agg := AggregateResult{Verdict: VerdictPass, Ignored: []IgnoredResult{{Path: "old.json", Reason: "stale"}}}
	out := Markdown(m, agg)
	for _, want := range []string{
		"### Local changes (not sharded)", "src/local.go",
		"### Path Dispositions", "docs/x.md: generated",
		"### Shard Results", "### Ignored Result Files", "old.json",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Markdown missing %q:\n%s", want, out)
		}
	}
}

func TestPathDispositionBlockersEdgeCases(t *testing.T) {
	m := Manifest{
		// The blank-path disposition is itself INVALID (bogus disposition): without the blank-path
		// skip it would emit "unknown path disposition for " (empty path). The docs/x.md entry has
		// an unknown disposition and a short rationale, both of which must be flagged.
		PathDispositions: []PathDisposition{
			{Path: "", Disposition: "bogus", Rationale: "short"},
			{Path: "docs/x.md", Disposition: "bogus", Rationale: "short"},
		},
	}
	blockers := pathDispositionBlockers(m)
	joined := strings.Join(blockers, "\n")
	if !strings.Contains(joined, "unknown path disposition for docs/x.md") {
		t.Errorf("expected an unknown-disposition blocker: %v", blockers)
	}
	if !strings.Contains(joined, "invalid disposition rationale for docs/x.md") {
		t.Errorf("expected an invalid-rationale blocker: %v", blockers)
	}
	for _, b := range blockers {
		// The blank-path disposition must be skipped before any validation, so no blocker may name
		// an empty path (which would render with a trailing "for " and nothing after).
		if strings.HasSuffix(b, "for ") || strings.Contains(b, "for :") {
			t.Errorf("a blank-path disposition must be skipped, not flagged: %v", blockers)
		}
	}
}

func TestSourceAssignmentBlockersNonSourceChunk(t *testing.T) {
	m := Manifest{
		SourcePaths: []string{"src/a.go"},
		ShardPlan: contextprofile.ShardPlan{Shards: []contextprofile.Shard{
			{ID: "1", Chunks: []contextprofile.Chunk{
				{Path: "src/a.go", Part: 1},
				{Path: "src/not-source.go", Part: 1},
			}},
		}},
	}
	blockers := sourceAssignmentBlockers(m)
	if !strings.Contains(strings.Join(blockers, "\n"), "includes non-source path src/not-source.go") {
		t.Errorf("expected a non-source-path blocker: %v", blockers)
	}
}

func TestCrossShardBlockersSingleShardWrongPlan(t *testing.T) {
	// A single-shard plan with a cross-shard result for the wrong plan hash: the result is ignored
	// and, because it is single-shard, no "missing cross-shard result" blocker is raised.
	m := Manifest{
		ShardPlan:        contextprofile.ShardPlan{PlanHash: "current", Shards: []contextprofile.Shard{{ID: "1"}}},
		CrossShardResult: &ReviewResult{PlanHash: "stale", Path: "cross.json"},
	}
	blockers, ignored, present := crossShardBlockers(m)
	if present {
		t.Error("a wrong-plan cross-shard result must not count as present")
	}
	if len(blockers) != 0 {
		t.Errorf("single-shard plan must raise no missing-cross-shard blocker: %v", blockers)
	}
	if len(ignored) != 1 || ignored[0].Path != "cross.json" {
		t.Errorf("expected the stale cross-shard result to be ignored: %v", ignored)
	}
}

func TestCanonicalReviewResultsSort(t *testing.T) {
	in := []ReviewResult{
		{ShardID: "shard-2", ID: "b"},
		{ShardID: "shard-1", ID: "z"},
		{ShardID: "shard-1", ID: "a"},
	}
	out := canonicalReviewResults(in)
	// Sorted by ShardID, then ID within the same ShardID.
	if out[0].ShardID != "shard-1" || out[0].ID != "a" {
		t.Errorf("expected shard-1/a first, got %+v", out[0])
	}
	if out[1].ShardID != "shard-1" || out[1].ID != "z" {
		t.Errorf("expected shard-1/z second (ID tie-break), got %+v", out[1])
	}
	if out[2].ShardID != "shard-2" {
		t.Errorf("expected shard-2 last, got %+v", out[2])
	}
}

func TestValidPathDisposition(t *testing.T) {
	for _, ok := range []string{DispositionGenerated, DispositionOutOfScope} {
		if !validPathDisposition(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	if validPathDisposition("bogus") {
		t.Error("an unknown disposition must be invalid")
	}
}

func TestValidRationale(t *testing.T) {
	if validRationale("short") {
		t.Error("a rationale under 12 characters must be invalid")
	}
	if !validRationale("a sufficiently descriptive rationale") {
		t.Error("a rationale of 12+ characters must be valid")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "  ", "x"); got != "x" {
		t.Errorf("expected first non-blank 'x', got %q", got)
	}
	if got := firstNonEmpty("", "   "); got != "" {
		t.Errorf("all-blank should return empty, got %q", got)
	}
}

func TestShardedReviewMarkdownEmpty(t *testing.T) {
	if got := ShardedReviewMarkdown(Manifest{}, AggregateResult{}); got != "" {
		t.Errorf("nothing read should render empty, got %q", got)
	}
}

func TestShardedReviewMarkdownRendersRowsAndChunks(t *testing.T) {
	m := Manifest{
		SourceManifestHash: "plan-hash",
		ShardResults:       []ReviewResult{{ShardID: "shard-1", ShardHash: "h1", Verdict: VerdictPass, Reviewer: "r", BlockingCount: 0, Path: "shard-1.h.result.json"}},
		CrossShardResult:   &ReviewResult{PlanHash: "plan-hash", Verdict: VerdictPass, Reviewer: "r", Path: "cross.json"},
		UnreadableResults:  []string{"broken.json"},
		ShardPlan: contextprofile.ShardPlan{Shards: []contextprofile.Shard{
			{ID: "1", Chunks: []contextprofile.Chunk{
				{Path: "src/big.go", Part: 1, Parts: 3},
				{Path: "src/big.go", Part: 2, Parts: 3},
				{Path: "src/small.go", Part: 1, Parts: 1}, // single-part: excluded from the chunk list
			}},
		}},
	}
	agg := AggregateResult{ShardsCovered: 1, ShardCount: 1, Ignored: []IgnoredResult{{Path: "old.json", Reason: "stale"}}}
	out := ShardedReviewMarkdown(m, agg)
	for _, want := range []string{
		"## Sharded Review", "Plan hash:", "cross-shard",
		"### Ignored result files", "old.json",
		"### Unreadable result files", "broken.json",
		"### Files reviewed as chunks", "src/big.go", "3 parts across shard-1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("ShardedReviewMarkdown missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "src/small.go") {
		t.Errorf("a single-part file must not appear in the chunk list:\n%s", out)
	}
}

func TestAppendUnique(t *testing.T) {
	got := appendUnique([]string{"a", "b"}, "b")
	if len(got) != 2 {
		t.Errorf("appendUnique must not add a duplicate: %v", got)
	}
	got = appendUnique([]string{"a"}, "c")
	if len(got) != 2 || got[1] != "c" {
		t.Errorf("appendUnique must add a new value: %v", got)
	}
}
