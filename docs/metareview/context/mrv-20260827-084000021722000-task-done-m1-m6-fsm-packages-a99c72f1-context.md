# metareview task-done context

Run ID: `mrv-20260827-084000021722000-task-done-m1-m6-fsm-packages-a99c72f1`

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

- Base: `a9f613bc09d1f340ff26014d3c06bd1e974c14c6`
- Head: `37449aaa6c8469d63428dd1d5b51f26780b33722`
- Branch: ``
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `48955`
- Filtered diff bytes: `43001`
- Risk level: `none`
- Generated files excluded: docs/metareview/reviews/mrv-20260827-073257644607000-artifact-2026-08-27-metareview-0-9-0-fsm-core-a0b8592f.md, docs/metareview/reviews/mrv-20260827-073257743851000-artifact-2026-08-27-metareview-0-9-0-fsm-judge-kinds-33d63bfb.md



## Review Manifest

- Manifest verdict: `NEEDS_REVISION`
- Source manifest hash: `a3e01418af9ab95f`
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- .gitattributes
- docs/0.8.0-candidates.md
- docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md
- docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md
- docs/tasks/m1-m6-fsm-packages.md
- go.mod
- go.sum
- internal/fsm/gate/git.go
- internal/fsm/judge/testdata/prompts/adjudicate.python.txt
- internal/fsm/judge/testdata/prompts/match.python.txt
- internal/fsm/judge/testdata/prompts/still-present.python.txt
- internal/fsm/machine/machine_test.go
- internal/fsm/machine/types.go
- internal/fsm/workflow/resolve.go
- internal/fsm/workflow/testdata/cmds-preimage.sha256
- internal/fsm/workflow/workflow_test.go

### Path Dispositions
- docs/metareview/reviews/mrv-20260827-073257644607000-artifact-2026-08-27-metareview-0-9-0-fsm-core-a0b8592f.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260827-073257743851000-artifact-2026-08-27-metareview-0-9-0-fsm-judge-kinds-33d63bfb.md: generated (metareview generated review artifact excluded from source manifest)

### Shards
- shard-01: .gitattributes, docs/0.8.0-candidates.md, docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md, docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md, docs/tasks/m1-m6-fsm-packages.md, go.mod, go.sum, internal/fsm/gate/git.go, internal/fsm/judge/testdata/prompts/adjudicate.python.txt, internal/fsm/judge/testdata/prompts/match.python.txt, internal/fsm/judge/testdata/prompts/still-present.python.txt, internal/fsm/machine/machine_test.go, internal/fsm/machine/types.go, internal/fsm/workflow/resolve.go, internal/fsm/workflow/testdata/cmds-preimage.sha256, internal/fsm/workflow/workflow_test.go

### Manifest Blockers
- missing shard result for shard-01

## Changed Files

- .gitattributes
- docs/0.8.0-candidates.md
- docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md
- docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md
- go.mod
- go.sum
- internal/fsm/gate/git.go
- internal/fsm/judge/testdata/prompts/adjudicate.python.txt
- internal/fsm/judge/testdata/prompts/match.python.txt
- internal/fsm/judge/testdata/prompts/still-present.python.txt
- internal/fsm/machine/machine_test.go
- internal/fsm/machine/types.go
- internal/fsm/workflow/resolve.go
- internal/fsm/workflow/testdata/cmds-preimage.sha256
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
index 2b04a29..f42214b 100644
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
@@ -172,7 +172,8 @@ argv element (including the rewritten `argv[0]`) that names an existing regular
 FileHashes, TimeoutMS, Env}` (`Env` nil when none — `omitempty` keeps the preimage stable). **Preimage:**
 `run.MarshalCanonical([]AllowedCmd sorted by name)` (escape-off encoder, the one every fsm struct→JSON path uses) → sha256
 hex; independent of declaration order (W4). `hash(path)` returns an error for missing or non-regular files (that is how
-"names an existing regular file" is decided; `workflow.FileSHA256` is the real one). The runner
+"names an existing regular file" is decided; `workflow.FileSHA256` is the real one and follows symlinks, so a symlinked
+script is pinned by its target's contents under the argv path). The runner
 executes `AllowedCmd.Argv` verbatim. `VerifyCmds(allowed /* = snap.AllowedCmds, never re-resolved */, workDir, hash)`:
 each pinned hash → `ERR_CMD_CHANGED{path, reason: mismatch|missing}`; an argv element that now resolves to a regular file
 without a `FileHashes` entry → `ERR_CMD_CHANGED{path, reason: appeared}`. Ledger: absolute paths make the sha per-machine
@@ -233,7 +234,7 @@ returns the first firing child's `Result`; `all` fires only when every child fir
 ```go
 type Instructions struct { Text string; Input map[string]any; Untrusted []string; OutputSchema json.RawMessage }
 type Diff struct { Text string; Truncated bool }                                  // Git.Diff(BaseSHA, head, MaxDiffBytes = 1<<20)
