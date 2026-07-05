# Metaswarm Integration

Metareview extends metaswarm review gates without taking ownership of metaswarm lifecycle files or hooks.

## Inspected Surfaces

This contract was drafted after inspecting a local metaswarm repository checkout on 2026-05-26. The concrete surfaces metareview integrates with are:

- `AGENTS.md` and `CLAUDE.md` for quality gates, Beads usage, self-reflection, and completion requirements
- `.claude/commands/start-task.md` for starting the tracked task pipeline
- `.claude/commands/pr-shepherd.md` and `skills/pr-shepherd/SKILL.md` for PR monitoring through CI, review comments, and merge readiness
- `.claude/commands/self-reflect.md` for learning capture before branch finish or after merge
- `skills/orchestrated-execution/SKILL.md` for the four-phase work-unit loop
- `skills/plan-review-gate/SKILL.md` and `rubrics/adversarial-review-rubric.md` for adversarial review gates

The machine-readable descriptor is `docs/integrations/metaswarm.integration.json`.

## Flow Contract

| Metaswarm stage | Metareview command | Gate behavior |
| --- | --- | --- |
| Artifact ready for implementation | `metareview review artifact <path>` | Creates a fail-closed scaffold; remains blocking while verdict is `NOT_REVIEWED`, reviewer rows are incomplete, or blockers remain. |
| Work unit claims done | `metareview review task-done <task-id-or-path> --base <base-ref> --evidence <file>` | Blocks task closure on unresolved blocking findings. |
| Epic locally complete | `metareview review epic-ready <epic-id-or-path> --base <base-ref> --evidence <file>` | Blocks epic landing on integration, acceptance, or intent-drift findings. |
| PR ready to push or merge | `metareview review pr-ready --base <base-ref> --evidence <file>` | Blocks PR readiness on branch-level blockers. |
| Confirmed PR merge | `metareview learn --post-merge <pr-number> --base <pre-merge-ref>` | Curates accepted/discarded learning and reviewer calibration. |

For lifecycle gates, `NEEDS_REVISION` means metaswarm should repair and re-run the same gate with `--previous-run <run-id>`. `ESCALATED` means the same-target autonomous loop is exhausted; human must narrow, split, or redesign the target.

Post-merge learning is advisory by default. In normal mode, a learning failure should be recorded and release cleanup may continue. In strict mode, the caller treats a nonzero learning exit as blocking release cleanup until the learning run succeeds or is explicitly waived.

Automatic hook installation is out of scope for this slice. Metaswarm remains the lifecycle owner; metareview supplies commands, review artifacts, and knowledge updates that metaswarm can invoke explicitly.
