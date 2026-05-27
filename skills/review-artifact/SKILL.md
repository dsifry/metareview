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
4. Run the required lenses as parallel subagents by default: Feasibility, Completeness, Scope and alignment, Architecture, Intent preservation. Invoking this artifact-review workflow is explicit authorization to delegate those lenses.
5. Only fall back to `in-session-emulated` when subagents are unavailable or the human explicitly requests no delegation. If falling back, state that the review is not independently adversarial and treat it as weaker evidence.
6. Update the review log with reviewer rows, per-reviewer verdicts, findings, evidence, execution mode, and the aggregate verdict.
7. Always return the actual artifact-review verdict from the reviewer set. Do not substitute a fixed example verdict; `NEEDS_REVISION` and `ESCALATE` are valid review results when supported by findings.
8. Blocking findings must cite file lines, artifact sections, command output, or task IDs.
9. For a re-review, run `metareview review artifact <path> --previous-run <run-id>` so the new run links to the prior attempt.

Use `metareview review artifact <path> --scaffold-only` only when explicitly creating a scaffold without claiming the review is complete.

## Gate Rule

A review execution is incomplete while required reviewer rows are missing, any reviewer lacks a verdict, or the aggregate verdict is `NOT_REVIEWED`. Do not call an artifact implementation-ready while the verdict is `ESCALATE` or `NEEDS_REVISION`, required reviewer rows are missing, reviewer verdicts are missing, or blocking findings remain unresolved unless the human explicitly accepts the risk.
