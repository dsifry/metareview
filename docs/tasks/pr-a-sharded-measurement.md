# PR-A — sharded review: measurement, plan and packs

Implements r7 §3, §4 and the coverage check in §7 of
`docs/specs/2026-08-27-metareview-0.8.3-sharded-review-results.md`. No ingestion and no gate change:
that is PR-B.

- `internal/gitcontext`: `CollectWith(root, Options{Base, Excludes, Exceptions, RunGit})`; per-file
  untruncated, exclude-filtered branch measurement (`BranchFiles`, `BranchDiffFull`, branch byte counts,
  all `json:"-"`); `AddedLines` keeps today's union but uses the untruncated branch text.
- `internal/contextprofile`: v4 content hashes (`fileHash`, `sourceDiffHash`, `chunkHash`, `shardHash`,
  `planHash`) with length-prefixed field encoding; `Chunk`; content-stable two-step assignment (hash
  buckets sized by `bits = ceil(log2(need))`, then a first-fit split inside an over-budget bucket);
  `ShardOptions.GroupBy` removed; risk reasons split (`DIFF_TRUNCATED` branch-only,
  `LOCAL_DIFF_TRUNCATED`, `DIFF_OVERSIZE`); the profile reports measured bytes rather than the truncated
  cap.
- `internal/shardpack` (new, exactly 100% statement coverage): transient pack sets under
  `.metareview/shards/<scope>/<slug>/<planHash>/`, atomic rename-aside/rename-in/remove-aside with
  rollback, `Prune`, `plan.json`, fenced chunk bodies built from the measured bytes.
- `internal/prready`, `internal/taskdone`: the plan is computed once and threaded to the context pack and
  the manifest; packs are written through `Options.ShardWriter` inside the run and pruned after success.

Done when `go test ./...` and `tests/run-all.sh` pass, `tests/go/test-shardpack-coverage.sh` reports 100%
with zero uncovered blocks, and `go vet` is clean.
