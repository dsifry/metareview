# Bounded Review FSM Escalation

## Problem

Metareview and metaswarm correctly block completion while review blockers remain open, but the current workflow leaves the repair loop unbounded. The task-done and PR-ready skills say to fix blockers and re-run with `--previous-run` until the verdict is `PASS` or `PASS_ADVISORY`. The deterministic gates also model ordinary blocker failure as `NEEDS_REVISION` without recording how many autonomous repair cycles have already happened.

That is safe against under-review, but unsafe against review churn. A legitimate bug fix can keep expanding as each review uncovers adjacent risks, test hardening, or cleanup opportunities. In warmstart-tng this showed up as a real LinkedIn import/RLS bug investigation that stayed relevant to the incident, but then entered repeated review-fix-review cycles without a clear stopping condition.

## Goals

1. Preserve the existing quality rule: unresolved blockers still block completion.
2. Add a hard stop for autonomous repair loops after a bounded number of failed metareview gate attempts, and document the matching metaswarm validation-loop stop rule.
3. Distinguish contract blockers from advisories and follow-up findings so review does not continuously widen task scope.
4. Make escalation durable in `.metareview/runs.jsonl`, generated review logs, and downstream gates.
5. Keep `PASS` and `PASS_ADVISORY` semantics unchanged: they are acceptable only with zero blockers.

## Non-Goals

- Do not let agents ignore blockers after the retry cap.
- Do not make `ESCALATED` a passing verdict.
- Do not implement autonomous LLM reviewer orchestration inside the Go CLI.
- Do not replace metaswarm as lifecycle owner when metaswarm is installed.
- Do not require every advisory to become a tracked issue in this slice; the spec only requires the review output to separate advisories and follow-ups from blockers.
- Do not implement a human-waiver subsystem in this slice. `ESCALATED` remains blocking for the same scope and target. Continuing after escalation requires a new task/spec target that represents narrowed, split, or redesigned work.

## Proposed FSM

### Review Verdict States

Metareview lifecycle gates will support these aggregate verdicts:

| Verdict | Meaning | Blocks downstream gates |
| --- | --- | --- |
| `PASS` | Gate passed with zero blocking findings. | No |
| `PASS_ADVISORY` | Gate ran in advisory mode or found non-blocking notes only; zero blockers remain. | No |
| `NEEDS_REVISION` | Blocking findings remain and autonomous repair may continue if attempts remain. | Yes |
| `ESCALATED` | Blocking findings remain, but the autonomous repair budget is exhausted or the workflow requires a human scope decision. | Yes |
| `NOT_REVIEWED` | Artifact scaffold or incomplete review. | Yes |

### State Transitions

```text
START
  -> REVIEW

REVIEW
  -> PASS            when blocker count is 0 and gate effect is strict
  -> PASS_ADVISORY   when blocker count is 0 and gate effect is advisory
  -> NEEDS_REVISION  when blockers exist and attemptNumber < maxAttempts
  -> ESCALATED       when blockers exist and attemptNumber >= maxAttempts

NEEDS_REVISION
  -> FIX
  -> VALIDATE
  -> REVIEW with --previous-run <run-id>

ESCALATED
  -> human decision:
       - narrow scope into a new task/spec target
       - split follow-up work into one or more new task/spec targets
       - redesign the approach and review that new artifact before implementation
```

The default `maxAttempts` is `3` review attempts per gate chain, counting the initial failed gate run as attempt 1. With the default, the third blocker-producing gate run escalates. A chain is identified by following `previousRunId` links back to the first run for the same scope and target.

## Finding Classification

Reviewer output must classify findings before the aggregate verdict is calculated.

| Classification | Definition | Effect |
| --- | --- | --- |
| `BLOCKER` | Violates the task/spec contract, fails validation, creates direct regression risk, compromises security, breaks tenant/data integrity, or prevents reliable operation of the requested workflow. | Keeps the gate in `NEEDS_REVISION` or `ESCALATED`. |
| `ADVISORY` | Improves clarity, maintainability, documentation, test breadth, or future resilience but is not required for the current contract. | May produce `PASS_ADVISORY` only when blocker count is zero. |
| `FOLLOW_UP` | A valid adjacent bug, feature, or risk outside the current contract. | Must not keep the current gate in repair loop unless a human widens scope. |

Re-review prompts and deterministic reviewer rubrics must say that re-review is for original contract compliance, unresolved prior blockers, and regressions introduced by the latest fix. A fresh reviewer may report a new blocker only when it is a direct blocker under the original contract or introduced by the latest repair.

