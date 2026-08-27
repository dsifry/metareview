# review-pr-ready

Run the local PR-ready review gate:

```bash
metareview review pr-ready [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>] [--github-pr <number>] [--include-working-tree] [--shard-result <path>]... [--cross-shard-result <path>]
```

Exit handling: `0` means verify `PASS`/`PASS_ADVISORY` with zero blockers; `1` with a review path means follow that log; nonzero without a path means read stderr. `NEEDS_REVISION` means fix blockers and rerun with `--previous-run`; `ESCALATED` means stop same-target retries and ask the human to narrow, split, or redesign. Use the generated `metareview PR Evidence` section after a passing verdict.

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
   `--shard-result` and `--cross-shard-result` pass a file in from elsewhere.
4. Re-run with `--previous-run <run-id>`. With every shard covered and the aggregate passing, the
   context-risk blocker becomes advisory and the lints run over the whole branch diff.
5. Commit the results in `docs/metareview/shards/` with the review log. After a fix round only the
   edited file's shard and the cross-shard result go ignored: re-review those two and leave the rest.

Local content is in no pack. Commit or remove staged, worktree and untracked files first — an
untracked file over 4,000 bytes raises `UNTRACKED_TRUNCATED`, which shard results can never satisfy.
