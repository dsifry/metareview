# metareview context: docs/specs/2026-08-26-metareview-0.9.0-build-plan.md

Run ID: `mrv-20260827-011658958410000-artifact-2026-08-26-metareview-0-9-0-build-plan-517b7aec`

## Target

- Path: `docs/specs/2026-08-26-metareview-0.9.0-build-plan.md`
- Repository mode: `metaswarm-extension`
- Git branch: `fsm-enhancements`
- Git head: `627b7d7`

## Artifact Excerpt

```markdown
# metareview 0.9.0 — TDD build & orchestration plan

> **Status:** READY FOR GO/NO-GO. Companion to
> [`2026-08-26-metareview-0.9.0-fsm-enhancements.md`](2026-08-26-metareview-0.9.0-fsm-enhancements.md)
> (the design spec). This document locks the interfaces, corrects the spec where it is wrong
> about the current binary, fixes the CLI contract the spec left open, and sequences the build
> so that independent packages can be written in parallel — each test-first, each under a
> hard 100% coverage gate.
>
> **Inputs:** the design spec; the pi session log that produced it
> (`harnesseval` session `2026-08-25T00-52-28…01a03667`); the harnesseval Python that is the
> port spec (`harnesseval/{judge,adjudicate,sdlc_loop,usage,model_router}.py`); the current
> Go binary on branch `fsm-enhancements` @ `57221cd`.

---

## 0. Corrections to the spec (lock these before code)

| # | Spec says | Reality / decision |
|---|---|---|
| C1 | "The Go binary already has 100% test coverage as a hard quality gate" (§18) | **Not measured, and not 100%.** No coverage gate exists in `tests/` or CI, and no `_test.go` was ever deleted — the belief came from the behavioral shell suite (`tests/go/*.sh`), which `go test -cover` cannot see. Measured 2026-08-26 on `57221cd` with **unit tests + the full shell suite** run against a `-cover`-instrumented binary (`GOFLAGS=-cover GOCOVERDIR=… bash tests/run-all.sh`): **86.3% total**; lowest `markdown` 70.0%, `learnsource` 70.8%, `contextpack` 76.1%, `knowledge` 77.8%, `tasksource` 79.2%, `cmd/metareview` 80.4%, `artifactreview` 80.4%; highest `integration` 100%, `reviewers` 97.2%, `githubcontext` 95.9%. (Unit tests alone: several packages at 0–30%.) **Decision:** M0 adds the gate that was believed to exist — combined unit+behavioral coverage via `GOFLAGS=-cover` (no script changes needed) — enforcing **100% on every `internal/fsm/...` package** and a **recorded per-package floor** (today's numbers) on legacy packages. Whether 0.9.0 also lifts legacy to 100% is §7 Q2. |
| C2 | `still-present` outputs `{still_present, confidence}` (§6) | The Python prompt returns `{reasoning, still_present}` only, and **fails closed** (parse failure ⇒ still present). **Decision:** the Go prompt asks for `confidence` too (this prompt is not part of Martian calibration, so it is safe to extend); the parser tolerates its absence (`confidence: 0`), and fail-closed is preserved and tested. |
| C3 | `bugs_remain` / `all_fixed` are listed as gates *and* `all_fixed` as a convergence atom | At the `verify` boundary the **convergence predicate** decides; `all_fixed` and `bugs_remain` are the two named outcomes of that evaluation, not separate gates. Both names stay valid in YAML (`verify→done: {gate: all_fixed}`, `verify→discover: {gate: bugs_remain}`) and resolve to the predicate result. |
| C4 | `executor: $SESSION` on the fix node (§5) | Superseded by `exec: inline`. Dropped from the shipped YAML. |
| C5 | CLI is written as `mrv fsm …` | The binary is `metareview`. All commands are `metareview fsm …`; docs mention `alias mrv=metareview` once. |
| C6 | "default workflows" (plural) ship built-ins only (§13, §16) | Only `sdlc-loop` was ever defined. **Decision (needs confirmation, §7):** ship two — `sdlc-loop` (the full loop) and `review-loop` (`discover→adjudicate→done`, read-only: the existing artifact/task-done review expressed as an FSM). |
| C7 | `advance` "returns NEEDS_INPUT for the agent to satisfy" — one sentence, no contract | Defined in §3.4 below: JSON shape, exit code 3, and the `record node-output` hand-back. |
| C8 | Exit codes and JSON output shapes: not specified | Defined in §3.5 below. |
| C9 | Token budget "source" is an open question (§15.4) | Judge calls self-report (the binary made them); the agent records session totals via `fsm record tokens`. **The Python silently drops judge tokens** (`_score_tin` is a dead key) — the Go port counts them, and a test asserts it. |
| C10 
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
