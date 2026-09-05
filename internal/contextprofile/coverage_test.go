package contextprofile

import (
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/gitcontext"
)

func TestMarkdownRendersAllSections(t *testing.T) {
	p := Profile{
		RawDiffBytes:            10,
		FilteredDiffBytes:       8,
		RiskLevel:               RiskContextRisk,
		RiskReasons:             []string{ReasonLargeDiff, ReasonUntrackedOmitted},
		GeneratedExcludedFiles:  []string{"docs/metareview/x.md"},
		UntrackedOmittedCount:   2,
		UntrackedTruncatedCount: 1,
	}
	out := Markdown(p)
	for _, want := range []string{
		"## Context Profile", "Raw diff bytes: `10`", "Filtered diff bytes: `8`",
		"Risk level: `context-risk`", "Risk reasons:", "Generated files excluded: docs/metareview/x.md",
		"Untracked files omitted: `2`", "Untracked files truncated: `1`",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Markdown missing %q:\n%s", want, out)
		}
	}
	// An empty risk level falls back to the "none" default.
	if !strings.Contains(Markdown(Profile{}), "Risk level: `none`") {
		t.Errorf("empty profile should render risk level none:\n%s", Markdown(Profile{}))
	}
}

func TestShardPlanMarkdown(t *testing.T) {
	if got := ShardPlanMarkdown(ShardPlan{}, ""); !strings.Contains(got, "Not sharded.") {
		t.Errorf("empty plan should be 'Not sharded.', got %q", got)
	}
	plan := ShardPlan{
		SourceDiffHash: "srchash",
		PlanHash:       "planhash",
		Shards:         []Shard{{ID: "1", Hash: "sh1", Bytes: 42, Chunks: []Chunk{{Path: "src/a.go", Part: 1}}}},
	}
	out := ShardPlanMarkdown(plan, "/repo/.metareview/shards/x/")
	for _, want := range []string{
		"## Context Shard Plan", "Source diff hash: `srchash`", "Plan hash: `planhash`",
		"shard-1 (`sh1`, 42 bytes): src/a.go", "pack `",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("ShardPlanMarkdown missing %q:\n%s", want, out)
		}
	}
	// Without a packDir the pack suffix is omitted.
	if strings.Contains(ShardPlanMarkdown(plan, ""), "pack `") {
		t.Error("no packDir should omit the pack suffix")
	}
}

func TestShardPaths(t *testing.T) {
	shard := Shard{Chunks: []Chunk{
		{Path: "b.go", Part: 1}, {Path: "a.go", Part: 1}, {Path: "a.go", Part: 2},
	}}
	got := ShardPaths(shard)
	if len(got) != 2 || got[0] != "a.go" || got[1] != "b.go" {
		t.Fatalf("expected deduped sorted [a.go b.go], got %v", got)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "  ", "x"); got != "x" {
		t.Errorf("expected 'x', got %q", got)
	}
	if got := firstNonEmpty("", "   "); got != "" {
		t.Errorf("all-blank should be empty, got %q", got)
	}
}

func TestDiffHeaderPath(t *testing.T) {
	if got := diffHeaderPath("diff --git a/x"); got != "" {
		t.Errorf("fewer than 4 fields should yield empty, got %q", got)
	}
	if got := diffHeaderPath("diff --git a/src/x.go b/src/x.go"); got != "src/x.go" {
		t.Errorf("expected src/x.go, got %q", got)
	}
}

func TestAddUntrackedProfiles(t *testing.T) {
	byPath := map[string]int{}
	addUntrackedProfiles(byPath, "--- notes.txt\nline one\nline two\n--- other.txt\nx\n")
	if byPath["notes.txt"] == 0 {
		t.Errorf("notes.txt should accumulate bytes: %v", byPath)
	}
	if byPath["other.txt"] == 0 {
		t.Errorf("other.txt should accumulate bytes: %v", byPath)
	}
}

