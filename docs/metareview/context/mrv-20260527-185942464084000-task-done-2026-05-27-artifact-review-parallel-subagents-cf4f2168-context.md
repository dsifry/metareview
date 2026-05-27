# metareview task-done context

Run ID: `mrv-20260527-185942464084000-task-done-2026-05-27-artifact-review-parallel-subagents-cf4f2168`

## Task

# Artifact Review Parallel Subagent Default

## Problem

The 0.2.0 artifact-review gate fails closed on incomplete scaffolds, but the process directions still leave room for one agent to treat the five reviewer lenses as a self-review. That weakens the adversarial review guarantee that prompted the gate.

## Requirements

- Artifact review means five independent reviewer lenses: Feasibility, Completeness, Scope and alignment, Architecture, and Intent preservation.
- The artifact-review workflow itself is explicit authorization to delegate those reviewer lenses.
- When subagents are available, run the five reviewer lenses as parallel subagents by default.
- When subagents are unavailable, or the human explicitly requests no delegation, record the execution mode as `in-session-emulated`.
- In-session emulation must state that the review is not independently adversarial and should be treated as weaker evidence.
- New artifact review scaffolds must not pre-label the review as `in-session-emulated`; the scaffold should start in a pending/delegation-intended mode and instruct agents to update the mode after real reviewer execution.
- A review execution is incomplete while required reviewer rows are empty, a reviewer lacks a verdict, or the aggregate verdict is `NOT_REVIEWED`.
- The artifact review must report the actual artifact-review verdict returned by the reviewer set, including `NEEDS_REVISION` or `ESCALATE` when that is what the review found. It must not force a deterministic example verdict; downstream readiness claims are what require zero unresolved blockers or explicit human acceptance.

## Non-Goals

- Do not implement LLM or subagent orchestration inside the Go CLI in this slice.
- Do not change deterministic task-done, epic-ready, or pr-ready reviewer logic.
- Do not require a specific final verdict from every artifact review run.

## Acceptance

- `skills/review-artifact/SKILL.md` states that parallel subagents are the default artifact-review execution mode and preserves the named five-lens set: Feasibility, Completeness, Scope and alignment, Architecture, and Intent preservation.
- The skill states that artifact-review invocation counts as explicit authorization for delegation.
- The fallback mode is named `in-session-emulated`, limited to unavailable subagents or explicit human no-delegation requests, marked not independently adversarial, and treated as weaker evidence.
- Go and JavaScript artifact-review scaffold generators no longer hardcode `in-session-emulated` as the initial execution mode; they start in pending/delegation-intended mode and instruct agents to update execution mode after real reviewer execution.
- The skill says review execution completion requires populated required reviewer rows, per-reviewer verdicts, and an aggregate verdict other than `NOT_REVIEWED`; artifact readiness still requires zero unresolved blocking findings unless explicitly human-accepted.
- Quickstart and agent integration docs mention the subagent default, the unavailable-subagent or human-no-delegation fallback trigger, and the weaker-evidence caveat.
- The skill tells agents to return the actual artifact-review verdict instead of substituting a fixed example result.
- `bash tests/manifest/test-skills.sh`, `bash tests/go/test-artifact-review.sh`, and `bash tests/lib/test-artifact-review.sh` assert the new delegation, fallback, scaffold, completion, and actual-verdict contract text.


## Git

- Base: `9ce6ef27be6e8d841363bd861cea4f09a63ea767`
- Head: `9ce6ef27be6e8d841363bd861cea4f09a63ea767`
- Branch: `main`
- Gate effect: `gate`

## Changed Files

- INSTALL.md
- README.md
- commands/review-artifact.md
- docs/README.claude.md
- docs/README.codex.md
- docs/index.html
- docs/quickstart.md
- internal/artifactreview/review.go
- lib/artifact-review.js
- skills/review-artifact/SKILL.md
- tests/go/test-artifact-review.sh
- tests/lib/test-artifact-review.sh
- tests/manifest/test-skills.sh
- docs/specs/2026-05-27-artifact-review-parallel-subagents.md

## Diff

````diff