-type ExecInput struct { Snap run.Snapshot; Node *workflow.Node; Diff Diff; StartIndex int /* st.NextIndex(key) */; Audit func(run.Event) error }
+type ExecInput struct { Snap run.Snapshot; Node *workflow.Node; Diff Diff; StartIndex int /* st.NextIndex(key) */; Audit func(run.Event) error; Runner converge.Caller /* the session's guarded runner: same audit closure, same ordinal */ }
 type NodeKind interface {
     Name() string; Info() workflow.KindInfo
     Instructions(snap run.Snapshot, node *workflow.Node, diff Diff, nonce string) (Instructions, error)
@@ -262,7 +263,7 @@ returns unchanged so the next `Advance` resumes from `StartIndex`.
 ```go
 type Deps struct {
     Store run.RunStore; Sidecar Sidecar; Kinds Registry; Git func(workDir string) gate.Git
-    Runner func(RunnerDeps) converge.Runner   // RunnerDeps{Allowed, WorkDir, RunID, Audit, CmdCalls func(name) int /* prior cmd_call count: the mock ordinal */}
+    Runner func(RunnerDeps) converge.Caller   // RunnerDeps{Allowed, WorkDir, RunID, Audit, CmdCalls func(name) int}; the ONE guarded factory. CmdCalls = number of cmd_call events for that name in the log so far (any context: atom, on_overflow, cmd kind; copied Origin events included), incremented on every successful cmd_call append — the durable mock ordinal
     Clock Clock; LookPath func(string) (string, error); FileHash func(string) (string, error)
     Workflows func(name string) ([]byte, error); ReadFile func(string) ([]byte, error); Nonce func() string
     MockLoad func(dir string) (hash string, err error); Terminal func(ctx, View) error   // spec 3's runs.jsonl record; nil → no-op
@@ -316,8 +317,8 @@ type NodeView struct { Name, Kind, Exec string; HasOutput, Applied bool }
    binary surfaces here as `ERR_WORKFLOW_INVALID` — the "older-binary run" signal; `list`/`export` still work);
    `Resolve(snap.Vars, false)`; `VerifyCmds(snap.AllowedCmds, WorkDir, FileHash)`; `Mock`: split on the **last** `#`, resolve
    `rel` against `snap.RepoRoot`, `MockLoad` hash vs stored, `Kinds.Mock() == (snap.Mock != "")`, else `ERR_MOCK_MISMATCH`;
-   build `runner ← Deps.Runner(snap.AllowedCmds, WorkDir, runID, audit)` and, when `w.Convergence != nil`, `pred ←
-   converge.Parse(w.Convergence, runner)`.
+   count `cmd_call` events per name; build `runner ← Deps.Runner(RunnerDeps{…})` and, when `w.Convergence != nil`, `pred ←
+   converge.MustParse(w.Convergence, runner)` (validated at Parse; cannot fail).
 `Advance` and `Record` re-run steps 1–2 under their own lock (no cached state is trusted across calls).
 
 ### 5.4 `Advance`
@@ -352,7 +353,8 @@ type NodeView struct { Name, Kind, Exec string; HasOutput, Applied bool }
        tt ← w.TerminalFor(state); err ← gate(tt.Gate)(snap); append gate; pass → chosen ← tt else failures += err
        if chosen == nil:
            r, err ← pred.Evaluate(snap)                                        (always when tt failed — bounded loops)
-           err → append gate{Name:"converge", Passed:false, Error:{ERR_CONVERGE_FAILED, Detail}} → step 8
+           err with the audit closure having failed → return that store error; err with ctx cancelled → return ctx.Err() (resumable);
+           other err → append gate{Name:"converge", Passed:false, Error:{ERR_CONVERGE_FAILED, Detail}} → step 8
            r.Stop && r.Class == fixed && r.Atom != "all_fixed" → same, ERR_CONVERGE_FAILED{reason: fixed_class}   (defense in depth; the real
            all_fixed atom firing is legitimate — design §9's example — and a non-firing result's class is irrelevant)
            append converge{Atom: CapText(r.Atom, MaxShort), Class: r.Class, Stop: r.Stop, Reason: CapText(MaxText)}
@@ -364,6 +366,7 @@ type NodeView struct { Name, Kind, Exec string; HasOutput, Applied bool }
    first.Gate, Outcome: failed, Head: head}; Deps.Terminal(ctx, View()); return GATE_FAILED with Gate = first, exit 1
 9  append transition{From, To, Gate, Outcome, Loop, ToKind, Head}
 9b if Outcome == overflow && OnOverflow != "" && !OverflowHandled: res, err ← runner.Run(OnOverflow, converge.Payload(snap));
