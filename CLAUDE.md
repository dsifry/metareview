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

Before saying work is done, run the appropriate metareview gate.

- `PASS`/`PASS_ADVISORY` proceed only with zero blockers.
- `NEEDS_REVISION` repairs via `--previous-run <run-id>`.
- `ESCALATED` stops same-target retries; human must narrow, split, or redesign the target.

Exit handling: `0` means verify `PASS`/`PASS_ADVISORY` with zero blockers; `1` with a review path means follow that log; nonzero without a path means read stderr.

## Process Overrides

A blocking finding is normally cleared by fixing it. When that is not possible and the workflow is
deliberately stepped outside of — an escalation — record it rather than working around it:

```bash
metareview override request <finding-id> --reason "<why the workflow was exited>" [--escalation "<context>"]
metareview override grant   <finding-id> --reason "<why the exception is accepted>"
metareview override list [--pending]
```

- **Requesting** is available to whoever is driving the run, including an orchestrating agent. It does
  **not** clear the gate: the finding keeps blocking and `override list --pending` exits nonzero, so CI
  stays red.
- **Granting** is the acknowledgement, and must come from outside the workflow — a human, or an authority
  explicitly designated as such. A reviewing agent never grants an override on its own findings.
- Both halves are recorded with actor, timestamp and reason, rendered under "Process Overrides" in
  `docs/metareview/FINDINGS.md`, and an override is never a fix (`fixedInRunId` stays empty), so post-merge
  learning can analyse exceptions separately from resolutions.

## Lifecycle Placement

- Before implementing a plan or spec: review the artifact.
- After each small implementation chunk: run task-done.
- After all child tasks for an epic are complete: run epic-ready.
- Before opening, pushing, or merging a PR: run pr-ready.
- After confirmed PR merge: run post-merge learning.

## Durable Output

Commit Markdown review/context artifacts in `docs/metareview/`. Keep transient `.metareview/findings.jsonl` and `.metareview/runs.jsonl` local unless the repository explicitly changes that contract.

In metaswarm repositories, use metareview to deepen metaswarm's existing review framework. Do not replace Beads task state, Superpowers workflows, or metaswarm PR shepherding.