diff --git a/INSTALL.md b/INSTALL.md
index a784707..e54eab3 100644
--- a/INSTALL.md
+++ b/INSTALL.md
@@ -101,7 +101,7 @@ metareview setup --check
 metareview review artifact docs/quickstart.md
 ```
 
-Artifact reviews create a Markdown review scaffold under `docs/metareview/reviews/` with an initial `NOT_REVIEWED` verdict. The default artifact command exits nonzero because the scaffold is not a completed review. Use `--scaffold-only` only when you explicitly want scaffold creation without passing the gate. Deterministic lifecycle gates such as `task-done`, `epic-ready`, and `pr-ready` report `PASS`, `PASS_ADVISORY`, or blocking findings. Treat every blocking finding and every `NOT_REVIEWED` artifact as open work until a re-review clears it.
+Artifact reviews create a Markdown review scaffold under `docs/metareview/reviews/` with an initial `NOT_REVIEWED` verdict. The default artifact command exits nonzero because the scaffold is not a completed review. Artifact review runs the required lenses as parallel subagents by default; use `in-session-emulated` only when subagents are unavailable or the human requests no delegation, and mark that result as not independently adversarial and weaker evidence. Use `--scaffold-only` only when you explicitly want scaffold creation without passing the gate. Deterministic lifecycle gates such as `task-done`, `epic-ready`, and `pr-ready` report `PASS`, `PASS_ADVISORY`, or blocking findings. Treat every blocking finding and every `NOT_REVIEWED` artifact as open work until a re-review clears it.
 
 ## Agent Workflow
 
diff --git a/README.md b/README.md
index 7e3f5cb..8676b03 100644
--- a/README.md
+++ b/README.md
@@ -171,7 +171,7 @@ flowchart TD
 
 The decomposition loop is intentionally fractal: a parent plan can be decomposed into child epics, each child can be decomposed again, and each level gets reviewed before implementation continues. After the iteration converges, metareview checks back against the original parent intent so accumulated local fixes do not quietly drift away from the user request.
 
-Every review produces Markdown artifacts under `docs/metareview/` and local transient state under `.metareview/`. A blocking finding is current work. A `NOT_REVIEWED` artifact scaffold is also current work, not a pass. Fix it, re-run with `--previous-run <run-id>`, and do not claim completion until the review reports `PASS` or `PASS_ADVISORY` with zero blockers.
+Every review produces Markdown artifacts under `docs/metareview/` and local transient state under `.metareview/`. A blocking finding is current work. A `NOT_REVIEWED` artifact scaffold is also current work, not a pass. Artifact review runs the five required lenses as parallel subagents by default; `in-session-emulated` fallback is weaker evidence and must say the review is not independently adversarial. Fix blockers, re-run with `--previous-run <run-id>`, and do not claim completion until the review reports `PASS` or `PASS_ADVISORY` with zero blockers.
 
 ## How Humans Use It
 
@@ -185,7 +185,7 @@ metareview review pr-ready --base main
 metareview learn --post-merge 42 --base pre-merge-sha
 ```
 
-Use the smallest gate that matches the decision you are making. If you are deciding whether a plan is good enough, use `artifact`; the default command creates a `NOT_REVIEWED` scaffold and exits nonzero until the required reviewer rows and final verdict are completed. Use `--scaffold-only` only for explicit scaffold generation. If you are deciding whether a task is done, use `task-done`. If you are deciding whether a branch is ready, use `pr-ready`.
+Use the smallest gate that matches the decision you are making. If you are deciding whether a plan is good enough, use `artifact`; the default command creates a `NOT_REVIEWED` scaffold and exits nonzero until the required reviewer rows and final verdict are completed. The reviewer set should return the actual artifact-review verdict it finds, not a fixed example result. Use `--scaffold-only` only for explicit scaffold generation. If you are deciding whether a task is done, use `task-done`. If you are deciding whether a branch is ready, use `pr-ready`.
 
 ## How Coding Agents Use It
 
diff --git a/commands/review-artifact.md b/commands/review-artifact.md
index 4bc6c39..36c80b0 100644
--- a/commands/review-artifact.md
+++ b/commands/review-artifact.md
@@ -2,4 +2,6 @@
 
 Invoke the metareview review-artifact skill.
 
+Artifact review authorizes the five required reviewer lenses to run as parallel subagents by default. Fall back to `in-session-emulated` only when subagents are unavailable or the human explicitly requests no delegation; mark fallback as not independently adversarial and weaker evidence.
+
 Arguments: `$ARGUMENTS`
