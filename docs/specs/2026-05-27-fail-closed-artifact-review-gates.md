# Fail-Closed Artifact Review Gates

## Problem

`metareview review artifact <path>` currently creates a scaffold and exits successfully while the generated review remains incomplete. The generated run record and Markdown review use `NOT_REVIEWED`, contain no reviewer results, and leave `.metareview/findings.jsonl` empty. In the warmstart-tng run that triggered this work, an agent treated the successful command as review completion even though zero adversarial reviewers had actually run.

This is a product and process hazard: the command is named like a completed review, but artifact review completion is delegated to manual agent behavior.

## Evidence

- `internal/artifactreview/review.go` sets artifact review status to `open` and verdict to `NOT_REVIEWED`.
- `internal/artifactreview/review.go` writes an empty reviewer table and "No reviewer findings recorded yet."
- `cmd/metareview/main.go` exits zero after printing the scaffold path for `review artifact`.
- `internal/reviewlog/reviewlog.go` marks unresolved blockers when Markdown contains `NEEDS_REVISION`, but it does not treat `NOT_REVIEWED` as unresolved.
- `internal/reviewers/prready.go` and `internal/reviewers/epicready.go` only block on unresolved blockers or `NEEDS_REVISION`, so unfinished artifact reviews can be ignored by later gates.
- `skills/review-artifact/SKILL.md` says agents must run the five lenses manually, but the gate rule does not make `NOT_REVIEWED` or missing reviewer rows explicitly blocking.

## Goals

1. Artifact review scaffolds must be clearly incomplete and must not be mistaken for a completed review.
2. Downstream gates must fail closed when they see an artifact review with `NOT_REVIEWED`, `ESCALATE`, missing verdict, or missing required reviewer rows.
3. The review-artifact skill and docs must state that `NOT_REVIEWED` is blocking until all required reviewer lenses are completed.
4. Existing scaffold generation remains available for agents that need a review workspace.

## Non-Goals

- Do not implement LLM or subagent orchestration inside the Go CLI in this slice.
- Do not replace metaswarm as lifecycle owner.
- Do not change deterministic task-done, epic-ready, or pr-ready reviewer logic except for how they consume incomplete prior artifact reviews.

## Proposed Behavior

### Artifact Scaffold Command

`metareview review artifact <path>` will still create the context pack and review log, but it must report that the review is incomplete. To avoid breaking existing automation more than necessary, the primary behavior change is:

- The command exits nonzero after creating a `NOT_REVIEWED` scaffold.
- Stderr explains that the artifact scaffold is not a passing review and lists the completion requirements.
- A new `--scaffold-only` flag keeps the current zero-exit behavior for explicit scaffold generation.

The printed stdout path remains the review log path so agents and scripts can still find the artifact.

### Review Log Semantics

Review log discovery must treat these artifact states as unresolved blockers:

- missing verdict
- `NOT_REVIEWED`
- `ESCALATE`
- `NEEDS_REVISION`
- required artifact reviewer table has no completed row for any of: feasibility, completeness, scope-alignment, architecture, intent-preservation

`PASS` remains acceptable only when no unresolved blocking findings are present. `PASS_ADVISORY` is acceptable only with zero blockers.

### Skill And Docs

The review-artifact skill and quickstart docs must say:

- `review artifact` creates an incomplete scaffold unless the review log is later completed.
- Agents must populate all five reviewer rows or explicitly mark a lens `NOT_APPLICABLE`.
- Agents must re-run with `--previous-run <run-id>` after fixes.
- Completion requires `PASS` or `PASS_ADVISORY` with zero blockers.

## Test Plan

Add tests before production code:

1. CLI: `metareview review artifact docs/plan.md` creates the scaffold, prints its path, and exits nonzero while the review remains `NOT_REVIEWED`.
2. CLI: `metareview review artifact docs/plan.md --scaffold-only` creates the same scaffold and exits zero.
3. Review log discovery: `NOT_REVIEWED` artifact logs report `HasUnresolvedBlockers = true`.
4. Review log discovery: artifact logs with missing required reviewer rows report unresolved blockers.
5. Epic-ready: an epic with a child or related review log in `NOT_REVIEWED` fails with an unresolved review blocker.
6. PR-ready: branch readiness fails when prior review logs include incomplete artifact reviews.
7. Completed artifact review fixture with all five required rows and `PASS` is accepted by discovery and does not block downstream gates.
8. Manifest/docs tests cover the new `--scaffold-only` flag and updated instructions.

## Acceptance Criteria

- The failure reproduced from warmstart-tng cannot recur silently: a scaffold-only artifact review is visibly non-passing and blocks downstream gates.
- Existing agents can still request a scaffold explicitly with `--scaffold-only`.
- Tests prove both fail-closed behavior and the explicit scaffold-only escape hatch.
- `go test ./...` and `bash tests/run-all.sh` pass.