### Persisted Classification Mapping

The current persisted findings model stores `classification` as a string and treats these open findings as unresolved blockers:

- `classification: "spec-contract"`
- `classification: "blocking"` with `severity: "critical"` or `severity: "high"`

This slice must stay backward-compatible with that model. The implementation should add a normalization layer rather than a breaking schema migration:

| Public class | Persisted representation | Counts as blocker |
| --- | --- | --- |
| `BLOCKER` | existing blocker representation: `spec-contract`, or `blocking` with high/critical severity | Yes |
| `ADVISORY` | `advisory` | No |
| `FOLLOW_UP` | `follow-up` | No |

Existing records with `spec-contract` or high/critical `blocking` remain blockers. Unknown classifications are non-blocking unless they match the existing blocker rules, and should be surfaced as warnings in review logs so new reviewer output cannot silently become blocking or non-blocking by accident.

## Run Record Changes

Every task-done, epic-ready, and pr-ready run record should include:

```json
{
  "attemptNumber": 1,
  "maxAttempts": 3,
  "blockingFindingCount": 0,
  "advisoryFindingCount": 0,
  "followUpFindingCount": 0,
  "escalationReason": ""
}
```

When a run escalates:

- `status` is `escalated`.
- `verdict` is `ESCALATED`.
- `escalationReason` names the exhausted budget or explicit escalation trigger.
- `blockingFindingCount` is greater than zero.
- the generated review log includes the full run chain and the unresolved blockers.

Run-chain lookup should be implemented in a shared internal boundary, for example `internal/runchain`, used by task-done, epic-ready, and pr-ready. Each gate may keep its local run-record struct, but attempt calculation, previous-run validation, max-attempt inheritance, and escalated-chain fail-fast behavior must be centralized so the three gates do not drift.

For artifact reviews, completed manual or subagent review logs may already return `ESCALATE`. Downstream discovery should continue treating `ESCALATE` and `ESCALATED` as unresolved blockers. A later cleanup slice may normalize artifact `ESCALATE` to `ESCALATED`, but this spec does not require that rename.

## CLI Behavior

### Existing Commands

The existing review commands keep their current shape:

```bash
metareview review task-done <task-id-or-path> --base <ref> --previous-run <run-id> --evidence <path>
metareview review epic-ready <epic-id-or-path> --base <ref> --previous-run <run-id> --evidence <path>
metareview review pr-ready --base <ref> --previous-run <run-id> --evidence <path>
```

The CLI computes `attemptNumber` from the `previousRunId` chain:

- no previous run and no prior escalated run for the same scope and target: attempt `1`
- previous run attempt `N`: attempt `N + 1`
- missing previous run ID: fail with a clear usage error before writing artifacts
- previous run for a different scope or target: fail with a clear usage error before writing artifacts
- previous run already has `verdict: ESCALATED`: fail fast before writing artifacts
- no previous run, but the latest run for the same scope and target has `verdict: ESCALATED`: fail fast before writing artifacts

### Optional Flags

Add:

```bash
--max-attempts <n>
```

Rules:

- default is `3`
- values below `1` are rejected
- the value is stored on every run
- a chain keeps using the first run's `maxAttempts`; changing the budget for an in-progress chain is out of scope for this slice

This flag exists for unusual workflows; docs should recommend the default.

## Downstream Gate Behavior

Review log discovery must treat these verdicts as unresolved blockers:

- missing verdict
- `NOT_REVIEWED`
- `NEEDS_REVISION`
- `ESCALATE`
- `ESCALATED`

Epic-ready and PR-ready gates must fail when child or prior review logs contain `ESCALATED`, just as they already fail on unresolved blockers.

The generated PR-ready evidence should include:

- latest verdict per relevant target
- current attempt count, such as `3/3`
- whether any target is escalated
- advisory and follow-up counts when available

This requires extending review-log summaries or joining them with `.metareview/runs.jsonl` metadata. The implementation must not infer attempt counts from Markdown text alone.

## Metaswarm Integration

When metaswarm owns lifecycle orchestration, metareview provides gate-attempt results and run metadata. Metareview does not observe validation-only failures that occur before a review gate is run. Metaswarm should track those separately in its execution state, such as `.beads/context/execution-state.md`, with:

- `validationAttemptNumber`
- `maxValidationAttempts`, default `3`
- latest validation command and exit code
- latest failing evidence path or log excerpt
- `ESCALATED` work-unit status when validation failures exhaust the budget

