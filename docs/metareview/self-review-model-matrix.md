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
| ghctx-1 | internal/githubcontext | HIGH | `redactMatch` leaks an unencrypted PEM private-key body (splits at base64 `=` padding) | **Fixed — PR #67** |
| ghctx-2 | internal/githubcontext | MED | `redactMatch` leaks a value prefix when the value contains `:` (`token=a:b`) | **Fixed — PR #67** |
| reviewers-1 | internal/reviewers | HIGH | `childEvidencePassed` accepts "broke"/"bypassed" as passing (unanchored `ok`/`pass`) → epic gate false-pass | **Fixed — PR #68** |
| reviewers-2 | internal/reviewers | MED | `servicePathPattern` bare-word alt flags docs/config as service changes → spurious blockers | **Fixed — PR #68** |
| reviewers-3 | internal/reviewers | LOW | `violatesNoEvalIntent` matches `retrieval(` (no `\b`) → false intent-drift blocker | **Fixed — PR #68** |
| gate-5 | internal/fsm/gate | MED | `RealExec` security-hardening flags/env are unpinned by any test (deletable, tests still green) | **Fixed — PR #70** |
| prready-3 | internal/prready | LOW | `readEvidence` truncates at a raw byte boundary → can split a UTF-8 rune | **Fixed — PR #69** |
| export-2 | internal/fsm/export | LOW | dead/tautological assertion in `TestF8Redaction` (can never fail) | open (low, documented) |
| prready-1 | internal/prready | MED | `verdictForCounts` ESCALATED branch (`>=`) unpinned across the repo | **Fixed — PR #69** |

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

### internal/fsm/export — _pending (export-1 debatable, export-2 dead assertion)_

## Aggregate (14 candidates: 8 real, 5 precision probes, 1 debatable)

| config | precision (probes rejected) | recall (real confirmed) | median latency | timeouts |
| --- | ---: | ---: | ---: | ---: |
| Opus 4.8 (claude -p) | 5/5 | **8/8** | ~24s | 0 |
| GLM-5.3 | 5/5 | 6/8 (2 timed out) | ~78s | 2 |
| GLM-5.3-flash | 5/5 | 6/8 (missed prready-1, prready-3) | ~5s | 0 |

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

## metareview recall/precision self-observations (calibration inputs)

- **Its deterministic eval lint (`\beval\s*\(`) fires on test/doc code** that legitimately contains an
  `eval(` literal (a test asserting eval-detection, a comment). The repo works around it by splitting the
  literal; candidate improvement: exempt `_test.go` fixtures / comment lines, or key on added non-comment
  lines only.
- **Stale findings from a failed run re-surface** from `.metareview/findings.jsonl` unless the re-run
  chains `--previous-run <opening-run-id>` — easy to trip; worth a louder hint in NEEDS_REVISION output.
