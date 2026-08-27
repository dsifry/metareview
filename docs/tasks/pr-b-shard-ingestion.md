# pr-b-shard-ingestion: shard-result ingestion and the gate change (0.8.3)

PR-B of metareview 0.8.3, implementing §5, §6, §7, §8 and §12 of
`docs/specs/2026-08-27-metareview-0.8.3-sharded-review-results.md` (r7). PR-A (measurement, plan,
packs) is already on the branch.

## What was built

**Slice 1 — result format and validation (`internal/reviewmanifest`, §5.1).** `ResultSchemaVersion`
is 1, distinct from `Manifest.SchemaVersion`. `ReviewResult` carries explicit JSON tags and no
`SourceManifestHash`. Freshness is by content hash — `shardHash` for a shard result, `planHash` for
cross-shard — and anything unmatched is ignored with a reason, never a blocker: there is no stale
category. `coveredChunks`/`coveredShardIds` and the per-result coverage blockers they fed are
deleted, along with `Shard.Paths` (now `contextprofile.ShardPaths`). The manifest hash is the plan
hash. `Aggregate` reports `ShardCount`/`ShardsCovered`/`CrossShard`/`PlanHash`, blocks a
filename-vs-content id mismatch and a duplicate result for one current shard, and blocks a
medium-or-higher finding unless it is `fixed` or `false-positive`. Result files over 256 KiB are
ignored. Ingested strings render through `markdown` sanitising.

**Slice 2 — discovery and retention (`internal/shardpack`, §5.2).** `Deps` gains `ReadFile`; the
`Writer` gains `Discover` and `GC`. `Discover` reads
`docs/metareview/shards/<scope>/<target-slug>/`, matches `shard-<id>.<shardHash>.result.json` and
`cross-shard.<planHash>.result.json`, and returns fresh, ignored (with reason) and unreadable
results; explicit `--shard-result` paths are added and must resolve inside the repository. `GC`
removes files matching those two patterns that name no current shard or plan.

**Slice 3 — gate semantics (`internal/reviewers`, `internal/prready`, `internal/taskdone`, §6).**
`ManifestContext` and the satisfaction rule. On the satisfied path the blocking "Review context
risk" becomes the advisory `architecture:context-risk-covered` (plan hash in `Found`, `pr:`-prefixed
on pr-ready), "Diff context was truncated" becomes advisory, and the deterministic lints run over
the full measured branch diff via the new `GitContext.BranchDiffFull`. The context-risk fingerprint
is reason-independent in all three scopes. The review log gains a `## Sharded Review` section placed
after the verdict value line. `--shard-result` and `--cross-shard-result` are validated in
`main.go` before the review package runs, exiting 2 with nothing written. Superseded result files
are collected only after a passing gate.

**Slice 4 — migration, docs, release (§7, §8, §11).** The `superseded` status alias for the three
legacy context-risk fingerprint prefixes, taken after one backup of `findings.jsonl`, working
without `--previous-run` and on escalated chains, with `fixedInRunID` left empty. 1 MiB scanner
buffers in the four JSONL readers. `learnsource.Collect` excludes `docs/metareview/shards/**`. Docs
and skills carry the sharded flow and the durable/transient lists. Version 0.8.3 in the five version
files, plus the CHANGELOG entry. `tests/go/test-sharded-review.sh` drives the whole loop and is
registered in `tests/run-all.sh`.

## Acceptance

- `go test ./...`, `go vet ./...`, `bash tests/run-all.sh` and `bash tests/go/test-shardpack-coverage.sh`
  (100%, zero uncovered blocks) all pass.
- The end-to-end shell test reaches `PASS_ADVISORY` on a ~300 KB branch, closes the context-risk row,
  re-cuts exactly one shard on a same-size edit (fresh N-1, ignored 2), collects the superseded
  files, ignores a result for another plan, and rejects a bad `--shard-result` with exit 2 and a
  byte-identical `runs.jsonl`.

This gate run covers the slice-4 range (`5ddbb72..HEAD`); the earlier ranges are gated separately in
`docs/tasks/pr-b-slices-1-2.md` and `docs/tasks/pr-b-slice-3.md`, so no review is truncated.
