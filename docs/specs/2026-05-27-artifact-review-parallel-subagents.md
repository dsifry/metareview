# Artifact Review Parallel Subagent Default

## Problem

The 0.2.0 artifact-review gate fails closed on incomplete scaffolds, but the process directions still leave room for one agent to treat the five reviewer lenses as a self-review. That weakens the adversarial review guarantee that prompted the gate.

## Requirements

- Artifact review means five independent reviewer lenses: Feasibility, Completeness, Scope and alignment, Architecture, and Intent preservation.
- The artifact-review workflow itself is explicit authorization to delegate those reviewer lenses.
- When subagents are available, run the five reviewer lenses as parallel subagents by default.
- When subagents are unavailable, or the human explicitly requests no delegation, record the execution mode as `in-session-emulated`.
- In-session emulation must state that the review is not independently adversarial and should be treated as weaker evidence.
- New artifact review scaffolds must not pre-label the review as `in-session-emulated`; the scaffold should start in a pending/delegation-intended mode and instruct agents to update the mode after real reviewer execution.
- A review execution is incomplete while required reviewer rows are empty, a reviewer lacks a verdict, or the aggregate verdict is `NOT_REVIEWED`.
- The artifact review must report the actual artifact-review verdict returned by the reviewer set, including `NEEDS_REVISION` or `ESCALATE` when that is what the review found. It must not force a deterministic example verdict; downstream readiness claims are what require zero unresolved blockers or explicit human acceptance.

## Non-Goals

- Do not implement LLM or subagent orchestration inside the Go CLI in this slice.
- Do not change deterministic task-done, epic-ready, or pr-ready reviewer logic.
- Do not require a specific final verdict from every artifact review run.

## Acceptance

- `skills/review-artifact/SKILL.md` states that parallel subagents are the default artifact-review execution mode and preserves the named five-lens set: Feasibility, Completeness, Scope and alignment, Architecture, and Intent preservation.
- The skill states that artifact-review invocation counts as explicit authorization for delegation.
- The fallback mode is named `in-session-emulated`, limited to unavailable subagents or explicit human no-delegation requests, marked not independently adversarial, and treated as weaker evidence.
- The legacy JavaScript implementation is removed so artifact-review scaffold generation has a single Go implementation path; the Go scaffold starts in pending/delegation-intended mode and instructs agents to update execution mode after real reviewer execution.
- The skill says review execution completion requires populated required reviewer rows, per-reviewer verdicts, and an aggregate verdict other than `NOT_REVIEWED`; artifact readiness still requires zero unresolved blocking findings unless explicitly human-accepted.
- Quickstart and agent integration docs mention the subagent default, the unavailable-subagent or human-no-delegation fallback trigger, and the weaker-evidence caveat.
- The skill tells agents to return the actual artifact-review verdict instead of substituting a fixed example result.
- `bash tests/manifest/test-skills.sh`, `bash tests/manifest/test-manifests.sh`, and `bash tests/go/test-artifact-review.sh` assert the new delegation, fallback, single-implementation, scaffold, completion, and actual-verdict contract text.
