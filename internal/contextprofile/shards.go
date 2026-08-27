package contextprofile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/dsifry/metareview/internal/gitcontext"
)

// DefaultMaxBytesPerShard is the shard budget. It is fixed in 0.8.3; tests lower
// it through ShardOptions.MaxBytesPerShard.
const DefaultMaxBytesPerShard = 60000

// maxBucketBits caps the bucket space at 4096 shards (var so tests can lower it).
const maxBucketBits = 12

type ShardOptions struct {
	MaxBytesPerShard int
	Scope            string
	TargetID         string
}

// Chunk is a byte range of one path's untruncated branch diff.
type Chunk struct {
	Path      string
	Part      int
	Parts     int
	ByteStart int
	ByteEnd   int
	Hash      string
}

type Shard struct {
	ID     string
	Chunks []Chunk
	// Paths is derived from Chunks (deduplicated, sorted); PR-B removes it once
	// reviewmanifest speaks chunks.
	Paths []string
	Bytes int
	Hash  string
}

type ShardPlan struct {
	SourceDiffHash string
	PlanHash       string
	Shards         []Shard
}

// hashFields encodes fields as len:bytes\0 so no value can forge a boundary.
func hashFields(values ...string) string {
	var b strings.Builder
	for _, v := range values {
		fmt.Fprintf(&b, "%d:%s\x00", len(v), v)
	}
	return b.String()
}

func shortHash(preimage string) string {
	sum := sha256.Sum256([]byte(preimage))
	return hex.EncodeToString(sum[:])[:16]
}

// SourceDiffHash is the v4 content key over the exclude-filtered branch diff.
func SourceDiffHash(files []gitcontext.BranchFile) string {
	sorted := append([]gitcontext.BranchFile{}, files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	values := []string{"mrv-source-v4"}
	for _, f := range sorted {
		values = append(values, f.Path, f.Hash)
	}
	return shortHash(hashFields(values...))
}

// PlanSourceHash is SourceDiffHash for a profile's branch files; the profile's
// local (staged/worktree/untracked) entries never contribute.
func PlanSourceHash(_ Profile, files []gitcontext.BranchFile) string {
	return SourceDiffHash(files)
}

func totalBytes(files []gitcontext.BranchFile) int {
	total := 0
	for _, f := range files {
		total += f.Bytes
	}
	return total
}

func needBuckets(total, budget int) int {
	if budget <= 0 {
		budget = DefaultMaxBytesPerShard
	}
	if total <= 0 {
		return 1
	}
	return (total + budget - 1) / budget
}

// bucketBits is the smallest b with 2^b >= need, capped at maxBucketBits.
func bucketBits(need int) int {
	bits := 0
	for (1 << bits) < need {
		bits++
		if bits >= maxBucketBits {
			return maxBucketBits
		}
	}
	return bits
}

// shardIDForPath renders the first `bits` bits of the path's bucket key as
// ceil(bits/4) lowercase hex digits.
func shardIDForPath(path string, bits int) string {
	if bits <= 0 {
		return "0"
	}
	sum := sha256.Sum256([]byte(hashFields("mrv-bucket-v4", path)))
	var value uint64
	for i := 0; i < 8; i++ {
		value = value<<8 | uint64(sum[i])
	}
	index := value >> (64 - uint(bits))
	digits := (bits + 3) / 4
	return fmt.Sprintf("%0*x", digits, index)
}

// chunksOf cuts one file's diff into consecutive pieces of at most budget bytes,
// preferring newline boundaries and hard-cutting an over-long line.
func chunksOf(file gitcontext.BranchFile, budget int) []Chunk {
	text := file.Diff
	if len(text) <= budget {
		return []Chunk{{Path: file.Path, Part: 1, Parts: 1, ByteStart: 0, ByteEnd: len(text),
			Hash: chunkHash(file.Path, 1, 1, text)}}
	}
	var bounds [][2]int
	start := 0
	for start < len(text) {
		end := start + budget
		if end >= len(text) {
			end = len(text)
		} else if nl := strings.LastIndexByte(text[start:end], '\n'); nl >= 0 {
			end = start + nl + 1
		}
		bounds = append(bounds, [2]int{start, end})
		start = end
	}
	chunks := make([]Chunk, 0, len(bounds))
	for i, b := range bounds {
		chunks = append(chunks, Chunk{
			Path: file.Path, Part: i + 1, Parts: len(bounds), ByteStart: b[0], ByteEnd: b[1],
			Hash: chunkHash(file.Path, i+1, len(bounds), text[b[0]:b[1]]),
		})
	}
	return chunks
}

func chunkHash(path string, part, parts int, text string) string {
	return shortHash(hashFields("mrv-chunk-v4", path, fmt.Sprint(part), fmt.Sprint(parts), text))
}

func shardHash(scope, targetID string, chunks []Chunk) string {
	sorted := append([]Chunk{}, chunks...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Path == sorted[j].Path {
			return sorted[i].Part < sorted[j].Part
		}
		return sorted[i].Path < sorted[j].Path
	})
	values := []string{"mrv-shard-v4", scope, targetID}
	for _, c := range sorted {
		values = append(values, c.Path, fmt.Sprint(c.Part), c.Hash)
	}
	return shortHash(hashFields(values...))
}

