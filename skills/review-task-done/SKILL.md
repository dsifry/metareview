---
name: review-task-done
description: Run metareview's deterministic task-done code review gate before claiming a local task is complete; use for task closure, chunk review, or pre-PR local review when code changed.
---

# Review Task Done

Run this before saying a coding task is done.

## Command

```bash
metareview review task-done <task-id-or-path> [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>] [--shard-result <path>]... [--cross-shard-result <path>]
```

Use `--base` for the reviewed diff, `--previous-run` after fixes, and `--evidence` for validation output. Use `--max-attempts` only on the first run; it sets the chain budget (default 3), with the first blocker run as attempt 1.

Prefer structured evidence receipts:

```bash
go run ./cmd/metareview evidence run -- go test ./...
```

Freeform evidence remains accepted as a fallback, but receipts preserve command, exit code, timestamps, and output hashes.

## Workflow

1. Run the command from the repository root.
2. Exit handling: `0` means verify `PASS`/`PASS_ADVISORY` with zero blockers; `1` with a review path means follow that log; nonzero without a path means read stderr.
3. `NEEDS_REVISION`: fix blockers and re-run with `--previous-run <run-id>`.
4. `ESCALATED`: stop same-target retries; human must narrow, split, or redesign the target.

The review updates `.metareview/findings.jsonl`, `.metareview/runs.jsonl`, `docs/metareview/FINDINGS.md`, and Markdown review/context artifacts.

## Sharded review

When the branch diff exceeds the review context limit, the gate returns `NEEDS_REVISION` with the
context-risk blocker and writes one prompt pack per shard under
`.metareview/shards/<scope>/<target-slug>/<planHash>/`, plus a `plan.json` naming every shard, its
hash, and `resultsDir`.

1. Set `--max-attempts` on the **first** run: a sharded gate costs a plan run, a results run, and one
   run per fix round. Mid-chain the flag is ignored.
2. Read `plan.json`. Dispatch one subagent per `shard-<id>.md` against
   `rubrics/task-done-review-rubric.md`, and one over `cross-shard.md` when there is more than one
   shard.
3. Write one result per shard into `resultsDir` as `shard-<id>.<shardHash>.result.json`, and
   `cross-shard.<planHash>.result.json` for a multi-shard plan. Each pack states the exact contract.
   `--shard-result` (repeatable) and `--cross-shard-result` (once: a plan has one cross-shard
   result) pass a file in from elsewhere, and take precedence over a file of the same identity
   in `resultsDir`.
4. Re-run with `--previous-run <run-id>`. With every shard covered and the aggregate passing, the
   context-risk blocker becomes advisory and the lints run over the whole branch diff.
5. Commit the results in `docs/metareview/shards/` with the review log. After a fix round only the
   edited file's shard and the cross-shard result go ignored: re-review those two and leave the rest.

Local content is in no pack. Commit or remove staged, worktree and untracked files first — an
untracked file over 4,000 bytes raises `UNTRACKED_TRUNCATED`, which shard results can never satisfy.
