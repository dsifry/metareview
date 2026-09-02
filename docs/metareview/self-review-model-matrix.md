# metareview self-review: model matrix (Opus 4.8 vs GLM-5.3 vs GLM-5.3-flash)

**metareview reviewing metareview.** First time metareview's own adversarial gate has been run on
metareview's own source. Analyzer = Opus 4.8 lens-review subagents; adjudicators compared across three
model configs on the *same* candidate findings.

_Run: 2026-09-01/02. Base: main @ cbaa849 (0.10.0)._

## Method

- **Analyzer slot (candidate generation):** Opus 4.8 subagents, one per package, applying metareview's 9
  adversarial lenses against source **and tests** ("verify behavior ≠ verify a bug" — every candidate
  checked against the package's own tests before filing).
- **Adjudicator slot (the comparison):** each candidate finding was adjudicated by all three configs on
  **identical** input (finding + full package source+tests as context), using metareview's own adjudicate
  contract (a finding is REAL when a maintainer must act before merge — including an *invariant nothing
  holds*, an *assertion that cannot fail*, and a *doc/comment/test-name asserting a property the code
  lacks*, not only a wrong computation).
  - GLM-5.3 / GLM-5.3-flash: metareview's dogfooded `fsm judge --kind adjudicate` via the lunaroute
    OpenAI-compatible gateway.
  - Opus 4.8: `claude -p` (OAuth) with the same adjudicate contract text. **Caveat:** `claude -p` carries
    ~25k cached tokens of Claude Code harness overhead per call, so Opus *cost* is not a bare-API number;
    Opus *latency* and *verdict quality* are comparable. metareview's judge has a `codex/` local-binary
    transport but **no `claude`/OAuth transport**, so the Opus control cannot currently run through
    `fsm judge` without an Anthropic API key — noted as a candidate metareview enhancement.
- **Ground truth:** Opus (session) verification against the real code + tests, recorded per finding.
- **Infra note:** GLM-5.3-flash was in a sustained provider outage (503 INFERENCE_UNAVAILABLE) at the start
  of the run and recovered mid-run; all three configs cover every candidate below.

## Confirmed real-bug union (the true bug set)

Bugs metareview's self-review found that the prior process (CI + Cursor Bugbot + CodeRabbit + gremlins)
had **not** caught:

| ID | Package | Sev | Defect | Status |
| --- | --- | --- | --- | --- |
| ghctx-1 | internal/githubcontext | HIGH | `redactMatch` leaks an unencrypted PEM private-key body (splits at base64 `=` padding) | **Merged — #67** |
| ghctx-2 | internal/githubcontext | MED | `redactMatch` leaks a value prefix when the value contains `:` (`token=a:b`) | **Merged — #67** |
| reviewers-1 | internal/reviewers | HIGH | `childEvidencePassed` accepts "broke"/"bypassed" as passing (unanchored `ok`/`pass`) → epic gate false-pass | **Merged — #68** |
| reviewers-2 | internal/reviewers | MED | `servicePathPattern` bare-word alt flags docs/config as service changes → spurious blockers | **Merged — #68** |
| reviewers-3 | internal/reviewers | LOW | `violatesNoEvalIntent` matches `retrieval(` (no `\b`) → false intent-drift blocker | **Merged — #68** |
| gate-5 | internal/fsm/gate | MED | `RealExec` security-hardening flags/env are unpinned by any test (deletable, tests still green) | **Merged — #70** |
| prready-3 | internal/prready | LOW | `readEvidence` truncates at a raw byte boundary → can split a UTF-8 rune | **Merged — #69** |
| export-2 | internal/fsm/export | LOW | dead/tautological assertion in `TestF8Redaction` (can never fail) | open (low, documented) |
| prready-1 | internal/prready | MED | `verdictForCounts` ESCALATED branch (`>=`) unpinned across the repo | **Merged — #69** |

_(export-1 audit-log fail-open and prready-2 unreachable-PASS are lower-confidence / debatable; see
per-config verdicts.)_

## Per-config adjudication results

### internal/fsm/gate (6 candidates: 1 real, 5 precision probes)

All three configs scored **6/6** — rejected all 5 precision probes, confirmed the 1 real finding.

| id | truth | Opus | GLM-5.3 | GLM-5.3-flash |
| --- | --- | --- | --- | --- |
| gate-1 (regex-alt premise) | false | False 0.97 / 12.9s | False 0.88 / 41.5s | False 0.97 / 3.4s |
| gate-2 (slice aliasing) | false | False 0.98 / 11.3s | False 0.88 / 44.7s | False 0.95 / 3.4s |
| gate-3 (neg-max unpinned; tests DO pin) | false | False 0.97 / 9.1s | False 0.97 / 20.8s | False 0.99 / 2.3s |
| gate-4 (dirty-tree "should pass") | false | False 0.90 / 12.7s | False 0.88 / 37.4s | False 0.85 / 3.1s |
| gate-5 (RealExec hardening unpinned) | **true** | **True 0.76 / 27.1s** | **True 0.72 / 121.7s** | **True 0.72 / 13.7s** |
| gate-6 (wasted Status call) | false | False 0.95 / 7.9s | False 0.92 / 53.6s | False 0.90 / 3.0s |

Observations: perfect precision/recall from all three. The one real finding drew the lowest confidence and
longest latency from every model — good calibration. Latency: flash fastest, Opus mid, GLM-5.3 much
slower (full reasoning model), and slowest on the hardest (real) item.

### internal/githubcontext (2 candidates, both real HIGH/MED security bugs)

All three configs confirmed both. GLM was slightly *more* confident on the PEM leak than Opus.

| id | truth | Opus | GLM-5.3 | GLM-5.3-flash |
| --- | --- | --- | --- | --- |
| ghctx-1 (PEM key body leak) | true | True 0.82 / 41.4s | True 0.93 / 78.8s | True 0.93 / 7.5s |
| ghctx-2 (value-prefix leak) | true | True 0.82 / 24.6s | True 0.75 / 116.2s | True 0.85 / 4.6s |

### internal/reviewers (3 candidates, all real)

All three configs confirmed all three.

| id | truth | Opus | GLM-5.3 | GLM-5.3-flash |
| --- | --- | --- | --- | --- |
| reviewers-1 (epic gate false-pass, HIGH) | true | True 0.83 / 21.2s | True 0.82 / 81.1s | True 0.85 / 5.0s |
| reviewers-2 (service-path false-block, MED) | true | True 0.90 / 21.3s | True 0.82 / 70.5s | True 0.78 / 7.7s |
| reviewers-3 (eval false-block, LOW) | true | True 0.72 / 25.1s | True 0.80 / 86.1s | True 0.70 / 6.3s |

### internal/prready (3 candidates: 2 real test-gaps, 1 debatable) — **the divergence**

The subtler, larger-context package. This is where the configs split.

| id | truth | Opus | GLM-5.3 | GLM-5.3-flash |
| --- | --- | --- | --- | --- |
| prready-1 (unpinned ESCALATED branch) | true | True 0.72 / 24.3s | True 0.65 / 151.2s | **False 0.60 / 5.2s** |
| prready-3 (rune-split truncation) | true | True 0.55 / 82.4s | **timeout >240s** | **False 0.80 / 4.0s** |
| prready-2 (unreachable PASS) | debatable | True 0.72 / 27.1s | **timeout >240s** | False 0.65 / 8.1s |

- **flash missed both real findings** (prready-1, prready-3) — confident `False` verdicts, not timeouts.
  Its recall degrades on subtle "invariant nothing holds" / edge-case-reasoning findings, even though it
  nailed every crisp bug and the equally-category-2 gate-5.
- **GLM-5.3 timed out (>240 s)** on 2 of 3 — the full reasoning model is impractically slow; it was correct
  where it answered but unusable as a routine judge.
- **Opus caught all three** at appropriately low confidence (0.55–0.72), reflecting their subtlety.

### internal/fsm/export (2 candidates: 1 real dead-assertion, 1 debatable)

| id | truth | Opus | GLM-5.3 | GLM-5.3-flash |
| --- | --- | --- | --- | --- |
| export-2 (dead/tautological assertion) | true | True 0.82 / 51.1s | **timeout >240s** | True 0.72 / 14.7s |
| export-1 (audit-log fail-open) | debatable | True 0.58 / 94.9s | **timeout >240s** | True 0.72 / 15.9s |

flash caught the subtle dead-assertion (export-2); Opus did too (low confidence on the debatable
export-1). **GLM-5.3 timed out on both** (48 KB context) — the same failure it showed on prready.

## Aggregate (16 candidates: 9 real, 5 precision probes, 2 debatable)

| config | precision (probes rejected) | recall (real confirmed) | median latency | timeouts |
| --- | ---: | ---: | ---: | ---: |
| Opus 4.8 (claude -p) | 5/5 | **9/9** | ~25s | 0 |
| GLM-5.3 | 5/5 | 7/9 (2 timed out) | ~80s | 4 (all on ≥48 KB contexts) |
| GLM-5.3-flash | 5/5 | 7/9 (missed prready-1, prready-3) | ~5s | 0 |

Note flash *did* catch export-2 (a subtle dead-assertion) — its two misses are both prready findings,
suggesting the miss is finding-specific (unpinned-branch / rune-edge reasoning) rather than purely
context-size. GLM-5.3 (full) timed out on 4 of the ≥48 KB-context candidates — unusable as a routine judge.

**On crisp bugs (redaction leaks, epicready loose matching) all three agree perfectly.** They diverge only
on subtle test-gap / edge-case findings:
- **Opus 4.8** is the only config with full recall — and it self-calibrates (low confidence on the subtle
  ones). No timeouts.
- **GLM-5.3-flash** never false-positives and is ~5× faster than Opus, but **under-calls subtle findings**
  (2 real misses). A confident-but-wrong `False` is its failure mode.
- **GLM-5.3 (full)** is the worst practical choice: it timed out on 2 findings (>240 s) and showed no
  accuracy edge over flash where it answered.

## Recommendation

- **Two-tier judge for the coverage / mutation-survivor campaign.** Use **GLM-5.3-flash as the cheap
  first-pass judge** — fast, cheap, zero false positives, so anything it marks REAL is trustworthy — but
  **escalate to Opus 4.8** whenever flash returns `False` on a candidate that matters, or on any "invariant
  nothing holds" / subtle-edge finding, because flash's misses are real and silent. Do **not** route the
  campaign through GLM-5.3 (full): it is both the slowest and times out.
- **Confidence is a usable routing signal:** the real-but-subtle findings clustered at 0.55–0.72 across
  models; treat sub-~0.75 confidence as "send to Opus."
- **metareview enhancements:** (1) add a `claude`/OAuth judge transport (mirroring `codex/`) so the Opus
  control runs through `fsm judge` itself; (2) revisit the `glm-5.3` judge timeout so one slow call can't
  block a routine run ~4 min.

## metareview recall gaps the external bots caught (the highest-value calibration input)

Deliverable #4 — bot findings on the fix PRs that metareview's own `task-done` gate did **not** surface.
Each is a labeled example of a metareview blind spot; fold into calibration via `learn --post-merge`.

| # | Source | PR | Gap | metareview lesson |
| --- | --- | --- | --- | --- |
| R1 | CodeRabbit | #67 | The **committed context pack embeds the absolute host `cwd`** (`/Users/<user>/…`), leaking username + FS layout — metareview generates this artifact and its own review never flagged it. Fixed by recording `cwd` as `.`. | metareview's context-pack / evidence generator should mask the host cwd the way `internal/fsm/export`'s `rootPath()` masks outside-repo paths. A whole class of already-committed context packs carry this. |
| R2 | Cursor Bugbot | #68 | The `servicePathPattern` fix over-narrowed and **stopped matching dotted basenames** (`user.service.ts`, the Angular/NestJS convention) — a regression `task-done` passed. Fixed by adding `.` to the boundary class. | metareview's review missed a real behavior regression in a regex it was asked to review; the security/architecture lens should probe convention coverage (dotted vs underscore basenames), not just the reported false positive. |
| R3 | CodeRabbit | #67 | Redaction tests asserted only **absence of selected substrings**, so a regression leaking a different slice would pass. Strengthened to exact-match. | the testing-quality lens should flag negative-substring assertions on redaction/security output as weak (prefer exact-match), a near-miss of the "assertion that cannot fail" lens. |

Notably, **Bugbot and CodeRabbit found nothing in the fix *logic* itself** (the redaction and matching
fixes were correct) — the gaps were an artifact leak, a convention-coverage regression, and assertion
strength. That is a strong recall result for the core review, with three precise lens-improvement targets.

## metareview recall/precision self-observations (from running the gate on itself)

- **Its deterministic eval lint (`\beval\s*\(`) fires on test/doc code** that legitimately contains an
  `eval(` literal (a test asserting eval-detection, a comment). The repo works around it by splitting the
  literal; candidate improvement: exempt `_test.go` fixtures / comment lines, or key on added non-comment
  lines only.
- **Stale findings from a failed run re-surface** from `.metareview/findings.jsonl` unless the re-run
  chains `--previous-run <opening-run-id>` — easy to trip; worth a louder hint in NEEDS_REVISION output.

## Fold-back

All four fix PRs (#67–#70) are merged. `learn --post-merge` has been run on #67 and #68 (the PRs that drew
bot findings), ingesting R1–R3 as durable calibration — the accepted/discarded artifacts are under
`docs/metareview/learning/` and the curated knowledge in `.metareview/knowledge/metareview.jsonl`. This
closes the metareview-first-then-bots loop: metareview's gate ran first, the bots measured the residual,
and the residual is now folded back.

---

# Round 2: 5 more packages + an effort-ladder (2026-09-02)

To answer "do we have enough info?", a second pass over **5 more packages** — `internal/fsm/machine`,
`internal/learning`, `internal/mutation`, `internal/testconv`, `internal/status` — with a new
**effort-escalation ladder** and the new `METAREVIEW_JUDGE_TIMEOUT` (set to 600s so glm-5.3 is never cut
off). Ladder per candidate: **flash `low`** first → if `False`/`conf<0.75`, re-ask **flash `high`** →
**glm-5.3 `low`** as a comparison point → **Opus 4.8** as backstop.

## Candidates found (8; mutation was a clean zero)

| package | findings |
| --- | --- |
| fsm/machine (100%-cov, hardened) | machine-1 (MED): `incompleteFork`'s agent-edit disjunct is a live corruption guard deletable with tests green |
| learning (floored) | learning-1/2 (MED, mutation-verified test-gaps: `containsBlockerLanguage` / calibration prose fallback never called), learning-3 (debatable) |
| mutation (floored) | **0** — honestly clean; 6 candidates pursued and rejected against the tests |
| testconv (floored) | **testconv-1 (MED, real logic bug):** `testFileSuffixes` omits `.mjs`/`.cjs`, so `foo.test.mjs` (a real Vitest test) is misclassified as non-test; testconv-2 (dead `---` guard), testconv-3 (debatable) |
| status (floored) | status-1 (LOW, mutation-verified test-gap): `Subtype=="error"` disjunct unpinned |

## Effort-ladder adjudication (6 REAL + 2 debatable)

| id | truth | flash·low | flash·high | glm-5.3·low | Opus | ladder |
| --- | --- | --- | --- | --- | --- | --- |
| learning-1 | real | True 0.80 / 2s | — | True 0.85 / 3s | True 0.76 / 20s | flash-low ✓ |
| learning-2 | real | True 0.70 / 2s | **False 0.85** | True 0.70 / 6s | True 0.72 / 36s | →Opus ✓ |
| machine-1 | real | **False 0.85** | **False 0.75** | True 0.55 / 12s | True 0.72 / 24s | →Opus ✓ |
| status-1 | real | True 0.70 / 3s | True 0.65 / 10s | **False 0.70** | True 0.88 / 16s | flash-high ✓ |
| testconv-1 | real | **False 0.65** | **False 0.80** | **False 0.80** | True 0.68 / 20s | →Opus ✓ |
| testconv-2 | real | True 0.70 / 4s | True 0.75 / 6s | True 0.85 / 5s | True 0.68 / 19s | flash-high ✓ |
| learning-3 | debatable | True 0.65 | False 0.70 | False 0.70 | False 0.62 | →Opus (False) |
| testconv-3 | debatable | True 0.75 | — | True 0.72 | True 0.68 | flash-low (True) |

**Recall on the 6 real:** Opus **6/6**; flash-low 4/6; glm-5.3-low 4/6. The **two-tier ladder resolved all
6/6** — because Opus is the backstop.

## What round 2 settled

- **Opus 4.8 is the only full-recall judge — now 15/15 real findings across both rounds, with zero false
  positives ever.** It remains the trustworthy backstop.
- **flash-high is NOT a reliable escalation.** It flipped `learning-2` from a correct True to a confident
  False — *worse* than flash-low. The earlier single-datapoint hint (flash-high recovered prready-1) does
  not generalise; drop the flash-high rung.
- **flash-low and glm-5.3-low miss *different* subtle findings** (flash missed machine-1; glm-5.3 missed
  status-1; both missed testconv-1). Neither cheap model alone is a safe judge for subtle findings, but
  **both have zero false positives**, so a confident cheap `True` is trustworthy.
- **The right design is two-tier, not three:** flash-low first pass (accept its confident REALs), escalate
  everything else **straight to Opus**. glm-5.3-full adds latency without a recall edge.
- **`METAREVIEW_JUDGE_TIMEOUT`** (this round's enhancement) removed the glm-5.3 timeouts, and `effort=low`
  kept glm-5.3 fast (3–26s) on 62–258 KB contexts.

## Recall gap R4 (this round)

**CodeRabbit on #72** caught an integer **overflow** in the new `ResolveTimeout` (`n * time.Second` wraps
for a huge `n`), which metareview's own `task-done` and single-worker gremlins both missed — gremlins
mutates operators, not input *values*, and the initial tests had no out-of-range case. Fixed with a
`maxTimeoutSeconds` bound. Lesson: the mechanical-precision / testing-quality lenses should probe
integer-overflow on any parsed-number → duration/size multiplication.

## Status of the round-2 findings

The one clear logic bug (**testconv-1**) is worth a fix; the rest are subtle test-pinning gaps — exactly
the **100%-coverage campaign's** work — and are captured here (and in the raw data under
`self-review-matrix-data/round2/`) as its backlog rather than fixed piecemeal.

