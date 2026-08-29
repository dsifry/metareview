# metareview context: docs/specs/2026-08-26-metareview-0.9.0-build-plan.md

Run ID: `mrv-20260827-014544247368000-artifact-2026-08-26-metareview-0-9-0-build-plan-517b7aec`

## Target

- Path: `docs/specs/2026-08-26-metareview-0.9.0-build-plan.md`
- Repository mode: `metaswarm-extension`
- Git branch: `fsm-enhancements`
- Git head: `03d5bee`

## Artifact Excerpt

```markdown
# metareview 0.9.0 — TDD build & orchestration plan

> **Status:** REVISION 3 (2026-08-26) — re-review pending. Companion to
> [`2026-08-26-metareview-0.9.0-fsm-enhancements.md`](2026-08-26-metareview-0.9.0-fsm-enhancements.md)
> (the design spec). This document locks the interfaces, corrects the spec where it is wrong about the
> current binary, fixes the CLI contract the spec left open, and sequences the build so independent
> packages can be written in parallel — each test-first, each under a hard 100% coverage gate.
>
> **Revision history.** r1 → review `…011658…` (36 blocking). r2 → review `…013019…` (28/36 resolved;
> 35 new blocking, concentrated in the r2 fork/resume model, hardcoded loop semantics, the `runs.jsonl`
> bridge, guardrail regressions, and test observables). r3 (this) makes persistence **event-sourced**,
> defines resume over **audit sequence positions**, generalizes loop/outcome semantics via **workflow
> keys**, specifies `Delta` application and invariants, replaces the `runs.jsonl` bridge with a versioned
> record and mapped verdict, and tightens guardrails and test observables. §0.2 maps every attempt-2
> blocking finding to its change.
>
> **Inputs:** the design spec; the pi session log (`harnesseval` session `2026-08-25T00-52-28…01a03667`);
> the harnesseval Python that is the port spec (`harnesseval/{judge,adjudicate,sdlc_loop,usage,model_router}.py`);
> the Go binary on `fsm-enhancements`.

---

## 0. Corrections to the spec (locked)

Rows marked **[design change]** alter the spec; all are accepted under §7.

| # | Spec says | Reality / decision |
|---|---|---|
| C1 | "100% coverage as a hard gate" (§18) | **[design change]** Not measured, not 100%, no gate existed. Measured 2026-08-26 on `57221cd`, unit + full shell suite against a `-cover` binary: **86.3% total** (lowest `markdown` 70.0, `learnsource` 70.8, `contextpack` 76.1; highest `integration` 100, `reviewers` 97.2). 0.9.0 adds the gate (§4.1): 100% on new packages, recorded floor on legacy; legacy lift is a follow-up branch (§7.2). |
| C2 | `still-present` outputs `{still_present, confidence}` | **[design change]** Python returns `{reasoning, still_present}` and fails closed. Go prompt adds `confidence`; parser tolerates absence; fail-closed preserved and tested. |
| C3 | `all_fixed`, `bugs_remain` are gates; `all_fixed` also an atom | **[design change]** One implementation `converge.AllFixed`; both gate names and the atom call it. Outcomes distinguish `fixed` / `stalled` / `overflow` / `custom` (§1.6). A give-up is never recorded as `all_fixed`. |
| C4 | `executor: $SESSION` | Superseded by `exec: inline`. |
| C5 | `mrv fsm …` | Binary is `metareview`; `alias mrv=metareview` mentioned once. |
| C6 | "default workflows" plural | **[design change, §7.1]** Ship `sdlc-loop` and `review-loop`; `review-loop` has clean-review outcomes (§6). |
| C7 | `NEEDS_INPUT` one sentence | Full contract §3.4. |
| C8 | Exit codes / JSON shapes | §3.2–3.4; AGENTS.md/CLAUDE.md/quickstart exit-handling amended in M8 (code 3). |
| C9 | Token source | Judge calls self-report; agent records via `record tokens`; both fold into `Tokens` (E10). |
| C10 | `mrv fsm run` | Not built. |
| C11 | Resume "from checkpoint" | **[design change]** Resume forks a child run from an **audit sequence position** (§1.7); parent immutable; the node at the resumed state re-runs and its exit gate is re-evaluated; HEAD is persisted per transition so git preconditions are checkable (`ERR_TREE_NOT_AT_CHECKPOINT`); `--work-dir` on fork. |
| C12 | Overflow end state | **[design change]** Safety stops → state `done`, `outcome: overflow`, status `STOPPED`, exit 1. |
| C13 | `exec: fork` = separate process | **[design change]** `fork` = the binary executes the node out-of-band (HTTP judge or `cmd:`); no respawn. |
| C14 | `fsm gate --input` | Kept (`--input <snapshot.json>`); git-ref fields validated (§1.8). |
| C15 | `fsm record <event>` arbitrary | Kept; u
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
