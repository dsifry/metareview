# pr-b-slice-3: sharded gate semantics (0.8.3)

Gate target for the second range of PR-B (`94c51e6..5ddbb72`), split from
`docs/tasks/pr-b-shard-ingestion.md` so no review range exceeds the 120 KB context cap.

## Scope

**`internal/reviewers` (§6).** `ManifestContext{Present, Verdict, Blockers, ShardCount,
ShardsCovered, CrossShard, PlanHash}` and the satisfaction rule: risk reasons limited to
`DIFF_TRUNCATED`/`LARGE_DIFF`, results present, every current shard covered, a cross-shard result
for a multi-shard plan, and a passing manifest verdict. On the satisfied path the blocking "Review
context risk" becomes the advisory `architecture:context-risk-covered` (stable fingerprint,
`pr:`-prefixed on pr-ready, plan hash in `Found`), "Diff context was truncated" becomes advisory,
and the deterministic lints run over the full measured branch diff via the new
`GitContext.BranchDiffFull`. Unsatisfied, the blocker stands with the manifest verdict and its first
ten blockers in `Found`. The context-risk fingerprint is reason-independent in all three scopes.

**`internal/prready`, `internal/taskdone`, `cmd/metareview`.** Results are discovered before the
reviewers run and aggregated into the manifest the gate and the context pack share. The review log
gains a `## Sharded Review` section placed after the verdict value line, so `reviewlog.parseMarkdown`
still reads the verdict token. `--shard-result` (repeatable) and `--cross-shard-result` are
validated in `main.go` before the review package runs, exiting 2 with nothing written. Superseded
result files are collected only after a passing gate.

## Acceptance

- `go test ./...`, `go vet ./...` and `bash tests/run-all.sh` pass.
- `bash tests/go/test-shardpack-coverage.sh` reports 100% with zero uncovered blocks.
