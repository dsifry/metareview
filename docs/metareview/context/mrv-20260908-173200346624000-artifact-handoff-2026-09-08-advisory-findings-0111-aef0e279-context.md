# metareview context: .handoffs/handoff-2026-09-08-advisory-findings-0111.md

Run ID: `mrv-20260908-173200346624000-artifact-handoff-2026-09-08-advisory-findings-0111-aef0e279`

## Target

- Path: `.handoffs/handoff-2026-09-08-advisory-findings-0111.md`
- Repository mode: `metaswarm-extension`
- Git branch: `advisory-findings-system`
- Git head: `f12650e`

## Artifact Excerpt

```markdown
# Handoff: 0.11.1 — phrasing discipline, six new hunt clauses, and the advisory-findings system

**Date**: 2026-09-08 · **Branch**: `main` @ `4e499d6` (v0.11.0) · **Author session**: Pi (benchmark + design session, `~/Developer/harnesseval`)

> Fresh agent, zero prior context. Implement the changes below in THIS repo and cut the
> **0.11.1** release. Everything here is evidence-backed and **design-ratified by the
> maintainer** — implement, do not relitigate (§3 decisions are marked RATIFIED). Evidence
> artifacts live in `~/Developer/harnesseval/analysis/` — **read-only**; do not modify that
> repo (a separate session owns the benchmark re-runs and the adjudicator follow-up, §9).

## 1. Objective

Ship metareview **0.11.1**: (A) assertive defect-claim phrasing discipline, (B) six new
hunt clauses, (C) the advisory-findings system (real, important, not-defects) with its three
gates, the smell/nit boundary, simplification hunts, and an in-loop staff-surrogate filter
applied **to advisories only**. No new lens (the ten stand), no era change, no numeric caps.
Then release per §6.

## 2. Background — the evidence this is built on (show your work)

### 2.1 Where 0.11.0 landed

The 0.11.0 lens upgrade (Runtime-reliability + five brief enhancements) was re-measured on
the benchmark cell mrv × glm-5.3-background × low, all six top-6 PRs, under the hardened v3
adjudicator (k=1 quick pass; batch `20260908-mrv011b-glm53-low`):

| harness | rec | hid/PR (confirmed bugs) | imp/PR | hal/PR | findings/PR |
|---|---:|---:|---:|---:|---:|
| mrv 0.10.x (8 lenses, same adjudicator) | 0.72 | 21.0 | 1.8 | 2.5 | — |
| **mrv 0.11.0 (10 lenses)** | **0.76** | **31.7** | 1.2 | 3.8 | 65.2 |
| CE (Compound Engineering, same adjudicator) | 0.81 | 39.0 | 21.8 | 4.3 | 77.8 |

The upgrade worked: hidden gold +51%, recall +4 points, acceptance findings 7/8 caught
(the one miss — whitespace-stripping on username split — is a FORMAT-DRIFT-family miss,
§5). Full details: `~/Developer/harnesseval/analysis/CE_RESIDUE_AFTER_UPGRADE.md`.

### 2.2 The remaining 7.3/PR gap to CE, decomposed

All 221 CE confirmed bugs were semantic-matched against the 0.11.0 run's findings
(`analysis/residue_tasks.json` + `analysis/residue_results_*.json`):

| component | ~size/PR | real miss? |
|---|---:|---|
| Golden-absorbed (mrv's counterpart matched a golden instead) | ~7 | **No** — incr-recall 0.96 vs CE 0.97 |
| Never reported by mrv | ~3–4.5 (27 items; CE restates 2–3×) | Yes — §3B addresses the ~6 that are new patterns |
| Reported but not confirmed (mrv's phrasing judged important/hallucination) | ~1.5 | Half-real — §3A addresses the phrasing half |

### 2.3 The phrasing evidence (why §3A)

The flip analysis (`analysis/all_matches.json`, 116 flip pairs, fixture at
harnesseval `tests/fixtures/flip_pairs.json`): near-verbatim same-issue texts get opposite
verdicts across runs — the `.env.example` openssl case is literally identical wording,
`bug` in one run, `true_hallucination` in another. Driver: **hedged phrasing ("may
produce nil", "presumably") gets punished while assertive phrasings of the same mechanism
get confirmed.** ~9% of identical issues flip verdict on phrasing alone.

### 2.4 The advisory evidence (why §3C)

CE produces **21.8 important-non-bug findings/PR vs our 1.2** under the same adjudicator.
These are real, specific, grounded-in-diff concerns that are not defects — the
"senior-reviewer-would-actually-say-this" class. Concrete examples from this cell:

- the client `ZAddGuestsInputSchema` duplicates the server's schema and **has already
  drifted** — the server copy lacks the uniqueness refinement;
- the credential-fetch block (~25 lines) is duplicated verbatim in
  `createAllCalendarEvents` and `updateAllCalendarEvents`;
- the spec joins usernames with a bare comma, so the whitespace path is never exercised.

The lab already counts this class (`hidden findings = bug_ungold + important_non_bug`,
"so bug-recall and i
```

## Service Inventory

No service inventory found.

## Knowledge Facts

No Beads knowledge facts found.

## Suggested Reviewers

- Feasibility
- Completeness
- Scope and alignment
- Architecture
- Intent preservation
- Security
- Testing-quality
- Data-migration
- Runtime-reliability
- Mechanical-precision
