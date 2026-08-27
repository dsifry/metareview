package prready

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/contextprofile"
	"github.com/dsifry/metareview/internal/reviewlog"
	"github.com/dsifry/metareview/internal/reviewmanifest"
	"github.com/dsifry/metareview/internal/shardpack"
)

// planFor runs the review once with a recording writer to obtain the plan the
// real run will compute, without writing any packs.
func planFor(t *testing.T, root string) contextprofile.ShardPlan {
	t.Helper()
	writer := &fakeWriter{}
	if _, err := Create(root, Options{Base: "main", ShardWriter: writer}); err != nil {
		t.Fatal(err)
	}
	if len(writer.lastPlan.Shards) == 0 {
		t.Fatal("fixture produced no shards")
	}
	return writer.lastPlan
}

func writeResult(t *testing.T, dir, name string, result reviewmanifest.ReviewResult) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestReviewLogListsIngestedAndIgnoredResults renders the §6 section and parses
// the log back: the verdict token must still be read, and a reviewer string
// carrying an mrvf--shaped token must not contaminate the run's finding IDs.
func TestReviewLogListsIngestedAndIgnoredResults(t *testing.T) {
	root := shardedRepo(t)
	plan := planFor(t, root)
	dir := shardpack.ResultsDir(root, "pr-ready", "feature")
	for _, shard := range plan.Shards {
		writeResult(t, dir, "shard-"+shard.ID+"."+shard.Hash+".result.json", reviewmanifest.ReviewResult{
			SchemaVersion: reviewmanifest.ResultSchemaVersion,
			ID:            "r-" + shard.ID,
			Kind:          reviewmanifest.KindShard,
			ShardID:       "shard-" + shard.ID,
			ShardHash:     shard.Hash,
			PlanHash:      plan.PlanHash,
			Verdict:       reviewmanifest.VerdictPass,
			Reviewer:      "agent mrvf-not-a-real-finding-id",
			ReviewedAt:    "2026-08-27T10:00:00Z",
			Evidence:      []reviewmanifest.EvidenceRef{{Path: "src/file00.txt", Line: 1}},
		})
	}
	writeResult(t, dir, "cross-shard."+plan.PlanHash+".result.json", reviewmanifest.ReviewResult{
		SchemaVersion: reviewmanifest.ResultSchemaVersion,
		ID:            "r-cross",
		Kind:          reviewmanifest.KindCrossShard,
		PlanHash:      plan.PlanHash,
		Verdict:       reviewmanifest.VerdictPass,
		Reviewer:      "cross-shard-reviewer",
		ReviewedAt:    "2026-08-27T10:00:00Z",
		Evidence:      []reviewmanifest.EvidenceRef{{Note: "integration seams reviewed"}},
	})
	// A result for a plan that no longer exists is listed, never a blocker.
	writeResult(t, dir, "shard-0.00112233445566ff.result.json", reviewmanifest.ReviewResult{
		SchemaVersion: reviewmanifest.ResultSchemaVersion,
		ID:            "r-old",
		Kind:          reviewmanifest.KindShard,
		ShardID:       "shard-0",
		ShardHash:     "00112233445566ff",
		PlanHash:      "8899aabbccddeeff",
		Verdict:       reviewmanifest.VerdictPass,
		Reviewer:      "shard-reviewer",
		ReviewedAt:    "2026-08-27T10:00:00Z",
		Evidence:      []reviewmanifest.EvidenceRef{{Note: "reviewed an earlier plan"}},
	})

	result, err := Create(root, Options{Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.ReviewRel)))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "## Sharded Review") {
		t.Fatalf("review log has no sharded section:\n%s", text)
	}
	verdictAt := strings.Index(text, "## Verdict")
	if shardedAt := strings.Index(text, "## Sharded Review"); shardedAt < verdictAt {
		t.Fatal("the sharded section must follow the verdict")
	}
	if !strings.Contains(text, "Ignored result files") {
		t.Fatalf("ignored results are not listed:\n%s", text)
	}
	if strings.Contains(text, "mrvf-not-a-real-finding-id") {
		t.Fatalf("an ingested mrvf- token reached the log verbatim:\n%s", text)
	}

	logs, err := reviewlog.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	var summary reviewlog.Summary
	for _, item := range logs {
		if item.RunID == result.RunID {
			summary = item
		}
	}
	if summary.Verdict != result.Verdict {
		t.Fatalf("parsed verdict = %q, want %q", summary.Verdict, result.Verdict)
	}
	for _, id := range summary.FindingIDs {
		if strings.Contains(id, "not-a-real-finding-id") {
			t.Fatalf("ingested text contaminated the finding IDs: %v", summary.FindingIDs)
		}
	}
}

func TestGCOnlyRunsAfterAPassingGate(t *testing.T) {
	root := shardedRepo(t)

	blocked := &fakeWriter{}
	result, err := Create(root, Options{Base: "main", ShardWriter: blocked})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Blocking {
		t.Fatalf("the fixture is expected to block, got %q", result.Verdict)
	}
	if blocked.collections != 0 {
		t.Fatalf("collections = %d, want none while the gate blocks", blocked.collections)
	}
}
