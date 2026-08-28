# Claude Code Integration

metareview supports Claude Code as a plugin and as a direct CLI.

## Install

```bash
claude plugin marketplace add dsifry/metareview-marketplace
claude plugin install metareview
```

For local development, install the plugin from the current checkout using the local plugin flow supported by your Claude Code build.

## Slash Commands

| Command | Purpose |
| --- | --- |
| `/setup` | Detect repository mode and prerequisites. |
| `/review-artifact` | Review specs, plans, docs, designs, and decompositions. |
| `/review-task-done` | Run task-done review before claiming local code work complete. |
| `/review-epic-ready` | Check parent readiness after child tasks are complete. |
| `/review-pr-ready` | Check PR readiness before push or merge. |
| `/learn-post-merge` | Extract post-merge learning after a PR merges. |
| `/status` | Show current review state. |

## Direct CLI Fallback

```bash
metareview setup --check
metareview evidence run -- go test ./... > /tmp/metareview-evidence.jsonl
metareview review artifact <path>
metareview review task-done <task-id-or-path> --base <base-ref> --evidence /tmp/metareview-evidence.jsonl
metareview review epic-ready <epic-id-or-path> --base <base-ref> --evidence /tmp/metareview-evidence.jsonl
metareview review pr-ready --base <base-ref> --evidence /tmp/metareview-evidence.jsonl
metareview learn --post-merge <pr-number> --base <pre-merge-ref>
```

In a source checkout without a packaged binary, use:

```bash
go run ./cmd/metareview review task-done <task-id-or-path> --base <base-ref> --evidence <file>
```

## Agent Contract

Claude Code agents must resolve every blocking finding before claiming completion. A `NOT_REVIEWED` artifact scaffold is also blocking; complete the required reviewer rows and final verdict before treating the artifact as reviewed. Artifact review authorizes the five required lenses to run as parallel subagents by default. If subagents are unavailable or the human requests no delegation, record `in-session-emulated` and state that the review is not independently adversarial and is weaker evidence.

Lifecycle gate results are actionable: `PASS`/`PASS_ADVISORY` proceed only with zero blockers; `NEEDS_REVISION` repairs via `--previous-run`; `ESCALATED` stops same-target retries; human must narrow, split, or redesign the target. Exit handling: `0` means verify a passing verdict; `1` with a review path means follow that log; nonzero without a path means read stderr.

Prefer structured evidence receipts from `metareview evidence run -- <command>` and, after a PR exists, `metareview evidence import --github-checks <pr-number>`. Task-done and PR-ready parse receipt files as validation evidence; epic-ready reads the supplied evidence text for child-completion signals.

Commit durable review and context Markdown under `docs/metareview/`, including the shard review results in `docs/metareview/shards/` and the FSM export bundles in `docs/metareview/fsm/`. Leave transient `.metareview/findings.jsonl`, `.metareview/runs.jsonl`, `.metareview/runs/` (FSM runs, self-ignoring) and `.metareview/shards/` local.

## Metaswarm Repositories

metareview augments metaswarm. It does not replace metaswarm's Beads task state, Superpowers workflows, or PR shepherding. Use it as the deeper review harness at artifact, task-done, epic-ready, pr-ready, and post-merge checkpoints.

## Workflow runs

`metareview fsm` drives `sdlc-loop` and `review-loop` as an audited state machine (the `/fsm` skill). Print the driver contract with `metareview fsm --agent-prompt`; see `docs/fsm/driving-a-workflow.md`.

### Sharded review (diffs over the context limit)

An exclude-filtered diff over 120,000 bytes cannot be held in one review context. metareview measures the real
branch diff, cuts it into shards, and writes a prompt pack per shard under
`.metareview/shards/<scope>/<target-slug>/<planHash>/`, with a `plan.json` naming every shard, its
hash, and the directory the results belong in.

1. Run the gate once. It reports `NEEDS_REVISION` with the context-risk blocker and writes the packs.
2. Read `plan.json`. Review one subagent per `shard-<id>.md` against `rubrics/task-done-review-rubric.md`,
   and one more over `cross-shard.md` when there is more than one shard.
3. Write a result per shard to `docs/metareview/shards/<scope>/<target-slug>/shard-<id>.<shardHash>.result.json`
   and, for a multi-shard plan, `cross-shard.<planHash>.result.json`. The pack states the exact
   contract.
4. Re-run the gate with `--previous-run <run-id>`. With every shard covered and the aggregate
   passing, the context-risk blocker becomes advisory and the deterministic lints run over the whole
   branch diff.

Set `--max-attempts` on the **first** run of the chain; mid-chain it is ignored. Commit the results
with the review log. Editing a file invalidates only its own shard's result: re-review that shard
and the cross-shard pack, and leave the rest. Local (staged, worktree, untracked) content is in no
pack, so on task-done commit or remove it first — an untracked file over 4,000 bytes raises
`UNTRACKED_TRUNCATED`, which shard results can never satisfy.
