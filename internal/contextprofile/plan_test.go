package contextprofile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/gitcontext"
)

func bf(path, diff string) gitcontext.BranchFile {
	return gitcontext.BranchFile{Path: path, Bytes: len(diff), Hash: fileHashOf(path, diff), Diff: diff}
}

func fileHashOf(path, diff string) string {
	sum := sha256.Sum256([]byte(fields("mrv-file-v4", path, diff)))
	return hex.EncodeToString(sum[:])[:16]
}

// fields mirrors the documented len:bytes\0 preimage encoding (spec §3). The
// tests derive expectations from the spec, never from the implementation.
func fields(values ...string) string {
	var b strings.Builder
	for _, v := range values {
		fmt.Fprintf(&b, "%d:%s\x00", len(v), v)
	}
	return b.String()
}

func branchProfile(files ...gitcontext.BranchFile) Profile {
	p := Profile{}
	for _, f := range files {
		p.Files = append(p.Files, FileProfile{Path: f.Path, DiffBytes: f.Bytes, Hash: f.Hash, Source: SourceBranch})
	}
	return p
}

func planOf(t *testing.T, budget int, files ...gitcontext.BranchFile) ShardPlan {
	t.Helper()
	plan, err := PlanShards(branchProfile(files...), files, ShardOptions{
		MaxBytesPerShard: budget, Scope: "pr-ready", TargetID: "branch",
	})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestSourceDiffHashFromDocumentedPreimage(t *testing.T) {
	a, b := bf("a.go", "diff-a"), bf("b.go", "diff-b")
	preimage := fields("mrv-source-v4", "a.go", a.Hash, "b.go", b.Hash)
	sum := sha256.Sum256([]byte(preimage))
	want := hex.EncodeToString(sum[:])[:16]

	if got := SourceDiffHash([]gitcontext.BranchFile{a, b}); got != want {
		t.Fatalf("SourceDiffHash = %s, want %s (documented preimage)", got, want)
	}
}

func TestSourceDiffHashOrderIndependent(t *testing.T) {
	a, b, c := bf("a.go", "aaa"), bf("b.go", "bbb"), bf("c.go", "ccc")
	one := SourceDiffHash([]gitcontext.BranchFile{a, b, c})
	two := SourceDiffHash([]gitcontext.BranchFile{c, a, b})
	if one != two {
		t.Fatalf("hash depends on input order: %s vs %s", one, two)
	}
}

func TestSourceDiffHashChangesOnSameSizeEdit(t *testing.T) {
	before := SourceDiffHash([]gitcontext.BranchFile{bf("a.go", "+if x == 1 {")})
	after := SourceDiffHash([]gitcontext.BranchFile{bf("a.go", "+if x != 1 {")})
	if before == after {
		t.Fatal("a same-length content edit must change the source diff hash")
	}
}

func TestSourceDiffHashIgnoresLocalChanges(t *testing.T) {
	files := []gitcontext.BranchFile{bf("a.go", "aaa")}
	clean := branchProfile(files...)
	dirty := clean
	dirty.Files = append(append([]FileProfile{}, clean.Files...),
		FileProfile{Path: "local.txt", DiffBytes: 999, Source: SourceWorktree},
		FileProfile{Path: "new.txt", DiffBytes: 42, Source: SourceUntracked},
	)
	if PlanSourceHash(clean, files) != PlanSourceHash(dirty, files) {
		t.Fatal("local (staged/worktree/untracked) files must not affect the source diff hash")
	}
}

func TestNewlinePathCannotCollide(t *testing.T) {
	// Two files whose naive concatenation would be identical.
	one := SourceDiffHash([]gitcontext.BranchFile{bf("a\nb.go", "x")})
	two := SourceDiffHash([]gitcontext.BranchFile{bf("a", "\nb.go"), bf("x", "")})
	if one == two {
		t.Fatal("length-prefixed fields must make these preimages distinct")
	}
}

func TestBucketBitsFromTotalBytes(t *testing.T) {
	// Pinned table from spec §4.2 — never recomputed in the test.
	for _, tc := range []struct{ need, bits int }{
		{1, 0}, {2, 1}, {3, 2}, {4, 2}, {5, 3}, {23, 5}, {4096, 12}, {100000, 12},
	} {
		if got := bucketBits(tc.need); got != tc.bits {
			t.Fatalf("bucketBits(%d) = %d, want %d", tc.need, got, tc.bits)
		}
	}
}

func TestPlanEmptyWhenNotTruncated(t *testing.T) {
	plan, err := PlanShards(Profile{}, nil, ShardOptions{MaxBytesPerShard: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Shards) != 0 || plan.PlanHash != "" {
		t.Fatalf("nil branch files must yield an empty plan: %+v", plan)
	}
}

func TestChunkNeverExceedsBudget(t *testing.T) {
	budget := 500
	long := "+" + strings.Repeat("x", 2_000) + "\n" // one line far over budget
	files := []gitcontext.BranchFile{
		bf("big.txt", strings.Repeat("+line\n", 300)),
		bf("longline.txt", long),
	}
	plan := planOf(t, budget, files...)
	seen := 0
	for _, s := range plan.Shards {
		for _, c := range s.Chunks {
			seen++
			if n := c.ByteEnd - c.ByteStart; n > budget {
				t.Fatalf("chunk %s part %d/%d is %d bytes, over budget %d", c.Path, c.Part, c.Parts, n, budget)
			}
		}
	}
	if seen < 5 {
		t.Fatalf("fixture produced only %d chunks; it must exercise splitting", seen)
	}
}

func TestOverLongLineHardCut(t *testing.T) {
	budget := 100
	files := []gitcontext.BranchFile{bf("one.txt", "+"+strings.Repeat("y", 350)+"\n")}
	plan := planOf(t, budget, files...)
	var parts int
	for _, s := range plan.Shards {
		for _, c := range s.Chunks {
			parts++
			if n := c.ByteEnd - c.ByteStart; n > budget {
				t.Fatalf("hard cut left a %d-byte chunk over budget %d", n, budget)
			}
		}
	}
	if parts < 4 {
		t.Fatalf("a 351-byte single line at budget %d must hard-cut into >= 4 chunks, got %d", budget, parts)
	}
}

// TestShardNeverExceedsBudget uses a fixture that *must* drive at least one
// bucket over budget, so it fails if the over-budget split (§4.2 step 2) is not
// implemented.
func TestShardNeverExceedsBudget(t *testing.T) {
	budget := 4_000
	var files []gitcontext.BranchFile
	for i := 0; i < 40; i++ {
		files = append(files, bf(fmt.Sprintf("pkg/file%02d.go", i), strings.Repeat("+x\n", 600)))
	}
	plan := planOf(t, budget, files...)

	sawSplit := false
	for _, s := range plan.Shards {
		if s.Bytes > budget {
			t.Fatalf("shard %s is %d bytes, over budget %d", s.ID, s.Bytes, budget)
		}
		if strings.Contains(s.ID, "-") {
			sawSplit = true
		}
	}
	if !sawSplit {
		t.Fatal("fixture did not exercise the over-budget split: no sub-shard id present")
	}
}

func TestEditChangesOnlyItsShard(t *testing.T) {
	budget := 4_000
	var files []gitcontext.BranchFile
	for i := 0; i < 24; i++ {
		files = append(files, bf(fmt.Sprintf("pkg/f%02d.go", i), strings.Repeat("+x\n", 500)))
	}
	before := planOf(t, budget, files...)
	if len(before.Shards) < 4 {
		t.Fatalf("fixture must span several buckets, got %d shards", len(before.Shards))
	}

	// Same-size edit in exactly one file.
	edited := append([]gitcontext.BranchFile{}, files...)
	edited[7] = bf(edited[7].Path, strings.Repeat("+y\n", 500))
	after := planOf(t, budget, edited...)

	beforeByID := map[string]string{}
	for _, s := range before.Shards {
		beforeByID[s.ID] = s.Hash
	}
	changed := 0
	for _, s := range after.Shards {
		if h, ok := beforeByID[s.ID]; !ok || h != s.Hash {
			changed++
		}
	}
	if changed != 1 {
		t.Fatalf("a one-file edit changed %d shards, want exactly 1", changed)
	}
	if before.PlanHash == after.PlanHash {
		t.Fatal("the plan hash must change when a shard changes")
	}
}

func TestOverBudgetBucketSplitsLocally(t *testing.T) {
	budget := 3_000
	var files []gitcontext.BranchFile
	for i := 0; i < 30; i++ {
		files = append(files, bf(fmt.Sprintf("pkg/g%02d.go", i), strings.Repeat("+z\n", 400)))
	}
	before := planOf(t, budget, files...)
	// Grow one file so its bucket must re-split.
	grown := append([]gitcontext.BranchFile{}, files...)
	grown[3] = bf(grown[3].Path, strings.Repeat("+z\n", 1_400))
	after := planOf(t, budget, grown...)

	bucketOf := func(id string) string { return strings.SplitN(id, "-", 2)[0] }
	target := ""
	for _, f := range []gitcontext.BranchFile{grown[3]} {
		target = bucketOf(shardIDForPath(f.Path, bucketBits(needBuckets(totalBytes(grown), budget))))
	}
	beforeByID := map[string]string{}
	for _, s := range before.Shards {
		beforeByID[s.ID] = s.Hash
	}
	for _, s := range after.Shards {
		if bucketOf(s.ID) == target {
			continue // the touched bucket may split; that is the point
		}
		if h, ok := beforeByID[s.ID]; !ok || h != s.Hash {
			t.Fatalf("shard %s outside the edited bucket changed", s.ID)
		}
	}
}

func TestBitsBoundaryRecutsAll(t *testing.T) {
	budget := 1_000
	small := []gitcontext.BranchFile{bf("a.go", strings.Repeat("+a\n", 200)), bf("b.go", strings.Repeat("+b\n", 200))}
	big := append([]gitcontext.BranchFile{}, small...)
	for i := 0; i < 20; i++ {
		big = append(big, bf(fmt.Sprintf("c%02d.go", i), strings.Repeat("+c\n", 400)))
	}
	if bucketBits(needBuckets(totalBytes(small), budget)) == bucketBits(needBuckets(totalBytes(big), budget)) {
		t.Skip("fixture did not cross a bits boundary")
	}
	// Documented consequence: ids are computed at a different depth, so shard
	// identity does not survive the boundary.
	beforeIDs := map[string]bool{}
	for _, s := range planOf(t, budget, small...).Shards {
		beforeIDs[s.ID] = true
	}
	after := planOf(t, budget, big...).Shards
	survived := 0
	for _, s := range after {
		if beforeIDs[s.ID] {
			survived++
		}
	}
	if len(after) == 0 {
		t.Fatal("the larger fixture produced no shards")
	}
	if survived == len(after) {
		t.Fatal("crossing the bits boundary must re-cut the plan, but every shard id survived")
	}
}

func TestLocalTruncationIsSeparateReason(t *testing.T) {
	branchOnly := FromGit(gitcontext.Context{DiffTruncated: true}, Options{})
	if !hasReason(branchOnly, ReasonDiffTruncated) || hasReason(branchOnly, ReasonLocalDiffTruncated) {
		t.Fatalf("branch truncation reasons = %v", branchOnly.RiskReasons)
	}
	local := FromGit(gitcontext.Context{StagedDiffTruncated: true}, Options{})
	if hasReason(local, ReasonDiffTruncated) || !hasReason(local, ReasonLocalDiffTruncated) {
		t.Fatalf("local truncation reasons = %v", local.RiskReasons)
	}
	worktree := FromGit(gitcontext.Context{WorkingTreeDiffTruncated: true}, Options{})
	if !hasReason(worktree, ReasonLocalDiffTruncated) {
		t.Fatalf("worktree truncation reasons = %v", worktree.RiskReasons)
	}
}

func TestLargeDiffKeepsTotal(t *testing.T) {
	// LARGE_DIFF still measures branch + local bytes, as it does today.
	ctx := gitcontext.Context{Diff: strings.Repeat("x", 60_000), StagedDiff: strings.Repeat("y", 70_000)}
	ctx.FilteredDiffBytes = len(ctx.Diff) + len(ctx.StagedDiff)
	if p := FromGit(ctx, Options{}); !hasReason(p, ReasonLargeDiff) {
		t.Fatalf("LARGE_DIFF must fire on the combined total: %v", p.RiskReasons)
	}
}

func TestDiffOversizeReason(t *testing.T) {
	old := maxBranchDiffBytes
	maxBranchDiffBytes = 1_000
	defer func() { maxBranchDiffBytes = old }()

	ctx := gitcontext.Context{
		DiffTruncated: true,
		BranchFiles:   []gitcontext.BranchFile{bf("big.go", strings.Repeat("+x\n", 2_000))},
	}
	p := FromGit(ctx, Options{})
	if !hasReason(p, ReasonDiffOversize) {
		t.Fatalf("reasons = %v, want %s", p.RiskReasons, ReasonDiffOversize)
	}
	plan, err := PlanShards(p, ctx.BranchFiles, ShardOptions{MaxBytesPerShard: 500})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Shards) != 0 {
		t.Fatal("no packs are planned when the branch diff is oversize")
	}
}

func hasReason(p Profile, reason string) bool {
	for _, r := range p.RiskReasons {
		if r == reason {
			return true
		}
	}
	return false
}
