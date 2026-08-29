# metareview task-done context

Run ID: `mrv-20260827-083607463083000-task-done-m1-m6-fsm-packages-a99c72f1`

## Task

# M1–M6: internal/fsm core packages

Implement `internal/fsm/{errs,converge,gate,workflow,machine,cmdexec,judge,mockai,kind}` per
`docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md` (r4) and `docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md`
(r5), test-first, under the combined coverage gate (`tests/coverage.sh`), reviewed per commit range (≤ 120 KB each).

## Acceptance

- Every §7/§8 test row has a discriminating test (literal pins; goldens regression-only behind an env flag).
- `go test ./internal/fsm/...` passes; every `internal/fsm/*` package at exactly 100% statements.
- `bash tests/coverage.sh` passes (legacy floor held).
- Dependency direction per spec 2 §1 (machine imports no kinds/judge/cmdexec/workflows).
- Every LLM/shell effect behind an interface; no shell, pinned argv, exact env in `cmdexec`.


## Git

- Base: `afadc0c4b9482667406bcc82f58a76321f3ed6d8`
- Head: `37449aaa6c8469d63428dd1d5b51f26780b33722`
- Branch: ``
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `140630`
- Filtered diff bytes: `120056`
- Risk level: `context-risk`
- Risk reasons: `LARGE_DIFF`
- Generated files excluded: docs/metareview/context/mrv-20260827-073257644607000-artifact-2026-08-27-metareview-0-9-0-fsm-core-a0b8592f-context.md, docs/metareview/context/mrv-20260827-073257743851000-artifact-2026-08-27-metareview-0-9-0-fsm-judge-kinds-33d63bfb-context.md, docs/metareview/reviews/mrv-20260827-073257644607000-artifact-2026-08-27-metareview-0-9-0-fsm-core-a0b8592f.md, docs/metareview/reviews/mrv-20260827-073257743851000-artifact-2026-08-27-metareview-0-9-0-fsm-judge-kinds-33d63bfb.md

## Context Shard Plan

- Source diff hash: `34befc71d6b80e8d`
- shard-01: .gitattributes, docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md, internal/fsm/cmdexec/cmdexec.go, internal/fsm/cmdexec/cmdexec_test.go, internal/fsm/workflow/resolve.go (59961 bytes, prompt pack `docs/metareview/shards/34befc71d6b80e8d-shard-01.md`)
- shard-02: docs/0.8.0-candidates.md, docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md, docs/tasks/m1-m6-fsm-packages.md, go.sum, internal/fsm/cmdexec/sha_test.go, internal/fsm/converge/converge.go, internal/fsm/gate/git.go, internal/fsm/judge/testdata/prompts/adjudicate.python.txt, internal/fsm/judge/testdata/prompts/match.python.txt, internal/fsm/judge/testdata/prompts/still-present.python.txt, internal/fsm/machine/harness_test.go, internal/fsm/machine/machine.go, internal/fsm/machine/machine_test.go, internal/fsm/machine/sidecar.go, internal/fsm/machine/types.go, internal/fsm/run/types.go, internal/fsm/workflow/testdata/cmds-preimage.sha256, internal/fsm/workflow/workflow.go, internal/fsm/workflow/workflow_test.go (59916 bytes, prompt pack `docs/metareview/shards/34befc71d6b80e8d-shard-02.md`)
- shard-03: go.mod (228 bytes, prompt pack `docs/metareview/shards/34befc71d6b80e8d-shard-03.md`)

## Review Manifest

- Manifest verdict: `NEEDS_REVISION`
- Source manifest hash: `886955ec2c85c55b`
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- .gitattributes
- docs/0.8.0-candidates.md
- docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md
- docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md
- docs/tasks/m1-m6-fsm-packages.md
- go.mod
- go.sum
- internal/fsm/cmdexec/cmdexec.go
- internal/fsm/cmdexec/cmdexec_test.go
- internal/fsm/cmdexec/sha_test.go
- internal/fsm/converge/converge.go
- internal/fsm/gate/git.go
- internal/fsm/judge/testdata/prompts/adjudicate.python.txt
- internal/fsm/judge/testdata/prompts/match.python.txt
- internal/fsm/judge/testdata/prompts/still-present.python.txt
- internal/fsm/machine/harness_test.go
- internal/fsm/machine/machine.go
- internal/fsm/machine/machine_test.go
- internal/fsm/machine/sidecar.go
- internal/fsm/machine/types.go
- internal/fsm/run/types.go
- internal/fsm/workflow/resolve.go
- internal/fsm/workflow/testdata/cmds-preimage.sha256
- internal/fsm/workflow/workflow.go
- internal/fsm/workflow/workflow_test.go

### Path Dispositions
- docs/metareview/context/mrv-20260827-073257644607000-artifact-2026-08-27-metareview-0-9-0-fsm-core-a0b8592f-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/context/mrv-20260827-073257743851000-artifact-2026-08-27-metareview-0-9-0-fsm-judge-kinds-33d63bfb-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260827-073257644607000-artifact-2026-08-27-metareview-0-9-0-fsm-core-a0b8592f.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260827-073257743851000-artifact-2026-08-27-metareview-0-9-0-fsm-judge-kinds-33d63bfb.md: generated (metareview generated review artifact excluded from source manifest)

### Shards
- shard-01: .gitattributes, docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md, internal/fsm/cmdexec/cmdexec.go, internal/fsm/cmdexec/cmdexec_test.go, internal/fsm/workflow/resolve.go
- shard-02: docs/0.8.0-candidates.md, docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md, docs/tasks/m1-m6-fsm-packages.md, go.sum, internal/fsm/cmdexec/sha_test.go, internal/fsm/converge/converge.go, internal/fsm/gate/git.go, internal/fsm/judge/testdata/prompts/adjudicate.python.txt, internal/fsm/judge/testdata/prompts/match.python.txt, internal/fsm/judge/testdata/prompts/still-present.python.txt, internal/fsm/machine/harness_test.go, internal/fsm/machine/machine.go, internal/fsm/machine/machine_test.go, internal/fsm/machine/sidecar.go, internal/fsm/machine/types.go, internal/fsm/run/types.go, internal/fsm/workflow/testdata/cmds-preimage.sha256, internal/fsm/workflow/workflow.go, internal/fsm/workflow/workflow_test.go
- shard-03: go.mod

### Manifest Blockers
- missing cross-shard result
- missing shard result for shard-01
- missing shard result for shard-02
- missing shard result for shard-03

## Changed Files

- .gitattributes
- docs/0.8.0-candidates.md
- docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md
- docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md
- go.mod
- go.sum
- internal/fsm/cmdexec/cmdexec.go
- internal/fsm/cmdexec/cmdexec_test.go
- internal/fsm/cmdexec/sha_test.go
- internal/fsm/converge/converge.go
- internal/fsm/gate/git.go
- internal/fsm/judge/testdata/prompts/adjudicate.python.txt
- internal/fsm/judge/testdata/prompts/match.python.txt
- internal/fsm/judge/testdata/prompts/still-present.python.txt
- internal/fsm/machine/harness_test.go
- internal/fsm/machine/machine.go
- internal/fsm/machine/machine_test.go
- internal/fsm/machine/sidecar.go
- internal/fsm/machine/types.go
- internal/fsm/run/types.go
- internal/fsm/workflow/resolve.go
- internal/fsm/workflow/testdata/cmds-preimage.sha256
- internal/fsm/workflow/workflow.go
- internal/fsm/workflow/workflow_test.go
- docs/tasks/m1-m6-fsm-packages.md

## Diff

````diff
diff --git a/.gitattributes b/.gitattributes
new file mode 100644
index 0000000..9897ec3
--- /dev/null
+++ b/.gitattributes
@@ -0,0 +1 @@
+internal/fsm/judge/testdata/prompts/* -text
diff --git a/docs/0.8.0-candidates.md b/docs/0.8.0-candidates.md
new file mode 100644
index 0000000..feff5d0
--- /dev/null
+++ b/docs/0.8.0-candidates.md
@@ -0,0 +1,210 @@
+# metareview 0.8.0 candidate improvements (empirically-driven)
+
+> **Source of evidence:** the harnesseval lab (`/Users/dsifry/Developer/harnesseval`), batch
+> `20260824-101905-cli-144cells`, 433 Phase-B runs across {vanilla-engineered, metareview-realistic,
+> superpowers-realistic, compound-realistic} × {opus-5, opus-4-5, gpt-5.6-sol, glm-5.2-vision-flex}
+> × {medium, xhigh}. Per-finding adjudication records + per-golden matches now persisted, so the
+> findings below are per-finding, not just aggregate. Re-derive anytime with
+> `python -m harnesseval.analysis` (writes `per_lens_attribution.json`, `adjudication_split.json`,
+> `per_golden_miss.json` to `results/`).
+>
+> **Status key:** 🔬 hypothesis · 🧪 build proposed · ✅ done · ❌ refuted.
+>
+> **Caveat:** N=6 PRs/cell; bootstrap CIs wide. These are strong *mechanistic* signals (per-finding,
+> per-lens) but the PR sample is small and skewed (cal.com, discourse, sentry — no keycloak/grafana
+> in this batch). Confirm on Phase C's 50 PRs before publishing. See harnesseval
+> `docs/METAREVIEW_IMPROVEMENTS.md` for the full hypothesis register.
+
+---
+
+## C1 — Deterministic reviewers emit 100% hallucination under adjudication (route to a Gate Findings section, not the finding stream)
+
+**Status:** 🧪 BUILT (untested/unverified) — see Verification gate below. **DO NOT TEST until the
+harnesseval batch `20260824-101905-cli-144cells` error-fill-in run finishes.**
+
+### Fairness check (cross-framework, done before building)
+Is this a metareview-specific issue or a cross-framework eval-unfairness? Checked
+`per_lens_attribution.json` across all 4 frameworks: **metareview-specific.** compound has only
+real reviewer passes (`compound-realistic/p0`–`p3`, `compound-persona/*`) — no meta/gate layer.
+superpowers has only severity-banded reviewer output (`important`/`minor`/`critical`) — no meta
+layer. vanilla is a single prompt. Only metareview has a deterministic-gate layer
+(`metareview-deterministic/*`, real_rate 0%) AND an orchestrator-narrative layer
+(`metareview-session`, real_rate 26%). So the eval was *unfair to metareview* — its
+precision/hallucination were polluted by noise the others don't carry. No compound/superpowers
+adapter change needed; the fix is metareview-side. (Eval-side guard: the harnesseval adapter
+could also filter `source_lens` starting `metareview-deterministic/` — a small future tweak.)
+
+### Evidence (per-finding, from `per_lens_attribution.json`)
+Every `metareview-deterministic/*` reviewer's output is adjudicated **100% hallucination**
+(real_rate = 0%, halluc_rate = 100%) across the batch:
+
+| deterministic reviewer | n_findings | matched | real_but_ungold | hallucination | real_rate |
+|---|---:|---:|---:|---:|---:|
+| test-reviewer | 25 | 0 | 0 | 25 | 0% |
+| code-quality-reviewer | 18 | 0 | 0 | 18 | 0% |
+| security-reviewer | 11 | 0 | 0 | 11 | 0% |
+| architecture-reviewer | 10 | 0 | 0 | 10 | 0% |
+| manifest | 10 | 0 | 0 | 10 | 0% |
+| lens-mixed | 7 | 0 | 0 | 7 | 0% |
+| validation-reviewer | 5 | 0 | 0 | 5 | 0% |
+| runtime | 4 | 0 | 0 | 3* | 25% |
+| pr-readiness-reviewer | 3 | 0 | 0 | 3 | 0% |
+| external-reviewer | 3 | 0 | 0 | 3 | 0% |
+| evidence | 2 | 0 | 0 | 2 | 0% |
+| harness | 2 | 0 | 0 | 2 | 0% |
+| verdict | 2 | 0 | 0 | 2 | 0% |
+| gates | 1 | 0 | 0 | 1 | 0% |
+| context-profile | 1 | 0 | 0 | 1 | 0% |
+
+This is the per-finding version of the H2 "deterministic_gate_recall = 0.00" finding: the gates
+fire (n_det_findings > 0) and produce findings, but **none** match a golden issue or get
+adjudicated as real-but-ungold. Across the whole batch, deterministic reviewers contributed 0
+true positives and 0 real-but-ungold finds.
+
+### Mechanism (traced to metareview's code)
+`internal/reviewers/taskdone.go` `RunTaskDone` emits 6 fixed findings
+(`internal/reviewers/taskdone.go:64-140`) — context-risk, eval, missing-test, TODO,
+truncated-diff, duplicate-path. Each finding's `Title` and `Finding` fields are **boilerplate
+strings** (e.g. `Title: "Missing test changes or validation evidence"`, `Finding: "Source code
+changed without corresponding test files or validation evidence."`). They describe the *gate
+class* that fired, not the *specific diff location/issue*. When the adjudicator asks "is this a
+real issue in THIS diff?" (harnesseval `adjudicate.py`), the generic text either (a) doesn't
+match a specific golden bug, or (b) gets judged "not a concrete defect" → hallucination.
+`internal/taskdone/review.go:514-528` (`classifiedFindingsMarkdown`) writes these into the review
+markdown under `## Blocking Findings` etc. with fixed `### ID: Title` headers, which harnesseval's
+Martian-EXTRACT_PROMPT then pulls into the findings stream verbatim.
+
+The deterministic gates are doing their *gate* job (block on eval/TODO/missing-test) — but their
+*finding text* is not review-quality prose, so when treated as findings they're 100% noise.
+
+### Proposed improvement (0.8.0 candidate) — DECIDED: route gates to a `## Gate Findings` section
+The fairness check settled option 2 over option 1: **tag deterministic findings as `gate` and
+route them to a separate `## Gate Findings (deterministic gates — NOT review findings; skip in
+finding extraction)` section in the review markdown.** The gates are a blocking signal, not a
+reviewer; treating their output as review findings was a categorization artifact (100%
+hallucination under adjudication). Marking them `gate` (vs `blocking`/`advisory`) makes the role
+explicit and lets downstream extractors skip them.
+
+### Build (done, in this branch — NOT tested/verified)
+- `internal/reviewers/taskdone.go`: set `Classification: "gate"` on all 6 deterministic findings
+  (context-risk, eval, missing-test, TODO, truncated-diff, duplicate-path); fixed the `finding()`
+  helper which was unconditionally overriding Classification to "blocking" (now only defaults if
+  empty).
+- `internal/findings/findings.go`: added `gate` to `canonicalClass` + `classForCount`; added `Gate`
+  to `ClassCounts` + `CountByClass` so gate findings don't pollute the Warnings count.
+- `internal/taskdone/review.go`: added the `## Gate Findings (...)` section to
+  `classifiedFindingsMarkdown`; routed gate records there in `classForDisplay`.
+- `go build` + `go test ./internal/...` pass. End-to-end smoke (throwaway repo): eval/missing-
+  test/TODO findings now render under `## Gate Findings` with `Classification: gate`; `## Blocking
+  Findings` is empty of gate noise. **This is a smoke check only — NOT the verification.**
+
+### Verification gate (NOT YET DONE — do not start until the current run finishes)
+The current harnesseval batch `20260824-101905-cli-144cells` is still finishing its error-fill-in
+runs. **Do not trigger any metareview-realistic re-run to verify C1 until that batch is done**
+(re-running mid-batch risks registry confusion + lost in-flight work). Once the batch is done:
+1. Re-run metareview-realistic on a small clean subset (e.g. the 6 hard PRs × opus medium) with
+   the new `bin/metareview`.
+2. Check `per_lens_attribution.json`: `metareview-deterministic/*` rows should either (a) vanish
+   from the finding stream entirely (if the adapter skips `## Gate Findings`), or (b) still
+   appear but no longer count toward precision/hallucination (if the adapter reads them but tags
+   by section). **The intended outcome is (a): the harnesseval adapter should skip the `## Gate
+   Findings` section** — that's the matching eval-side change (see below).
+3. Confirm `metareview-realistic` raw precision / `n_hallucination` improve vs the pre-fix batch
+   (the deterministic noise was 100% hallucination; removing it should drop hallucination counts
+   by ~the gate-finding volume).
+4. Confirm lens recall is unaffected (gates never matched gold anyway — `deterministic_tp` was 0).
+
+### Matching eval-side change (harnesseval, for the build agent — do AFTER the run finishes)
+The harnesseval `metareview_realistic.py` extractor should **skip the `## Gate Findings` section**
+when parsing the review markdown into findings (treat it as gate verdicts, not review findings).
+This is the eval-side half of C1; without it, the gates still enter the finding stream and
+still get adjudicated hallucination. Land this alongside the metareview-side change so the two
+agree. (Compound/superpowers need no such change — no gate layer.)
+
+---
+
+## C2 — Orchestrator session prose is 92% hallucination; value is in the lens subagents, not the orchestrator's summary (separate Orchestrator Notes from findings)
+
+**Status:** 🧪 BUILT (untested/unverified) — same verification gate as C1: **DO NOT TEST until the
+`20260824-101905-cli-144cells` error-fill-in run finishes.**
+
+### Fairness check (cross-framework)
+Same as C1: `metareview-session` (the orchestrator narrative) is metareview-specific.
+compound/superpowers don't emit an orchestrator-prose layer into the finding stream. So this is
+metareview-side; no cross-framework eval adjustment needed. (Eval-side guard: the harnesseval
+adapter should skip the `## Orchestrator Notes` section too — see matching eval-side change.)
+
+### Evidence (per-finding, from `per_lens_attribution.json`)
+`metareview-session` findings (the orchestrator's own prose — context notes, "All 6 lenses
+returned", consolidation narrative) are adjudicated **92% hallucination**:
+
+| source | n_findings | matched | real_but_ungold | hallucination | real_rate |
+|---|---:|---:|---:|---:|---:|
+| metareview-session | 83 | 0 | 7 | 76 | 8% |
+
+Zero matches to gold, 7 real-but-ungold, 76 hallucination. The orchestrator's prose — the
+meta-context, the "I filtered file-not-found artifacts" preamble, the "consolidated findings"
+headers — is almost all noise when judged as findings. The 7 real-but-ungold are likely real
+bugs the orchestrator mentioned in passing while summarizing; the 76 are prose-as-finding
+artifacts.
+
+By contrast, the LLM *lenses* (the subagents) are where the value is:
+`metareview-lens/security` real_rate 71%, `metareview-lens/feasibility` 68%,
+`metareview-lens/architecture` 58%, `metareview-lens/completeness` 57%. The lenses catch bugs;
+the orchestrator's prose about the lenses does not.
+
+### Mechanism
+`internal/artifactreview/review.go:126-143` writes the review scaffold, and the orchestrator
+(claude/codex, driven by the review-artifact skill) fills in a narrative summary before/around
+the lens results. harnesseval's Martian-EXTRACT_PROMPT then extracts *every atomic sentence* from
+the orchestrator's prose as a candidate finding — including the meta-context ("the checkout is
+sparse", "I filtered file-not-found artifacts") and the consolidation headers ("All 6 lenses
+returned. Consolidated findings:"). These aren't bugs; they're orchestration narrative, and the
+adjudicator correctly calls them hallucination.
+
+### Build (done, in this branch — NOT tested/verified)
+- `internal/artifactreview/review.go`: added `## Orchestrator Notes (not findings)` section to
+  the artifact-review scaffold template (between Reviewer Results and `## Findings`), with
+  explicit "do not extract as findings" guidance.
+- `rubrics/artifact-review-rubric.md`: added "Output Structure (orchestrator notes vs findings)"
+  section codifying the separation.
+- `skills/review-artifact/SKILL.md` step 6: directs orchestrator to put context/synthesis in
+  `## Orchestrator Notes` only; only `## Findings` is the finding stream.
+- `go build` + tests pass. Scaffold smoke check: `## Orchestrator Notes (not findings)` renders.
+  **Smoke only — NOT the verification.**
+
+### Verification gate (NOT YET DONE — same gate as C1)
+Once the `20260824-101905-cli-144cells` batch is done, re-run metareview-realistic on a small
+clean subset and check `per_lens_attribution.json`: `metareview-session` findings should drop to
+~0 in the finding stream (the orchestrator prose now lives in `## Orchestrator Notes`, which the
+harnesseval adapter should skip — see matching eval-side change). The `metareview-lens/*`
+real_rates should be unaffected (the lenses are the value). Total hallucination should fall by
+~the 76–98 session-hallucinations removed.
+
+### Matching eval-side change (harnesseval, for the build agent — do AFTER the run finishes)
+The harnesseval `metareview_realistic.py` extractor should **skip the `## Orchestrator Notes
+(not findings)` section** when parsing the review markdown. Without this, the prose still enters
+the finding stream. Land alongside the metareview-side change.
+
+---
+
+## Context: what's already done in 0.8.0 (so these are *additional* candidates)
+
+- **Security lens added** (commit `f0a3896`, 0.7.0; 8 lenses in 0.8.0). harnesseval confirms it
+  works: `metareview-lens/security` real_rate 71%, 27 matched gold + 136 real-but-ungold across
+  the batch. The H1 gap is closed for the realistic path. (The harnesseval *api-direct adapter*
+  still lacks it — a harnesseval-side fix, not metareview.)
+- **Architecture lens deepened** (0.8.0, H1b/H1c) with data-model + principal-engineer checks.
+- **Adversarial stance + testing-quality/data-migration lenses** (commit `c19fa3a`, 0.8.0).
+- **Missing-test ownership precedence** (commit `b5c5e57`) — may already address part of C1
+  (the test-reviewer finding); check whether the finding text is now diff-specific.
+
+C1 and C2 are about the *deterministic gates* and the *orchestrator prose*, which the lens work
+above didn't touch. They're the next layer.
+
+## Cross-reference (harnesseval docs)
+- Full hypothesis register: `harnesseval/docs/METAREVIEW_IMPROVEMENTS.md` (H1–H5, H1b/H1c done).
+- Per-finding evidence: `harnesseval/results/per_lens_attribution.json`,
+  `adjudication_split.json`, `per_golden_miss.json`.
+- Analysis code: `harnesseval/harnesseval/analysis.py` (`per_lens_attribution`,
+  `adjudication_split`, `per_golden_miss_analysis`).
diff --git a/docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md b/docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md
index aa33bcc..f42214b 100644
--- a/docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md
+++ b/docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md
@@ -25,7 +25,7 @@
 > algorithm (§5.3b); `tree.Status` capped; `Workflow.Hash` preimage; `initial_terminal`, `unknown_var`, `bad_yaml` reasons
 > in order; `RepoMode` override tighten-only; token counters capped per record (`tokens_too_large`); `GIT_*` scrubbed by
 > prefix and excludes/attributes disabled for `WorkTree`; `Snapshot.Clone` keeps `TimeoutMS/Env`; `CmdsSHA256` uses
-> `run.MarshalCanonical`; `Sidecar.Read` capped + `O_NOFOLLOW` + `ValidateRunID`; `Untrusted` includes `error.detail`;
+> `run.MarshalCanonical`; `Sidecar.Read` capped + `O_NOFOLLOW` + `ValidateRunID`; every error Detail is untrusted (spec 5 marks `error.detail`);
 > ctx cancellation is returned, never a pseudo-gate; sidecar obligation for forks restated (child's own bytes).
 >
 > **Open for Dave — D1 (`AllFixed` needs a non-empty `AllFound`):**
@@ -48,7 +48,8 @@
 ```
 internal/fsm/errs       Error{Code, Detail, Fields}; E, Wrap, Is, Code, As                       (implemented)
 internal/fsm/converge   AllFixed; Result; Predicate; Validate/Parse; atoms; Payload; CmdResult/Runner  (implemented)
-internal/fsm/gate       Git (exec seam + Fake); ValidSHA/ValidRef; 7 gates; TreeHash; Cut          (implemented)
+internal/fsm/gate       Git (exec seam + Fake); ValidSHA/ValidRef; 9 gates; TreeHash; Cut          (implemented)
+workflows/              embedded shipped YAMLs: Names(), Read(name)                                  (implemented, M0)
 internal/fsm/workflow   YAML → Workflow; validation; $VAR resolution; ResolveCmds/VerifyCmds + cmds_sha256
 internal/fsm/machine    Deps; Init/Open/Advance/Record/View; consumed interfaces; Sidecar (FS + Mem)
 ```
@@ -95,9 +96,9 @@ nodes:
   fix:        { kind: agent-edit }
   verify:     { kind: still-present, model: $JUDGE, effort: $JUDGE_EFFORT }
 transitions:                                               # list form (shipped)
-  - { from: discover, to: done, gate: findings_empty, outcome: clean }
+  - { from: discover, to: done, gate: nothing_found, outcome: clean }
   - { from: discover, to: adjudicate, gate: findings_nonempty }
-  - { from: adjudicate, to: done, gate: confirmed_empty, outcome: clean }
+  - { from: adjudicate, to: done, gate: nothing_confirmed, outcome: clean }
   - { from: adjudicate, to: fix, gate: confirmed_nonempty }
   - { from: fix, to: verify, gate: commit_exists }
   - { from: verify, to: done, gate: all_fixed, outcome: fixed }
@@ -113,14 +114,15 @@ accepted and ignored. `nodes.<state>`: `kind` + optional `exec`, `model`, `effor
 (names). Node `cmd:`, `on_overflow:`, and `{cmd: <name>}` atoms reference `cmds` by name. Unknown top-level keys →
 `unknown_key`. `$NAME` grammar: `\$([A-Z_][A-Z0-9_]*)` (longest match; `$JUDGE_EFFORT` is one token); `$$` is a literal
 `$`; any other `$` (`$1`, `${X}`, a trailing `$`) is left literal. Substitution covers `model`, `effort`, top-level string
-params and strings inside list params (nested maps are not walked), and every argv element. Shipped YAMLs: `sdlc-loop` gained `discover→done findings_empty (clean)` and
-`adjudicate→done confirmed_empty (clean)` rows and the r2-form escape-hatch comment (ledger); `review-loop` unchanged.
+params and strings inside list params (nested maps are not walked), and every argv element. Shipped YAMLs: `sdlc-loop` gained `discover→done nothing_found (clean)` and
+`adjudicate→done nothing_confirmed (clean)` rows, omits `exec` on `fix`/`verify` (inferred), and carries the r2-form
+escape-hatch comment (ledger); `review-loop` unchanged.
 
 ### 2.3 Static validation (Parse) — `ERR_WORKFLOW_INVALID{reason, at}`, first failure in this order
 | reason | rule |
 |---|---|
 | `missing_kinds` | `Options.Kinds` nil/empty |
-| `bad_yaml` | the document does not decode (malformed YAML, duplicate mapping keys, non-mapping root); `at = document` |
+| `bad_yaml` | the document does not decode (malformed YAML, duplicate mapping keys, non-mapping root, wrong scalar type such as `version: one`); `at = document` |
 | `unknown_key` | unknown key at the top level, inside a `cmds.<name>` or transition entry, or a reserved node key with a non-string value |
 | `missing_name`, `bad_version` | `workflow` non-empty; `version == 1` |
 | `no_initial`, `bad_state` | `states` non-empty; each `^[a-z][a-z0-9_-]{0,31}$`, unique, `judge` reserved (spec 5's `fsm judge` node) |
@@ -170,7 +172,8 @@ argv element (including the rewritten `argv[0]`) that names an existing regular
 FileHashes, TimeoutMS, Env}` (`Env` nil when none — `omitempty` keeps the preimage stable). **Preimage:**
 `run.MarshalCanonical([]AllowedCmd sorted by name)` (escape-off encoder, the one every fsm struct→JSON path uses) → sha256
 hex; independent of declaration order (W4). `hash(path)` returns an error for missing or non-regular files (that is how
-"names an existing regular file" is decided; `workflow.FileSHA256` is the real one). The runner
+"names an existing regular file" is decided; `workflow.FileSHA256` is the real one and follows symlinks, so a symlinked
+script is pinned by its target's contents under the argv path). The runner
 executes `AllowedCmd.Argv` verbatim. `VerifyCmds(allowed /* = snap.AllowedCmds, never re-resolved */, workDir, hash)`:
 each pinned hash → `ERR_CMD_CHANGED{path, reason: mismatch|missing}`; an argv element that now resolves to a regular file
 without a `FileHashes` entry → `ERR_CMD_CHANGED{path, reason: appeared}`. Ledger: absolute paths make the sha per-machine
@@ -200,6 +203,7 @@ Exit ≥ 2 → `ERR_GIT{op, exit}` with stderr; exit 1 is an answer only for `Is
 | `confirmed_nonempty` / `confirmed_empty` | `len(Confirmed) > 0` / `== 0` | `ERR_NO_CONFIRMED` / `ERR_CONFIRMED_PRESENT` |
 | `commit_exists` | `FixEntryHead == ""` → `ERR_GATE_INAPPLICABLE`; `CommitCount(FixEntryHead, Head) > 0 && clean` | `ERR_NO_COMMIT`, Detail = count + porcelain + `WorkingDiff(MaxDetail)` via `CapDetail`; any Git failure → `ERR_GIT` gate error (evidence in the audit; recovery by fork — ledgered) |
 | `all_fixed` / `bugs_remain` | `converge.AllFixed` / `!` | `ERR_BUGS_REMAIN` / `ERR_ALL_FIXED` |
