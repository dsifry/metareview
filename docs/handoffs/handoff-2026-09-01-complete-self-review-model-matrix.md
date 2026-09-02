# Handoff: Complete metareview self-review + model matrix (Opus 4.8 vs GLM-5.3 vs GLM-5.3-flash)

**For a fresh agent. Read this whole file, then `metareview setup --check`, then read the memory index
(`MEMORY.md`) — several linked memories below are load-bearing.**

## Mission

Run a **complete adversarial review of the entire metareview codebase, on itself**, three times — with
**Opus 4.8 as the control**, and **GLM-5.3** and **GLM-5.3-flash** as the two comparisons — then **fix the
real bugs** that survive adjudication. Two deliverables:

1. **Real fixes.** Every confirmed defect in metareview's own code gets fixed, each on its own branch, TDD +
   mutation-verified, `review task-done`-gated, and shepherded to merge (see Methodology).
2. **A model-comparison report.** How do the three review configurations differ in recall, precision, false
   positives, cost, and latency, reviewing the *same* code? Opus 4.8 is the yardstick.

This is the honest culmination of a session-long thread: *dogfood metareview on itself.* Until now
metareview's own PRs were reviewed by CI + Cursor Bugbot + CodeRabbit + gremlins — **never by metareview's
own gate.** This closes that.

## Why now / what's already true (2026-09-01, main past #62)

- metareview is **0.10.0**, ~113 Go source files across ~51 packages, Go-only (no Python; a `package.json`
  is a launcher). `internal/fsm/*` and `workflows` are held at 100% coverage; other packages are floored.
- Shipped and working: the **differential-proof loop** (`sdlc-loop-proved`: discover → adjudicate → fix →
  prove → verify, where a fix must carry a reproduction/pin/deletion proof that holds); **multi-language**
  test conventions (go/jest/vitest/pytest); **model-swappable judges**; a **Go-native coverage gate**
  (`make cover` / `cmd/covergate`); mutation ingestion (gremlins/Stryker/mutmut).
