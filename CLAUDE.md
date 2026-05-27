# metareview Claude Code Instructions

Use metareview as the local review harness for artifacts, code chunks, epics, PR readiness, and post-merge learning.

## Commands

- `/setup` checks repository mode and prerequisites.
- `/review-artifact <path>` reviews specs, plans, designs, decompositions, and docs.
- `/review-task-done <task-id-or-path>` runs the task-done code review gate.
- `/review-epic-ready <epic-id-or-path>` checks parent readiness after child tasks complete.
- `/review-pr-ready --base <base-ref>` checks local PR readiness before push or merge.
- `/learn-post-merge <pr-number> --base <pre-merge-ref>` extracts post-merge learning.
- `/status` reports current review state.

If the plugin command is unavailable in a source checkout, run the CLI directly:

```bash
metareview review task-done <task-id-or-path> --base <base-ref> --evidence <file>
go run ./cmd/metareview review task-done <task-id-or-path> --base <base-ref> --evidence <file>
```

## Completion Rule

Before saying work is done, run the appropriate metareview gate. A `PASS_ADVISORY` result is acceptable only when there is no blocking finding. Any blocker must be fixed and reviewed again with `--previous-run <run-id>`.

## Lifecycle Placement

- Before implementing a plan or spec: review the artifact.
- After each small implementation chunk: run task-done.
- After all child tasks for an epic are complete: run epic-ready.
- Before opening, pushing, or merging a PR: run pr-ready.
- After confirmed PR merge: run post-merge learning.

## Durable Output

Commit Markdown review/context artifacts in `docs/metareview/`. Keep transient `.metareview/findings.jsonl` and `.metareview/runs.jsonl` local unless the repository explicitly changes that contract.

In metaswarm repositories, use metareview to deepen metaswarm's existing review framework. Do not replace Beads task state, Superpowers workflows, or metaswarm PR shepherding.
