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
4. Run the required lenses as parallel subagents by default: Feasibility, Completeness, Scope and alignment, Architecture, Intent preservation, Security, Testing-quality, Data-migration, Mechanical-precision. Invoking this artifact-review workflow is explicit authorization to delegate those lenses. The Security lens uses `rubrics/security-review-rubric.md` (OWASP classes scoped to a diff review). The Testing-quality lens uses `rubrics/testing-quality-rubric.md`. The Data-migration lens uses `rubrics/data-migration-rubric.md`. The Mechanical-precision lens uses `rubrics/mechanical-precision-rubric.md` (can an implementer build exactly what is written, without inventing an undefined detail). Lenses take an adversarial stance: assume there may be a fundamental mistake hiding in this design; hunt for it, do not confirm the artifact is well-shaped.
5. Only fall back to `in-session-emulated` when subagents are unavailable or the human explicitly requests no delegation. If falling back, state that the review is not independently adversarial and treat it as weaker evidence.
6. Update the review log with reviewer rows, per-reviewer verdicts, findings, evidence, execution mode, and the aggregate verdict. Keep orchestrator context/synthesis (checkout notes, filtering rationale, consolidation narrative) in the `## Orchestrator Notes (not findings)` section ONLY — it is audit trail, not a finding stream. Only the `## Findings` section and its classified `## Blocking Findings`, `## Advisory Findings`, `## Follow-up Findings`, and `## Warnings` sections contain review findings. The deterministic gates (eval-injection, missing-test, etc.) remain blocking findings for the task-done verdict; downstream eval extractors should skip findings sourced from `metareview-deterministic/*` and `metareview-session`.
7. Always return the actual artifact-review verdict from the reviewer set. Do not substitute a fixed example verdict; `NEEDS_REVISION` and `ESCALATE` are valid review results when supported by findings.
8. Blocking findings must cite file lines, artifact sections, command output, or task IDs.
9. For a re-review, run `metareview review artifact <path> --previous-run <run-id>` so the new run links to the prior attempt.

Use `metareview review artifact <path> --scaffold-only` only when explicitly creating a scaffold without claiming the review is complete.

## Orchestrator Discipline

The orchestrator (the host agent running this workflow) is a thin dispatcher and aggregator; the
lenses do the analysis. Most review cost lives in the orchestrator and lens subagent turns, so
keep the orchestrator lean:

- **Be terse.** Run the commands, dispatch the lenses, write the review log, return the verdict.
  Do not narrate plans, reasoning, or progress.
- **Trust the lenses; do not re-verify.** After the lenses return, write each lens's findings and
  verdict into the review log and aggregate the verdict. Do not re-read files, re-run `git diff`,
  or re-check lens findings — the lenses already did that work. Re-verifying adds orchestrator
  turns (cost) without improving the review.
- **Keep the aggregation small.** Write each lens's findings into the review log as that lens
  returns (per-lens edits), not held in one large final write. The orchestrator's final reply is
  the verdict plus a one-line summary, not a re-emission of the findings. On large diffs a single
  findings-laden message can overflow the model's per-message output limit and truncate the
  review; per-lens writes avoid this.

## Gate Rule

A review execution is incomplete while required reviewer rows are missing, any reviewer lacks a verdict, or the aggregate verdict is `NOT_REVIEWED`. Do not call an artifact implementation-ready while the verdict is `ESCALATE` or `NEEDS_REVISION`, required reviewer rows are missing, reviewer verdicts are missing, or blocking findings remain unresolved unless the human explicitly accepts the risk.
