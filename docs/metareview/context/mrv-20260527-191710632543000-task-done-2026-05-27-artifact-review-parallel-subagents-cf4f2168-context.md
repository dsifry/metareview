# metareview task-done context

Run ID: `mrv-20260527-191710632543000-task-done-2026-05-27-artifact-review-parallel-subagents-cf4f2168`

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
- The legacy JavaScript implementation is removed so artifact-review scaffold generation has a single Go implementation path; the Go scaffold starts in pending/delegation-intended mode and instructs agents to update execution mode after real reviewer execution.
- The skill says review execution completion requires populated required reviewer rows, per-reviewer verdicts, and an aggregate verdict other than `NOT_REVIEWED`; artifact readiness still requires zero unresolved blocking findings unless explicitly human-accepted.
- Quickstart and agent integration docs mention the subagent default, the unavailable-subagent or human-no-delegation fallback trigger, and the weaker-evidence caveat.
- The skill tells agents to return the actual artifact-review verdict instead of substituting a fixed example result.
- `bash tests/manifest/test-skills.sh`, `bash tests/manifest/test-manifests.sh`, and `bash tests/go/test-artifact-review.sh` assert the new delegation, fallback, single-implementation, scaffold, completion, and actual-verdict contract text.


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
- internal/gitcontext/gitcontext.go
- lib/artifact-review.js
- lib/context-pack.js
- lib/markdown.js
- lib/repo-detect.js
- lib/state.js
- lib/version.js
- skills/review-artifact/SKILL.md
- tests/go/test-artifact-review.sh
- tests/lib/test-artifact-review.sh
- tests/lib/test-context-pack.sh
- tests/lib/test-repo-detect.sh
- tests/lib/test-state.sh
- tests/manifest/test-manifests.sh
- tests/manifest/test-skills.sh
- tests/run-all.sh
- docs/specs/2026-05-27-artifact-review-parallel-subagents.md
- internal/gitcontext/gitcontext_test.go

## Diff

