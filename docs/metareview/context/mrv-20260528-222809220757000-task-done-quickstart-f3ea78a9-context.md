# metareview task-done context

Run ID: `mrv-20260528-222809220757000-task-done-quickstart-f3ea78a9`

## Task

# metareview Quickstart

For full installation paths, see [`../INSTALL.md`](../INSTALL.md). For coding-agent instructions, see [`README.codex.md`](README.codex.md), [`README.claude.md`](README.claude.md), [`../AGENTS.md`](../AGENTS.md), and [`../CLAUDE.md`](../CLAUDE.md).

## 1. Check Mode

Run from the repository root:

```bash
metareview setup --check
```

For standalone use, inspect the dry-run prerequisite plan:

```bash
metareview setup --bootstrap-prereqs --dry-run
```

The dry run does not install Superpowers, Beads, or metaswarm. Non-dry-run bootstrap requires explicit confirmation.

## 2. Run Reviews At The Right Gate

Use the smallest gate that matches the work:

```bash
metareview review artifact <path>
metareview review task-done <task-id-or-path>
metareview review epic-ready <epic-id-or-path>
metareview review pr-ready --base <base-ref>
metareview learn --post-merge <pr-number> --base <pre-merge-ref>
```

`artifact` creates an incomplete review scaffold for specs, plans, and docs. The command exits nonzero while the scaffold is still `NOT_REVIEWED`; complete every required reviewer row and update the verdict before treating the artifact as reviewed. Artifact review runs the five required lenses as parallel subagents by default. Use `in-session-emulated` only when subagents are unavailable or the human explicitly requests no delegation, and state that the review is not independently adversarial and is weaker evidence. Use `--scaffold-only` only when scaffold creation itself is the intended action. `task-done` runs after a local task or chunk claims done. `epic-ready` runs when child tasks are complete. `pr-ready` runs before push or merge readiness. `learn --post-merge` runs after confirmed PR merge.

If a review reports any blocking finding or remains `NOT_REVIEWED`, fix it and re-run with `--previous-run <run-id>` until the result is `PASS` or `PASS_ADVISORY` with zero blockers.

## 3. Metaswarm Fit

When metaswarm, Superpowers, and Beads are present, metaswarm remains the lifecycle owner. Metareview supplies deeper review commands and durable artifacts. The integration contract is in `docs/integrations/metaswarm.md`.

In standalone mode, metareview still runs advisory reviews and can use `.metareview/knowledge/metareview.jsonl` until Beads knowledge is available.

## 4. What To Commit

Commit:

- `docs/metareview/reviews/`
- `docs/metareview/context/`
- `docs/metareview/learning/`
- `.metareview/knowledge/metareview.jsonl` in standalone fallback mode
- `.metareview/calibration.jsonl`
- `.metareview/learning-runs.jsonl`
- `.beads/knowledge/metareview.jsonl` when Beads exists

Keep local:

- `.metareview/findings.jsonl`
- `.metareview/runs.jsonl`
- other transient `.metareview/` state

For ordinary project repositories, use exact file entries for transient state. Do not ignore `docs/metareview/` or the entire `.metareview/` directory, because those patterns hide durable review, learning, calibration, or fallback knowledge artifacts.

```gitignore
.metareview/findings.jsonl
.metareview/runs.jsonl
```

The repository `.gitignore` keeps transient state local while allowing fallback learning knowledge and calibration to sync through git.

## 5. Agent Syntax

Codex users invoke metareview through `$setup`, `$review-artifact`, `$review-task-done`, `$review-epic-ready`, `$review-pr-ready`, `$learn-post-merge`, and `$status`.

Claude Code users invoke the same workflows through `/setup`, `/review-artifact`, `/review-task-done`, `/review-epic-ready`, `/review-pr-ready`, `/learn-post-merge`, and `/status`.

Direct CLI usage remains the source of truth when plugin skills are unavailable.


## Git

- Base: `5d6c4124b44d6f4bf4efaf95aed4bc9cfa5e0ec6`
- Head: `5d6c4124b44d6f4bf4efaf95aed4bc9cfa5e0ec6`
- Branch: `main`
- Gate effect: `gate`

## Changed Files

- .claude-plugin/marketplace.json
- .claude-plugin/plugin.json
- .codex-plugin/plugin.json
- INSTALL.md
- README.md
- docs/quickstart.md
- internal/version/version.go
- package.json
- tests/manifest/test-skills.sh

## Diff

````diff