diff --git a/docs/README.claude.md b/docs/README.claude.md
index edc59a9..14f31f6 100644
--- a/docs/README.claude.md
+++ b/docs/README.claude.md
@@ -42,7 +42,7 @@ go run ./cmd/metareview review task-done <task-id-or-path> --base <base-ref> --e
 
 ## Agent Contract
 
-Claude Code agents must resolve every blocking finding before claiming completion. A `NOT_REVIEWED` artifact scaffold is also blocking; complete the required reviewer rows and final verdict before treating the artifact as reviewed. Re-run the review with `--previous-run <run-id>` after fixes. `PASS_ADVISORY` is acceptable only when the report has zero blocking findings.
+Claude Code agents must resolve every blocking finding before claiming completion. A `NOT_REVIEWED` artifact scaffold is also blocking; complete the required reviewer rows and final verdict before treating the artifact as reviewed. Artifact review authorizes the five required lenses to run as parallel subagents by default. If subagents are unavailable or the human requests no delegation, record `in-session-emulated` and state that the review is not independently adversarial and is weaker evidence. Re-run the review with `--previous-run <run-id>` after fixes. `PASS_ADVISORY` is acceptable only when the report has zero blocking findings.
 
 Commit durable review and context Markdown under `docs/metareview/`. Leave transient `.metareview/findings.jsonl` and `.metareview/runs.jsonl` local.
 
diff --git a/docs/README.codex.md b/docs/README.codex.md
index 874c104..a937ec3 100644
--- a/docs/README.codex.md
+++ b/docs/README.codex.md
@@ -49,7 +49,7 @@ In a source checkout without a packaged binary, prefix commands with `go run ./c
 
 ## Agent Contract
 
-Codex agents must not claim work complete while a blocking finding remains open or while an artifact review remains `NOT_REVIEWED`. The default artifact command exits nonzero after scaffold creation until agents complete the required reviewer rows and final verdict. Fix blockers, re-run with `--previous-run <run-id>`, and proceed only after `PASS` or `PASS_ADVISORY` with zero blockers.
+Codex agents must not claim work complete while a blocking finding remains open or while an artifact review remains `NOT_REVIEWED`. The default artifact command exits nonzero after scaffold creation until agents complete the required reviewer rows and final verdict. Artifact review authorizes the five required lenses to run as parallel subagents by default. If subagents are unavailable or the human requests no delegation, record `in-session-emulated` and state that the review is not independently adversarial and is weaker evidence. Fix blockers, re-run with `--previous-run <run-id>`, and proceed only after `PASS` or `PASS_ADVISORY` with zero blockers.
 
 Commit durable review artifacts under `docs/metareview/`. Keep transient `.metareview/findings.jsonl` and `.metareview/runs.jsonl` local.
 
diff --git a/docs/index.html b/docs/index.html
index 7aa4b7d..23943e1 100644
--- a/docs/index.html
+++ b/docs/index.html
@@ -704,7 +704,7 @@ npm run build
     <section>
       <div class="wrap">
         <h2>Try a first local review</h2>
-        <p class="section-intro">Point metareview at an existing doc, spec, or plan. Artifact review creates a review scaffold and context pack, then fails closed until the reviewer rows and verdict are completed.</p>
+        <p class="section-intro">Point metareview at an existing doc, spec, or plan. Artifact review creates a review scaffold and context pack, defaults to parallel subagents for the required lenses, then fails closed until the reviewer rows and verdict are completed.</p>
         <div class="terminal">
           <div class="terminal-bar">artifact review</div>
           <pre><code>metareview review artifact docs/quickstart.md
diff --git a/docs/quickstart.md b/docs/quickstart.md
index b2025b5..0a1a188 100644
--- a/docs/quickstart.md
+++ b/docs/quickstart.md
@@ -30,7 +30,7 @@ metareview review pr-ready --base <base-ref>
 metareview learn --post-merge <pr-number> --base <pre-merge-ref>
 ```
 
