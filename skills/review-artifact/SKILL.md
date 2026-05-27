---
name: review-artifact
description: Review a spec, plan, design, decomposition, architecture doc, pre-mortem, runbook, or acceptance report with metareview.
---

# metareview review artifact

Use when reviewing a Markdown artifact before implementation or before a gate is considered passed.

## Workflow

1. Run `metareview review artifact <path>` to create the review scaffold. The command exits nonzero while the review is still `NOT_REVIEWED`; this is expected and is blocking.
2. Read the generated context pack and review log path.
3. Use `rubrics/artifact-review-rubric.md`.
4. Run the listed reviewer lenses independently. If true subagents are unavailable, record `in-session-emulated`.
5. Update the review log with reviewer rows, verdict, findings, and evidence.
6. Blocking findings must cite file lines, artifact sections, command output, or task IDs.
7. For a re-review, run `metareview review artifact <path> --previous-run <run-id>` so the new run links to the prior attempt.

Use `metareview review artifact <path> --scaffold-only` only when explicitly creating a scaffold without claiming the review is complete.

## Gate Rule

Do not call an artifact implementation-ready while the verdict is `NOT_REVIEWED`, `ESCALATE`, `NEEDS_REVISION`, missing required reviewer rows, or while blocking findings remain unresolved unless the human explicitly accepts the risk.
