# pr-b-slices-1-2: result format, validation and discovery (0.8.3)

Gate target for the first range of PR-B (`c9e6503..94c51e6`), split from
`docs/tasks/pr-b-shard-ingestion.md` so no review range exceeds the 120 KB context cap.

## Scope

**`internal/reviewmanifest` (§5.1).** `ResultSchemaVersion = 1`, distinct from
`Manifest.SchemaVersion`. Explicit JSON tags on `ReviewResult`, with `SourceManifestHash` removed
from it. Freshness by content hash — `shardHash` for a shard result, `planHash` for cross-shard —
and anything unmatched ignored with a reason rather than blocking: there is no stale category.
Identity rules: the filename id must equal the id inside, two fresh results for one shard block, and
`ShardsCovered` counts distinct current shards. Dispositions `fixed`/`false-positive` close a
finding; `waived`/`accepted-risk`/`deferred`/`open` block at medium or above. A result file over
256 KiB is ignored. `coveredChunks`/`coveredShardIds` and the coverage blockers they fed are deleted
— the hashes already commit to those sets — and `Shard.Paths` becomes `contextprofile.ShardPaths`.
The manifest hash is the plan hash.

**`internal/shardpack` (§5.2).** `Deps.ReadFile`, plus `Discover` and `GC` on the `Writer`.
`Discover` returns fresh, ignored (with reason) and unreadable results from
`docs/metareview/shards/<scope>/<target-slug>/`; explicit paths must resolve inside the repository.
`GC` removes result files matching the two filename patterns that name no current shard or plan.

## Acceptance

- `go test ./...`, `go vet ./...` and `bash tests/run-all.sh` pass.
- `bash tests/go/test-shardpack-coverage.sh` reports 100% with zero uncovered blocks.
