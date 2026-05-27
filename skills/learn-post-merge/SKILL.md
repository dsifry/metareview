---
name: learn-post-merge
description: Run metareview post-merge learning after a PR or local merge completes; curates reusable review knowledge, records discarded low-value candidates, and appends reviewer calibration.
---

# Learn Post Merge

Run this after a PR merge or after a local epic lands.

## Command

```bash
metareview learn --post-merge <pr-number> --base <pre-merge-ref> [--github-pr <number>] [--session-root <path>]
```

Use `--base` for the final reviewed diff, `--github-pr` when the GitHub PR number differs from the merge identifier, and `--session-root` when the relevant coding session history is in a nonstandard location.

## Workflow

1. Run the command from the repository root after confirmed merge.
2. Inspect the accepted learning log and discard log under `docs/metareview/learning/`.
3. Keep accepted knowledge only when it would change a future reviewer’s behavior on a similar task.
4. Commit the learning artifacts and JSONL knowledge/calibration changes with the merge follow-up work.

GitHub and session history are optional local context. Missing adapters are recorded as unavailable context rather than failing learning.

## Metaswarm Contract

In a metaswarm flow:

- `metareview review task-done <task-id-or-path>` runs when a work unit claims done.
- `metareview review epic-ready <epic-id-or-path>` runs before an epic is called locally complete.
- `metareview review pr-ready --base <base-ref>` runs before PR push or merge readiness.
- `metareview learn --post-merge <pr-number> --base <pre-merge-ref>` runs after confirmed PR merge.

Post-merge learning is advisory by default. In strict mode, the caller treats a nonzero learning exit as blocking release cleanup until the run succeeds or is explicitly waived.