-`artifact` creates an incomplete review scaffold for specs, plans, and docs. The command exits nonzero while the scaffold is still `NOT_REVIEWED`; complete every required reviewer row and update the verdict before treating the artifact as reviewed. Use `--scaffold-only` only when scaffold creation itself is the intended action. `task-done` runs after a local task or chunk claims done. `epic-ready` runs when child tasks are complete. `pr-ready` runs before push or merge readiness. `learn --post-merge` runs after confirmed PR merge.
+`artifact` creates an incomplete review scaffold for specs, plans, and docs. The command exits nonzero while the scaffold is still `NOT_REVIEWED`; complete every required reviewer row and update the verdict before treating the artifact as reviewed. Artifact review runs the five required lenses as parallel subagents by default. Use `in-session-emulated` only when subagents are unavailable or the human explicitly requests no delegation, and state that the review is not independently adversarial and is weaker evidence. Use `--scaffold-only` only when scaffold creation itself is the intended action. `task-done` runs after a local task or chunk claims done. `epic-ready` runs when child tasks are complete. `pr-ready` runs before push or merge readiness. `learn --post-merge` runs after confirmed PR merge.
 
 If a review reports any blocking finding or remains `NOT_REVIEWED`, fix it and re-run with `--previous-run <run-id>` until the result is `PASS` or `PASS_ADVISORY` with zero blockers.
 