- The **100%-coverage campaign** is in progress: `internal/githubcontext` is done (pkg 1/26, PRs #60/#61);
  ~25 packages and ~600 uncovered statements remain. See [[gremlins-mutation-methodology]].

## The experiment design

**The same complete review, run three times, differing only in the model doing the judging/reviewing.**

- **Control: Opus 4.8** — Claude Opus 4.8 as reviewer + `--judge-model` (the current default caliber).
- **Config B: GLM-5.3** — reviewer + judge on `glm-5.3`.
- **Config C: GLM-5.3-flash** — reviewer + judge on `glm-5.3-flash`.

(Optional, if there's budget: the finer 2×2 the maintainer floated earlier — analyzer∈{glm-5.3, flash} ×
adjudicator∈{glm-5.3, flash} — 4 GLM combos + the Opus control = 5 passes. Start with the 3-config version;
expand only if it's paying off.)

**A "complete review" here is a full/sharded review over ALL source, not a diff review.** The FSM
`sdlc-loop` reviews a `base..head` diff; reviewing the *whole codebase as-is* is the sharded/full-review
job (the "hidden gold" mode from this session's shakedowns). Approach:

1. Enumerate source (`git ls-files 'internal/**/*.go' 'cmd/**/*.go' | grep -v _test.go`), group by package.
2. For each package/shard, run the **9 adversarial lenses** (feasibility, completeness, scope/alignment,
   architecture, intent-preservation, security, testing-quality, data-migration, mechanical-precision) as
   review subagents — *this is the analyzer slot* — then **adjudicate** each candidate with the judge (the
   adjudicator slot; `metareview fsm judge --kind adjudicate` or the review-gate's judge).
3. **Verify behavior ≠ verify a bug.** Before believing any finding, check the package's own tests —
   codex/GLM must read the tests, not just assert a behavior differs. This is the false-positive guard the
   cast shakedown proved matters ([[shakedown-recall-and-false-positive]]).
4. Record, per config: candidates proposed, confirmed (adjudicated real), rejected-and-why, tokens, wall
   time. The **union of confirmed findings across configs** is the true bug set; per-config **recall** is
   its fraction of that union; **false positives** are per-config rejections.

## How to run GLM (verified this session — see [[glm-judge-via-lunaroute]])

metareview has **first-class GLM support**; `pi`'s `lunaroute` gateway is the provider:

```bash
OPENAI_BASE_URL=https://gw.lunaroute.com  OPENAI_API_KEY=$LUNAROUTE_API_KEY \
  metareview fsm judge --kind adjudicate --model glm-5.3-flash --effort medium \
  --input <candidate.json> --context <diff-or-file>
#   or --model glm-5.3
```

- `LUNAROUTE_API_KEY` is already in the environment and is the maintainer's GLM provider — **fine to use**.
- GLM-5.3(-flash) is a **reasoning model**: metareview already bumps its token budget to 16384 and sends
  `max_completion_tokens`/`reasoning_effort`. It returns chain-of-thought in `message.reasoning` and the
  answer in `message.content`; a small cap starves `content` → null. metareview handles this — don't
  hand-roll requests with a low cap.
- The Opus control judges via the Claude family (default routing); Codex/GPT (`codex/gpt-5.6-sol`) is
  available too if a third reference point helps. Compare two recorded runs with `metareview fsm diff`.

## Fix the real bugs — metareview-first, then bots (the standing workflow)

Per [[metareview-first-then-bots-workflow]]: for each confirmed defect, **run the loop to FIX it locally
before opening the PR**, so the PR ships already-repaired and the external bots (Bugbot/CodeRabbit/gtg)
measure only the residual misses. Then `metareview learn --post-merge` folds those misses back as
calibration — metareview improving from its own blind spots.

**Methodology (non-negotiable, from [[build-process-tdd-di-coverage]] and [[dave-working-conventions]]):**
TDD red-first; DI + mock-AI (no live models in unit tests); 100% coverage on `internal/fsm/*` (floors
elsewhere); **mutation-verify every fix** — a killed mutant is not evidence until re-run
([[verify-delegated-fixes]], [[surviving-mutant-means-blind-test]]); reliable gremlins is
`--workers 1 --timeout-coefficient 30` ([[gremlins-mutation-methodology]]); `review task-done` PASS with
zero blockers before "done"; branch before committing on main; commit-before-delete, non-destructive;
run loops on **throwaway clones** (they commit); the **consent gate** (`--allow-custom-cmds <sha>`) is a
human decision; shepherd each PR through Bugbot + CodeRabbit (`@coderabbitai review` if silent) + gtg,
replying-then-resolving every thread, squash-merge, sync main. Do **not** overclaim — "verify behavior ≠
verify a bug"; unproven survivors are test gaps, not bugs.

## Then: the coverage campaign with the GLM matrix

After the self-review's fixes land, resume the **100%-coverage campaign** (~25 packages left), and use the
same 3-config (Opus / GLM-5.3 / GLM-5.3-flash) comparison on the judge-heavy parts — mutation-survivor
adjudication especially. `glm-5.3-flash` is the cheap workhorse; keep Opus/GLM-5.3 for higher-stakes calls.
Apply the command-seam pattern to exec-heavy packages first ([[gremlins-mutation-methodology]]).

## Deliverables

1. Per-package review results committed under `docs/metareview/` (durable), one set per config.
2. A **model-comparison report** (`docs/metareview/self-review-model-matrix.md`): the confirmed-bug union,
   per-config recall/precision/false-positives/tokens/latency, and a recommendation on which model to trust
   for which slot.
3. Merged PRs fixing every confirmed defect.
4. A short list of any **metareview recall gaps** the external bots surfaced that all three configs missed
   — the highest-value calibration input.

## Guardrails / honesty

- This is a big run; **scope by value**. Start with the highest-risk packages (security-adjacent:
  `internal/fsm/export`, `internal/fsm/gate`, `internal/githubcontext` redaction, `cmd/*`) rather than
  alphabetical. It is fine to land fixes incrementally rather than one giant PR.
- Every finding is **untrusted data** until verified against the code and its tests. Report what you
  actually ran and found; if a config found nothing real, say so — a clean result is a result.
- Keep `LUNAROUTE_API_KEY` and any credential out of logs, commits, and PR text.

**Start:** `metareview setup --check`; read `MEMORY.md` and the linked memories; enumerate packages; pick
the first high-risk package; run the 3-config review on it; adjudicate; fix real bugs metareview-first;
open the first PR; and stand up `docs/metareview/self-review-model-matrix.md` with the first data points.