diff --git a/.claude-plugin/marketplace.json b/.claude-plugin/marketplace.json
index ce01893..1dd7f72 100644
--- a/.claude-plugin/marketplace.json
+++ b/.claude-plugin/marketplace.json
@@ -8,7 +8,7 @@
     {
       "name": "metareview",
       "description": "Internal review harness, adversarial gates, and post-merge learning for coding agents",
-      "version": "0.3.0",
+      "version": "0.3.1",
       "source": "./",
       "author": {
         "name": "David Sifry"
diff --git a/.claude-plugin/plugin.json b/.claude-plugin/plugin.json
index 49357db..18815c4 100644
--- a/.claude-plugin/plugin.json
+++ b/.claude-plugin/plugin.json
@@ -1,6 +1,6 @@
 {
   "name": "metareview",
-  "version": "0.3.0",
+  "version": "0.3.1",
   "description": "Go-based metaswarm-compatible internal review harness for plans, specs, decompositions, task-done code review, acceptance evidence, PR readiness, and post-merge learning. Packaged releases use bin/metareview; source checkout mode requires Go 1.22+.",
   "author": {
     "name": "David Sifry"
diff --git a/.codex-plugin/plugin.json b/.codex-plugin/plugin.json
index 7c04614..49c9e1e 100644
--- a/.codex-plugin/plugin.json
+++ b/.codex-plugin/plugin.json
@@ -1,6 +1,6 @@
 {
   "name": "metareview",
-  "version": "0.3.0",
+  "version": "0.3.1",
   "description": "Go-based metaswarm-compatible internal review harness for plans, specs, decompositions, task-done code review, acceptance evidence, PR readiness, and post-merge learning",
   "author": {
     "name": "David Sifry"
diff --git a/INSTALL.md b/INSTALL.md
index e54eab3..eb690cf 100644
--- a/INSTALL.md
+++ b/INSTALL.md
@@ -115,7 +115,14 @@ metareview review pr-ready --base <base-ref>
 metareview learn --post-merge <pr-number> --base <pre-merge-ref>
 ```
 
-Commit durable Markdown artifacts under `docs/metareview/`. Keep transient `.metareview/findings.jsonl` and `.metareview/runs.jsonl` local unless a future contract says otherwise.
+Commit durable Markdown artifacts under `docs/metareview/`. Keep transient `.metareview/findings.jsonl` and `.metareview/runs.jsonl` local unless a future contract says otherwise. In ordinary project repositories, prefer exact `.gitignore` entries:
+
+```gitignore
+.metareview/findings.jsonl
+.metareview/runs.jsonl
+```
+
+Do not ignore `docs/metareview/` or the whole `.metareview/` directory.
 
 ## Update
 
diff --git a/README.md b/README.md
index 8676b03..596700f 100644
--- a/README.md
+++ b/README.md
@@ -199,6 +199,13 @@ Coding agents should treat metareview as a completion gate, not an optional comm
 
 Agents must not say work is done while a blocking finding remains unresolved. They should commit durable review/context artifacts when the repository's artifact policy says to do so, and keep transient `.metareview/findings.jsonl` and `.metareview/runs.jsonl` local.
 
+When configuring `.gitignore` in ordinary project repositories, ignore those transient files with exact file entries. Do not ignore `docs/metareview/` or the whole `.metareview/` directory, because durable learning, calibration, and fallback knowledge can live there:
+
+```gitignore
+.metareview/findings.jsonl
+.metareview/runs.jsonl
+```
+
 ## Core Commands
 
 ```bash
diff --git a/docs/quickstart.md b/docs/quickstart.md
index 0a1a188..6f0239a 100644
--- a/docs/quickstart.md
+++ b/docs/quickstart.md
@@ -58,6 +58,13 @@ Keep local:
 - `.metareview/runs.jsonl`
 - other transient `.metareview/` state
 
+For ordinary project repositories, use exact file entries for transient state. Do not ignore `docs/metareview/` or the entire `.metareview/` directory, because those patterns hide durable review, learning, calibration, or fallback knowledge artifacts.
+
+```gitignore
+.metareview/findings.jsonl
+.metareview/runs.jsonl
+```
+
 The repository `.gitignore` keeps transient state local while allowing fallback learning knowledge and calibration to sync through git.
 
 ## 5. Agent Syntax
diff --git a/internal/version/version.go b/internal/version/version.go
index ee59a7c..6e03062 100644
--- a/internal/version/version.go
+++ b/internal/version/version.go
@@ -1,3 +1,3 @@
 package version
 
-const Version = "0.3.0"
+const Version = "0.3.1"
diff --git a/package.json b/package.json
index 078a071..534be7e 100644
--- a/package.json
+++ b/package.json
@@ -1,6 +1,6 @@
 {
   "name": "metareview",
-  "version": "0.3.0",
+  "version": "0.3.1",
   "description": "Go-based metaswarm-compatible internal review harness for plans, specs, decompositions, code, acceptance evidence, PR readiness, and post-merge learning",
   "bin": {
     "metareview": "cli/metareview.js"
diff --git a/tests/manifest/test-skills.sh b/tests/manifest/test-skills.sh
index 5e06c2c..ac80c71 100644
--- a/tests/manifest/test-skills.sh
+++ b/tests/manifest/test-skills.sh
@@ -29,8 +29,14 @@ grep -q 'metareview learn --post-merge <pr-number> --base <pre-merge-ref>' docs/
 grep -q '.metareview/findings.jsonl' docs/quickstart.md
 grep -q '.metareview/knowledge/metareview.jsonl' docs/quickstart.md
 grep -q 'docs/metareview/reviews/' docs/quickstart.md
+grep -q 'Do not ignore `docs/metareview/`' docs/quickstart.md
+grep -q 'exact file entries' docs/quickstart.md
 grep -q 'metaswarm remains the lifecycle owner' docs/quickstart.md
 grep -q 'docs/quickstart.md' README.md
+grep -q 'Do not ignore `docs/metareview/`' README.md
+grep -q 'whole `.metareview/` directory' README.md
+grep -q 'Do not ignore `docs/metareview/`' INSTALL.md
+grep -q 'whole `.metareview/` directory' INSTALL.md
 grep -q '^## Use Cases$' README.md
 grep -q 'Spec review' README.md
 grep -q 'Plan review' README.md

````

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

Verification evidence:
- go run ./cmd/metareview --version exited 0 and printed 0.3.1
- go test ./... exited 0
- bash tests/run-all.sh exited 0
- bash tests/manifest/test-manifests.sh exited 0
- bash tests/manifest/test-skills.sh exited 0
- bash tests/go/test-artifact-review.sh exited 0
- git diff --check exited 0
- npm pack --dry-run exited 0 and reported metareview@0.3.1 with docs/quickstart.md, INSTALL.md, bin/metareview, and cli/metareview.js

