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
metareview review artifact <path>
metareview review task-done <task-id-or-path> --base <base-ref> --evidence <file>
metareview review epic-ready <epic-id-or-path>
metareview review pr-ready --base <base-ref>
metareview learn --post-merge <pr-number> --base <pre-merge-ref>
```

In a source checkout without a packaged binary, use:

```bash
go run ./cmd/metareview review task-done <task-id-or-path> --base <base-ref> --evidence <file>
```

## Agent Contract

Claude Code agents must resolve every blocking finding before claiming completion. A `NOT_REVIEWED` artifact scaffold is also blocking; complete the required reviewer rows and final verdict before treating the artifact as reviewed. Re-run the review with `--previous-run <run-id>` after fixes. `PASS_ADVISORY` is acceptable only when the report has zero blocking findings.

Commit durable review and context Markdown under `docs/metareview/`. Leave transient `.metareview/findings.jsonl` and `.metareview/runs.jsonl` local.

## Metaswarm Repositories

metareview augments metaswarm. It does not replace metaswarm's Beads task state, Superpowers workflows, or PR shepherding. Use it as the deeper review harness at artifact, task-done, epic-ready, pr-ready, and post-merge checkpoints.