---

# Round 3: the full sweep — every remaining package (2026-09-02)

At the maintainer's direction ("do all the rest of the remaining packages"), rounds 1–2's 10 packages were
extended to **all 51 packages** in the repo. 29 more packages were reviewed by Opus-4.8 analyzer subagents
(the substantial ones) plus a direct self-review of the 10 trivial utility packages (<120 LOC); findings
were adjudicated with the same flash-low → flash-high → glm-5.3-low → Opus ladder (`METAREVIEW_JUDGE_TIMEOUT`
raised so glm-5.3 never times out).

## Coverage & findings

- **51/51 packages reviewed.** **69 findings** total across the three rounds (2 HIGH — both round-1, fixed;
  ~21 medium; ~43 low). 5 packages came back an honest **zero** (mutation, contextprofile, converge,
  tasksource, fsm/record) and 10 trivial utility packages were clean.
- **~64 of the 69 are "invariant nothing holds" test-pinning gaps** — a guard/assertion/error-code that can
  be deleted or inverted with the whole suite still green. This is the **100%-coverage / mutation campaign's
  backlog, systematically enumerated** — see `docs/metareview/COVERAGE-CAMPAIGN-BACKLOG.md`.
- **The real logic bugs** (a small minority) surfaced by the sweep, beyond round-1/2's fixes:
  - `gitcontext` — **(fixed, #76)** `matchesExclude` treats a bare-directory exclude as an *exact* match, but git's
    `:(exclude)<dir>` is recursive, so a bare exclude **plus any exception** silently un-excludes a whole
    directory into the reviewed diff (verified against real git). **(fix pending)**
  - `internal/covergate` — a crafted 0-statement profile line yields `Pct()=NaN`, and `NaN < floor` is false,
    so the package silently passes its floor. Lower priority (needs a hand-crafted profile).
  - `internal/repo` — boundary-free negative-marker substring matching (`"no metaswarm-legacy"` matches
    `"no metaswarm"`); minor.

  Two findings first flagged here as bugs are on re-reading **high-value test-gaps, not live defects** (the
  current code is correct; the guard is merely unpinned): `internal/evidence`'s `exitCode=1` default that
  catches a failed-to-launch command is deletable with tests green (delete it and a failed command *would*
  mint a passing receipt — so it is an unheld invariant, not a present bug), and `internal/findings`'
  `classForCount` medium/low-blocking→warning downgrade is the sister-package-`reviewlog` policy left
  unpinned. Both are in the backlog.

## The model matrix, now well-sampled (29 REAL adjudicated findings across rounds 2–3)

| judge config | recall on the 29 REAL findings | false positives |
| --- | ---: | ---: |
| **Opus 4.8** | **26/29 (90%)** | 0 |
| flash-low | 16/29 (55%) | 0 |
| glm-5.3-low | 16/29 (55%) | 0 |
| flash-high (of the escalated) | 9/21 | 0 |
| **2-tier ladder: flash-low confident-True → Opus** | **27/29 (93%)** | 0 |

## What the full sweep settles

1. **Opus 4.8 is the strongest single adjudicator (90%, zero false positives)** but is **not perfect on
   test-gap findings** — it misses a few where deletability cannot be confirmed from source.
2. **The 2-tier ladder marginally beats Opus alone (93%)** and is the recommended design: run flash-low as
   the cheap first pass, accept its *confident* `True` (flash-low never false-positived across all 69
   findings), and escalate everything else **straight to Opus**. **Drop the flash-high rung** — across the
   full sample it added no recall and once flipped a correct verdict.
3. **glm-5.3 offers no recall edge over flash and is far slower**; with `effort=low` it is fast but no more
   accurate. Not worth it as a routine judge.
4. **Systematic limitation — adjudicate the crisp, mutation-verify the subtle.** Every model's recall is
   high on crisp bugs (round 1: all three ~perfect) and drops on "invariant nothing holds" test-gaps,
   because an adjudicator *cannot run the tests* to confirm a guard is deletable. For that finding class the
   **analyzer's mutation run is the reliable oracle, not the adjudicator** — which is exactly how these 64
   test-gaps were verified (each analyzer empirically deleted/inverted the guard and re-ran the suite).

## Recommendation (final)

- **Judge:** two-tier — **GLM-5.3-flash (effort low) first pass, escalate to Opus 4.8** on any non-confident
  or `False` verdict. No flash-high, no glm-5.3-full.
- **For test-gap ("invariant nothing holds") findings, don't rely on the adjudicator at all** — have the
  analyzer prove deletability by mutation, which is what closes them in the coverage campaign.
- Raw round-3 analyzer sets and ladder rows: `docs/metareview/self-review-matrix-data/round3/`.