diff --git a/internal/artifactreview/review.go b/internal/artifactreview/review.go
index 1097b86..5339b05 100644
--- a/internal/artifactreview/review.go
+++ b/internal/artifactreview/review.go
@@ -106,7 +106,7 @@ func Create(root, target, previousRun string, at time.Time) (Result, error) {
 		SchemaVersion: 1,
 		ID:            runID, Scope: "artifact",
 		Target: map[string]string{"type": "path", "path": target},
-		Status: "open", Verdict: "NOT_REVIEWED", ExecutionMode: "in-session-emulated",
+		Status: "open", Verdict: "NOT_REVIEWED", ExecutionMode: "pending-parallel-subagents",
 		PreviousRunID: prev, BaseSHA: head, HeadSHA: head, ContextPath: ctx.ContextRel, ReviewPath: reviewRel,
 		Reviewers:  []string{"feasibility", "completeness", "scope-alignment", "architecture", "intent-preservation"},
 		FindingIDs: []string{}, SourceRefs: []map[string]string{{"type": "path", "path": target}},
@@ -126,11 +126,11 @@ func Create(root, target, previousRun string, at time.Time) (Result, error) {
 		"Run ID: " + markdown.InlineCode(runID) + "\n\n" +
 		"Target: " + markdown.InlineCode(target) + "\n\n" +
 		"Context pack: " + markdown.InlineCode(ctx.ContextRel) + "\n\n" +
-		"Execution mode: `in-session-emulated`\n\n" +
+		"Execution mode: `pending-parallel-subagents`\n\n" +
 		"Previous run: " + markdown.InlineCode(prevDisplay) + "\n\n" +
 		"## Verdict\n\nNOT_REVIEWED\n\n" +
-		"## Completion Requirements\n\nThis scaffold is not a completed review. It blocks downstream gates until all required reviewer rows are populated and the verdict is `PASS` or `PASS_ADVISORY` with zero blocking findings.\n\n" +
-		"## Reviewer Prompts\n\nUse `rubrics/artifact-review-rubric.md` and the context pack above. Run these lenses independently before aggregation:\n\n" +
+		"## Completion Requirements\n\nThis scaffold is not a completed review. Artifact review defaults to parallel subagents for the five required lenses. The artifact-review workflow is explicit authorization to delegate those lenses. Only use `in-session-emulated` when subagents are unavailable or the human explicitly requested no delegation; if used, state that the review is not independently adversarial and treat it as weaker evidence. Completion requires every required reviewer row to be populated, each reviewer to have a verdict, blocking findings to be fixed and re-reviewed or explicitly human-accepted, and the aggregate verdict to be the actual artifact-review verdict returned by the reviewer set rather than a fixed example result.\n\n" +
+		"## Reviewer Prompts\n\nUse `rubrics/artifact-review-rubric.md` and the context pack above. Run these lenses as parallel subagents by default before aggregation:\n\n" +
 		"- Feasibility\n" +
 		"- Completeness\n" +
 		"- Scope and alignment\n" +
diff --git a/lib/artifact-review.js b/lib/artifact-review.js
index 3b46b4b..e0f1f06 100644
--- a/lib/artifact-review.js
+++ b/lib/artifact-review.js
@@ -86,7 +86,7 @@ function createArtifactReview(root, target, options = {}) {
       target: { type: 'path', path: target },
       status: 'open',
       verdict: 'NOT_REVIEWED',
-      executionMode: 'in-session-emulated',
+      executionMode: 'pending-parallel-subagents',
       previousRunId: options.previousRunId || null,
       baseSha: head,
       headSha: head,
@@ -112,7 +112,7 @@ Target: ${inlineCode(target)}
 
 Context pack: ${inlineCode(context.contextPath)}
 
-Execution mode: \`in-session-emulated\`
+Execution mode: \`pending-parallel-subagents\`
 
 Previous run: ${inlineCode(runRecord.previousRunId || 'none')}
 
@@ -122,11 +122,11 @@ NOT_REVIEWED
 
 ## Completion Requirements
 
-This scaffold is not a completed review. It blocks downstream gates until all required reviewer rows are populated and the verdict is \`PASS\` or \`PASS_ADVISORY\` with zero blocking findings.
+This scaffold is not a completed review. Artifact review defaults to parallel subagents for the five required lenses. The artifact-review workflow is explicit authorization to delegate those lenses. Only use \`in-session-emulated\` when subagents are unavailable or the human explicitly requested no delegation; if used, state that the review is not independently adversarial and treat it as weaker evidence. Completion requires every required reviewer row to be populated, each reviewer to have a verdict, blocking findings to be fixed and re-reviewed or explicitly human-accepted, and the aggregate verdict to be the actual artifact-review verdict returned by the reviewer set rather than a fixed example result.
 
 ## Reviewer Prompts
 
-Use \`rubrics/artifact-review-rubric.md\` and the context pack above. Run these lenses independently before aggregation:
+Use \`rubrics/artifact-review-rubric.md\` and the context pack above. Run these lenses as parallel subagents by default before aggregation:
 
 - Feasibility
 - Completeness
diff --git a/skills/review-artifact/SKILL.md b/skills/review-artifact/SKILL.md
index 25a918c..e13eb69 100644
--- a/skills/review-artifact/SKILL.md
+++ b/skills/review-artifact/SKILL.md
@@ -12,13 +12,15 @@ Use when reviewing a Markdown artifact before implementation or before a gate is
 1. Run `metareview review artifact <path>` to create the review scaffold. The command exits nonzero while the review is still `NOT_REVIEWED`; this is expected and is blocking.
 2. Read the generated context pack and review log path.
 3. Use `rubrics/artifact-review-rubric.md`.
-4. Run the listed reviewer lenses independently. If true subagents are unavailable, record `in-session-emulated`.
-5. Update the review log with reviewer rows, verdict, findings, and evidence.
-6. Blocking findings must cite file lines, artifact sections, command output, or task IDs.
-7. For a re-review, run `metareview review artifact <path> --previous-run <run-id>` so the new run links to the prior attempt.
+4. Run the required lenses as parallel subagents by default: Feasibility, Completeness, Scope and alignment, Architecture, Intent preservation. Invoking this artifact-review workflow is explicit authorization to delegate those lenses.
+5. Only fall back to `in-session-emulated` when subagents are unavailable or the human explicitly requests no delegation. If falling back, state that the review is not independently adversarial and treat it as weaker evidence.
+6. Update the review log with reviewer rows, per-reviewer verdicts, findings, evidence, execution mode, and the aggregate verdict.
+7. Always return the actual artifact-review verdict from the reviewer set. Do not substitute a fixed example verdict; `NEEDS_REVISION` and `ESCALATE` are valid review results when supported by findings.
+8. Blocking findings must cite file lines, artifact sections, command output, or task IDs.
+9. For a re-review, run `metareview review artifact <path> --previous-run <run-id>` so the new run links to the prior attempt.
 
 Use `metareview review artifact <path> --scaffold-only` only when explicitly creating a scaffold without claiming the review is complete.
 
 ## Gate Rule
 
-Do not call an artifact implementation-ready while the verdict is `NOT_REVIEWED`, `ESCALATE`, `NEEDS_REVISION`, missing required reviewer rows, or while blocking findings remain unresolved unless the human explicitly accepts the risk.
+A review execution is incomplete while required reviewer rows are missing, any reviewer lacks a verdict, or the aggregate verdict is `NOT_REVIEWED`. Do not call an artifact implementation-ready while the verdict is `ESCALATE` or `NEEDS_REVISION`, required reviewer rows are missing, reviewer verdicts are missing, or blocking findings remain unresolved unless the human explicitly accepts the risk.
diff --git a/tests/go/test-artifact-review.sh b/tests/go/test-artifact-review.sh
index 5146c98..1087fdf 100644
--- a/tests/go/test-artifact-review.sh
+++ b/tests/go/test-artifact-review.sh
@@ -40,11 +40,17 @@ test "$artifact_code" -eq 1
 review_path="$(cat "$TMP/artifact.out")"
 test -f "$repo/$review_path"
 grep -q 'NOT_REVIEWED' "$repo/$review_path"
+grep -q 'Execution mode: `pending-parallel-subagents`' "$repo/$review_path"
+grep -q 'Artifact review defaults to parallel subagents' "$repo/$review_path"
+grep -q 'Only use `in-session-emulated` when subagents are unavailable or the human explicitly requested no delegation' "$repo/$review_path"
+grep -q 'not independently adversarial' "$repo/$review_path"
 grep -q 'Artifact review scaffold created but not completed' "$TMP/artifact.err"
 test -f .metareview/runs.jsonl
 test -f .metareview/findings.jsonl
 test -f docs/metareview/FINDINGS.md
 first_run="$(node -e "const fs=require('fs'); const lines=fs.readFileSync('.metareview/runs.jsonl','utf8').trim().split('\\n').map(JSON.parse); console.log(lines[0].id)")"
+first_mode="$(node -e "const fs=require('fs'); const lines=fs.readFileSync('.metareview/runs.jsonl','utf8').trim().split('\\n').map(JSON.parse); console.log(lines[0].executionMode)")"
+test "$first_mode" = "pending-parallel-subagents"
 
 second_review="$("$TMP/metareview" review artifact docs/plan.md --previous-run "$first_run" --scaffold-only)"
 test -f "$repo/$second_review"
diff --git a/tests/lib/test-artifact-review.sh b/tests/lib/test-artifact-review.sh
index 2064a7f..47ede50 100644
--- a/tests/lib/test-artifact-review.sh
+++ b/tests/lib/test-artifact-review.sh
@@ -27,7 +27,10 @@ assert(fs.existsSync(path.join(root, '.metareview', 'findings.jsonl')), 'finding
 assert(fs.existsSync(path.join(root, 'docs', 'metareview', 'FINDINGS.md')), 'findings index missing');
 
 const log = fs.readFileSync(path.join(root, result.reviewLogPath), 'utf8');
-assert(log.includes('Execution mode: `in-session-emulated`'), 'missing execution mode');
+assert(log.includes('Execution mode: `pending-parallel-subagents`'), 'missing pending execution mode');
+assert(log.includes('Artifact review defaults to parallel subagents'), 'missing parallel subagent default instruction');
+assert(log.includes('Only use `in-session-emulated` when subagents are unavailable or the human explicitly requested no delegation'), 'missing fallback execution mode instruction');
+assert(log.includes('not independently adversarial'), 'missing weak fallback evidence warning');
 assert(log.includes('Previous run: `none`'), 'missing previous run none');
 assert(log.includes('## Reviewer Prompts'), 'missing reviewer prompts');
 assert(log.includes('## Aggregation Instructions'), 'missing aggregation instructions');
@@ -36,6 +39,7 @@ const runs = fs.readFileSync(path.join(root, '.metareview', 'runs.jsonl'), 'utf8
 assert(runs.length === 1, 'expected one run record');
 assert(runs[0].scope === 'artifact', 'run scope mismatch');
 assert(runs[0].status === 'open', 'run status mismatch');
+assert(runs[0].executionMode === 'pending-parallel-subagents', 'run execution mode mismatch');
 assert(runs[0].previousRunId === null, 'first run should not link a previous run');
 
 const retryNow = new Date('2026-05-26T12:35:56Z');
diff --git a/tests/manifest/test-skills.sh b/tests/manifest/test-skills.sh
index cbb0db0..e18451c 100644
--- a/tests/manifest/test-skills.sh
+++ b/tests/manifest/test-skills.sh
@@ -80,6 +80,18 @@ grep -q '^## How Coding Agents Use It$' README.md
 grep -q '^## Philosophy$' README.md
 grep -q 'metareview review task-done' skills/review-task-done/SKILL.md
 grep -q -- '--scaffold-only' skills/review-artifact/SKILL.md
+grep -q 'parallel subagents by default' skills/review-artifact/SKILL.md
+grep -q 'explicit authorization' skills/review-artifact/SKILL.md
+grep -q 'not independently adversarial' skills/review-artifact/SKILL.md
+grep -q 'Feasibility, Completeness, Scope and alignment, Architecture, Intent preservation' skills/review-artifact/SKILL.md
+grep -q 'return the actual artifact-review verdict' skills/review-artifact/SKILL.md
+grep -q 'parallel subagents by default' docs/quickstart.md
+grep -q 'in-session-emulated' docs/quickstart.md
+grep -q 'weaker evidence' docs/quickstart.md
+grep -q 'not independently adversarial' docs/README.codex.md
+grep -q 'weaker evidence' docs/README.codex.md
+grep -q 'not independently adversarial' docs/README.claude.md
+grep -q 'weaker evidence' docs/README.claude.md
 grep -q -- '--scaffold-only' docs/quickstart.md
 grep -q 'metareview review task-done' commands/review-task-done.md
 grep -q 'metareview review epic-ready' skills/review-epic-ready/SKILL.md
--- docs/specs/2026-05-27-artifact-review-parallel-subagents.md
+# Artifact Review Parallel Subagent Default
+
+## Problem
+
+The 0.2.0 artifact-review gate fails closed on incomplete scaffolds, but the process directions still leave room for one agent to treat the five reviewer lenses as a self-review. That weakens the adversarial review guarantee that prompted the gate.
+
+## Requirements
+
+- Artifact review means five independent reviewer lenses: Feasibility, Completeness, Scope and alignment, Architecture, and Intent preservation.
+- The artifact-review workflow itself is explicit authorization to delegate those reviewer lenses.
+- When subagents are available, run the five reviewer lenses as parallel subagents by default.
+- When subagents are unavailable, or the human explicitly requests no delegation, record the execution mode as `in-session-emulated`.
+- In-session emulation must state that the review is not independently adversarial and should be treated as weaker evidence.
+- New artifact review scaffolds must not pre-label the review as `in-session-emulated`; the scaffold should start in a pending/delegation-intended mode and instruct agents to update the mode after real reviewer execution.
+- A review execution is incomplete while required reviewer rows are empty, a reviewer lacks a verdict, or the aggregate verdict is `NOT_REVIEWED`.
+- The artifact review must report the actual artifact-review verdict returned by the reviewer set, including `NEEDS_REVISION` or `ESCALATE` when that is what the review found. It must not force a deterministic example verdict; downstream readiness claims are what require zero unresolved blockers or explicit human acceptance.
+
+## Non-Goals
+
+- Do not implement LLM or subagent orchestration inside the Go CLI in this slice.
+- Do not change deterministic task-done, epic-ready, or pr-ready reviewer logic.
+- Do not require a specific final verdict from every artifact review run.
+
+## Acceptance
+
+- `skills/review-artifact/SKILL.md` states that parallel subagents are the default artifact-review execution mode and preserves the named five-lens set: Feasibility, Completeness, Scope and alignment, Architecture, and Intent preservation.
+- The skill states that artifact-review invocation counts as explicit authorization for delegation.
+- The fallback mode is named `in-session-emulated`, limited to unavailable subagents or explicit human no-delegation requests, marked not independently adversarial, and treated as weaker evidence.
+- Go and JavaScript artifact-review scaffold generators no longer hardcode `in-session-emulated` as the initial execution mode; they start in pending/delegation-intended mode and instruct agents to update execution mode after real reviewer execution.
+- The skill says review execution completion requires populated required reviewer rows, per-reviewer verdicts, and an aggregate verdict other than `NOT_REVIEWED`; artifact readiness still requires zero unresolved blocking findings unless explicitly human-accepted.
+- Quickstart and agent integration docs mention the subagent default, the unavailable-subagent or human-no-delegation fallback trigger, and the weaker-evidence caveat.
+- The skill tells agents to return the actual artifact-review verdict instead of substituting a fixed example result.
+- `bash tests/manifest/test-skills.sh`, `bash tests/go/test-artifact-review.sh`, and `bash tests/lib/test-artifact-review.sh` assert the new delegation, fallback, scaffold, completion, and actual-verdict contract text.
````

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

Verification evidence:
- bash tests/manifest/test-skills.sh exited 0 after RED/GREEN wording assertions
- bash tests/lib/test-artifact-review.sh exited 0
- bash tests/go/test-artifact-review.sh exited 0
- go test ./... exited 0
- bash tests/run-all.sh exited 0
- git diff --check exited 0

