# metareview context: docs/specs/2026-05-27-artifact-review-parallel-subagents.md

Run ID: `mrv-20260527-184753259088000-artifact-2026-05-27-artifact-review-parallel-subagents-cf4f2168`

## Target

- Path: `docs/specs/2026-05-27-artifact-review-parallel-subagents.md`
- Repository mode: `metaswarm-extension`
- Git branch: `main`
- Git head: `9ce6ef2`

## Artifact Excerpt

```markdown
# Artifact Review Parallel Subagent Default

## Problem

The 0.2.0 artifact-review gate fails closed on incomplete scaffolds, but the process directions still leave room for one agent to treat the five reviewer lenses as a self-review. That weakens the adversarial review guarantee that prompted the gate.

## Requirements

- Artifact review means five independent reviewer lenses: Feasibility, Completeness, Scope and alignment, Architecture, and Intent preservation.
- The artifact-review workflow itself is explicit authorization to delegate those reviewer lenses.
- When subagents are available, run the five reviewer lenses as parallel subagents by default.
- When subagents are unavailable, or the human explicitly requests no delegation, record the execution mode as `in-session-emulated`.
- In-session emulation must state that the review is not independently adversarial and should be treated as weaker evidence.
- A review is incomplete while required reviewer rows are empty, a reviewer lacks a verdict, blocking findings are unresolved, or the aggregate verdict is `NOT_REVIEWED`.
- The artifact review must report the actual artifact-review verdict returned by the reviewer set. It must not force a deterministic example verdict; downstream readiness claims are what require zero unresolved blockers or explicit human acceptance.

## Non-Goals

- Do not implement LLM or subagent orchestration inside the Go CLI in this slice.
- Do not change deterministic task-done, epic-ready, or pr-ready reviewer logic.
- Do not require a specific final verdict from every artifact review run.

## Acceptance

- `skills/review-artifact/SKILL.md` states that parallel subagents are the default artifact-review execution mode.
- The skill states that artifact-review invocation counts as explicit authorization for delegation.
- The fallback mode is named `in-session-emulated` and marked not independently adversarial.
- Quickstart and agent integration docs mention the subagent default and fallback caveat.
- Tests assert the presence of the new behavioral contract.

```

## Service Inventory

No service inventory found.

## Knowledge Facts

No Beads knowledge facts found.

## Suggested Reviewers

- feasibility
- completeness
- scope/alignment
- architecture
- intent preservation