`````diff


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
diff --git a/internal/gitcontext/gitcontext.go b/internal/gitcontext/gitcontext.go
index afe93f9..deef102 100644
--- a/internal/gitcontext/gitcontext.go
+++ b/internal/gitcontext/gitcontext.go
@@ -11,7 +11,7 @@ import (
 	"unicode/utf8"
 )
 
-const maxDiffBytes = 60000
+const maxDiffBytes = 120000
 const maxUntrackedFiles = 20
 const maxUntrackedFileBytes = 4000
 
diff --git a/lib/artifact-review.js b/lib/artifact-review.js
deleted file mode 100644
index 3b46b4b..0000000
--- a/lib/artifact-review.js
+++ /dev/null
@@ -1,183 +0,0 @@
-'use strict';
-
-const fs = require('fs');
-const path = require('path');
-const childProcess = require('child_process');
-const { buildContextPack } = require('./context-pack');
-const { appendJsonl, createRunId } = require('./state');
-const { inlineCode } = require('./markdown');
-
-function gitHead(root) {
-  try {
-    return childProcess.execSync('git rev-parse HEAD', { cwd: root, stdio: ['ignore', 'pipe', 'ignore'] }).toString().trim();
-  } catch {
-    return 'unavailable';
-  }
-}
-
-function ensureEmptyJsonl(filePath) {
-  fs.mkdirSync(path.dirname(filePath), { recursive: true });
-  if (!fs.existsSync(filePath)) fs.writeFileSync(filePath, '');
-}
-
-function ensureFindingsIndex(root) {
-  const findingsPath = path.join(root, 'docs', 'metareview', 'FINDINGS.md');
-  fs.mkdirSync(path.dirname(findingsPath), { recursive: true });
-  if (!fs.existsSync(findingsPath)) {
-    fs.writeFileSync(findingsPath, `# metareview Findings
-
-No unresolved findings recorded yet.
-`);
-  }
-}
-
-function removeIfCreated(filePath, existedBefore) {
-  if (!existedBefore && fs.existsSync(filePath)) {
-    fs.rmSync(filePath, { force: true });
-  }
-}
-
-function restoreOrRemove(filePath, existedBefore, previousContent) {
-  if (existedBefore) {
-    fs.writeFileSync(filePath, previousContent);
-    return;
-  }
-  removeIfCreated(filePath, false);
-}
-
-function cleanupArtifacts(paths) {
-  fs.rmSync(paths.reviewPath, { force: true });
-  restoreOrRemove(paths.contextPath, paths.contextExisted, paths.previousContext);
-  removeIfCreated(paths.findingsJsonlPath, paths.findingsJsonlExisted);
-  removeIfCreated(paths.findingsIndexPath, paths.findingsIndexExisted);
-}
-
-function createArtifactReview(root, target, options = {}) {
-  const now = options.now || new Date();
-  const runId = createRunId('artifact', target, now);
-  const reviewRel = `docs/metareview/reviews/${runId}.md`;
-  const reviewPath = path.join(root, reviewRel);
-  if (fs.existsSync(reviewPath)) {
-    throw new Error(`Review log already exists for run ${runId}: ${reviewRel}`);
-  }
-
-  const contextRel = `docs/metareview/context/${runId}-context.md`;
-  const contextPath = path.join(root, contextRel);
-  const contextExisted = fs.existsSync(contextPath);
-  const previousContext = contextExisted ? fs.readFileSync(contextPath, 'utf8') : null;
-  const findingsJsonlPath = path.join(root, '.metareview', 'findings.jsonl');
-  const findingsIndexPath = path.join(root, 'docs', 'metareview', 'FINDINGS.md');
-  const findingsJsonlExisted = fs.existsSync(findingsJsonlPath);
-  const findingsIndexExisted = fs.existsSync(findingsIndexPath);
-
-  try {
-    const context = buildContextPack(root, target, { now });
-    if (context.runId !== runId) {
-      throw new Error(`Context pack run ID mismatch: expected ${runId}, got ${context.runId}`);
-    }
-    fs.mkdirSync(path.dirname(reviewPath), { recursive: true });
-
-    const head = gitHead(root);
-    const nowIso = now.toISOString();
-    const runRecord = {
-      schemaVersion: 1,
-      id: context.runId,
-      scope: 'artifact',
-      target: { type: 'path', path: target },
-      status: 'open',
-      verdict: 'NOT_REVIEWED',
-      executionMode: 'in-session-emulated',
-      previousRunId: options.previousRunId || null,
-      baseSha: head,
-      headSha: head,
-      contextPackPath: context.contextPath,
-      reviewLogPath: reviewRel,
-      reviewers: ['feasibility', 'completeness', 'scope-alignment', 'architecture', 'intent-preservation'],
-      findingIds: [],
-      sourceRefs: [{ type: 'path', path: target }],
-      createdAt: nowIso,
-      updatedAt: nowIso,
-      repoRoot: root,
-      gitHead: head
-    };
-
-    ensureEmptyJsonl(findingsJsonlPath);
-    ensureFindingsIndex(root);
-
-    const content = `# metareview: artifact review
-
-Run ID: ${inlineCode(context.runId)}
-
-Target: ${inlineCode(target)}
-
-Context pack: ${inlineCode(context.contextPath)}
-
-Execution mode: \`in-session-emulated\`
-
-Previous run: ${inlineCode(runRecord.previousRunId || 'none')}
-
-## Verdict
-
-NOT_REVIEWED
-
-## Completion Requirements
-
-This scaffold is not a completed review. It blocks downstream gates until all required reviewer rows are populated and the verdict is \`PASS\` or \`PASS_ADVISORY\` with zero blocking findings.
-
-## Reviewer Prompts
-
-Use \`rubrics/artifact-review-rubric.md\` and the context pack above. Run these lenses independently before aggregation:
-
-- Feasibility
-- Completeness
-- Scope and alignment
-- Architecture
-- Intent preservation
-
-For each lens, produce:
-
-- verdict
-- blocking findings with evidence
-- warnings with evidence
-- knowledge candidates
-
-## Aggregation Instructions
-
-After all lenses are complete:
-
-1. Deduplicate findings.
-2. Assign stable finding IDs.
-3. Separate blockers from warnings.
-4. Update this review log verdict.
-5. Append machine-readable findings to \`.metareview/findings.jsonl\`.
-6. Update \`docs/metareview/FINDINGS.md\`.
-
-## Reviewer Results
-
-| Reviewer | Verdict | Blocking | Warnings | Notes |
-| --- | --- | ---: | ---: | --- |
-
-## Findings
-
-No reviewer findings recorded yet.
-`;
-
-    fs.writeFileSync(reviewPath, content);
-    appendJsonl(path.join(root, '.metareview', 'runs.jsonl'), runRecord);
-    return { runId: context.runId, reviewLogPath: reviewRel, contextPath: context.contextPath };
-  } catch (error) {
-    cleanupArtifacts({
-      reviewPath,
-      contextPath,
-      contextExisted,
-      previousContext,
-      findingsJsonlPath,
-      findingsJsonlExisted,
-      findingsIndexPath,
-      findingsIndexExisted
-    });
-    throw error;
-  }
-}
-
-module.exports = { createArtifactReview };
diff --git a/lib/context-pack.js b/lib/context-pack.js
deleted file mode 100644
index 04b6b61..0000000
--- a/lib/context-pack.js
+++ /dev/null
@@ -1,165 +0,0 @@
-'use strict';
-
-const fs = require('fs');
-const path = require('path');
-const childProcess = require('child_process');
-const { createRunId } = require('./state');
-const { detectRepo } = require('./repo-detect');
-const { inlineCode, normalizeMarkdownInline, plainText } = require('./markdown');
-
-function readIfExists(filePath) {
-  try {
-    const stat = fs.statSync(filePath);
-    if (!stat.isFile()) return '';
-    fs.accessSync(filePath, fs.constants.R_OK);
-    return fs.readFileSync(filePath, 'utf8');
-  } catch (_error) {
-    return '';
-  }
-}
-
-function isPathInsideRoot(root, filePath) {
-  try {
-    const realRootPath = fs.realpathSync(path.resolve(root));
-    const realFilePath = fs.realpathSync(filePath);
-    return realFilePath === realRootPath || realFilePath.startsWith(`${realRootPath}${path.sep}`);
-  } catch (_error) {
-    return false;
-  }
-}
-
-function readRepoFileIfExists(root, filePath) {
-  if (!isPathInsideRoot(root, filePath)) return '';
-  return readIfExists(filePath);
-}
-
-function fencedCodeBlock(language, content) {
-  const backtickRuns = String(content).match(/`+/g) || [];
-  const longestRun = backtickRuns.reduce((max, run) => Math.max(max, run.length), 0);
-  const fence = '`'.repeat(Math.max(3, longestRun + 1));
-  return `${fence}${language}\n${content}\n${fence}`;
-}
-
-function listKnowledge(root) {
-  const dir = path.join(root, '.beads', 'knowledge');
-  try {
-    if (!fs.statSync(dir).isDirectory()) return [];
-  } catch (_error) {
-    return [];
-  }
-
-  let files;
-  try {
-    files = fs.readdirSync(dir);
-  } catch (_error) {
-    return [];
-  }
-
-  return files
-    .filter(file => file.endsWith('.jsonl'))
-    .flatMap(file => {
-      const full = path.join(dir, file);
-      return readRepoFileIfExists(root, full)
-        .split('\n')
-        .filter(Boolean)
-        .slice(0, 5)
-        .map(line => {
-          const displayFile = normalizeMarkdownInline(file);
-          try {
-            const parsed = JSON.parse(line);
-            return `- ${displayFile}: ${normalizeMarkdownInline(parsed.fact || parsed.recommendation || parsed.id || line)}`;
-          } catch {
-            return `- ${displayFile}: ${normalizeMarkdownInline(line)}`;
-          }
-        });
-    });
-}
-
-function gitSummary(root) {
-  try {
-    const head = childProcess.execSync('git rev-parse --short HEAD', { cwd: root, stdio: ['ignore', 'pipe', 'ignore'] }).toString().trim();
-    const branch = childProcess.execSync('git branch --show-current', { cwd: root, stdio: ['ignore', 'pipe', 'ignore'] }).toString().trim();
-    return { head, branch };
-  } catch {
-    return { head: 'unavailable', branch: 'unavailable' };
-  }
-}
-
-function assertTargetInsideRoot(root, target) {
-  const rootPath = path.resolve(root);
-  const targetPath = path.resolve(rootPath, target);
-  if (targetPath !== rootPath && !targetPath.startsWith(`${rootPath}${path.sep}`)) {
-    throw new Error(`Target artifact is outside repository root: ${target}`);
-  }
-  if (!fs.existsSync(targetPath)) {
-    throw new Error(`Target artifact not found: ${target}`);
-  }
-  const stat = fs.statSync(targetPath);
-  if (!stat.isFile()) {
-    throw new Error(`Target artifact is not a regular file: ${target}`);
-  }
-  const realRootPath = fs.realpathSync(rootPath);
-  const realTargetPath = fs.realpathSync(targetPath);
-  if (realTargetPath !== realRootPath && !realTargetPath.startsWith(`${realRootPath}${path.sep}`)) {
-    throw new Error(`Target artifact is outside repository root: ${target}`);
-  }
-  return targetPath;
-}
-
-function buildContextPack(root, target, options = {}) {
-  const now = options.now || new Date();
-  const runId = createRunId('artifact', target, now);
-  const targetPath = assertTargetInsideRoot(root, target);
-  const titleTarget = plainText(target);
-
-  const report = detectRepo(root);
-  const git = gitSummary(root);
-  const serviceInventoryPath = report.files.serviceInventory;
-  const serviceInventoryDisplayPath = serviceInventoryPath ? normalizeMarkdownInline(serviceInventoryPath) : null;
-  const serviceInventory = serviceInventoryPath
-    ? readRepoFileIfExists(root, path.join(root, serviceInventoryPath)).slice(0, 2000)
-    : '';
-  const knowledge = listKnowledge(root);
-  const artifact = readIfExists(targetPath).slice(0, 4000);
-
-  const contextRel = `docs/metareview/context/${runId}-context.md`;
-  const outputPath = path.join(root, contextRel);
-  fs.mkdirSync(path.dirname(outputPath), { recursive: true });
-
-  const content = `# metareview context: ${titleTarget}
-
-Run ID: ${inlineCode(runId)}
-
-## Target
-
-- Path: ${inlineCode(target)}
-- Repository mode: ${inlineCode(report.mode)}
-- Git branch: ${inlineCode(git.branch)}
-- Git head: ${inlineCode(git.head)}
-
-## Artifact Excerpt
-
-${fencedCodeBlock('markdown', artifact)}
-
-## Service Inventory
-
-${serviceInventoryDisplayPath ? `Source: ${inlineCode(serviceInventoryDisplayPath)}\n\n${fencedCodeBlock('markdown', serviceInventory)}` : 'No service inventory found.'}
-
-## Knowledge Facts
-
-${knowledge.length ? knowledge.join('\n') : 'No Beads knowledge facts found.'}
-
-## Suggested Reviewers
-
-- feasibility
-- completeness
-- scope/alignment
-- architecture
-- intent preservation
-`;
-
-  fs.writeFileSync(outputPath, content);
-  return { runId, contextPath: contextRel };
-}
-
-module.exports = { buildContextPack };
diff --git a/lib/markdown.js b/lib/markdown.js
deleted file mode 100644
index 4ffebd3..0000000
--- a/lib/markdown.js
+++ /dev/null
@@ -1,25 +0,0 @@
-'use strict';
-
-function normalizeMarkdownInline(value) {
-  return String(value).replace(/[\r\n]+/g, ' ').replace(/\s+/g, ' ').trim();
-}
-
-function inlineCode(value) {
-  const normalized = normalizeMarkdownInline(value);
-  const backtickRuns = normalized.match(/`+/g) || [];
-  const longestRun = backtickRuns.reduce((max, run) => Math.max(max, run.length), 0);
-  const delimiter = '`'.repeat(Math.max(1, longestRun + 1));
-  const needsPadding = normalized.startsWith('`') || normalized.endsWith('`');
-  const content = needsPadding ? ` ${normalized} ` : normalized;
-  return `${delimiter}${content}${delimiter}`;
-}
-
-function plainText(value) {
-  return normalizeMarkdownInline(value).replace(/`+/g, '').trim();
-}
-
-module.exports = {
-  inlineCode,
-  normalizeMarkdownInline,
-  plainText
-};
diff --git a/lib/repo-detect.js b/lib/repo-detect.js
deleted file mode 100644
index dcd187d..0000000
--- a/lib/repo-detect.js
+++ /dev/null
@@ -1,91 +0,0 @@
-'use strict';
-
-const fs = require('fs');
-const path = require('path');
-
-function exists(root, rel) {
-  return fs.existsSync(path.join(root, rel));
-}
-
-function readIfExists(root, rel) {
-  const full = path.join(root, rel);
-  try {
-    const stat = fs.statSync(full);
-    if (!stat.isFile()) return '';
-    fs.accessSync(full, fs.constants.R_OK);
-    return fs.readFileSync(full, 'utf8');
-  } catch (_error) {
-    return '';
-  }
-}
-
-function hasMetaswarmInstructionMarker(text) {
-  const normalized = text.toLowerCase();
-  const negatedMarkers = [
-    'does not use metaswarm',
-    'not using metaswarm',
-    'without metaswarm',
-    'no metaswarm'
-  ];
-
-  if (negatedMarkers.some(marker => normalized.includes(marker))) {
-    return false;
-  }
-
-  return normalized.includes('uses metaswarm') ||
-    normalized.includes('metaswarm workflows') ||
-    normalized.includes('metaswarm');
-}
-
-function hasMetaswarmMarker(root) {
-  const instructionText = [
-    readIfExists(root, 'AGENTS.md'),
-    readIfExists(root, 'CLAUDE.md'),
-    readIfExists(root, 'GEMINI.md')
-  ].join('\n');
-
-  return hasMetaswarmInstructionMarker(instructionText) ||
-    exists(root, '.claude/plugins/metaswarm') ||
-    exists(root, '.codex/plugins/metaswarm') ||
-    exists(root, 'docs/metaswarm') ||
-    exists(root, '.beads/context/project-context.md');
-}
-
-function findServiceInventory(root) {
-  const candidates = [
-    'docs/SERVICE_INVENTORY.md',
-    'SERVICE_INVENTORY.md',
-    'docs/service-inventory.md',
-    'docs/architecture/SERVICE_INVENTORY.md'
-  ];
-  return candidates.find(rel => exists(root, rel)) || null;
-}
-
-function detectRepo(root) {
-  const capabilities = {
-    git: exists(root, '.git'),
-    beads: exists(root, '.beads') || exists(root, '.beads/issues.jsonl'),
-    metaswarm: hasMetaswarmMarker(root),
-    serviceInventory: Boolean(findServiceInventory(root)),
-    metareviewState: exists(root, '.metareview')
-  };
-
-  let mode = 'advisory';
-  if (capabilities.metaswarm) {
-    mode = 'metaswarm-extension';
-  } else if (capabilities.beads) {
-    mode = 'standalone-full';
-  } else if (capabilities.git) {
-    mode = 'standalone-minimal';
-  }
-
-  return {
-    mode,
-    capabilities,
-    files: {
-      serviceInventory: findServiceInventory(root)
-    }
-  };
-}
-
-module.exports = { detectRepo };
diff --git a/lib/state.js b/lib/state.js
deleted file mode 100644
index cdfeb9d..0000000
--- a/lib/state.js
+++ /dev/null
@@ -1,58 +0,0 @@
-'use strict';
-
-const fs = require('fs');
-const path = require('path');
-const crypto = require('crypto');
-
-function ensureDir(dir) {
-  fs.mkdirSync(dir, { recursive: true });
-}
-
-function appendJsonl(filePath, record) {
-  ensureDir(path.dirname(filePath));
-  fs.appendFileSync(filePath, `${JSON.stringify(record)}\n`);
-}
-
-function readJsonl(filePath) {
-  if (!fs.existsSync(filePath)) return [];
-  return fs.readFileSync(filePath, 'utf8')
-    .split('\n')
-    .filter(line => line.trim().length > 0)
-    .map(line => JSON.parse(line));
-}
-
-function slugify(value) {
-  return String(value)
-    .replace(/\.[a-z0-9]+$/i, '')
-    .toLowerCase()
-    .replace(/[^a-z0-9]+/g, '-')
-    .replace(/^-+|-+$/g, '')
-    .slice(0, 48) || 'target';
-}
-
-function formatDate(date) {
-  const pad = n => String(n).padStart(2, '0');
-  const ms = String(date.getUTCMilliseconds()).padStart(3, '0');
-  return `${date.getUTCFullYear()}${pad(date.getUTCMonth() + 1)}${pad(date.getUTCDate())}-${pad(date.getUTCHours())}${pad(date.getUTCMinutes())}${pad(date.getUTCSeconds())}${ms}`;
-}
-
-function targetHash(target) {
-  return crypto.createHash('sha1').update(String(target)).digest('hex').slice(0, 8);
-}
-
-function createRunId(scope, target, date = new Date()) {
-  const base = path.basename(String(target));
-  return `mrv-${formatDate(date)}-${slugify(scope)}-${slugify(base)}-${targetHash(target)}`;
-}
-
-function createFindingId(runId, index) {
-  return `${runId.replace(/^mrv-/, 'mrvf-')}-${String(index).padStart(3, '0')}`;
-}
-
-module.exports = {
-  appendJsonl,
-  readJsonl,
-  createRunId,
-  createFindingId,
-  slugify
-};
diff --git a/lib/version.js b/lib/version.js
deleted file mode 100644
index a7d72d3..0000000
--- a/lib/version.js
+++ /dev/null
@@ -1,5 +0,0 @@
-'use strict';
-
-const VERSION = '0.1.0';
-
-module.exports = { VERSION };
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
deleted file mode 100644
index 2064a7f..0000000
--- a/tests/lib/test-artifact-review.sh
+++ /dev/null
@@ -1,209 +0,0 @@
-#!/usr/bin/env bash
-set -euo pipefail
-
-node - <<'NODE'
-const fs = require('fs');
-const path = require('path');
-const os = require('os');
-const { createArtifactReview } = require('./lib/artifact-review');
-const { createRunId } = require('./lib/state');
-
-function assert(cond, msg) { if (!cond) throw new Error(msg); }
-function mkdirp(p) { fs.mkdirSync(p, { recursive: true }); }
-function write(p, text) { mkdirp(path.dirname(p)); fs.writeFileSync(p, text); }
-
-const root = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-artifact-'));
-mkdirp(path.join(root, '.git'));
-write(path.join(root, 'docs', 'plan.md'), '# Plan\n\nBuild the artifact review harness.\n');
-
-const now = new Date('2026-05-26T12:34:56Z');
-const expectedRunId = createRunId('artifact', 'docs/plan.md', now);
-const result = createArtifactReview(root, 'docs/plan.md', { now });
-assert(result.runId === expectedRunId, `run id mismatch: ${result.runId}`);
-assert(result.reviewLogPath === `docs/metareview/reviews/${expectedRunId}.md`, result.reviewLogPath);
-assert(fs.existsSync(path.join(root, result.reviewLogPath)), 'review log missing');
-assert(fs.existsSync(path.join(root, '.metareview', 'runs.jsonl')), 'runs jsonl missing');
-assert(fs.existsSync(path.join(root, '.metareview', 'findings.jsonl')), 'findings jsonl missing');
-assert(fs.existsSync(path.join(root, 'docs', 'metareview', 'FINDINGS.md')), 'findings index missing');
-
-const log = fs.readFileSync(path.join(root, result.reviewLogPath), 'utf8');
-assert(log.includes('Execution mode: `in-session-emulated`'), 'missing execution mode');
-assert(log.includes('Previous run: `none`'), 'missing previous run none');
-assert(log.includes('## Reviewer Prompts'), 'missing reviewer prompts');
-assert(log.includes('## Aggregation Instructions'), 'missing aggregation instructions');
-
-const runs = fs.readFileSync(path.join(root, '.metareview', 'runs.jsonl'), 'utf8').trim().split('\n').map(JSON.parse);
-assert(runs.length === 1, 'expected one run record');
-assert(runs[0].scope === 'artifact', 'run scope mismatch');
-assert(runs[0].status === 'open', 'run status mismatch');
-assert(runs[0].previousRunId === null, 'first run should not link a previous run');
-
-const retryNow = new Date('2026-05-26T12:35:56Z');
-const retryExpectedRunId = createRunId('artifact', 'docs/plan.md', retryNow);
-const retry = createArtifactReview(root, 'docs/plan.md', { now: retryNow, previousRunId: result.runId });
-assert(retry.runId === retryExpectedRunId, `retry run id mismatch: ${retry.runId}`);
-const updatedRuns = fs.readFileSync(path.join(root, '.metareview', 'runs.jsonl'), 'utf8').trim().split('\n').map(JSON.parse);
-assert(updatedRuns.length === 2, 'expected two run records after re-review');
-assert(updatedRuns[1].previousRunId === result.runId, 're-review did not link previous run');
-
-const preserveRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-artifact-preserve-'));
-mkdirp(path.join(preserveRoot, '.git'));
-write(path.join(preserveRoot, 'docs', 'plan.md'), '# Plan\n');
-const existingFinding = '{"id":"existing","status":"open"}\n';
-write(path.join(preserveRoot, '.metareview', 'findings.jsonl'), existingFinding);
-createArtifactReview(preserveRoot, 'docs/plan.md', { now });
-const preservedFindings = fs.readFileSync(path.join(preserveRoot, '.metareview', 'findings.jsonl'), 'utf8');
-assert(preservedFindings === existingFinding, 'existing findings jsonl content changed');
-
-const collisionRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-artifact-collision-'));
-mkdirp(path.join(collisionRoot, '.git'));
-write(path.join(collisionRoot, 'docs', 'plan.md'), '# Plan\n');
-const collision = createArtifactReview(collisionRoot, 'docs/plan.md', { now });
-const collisionContextPath = path.join(collisionRoot, collision.contextPath);
-fs.writeFileSync(collisionContextPath, 'SENTINEL');
-let collisionMessage = '';
-try {
-  createArtifactReview(collisionRoot, 'docs/plan.md', { now });
-} catch (error) {
-  collisionMessage = error.message;
-}
-assert(collisionMessage.includes('Review log already exists'), `unexpected collision error: ${collisionMessage}`);
-assert(fs.readFileSync(collisionContextPath, 'utf8') === 'SENTINEL', 'duplicate run rewrote context pack before throwing');
-
-const writeFailureRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-artifact-write-fail-'));
-mkdirp(path.join(writeFailureRoot, '.git'));
-write(path.join(writeFailureRoot, 'docs', 'plan.md'), '# Plan\n');
-const reviewDir = path.join(writeFailureRoot, 'docs', 'metareview', 'reviews');
-mkdirp(reviewDir);
-fs.chmodSync(reviewDir, 0o555);
-let writeFailureMessage = '';
-try {
-  createArtifactReview(writeFailureRoot, 'docs/plan.md', { now });
-} catch (error) {
-  writeFailureMessage = error.message;
-} finally {
-  fs.chmodSync(reviewDir, 0o755);
-}
-assert(writeFailureMessage, 'expected review log write failure');
-const failedRunsPath = path.join(writeFailureRoot, '.metareview', 'runs.jsonl');
-const failedRuns = fs.existsSync(failedRunsPath) ? fs.readFileSync(failedRunsPath, 'utf8') : '';
-assert(failedRuns.trim() === '', 'run record was appended after review log write failed');
-const failedContextPath = path.join(writeFailureRoot, 'docs', 'metareview', 'context', `${expectedRunId}-context.md`);
-assert(!fs.existsSync(failedContextPath), 'context pack remained after review log write failed');
-assert(!fs.existsSync(path.join(writeFailureRoot, '.metareview', 'findings.jsonl')), 'new findings jsonl remained after review log write failed');
-assert(!fs.existsSync(path.join(writeFailureRoot, 'docs', 'metareview', 'FINDINGS.md')), 'new findings index remained after review log write failed');
-
-const preserveFailureRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-artifact-preserve-fail-'));
-mkdirp(path.join(preserveFailureRoot, '.git'));
-write(path.join(preserveFailureRoot, 'docs', 'plan.md'), '# Plan\n');
-const preexistingFinding = '{"id":"preexisting","status":"open"}\n';
-const preexistingIndex = '# Existing Findings\n';
-write(path.join(preserveFailureRoot, '.metareview', 'findings.jsonl'), preexistingFinding);
-write(path.join(preserveFailureRoot, 'docs', 'metareview', 'FINDINGS.md'), preexistingIndex);
-const preserveFailureReviewDir = path.join(preserveFailureRoot, 'docs', 'metareview', 'reviews');
-mkdirp(preserveFailureReviewDir);
-fs.chmodSync(preserveFailureReviewDir, 0o555);
-try {
-  createArtifactReview(preserveFailureRoot, 'docs/plan.md', { now });
-} catch (_error) {
-} finally {
-  fs.chmodSync(preserveFailureReviewDir, 0o755);
-}
-assert(fs.readFileSync(path.join(preserveFailureRoot, '.metareview', 'findings.jsonl'), 'utf8') === preexistingFinding, 'pre-existing findings jsonl was removed or changed after write failure');
-assert(fs.readFileSync(path.join(preserveFailureRoot, 'docs', 'metareview', 'FINDINGS.md'), 'utf8') === preexistingIndex, 'pre-existing findings index was removed or changed after write failure');
-
-const preserveContextFailureRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-artifact-preserve-context-fail-'));
-mkdirp(path.join(preserveContextFailureRoot, '.git'));
-write(path.join(preserveContextFailureRoot, 'docs', 'plan.md'), '# Plan\n');
-const preexistingContextPath = path.join(preserveContextFailureRoot, 'docs', 'metareview', 'context', `${expectedRunId}-context.md`);
-write(preexistingContextPath, 'PREEXISTING CONTEXT');
-const preserveContextReviewDir = path.join(preserveContextFailureRoot, 'docs', 'metareview', 'reviews');
-mkdirp(preserveContextReviewDir);
-fs.chmodSync(preserveContextReviewDir, 0o555);
-try {
-  createArtifactReview(preserveContextFailureRoot, 'docs/plan.md', { now });
-} catch (_error) {
-} finally {
-  fs.chmodSync(preserveContextReviewDir, 0o755);
-}
-assert(fs.readFileSync(preexistingContextPath, 'utf8') === 'PREEXISTING CONTEXT', 'pre-existing context pack was removed or changed after write failure');
-
-const stateInitFailureRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-artifact-state-init-fail-'));
-mkdirp(path.join(stateInitFailureRoot, '.git'));
-write(path.join(stateInitFailureRoot, 'docs', 'plan.md'), '# Plan\n');
-write(path.join(stateInitFailureRoot, '.metareview'), 'not a directory\n');
-let stateInitFailureMessage = '';
-try {
-  createArtifactReview(stateInitFailureRoot, 'docs/plan.md', { now });
-} catch (error) {
-  stateInitFailureMessage = error.message;
-}
-assert(stateInitFailureMessage, 'expected state initialization failure');
-assert(fs.readFileSync(path.join(stateInitFailureRoot, '.metareview'), 'utf8') === 'not a directory\n', 'pre-existing .metareview file was changed');
-assert(!fs.existsSync(path.join(stateInitFailureRoot, 'docs', 'metareview', 'reviews', `${expectedRunId}.md`)), 'review log remained after state initialization failed');
-assert(!fs.existsSync(path.join(stateInitFailureRoot, 'docs', 'metareview', 'context', `${expectedRunId}-context.md`)), 'context pack remained after state initialization failed');
-assert(!fs.existsSync(path.join(stateInitFailureRoot, 'docs', 'metareview', 'FINDINGS.md')), 'findings index remained after state initialization failed');
-
-const appendFailureRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-artifact-append-fail-'));
-mkdirp(path.join(appendFailureRoot, '.git'));
-write(path.join(appendFailureRoot, 'docs', 'plan.md'), '# Plan\n');
-mkdirp(path.join(appendFailureRoot, '.metareview', 'runs.jsonl'));
-let appendFailureMessage = '';
-try {
-  createArtifactReview(appendFailureRoot, 'docs/plan.md', { now });
-} catch (error) {
-  appendFailureMessage = error.message;
-}
-assert(appendFailureMessage, 'expected run append failure');
-assert(!fs.existsSync(path.join(appendFailureRoot, 'docs', 'metareview', 'reviews', `${expectedRunId}.md`)), 'review log remained after run append failed');
-assert(!fs.existsSync(path.join(appendFailureRoot, 'docs', 'metareview', 'context', `${expectedRunId}-context.md`)), 'context pack remained after run append failed');
-assert(!fs.existsSync(path.join(appendFailureRoot, '.metareview', 'findings.jsonl')), 'new findings jsonl remained after run append failed');
-assert(!fs.existsSync(path.join(appendFailureRoot, 'docs', 'metareview', 'FINDINGS.md')), 'new findings index remained after run append failed');
-
-const preserveAppendFailureRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-artifact-preserve-append-fail-'));
-mkdirp(path.join(preserveAppendFailureRoot, '.git'));
-write(path.join(preserveAppendFailureRoot, 'docs', 'plan.md'), '# Plan\n');
-write(path.join(preserveAppendFailureRoot, '.metareview', 'findings.jsonl'), preexistingFinding);
-write(path.join(preserveAppendFailureRoot, 'docs', 'metareview', 'FINDINGS.md'), preexistingIndex);
-const preexistingAppendContextPath = path.join(preserveAppendFailureRoot, 'docs', 'metareview', 'context', `${expectedRunId}-context.md`);
-write(preexistingAppendContextPath, 'PREEXISTING APPEND CONTEXT');
-mkdirp(path.join(preserveAppendFailureRoot, '.metareview', 'runs.jsonl'));
-try {
-  createArtifactReview(preserveAppendFailureRoot, 'docs/plan.md', { now });
-} catch (_error) {
-}
-assert(!fs.existsSync(path.join(preserveAppendFailureRoot, 'docs', 'metareview', 'reviews', `${expectedRunId}.md`)), 'review log remained after preserved run append failed');
-assert(fs.readFileSync(path.join(preserveAppendFailureRoot, '.metareview', 'findings.jsonl'), 'utf8') === preexistingFinding, 'pre-existing findings jsonl was removed or changed after append failure');
-assert(fs.readFileSync(path.join(preserveAppendFailureRoot, 'docs', 'metareview', 'FINDINGS.md'), 'utf8') === preexistingIndex, 'pre-existing findings index was removed or changed after append failure');
-assert(fs.readFileSync(preexistingAppendContextPath, 'utf8') === 'PREEXISTING APPEND CONTEXT', 'pre-existing context pack was removed or changed after append failure');
-
-const displayRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-artifact-display-'));
-mkdirp(path.join(displayRoot, '.git'));
-write(path.join(displayRoot, 'docs', 'bad name.md'), '# Plan\n');
-const unsafeTarget = 'docs/bad\nname.md';
-const unsafePath = path.join(displayRoot, unsafeTarget);
-mkdirp(path.dirname(unsafePath));
-fs.renameSync(path.join(displayRoot, 'docs', 'bad name.md'), unsafePath);
-const unsafeResult = createArtifactReview(displayRoot, unsafeTarget, { now });
-const unsafeLog = fs.readFileSync(path.join(displayRoot, unsafeResult.reviewLogPath), 'utf8');
-assert(unsafeLog.includes('Target: `docs/bad name.md`'), 'target display was not sanitized');
-assert(!unsafeLog.includes('Target: `docs/bad\nname.md`'), 'target display contains newline');
-
-const backtickRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-artifact-backtick-'));
-mkdirp(path.join(backtickRoot, '.git'));
-const injectedTarget = 'docs/bad` **INJECTED** `.md';
-write(path.join(backtickRoot, injectedTarget), '# Plan\n');
-const backtickResult = createArtifactReview(backtickRoot, injectedTarget, { now });
-const backtickLog = fs.readFileSync(path.join(backtickRoot, backtickResult.reviewLogPath), 'utf8');
-assert(backtickLog.includes('Target: ``docs/bad` **INJECTED** `.md``'), 'target backticks did not use safe code span delimiters');
-assert(!backtickLog.includes('Target: `docs/bad` **INJECTED** `.md`'), 'target contains raw injectable backticks');
-
-const leadingBacktickRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-artifact-leading-backtick-'));
-mkdirp(path.join(leadingBacktickRoot, '.git'));
-const leadingBacktickTarget = '` **INJECTED** `/plan.md';
-write(path.join(leadingBacktickRoot, leadingBacktickTarget), '# Plan\n');
-const leadingBacktickResult = createArtifactReview(leadingBacktickRoot, leadingBacktickTarget, { now });
-const leadingBacktickLog = fs.readFileSync(path.join(leadingBacktickRoot, leadingBacktickResult.reviewLogPath), 'utf8');
-assert(leadingBacktickLog.includes('Target: `` ` **INJECTED** `/plan.md ``'), 'leading backtick target did not use padded safe code span');
-assert(!leadingBacktickLog.includes('Target: ``` **INJECTED** `/plan.md``'), 'leading backtick target used ambiguous unpadded code span');
-NODE
diff --git a/tests/lib/test-context-pack.sh b/tests/lib/test-context-pack.sh
deleted file mode 100755
index e9252fd..0000000
--- a/tests/lib/test-context-pack.sh
+++ /dev/null
@@ -1,121 +0,0 @@
-#!/usr/bin/env bash
-set -euo pipefail
-
-node - <<'NODE'
-const fs = require('fs');
-const path = require('path');
-const os = require('os');
-const { buildContextPack } = require('./lib/context-pack');
-const { createRunId } = require('./lib/state');
-
-function assert(cond, msg) { if (!cond) throw new Error(msg); }
-function assertThrows(fn, pattern, msg) {
-  try {
-    fn();
-  } catch (error) {
-    assert(pattern.test(String(error.message)), `${msg}: ${error.message}`);
-    return;
-  }
-  throw new Error(msg);
-}
-function mkdirp(p) { fs.mkdirSync(p, { recursive: true }); }
-function write(p, text) { mkdirp(path.dirname(p)); fs.writeFileSync(p, text); }
-
-const root = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-context-'));
-mkdirp(path.join(root, '.git'));
-write(path.join(root, 'docs', 'SERVICE_INVENTORY.md'), '# Service Inventory\n\nExisting services.\n');
-write(path.join(root, '.beads', 'knowledge', 'gotchas.jsonl'), '{"id":"fact-1","fact":"Use constructor injection for services."}\n');
-write(path.join(root, '.beads', 'knowledge', 'newlines.jsonl'), '{"id":"fact-2","fact":"first\\n## injected"}\n');
-write(path.join(root, '.beads', 'knowledge', 'evil\n## injected.jsonl'), '{"id":"fact-3","fact":"filename fact"}\n');
-mkdirp(path.join(root, '.beads', 'knowledge', 'bad.jsonl'));
-write(path.join(root, 'docs', 'spec.md'), '# Spec\n\nBuild a thing.\n');
-write(path.join(root, 'docs', 'fenced.md'), '# Fenced\n\n```markdown\n## Injected\n```\n');
-write(path.join(root, 'docs', 'evil\n## injected.md'), '# Evil target\n');
-write(path.join(root, '` **INJECTED** `target.md'), '# Backtick target\n');
-
-const now = new Date('2026-05-26T12:34:56Z');
-const expectedRunId = createRunId('artifact', 'docs/spec.md', now);
-const result = buildContextPack(root, 'docs/spec.md', { now });
-assert(result.contextPath === `docs/metareview/context/${expectedRunId}-context.md`, result.contextPath);
-
-const fullPath = path.join(root, result.contextPath);
-assert(fs.existsSync(fullPath), 'context pack was not written');
-const content = fs.readFileSync(fullPath, 'utf8');
-assert(content.includes('# metareview context:'), 'missing title');
-assert(content.includes('docs/spec.md'), 'missing target path');
-assert(content.includes('SERVICE_INVENTORY.md'), 'missing service inventory mention');
-assert(content.includes('Use constructor injection'), 'missing knowledge fact');
-assert(content.includes('first ## injected'), 'knowledge newlines should be normalized');
-assert(!content.includes('\n## injected\n'), 'knowledge newline created standalone heading');
-assert(content.includes('- evil ## injected.jsonl: filename fact'), 'knowledge filename should be single-line sanitized');
-assert(!content.includes('\n## injected.jsonl'), 'knowledge filename created standalone heading');
-
-const evilTarget = 'docs/evil\n## injected.md';
-const evilTargetResult = buildContextPack(root, evilTarget, { now });
-const evilTargetContent = fs.readFileSync(path.join(root, evilTargetResult.contextPath), 'utf8');
-assert(evilTargetContent.includes('# metareview context: docs/evil ## injected.md'), 'target title should be single-line sanitized');
-assert(evilTargetContent.includes('- Path: `docs/evil ## injected.md`'), 'target path should be single-line sanitized');
-assert(!evilTargetContent.includes('\n## injected.md'), 'target path created standalone heading');
-
-const backtickTarget = '` **INJECTED** `target.md';
-const backtickTargetResult = buildContextPack(root, backtickTarget, { now });
-const backtickTargetContent = fs.readFileSync(path.join(root, backtickTargetResult.contextPath), 'utf8');
-assert(backtickTargetContent.includes('# metareview context: **INJECTED** target.md'), 'target title should strip backticks to avoid heading ambiguity');
-assert(backtickTargetContent.includes('- Path: `` ` **INJECTED** `target.md ``'), 'target path should use padded safe code span delimiters');
-assert(!backtickTargetContent.includes('- Path: `` ` **INJECTED** `target.md``'), 'target path used ambiguous unpadded code span');
-
-const fencedResult = buildContextPack(root, 'docs/fenced.md', { now });
-const fencedContent = fs.readFileSync(path.join(root, fencedResult.contextPath), 'utf8');
-assert(/## Artifact Excerpt\n\n````+markdown\n# Fenced/.test(fencedContent), 'artifact excerpt should use a fence longer than artifact backticks');
-
-assertThrows(
-  () => buildContextPack(root, 'docs/missing.md', { now }),
-  /Target artifact not found: docs\/missing\.md/,
-  'missing targets should be rejected'
-);
-assertThrows(
-  () => buildContextPack(root, '../outside.md', { now }),
-  /Target artifact is outside repository root: \.\.\/outside\.md/,
-  'path traversal should be rejected'
-);
-
-const outside = path.join(os.tmpdir(), `metareview-outside-${process.pid}.md`);
-fs.writeFileSync(outside, '# Outside\n');
-try {
-  fs.symlinkSync(outside, path.join(root, 'docs', 'linked.md'));
-  assertThrows(
-    () => buildContextPack(root, 'docs/linked.md', { now }),
-    /outside repository root/,
-    'symlink escape should be rejected'
-  );
-} catch (error) {
-  if (!['EACCES', 'EPERM', 'ENOTSUP', 'EOPNOTSUPP'].includes(error.code)) {
-    throw error;
-  }
-} finally {
-  fs.rmSync(outside, { force: true });
-}
-
-const fileKnowledgeRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-context-file-knowledge-'));
-mkdirp(path.join(fileKnowledgeRoot, '.git'));
-write(path.join(fileKnowledgeRoot, '.beads', 'knowledge'), 'not a directory\n');
-write(path.join(fileKnowledgeRoot, 'docs', 'spec.md'), '# Spec\n');
-const fileKnowledgeResult = buildContextPack(fileKnowledgeRoot, 'docs/spec.md', { now });
-const fileKnowledgeContent = fs.readFileSync(path.join(fileKnowledgeRoot, fileKnowledgeResult.contextPath), 'utf8');
-assert(fileKnowledgeContent.includes('No Beads knowledge facts found.'), 'file-shaped knowledge path should be ignored');
-
-const outsideKnowledge = path.join(os.tmpdir(), `metareview-outside-knowledge-${process.pid}.jsonl`);
-fs.writeFileSync(outsideKnowledge, '{"id":"outside","fact":"OUTSIDE_KNOWLEDGE_SENTINEL"}\n');
-try {
-  fs.symlinkSync(outsideKnowledge, path.join(root, '.beads', 'knowledge', 'linked.jsonl'));
-  const linkedKnowledgeResult = buildContextPack(root, 'docs/spec.md', { now });
-  const linkedKnowledgeContent = fs.readFileSync(path.join(root, linkedKnowledgeResult.contextPath), 'utf8');
-  assert(!linkedKnowledgeContent.includes('OUTSIDE_KNOWLEDGE_SENTINEL'), 'outside symlinked knowledge should be ignored');
-} catch (error) {
-  if (!['EACCES', 'EPERM', 'ENOTSUP', 'EOPNOTSUPP'].includes(error.code)) {
-    throw error;
-  }
-} finally {
-  fs.rmSync(outsideKnowledge, { force: true });
-}
-NODE
diff --git a/tests/lib/test-repo-detect.sh b/tests/lib/test-repo-detect.sh
deleted file mode 100644
index b716330..0000000
--- a/tests/lib/test-repo-detect.sh
+++ /dev/null
@@ -1,63 +0,0 @@
-#!/usr/bin/env bash
-set -euo pipefail
-
-ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
-TMP="$(mktemp -d)"
-trap 'rm -rf "$TMP"' EXIT
-
-node - <<'NODE'
-const fs = require('fs');
-const path = require('path');
-const os = require('os');
-const { detectRepo } = require('./lib/repo-detect');
-
-function mkdirp(p) { fs.mkdirSync(p, { recursive: true }); }
-function write(p, text = '') { mkdirp(path.dirname(p)); fs.writeFileSync(p, text); }
-function assert(cond, msg) { if (!cond) throw new Error(msg); }
-
-const root = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-detect-'));
-
-const advisory = path.join(root, 'advisory');
-mkdirp(advisory);
-let report = detectRepo(advisory);
-assert(report.mode === 'advisory', `expected advisory, got ${report.mode}`);
-assert(report.capabilities.git === false, 'advisory fixture should not have git');
-
-const standalone = path.join(root, 'standalone');
-mkdirp(path.join(standalone, '.git'));
-report = detectRepo(standalone);
-assert(report.mode === 'standalone-minimal', `expected standalone-minimal, got ${report.mode}`);
-assert(report.capabilities.git === true, 'standalone fixture should have git');
-
-const negatedMetaswarm = path.join(root, 'negated-metaswarm');
-mkdirp(path.join(negatedMetaswarm, '.git'));
-write(path.join(negatedMetaswarm, 'AGENTS.md'), 'This repository does not use metaswarm.');
-report = detectRepo(negatedMetaswarm);
-assert(report.mode === 'standalone-minimal', `expected standalone-minimal, got ${report.mode}`);
-assert(report.capabilities.metaswarm === false, 'negated metaswarm fixture should not detect metaswarm');
-
-const directoryAgents = path.join(root, 'directory-agents');
-mkdirp(path.join(directoryAgents, '.git'));
-mkdirp(path.join(directoryAgents, 'AGENTS.md'));
-report = detectRepo(directoryAgents);
-assert(report.mode === 'standalone-minimal', `expected standalone-minimal, got ${report.mode}`);
-assert(report.capabilities.metaswarm === false, 'directory AGENTS.md fixture should not detect metaswarm');
-
-const beads = path.join(root, 'beads');
-mkdirp(path.join(beads, '.git'));
-mkdirp(path.join(beads, '.beads', 'knowledge'));
-write(path.join(beads, '.beads', 'issues.jsonl'), '');
-report = detectRepo(beads);
-assert(report.mode === 'standalone-full', `expected standalone-full, got ${report.mode}`);
-assert(report.capabilities.beads === true, 'beads fixture should detect beads');
-
-const metaswarm = path.join(root, 'metaswarm');
-mkdirp(path.join(metaswarm, '.git'));
-mkdirp(path.join(metaswarm, '.beads', 'knowledge'));
-write(path.join(metaswarm, 'AGENTS.md'), 'This repo uses metaswarm workflows.');
-write(path.join(metaswarm, 'docs', 'SERVICE_INVENTORY.md'), '# Service Inventory\n');
-report = detectRepo(metaswarm);
-assert(report.mode === 'metaswarm-extension', `expected metaswarm-extension, got ${report.mode}`);
-assert(report.capabilities.metaswarm === true, 'metaswarm fixture should detect metaswarm');
-assert(report.files.serviceInventory === 'docs/SERVICE_INVENTORY.md', 'service inventory path mismatch');
-NODE
diff --git a/tests/lib/test-state.sh b/tests/lib/test-state.sh
deleted file mode 100644
index 1a58ae9..0000000
--- a/tests/lib/test-state.sh
+++ /dev/null
@@ -1,44 +0,0 @@
-#!/usr/bin/env bash
-set -euo pipefail
-
-node - <<'NODE'
-const fs = require('fs');
-const path = require('path');
-const os = require('os');
-const crypto = require('crypto');
-const { appendJsonl, readJsonl, createRunId, createFindingId } = require('./lib/state');
-
-function assert(cond, msg) { if (!cond) throw new Error(msg); }
-function targetHash(target) {
-  return crypto.createHash('sha1').update(target).digest('hex').slice(0, 8);
-}
-
-const root = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-state-'));
-const file = path.join(root, '.metareview', 'runs.jsonl');
-
-appendJsonl(file, { id: 'one', value: 1 });
-appendJsonl(file, { id: 'two', value: 2 });
-const rows = readJsonl(file);
-assert(rows.length === 2, `expected 2 rows, got ${rows.length}`);
-assert(rows[0].id === 'one', 'first row mismatch');
-assert(rows[1].value === 2, 'second row mismatch');
-
-const target = 'docs/specs/example plan.md';
-const runId = createRunId('artifact', target, new Date('2026-05-26T12:34:56Z'));
-assert(runId === `mrv-20260526-123456000-artifact-example-plan-${targetHash(target)}`, `run id mismatch: ${runId}`);
-
-const findingId = createFindingId(runId, 7);
-assert(findingId === `mrvf-20260526-123456000-artifact-example-plan-${targetHash(target)}-007`, `finding id mismatch: ${findingId}`);
-
-const firstRunId = createRunId('artifact', target, new Date('2026-05-26T12:34:01Z'));
-const secondRunId = createRunId('artifact', target, new Date('2026-05-26T12:34:02Z'));
-assert(firstRunId !== secondRunId, 'run ids should differ for same-minute runs with different seconds');
-
-const firstMsRunId = createRunId('artifact', target, new Date('2026-05-26T12:34:56.001Z'));
-const secondMsRunId = createRunId('artifact', target, new Date('2026-05-26T12:34:56.002Z'));
-assert(firstMsRunId !== secondMsRunId, 'run ids should differ for same-second runs with different milliseconds');
-
-const specApiRunId = createRunId('artifact', 'docs/specs/api/spec.md', new Date('2026-05-26T12:34:56Z'));
-const specDesignRunId = createRunId('artifact', 'docs/design/spec.md', new Date('2026-05-26T12:34:56Z'));
-assert(specApiRunId !== specDesignRunId, 'run ids should differ for same-timestamp targets with same basename');
-NODE
diff --git a/tests/manifest/test-manifests.sh b/tests/manifest/test-manifests.sh
index 842f6da..d622059 100755
--- a/tests/manifest/test-manifests.sh
+++ b/tests/manifest/test-manifests.sh
@@ -51,6 +51,8 @@ if (!JSON.stringify(JSON.parse(fs.readFileSync(".agents/plugins/marketplace.json
   throw new Error("marketplace does not advertise post-merge learning");
 }
 if (pkg.files.includes("lib/")) throw new Error("package still advertises lib/ as shipped runtime");
+if (fs.existsSync("lib")) throw new Error("legacy JS implementation directory must not exist");
+if (fs.existsSync("tests/lib")) throw new Error("legacy JS implementation tests must not exist");
 for (const required of ["bin/", "cmd/", "internal/", "go.mod"]) {
   if (!pkg.files.includes(required)) throw new Error(`package files missing ${required}`);
 }
diff --git a/tests/manifest/test-skills.sh b/tests/manifest/test-skills.sh
index cbb0db0..5e06c2c 100644
--- a/tests/manifest/test-skills.sh
+++ b/tests/manifest/test-skills.sh
@@ -80,6 +80,22 @@ grep -q '^## How Coding Agents Use It$' README.md
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
+if [ -d lib ] || [ -d tests/lib ]; then
+  echo "legacy JS implementation and tests must not exist" >&2
+  exit 1
+fi
 grep -q -- '--scaffold-only' docs/quickstart.md
 grep -q 'metareview review task-done' commands/review-task-done.md
 grep -q 'metareview review epic-ready' skills/review-epic-ready/SKILL.md
diff --git a/tests/run-all.sh b/tests/run-all.sh
index a042ec2..f533380 100755
--- a/tests/run-all.sh
+++ b/tests/run-all.sh
@@ -7,10 +7,6 @@ cd "$ROOT"
 bash tests/manifest/test-manifests.sh
 bash tests/manifest/test-skills.sh
 
-if [ -f tests/lib/test-repo-detect.sh ]; then bash tests/lib/test-repo-detect.sh; fi
-if [ -f tests/lib/test-state.sh ]; then bash tests/lib/test-state.sh; fi
-if [ -f tests/lib/test-context-pack.sh ]; then bash tests/lib/test-context-pack.sh; fi
-if [ -f tests/lib/test-artifact-review.sh ]; then bash tests/lib/test-artifact-review.sh; fi
 if [ -f tests/go/test-cli-baseline.sh ]; then bash tests/go/test-cli-baseline.sh; fi
 if [ -f tests/go/test-npm-wrapper-cwd.sh ]; then bash tests/go/test-npm-wrapper-cwd.sh; fi
 if [ -f tests/go/test-setup-check.sh ]; then bash tests/go/test-setup-check.sh; fi
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
+- The legacy JavaScript implementation is removed so artifact-review scaffold generation has a single Go implementation path; the Go scaffold starts in pending/delegation-intended mode and instructs agents to update execution mode after real reviewer execution.
+- The skill says review execution completion requires populated required reviewer rows, per-reviewer verdicts, and an aggregate verdict other than `NOT_REVIEWED`; artifact readiness still requires zero unresolved blocking findings unless explicitly human-accepted.
+- Quickstart and agent integration docs mention the subagent default, the unavailable-subagent or human-no-delegation fallback trigger, and the weaker-evidence caveat.
+- The skill tells agents to return the actual artifact-review verdict instead of substituting a fixed example result.
+- `bash tests/manifest/test-skills.sh`, `bash tests/manifest/test-manifests.sh`, and `bash tests/go/test-artifact-review.sh` assert the new delegation, fallback, single-implementation, scaffold, completion, and actual-verdict contract text.
--- internal/gitcontext/gitcontext_test.go
+package gitcontext
+
+import "testing"
+
+func TestMaxDiffBytesAccommodatesMediumDeletionReviews(t *testing.T) {
+	if maxDiffBytes < 100_000 {
+		t.Fatalf("maxDiffBytes = %d, want at least 100000 for medium deletion reviews", maxDiffBytes)
+	}
+}
`````

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

Verification evidence:
- bash tests/manifest/test-manifests.sh exited 0 and verified npm build, npm pack dry-run, packaged wrapper, and copied-wrapper behavior
- bash tests/manifest/test-skills.sh exited 0 and verified no legacy JS implementation/tests remain
- bash tests/go/test-artifact-review.sh exited 0
- go test ./internal/gitcontext exited 0 after adding medium deletion review limit regression test
- go test ./... exited 0
- bash tests/run-all.sh exited 0
- git diff --check exited 0