+| `nothing_found` / `nothing_confirmed` | `len(AllFound) == 0 && len(Findings|Confirmed) == 0` — the iteration-0 clean exits; refuse once any bug is known | `ERR_BUGS_KNOWN` (bugs known) / `ERR_FINDINGS_PRESENT` · `ERR_CONFIRMED_PRESENT` |
 
 ## 4. `converge` (implemented)
 ```go
@@ -230,7 +234,7 @@ returns the first firing child's `Result`; `all` fires only when every child fir
 ```go
 type Instructions struct { Text string; Input map[string]any; Untrusted []string; OutputSchema json.RawMessage }
 type Diff struct { Text string; Truncated bool }                                  // Git.Diff(BaseSHA, head, MaxDiffBytes = 1<<20)
-type ExecInput struct { Snap run.Snapshot; Node *workflow.Node; Diff Diff; StartIndex int /* st.NextIndex(key) */; Audit func(run.Event) error }
+type ExecInput struct { Snap run.Snapshot; Node *workflow.Node; Diff Diff; StartIndex int /* st.NextIndex(key) */; Audit func(run.Event) error; Runner converge.Caller /* the session's guarded runner: same audit closure, same ordinal */ }
 type NodeKind interface {
     Name() string; Info() workflow.KindInfo
     Instructions(snap run.Snapshot, node *workflow.Node, diff Diff, nonce string) (Instructions, error)
@@ -247,7 +251,9 @@ type Sidecar interface {
 }
 // Sidecar names: ^[a-z][a-z0-9._-]{0,63}$, never `audit.*` or `lock`; runID must pass run.ValidateRunID. FS impl under <root>/.metareview/runs/<id>/; Mem for tests.
 ```
-`Audit` appends immediately (durable) and rebinds the machine's state; it returns store errors so the executor stops.
+`Audit` appends immediately (durable) and rebinds the machine's state; it returns store errors so the executor stops,
+and the machine remembers that store error: when `Execute`/`Runner.Run` then fails, the store error is returned unchanged
+(no pseudo-gate, no handler warn) so the next `Advance` resumes.
 Executors number `llm_call.Index` from `StartIndex` (the fold's next index for the key), so an interrupted execution
 resumes with a continuing index and its earlier spend stays audited. `Execute` is never retried by the machine inside
 one `Advance`; a failure → `executor` pseudo-gate (§5.4 step 5), except `ctx.Err() != nil` (interrupt), which `Advance`
@@ -257,7 +263,7 @@ returns unchanged so the next `Advance` resumes from `StartIndex`.
 ```go
 type Deps struct {
     Store run.RunStore; Sidecar Sidecar; Kinds Registry; Git func(workDir string) gate.Git
-    Runner func(allowed []run.AllowedCmd, workDir, runID string, audit func(run.Event) error) converge.Runner
+    Runner func(RunnerDeps) converge.Caller   // RunnerDeps{Allowed, WorkDir, RunID, Audit, CmdCalls func(name) int}; the ONE guarded factory. CmdCalls = number of cmd_call events for that name in the log so far (any context: atom, on_overflow, cmd kind; copied Origin events included), incremented on every successful cmd_call append — the durable mock ordinal
     Clock Clock; LookPath func(string) (string, error); FileHash func(string) (string, error)
     Workflows func(name string) ([]byte, error); ReadFile func(string) ([]byte, error); Nonce func() string
     MockLoad func(dir string) (hash string, err error); Terminal func(ctx, View) error   // spec 3's runs.jsonl record; nil → no-op
@@ -267,18 +273,20 @@ type OpenOptions struct { Repair bool }
 // Typed string sets: Status ∈ {ADVANCED, NEEDS_INPUT, DONE, STOPPED, GATE_FAILED}; NextAction ∈ {advance, record, none}; RecordOptions.Kind ∈ {node-output, tokens, event} — exported constants.
 func Init(ctx, Deps, InitOptions) (*Machine, error); func Open(ctx, Deps, runID string, OpenOptions) (*Machine, error)
 func (m *Machine) Advance(ctx) (AdvanceResult, error); func (m *Machine) Record(ctx, RecordOptions) (RecordResult, error); func (m *Machine) View() View
-type AdvanceResult struct { Status string; From, To run.State; Gate *run.GateData /* first failing */; Outcome run.Outcome; StopReason string; NeedsInput *NeedsInput; Warnings []string /* warn events appended by this call */; Untrusted []string /* "gate.detail","warnings","stop_reason","error.detail" when non-empty — every Detail may carry repo/third-party bytes */; ExitCode int; RunID string }
+type AdvanceResult struct { Status string; From, To run.State; Gate *run.GateData /* first failing */; Outcome run.Outcome; StopReason string; NeedsInput *NeedsInput; Warnings []string /* warn events appended by this call */; Untrusted []string /* "gate.detail","warnings","stop_reason" when non-empty; every returned error's Detail is untrusted too and spec 5 marks it "error.detail" */; ExitCode int; RunID string }
 type NeedsInput struct { Node string; Kind, Exec, Model, Effort string; Instructions Instructions; Record string }
 type RecordOptions struct { Kind string /* node-output|tokens|event */; Node string; Data json.RawMessage; Replace bool; Name string }
-type RecordResult struct { Seq int64; Type run.EventType; Key string }
-type View struct { RunID, Workflow string; Snapshot run.Snapshot; Node *NodeView; NextAction string /* advance|record|none */; Torn bool; FailedGate *run.GateData /* last gate{passed:false} before a failed transition */ }
+type RecordResult struct { Seq int64; Type string; Key string }
+type View struct { RunID, Workflow string; Snapshot run.Snapshot; Node *NodeView; NextAction string /* none when terminal; record when the node is host-executed, its output is absent, and a needs_input for the key exists; else advance */; Torn bool; FailedGate *run.GateData /* last gate{passed:false} in the log when Outcome == failed */ }
+func (m *Machine) RunID() string
 type NodeView struct { Name, Kind, Exec string; HasOutput, Applied bool }
 ```
 
 ### 5.3 `Init`
 1. Load YAML: name → `Deps.Workflows` (`ERR_WORKFLOW_NOT_FOUND`); path (`/` or `.yaml`) → `ReadFile`, ≤ 256 KB
    (`ERR_WORKFLOW_TOO_LARGE`). `Parse(raw, Options{Kinds: Kinds.Info()})`; `Resolve(vars, calibration)`.
-   `RepoMode` override must be `""` or `enforcing` (tighten-only; `advisory` over a workflow's `enforcing` → `ERR_BAD_REPO_MODE`).
+   `RepoMode` override must be `""` or `enforcing` (tighten-only; anything else → `ERR_BAD_REPO_MODE`); `Open` applies the
+   stored `snap.RepoMode` over the sidecar's. `WorkDir` and `RepoRoot` must be absolute (`ERR_WORKDIR_FOREIGN{reason: relative}`).
    Git failures during `Init`/`Advance` outside gates are returned as the `ERR_GIT`/`ERR_GIT_REF` error unchanged (retryable);
    only gates convert Git failures into gate errors.
 2. `ResolveCmds`; `len(Cmds) > 0 && AllowCustomCmds != sha` → `ERR_CMDS_NOT_ALLOWED{sha}`, Detail = the printed list
@@ -287,7 +295,8 @@ type NodeView struct { Name, Kind, Exec string; HasOutput, Applied bool }
    `BaseSHA`; `Status` + `WorkTree` → `TreeHash`.
 4. Goldens: `ReadFile` ≤ 512 KB, JSON array of `run.Golden` with `DisallowUnknownFields`, ≤ `MaxGoldens` → else
    `ERR_GOLDENS_INVALID{path}`.
-5. Mock: `MockDir` (made relative to `RepoRoot`) → `MockLoad` → `Mock = rel + "#" + hash[:16]` (`ERR_MOCK_INVALID`);
+5. Mock: `MockDir` (relative → under `RepoRoot`; must be inside `RepoRoot` else `ERR_MOCK_INVALID{reason: outside}`) →
+   `MockLoad` → `Mock = rel + "#" + hash[:16]` (`ERR_MOCK_INVALID`);
    `Kinds.Mock()` must equal `MockDir != ""` (`ERR_MOCK_MISMATCH`).
 6. `runID` ← `RunID` or `run.RunID(w.Name, Clock().Time)`; **`run.Create` first**, then `Sidecar.Write(runID,
    "workflow.yaml", raw)` (failure → the error; the run exists without a sidecar and `Open` reports `ERR_SIDECAR`); then
@@ -308,8 +317,8 @@ type NodeView struct { Name, Kind, Exec string; HasOutput, Applied bool }
    binary surfaces here as `ERR_WORKFLOW_INVALID` — the "older-binary run" signal; `list`/`export` still work);
    `Resolve(snap.Vars, false)`; `VerifyCmds(snap.AllowedCmds, WorkDir, FileHash)`; `Mock`: split on the **last** `#`, resolve
    `rel` against `snap.RepoRoot`, `MockLoad` hash vs stored, `Kinds.Mock() == (snap.Mock != "")`, else `ERR_MOCK_MISMATCH`;
-   build `runner ← Deps.Runner(snap.AllowedCmds, WorkDir, runID, audit)` and, when `w.Convergence != nil`, `pred ←
-   converge.Parse(w.Convergence, runner)`.
+   count `cmd_call` events per name; build `runner ← Deps.Runner(RunnerDeps{…})` and, when `w.Convergence != nil`, `pred ←
+   converge.MustParse(w.Convergence, runner)` (validated at Parse; cannot fail).
 `Advance` and `Record` re-run steps 1–2 under their own lock (no cached state is trusted across calls).
 
 ### 5.4 `Advance`
@@ -344,9 +353,11 @@ type NodeView struct { Name, Kind, Exec string; HasOutput, Applied bool }
        tt ← w.TerminalFor(state); err ← gate(tt.Gate)(snap); append gate; pass → chosen ← tt else failures += err
        if chosen == nil:
            r, err ← pred.Evaluate(snap)                                        (always when tt failed — bounded loops)
-           err → append gate{Name:"converge", Passed:false, Error:{ERR_CONVERGE_FAILED, Detail}} → step 8
-           r.Class == fixed → same, ERR_CONVERGE_FAILED{reason: fixed_class}   (defense in depth over converge's placement rule)
-           append converge{Atom: r.Atom, Class: r.Class, Stop: r.Stop, Reason}
+           err with the audit closure having failed → return that store error; err with ctx cancelled → return ctx.Err() (resumable);
+           other err → append gate{Name:"converge", Passed:false, Error:{ERR_CONVERGE_FAILED, Detail}} → step 8
+           r.Stop && r.Class == fixed && r.Atom != "all_fixed" → same, ERR_CONVERGE_FAILED{reason: fixed_class}   (defense in depth; the real
+           all_fixed atom firing is legitimate — design §9's example — and a non-firing result's class is irrelevant)
+           append converge{Atom: CapText(r.Atom, MaxShort), Class: r.Class, Stop: r.Stop, Reason: CapText(MaxText)}
            if r.Stop: chosen ← synthetic {From: state, To: tt.To, Gate: r.Atom, Outcome: r.Class}
        if chosen == nil: for t in Outgoing(state) except tt: err ← gate(t.Gate)(snap); append gate; pass → chosen ← t; break; else failures += err
    else: for t in Outgoing(state): err ← gate(t.Gate)(snap); append gate; pass → chosen ← t; break; else failures += err
@@ -355,6 +366,7 @@ type NodeView struct { Name, Kind, Exec string; HasOutput, Applied bool }
    first.Gate, Outcome: failed, Head: head}; Deps.Terminal(ctx, View()); return GATE_FAILED with Gate = first, exit 1
 9  append transition{From, To, Gate, Outcome, Loop, ToKind, Head}
 9b if Outcome == overflow && OnOverflow != "" && !OverflowHandled: res, err ← runner.Run(OnOverflow, converge.Payload(snap));
+   audit failure → return it; ctx cancelled → return ctx.Err() (nothing recorded, retried on resume);
    append overflow_handler{Name, Argv: snap.AllowedCmds[name].Argv, InputHash: sha256(payload), Stdout/Stderr: CapText(MaxDetail/MaxStderr)+flags,
    ExitCode (−1 when err), DurationMS, Error: code}; err or exit≠0 → also warn{OVERFLOW_HANDLER_FAILED}   (at-least-once: a crash between the
    runner's cmd_call and this append re-runs the command)
@@ -370,7 +382,8 @@ node-scoped events (`needs_input`, `node_output`, `delta_applied`, `llm_call`);
 - `node-output`: not terminal (`ERR_RUN_TERMINAL`); state has a node and `Node == node.Name` (`ERR_NODE_MISMATCH`); exec
   `inline|subagent` (`ERR_NODE_NOT_HOST`); `!Applied[k]` (`ERR_NODE_OUTPUT_APPLIED`); `NodeOutputs[k]` absent unless
   `Replace` (`ERR_NODE_OUTPUT_EXISTS`); `Decode` ok (`ERR_NODE_OUTPUT_INVALID`, nothing appended); append `node_output`.
-- `tokens`: `run.TokenTotals`, `DisallowUnknownFields`, no negative field → else `ERR_RECORD_TOKENS`; append `tokens`.
+- `tokens`: `run.TokenTotals`, `DisallowUnknownFields`, no negative field, no field above `MaxTokenCounter` → else `ERR_RECORD_TOKENS`; append `tokens`.
+- any other `Kind` → `ERR_RECORD_NAME{reason: kind}`.
 - `event`: `Name` ~ `^[a-z][a-z0-9_-]{0,63}$`, not a run event type, not `mrv_*` → else `ERR_RECORD_NAME{reason:
   syntax|event_type|reserved}`; `Data ≤ MaxPayload − 128` (`ERR_RECORD_TOO_LARGE`); append `record{name, data}`.
 
@@ -406,15 +419,17 @@ behind `FSM_MACHINE_UPDATE_GOLDEN=1` with the run package's "drift ≠ regenerat
 | pkg | rows |
 |---|---|
 | workflow | W0 order row: a document with an unknown top-level key **and** `version: 2` → `unknown_key` (first failure in table order). W1 both shipped YAMLs + the §2.2 example + a mapping-form twin of review-loop (order preserved, `*→failed` ignored, `->` and `→`): assert `Transitions`, `Nodes` (exec defaulted for `fix`/`verify`), `Cmds` (`Timeout 30s`, default `60s`), `Refs`/`CmdRefs` literals, `Hash` literal, `Warnings` empty; W2 one fixture per reason **and per sub-rule** (each one edit from a valid base): `bad_cmd` ×8 (argv empty / non-string / empty element / timeout non-integer / 0 / 3601 / name regex / > MaxAllowedCmds / > MaxArgv), `bad_env` one per reserved literal and prefix + regex + duplicate + count, `bad_var` ×3, `bad_state` ×3 (incl. `judge`), `failed_reserved` ×3 (undeclared / has node / in a transition), `duplicate_transition` (same `(from, gate)`, different `to`), `loop_terminal` ×2, `unknown_cmd` ×3, `cmd_without_kind` ×2, `unknown_var` ×3 (node, cmd, list param), `bad_outcome` ×2 (`great`, `failed`), `bad_yaml` ×2 (malformed, duplicate key), `bad_version` (`one`), `initial_terminal`, `bad_params`, `missing_kinds`; acceptance boundaries: timeout 1 and 3600, 32-char state, `MaxVars`/`MaxEnv`/`MaxArgv`/`MaxAllowedCmds` at cap; assert `reason` + `at`; W3 `Resolve`: `$JUDGE`/`$JUDGE_EFFORT` prefix pair → `Model=="a"`, `Effort=="b"`; caller value beats `Default`; list params substituted, non-string list elements and nested maps untouched (literal asserts), `$1`/`${X}` left literal; `Refs`/`CmdRefs` from a list param; `$$` literal; `$JUDGEX` → `ERR_VAR_UNKNOWN`; calibration pins asserted as literals; caller `FOO` → `ERR_VAR_UNKNOWN`; required unset; calibration refuses caller `JUDGE` **and** `JUDGE_EFFORT`; calibration on a workflow without `JUDGE` var is a no-op; re-resolve of stored pinned vars succeeds; argv substitution; W4 `ResolveCmds`: fake lookPath/hash; `argv[0]` rewritten absolute (`bash` → `/bin/bash`, `./s.sh` → `<workDir>/s.sh`), relative lookPath result → `ERR_CMD_NOT_FOUND`; closure over `["bash","./s.sh"]` + absolute path; `/bin/bash` itself appears in `FileHashes`; non-nil empty map; **hand-authored preimage** (`testdata/cmds-preimage.json` + `.sha256` from `shasum`) with two cmds declared out of order, `TimeoutMS 1500`, `Env` set; one-byte edit → different sha; `VerifyCmds` mismatch/missing/appeared; a directory argv element is not hashed; **no re-resolution**: after pinning `/bin/bash`, point `lookPath` elsewhere and edit the pinned file → `ERR_CMD_CHANGED{/bin/bash, mismatch}`, and the inverse (pinned intact, lookPath moved) → no error; non-absolute `workDir` refused; W5 `VarsReferencedBy` (node ∪ cmd, sorted, resolved copy), `Outgoing`, `IsTerminal` incl. `failed`, `LoopTransition`, `TerminalFor`, `loop_without_clean_exit` warning |
-| machine | M0 fakes: `gate.FailingFake{Fake; FailAt string}` (exported from `gate`, fails exactly one method) drives per-call-site `ERR_GIT{op}` rows for every Git call in `Init`/`Advance`; a counting store fails `Lock`, `EventsWithLines`, or append #N; seam row: `Fake.Diffs["<base>..<head>"] = "D"` with a small `MaxDiffBytes` → the fake kind/executor observe `diff.Text == "D"`, `Truncated == true`, `nonce == "n1"`. M1 `Init`: hand-written expected sequence `[init(no stamps), tree(State=initial, Iter 0), warn?]` with every `InitData` field asserted literally (embedded + path workflows; `workflow.yaml` sidecar bytes == raw; `Create` before sidecar observed via a fake store/sidecar call log; `ERR_RUN_EXISTS` leaves the victim's sidecar intact); `ERR_WORKFLOW_NOT_FOUND`, `ERR_WORKFLOW_TOO_LARGE`; goldens ok/unknown field/over cap/over bytes; consent list as a hand-written literal (pinned/unpinned marks, env **names** only, no process env values) + sha in Detail, wrong sha, no cmds; `ERR_MOCK_INVALID`; `RepoMode` override `enforcing` accepted / `advisory` over `enforcing` refused; `RevParse` base (`main` → sha); `ERR_WORKDIR_FOREIGN`; `ERR_BAD_REPO_MODE`; mock hash pinned + `Kinds.Mock()` mismatch; unknown `--var`; M2 `Advance` on both shipped workflows with a fake Registry: hand-written expected event-type sequences per path (`review-loop` clean/reviewed; `sdlc-loop` clean at discover, clean at adjudicate, fixed after 1 iteration, loop once then fixed), literal asserts on transition fields, `needs_input` once across `advance, record tokens, advance` and again at `discover@1`, `View.NextAction` per step; goldens regression-only; M3 gate failure: two failing gates → `Gate` is the first in evaluation order and `transition.Gate` names it; loop-boundary variant (tt and the loop gate both fail → tt named); two passing gates → the first taken; `ERR_INSTRUCTIONS_FAILED` (needs_input already present stays, nothing further); ctx cancelled during `Execute` → error returned, no pseudo-gate, next `Advance` resumes with `StartIndex`; `ERR_GATE_INAPPLICABLE`; executor error → `executor` pseudo-gate with earlier `llm_call`s kept and `StartIndex` honoured on the next fork (interrupted-execution fixture: pre-seeded `llm_call` index 0, executor asserts `StartIndex == 1`); decode error / Reduce error / rejected `delta_applied` (status subset from a fake cmd kind) → `node_output` pseudo-gate; M4 loop: cumulative regression (iter 3 fixes its own bug, 7 remain: loop taken, `AllFound == 8`, `Unfixed == 7`, all 8 statuses, not `fixed`); **gate-first**: `max_iterations: 1` with all bugs fixed at verify → `fixed`, zero `converge` events; negative control one bug left → `converge{max_iterations}` → `overflow`; `stalled` via nil-then-plateau and via regression (`Prev 3 → Unfixed 5`); `budget` via `llm_call` tokens and via `record tokens`; `custom` via cmd atom; converge error → `converge` pseudo-gate and no loop taken; `fixed_class` guard via a fake predicate; user workflow whose terminal gate is `confirmed_empty` (not `all_fixed`) with all bugs fixed and findings present → convergence evaluated (bounded) — the `max_iterations` stop fires; emitter caps: a 5 KB cmd-atom reason → `converge.Reason` and `StopReason` capped; overflow handler once, `overflow_handler` fields literal, failure warn, **not** run for `stalled`/`custom`, resumed after a crash (fixture: terminal overflow run without handler → `Advance` runs it, then `ERR_RUN_TERMINAL`); M5 tree: identical porcelain + different `WorkTree` → advisory warn (+ tree) vs enforcing `repo_mode` gate with **no** tree (a second `Advance` re-detects); porcelain ≈ 5 KB → warn Detail capped at `MaxText`, tree Status intact; 70 KB porcelain → `tree.Status` capped with flag; agent-edit exempt; baseline `tree` appended when `TreeHash == ""` (fork-from-initial fixture); `tree` only on change (count); M6 `Record` refusals per code and sub-reason (`syntax`, `event_type` (`transition`), `reserved`), `ERR_RECORD_TOKENS` on unknown field and on `-1`, `Replace`, terminal `tokens` allowed, `ERR_NODE_OUTPUT_INVALID` leaves `Events` byte-identical; M7 `Open`: `ERR_WORKFLOW_CHANGED` via sidecar edit; embedded bytes replaced by a workflow with different transitions while the sidecar is intact → `Advance` follows the **sidecar's** transitions; `ERR_CMD_CHANGED`; `ERR_MOCK_MISMATCH` via scenario edit and via registry mismatch; torn → `ERR_AUDIT_TORN`; `Repair` → warn Detail literal + fold ok; `Repair` at offset 0 → `ERR_RUN_NOT_FOUND`; `ERR_SIDECAR{missing}`; M8 stamps: every event's `At` equals the injected clock sequence, `State/Iter/Mock/Node` per §5.4 tail (`cmd_call` has no Node), non-mock runs never carry `Mock: true` (`MockTainted == false`), mock runs carry it on every non-init event; M9 §5.7 table incl. `StopReason` ("atom: reason"), `Untrusted` list, `Deps.Terminal` called for every terminal outcome incl. `failed`, again on a later `Advance` of a terminal run (idempotency is spec 3's), and on the 9b resume path; `Terminal` error returned with the transition durable; `ERR_AUDIT_FULL` surfaced; a counting store that fails append #N for every N of the happy sequence returns the error unchanged; FS `Sidecar`: symlink refused, exists refused, mode 0600, missing run → `ERR_SIDECAR` |
+| machine | M0 fakes: a per-call-site failing Git wrapper (`failingGit{Git; at}` in `machine/harness_test.go`, fails exactly one method) drives per-call-site `ERR_GIT{op}` rows for every Git call in `Init`/`Advance`; a counting store fails `Lock`, `EventsWithLines`, or append #N; seam row: `Fake.Diffs["<base>..<head>"] = "D"` with a small `MaxDiffBytes` → the fake kind/executor observe `diff.Text == "D"`, `Truncated == true`, `nonce == "n1"`. M1 `Init`: hand-written expected sequence `[init(no stamps), tree(State=initial, Iter 0), warn?]` with every `InitData` field asserted literally (embedded + path workflows; `workflow.yaml` sidecar bytes == raw; `Create` before sidecar observed via a fake store/sidecar call log; `ERR_RUN_EXISTS` leaves the victim's sidecar intact); `ERR_WORKFLOW_NOT_FOUND`, `ERR_WORKFLOW_TOO_LARGE`; goldens ok/unknown field/over cap/over bytes; consent list as a hand-written literal (pinned/unpinned marks, env **names** only, no process env values) + sha in Detail, wrong sha, no cmds; `ERR_MOCK_INVALID`; `RepoMode` override `enforcing` accepted / `advisory` over `enforcing` refused; `RevParse` base (`main` → sha); `ERR_WORKDIR_FOREIGN`; `ERR_BAD_REPO_MODE`; mock hash pinned + `Kinds.Mock()` mismatch; unknown `--var`; M2 `Advance` on both shipped workflows with a fake Registry: hand-written expected event-type sequences per path (`review-loop` clean/reviewed; `sdlc-loop` clean at discover, clean at adjudicate, fixed after 1 iteration, loop once then fixed), literal asserts on transition fields, `needs_input` once across `advance, record tokens, advance` and again at `discover@1`, `View.NextAction` per step; goldens regression-only; M3 gate failure: two failing gates → `Gate` is the first in evaluation order and `transition.Gate` names it; loop-boundary variant (tt and the loop gate both fail → tt named); two passing gates → the first taken; `ERR_INSTRUCTIONS_FAILED` (needs_input already present stays, nothing further); ctx cancelled during `Execute` → error returned, no pseudo-gate, next `Advance` resumes with `StartIndex`; `ERR_GATE_INAPPLICABLE`; executor error → `executor` pseudo-gate with earlier `llm_call`s kept and `StartIndex` honoured on the next fork (interrupted-execution fixture: pre-seeded `llm_call` index 0, executor asserts `StartIndex == 1`); decode error / Reduce error / rejected `delta_applied` (status subset from a fake cmd kind) → `node_output` pseudo-gate; M4 loop: cumulative regression (iter 3 fixes its own bug, 7 remain: loop taken, `AllFound == 8`, `Unfixed == 7`, all 8 statuses, not `fixed`); **gate-first**: `max_iterations: 1` with all bugs fixed at verify → `fixed`, zero `converge` events; negative control one bug left → `converge{max_iterations}` → `overflow`; `stalled` via nil-then-plateau and via regression (`Prev 3 → Unfixed 5`); `budget` via `llm_call` tokens and via `record tokens`; `custom` via cmd atom; converge error → `converge` pseudo-gate and no loop taken; `fixed_class` guard via a fake predicate; user workflow whose terminal gate is `confirmed_empty` (not `all_fixed`) with all bugs fixed and findings present → convergence evaluated (bounded) — the `max_iterations` stop fires; emitter caps: a 5 KB cmd-atom reason → `converge.Reason` and `StopReason` capped; overflow handler once, `overflow_handler` fields literal, failure warn, **not** run for `stalled`/`custom`, resumed after a crash (fixture: terminal overflow run without handler → `Advance` runs it, then `ERR_RUN_TERMINAL`); M5 tree: identical porcelain + different `WorkTree` → advisory warn (+ tree) vs enforcing `repo_mode` gate with **no** tree (a second `Advance` re-detects); porcelain ≈ 5 KB → warn Detail capped at `MaxText`, tree Status intact; 70 KB porcelain → `tree.Status` capped with flag; agent-edit exempt; baseline `tree` appended when `TreeHash == ""` (fork-from-initial fixture); `tree` only on change (count); M6 `Record` refusals per code and sub-reason (`syntax`, `event_type` (`transition`), `reserved`), `ERR_RECORD_TOKENS` on unknown field and on `-1`, `Replace`, terminal `tokens` allowed, `ERR_NODE_OUTPUT_INVALID` leaves `Events` byte-identical; M7 `Open`: `ERR_WORKFLOW_CHANGED` via sidecar edit; embedded bytes replaced by a workflow with different transitions while the sidecar is intact → `Advance` follows the **sidecar's** transitions; `ERR_CMD_CHANGED`; `ERR_MOCK_MISMATCH` via scenario edit and via registry mismatch; torn → `ERR_AUDIT_TORN`; `Repair` → warn Detail literal + fold ok; `Repair` at offset 0 → `ERR_RUN_NOT_FOUND`; `ERR_SIDECAR{missing}`; M8 stamps: every event's `At` equals the injected clock sequence, `State/Iter/Mock/Node` per §5.4 tail (`cmd_call` has no Node), non-mock runs never carry `Mock: true` (`MockTainted == false`), mock runs carry it on every non-init event; M9 §5.7 table incl. `StopReason` ("atom: reason"), `Untrusted` list, `Deps.Terminal` called for every terminal outcome incl. `failed`, again on a later `Advance` of a terminal run (idempotency is spec 3's), and on the 9b resume path; `Terminal` error returned with the transition durable; `ERR_AUDIT_FULL` surfaced; a counting store that fails append #N for every N of the happy sequence returns the error unchanged; FS `Sidecar`: symlink refused, exists refused, mode 0600, missing run → `ERR_SIDECAR` |
 
 ## 8. Ledger
 - `cmds:` single top-level declaration referenced by name; per-cmd `timeout`/`env` are consent-covered (design §16 inline argv retired).
 - `failed` reserved; `duplicate_transition` on `(from, gate)`; loop safety reasons; `bad_state` (`judge` reserved for spec 5); `bad_env` reserved names.
 - Loop boundary is order-independent: `TerminalFor` gate first, convergence only when `!AllFixed`, then the loop gate and remaining transitions (C3 gate-first, made structural).
 - Converge errors are the `converge` pseudo-gate; enforcing edits, executor and decode failures are `repo_mode`/`executor`/`node_output` pseudo-gates; the failed transition names the first failing gate.
-- `needs_input` once per key; `tree` at `Init` and on change (content-aware `WorkTree`; agent-edit states may emit one per advance while the agent edits — accepted).
-- `commit_exists` = `FixEntryHead..HEAD` + `ERR_GATE_INAPPLICABLE` (SCP3-5); Git failures inside gates are gate errors (recovery by fork) — accepted.
+- `needs_input` once per key; `tree` at `Init`, as a baseline when `TreeHash == ""`, and on change (content-aware `WorkTree`; agent-edit states may emit one per advance while the agent edits — accepted).
+- `commit_exists` = `FixEntryHead..HEAD` + `ERR_GATE_INAPPLICABLE` (SCP3-5); Git failures inside gates are gate errors (recovery by fork) — accepted. `clean` counts untracked files: a host that leaves stray files after its commit gets `GATE_FAILED` (visible; fork to retry) — spec 5 documents it and makes sure `.metareview/` is ignored in the target repo.
+- Registry defaults (`DefaultExec`) are not persisted: a binary whose registry defaults change reinterprets in-flight runs' inferred `exec` — accepted for 0.9.0 (shipped YAMLs set `model`/`effort`, inferred exec only on `fix`/`verify`); a `Kinds.Info()` digest in `init` is a follow-up.
+- The consent preimage has no version tag: a future `AllowedCmd` field changes every `cmds_sha256` for unchanged YAML — accepted, ledgered.
 - `Open` verifies the run's `workflow.yaml` sidecar (written after `Create`, `O_EXCL`); forks copy the parent's sidecar (spec 3 r2 obligation; also `Export` includes it).
 - `ERR_RECORD_NAME` narrows locked C15 (reserved names refused; plan E13's `record transition` row becomes an `ERR_RECORD_NAME` row in spec 5).
 - `machine` does not import `workflows` (plan §1.1 had the edge); the CLI passes `Deps.Workflows`.
diff --git a/docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md b/docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md
index 175bb76..b5d9203 100644
--- a/docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md
+++ b/docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md
@@ -50,12 +50,16 @@ internal/fsm/mockai    scenario files → judge.Script + cmdexec fake rows; cont
 type Spec struct { Name string /* declared cmd name; the fake keys on it, the exec runner ignores it */; Argv []string; Dir string; Stdin []byte; Timeout time.Duration; Env []string }
 type Result struct { Stdout, Stderr []byte; ExitCode int; Duration time.Duration }
 type Runner interface { Run(ctx, Spec) (Result, error) }
-type Guarded struct { Runner Runner; Allowed []run.AllowedCmd; Dir string; RunID string; FileHash func(string) (string, error); Audit func(run.Event) error; Environ func() []string; Clock func() time.Time }
+type Guarded struct { Runner Runner; Allowed []run.AllowedCmd; Dir string; RunID string; FileHash func(string) (string, error); Audit func(run.Event) error; Environ func() []string; CmdCalls func(name string) int /* from machine.RunnerDeps */ }
 func NewExecRunner() Runner                                                        // the real one; the fake is mockai's
 func (g Guarded) Run(ctx, name string, stdin []byte) (converge.CmdResult, error)   // converge.Runner
 func (g Guarded) Call(ctx, name string, stdin []byte, out any) error               // shares Run's unaudited core; ONE cmd_call per call (audited after decode)
+var _ converge.Caller = Guarded{}                                                  // Caller = Runner + Call, declared in converge
 ```
-Spec 2's `Deps.Runner` and this package's `kind.Deps.Guarded` are the **same closure** (spec 5 builds one `Guarded` factory).
+`machine.Deps.Runner func(RunnerDeps) converge.Caller` is the ONE guarded factory (spec 5 wires `Guarded{Runner: NewExecRunner(), …}`
+or the mock runner); the machine hands the same value to executors as `ExecInput.Runner`. `Spec.Ordinal = CmdCalls(name)`.
+Implemented (`internal/fsm/cmdexec`, 100%): a parent-context cancellation returns `ctx.Err()` (the machine treats it as
+resumable); `DurationMS` comes from the runner's measured `Result.Duration` (no `Clock`).
 `Run`: `name ∈ Allowed` else `ERR_CMD_NOT_ALLOWED{name}` **without audit** (the fold refuses unsanctioned names; the
 check is defense in depth — workflow validation already guarantees names); `Argv[0]` must be absolute else
 `ERR_CMD_NOT_ALLOWED{reason: relative}` (no audit); re-verify with `workflow.VerifyCmds` (`cmdexec → workflow` edge;
@@ -148,8 +152,8 @@ Overrides are agent-satisfiable and unstamped (ledger): spec 5 lists them with t
   cache_read_input_tokens, cache_creation_input_tokens, output_tokens}`.
 - **OpenAI-compatible** `POST {base}/v1/chat/completions`: `model`, `messages`, `max_completion_tokens` (= cap; `glm*`/`kimi*`
   → `max(cap, 16384)`); `reasoning_effort` table — `gpt*`/`openai/*`/`glm*`: `low→low, medium→medium, high→high,
-  xhigh→high`; `kimi*`: `low→low, medium→high, high→high, xhigh→max`; `temperature: 1` when `reasoning_effort` is sent
-  else `temperature: 0` (`model_router.py:151`). `Authorization: Bearer`. Text = `choices[0].message.content` (string).
+  xhigh→high`; `kimi*`: `low→low, medium→high, high→high, xhigh→max`; `temperature: 1` always (every accepted effort maps to a `reasoning_effort`; the reference's `temperature: 0` branch is
+  unreachable here, `model_router.py:151`). `Authorization: Bearer`. Text = `choices[0].message.content` (string).
   Tokens: `prompt_tokens` (`Input`), `prompt_tokens_details.cached_tokens` (`CacheRead`, 0 if absent),
   `completion_tokens_details.reasoning_tokens` (`Reasoning`, 0 if absent), `Output = max(0, completion_tokens − Reasoning)`.
 No text / non-JSON body / missing `usage` → `ERR_JUDGE_RESPONSE{detail}` (tokens of earlier attempts kept). **Retry** (≤ 5
@@ -183,7 +187,7 @@ Executors `Audit` immediately after each `Judge.Call` returns; a non-parse error
 ## 4. `kind`
 ### 4.1 Common
 ```go
-type Deps struct { Judge judge.Judge; Guarded func(allowed []run.AllowedCmd, workDir, runID string, audit func(run.Event) error) converge.Runner /* the same closure as machine.Deps.Runner; Call = cmdexec.Call(runner, …) */; Mock bool }
+type Deps struct { Judge judge.Judge; Mock bool }   // no runner here: the cmd kind uses ExecInput.Runner (converge.Caller — Run + Call), the session's single guarded runner
 func New(d Deps) (*Registry, error)   // Registry.Mock() == d.Mock; New refuses Mock:true with a non-*judge.MockJudge and Mock:false with one (ERR_MOCK_MISMATCH)
 // Bug.Verdict constants: run.VerdictMatched = "matched", run.VerdictRealButUngold = "real_but_ungold", run.VerdictHallucination = "hallucination" (typed in run; Decode validates the set for every kind incl. cmd)
 // Registry.Executor(name) for host-only kinds returns (nil, false).
diff --git a/go.mod b/go.mod
index e9838d5..ac370ae 100644
--- a/go.mod
+++ b/go.mod
@@ -2,4 +2,4 @@ module github.com/dsifry/metareview
 
 go 1.26
 
-require gopkg.in/yaml.v3 v3.0.1 // indirect
+require gopkg.in/yaml.v3 v3.0.1
diff --git a/go.sum b/go.sum
index 4bc0337..a62c313 100644
--- a/go.sum
+++ b/go.sum
@@ -1,3 +1,4 @@
+gopkg.in/check.v1 v0.0.0-20161208181325-20d25e280405 h1:yhCVgyC4o1eVCa2tZl7eS0r+SDo693bJlVdllGtEeKM=
 gopkg.in/check.v1 v0.0.0-20161208181325-20d25e280405/go.mod h1:Co6ibVJAznAaIkqp8huTwlJQCZ016jof/cbN4VW5Yz0=
 gopkg.in/yaml.v3 v3.0.1 h1:fxVm/GzAzEWqLHuvctI91KS9hhNmmWOoWu0XTYJS7CA=
 gopkg.in/yaml.v3 v3.0.1/go.mod h1:K4uyk7z7BCEPqu6E+C64Yfv1cQ7kz7rIZviUmN+EgEM=
diff --git a/internal/fsm/cmdexec/cmdexec.go b/internal/fsm/cmdexec/cmdexec.go
new file mode 100644
index 0000000..b5e4e87
--- /dev/null
+++ b/internal/fsm/cmdexec/cmdexec.go
@@ -0,0 +1,267 @@
+// Package cmdexec runs consented commands: argv only (no shell), pinned
+// absolute argv[0], hash re-verification, an exact environment allow-list,
+// a process-group timeout, bounded output, typed decode, and one audited
+// cmd_call per execution. Nothing here is reachable without consent.
+package cmdexec
+
+import (
+	"bytes"
+	"context"
+	"crypto/sha256"
+	"encoding/hex"
+	"encoding/json"
+	"errors"
+	"fmt"
+	"os"
+	"os/exec"
+	"path/filepath"
+	"strings"
+	"syscall"
+	"time"
+
+	"github.com/dsifry/metareview/internal/fsm/converge"
+	"github.com/dsifry/metareview/internal/fsm/errs"
+	"github.com/dsifry/metareview/internal/fsm/run"
+	"github.com/dsifry/metareview/internal/fsm/workflow"
+)
+
+// Error codes.
+const (
+	CodeCmdNotAllowed    = "ERR_CMD_NOT_ALLOWED"
+	CodeCmdTimeout       = "ERR_CMD_TIMEOUT"
+	CodeCmdFailed        = "ERR_CMD_FAILED"
+	CodeCmdOutputInvalid = "ERR_CMD_OUTPUT_INVALID"
+	DefaultTimeout       = 60 * time.Second
+	WaitDelay            = 2 * time.Second
+	baseEnvNames         = "PATH HOME LANG TMPDIR"
+	EnvRunID             = "MRV_RUN_ID"
+	MaxOutput            = run.MaxPayload
+	unknownExit          = -1
+)
+
+// Spec is one execution request.
+type Spec struct {
+	Name    string // declared cmd name; the fake runner keys on it, the exec runner ignores it
+	Ordinal int    // prior cmd_call count for Name (durable mock ordinal)
+	Argv    []string
+	Dir     string
+	Stdin   []byte
+	Timeout time.Duration
+	Env     []string // the full child environment, KEY=VALUE
+}
+
+// Result is what the process produced. Stdout/Stderr are capped at
+// MaxOutput+1 bytes (the extra byte marks overflow).
+type Result struct {
+	Stdout, Stderr []byte
+	ExitCode       int
+	Duration       time.Duration
+}
+
+// Runner executes a Spec. Timeouts are reported as ERR_CMD_TIMEOUT; a parent
+// context cancellation is returned as ctx.Err(); a process that could not
+// start is ERR_CMD_FAILED{reason: spawn}.
+type Runner interface {
+	Run(ctx context.Context, s Spec) (Result, error)
+}
+
+// Guarded wraps a Runner with the consent guardrails.
+type Guarded struct {
+	Runner   Runner
+	Allowed  []run.AllowedCmd
+	Dir      string
+	RunID    string
+	FileHash func(string) (string, error)
+	Audit    func(run.Event) error
+	Environ  func() []string
+	Clock    func() time.Time
+	CmdCalls func(name string) int // prior cmd_call count (nil → 0)
+}
+
+var _ converge.Caller = Guarded{}
+
+func (g Guarded) find(name string) (run.AllowedCmd, bool) {
+	for _, c := range g.Allowed {
+		if c.Name == name {
+			return c, true
+		}
+	}
+	return run.AllowedCmd{}, false
+}
+
+// env builds the exact child environment.
+func (g Guarded) env(c run.AllowedCmd) []string {
+	present := map[string]string{}
+	if g.Environ != nil {
+		for _, kv := range g.Environ() {
+			k, v, ok := strings.Cut(kv, "=")
+			if ok {
+				present[k] = v
+			}
+		}
+	}
+	var out []string
+	for _, k := range strings.Fields(baseEnvNames) {
+		if v, ok := present[k]; ok {
+			out = append(out, k+"="+v)
+		}
+	}
+	out = append(out, EnvRunID+"="+g.RunID)
+	for _, k := range c.Env {
+		if v, ok := present[k]; ok {
+			out = append(out, k+"="+v)
+		}
+	}
+	return out
+}
+
+// exec is the shared unaudited core; it returns the audit payload alongside.
+func (g Guarded) exec(ctx context.Context, name string, stdin []byte) (converge.CmdResult, run.CmdCallData, error) {
+	c, ok := g.find(name)
+	if !ok {
+		return converge.CmdResult{}, run.CmdCallData{}, errs.E(CodeCmdNotAllowed, "command "+name+" is not consented", "name", name)
+	}
+	if len(c.Argv) == 0 || !filepath.IsAbs(c.Argv[0]) {
+		return converge.CmdResult{}, run.CmdCallData{}, errs.E(CodeCmdNotAllowed, "consented argv[0] is not an absolute path", "name", name, "reason", "relative")
+	}
+	if err := workflow.VerifyCmds([]run.AllowedCmd{c}, g.Dir, g.FileHash); err != nil {
+		return converge.CmdResult{}, run.CmdCallData{}, err
+	}
+	timeout := DefaultTimeout
+	if c.TimeoutMS > 0 {
+		timeout = time.Duration(c.TimeoutMS) * time.Millisecond
+	}
+	ordinal := 0
+	if g.CmdCalls != nil {
+		ordinal = g.CmdCalls(name)
+	}
+	sum := sha256.Sum256(stdin)
+	spec := Spec{Name: name, Ordinal: ordinal, Argv: append([]string(nil), c.Argv...), Dir: g.Dir, Stdin: stdin, Timeout: timeout, Env: g.env(c)}
+	res, err := g.Runner.Run(ctx, spec)
+	data := run.CmdCallData{Name: name, Argv: spec.Argv, InputHash: hex.EncodeToString(sum[:]), ExitCode: res.ExitCode, DurationMS: res.Duration.Milliseconds()}
+	data.Stdout, data.StdoutTruncated = run.CapText(string(res.Stdout), run.MaxDetail)
+	data.Stderr, data.StderrTruncated = run.CapText(string(res.Stderr), run.MaxStderr)
+	out := converge.CmdResult{Stdout: res.Stdout, Stderr: res.Stderr, ExitCode: res.ExitCode, Duration: res.Duration}
+	switch {
+	case err != nil:
+		data.ExitCode = unknownExit
+		data.Error = errs.Code(err)
+		if data.Error == "" {
+			data.Error = CodeCmdFailed
+			err = errs.Wrap(errs.E(CodeCmdFailed, err.Error(), "name", name, "reason", "spawn"), err)
+		}
+	case len(res.Stdout) > MaxOutput || len(res.Stderr) > MaxOutput:
+		data.Error = CodeCmdOutputInvalid
+		err = errs.E(CodeCmdOutputInvalid, "command output exceeds MaxPayload", "name", name, "reason", "too_large")
+	case res.ExitCode != 0:
+		data.Error = CodeCmdFailed
+		err = errs.E(CodeCmdFailed, fmt.Sprintf("command %s exited %d", name, res.ExitCode), "name", name, "exit", fmt.Sprint(res.ExitCode))
+	}
+	return out, data, err
+}
+
+func (g Guarded) audit(data run.CmdCallData) error {
+	if g.Audit == nil {
+		return nil
+	}
+	return g.Audit(run.Event{Type: run.TypeCmdCall, Data: run.MarshalCanonical(data)})
+}
+
+// Run executes a consented command and audits it (converge.Runner).
+func (g Guarded) Run(ctx context.Context, name string, stdin []byte) (converge.CmdResult, error) {
+	res, data, err := g.exec(ctx, name, stdin)
+	if data.Name == "" {
+		return converge.CmdResult{}, err // pre-exec refusal: never audited
+	}
+	if aerr := g.audit(data); aerr != nil {
+		return converge.CmdResult{}, aerr
+	}
+	return res, err
+}
+
+// Call runs and decodes the full stdout into out (DisallowUnknownFields);
+// the single cmd_call it audits carries the decode error when there is one.
+func (g Guarded) Call(ctx context.Context, name string, stdin []byte, out any) error {
+	res, data, err := g.exec(ctx, name, stdin)
+	if data.Name == "" {
+		return err
+	}
+	if err == nil {
+		dec := json.NewDecoder(bytes.NewReader(res.Stdout))
+		dec.DisallowUnknownFields()
+		if derr := dec.Decode(out); derr != nil {
+			err = errs.E(CodeCmdOutputInvalid, "command "+name+" stdout did not decode: "+derr.Error(), "name", name, "reason", "decode")
+			data.Error = CodeCmdOutputInvalid
+		}
+	}
+	if aerr := g.audit(data); aerr != nil {
+		return aerr
+	}
+	return err
+}
+
+// ---------------------------------------------------------------- exec runner
+
+type execRunner struct{}
+
+// NewExecRunner returns the real process runner.
+func NewExecRunner() Runner { return execRunner{} }
+
+// cappingWriter keeps at most MaxOutput+1 bytes and keeps draining.
+type cappingWriter struct{ buf bytes.Buffer }
+
+func (w *cappingWriter) Write(p []byte) (int, error) {
+	if room := MaxOutput + 1 - w.buf.Len(); room > 0 {
+		if len(p) > room {
+			w.buf.Write(p[:room])
+		} else {
+			w.buf.Write(p)
+		}
+	}
+	return len(p), nil
+}
+
+func (execRunner) Run(ctx context.Context, s Spec) (Result, error) {
+	timeout := s.Timeout
+	if timeout <= 0 {
+		timeout = DefaultTimeout
+	}
+	tctx, cancel := context.WithTimeout(ctx, timeout)
+	defer cancel()
+	cmd := exec.CommandContext(tctx, s.Argv[0], s.Argv[1:]...)
+	cmd.Dir = s.Dir
+	cmd.Env = s.Env
+	cmd.Stdin = bytes.NewReader(s.Stdin)
+	var stdout, stderr cappingWriter
+	cmd.Stdout, cmd.Stderr = &stdout, &stderr
+	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
+	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
+	cmd.WaitDelay = WaitDelay
+	start := time.Now()
+	err := cmd.Run()
+	res := Result{Stdout: stdout.buf.Bytes(), Stderr: stderr.buf.Bytes(), Duration: time.Since(start)}
+	switch {
+	case err == nil:
+		return res, nil
+	case ctx.Err() != nil:
+		res.ExitCode = unknownExit
+		return res, ctx.Err()
+	case tctx.Err() != nil:
+		res.ExitCode = unknownExit
+		return res, errs.E(CodeCmdTimeout, fmt.Sprintf("command exceeded %s", timeout), "timeout", timeout.String())
+	}
+	var ee *exec.ExitError
+	if errors.As(err, &ee) {
+		res.ExitCode = ee.ExitCode()
+		return res, nil
+	}
+	res.ExitCode = unknownExit
+	return res, errs.Wrap(errs.E(CodeCmdFailed, err.Error(), "reason", "spawn"), err)
+}
+
+// Executable returns the absolute path of the running binary (tests pin it
+// as argv[0]).
+func Executable() string {
+	p, _ := os.Executable()
+	return p
+}
diff --git a/internal/fsm/cmdexec/cmdexec_test.go b/internal/fsm/cmdexec/cmdexec_test.go
new file mode 100644
index 0000000..87a8f3b
--- /dev/null
+++ b/internal/fsm/cmdexec/cmdexec_test.go
@@ -0,0 +1,372 @@
+package cmdexec
+
+import (
+	"context"
+	"encoding/json"
+	"errors"
+	"fmt"
+	"os"
+	"os/exec"
+	"path/filepath"
+	"sort"
+	"strconv"
+	"strings"
+	"syscall"
+	"testing"
+	"time"
+
+	"github.com/dsifry/metareview/internal/fsm/converge"
+	"github.com/dsifry/metareview/internal/fsm/errs"
+	"github.com/dsifry/metareview/internal/fsm/run"
+)
+
+// TestHelperProcess is the child side of the real-runner rows. Activation is
+// through argv (the environment is scrubbed): everything after "--" is the
+// mode and its arguments.
+func TestHelperProcess(t *testing.T) {
+	args := os.Args
+	i := 0
+	for ; i < len(args); i++ {
+		if args[i] == "--" {
+			break
+		}
+	}
+	if i == len(args) {
+		return
+	}
+	rest := args[i+1:]
+	switch rest[0] {
+	case "echo":
+		stdin, _ := readAll(os.Stdin)
+		env := os.Environ()
+		sort.Strings(env)
+		wd, _ := os.Getwd()
+		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"args": rest[1:], "env": env, "stdin": string(stdin), "wd": wd})
+	case "exit":
+		n, _ := strconv.Atoi(rest[1])
+		fmt.Fprint(os.Stderr, "failing")
+		os.Exit(n)
+	case "sleep-grandchild":
+		c := exec.Command("sleep", "30")
+		_ = c.Start()
+		fmt.Fprintln(os.Stdout, c.Process.Pid)
+		_ = c.Wait()
+	case "big":
+		n, _ := strconv.Atoi(rest[1])
+		os.Stdout.WriteString(strings.Repeat("x", n))
+	case "bigerr":
+		n, _ := strconv.Atoi(rest[1])
+		os.Stderr.WriteString(strings.Repeat("e", n))
+	case "json":
+		os.Stdout.WriteString(rest[1])
+	case "slow-ok":
+		time.Sleep(200 * time.Millisecond)
+		os.Stdout.WriteString("done")
+	}
+	os.Exit(0)
+}
+
+func readAll(f *os.File) ([]byte, error) {
+	var out []byte
+	buf := make([]byte, 4096)
+	for {
+		n, err := f.Read(buf)
+		out = append(out, buf[:n]...)
+		if err != nil {
+			return out, nil
+		}
+	}
+}
+
+func helperArgv(mode string, args ...string) []string {
+	return append([]string{Executable(), "-test.run=TestHelperProcess", "--", mode}, args...)
+}
+
+type recordingRunner struct {
+	specs []Spec
+	res   Result
+	err   error
+}
+
+func (r *recordingRunner) Run(_ context.Context, s Spec) (Result, error) {
+	r.specs = append(r.specs, s)
+	return r.res, r.err
+}
+
+func hashFor(m map[string]string) func(string) (string, error) {
+	return func(p string) (string, error) {
+		if h, ok := m[p]; ok {
+			return h, nil
+		}
+		return "", errors.New("no such file")
+	}
+}
+
+func TestX1RealRunner(t *testing.T) {
+	ctx := context.Background()
+	exe := Executable()
+	hashes := map[string]string{exe: "hexe"}
+	var audits []run.CmdCallData
+	g := Guarded{
+		Runner: NewExecRunner(), Dir: t.TempDir(), RunID: "mrv-x1-run", FileHash: hashFor(hashes),
+		Environ: func() []string {
+			return []string{"PATH=/usr/bin:/bin", "HOME=/home/t", "SECRET_TOKEN=s3cr3t", "TOKEN=tok", "LANG=C"}
+		},
+		Audit: func(ev run.Event) error {
+			var d run.CmdCallData
+			_ = json.Unmarshal(ev.Data, &d)
+			audits = append(audits, d)
+			return nil
+		},
+	}
+	t.Setenv("SECRET_TOKEN", "parent-secret")
+	g.Allowed = []run.AllowedCmd{
+		{Name: "echo", Argv: helperArgv("echo", "; rm -rf x", "$HOME", "*", "two words"), FileHashes: map[string]string{exe: "hexe"}, Env: []string{"TOKEN", "UNSET_NAME"}},
+		{Name: "exit3", Argv: helperArgv("exit", "3"), FileHashes: map[string]string{exe: "hexe"}},
+		{Name: "grand", Argv: helperArgv("sleep-grandchild"), FileHashes: map[string]string{exe: "hexe"}, TimeoutMS: 300},
+		{Name: "slow", Argv: helperArgv("slow-ok"), FileHashes: map[string]string{exe: "hexe"}, TimeoutMS: 2000},
+		{Name: "big", Argv: helperArgv("big", strconv.Itoa(MaxOutput+1)), FileHashes: map[string]string{exe: "hexe"}},
+		{Name: "bigok", Argv: helperArgv("big", strconv.Itoa(MaxOutput)), FileHashes: map[string]string{exe: "hexe"}},
+		{Name: "bigerr", Argv: helperArgv("bigerr", strconv.Itoa(MaxOutput+1)), FileHashes: map[string]string{exe: "hexe"}},
+	}
+	// argv verbatim, exact env, stdin, dir
+	res, err := g.Run(ctx, "echo", []byte("input-bytes"))
+	if err != nil {
+		t.Fatal(err)
+	}
+	var got struct {
+		Args  []string `json:"args"`
+		Env   []string `json:"env"`
+		Stdin string   `json:"stdin"`
+		WD    string   `json:"wd"`
+	}
+	if err := json.Unmarshal(res.Stdout, &got); err != nil {
+		t.Fatalf("%v: %s", err, res.Stdout)
+	}
+	if strings.Join(got.Args, "|") != "; rm -rf x|$HOME|*|two words" {
+		t.Fatalf("argv not literal: %v", got.Args)
+	}
+	wantEnv := []string{"HOME=/home/t", "LANG=C", "MRV_RUN_ID=mrv-x1-run", "PATH=/usr/bin:/bin", "TOKEN=tok"}
+	if strings.Join(got.Env, ",") != strings.Join(wantEnv, ",") {
+		t.Fatalf("env set:\n%v\n%v", got.Env, wantEnv)
+	}
+	if got.Stdin != "input-bytes" {
+		t.Fatal("stdin")
+	}
+	if wd, _ := filepath.EvalSymlinks(g.Dir); got.WD != wd && got.WD != g.Dir {
+		t.Fatalf("dir %s vs %s", got.WD, g.Dir)
+	}
+	a := audits[0]
+	if a.Name != "echo" || a.Argv[0] != exe || a.ExitCode != 0 || a.Error != "" || a.InputHash != sha("input-bytes") || a.Stdout == "" || a.DurationMS < 0 {
+		t.Fatalf("audit: %+v", a)
+	}
+	// exit code
+	_, err = g.Run(ctx, "exit3", nil)
+	if e := errs.As(err); e == nil || e.Code != CodeCmdFailed || e.Field("exit") != "3" || audits[1].ExitCode != 3 || !strings.HasPrefix(audits[1].Stderr, "failing") || audits[1].Error != CodeCmdFailed {
+		t.Fatalf("exit: %v %+v", err, audits[1])
+	}
+	// timeout kills the group: grandchild gone, elapsed within [timeout, timeout+WaitDelay+1s]
+	start := time.Now()
+	res, err = g.Run(ctx, "grand", nil)
+	elapsed := time.Since(start)
+	if !errs.Is(err, CodeCmdTimeout) || elapsed < 300*time.Millisecond || elapsed > 300*time.Millisecond+WaitDelay+time.Second {
+		t.Fatalf("timeout: %v after %s", err, elapsed)
+	}
+	pid, _ := strconv.Atoi(strings.TrimSpace(string(res.Stdout)))
+	if pid <= 0 {
+		t.Fatalf("grandchild pid not reported: %q", res.Stdout)
+	}
+	deadline := time.Now().Add(3 * time.Second)
+	for time.Now().Before(deadline) && syscall.Kill(pid, 0) == nil {
+		time.Sleep(50 * time.Millisecond)
+	}
+	if syscall.Kill(pid, 0) == nil {
+		_ = syscall.Kill(pid, syscall.SIGKILL)
+		t.Fatal("grandchild survived the timeout")
+	}
+	if audits[2].ExitCode != -1 || audits[2].Error != CodeCmdTimeout {
+		t.Fatalf("timeout audit: %+v", audits[2])
+	}
+	// positive timeout row: 2 s budget, 200 ms child
+	if res, err := g.Run(ctx, "slow", nil); err != nil || string(res.Stdout) != "done" {
+		t.Fatalf("slow-ok: %v", err)
+	}
+	// output caps: exactly MaxOutput accepted, over → too_large (stdout and stderr)
+	if res, err := g.Run(ctx, "bigok", nil); err != nil || len(res.Stdout) != MaxOutput {
+		t.Fatalf("at cap: %v %d", err, len(res.Stdout))
+	}
+	if _, err := g.Run(ctx, "big", nil); !errs.Is(err, CodeCmdOutputInvalid) || errs.As(err).Field("reason") != "too_large" {
+		t.Fatalf("over cap: %v", err)
+	}
+	if _, err := g.Run(ctx, "bigerr", nil); !errs.Is(err, CodeCmdOutputInvalid) {
+		t.Fatalf("stderr over cap: %v", err)
+	}
+	last := audits[len(audits)-1]
+	if !last.StderrTruncated || last.Error != CodeCmdOutputInvalid {
+		t.Fatalf("truncation flag: %+v", last)
+	}
+	// parent-context cancellation is returned as ctx.Err(), not a timeout
+	cctx, cancel := context.WithCancel(ctx)
+	go func() { time.Sleep(100 * time.Millisecond); cancel() }()
+	if _, err := g.Run(cctx, "grand", nil); !errors.Is(err, context.Canceled) {
+		t.Fatalf("parent cancel: %v", err)
+	}
+	// spawn failure: pinned binary vanished
+	g.Allowed = append(g.Allowed, run.AllowedCmd{Name: "gone", Argv: []string{"/nonexistent/binary-xyz"}, FileHashes: map[string]string{}})
+	_, err = g.Run(ctx, "gone", nil)
+	if e := errs.As(err); e == nil || e.Code != CodeCmdFailed || e.Field("reason") != "spawn" || audits[len(audits)-1].ExitCode != -1 {
+		t.Fatalf("spawn: %v", err)
+	}
+	// default timeout observed through a recording runner
+	rr := &recordingRunner{res: Result{Stdout: []byte("{}")}}
+	g2 := Guarded{Runner: rr, Allowed: []run.AllowedCmd{{Name: "d", Argv: []string{"/bin/true"}, FileHashes: map[string]string{}}, {Name: "ms", Argv: []string{"/bin/true"}, FileHashes: map[string]string{}, TimeoutMS: 1500}}, FileHash: hashFor(nil)}
+	if _, err := g2.Run(ctx, "d", nil); err != nil || rr.specs[0].Timeout != DefaultTimeout {
+		t.Fatalf("default timeout: %v %s", err, rr.specs[0].Timeout)
+	}
+	if _, err := g2.Run(ctx, "ms", nil); err != nil || rr.specs[1].Timeout != 1500*time.Millisecond {
+		t.Fatalf("timeout unit: %s", rr.specs[1].Timeout)
+	}
+	// the exec runner with a zero Timeout falls back to the default
+	er := NewExecRunner()
+	if _, err := er.Run(ctx, Spec{Argv: helperArgv("json", `{}`), Env: []string{}}); err != nil {
+		t.Fatal(err)
+	}
+}
+
+func sha(s string) string {
+	h := run.OutputHash
+	_ = h
+	sum := sha256Sum([]byte(s))
+	return sum
+}
+
+func TestX2Guarded(t *testing.T) {
+	ctx := context.Background()
+	hashes := map[string]string{"/bin/sh": "h1", "/w/s.sh": "h2"}
+	var audits []run.CmdCallData
+	auditErr := error(nil)
+	rr := &recordingRunner{res: Result{Stdout: []byte(`{"stop": true, "reason": "r"}`), Stderr: []byte("e"), Duration: 1500 * time.Millisecond}}
+	g := Guarded{
+		Runner: rr, Dir: "/w", RunID: "mrv-x2", FileHash: hashFor(hashes),
+		Allowed: []run.AllowedCmd{
+			{Name: "ok", Argv: []string{"/bin/sh", "./s.sh"}, FileHashes: map[string]string{"/bin/sh": "h1", "/w/s.sh": "h2"}, TimeoutMS: 1500, Env: []string{"TOKEN"}},
+			{Name: "rel", Argv: []string{"sh"}, FileHashes: map[string]string{}},
+		},
+		Environ:  func() []string { return []string{"TOKEN=t", "PATH=/bin", "IGNORED=x"} },
+		CmdCalls: func(name string) int { return 7 },
+		Audit: func(ev run.Event) error {
+			if auditErr != nil {
+				return auditErr
+			}
+			var d run.CmdCallData
+			_ = json.Unmarshal(ev.Data, &d)
+			audits = append(audits, d)
+			return nil
+		},
+	}
+	// not-allowed and relative argv[0]: refused, not audited
+	if _, err := g.Run(ctx, "nope", nil); !errs.Is(err, CodeCmdNotAllowed) || len(audits) != 0 {
+		t.Fatalf("not allowed: %v", err)
+	}
+	if err := g.Call(ctx, "nope", nil, &struct{}{}); !errs.Is(err, CodeCmdNotAllowed) || len(audits) != 0 {
+		t.Fatalf("not allowed call: %v", err)
+	}
+	if _, err := g.Run(ctx, "rel", nil); !errs.Is(err, CodeCmdNotAllowed) || errs.As(err).Field("reason") != "relative" || len(audits) != 0 {
+		t.Fatalf("relative: %v", err)
+	}
+	// hash mismatch / missing / appeared: refused, not audited
+	hashes["/w/s.sh"] = "changed"
+	if _, err := g.Run(ctx, "ok", nil); errs.Code(err) != "ERR_CMD_CHANGED" || len(audits) != 0 {
+		t.Fatalf("changed: %v", err)
+	}
+	hashes["/w/s.sh"] = "h2"
+	// success: pinned argv executed verbatim, env exact, ordinal, audit fields
+	res, err := g.Run(ctx, "ok", []byte("in"))
+	if err != nil || string(res.Stdout) != `{"stop": true, "reason": "r"}` || res.Duration != 1500*time.Millisecond {
+		t.Fatalf("run: %v %+v", err, res)
+	}
+	sp := rr.specs[0]
+	if sp.Name != "ok" || sp.Ordinal != 7 || strings.Join(sp.Argv, " ") != "/bin/sh ./s.sh" || sp.Dir != "/w" || string(sp.Stdin) != "in" || sp.Timeout != 1500*time.Millisecond || strings.Join(sp.Env, ",") != "PATH=/bin,MRV_RUN_ID=mrv-x2,TOKEN=t" {
+		t.Fatalf("spec: %+v", sp)
+	}
+	a := audits[0]
+	if a.Name != "ok" || a.InputHash != sha256Sum([]byte("in")) || a.Stdout != `{"stop": true, "reason": "r"}` || a.Stderr != "e" || a.DurationMS != 1500 || a.Error != "" || a.ExitCode != 0 {
+		t.Fatalf("audit: %+v", a)
+	}
+	// Call decodes the full stdout; the audited copy is truncated when large
+	big := `{"stop": true, "reason": "` + strings.Repeat("r", run.MaxDetail) + `"}`
+	rr.res = Result{Stdout: []byte(big)}
+	var out struct {
+		Stop   bool   `json:"stop"`
+		Reason string `json:"reason"`
+	}
+	if err := g.Call(ctx, "ok", nil, &out); err != nil || !out.Stop || len(out.Reason) != run.MaxDetail {
+		t.Fatalf("call full stdout: %v", err)
+	}
+	if last := audits[len(audits)-1]; !last.StdoutTruncated || last.Error != "" || len(audits) != 2 {
+		t.Fatalf("one audit per call, truncated copy: %+v", last)
+	}
+	// decode failures carry the error in the same single cmd_call
+	for _, bad := range []string{`{"stop": true, "extra": 1}`, `nope`} {
+		rr.res = Result{Stdout: []byte(bad)}
+		err := g.Call(ctx, "ok", nil, &out)
+		if e := errs.As(err); e == nil || e.Code != CodeCmdOutputInvalid || e.Field("reason") != "decode" || audits[len(audits)-1].Error != CodeCmdOutputInvalid {
+			t.Fatalf("decode %q: %v", bad, err)
+		}
+	}
+	// failure exit through Call: audited with exit and no decode attempted
+	rr.res = Result{ExitCode: 4}
+	if err := g.Call(ctx, "ok", nil, &out); errs.Code(err) != CodeCmdFailed || audits[len(audits)-1].ExitCode != 4 {
+		t.Fatalf("call exit: %v", err)
+	}
+	// runner error with a code (timeout) and without (spawn)
+	rr.res, rr.err = Result{ExitCode: -1}, errs.E(CodeCmdTimeout, "slow")
+	if _, err := g.Run(ctx, "ok", nil); !errs.Is(err, CodeCmdTimeout) || audits[len(audits)-1].Error != CodeCmdTimeout || audits[len(audits)-1].ExitCode != -1 {
+		t.Fatalf("timeout passthrough: %v", err)
+	}
+	rr.err = errors.New("fork failed")
+	if _, err := g.Run(ctx, "ok", nil); errs.Code(err) != CodeCmdFailed || errs.As(err).Field("reason") != "spawn" {
+		t.Fatalf("spawn wrap: %v", err)
+	}
+	rr.err = nil
+	// audit failure propagates from Run and Call
+	auditErr = errors.New("store full")
+	rr.res = Result{Stdout: []byte(`{}`)}
+	if _, err := g.Run(ctx, "ok", nil); err == nil || err.Error() != "store full" {
+		t.Fatal("run audit error")
+	}
+	if err := g.Call(ctx, "ok", nil, &struct{}{}); err == nil || err.Error() != "store full" {
+		t.Fatal("call audit error")
+	}
+	auditErr = nil
+	// nil Audit / nil CmdCalls / nil Environ are tolerated
+	g3 := Guarded{Runner: rr, Allowed: g.Allowed, Dir: "/w", FileHash: hashFor(hashes)}
+	if _, err := g3.Run(ctx, "ok", nil); err != nil || rr.specs[len(rr.specs)-1].Ordinal != 0 || strings.Join(rr.specs[len(rr.specs)-1].Env, ",") != "MRV_RUN_ID=" {
+		t.Fatalf("nil seams: %v %+v", err, rr.specs[len(rr.specs)-1])
+	}
+	// the Caller interface is satisfied and usable as a converge.Runner
+	var c converge.Caller = g
+	var _ converge.Runner = c
+	// output exactly at cap accepted; over cap refused through the recording runner too
+	rr.res = Result{Stdout: make([]byte, MaxOutput)}
+	if _, err := g3.Run(ctx, "ok", nil); err != nil {
+		t.Fatal("at cap")
+	}
+	rr.res = Result{Stderr: make([]byte, MaxOutput+1)}
+	if _, err := g3.Run(ctx, "ok", nil); !errs.Is(err, CodeCmdOutputInvalid) {
+		t.Fatal("over cap")
+	}
+}
+
+func TestX3CappingWriter(t *testing.T) {
+	var w cappingWriter
+	n, _ := w.Write(make([]byte, MaxOutput))
+	m, _ := w.Write(make([]byte, 10))
+	k, _ := w.Write(make([]byte, 10))
+	if n != MaxOutput || m != 10 || k != 10 || w.buf.Len() != MaxOutput+1 {
+		t.Fatalf("capping writer keeps draining: %d", w.buf.Len())
+	}
+	if Executable() == "" || !filepath.IsAbs(Executable()) {
+		t.Fatal("Executable is absolute")
+	}
+}
diff --git a/internal/fsm/cmdexec/sha_test.go b/internal/fsm/cmdexec/sha_test.go
new file mode 100644
index 0000000..4043b3e
--- /dev/null
+++ b/internal/fsm/cmdexec/sha_test.go
@@ -0,0 +1,11 @@
+package cmdexec
+
+import (
+	"crypto/sha256"
+	"encoding/hex"
+)
+
+func sha256Sum(b []byte) string {
+	s := sha256.Sum256(b)
+	return hex.EncodeToString(s[:])
+}
diff --git a/internal/fsm/converge/converge.go b/internal/fsm/converge/converge.go
index c37f440..1e34295 100644
--- a/internal/fsm/converge/converge.go
+++ b/internal/fsm/converge/converge.go
@@ -49,6 +49,13 @@ type Result struct {
 	Reason string
 }
 
+// Caller is Runner plus the typed-decode Call the cmd kind needs; the single
+// Guarded factory returns one and the machine hands it to executors.
+type Caller interface {
+	Runner
+	Call(ctx context.Context, name string, stdin []byte, out any) error
+}
+
 // Predicate is one node of the convergence tree.
 type Predicate interface {
 	Name() string
diff --git a/internal/fsm/gate/git.go b/internal/fsm/gate/git.go
index 371fee3..2b635c0 100644
--- a/internal/fsm/gate/git.go
+++ b/internal/fsm/gate/git.go
@@ -10,6 +10,7 @@ import (
 	"fmt"
 	"os"
 	"os/exec"
+	"path/filepath"
 	"regexp"
 	"strconv"
 	"strings"
@@ -245,15 +246,12 @@ func (g *execGit) CommonDir(ctx context.Context) (string, error) {
 // changes (not just paths) move the hash. The scratch index lives in the
 // OS temp dir and is removed afterwards.
 func (g *execGit) WorkTree(ctx context.Context) (string, error) {
-	f, err := os.CreateTemp("", "mrv-index-*")
+	dir, err := os.MkdirTemp("", "mrv-index-*")
 	if err != nil {
 		return "", errs.Wrap(errs.E(CodeGit, "scratch index: "+err.Error(), "op", "write-tree"), err)
 	}
-	name := f.Name()
-	_ = f.Close()
-	_ = os.Remove(name)
-	defer os.Remove(name)
-	env := []string{"GIT_INDEX_FILE=" + name}
+	defer os.RemoveAll(dir)
+	env := []string{"GIT_INDEX_FILE=" + filepath.Join(dir, "index")}
 	if _, code, err := g.runEnv(ctx, env, "add", "-A", "--"); err != nil {
 		return "", err
 	} else if code != 0 {
diff --git a/internal/fsm/judge/testdata/prompts/adjudicate.python.txt b/internal/fsm/judge/testdata/prompts/adjudicate.python.txt
new file mode 100644
index 0000000..7a989c1
--- /dev/null
+++ b/internal/fsm/judge/testdata/prompts/adjudicate.python.txt
@@ -0,0 +1,20 @@
+# source: harnesseval@19ff9a8 harnesseval/adjudicate.py:21 sha256=8b7b1897172f2ee101cb363e9d517205edb295b970c8975275b8c01fc64cc784
+You are verifying whether a code review finding identifies a REAL problem in the diff.
+
+Diff (unified):
+```diff
+{diff}
+```
+
+Proposed finding:
+{candidate}
+
+Instructions:
+- Determine if this finding describes a real, verifiable problem present in the diff
+  (a bug, security issue, correctness problem, or a clear defect the code introduces).
+- It is NOT real if it is: a style nit, speculation about code not in the diff, a
+  misreading of the diff, a duplicate of something already fine, or vague/general.
+- Be strict: "real" means a reasonable reviewer would agree the diff has this problem.
+
+Respond with ONLY a JSON object:
+{{"reasoning": "brief explanation grounded in the diff", "is_real": true/false, "confidence": 0.0-1.0}}
\ No newline at end of file
diff --git a/internal/fsm/judge/testdata/prompts/match.python.txt b/internal/fsm/judge/testdata/prompts/match.python.txt
new file mode 100644
index 0000000..9d891f2
--- /dev/null
+++ b/internal/fsm/judge/testdata/prompts/match.python.txt
@@ -0,0 +1,17 @@
+# source: harnesseval@19ff9a8 harnesseval/judge.py:22 sha256=43882bd12af0f8f41cd284ff1dc248919f06895e25cdcd7fd50d7a80c53214fc
+You are evaluating AI code review tools.
+Determine if the candidate issue matches the golden (expected) comment.
+
+Golden Comment (the issue we're looking for):
+{golden_comment}
+
+Candidate Issue (from the tool's review):
+{candidate}
+
+Instructions:
+- Determine if the candidate identifies the SAME underlying issue as the golden comment
+- Accept semantic matches - different wording is fine if it's the same problem
+- Focus on whether they point to the same bug, concern, or code issue
+
+Respond with ONLY a JSON object:
+{{"reasoning": "brief explanation", "match": true/false, "confidence": 0.0-1.0}}
\ No newline at end of file
diff --git a/internal/fsm/judge/testdata/prompts/still-present.python.txt b/internal/fsm/judge/testdata/prompts/still-present.python.txt
new file mode 100644
index 0000000..d4883e9
--- /dev/null
+++ b/internal/fsm/judge/testdata/prompts/still-present.python.txt
@@ -0,0 +1,14 @@
+# source: harnesseval@19ff9a8 harnesseval/sdlc_loop.py:321 sha256=562691a1f484fc034a0e6a9734c3c9621909a2e70a2309755502f7b496763feb
+You are verifying whether a specific bug still exists in the current code.
+
+Original bug description (from a human reviewer):
+{golden_comment}
+
+Current diff (base..HEAD, after fixes were applied):
+```diff
+{repo and _diff(repo, base_ref)[:30000]}
+```
+
+Does the bug described above STILL EXIST in the current code? (True = bug is still present /
+not fixed. False = the bug has been fixed or no longer applies.)
+Respond with ONLY a JSON object: {{"reasoning": "...", "still_present": true/false}}
\ No newline at end of file
diff --git a/internal/fsm/machine/harness_test.go b/internal/fsm/machine/harness_test.go
index 4255251..346c2c9 100644
--- a/internal/fsm/machine/harness_test.go
+++ b/internal/fsm/machine/harness_test.go
@@ -259,17 +259,21 @@ func (c *countingStore) RepairTail(id string) error {
 // ---- runner ----
 
 type fakeRunner struct {
-	calls  []string
-	stdins [][]byte
-	res    converge.CmdResult
-	err    error
-	audit  func(run.Event) error
-	name   string
+	calls    []string
+	stdins   [][]byte
+	res      converge.CmdResult
+	err      error
+	audit    func(run.Event) error
+	ordinal  func(string) int
+	ordinals []int
 }
 
 func (f *fakeRunner) Run(_ context.Context, name string, stdin []byte) (converge.CmdResult, error) {
 	f.calls = append(f.calls, name)
 	f.stdins = append(f.stdins, stdin)
+	if f.ordinal != nil {
+		f.ordinals = append(f.ordinals, f.ordinal(name))
+	}
 	if f.audit != nil {
 		d := run.CmdCallData{Name: name, Argv: []string{"/bin/true"}, InputHash: "x", ExitCode: f.res.ExitCode}
 		if err := f.audit(run.Event{Type: run.TypeCmdCall, Data: run.MarshalCanonical(d)}); err != nil {
@@ -279,6 +283,14 @@ func (f *fakeRunner) Run(_ context.Context, name string, stdin []byte) (converge
 	return f.res, f.err
 }
 
+func (f *fakeRunner) Call(ctx context.Context, name string, stdin []byte, out any) error {
+	res, err := f.Run(ctx, name, stdin)
+	if err != nil {
+		return err
+	}
+	return json.Unmarshal(res.Stdout, out)
+}
+
 // ---- harness ----
 
 type harness struct {
@@ -305,8 +317,9 @@ func newHarness(t *testing.T) *harness {
 	h.deps = Deps{
 		Store: h.store, Sidecar: h.sidecar, Kinds: h.reg,
 		Git: func(dir string) gate.Git { return h.git.get(dir) },
-		Runner: func(_ []run.AllowedCmd, _, _ string, audit func(run.Event) error) converge.Runner {
-			h.runner.audit = audit
+		Runner: func(d RunnerDeps) converge.Caller {
+			h.runner.audit = d.Audit
+			h.runner.ordinal = d.CmdCalls
 			return h.runner
 		},
 		Clock: func() run.Time {
diff --git a/internal/fsm/machine/machine.go b/internal/fsm/machine/machine.go
index 0869197..7a25fc4 100644
--- a/internal/fsm/machine/machine.go
+++ b/internal/fsm/machine/machine.go
@@ -37,10 +37,11 @@ type session struct {
 	log      run.Log
 	w        *workflow.Workflow // resolved
 	pred     converge.Predicate
-	runner   converge.Runner
+	runner   converge.Caller
 	git      gate.Git
 	warns    []string
-	auditErr error // the first store error seen through the audit closure
+	auditErr error          // the first store error seen through the audit closure
+	cmdCalls map[string]int // prior cmd_call count per command name (durable ordinal for mocks)
 	unlock   func()
 }
 
@@ -88,6 +89,9 @@ func Init(ctx context.Context, deps Deps, o InitOptions) (*Machine, error) {
 	if err != nil {
 		return nil, err
 	}
+	if !filepath.IsAbs(o.WorkDir) || !filepath.IsAbs(o.RepoRoot) {
+		return nil, errs.E(CodeWorkdirForeign, "work dir and repo root must be absolute", "reason", "relative", "work_dir", o.WorkDir, "repo_root", o.RepoRoot)
+	}
 	// 2. commands + consent
 	allowed, sha, err := workflow.ResolveCmds(w, o.WorkDir, deps.LookPath, deps.FileHash)
 	if err != nil {
@@ -144,11 +148,14 @@ func Init(ctx context.Context, deps Deps, o InitOptions) (*Machine, error) {
 		if !filepath.IsAbs(dir) {
 			dir = filepath.Join(o.RepoRoot, dir)
 		}
+		rel, inside := relInside(o.RepoRoot, dir)
+		if !inside {
+			return nil, errs.E(CodeMockInvalid, "mock scenario must live inside the repository", "dir", o.MockDir, "reason", "outside")
+		}
 		h, err := deps.MockLoad(dir)
-		if err != nil {
-			return nil, errs.E(CodeMockInvalid, err.Error(), "dir", o.MockDir)
+		if err != nil || len(h) < 16 {
+			return nil, errs.E(CodeMockInvalid, fmt.Sprintf("scenario load failed: %v", err), "dir", o.MockDir)
 		}
-		rel := strings.TrimPrefix(strings.TrimPrefix(filepath.Clean(dir), filepath.Clean(o.RepoRoot)), string(filepath.Separator))
 		mock = rel + "#" + h[:16]
 	}
 	if deps.Kinds.Mock() != (mock != "") {
@@ -200,6 +207,20 @@ func Init(ctx context.Context, deps Deps, o InitOptions) (*Machine, error) {
 	return m, nil
 }
 
+// relInside returns dir relative to root ("." for root itself) and whether
+// dir is inside root. Both must be absolute (Init checks).
+func relInside(root, dir string) (string, bool) {
+	rootC, dirC := filepath.Clean(root), filepath.Clean(dir)
+	if dirC == rootC {
+		return ".", true
+	}
+	prefix := rootC + string(filepath.Separator)
+	if !strings.HasPrefix(dirC, prefix) {
+		return "", false
+	}
+	return strings.TrimPrefix(dirC, prefix), true
+}
+
 // consentList renders the human-readable command list for ERR_CMDS_NOT_ALLOWED.
 func consentList(allowed []run.AllowedCmd, workDir string) string {
 	var b strings.Builder
@@ -347,9 +368,12 @@ func (m *Machine) load(ctx context.Context, repair bool) (*session, error) {
 	}
 	if snap.Mock != "" {
 		i := strings.LastIndex(snap.Mock, "#")
+		if i < 0 {
+			return nil, errs.E(CodeMockMismatch, "malformed mock identity", "mock", snap.Mock)
+		}
 		rel, want := snap.Mock[:i], snap.Mock[i+1:]
 		h, err := deps.MockLoad(filepath.Join(snap.RepoRoot, rel))
-		if err != nil || h[:16] != want {
+		if err != nil || len(h) < 16 || h[:16] != want {
 			return nil, errs.E(CodeMockMismatch, "mock scenario changed or missing", "mock", snap.Mock)
 		}
 	}
@@ -357,7 +381,16 @@ func (m *Machine) load(ctx context.Context, repair bool) (*session, error) {
 		return nil, errs.E(CodeMockMismatch, "the kind registry's mock mode does not match the run")
 	}
 	sess.git = deps.Git(snap.WorkDir)
-	sess.runner = deps.Runner(snap.AllowedCmds, snap.WorkDir, m.runID, sess.audit)
+	sess.cmdCalls = map[string]int{}
+	for _, ev := range log.Events {
+		if ev.Type == run.TypeCmdCall {
+			var cd run.CmdCallData
+			if json.Unmarshal(ev.Data, &cd) == nil {
+				sess.cmdCalls[cd.Name]++
+			}
+		}
+	}
+	sess.runner = deps.Runner(RunnerDeps{Allowed: snap.AllowedCmds, WorkDir: snap.WorkDir, RunID: m.runID, Audit: sess.audit, CmdCalls: func(name string) int { return sess.cmdCalls[name] }})
 	if w.Convergence != nil {
 		sess.pred = converge.MustParse(w.Convergence, sess.runner) // validated by workflow.Parse
 	}
@@ -416,6 +449,12 @@ func (s *session) audit(ev run.Event) error {
 		}
 		return err
 	}
+	if ev.Type == run.TypeCmdCall {
+		var cd run.CmdCallData
+		if json.Unmarshal(ev.Data, &cd) == nil {
+			s.cmdCalls[cd.Name]++
+		}
+	}
 	return nil
 }
 
@@ -529,7 +568,7 @@ func (s *session) advance() (AdvanceResult, error) {
 		if err := s.append(run.TypeTree, tree, ""); err != nil {
 			return AdvanceResult{}, err
 		}
-	case h != snap.TreeHash && (node == nil || node.Kind != "agent-edit"):
+	case h != snap.TreeHash && (node == nil || run.Kind(node.Kind) != run.KindAgentEdit):
 		if snap.RepoMode == "enforcing" {
 			ge := pseudoGate(GateRepoMode, CodeUnsanctionedEdit, "working tree changed outside an agent-edit node:\n"+porcelain)
 			if err := s.gateEvent(GateRepoMode, ge); err != nil {
@@ -573,7 +612,7 @@ func (s *session) runNode(node *workflow.Node, head string) (AdvanceResult, bool
 		diff := Diff{Text: text, Truncated: truncated}
 		if node.Exec == "fork" {
 			ex, _ := s.m.deps.Kinds.Executor(node.Kind)
-			out, err := ex.Execute(s.ctx, ExecInput{Snap: snap, Node: node, Diff: diff, StartIndex: s.st.NextIndex(k), Audit: s.audit})
+			out, err := ex.Execute(s.ctx, ExecInput{Snap: snap, Node: node, Diff: diff, StartIndex: s.st.NextIndex(k), Audit: s.audit, Runner: s.runner})
 			if err != nil {
 				if s.ctx.Err() != nil {
 					return AdvanceResult{}, true, err
@@ -674,7 +713,10 @@ func (s *session) transitions(head string) (AdvanceResult, error) {
 			if err != nil && s.auditErr != nil {
 				return AdvanceResult{}, s.auditErr // the atom's cmd_call could not be stored: abort, not a gate failure
 			}
-			if err != nil || (r.Class == run.OutcomeFixed && r.Atom != "all_fixed") {
+			if err != nil && s.ctx.Err() != nil {
+				return AdvanceResult{}, err // interrupted inside an atom: resumable, never a gate failure
+			}
+			if err != nil || (r.Stop && r.Class == run.OutcomeFixed && r.Atom != "all_fixed") {
 				detail := "convergence evaluation failed"
 				reason := "error"
 				if err != nil {
@@ -691,12 +733,13 @@ func (s *session) transitions(head string) (AdvanceResult, error) {
 				return s.fail(ge, head)
 			}
 			reason, _ := run.CapText(r.Reason, run.MaxText)
-			if err := s.append(run.TypeConverge, run.ConvergeData{Atom: r.Atom, Class: r.Class, Stop: r.Stop, Reason: reason}, ""); err != nil {
+			atom, _ := run.CapText(r.Atom, run.MaxShort)
+			if err := s.append(run.TypeConverge, run.ConvergeData{Atom: atom, Class: r.Class, Stop: r.Stop, Reason: reason}, ""); err != nil {
 				return AdvanceResult{}, err
 			}
 			if r.Stop {
-				chosen = &workflow.Transition{From: snap.State, To: tt.To, Gate: r.Atom, Outcome: r.Class}
-				return s.finish(s.transitionData(*chosen, head), r.Atom+": "+reason)
+				chosen = &workflow.Transition{From: snap.State, To: tt.To, Gate: atom, Outcome: r.Class}
+				return s.finish(s.transitionData(*chosen, head), atom+": "+reason)
 			}
 		}
 		if chosen == nil {
@@ -804,6 +847,9 @@ func (s *session) overflowHandler() error {
 	if s.auditErr != nil {
 		return s.auditErr // the runner's own cmd_call could not be stored: abort, the handler is retried on resume
 	}
+	if s.ctx.Err() != nil {
+		return s.ctx.Err() // interrupted: nothing recorded, the handler is retried on resume
+	}
 	var argv []string
 	for _, c := range snap.AllowedCmds {
 		if c.Name == name {
diff --git a/internal/fsm/machine/machine_test.go b/internal/fsm/machine/machine_test.go
index fcf7690..c371ffa 100644
--- a/internal/fsm/machine/machine_test.go
+++ b/internal/fsm/machine/machine_test.go
@@ -111,6 +111,9 @@ func TestM1InitErrors(t *testing.T) {
 		{"goldens-null-ok", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, GoldensPath: "/x/gnull.json"}, func() { h.files["/x/gnull.json"] = []byte(`null`) }, ""},
 		{"mock-registry-mismatch", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, MockDir: "scen/ok"}, func() { h.mockHash["/repo/scen/ok"] = strings.Repeat("a", 64) }, CodeMockMismatch},
 		{"base-unknown", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "nope"}, nil, gate.CodeGit},
+		{"workdir-relative", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, WorkDir: "rel"}, nil, CodeWorkdirForeign},
+		{"mock-outside-root", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, MockDir: "/other/scen"}, func() { h.mockHash["/other/scen"] = strings.Repeat("b", 64) }, CodeMockInvalid},
+		{"mock-short-hash", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, MockDir: "scen/short"}, func() { h.mockHash["/repo/scen/short"] = "abc" }, CodeMockInvalid},
 		{"workdir-foreign", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, WorkDir: "/elsewhere"}, func() {
 			h.git.byDir["/elsewhere"] = &gate.Fake{HeadSHA: shaHead, Common: "/other/.git"}
 		}, CodeWorkdirForeign},
@@ -141,6 +144,18 @@ func TestM1InitErrors(t *testing.T) {
 	if m.View().Snapshot.Mock != "scen/ok#"+strings.Repeat("a", 16) {
 		t.Fatalf("mock id %q", m.View().Snapshot.Mock)
 	}
+	h.mockHash["/repo"] = strings.Repeat("c", 64)
+	if mr := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, MockDir: "."}); mr.View().Snapshot.Mock != ".#"+strings.Repeat("c", 16) {
+		t.Fatalf("mock at repo root: %q", mr.View().Snapshot.Mock)
+	}
+	if _, err := Open(context.Background(), h.deps, m.runID, OpenOptions{}); err != nil {
+		t.Fatal(err)
+	}
+	h.mockHash["/repo/scen/ok"] = "short"
+	if _, err := Open(context.Background(), h.deps, m.runID, OpenOptions{}); !errs.Is(err, CodeMockMismatch) {
+		t.Fatalf("short hash on open: %v", err)
+	}
+	h.mockHash["/repo/scen/ok"] = strings.Repeat("a", 64)
 	if !h.events(m)[1].Mock {
 		t.Fatal("mock runs stamp Mock on every later event")
 	}
@@ -224,6 +239,14 @@ func TestM2ReviewLoop(t *testing.T) {
 	if v := m.View(); v.NextAction != NextRecord || v.Node.HasOutput {
 		t.Fatalf("view after needs_input: %+v", v)
 	}
+	if r.NeedsInput.Instructions.Input["diff_truncated"] != false {
+		t.Fatal("not truncated")
+	}
+	MaxDiffBytes = 2
+	if r := h.advance(m); r.NeedsInput.Instructions.Input["diff"] != "DI" || r.NeedsInput.Instructions.Input["diff_truncated"] != true {
+		t.Fatalf("truncated diff: %v", r.NeedsInput.Instructions.Input)
+	}
+	MaxDiffBytes = 1 << 20
 	// a second advance and a tokens record do not re-append needs_input
 	h.advance(m)
 	if _, err := m.Record(context.Background(), RecordOptions{Kind: RecordTokens, Data: json.RawMessage(`{"input":7,"output":3}`)}); err != nil {
@@ -272,7 +295,7 @@ func TestM2ReviewLoop(t *testing.T) {
 		t.Fatalf("reviewed sequence:\n%s\n%s", got, want)
 	}
 	ex := h.reg.execs["match-then-adjudicate"]
-	if len(ex.calls) != 1 || ex.calls[0].StartIndex != 0 || ex.calls[0].Diff.Text != "DIFF" || ex.calls[0].Node.Name != "adjudicate" || ex.calls[0].Node.Model != "gpt-5.2" {
+	if len(ex.calls) != 1 || ex.calls[0].StartIndex != 0 || ex.calls[0].Diff.Text != "DIFF" || ex.calls[0].Node.Name != "adjudicate" || ex.calls[0].Node.Model != "gpt-5.2" || ex.calls[0].Runner == nil {
 		t.Fatalf("exec input: %+v", ex.calls)
 	}
 	evs := h.events(m2)
@@ -693,6 +716,9 @@ func TestM4Convergence(t *testing.T) {
 	if !strings.Contains(string(h.runner.stdins[0]), `"vars":{"JUDGE":"sha256:`) {
 		t.Fatal("cmd atom receives the redacted payload")
 	}
+	if len(h.runner.ordinals) != 1 || h.runner.ordinals[0] != 0 {
+		t.Fatalf("first call ordinal 0: %v", h.runner.ordinals)
+	}
 	// converge error
 	h = newHarness(t)
 	wf = sdlcWith(t, h, "custom2.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
@@ -720,12 +746,38 @@ func TestM4Convergence(t *testing.T) {
 	if err != nil {
 		t.Fatal(err)
 	}
-	sess.pred = fixedPred{}
+	sess.pred = fixedPred{stop: true}
 	r, err = sess.advance()
 	sess.unlock()
 	if err != nil || r.Gate == nil || r.Gate.Name != GateConverge || !strings.Contains(r.Gate.Error.Detail, "fixed_class") {
 		t.Fatalf("fixed_class: %+v %v", r, err)
 	}
+	// a NON-firing fixed-class result (any: [all_fixed, …] with bugs remaining) is not a failure: the loop is taken
+	h = newHarness(t)
+	m = loopRun(t, h, 1, "sdlc-loop")
+	sess, _ = m.load(context.Background(), false)
+	sess.pred = fixedPred{stop: false}
+	r, err = sess.advance()
+	sess.unlock()
+	if err != nil || r.Status != StatusAdvanced || r.To != "discover" {
+		t.Fatalf("non-firing fixed class: %+v %v", r, err)
+	}
+	// the real all_fixed atom firing on a confirmed_empty terminal gate → fixed (design §9 example)
+	h = newHarness(t)
+	wf = sdlcWith(t, h, "af.yaml", "  - {from: verify,     to: done,       gate: all_fixed,   outcome: fixed}", "  - {from: verify,     to: done,       gate: confirmed_empty,   outcome: fixed}")
+	h.files["/x/af.yaml"] = []byte(strings.Replace(string(h.files["/x/af.yaml"]), "any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]", "any: [all_fixed, {max_iterations: 5}]", 1))
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	m = h.mustInit(InitOptions{Workflow: wf, Vars: sdlcVars})
+	h.advance(m)
+	h.record(m, "discover", findings(1))
+	h.advance(m)
+	h.advance(m)
+	h.advance(m)
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	h.advance(m)
+	if r := h.advance(m); r.Outcome != run.OutcomeFixed || r.StopReason != "all_fixed: all bugs fixed" {
+		t.Fatalf("all_fixed atom: %+v", r)
+	}
 	// user workflow whose terminal gate is confirmed_empty: convergence still bounds the loop
 	h = newHarness(t)
 	wf = sdlcWith(t, h, "ce.yaml", "  - {from: verify,     to: done,       gate: all_fixed,   outcome: fixed}", "  - {from: verify,     to: done,       gate: confirmed_empty,   outcome: fixed}")
@@ -742,6 +794,33 @@ func TestM4Convergence(t *testing.T) {
 	if r := h.advance(m); r.Outcome != run.OutcomeOverflow {
 		t.Fatalf("bounded even when all fixed: %+v", r)
 	}
+	// Atom cap: an `all` of 32 long-named cmd atoms yields a >1 KB name; it is capped, never a store rejection
+	h = newHarness(t)
+	var names, decls []string
+	for i := 0; i < 32; i++ {
+		n := "cmd-" + strings.Repeat("x", 26) + string(rune('a'+i%16)) // 16 declared names, each referenced twice
+		names = append(names, "{cmd: "+n+"}")
+		if i < 16 {
+			decls = append(decls, "  "+n+": {argv: [bash, -c, echo]}")
+		}
+	}
+	wf = sdlcWith(t, h, "wide.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
+		"  all: ["+strings.Join(names, ", ")+"]\ncmds:\n"+strings.Join(decls, "\n")+"\nrepo_mode: advisory")
+	h.runner.res = converge.CmdResult{Stdout: []byte(`{"stop": true, "reason": "x"}`)}
+	_, sha, _ = workflow.ResolveCmds(mustResolve(t, h, wf), "/repo", h.deps.LookPath, h.deps.FileHash)
+	allPresent(h)
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	m = h.mustInit(InitOptions{Workflow: wf, Vars: sdlcVars, AllowCustomCmds: sha})
+	h.advance(m)
+	h.record(m, "discover", findings(1))
+	h.advance(m)
+	h.advance(m)
+	h.advance(m)
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	h.advance(m)
+	if r := h.advance(m); r.Outcome != run.OutcomeCustom || len(r.StopReason) > run.MaxShort+run.MaxText+8 {
+		t.Fatalf("wide all: %+v", r)
+	}
 	// emitter cap: 5 KB cmd reason capped in converge event and StopReason
 	h = newHarness(t)
 	wf = sdlcWith(t, h, "custom3.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
@@ -776,12 +855,12 @@ func allPresent(h *harness) {
 	}
 }
 
-type fixedPred struct{}
+type fixedPred struct{ stop bool }
 
 func (fixedPred) Name() string       { return "evil" }
 func (fixedPred) Class() run.Outcome { return run.OutcomeFixed }
-func (fixedPred) Evaluate(context.Context, run.Snapshot) (converge.Result, error) {
-	return converge.Result{Stop: true, Atom: "evil", Class: run.OutcomeFixed, Reason: "ha"}, nil
+func (p fixedPred) Evaluate(context.Context, run.Snapshot) (converge.Result, error) {
+	return converge.Result{Stop: p.stop, Atom: "evil", Class: run.OutcomeFixed, Reason: "ha"}, nil
 }
 
 func mustResolve(t *testing.T, h *harness, path string) *workflow.Workflow {
@@ -891,6 +970,9 @@ func TestM4OverflowHandler(t *testing.T) {
 	if r.Status != StatusStopped || r.Outcome != run.OutcomeOverflow || !m3.View().Snapshot.OverflowHandled || countType(h3.events(m3), run.TypeOverflowHandler) != 1 {
 		t.Fatalf("resume handler: %+v", r)
 	}
+	if o := h3.runner.ordinals; len(o) != 2 || o[0] != 0 || o[1] != 0 {
+		t.Fatalf("ordinals: the crashed call stored no cmd_call, so the resume is ordinal 0 again: %v", o)
+	}
 	if _, err := m3.Advance(context.Background()); !errs.Is(err, CodeRunTerminal) {
 		t.Fatal("terminal after resume")
 	}
@@ -1641,7 +1723,7 @@ func TestM9Residue(t *testing.T) {
 		hb.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
 		allPresent(hb)
 		mb := hb.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars})
-		for _, step := range []func(*harness, *Machine) error{adv, rec("discover", findings(1)), adv, adv, adv, rec("fix", `{"commit":"` + shaFix + `","summary":"s"}`), adv, adv} {
+		for _, step := range []func(*harness, *Machine) error{adv, rec("discover", findings(1)), adv, adv, adv, rec("fix", `{"commit":"`+shaFix+`","summary":"s"}`), adv, adv} {
 			if err := step(hb, mb); err != nil {
 				t.Fatal(err)
 			}
@@ -1687,6 +1769,68 @@ func TestM9Residue(t *testing.T) {
 	if _, err := Open(ctx, hj.deps, "mrv-crafted-invalid-torn", OpenOptions{Repair: true}); err == nil || !strings.Contains(err.Error(), "stamp") {
 		t.Fatalf("fold-invalid after repair: %v", err)
 	}
+	// malformed mock identity in a crafted init → ERR_MOCK_MISMATCH, not a panic
+	initJ.RunID = "mrv-crafted-bad-mock"
+	initJ.Mock = "nohash"
+	if _, err := hj.store.Create("mrv-crafted-bad-mock", run.Event{SchemaVersion: run.SchemaVersion, At: initJ.CreatedAt, Type: run.TypeInit, Data: run.MarshalCanonical(initJ)}); err != nil {
+		t.Fatal(err)
+	}
+	_ = hj.deps.Sidecar.Write("mrv-crafted-bad-mock", SidecarWorkflow, raw)
+	hj.reg.mock = true
+	if _, err := Open(ctx, hj.deps, "mrv-crafted-bad-mock", OpenOptions{}); !errs.Is(err, CodeMockMismatch) {
+		t.Fatalf("malformed mock: %v", err)
+	}
+	hj.reg.mock = false
+	// on_overflow interrupted by the parent context: nothing recorded, handler retried on resume
+	ho := newHarness(t)
+	wf := sdlcWith(t, ho, "ov-cancel.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
+		"  any: [{max_iterations: 1}]\ncmds:\n  notify: {argv: [bash, -c, echo]}\non_overflow: notify\nrepo_mode: advisory")
+	_, sha, _ := workflow.ResolveCmds(mustResolve(t, ho, wf), "/repo", ho.deps.LookPath, ho.deps.FileHash)
+	allPresent(ho)
+	ho.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	ho.runner.err = context.Canceled
+	mo := ho.mustInit(InitOptions{Workflow: wf, Vars: sdlcVars, AllowCustomCmds: sha})
+	ho.advance(mo)
+	ho.record(mo, "discover", findings(1))
+	ho.advance(mo)
+	ho.advance(mo)
+	ho.advance(mo)
+	ho.record(mo, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	ho.advance(mo)
+	cctx, ccancel := context.WithCancel(ctx)
+	ccancel()
+	ho.runner.audit = nil
+	if _, err := mo.Advance(cctx); !errors.Is(err, context.Canceled) {
+		t.Fatalf("interrupted handler: %v", err)
+	}
+	if countType(ho.events(mo), run.TypeOverflowHandler) != 0 || mo.View().Snapshot.OverflowHandled {
+		t.Fatal("interrupted handler must not be recorded")
+	}
+	// interrupted inside a cmd atom: returned, never a converge pseudo-gate
+	hc := newHarness(t)
+	wfc := sdlcWith(t, hc, "cv-cancel.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
+		"  any: [{cmd: notify}]\ncmds:\n  notify: {argv: [bash, -c, echo]}\nrepo_mode: advisory")
+	_, shac, _ := workflow.ResolveCmds(mustResolve(t, hc, wfc), "/repo", hc.deps.LookPath, hc.deps.FileHash)
+	allPresent(hc)
+	hc.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	hc.runner.err = context.Canceled
+	mc := hc.mustInit(InitOptions{Workflow: wfc, Vars: sdlcVars, AllowCustomCmds: shac})
+	hc.advance(mc)
+	hc.record(mc, "discover", findings(1))
+	hc.advance(mc)
+	hc.advance(mc)
+	hc.advance(mc)
+	hc.record(mc, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	hc.advance(mc)
+	cctx2, ccancel2 := context.WithCancel(ctx)
+	ccancel2()
+	hc.runner.audit = nil
+	if _, err := mc.Advance(cctx2); !errors.Is(err, context.Canceled) {
+		t.Fatalf("interrupted atom: %v", err)
+	}
+	if mc.View().Snapshot.Outcome != "" {
+		t.Fatal("interrupted atom must not fail the run")
+	}
 	// FS Read with bad args
 	if _, err := (FSSidecar{Root: root}).Read("bad id", "x"); errs.As(err).Field("reason") != "path" {
 		t.Fatal("read bad id")
diff --git a/internal/fsm/machine/sidecar.go b/internal/fsm/machine/sidecar.go
index db8c629..fc0bd52 100644
--- a/internal/fsm/machine/sidecar.go
+++ b/internal/fsm/machine/sidecar.go
@@ -69,7 +69,11 @@ func (f FSSidecar) Write(runID, name string, b []byte) error {
 		return sidecarErr("path", err.Error())
 	}
 	_, werr := fh.Write(b)
-	if err := errors.Join(werr, fh.Close()); err != nil {
+	var serr error
+	if syncer, ok := fh.(interface{ Sync() error }); ok {
+		serr = syncer.Sync()
+	}
+	if err := errors.Join(werr, serr, fh.Close()); err != nil {
 		return sidecarErr("path", err.Error())
 	}
 	return nil
diff --git a/internal/fsm/machine/types.go b/internal/fsm/machine/types.go
index 32377b6..fd01205 100644
--- a/internal/fsm/machine/types.go
+++ b/internal/fsm/machine/types.go
@@ -28,8 +28,8 @@ type Diff struct {
 	Truncated bool
 }
 
-// MaxDiffBytes bounds the diff handed to kinds.
-const MaxDiffBytes = 1 << 20
+// MaxDiffBytes bounds the diff handed to kinds (a variable so tests can force truncation).
+var MaxDiffBytes = 1 << 20
 
 // ExecInput is everything a fork executor gets.
 type ExecInput struct {
@@ -38,6 +38,7 @@ type ExecInput struct {
 	Diff       Diff
 	StartIndex int
 	Audit      func(run.Event) error
+	Runner     converge.Caller // the session's guarded runner (same audit closure and ordinal source)
 }
 
 // NodeKind describes one kind of node work.
@@ -72,13 +73,22 @@ type Sidecar interface {
 	List(runID string) ([]string, error)
 }
 
+// RunnerDeps is what the single Guarded factory receives per session.
+type RunnerDeps struct {
+	Allowed  []run.AllowedCmd
+	WorkDir  string
+	RunID    string
+	Audit    func(run.Event) error
+	CmdCalls func(name string) int // prior cmd_call count for a command (mock ordinal)
+}
+
 // Deps wires the machine. Nothing here is optional except Terminal.
 type Deps struct {
 	Store     run.RunStore
 	Sidecar   Sidecar
 	Kinds     Registry
 	Git       func(workDir string) gate.Git
-	Runner    func(allowed []run.AllowedCmd, workDir, runID string, audit func(run.Event) error) converge.Runner
+	Runner    func(RunnerDeps) converge.Caller
 	Clock     Clock
 	LookPath  func(string) (string, error)
 	FileHash  func(string) (string, error)
diff --git a/internal/fsm/run/types.go b/internal/fsm/run/types.go
index f6bc894..56f1abd 100644
--- a/internal/fsm/run/types.go
+++ b/internal/fsm/run/types.go
@@ -76,6 +76,14 @@ type Golden struct {
 }
 
 // Bug is a confirmed bug (the fix/verify unit).
+// Bug.Verdict vocabulary (design §6).
+const (
+	VerdictMatched       = "matched"
+	VerdictRealButUngold = "real_but_ungold"
+	VerdictHallucination = "hallucination"
+)
+
+// Bug is a confirmed (or rejected) finding.
 type Bug struct {
 	ID         string  `json:"id"`
 	Desc       string  `json:"desc"`
diff --git a/internal/fsm/workflow/resolve.go b/internal/fsm/workflow/resolve.go
index 8c22ca6..82b2795 100644
--- a/internal/fsm/workflow/resolve.go
+++ b/internal/fsm/workflow/resolve.go
@@ -59,6 +59,11 @@ func (w *Workflow) Resolve(vars map[string]string, calibration bool) (*Workflow,
 	for s, n := range w.Nodes {
 		nn := *n
 		nn.Model, nn.Effort = sub(n.Model), sub(n.Effort)
+		for _, v := range []string{nn.Model, nn.Effort} {
+			if _, over := run.CapText(v, run.MaxShort); over {
+				return nil, nil, invalid("bad_var", "nodes."+n.Name, "resolved model/effort exceeds MaxShort")
+			}
+		}
 		nn.Params = make(map[string]any, len(n.Params))
 		for k, v := range n.Params {
 			switch x := v.(type) {
@@ -170,9 +175,10 @@ func VerifyCmds(allowed []run.AllowedCmd, workDir string, hash func(string) (str
 	return nil
 }
 
-// FileSHA256 hashes a regular file; directories and missing paths error.
+// FileSHA256 hashes a regular file, following symlinks (a symlinked script is
+// pinned by its target's contents); directories and missing paths error.
 func FileSHA256(path string) (string, error) {
-	st, err := os.Lstat(path)
+	st, err := os.Stat(path)
 	if err != nil {
 		return "", err
 	}
diff --git a/internal/fsm/workflow/testdata/cmds-preimage.sha256 b/internal/fsm/workflow/testdata/cmds-preimage.sha256
index 9e5ebc0..529e116 100644
--- a/internal/fsm/workflow/testdata/cmds-preimage.sha256
+++ b/internal/fsm/workflow/testdata/cmds-preimage.sha256
@@ -1 +1 @@
-d5f22e88404dc2366f7168b75ad299af15f45c53068f0abe4fe39c901e6804e3
+76d37fa8f97d1468432efdc97ba42dc770cbbe032bcf72ca31811af60f2b0f53
diff --git a/internal/fsm/workflow/workflow.go b/internal/fsm/workflow/workflow.go
index f6052ed..58d2e48 100644
--- a/internal/fsm/workflow/workflow.go
+++ b/internal/fsm/workflow/workflow.go
@@ -166,7 +166,10 @@ func Parse(raw []byte, opts Options) (*Workflow, error) {
 	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
 	dec.KnownFields(true)
 	if err := dec.Decode(&rw); err != nil {
-		return nil, invalid("unknown_key", "document", err.Error())
+		if strings.Contains(err.Error(), "not found in type") {
+			return nil, invalid("unknown_key", "document", err.Error())
+		}
+		return nil, invalid("bad_yaml", "document", err.Error())
 	}
 	sum := sha256.Sum256(raw)
 	w := &Workflow{
diff --git a/internal/fsm/workflow/workflow_test.go b/internal/fsm/workflow/workflow_test.go
index 15d9423..f07cb29 100644
--- a/internal/fsm/workflow/workflow_test.go
+++ b/internal/fsm/workflow/workflow_test.go
@@ -175,6 +175,10 @@ func TestW2Reasons(t *testing.T) {
 		src              string
 	}{
 		{"unknown-top-key", "unknown_key", "document", example + "extra: 1\n"},
+		{"bad-yaml-malformed", "bad_yaml", "document", "workflow: [\n"},
+		{"bad-yaml-dup-key", "bad_yaml", "document", example + "workflow: again\n"},
+		{"bad-yaml-scalar-root", "bad_yaml", "document", "just a string\n"},
+		{"bad-version-string", "bad_yaml", "document", edit("version: 1", "version: one")},
 		{"transitions-scalar", "unknown_key", "transitions", scalarTransitions()},
 		{"node-kind-not-string", "unknown_key", "nodes.fix.kind", edit("fix:        { kind: agent-edit }", "fix:        { kind: [agent-edit] }")},
 		{"transition-unknown-field", "unknown_key", "transitions[0]", edit("gate: findings_empty, outcome: clean }", "gate: findings_empty, outcome: clean, extra: 1 }")},
@@ -299,6 +303,51 @@ transitions:
   "*→failed": { on: gate_error }
 `
 
+func TestW2ReservedEnvAndBoundaries(t *testing.T) {
+	for _, name := range []string{"PATH", "HOME", "LANG", "TMPDIR", "BASH_ENV", "ENV", "PYTHONPATH", "PYTHONSTARTUP", "PYTHONHOME", "NODE_OPTIONS", "NODE_PATH", "PERL5OPT", "PERL5LIB", "RUBYOPT", "RUBYLIB", "JAVA_TOOL_OPTIONS", "SHELLOPTS", "PS4", "IFS", "CDPATH", "GLOBIGNORE", "PROMPT_COMMAND", "MRV_RUN_ID", "MRV_X", "LD_PRELOAD", "LD_LIBRARY_PATH", "DYLD_INSERT_LIBRARIES", "GIT_DIR", "GIT_CONFIG_COUNT"} {
+		if r, _ := reasonOf(t, edit("env: [SLACK_WEBHOOK]", "env: ["+name+"]")); r != "bad_env" {
+			t.Errorf("reserved %s: %s", name, r)
+		}
+	}
+	// acceptance boundaries
+	for _, ok := range []string{edit("timeout: 30", "timeout: 1"), edit("timeout: 30", "timeout: 3600"), renameState(strings.Repeat("s", 32)), edit("env: [SLACK_WEBHOOK]", "env: [A1, A2, A3, A4, A5, A6, A7, A8, A9, A10, A11, A12, A13, A14, A15, A16]")} {
+		w := mustParse(t, ok)
+		_ = w
+	}
+	if w := mustParse(t, edit("timeout: 30", "timeout: 3600")); w.Cmds["notify"].Timeout != 3600*time.Second {
+		t.Fatal("3600 accepted")
+	}
+	if r, _ := reasonOf(t, renameState(strings.Repeat("s", 33))); r != "bad_state" {
+		t.Fatal("33-char state refused")
+	}
+	var vars []string
+	for i := 0; i < run.MaxVars-3; i++ {
+		vars = append(vars, "V"+strings.Repeat("A", i+1)+": {default: x}")
+	}
+	mustParse(t, edit("vars: { JUDGE: {required: true}, JUDGE_EFFORT: {required: true}, REVIEWER: {default: claude-opus-5} }", "vars: { JUDGE: {required: true}, JUDGE_EFFORT: {required: true}, REVIEWER: {default: claude-opus-5}, "+strings.Join(vars, ", ")+" }"))
+	var cmds []string
+	for i := 0; i < run.MaxAllowedCmds-1; i++ {
+		cmds = append(cmds, "  c"+strings.Repeat("x", i)+": { argv: [a] }")
+	}
+	mustParse(t, edit("  notify: { argv: [bash, ./scripts/notify.sh, --model, $JUDGE], timeout: 30, env: [SLACK_WEBHOOK] }", "  notify: { argv: [bash] }\n"+strings.Join(cmds, "\n")))
+	mustParse(t, edit("argv: [bash, ./scripts/notify.sh, --model, $JUDGE]", "argv: ["+strings.Repeat("a, ", run.MaxArgv-1)+"a]"))
+	// caller beats Default; $1 / ${X} left literal; nested maps not walked
+	w := mustParse(t, edit("lenses: 8 }", "lenses: 8, note: '$1 ${JUDGE} $', deep: {model: $JUDGE} }"))
+	r, eff, err := w.Resolve(map[string]string{"JUDGE": "j", "JUDGE_EFFORT": "e", "REVIEWER": "override"}, false)
+	if err != nil || eff["REVIEWER"] != "override" || r.Nodes["discover"].Model != "override" {
+		t.Fatalf("caller beats default: %v %v", err, eff)
+	}
+	if r.Nodes["discover"].Params["note"] != "$1 ${JUDGE} $" || r.Nodes["discover"].Params["deep"].(map[string]any)["model"] != "$JUDGE" {
+		t.Fatalf("literal/nested: %v", r.Nodes["discover"].Params)
+	}
+}
+
+// renameState renames the adjudicate state (not the kind) in the example.
+func renameState(name string) string {
+	s := strings.ReplaceAll(example, "adjudicate", name)
+	return strings.ReplaceAll(s, "match-then-"+name, "match-then-adjudicate")
+}
+
 func TestW2Warnings(t *testing.T) {
 	src := edit("  - { from: discover, to: done, gate: findings_empty, outcome: clean }\n", "")
 	src = strings.Replace(src, "  - { from: adjudicate, to: done, gate: confirmed_empty, outcome: clean }\n", "", 1)
@@ -335,6 +384,9 @@ func TestW3Resolve(t *testing.T) {
 		t.Fatalf("params: %v", r2.Nodes["discover"].Params)
 	}
 	// errors
+	if _, _, err := w.Resolve(map[string]string{"JUDGE": strings.Repeat("m", run.MaxShort+1), "JUDGE_EFFORT": "b"}, false); !errs.Is(err, CodeWorkflowInvalid) || errs.As(err).Field("reason") != "bad_var" {
+		t.Fatalf("over-long model: %v", err)
+	}
 	if _, _, err := w.Resolve(map[string]string{"JUDGE": "a"}, false); !errs.Is(err, CodeVarUnset) || errs.As(err).Field("name") != "JUDGE_EFFORT" {
 		t.Fatalf("unset: %v", err)
 	}
@@ -429,11 +481,22 @@ func TestW4ResolveCmds(t *testing.T) {
 	if string(canon) != strings.TrimSpace(strings.ReplaceAll(string(fixture), "WORK", work)) {
 		t.Fatalf("preimage drift:\n%s\n%s", canon, fixture)
 	}
-	// the sha over the WORK-substituted fixture must equal CmdsSHA256 (the .sha256 file pins the fixture with WORK=/w)
-	if sha != CmdsSHA256(allowed) {
-		t.Fatal("ResolveCmds sha == CmdsSHA256")
+	// the pinned .sha256 (WORK=/w) must equal CmdsSHA256 of the list resolved in /w through the fakes
+	hashesW := map[string]string{"/bin/bash": "hb", "/w/scripts/notify.sh": "hs"}
+	lookW := func(name string) (string, error) {
+		switch name {
+		case "bash":
+			return "/bin/bash", nil
+		case "/w/scripts/notify.sh":
+			return name, nil
+		}
+		return "", errors.New("nf")
+	}
+	allowedW, shaW, err := ResolveCmds(r, "/w", lookW, hashFor2(hashesW))
+	if err != nil || shaW != wantSha || CmdsSHA256(allowedW) != wantSha {
+		t.Fatalf("pinned preimage sha: %s vs %s (%v)", shaW, wantSha, err)
 	}
-	sum := sha256.Sum256([]byte(strings.TrimSpace(string(fixture))))
+	sum := sha256.Sum256([]byte(strings.ReplaceAll(strings.TrimSpace(string(fixture)), "WORK", "/w")))
 	if hex.EncodeToString(sum[:]) != wantSha {
 		t.Fatalf("fixture sha256 %s != pinned %s", hex.EncodeToString(sum[:]), wantSha)
 	}
@@ -482,6 +545,19 @@ func TestW4ResolveCmds(t *testing.T) {
 	if err := VerifyCmds(allowed, work, hash); !errs.Is(err, CodeCmdChanged) || errs.As(err).Field("reason") != "appeared" || errs.As(err).Field("path") != filepath.Join(work, "--model") {
 		t.Fatalf("appeared: %v", err)
 	}
+	// symlinked script: hashed through the link (target contents), keyed by the argv path
+	link := filepath.Join(work, "scripts", "link.sh")
+	if err := os.Symlink(script, link); err != nil {
+		t.Fatal(err)
+	}
+	if h, err := FileSHA256(link); err != nil || h != hexSum("#!/bin/sh\n") {
+		t.Fatalf("symlink hashed through: %s %v", h, err)
+	}
+	dirLink := filepath.Join(work, "dirlink")
+	_ = os.Symlink(work, dirLink)
+	if _, err := FileSHA256(dirLink); !errs.Is(err, CodeCmdChanged) {
+		t.Fatal("symlink to a directory is irregular")
+	}
 	// FileSHA256: regular, directory, missing
 	h, err := FileSHA256(script)
 	if err != nil || h != hexSum("#!/bin/sh\n") {
@@ -504,6 +580,15 @@ func TestW4ResolveCmds(t *testing.T) {
 	}
 }
 
+func hashFor2(m map[string]string) func(string) (string, error) {
+	return func(p string) (string, error) {
+		if h, ok := m[p]; ok {
+			return h, nil
+		}
+		return "", errors.New("no such file")
+	}
+}
+
 func hexSum(s string) string {
 	sum := sha256.Sum256([]byte(s))
 	return hex.EncodeToString(sum[:])


--- docs/tasks/m1-m6-fsm-packages.md
+# M1–M6: internal/fsm core packages
+
+Implement `internal/fsm/{errs,converge,gate,workflow,machine,cmdexec,judge,mockai,kind}` per
+`docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md` (r4) and `docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md`
+(r5), test-first, under the combined coverage gate (`tests/coverage.sh`), reviewed per commit range (≤ 120 KB each).
+
+## Acceptance
+
+- Every §7/§8 test row has a discriminating test (literal pins; goldens regression-only behind an env flag).
+- `go test ./internal/fsm/...` passes; every `internal/fsm/*` package at exactly 100% statements.
+- `bash tests/coverage.sh` passes (legacy floor held).
+- Dependency direction per spec 2 §1 (machine imports no kinds/judge/cmdexec/workflows).
+- Every LLM/shell effect behind an interface; no shell, pinned argv, exact env in `cmdexec`.
````

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

coverage gate run after commit 1d6284b (M4/M5/M6 complete):
internal/markdown                                   70.0%  ok
internal/prready                                    85.7%  ok
internal/repo                                       87.9%  ok
internal/reviewers                                  97.2%  ok
internal/reviewlog                                  90.2%  ok
internal/reviewmanifest                             90.5%  ok
internal/reviewstate                                92.1%  ok
internal/runchain                                   90.1%  ok
internal/sessionhistory                             86.2%  ok
internal/setup                                      88.5%  ok
internal/state                                      81.6%  ok
internal/taskdone                                   87.0%  ok
internal/tasksource                                 79.2%  ok
workflows                                          100.0%  ok
coverage gate passed

[exited with code 0]