+   audit failure → return it; ctx cancelled → return ctx.Err() (nothing recorded, retried on resume);
    append overflow_handler{Name, Argv: snap.AllowedCmds[name].Argv, InputHash: sha256(payload), Stdout/Stderr: CapText(MaxDetail/MaxStderr)+flags,
    ExitCode (−1 when err), DurationMS, Error: code}; err or exit≠0 → also warn{OVERFLOW_HANDLER_FAILED}   (at-least-once: a crash between the
    runner's cmd_call and this append re-runs the command)
@@ -424,7 +427,9 @@ behind `FSM_MACHINE_UPDATE_GOLDEN=1` with the run package's "drift ≠ regenerat
 - Loop boundary is order-independent: `TerminalFor` gate first, convergence only when `!AllFixed`, then the loop gate and remaining transitions (C3 gate-first, made structural).
 - Converge errors are the `converge` pseudo-gate; enforcing edits, executor and decode failures are `repo_mode`/`executor`/`node_output` pseudo-gates; the failed transition names the first failing gate.
 - `needs_input` once per key; `tree` at `Init`, as a baseline when `TreeHash == ""`, and on change (content-aware `WorkTree`; agent-edit states may emit one per advance while the agent edits — accepted).
-- `commit_exists` = `FixEntryHead..HEAD` + `ERR_GATE_INAPPLICABLE` (SCP3-5); Git failures inside gates are gate errors (recovery by fork) — accepted.
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
diff --git a/internal/fsm/machine/machine_test.go b/internal/fsm/machine/machine_test.go
index aab74a0..c371ffa 100644
--- a/internal/fsm/machine/machine_test.go
+++ b/internal/fsm/machine/machine_test.go
@@ -239,6 +239,14 @@ func TestM2ReviewLoop(t *testing.T) {
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
diff --git a/internal/fsm/machine/types.go b/internal/fsm/machine/types.go
index 9e87efe..fd01205 100644
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
diff --git a/internal/fsm/workflow/resolve.go b/internal/fsm/workflow/resolve.go
index fa25ea7..82b2795 100644
--- a/internal/fsm/workflow/resolve.go
+++ b/internal/fsm/workflow/resolve.go
@@ -175,9 +175,10 @@ func VerifyCmds(allowed []run.AllowedCmd, workDir string, hash func(string) (str
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
diff --git a/internal/fsm/workflow/workflow_test.go b/internal/fsm/workflow/workflow_test.go
index 1d7c438..f07cb29 100644
--- a/internal/fsm/workflow/workflow_test.go
+++ b/internal/fsm/workflow/workflow_test.go
@@ -303,6 +303,51 @@ transitions:
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
@@ -436,11 +481,22 @@ func TestW4ResolveCmds(t *testing.T) {
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
@@ -489,6 +545,19 @@ func TestW4ResolveCmds(t *testing.T) {
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
@@ -511,6 +580,15 @@ func TestW4ResolveCmds(t *testing.T) {
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