Metaswarm should apply this transition rule:

```text
VALIDATION_FAIL or REVIEW_FAIL
  if validationAttemptNumber or review attemptNumber is below its max: repair and re-run
  else: stop autonomous repair, mark work unit ESCALATED, report failure history
```

Escalation should not be treated as abandoned work. It is a controlled stop that asks for a human decision:

- create a new task/spec target for continued work in the same product area
- narrow the scope
- split a follow-up bead
- redesign the approach

In this slice, metareview does not provide a same-target restart override after escalation. If the human decides to continue, the work must be represented as a new task/spec target so the new contract is explicit and independently reviewable. This slice only documents metaswarm validation-loop state. It does not require the metareview CLI to write `.beads/context/execution-state.md`.

## Documentation Changes

Update:

- `skills/review-task-done/SKILL.md`
- `skills/review-epic-ready/SKILL.md`
- `skills/review-pr-ready/SKILL.md`
- `docs/quickstart.md`
- `docs/README.codex.md`
- `docs/README.claude.md`
- `INSTALL.md`
- metaswarm integration docs

Required wording:

- Re-run blockers with `--previous-run`, but stop after the configured max attempts.
- `ESCALATED` is blocking and not a pass.
- Advisory and follow-up findings do not extend the repair loop unless the human widens scope.
- A fresh re-review may introduce new blockers only for the original contract or regressions introduced by the latest fix.
- Metareview caps review-gate attempts; metaswarm caps validation-only attempts in execution state.

## Test Plan

Add tests before implementation:

1. Task-done first blocker run records `attemptNumber: 1`, `maxAttempts: 3`, `verdict: NEEDS_REVISION`, and exits nonzero.
2. Task-done second blocker run with `--previous-run` records `attemptNumber: 2`, preserves the run chain, and remains `NEEDS_REVISION`.
3. Task-done third blocker run with `--previous-run` records `attemptNumber: 3`, `verdict: ESCALATED`, `status: escalated`, non-empty `escalationReason`, and exits nonzero.
4. A fourth task-done run against the same escalated chain fails fast before writing artifacts. Continuing after escalation requires a new task/spec target created after a human decision.
5. `--max-attempts 2` escalates on the second blocker run.
6. Invalid `--max-attempts 0` exits with usage code and writes no artifacts.
7. Missing `--previous-run` target fails before writing artifacts when the ID does not exist.
8. Mismatched `--previous-run` scope or target fails before writing artifacts.
9. Epic-ready first/second/third blocker runs follow the same attempt and escalation behavior as task-done.
10. PR-ready first/second/third blocker runs follow the same attempt and escalation behavior as task-done.
11. The shared run-chain boundary has unit tests for first run, missing previous run, mismatched previous-run scope/target, max-attempt inheritance, escalated previous-run fail-fast, and same-target new-run fail-fast after escalation.
12. Advisory-only and follow-up-only findings persist their counts and do not produce `NEEDS_REVISION` or `ESCALATED`.
13. Mixed blocker/advisory/follow-up findings persist all three counts, while only blocker count controls `NEEDS_REVISION` or `ESCALATED`.
14. Existing findings with `spec-contract` or high/critical `blocking` classification still count as unresolved blockers.
15. Unknown classifications that do not match legacy blocker rules are non-blocking and are surfaced as review-log warnings.
16. PR-ready fails when prior review logs include `ESCALATED`.
17. Epic-ready fails when child review logs include `ESCALATED`.
18. Review log discovery treats `ESCALATE` and `ESCALATED` as unresolved.
19. Generated PR-ready evidence includes attempt count and escalated status by reading structured run metadata.
20. Manifest/docs tests assert the bounded-loop instructions, classification mapping, validation-loop ownership, and `ESCALATED` semantics.

## Acceptance Criteria

- Agents can no longer loop indefinitely on the same gate chain without an explicit human decision.
- Same-target restart after `ESCALATED` fails fast; continuation requires a new task/spec target.
- `ESCALATED` appears in run records and review logs when blockers remain at the attempt cap.
- `ESCALATED` blocks epic-ready and PR-ready.
- `PASS` and `PASS_ADVISORY` remain acceptable only with zero blocking findings.
- Advisory and follow-up findings are visibly separated from blockers.
- Documentation tells agents that review iteration closes contract blockers and does not continuously expand scope.
- Metaswarm validation-only loops have documented execution-state fields and the same default attempt cap.
- `go test ./...` and `bash tests/run-all.sh` pass.
