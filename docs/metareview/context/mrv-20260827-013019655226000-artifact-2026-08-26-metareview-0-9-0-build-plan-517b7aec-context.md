# metareview context: docs/specs/2026-08-26-metareview-0.9.0-build-plan.md

Run ID: `mrv-20260827-013019655226000-artifact-2026-08-26-metareview-0-9-0-build-plan-517b7aec`

## Target

- Path: `docs/specs/2026-08-26-metareview-0.9.0-build-plan.md`
- Repository mode: `metaswarm-extension`
- Git branch: `fsm-enhancements`
- Git head: `7ad0d83`

## Artifact Excerpt

```markdown
# metareview 0.9.0 — TDD build & orchestration plan

> **Status:** REVISION 2 (2026-08-26) — re-review pending. Companion to
> [`2026-08-26-metareview-0.9.0-fsm-enhancements.md`](2026-08-26-metareview-0.9.0-fsm-enhancements.md)
> (the design spec). This document locks the interfaces, corrects the spec where it is wrong about the
> current binary, fixes the CLI contract the spec left open, and sequences the build so independent
> packages can be written in parallel — each test-first, each under a hard 100% coverage gate.
>
> **Revision 2** answers artifact review
> `mrv-20260827-011658958410000-artifact-2026-08-26-metareview-0-9-0-build-plan-517b7aec`
> (8 lenses, NEEDS_REVISION, 36 blocking). §0.1 maps every blocking finding to the change that resolves it.
>
> **Inputs:** the design spec; the pi session log that produced it (`harnesseval` session
> `2026-08-25T00-52-28…01a03667`); the harnesseval Python that is the port spec
> (`harnesseval/{judge,adjudicate,sdlc_loop,usage,model_router}.py`); the Go binary on `fsm-enhancements`.

---

## 0. Corrections to the spec (locked)

Rows marked **[design change]** alter the spec rather than clarify it; each is accepted by Dave's go
decision of 2026-08-26 or listed in §7 for explicit acceptance.

| # | Spec says | Reality / decision |
|---|---|---|
| C1 | "100% coverage as a hard gate" (§18) | **[design change]** Not measured, not 100%, no gate existed. Measured 2026-08-26 on `57221cd`, unit + full shell suite against a `-cover` binary: **86.3% total** (lowest `markdown` 70.0, `learnsource` 70.8, `contextpack` 76.1; highest `integration` 100, `reviewers` 97.2). 0.9.0 adds the gate (§4.1): 100% on new packages, recorded floor on legacy; legacy lift is a follow-up branch (§7.2). |
| C2 | `still-present` outputs `{still_present, confidence}` | Python returns `{reasoning, still_present}` and fails closed. Go prompt adds `confidence` (not calibration-relevant); parser tolerates absence; fail-closed preserved and tested. |
| C3 | `all_fixed`, `bugs_remain` are gates; `all_fixed` also a convergence atom | **[design change]** One implementation, three names. `converge.AllFixed(snap)` is the single function; `gate.Builtin["all_fixed"]` and `["bugs_remain"]` call it, and the `all_fixed` atom calls it. At the verify boundary the machine evaluates, in order: `all_fixed` → `verify→done` with `outcome: fixed`; else the workflow's `convergence` predicate → if it fires, `verify→done` with `gate: <atom name>` and `outcome: stalled | overflow | custom`; else `bugs_remain` → `verify→discover`. A give-up is **never** recorded as `all_fixed` (§1.6). |
| C4 | `executor: $SESSION` | Superseded by `exec: inline`. |
| C5 | `mrv fsm …` | Binary is `metareview`; `alias mrv=metareview` mentioned once in docs. |
| C6 | "default workflows" plural, built-ins only | **[design change, accepted §7.1]** Ship `sdlc-loop` and `review-loop`. `review-loop` is `discover → adjudicate → done` with **clean-review outcomes** (§6): zero findings or zero confirmed is `outcome: clean`, not a gate error. |
| C7 | `NEEDS_INPUT` one sentence | Full contract §3.4. |
| C8 | Exit codes / JSON shapes unspecified | §3.2–3.4; AGENTS.md/CLAUDE.md exit-handling text is amended in M8 to add code 3 (§3.3). |
| C9 | Token source open | Judge calls self-report; agent records session totals via `record tokens`; both fold into `Snapshot.Tokens` and both are tested against `budget` (E10). |
| C10 | `mrv fsm run` optional | Not built; `fsm state` carries `next_action`. |
| C11 | Resume "from checkpoint" | **[design change]** Resume **forks a child run** and never mutates the parent (§1.7). Entry gate of the resumed state is re-evaluated. Git side effects are checked (`ERR_TREE_NOT_RESET`). Workflow changes are detected (`WorkflowHash`) and must be explicitly accepted. |
| C12 | Overflow end state unspecified | **[design change]** Safety stops end in state `done` with `outcome: overflow`, CLI `status: STOPPED`, 
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