// PlanShards assigns every branch chunk to exactly one shard: first by a hash
// bucket of its path (stable under edits), then by splitting an over-budget
// bucket over its own (Path, Part)-sorted chunk list.
func PlanShards(profile Profile, branchFiles []gitcontext.BranchFile, options ShardOptions) (ShardPlan, error) {
	if len(branchFiles) == 0 {
		return ShardPlan{}, nil
	}
	total := totalBytes(branchFiles)
	if total > maxBranchDiffBytes {
		return ShardPlan{}, nil
	}
	budget := options.MaxBytesPerShard
	if budget <= 0 {
		budget = DefaultMaxBytesPerShard
	}
	bits := bucketBits(needBuckets(total, budget))

	buckets := map[string][]Chunk{}
	for _, f := range branchFiles {
		id := shardIDForPath(f.Path, bits)
		buckets[id] = append(buckets[id], chunksOf(f, budget)...)
	}
	ids := make([]string, 0, len(buckets))
	for id := range buckets {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var shards []Shard
	for _, id := range ids {
		chunks := buckets[id]
		sort.Slice(chunks, func(i, j int) bool {
			if chunks[i].Path == chunks[j].Path {
				return chunks[i].Part < chunks[j].Part
			}
			return chunks[i].Path < chunks[j].Path
		})
		part, bytes, index := []Chunk{}, 0, 1
		flush := func() {
			if len(part) == 0 {
				return
			}
			shardID := id
			if index > 1 {
				shardID = fmt.Sprintf("%s-%d", id, index)
			}
			shards = append(shards, newShard(shardID, part, bytes, options))
			index++
			part, bytes = []Chunk{}, 0
		}
		for _, c := range chunks {
			size := c.ByteEnd - c.ByteStart
			if bytes+size > budget {
				flush()
			}
			part = append(part, c)
			bytes += size
		}
		flush()
	}
	plan := ShardPlan{SourceDiffHash: SourceDiffHash(branchFiles), Shards: shards}
	values := []string{"mrv-plan-v4", plan.SourceDiffHash}
	for _, s := range shards {
		values = append(values, s.ID, s.Hash)
	}
	plan.PlanHash = shortHash(hashFields(values...))
	return plan, nil
}

func newShard(id string, chunks []Chunk, bytes int, options ShardOptions) Shard {
	paths := map[string]bool{}
	for _, c := range chunks {
		paths[c.Path] = true
	}
	list := make([]string, 0, len(paths))
	for p := range paths {
		list = append(list, p)
	}
	sort.Strings(list)
	return Shard{
		ID:     id,
		Chunks: append([]Chunk{}, chunks...),
		Paths:  list,
		Bytes:  bytes,
		Hash:   shardHash(options.Scope, options.TargetID, chunks),
	}
}

// ShardPlanMarkdown renders the plan; packDir is printed only when non-empty.
func ShardPlanMarkdown(plan ShardPlan, packDir string) string {
	if len(plan.Shards) == 0 {
		return "## Context Shard Plan\n\nNot sharded."
	}
	lines := []string{
		"## Context Shard Plan",
		"",
		"- Source diff hash: `" + plan.SourceDiffHash + "`",
		"- Plan hash: `" + plan.PlanHash + "`",
	}
	for _, shard := range plan.Shards {
		line := "- shard-" + shard.ID + " (`" + shard.Hash + "`, " + fmt.Sprint(shard.Bytes) + " bytes): " +
			strings.Join(shard.Paths, ", ")
		if packDir != "" {
			line += " — pack `" + strings.TrimSuffix(packDir, "/") + "/shard-" + shard.ID + ".md`"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