func TestSourceOf(t *testing.T) {
	git := gitcontext.Context{
		ChangedFiles:   []string{"branch.go"},
		StagedFiles:    []string{"staged.go"},
		UntrackedFiles: []string{"untracked.go"},
	}
	cases := map[string]string{
		"branch.go":    SourceBranch,
		"staged.go":    SourceStaged,
		"untracked.go": SourceUntracked,
		"other.go":     SourceWorktree,
	}
	for path, want := range cases {
		if got := sourceOf(git, path); got != want {
			t.Errorf("sourceOf(%q) = %q, want %q", path, got, want)
		}
	}
}

// FromGit uses the measured raw branch bytes plus local contributions when BranchRawDiffBytes is set.
func TestFromGitBranchRawDiffBytes(t *testing.T) {
	git := gitcontext.Context{
		BranchFiles:        []gitcontext.BranchFile{{Path: "src/a.go", Bytes: 400, Hash: "h"}},
		BranchRawDiffBytes: 1000,
		StagedDiff:         "diff --git a/s.go b/s.go\n+staged\n",
	}
	p := FromGit(git, Options{})
	local := len(git.StagedDiff)
	if p.RawDiffBytes != 1000+local {
		t.Fatalf("RawDiffBytes = %d, want %d", p.RawDiffBytes, 1000+local)
	}
	if p.FilteredDiffBytes != 400+local {
		t.Fatalf("FilteredDiffBytes = %d, want %d", p.FilteredDiffBytes, 400+local)
	}
}

func TestNeedBuckets(t *testing.T) {
	if got := needBuckets(100, 0); got != 1 { // budget<=0 -> default (60000), 100/60000 -> 1
		t.Errorf("needBuckets(100,0) = %d, want 1", got)
	}
	if got := needBuckets(0, 100); got != 1 { // total<=0 -> 1
		t.Errorf("needBuckets(0,100) = %d, want 1", got)
	}
	if got := needBuckets(250, 100); got != 3 { // ceil(250/100)
		t.Errorf("needBuckets(250,100) = %d, want 3", got)
	}
}

func TestShardIDForPathZeroBits(t *testing.T) {
	if got := shardIDForPath("x", 0); got != "0" {
		t.Errorf("shardIDForPath with 0 bits should be \"0\", got %q", got)
	}
}

// shardHash is stable regardless of input chunk order (it sorts by path then part).
func TestShardHashOrderIndependent(t *testing.T) {
	a := Chunk{Path: "a.go", Part: 1, Hash: "h1"}
	b := Chunk{Path: "a.go", Part: 2, Hash: "h2"}
	c := Chunk{Path: "b.go", Part: 1, Hash: "h3"}
	h1 := shardHash("task-done", "t", []Chunk{a, b, c})
	h2 := shardHash("task-done", "t", []Chunk{c, b, a})
	if h1 == "" || h1 != h2 {
		t.Fatalf("shardHash must be order-independent: %q vs %q", h1, h2)
	}
}

// A single over-budget branch file splits into multiple chunks that overflow one bucket, forcing the
// in-bucket flush and a multi-part shard set.
func TestPlanShardsBudgetDefaultAndFlush(t *testing.T) {
	diff := strings.Repeat("x", 250)
	files := []gitcontext.BranchFile{{Path: "big.go", Bytes: len(diff), Diff: diff}}
	// MaxBytesPerShard omitted -> the default budget path is taken; but the default is large, so the
	// file is one shard. A tiny explicit budget forces the chunk split + flush.
	if _, err := PlanShards(branchProfileOf(files), files, ShardOptions{Scope: "task-done", TargetID: "t"}); err != nil {
		t.Fatalf("default-budget PlanShards: %v", err)
	}
	plan, err := PlanShards(branchProfileOf(files), files, ShardOptions{MaxBytesPerShard: 100, Scope: "task-done", TargetID: "t"})
	if err != nil {
		t.Fatalf("small-budget PlanShards: %v", err)
	}
	if len(plan.Shards) < 2 {
		t.Fatalf("a 250-byte file at budget 100 should span multiple shards, got %d", len(plan.Shards))
	}
}

func branchProfileOf(files []gitcontext.BranchFile) Profile {
	return FromGit(gitcontext.Context{BranchFiles: files}, Options{})
}
